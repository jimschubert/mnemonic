package main

import (
	"fmt"
	"io"
	"log/slog"
	"maps"
	"os"
	"slices"
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/jimschubert/mnemonic/internal/config"
	"github.com/jimschubert/mnemonic/internal/controller"
	"github.com/jimschubert/mnemonic/internal/daemon"
	"github.com/jimschubert/mnemonic/internal/lint"
	"github.com/jimschubert/mnemonic/internal/logging"
	"github.com/jimschubert/mnemonic/internal/store"
	"github.com/jimschubert/mnemonic/internal/store/sqlitestore"
	"github.com/jimschubert/mnemonic/internal/store/yamlstore"
)

// lintKeyMap defines a set of keybindings. To work for help it must satisfy
// key.Map. It could also very easily be a map[string]key.Binding.
type lintKeyMap struct {
	MergeA   key.Binding
	MergeB   key.Binding
	KeepBoth key.Binding
	DeleteA  key.Binding
	DeleteB  key.Binding
	Help     key.Binding
	Quit     key.Binding
}

// ShortHelp returns keybindings to be shown in the mini help view. It's part
// of the key.Map interface.
func (k lintKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.MergeA, k.MergeB, k.KeepBoth, k.DeleteA, k.DeleteB, k.Quit}
}

// FullHelp returns keybindings for the expanded help view. It's part of the
// key.Map interface.
func (k lintKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.MergeA, k.MergeB, k.KeepBoth, k.DeleteA, k.DeleteB}, // first column
		{k.Help, k.Quit}, // second column
	}
}

var lintKeys = lintKeyMap{
	MergeA: key.NewBinding(
		key.WithKeys("m", "right"),
		key.WithHelp("m/→", "merge A → B"),
	),
	MergeB: key.NewBinding(
		key.WithKeys("n", "left"),
		key.WithHelp("n/←", "merge B → A"),
	),
	DeleteA: key.NewBinding(
		key.WithKeys("d"),
		key.WithHelp("d", "del A"),
	),
	DeleteB: key.NewBinding(
		key.WithKeys("f"),
		key.WithHelp("f", "del B"),
	),
	KeepBoth: key.NewBinding(
		key.WithKeys("k"),
		key.WithHelp("k", "keep A+B"),
	),
	Help: key.NewBinding(
		key.WithKeys("?"),
		key.WithHelp("?", "toggle help"),
	),
	Quit: key.NewBinding(
		key.WithKeys("q", "esc", "ctrl+c"),
		key.WithHelp("q", "quit"),
	),
}

// LintCmd provides an interactive TUI to resolve memory health issues.
type LintCmd struct {
	GlobalDir string   `short:"g" default:"~/.mnemonic" help:"Directory for global data" env:"MNEMONIC_GLOBAL_DIR"`
	LocalDir  string   `short:"l" default:".mnemonic" help:"Directory for project data" env:"MNEMONIC_LOCAL_DIR"`
	Team      []string `short:"t" help:"Team data directories (repeatable); scope will become team:<basename>" env:"MNEMONIC_TEAM_DIRS" sep:","`
	Threshold float64  `default:"0.90" help:"Similarity threshold for merge suggestions"`

	Embedding embeddable `embed:"" prefix:"embedding-"`
	Store     storeFlags `embed:"" prefix:"store-"`
}

//goland:noinspection GoUnhandledErrorResult
func (c *LintCmd) Run(logger *slog.Logger, conf config.Config) error {
	c.Embedding.applyConfig(&conf)
	c.Store.applyConfig(&conf)

	noopLog := slog.New(slog.NewTextHandler(io.Discard, nil))
	if conf.Embeddings.Endpoint == "" || conf.Embeddings.Model == "" {
		return fmt.Errorf("embeddings not available: endpoint and model must be configured")
	}

	scopes := createScopes(c.GlobalDir, c.LocalDir, c.Team)
	var (
		ctrl    *controller.MemoryController
		backend entryStoreBackend
		err     error
	)

	if daemon.IsRunning(conf) {
		backend = daemonEntryStoreBackend{client: newDaemonAdminClient(conf)}
		entries, err := backend.All(slices.Collect(maps.Keys(scopes)))
		if err != nil {
			return fmt.Errorf("retrieving daemon entries: %w", err)
		}

		// need a snapshot-only index, so don't use globaldir here
		snapshotDir, cleanupSnapshotDir, err := newSnapshotTempDir("mnemonic-lint-")
		if err != nil {
			return err
		}

		ctrl, err = controller.New(conf,
			controller.WithStore(store.NewSnapshotStore(entries)),
			controller.WithLogger(noopLog),
			controller.WithSkipInitialSync(true),
			controller.WithMnemonicDir(snapshotDir),
		)
		defer func() {
			if ctrl != nil {
				_ = ctrl.Close()
			}
			cleanupSnapshotDir()
		}()
		if err != nil {
			return fmt.Errorf("creating snapshot controller: %w", err)
		}
		if err := ctrl.BuildIndexes(true); err != nil {
			return fmt.Errorf("building daemon-backed index: %w", err)
		}
	} else {
		var st store.Store
		var err error
		switch conf.Store.Type {
		case "sqlite":
			sqlitePath := conf.SQLiteStorePath()
			logger.Info("using SQLite store for linting", "store_type", "sqlite", "sqlite_path", sqlitePath)
			st, err = sqlitestore.New(sqlitePath,
				logging.ForScope(conf, "store"),
				sqlitestore.WithConfiguredScopes(slices.Collect(maps.Keys(scopes))),
				sqlitestore.WithAutoHitCounting(false),
			)
		default:
			logger.Info("using YAML store for linting", "store_type", "yaml")
			st, err = yamlstore.New(scopes, logging.ForScope(conf, "store"), yamlstore.WithAutoHitCounting(false))
		}

		if err != nil {
			return err
		}
		ctrl, err = controller.New(conf,
			controller.WithStore(st),
			controller.WithLogger(noopLog),
			controller.WithSkipInitialSync(false),
			controller.WithMnemonicDir(c.GlobalDir),
		)
		if err != nil {
			return err
		}
		backend = ctrl
		defer ctrl.Close()
	}

	l := lint.New(ctrl)
	actions, err := l.Analyze(c.Threshold)
	if err != nil {
		return fmt.Errorf("analyzing store: %w", err)
	}

	if len(actions) == 0 {
		fmt.Println("No memory health issues found.")
		return nil
	}

	p := tea.NewProgram(newLintModel(actions, backend))
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("running linter TUI: %w", err)
	}

	return nil
}

func newSnapshotTempDir(pattern string) (string, func(), error) {
	dir, err := os.MkdirTemp("", pattern)
	if err != nil {
		return "", nil, fmt.Errorf("creating temporary lint index dir: %w", err)
	}
	return dir, func() {
		_ = os.RemoveAll(dir)
	}, nil
}

type lintModel struct {
	actions      []lint.Action
	index        int
	backend      entryStoreBackend
	keys         lintKeyMap
	help         help.Model
	showFullHelp bool

	err      error
	finished bool
}

func newLintModel(actions []lint.Action, backend entryStoreBackend) lintModel {
	return lintModel{
		actions: actions,
		backend: backend,
		keys:    lintKeys,
		help:    help.New(),
	}
}

func (m lintModel) Init() tea.Cmd {
	return nil
}

func (m lintModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// nolint:gocritic
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, m.keys.Quit):
			return m, tea.Quit
		case key.Matches(msg, m.keys.Help):
			m.showFullHelp = !m.showFullHelp
		case key.Matches(msg, m.keys.KeepBoth):
			m.index++
		case key.Matches(msg, m.keys.MergeA):
			action := m.actions[m.index]
			if err := m.backend.Merge(action.Right.ID, action.Left.ID); err != nil {
				m.err = err
				return m, tea.Quit
			}
			m.index++
		case key.Matches(msg, m.keys.MergeB):
			action := m.actions[m.index]
			if err := m.backend.Merge(action.Left.ID, action.Right.ID); err != nil {
				m.err = err
				return m, tea.Quit
			}
			m.index++
		case key.Matches(msg, m.keys.DeleteB):
			action := m.actions[m.index]
			if err := m.backend.Delete(action.Right.ID); err != nil {
				m.err = err
				return m, tea.Quit
			}
			m.index++
		case key.Matches(msg, m.keys.DeleteA):
			action := m.actions[m.index]
			if err := m.backend.Delete(action.Left.ID); err != nil {
				m.err = err
				return m, tea.Quit
			}
			m.index++
		}
	}

	if m.index >= len(m.actions) {
		m.finished = true
		return m, tea.Quit
	}

	return m, nil
}

func (m lintModel) View() tea.View {
	if m.err != nil {
		return tea.NewView(fmt.Sprintf("Error: %v\n", m.err))
	}
	if m.finished {
		return tea.NewView("Linting complete!\n")
	}

	action := m.actions[m.index]

	s := fmt.Sprintf("\nProposed Action (%d/%d):\n", m.index+1, len(m.actions))
	s += titleStyle.Render(string(action.Type)) + "\n\n"

	s += "Entry A: "
	s += entryMetaStyle.Render(fmt.Sprintf("%s %v", action.Left.ID, action.Left.Tags)) + "\n"
	s += entryStyle.Render(m.truncateTo(action.Left.Content, 200)) + "\n\n"

	s += "Entry B: "
	s += entryMetaStyle.Render(fmt.Sprintf("%s %v", action.Right.ID, action.Right.Tags)) + "\n"
	s += entryStyle.Render(m.truncateTo(action.Right.Content, 200)) + "\n\n"

	s += fmt.Sprintf("Similarity Score: %s\n\n", simStyle.Render(fmt.Sprintf("%.2f%%", action.Similarity*100)))

	var helpView string
	if m.showFullHelp {
		helpView = m.help.FullHelpView(m.keys.FullHelp())
	} else {
		helpView = m.help.ShortHelpView(m.keys.ShortHelp())
	}

	height := 8 - strings.Count(s, "\n") - strings.Count(helpView, "\n")
	if height < 0 {
		height = 1
	}

	return tea.NewView(s + strings.Repeat("\n", height) + helpView)
}

func (m lintModel) truncateTo(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
