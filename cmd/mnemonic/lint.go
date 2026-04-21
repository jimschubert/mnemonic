package main

import (
	"fmt"
	"io"
	"log/slog"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jimschubert/mnemonic/internal/config"
	"github.com/jimschubert/mnemonic/internal/controller"
	"github.com/jimschubert/mnemonic/internal/lint"
	"github.com/jimschubert/mnemonic/internal/store/yamlstore"
)

// LintCmd provides an interactive TUI to resolve memory health issues.
type LintCmd struct {
	GlobalDir string   `short:"g" default:"~/.mnemonic" help:"Directory for global data" env:"MNEMONIC_GLOBAL_DIR"`
	LocalDir  string   `short:"l" default:".mnemonic" help:"Directory for project data" env:"MNEMONIC_LOCAL_DIR"`
	Team      []string `short:"t" help:"Team data directories (repeatable); scope will become team:<basename>" env:"MNEMONIC_TEAM_DIRS" sep:","`
	Threshold float64  `default:"0.90" help:"Similarity threshold for merge suggestions"`

	Embedding embeddable `embed:"" prefix:"embedding-"`
}

//goland:noinspection GoUnhandledErrorResult
func (c *LintCmd) Run(logger *slog.Logger, conf config.Config) error {
	c.Embedding.applyConfig(&conf)

	noopLog := slog.New(slog.NewTextHandler(io.Discard, nil))
	if conf.Embeddings.Endpoint == "" || conf.Embeddings.Model == "" {
		return fmt.Errorf("embeddings not available: endpoint and model must be configured")
	}

	scopes := createScopes(c.GlobalDir, c.LocalDir, c.Team)
	ys, err := yamlstore.New(scopes, noopLog, yamlstore.WithAutoHitCounting(false))
	if err != nil {
		return fmt.Errorf("creating YAML store: %w", err)
	}

	ctrl, err := controller.New(conf,
		controller.WithStore(ys),
		controller.WithLogger(noopLog),
		controller.WithSkipInitialSync(false),
		controller.WithMnemonicDir(c.GlobalDir),
	)
	if err != nil {
		return err
	}
	defer ctrl.Close() // nolint:errcheck

	l := lint.New(ctrl)
	actions, err := l.Analyze(c.Threshold)
	if err != nil {
		return fmt.Errorf("analyzing store: %w", err)
	}

	if len(actions) == 0 {
		fmt.Println("No memory health issues found.")
		return nil
	}

	p := tea.NewProgram(newLintModel(actions, ctrl))
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("running linter TUI: %w", err)
	}

	return nil
}

type lintModel struct {
	actions  []lint.Action
	index    int
	ctrl     *controller.MemoryController
	err      error
	finished bool
}

func newLintModel(actions []lint.Action, ctrl *controller.MemoryController) lintModel {
	return lintModel{
		actions: actions,
		ctrl:    ctrl,
	}
}

func (m lintModel) Init() tea.Cmd {
	return nil
}

func (m lintModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "k", " ":
			// keep
			m.index++
		case "m":
			// merge A into B
			action := m.actions[m.index]
			if err := m.ctrl.Merge(action.Right.ID, action.Left.ID); err != nil {
				m.err = err
				return m, tea.Quit
			}
			m.index++
		case "n":
			// merge B into A
			action := m.actions[m.index]
			if err := m.ctrl.Merge(action.Left.ID, action.Right.ID); err != nil {
				m.err = err
				return m, tea.Quit
			}
			m.index++
		case "f":
			// delete B
			action := m.actions[m.index]
			if err := m.ctrl.Delete(action.Right.ID); err != nil {
				m.err = err
				return m, tea.Quit
			}
			m.index++
		case "d":
			// delete A
			action := m.actions[m.index]
			if err := m.ctrl.Delete(action.Left.ID); err != nil {
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
func (m lintModel) truncateTo(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func (m lintModel) View() string {
	if m.err != nil {
		return fmt.Sprintf("Error: %v\n", m.err)
	}
	if m.finished {
		return "Linting complete!\n"
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

	s += keyStyle.Render("[m]") + " merge A into B | " +
		keyStyle.Render("[n]") + " merge B into A | " +
		keyStyle.Render("[d]") + " delete A | " +
		keyStyle.Render("[f]") + " delete B | " +
		keyStyle.Render("[k]") + " keep both | " +
		keyStyle.Render("[q]") + " quit\n"

	return s
}
