package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/jimschubert/mnemonic/internal/compact"
	"github.com/jimschubert/mnemonic/internal/config"
	"github.com/jimschubert/mnemonic/internal/controller"
	"github.com/jimschubert/mnemonic/internal/embed"
	"github.com/jimschubert/mnemonic/internal/logging"
	"github.com/jimschubert/mnemonic/internal/store/yamlstore"
)

// CompactCmd allows for compacting a memory by querying an LLM to reduce overall token size of the memory without losing detail.
type CompactCmd struct {
	GlobalDir string   `short:"g" default:"~/.mnemonic" help:"Directory for global data" env:"MNEMONIC_GLOBAL_DIR"`
	LocalDir  string   `short:"l" default:".mnemonic" help:"Directory for project data" env:"MNEMONIC_LOCAL_DIR"`
	Team      []string `short:"t" help:"Team data directories (repeatable); scope will become team:<basename>" env:"MNEMONIC_TEAM_DIRS" sep:","`

	BaseURL string `default:"http://localhost:1234/v1" help:"Base URL of OpenAI-compatible /chat/completions API" env:"OPENAI_BASE_URL"`
	ApiKey  string `default:"" help:"API key to access BaseURL" env:"OPENAI_API_KEY"`
	Model   string `required:"" help:"Model to use for compaction"`

	Caveman string `name:"caveman" default:"off" enum:"off,lite,full,ultra" help:"Caveman mode selection for more aggressive compaction (see https://juliusbrussee.github.io/caveman/)"`
	Yes     bool   `help:"Skip the destructive caveman confirmation prompt"`

	Embedding embeddable `embed:"" prefix:"embedding-"`
}

var (
	cavemanWarningStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FF5F5F"))
	cavemanPromptStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFD166"))
	cavemanNoteStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#AAAAAA"))
)

// confirmCavemanMode prompts the user b/c caveman could be seen as negatively destructive.
func confirmCavemanMode(in io.Reader, out io.Writer, mode compact.CavemanMode, timeout time.Duration) error {
	if _, err := fmt.Fprintf(out,
		"%s\n%s\n",
		cavemanWarningStyle.Render(fmt.Sprintf("Caveman mode (%s) is DESTRUCTIVE and can't be undone.", mode)),
		cavemanNoteStyle.Render("Ensure you have committed your global/project/team memories before continuing."),
	); err != nil {
		return fmt.Errorf("writing caveman warning: %w", err)
	}

	if _, err := fmt.Fprintf(out, "%s ", cavemanPromptStyle.Render("ARE YOU SURE? y/N")); err != nil {
		return fmt.Errorf("writing caveman prompt: %w", err)
	}

	type response struct {
		line string
		err  error
	}

	responses := make(chan response, 1)
	go func() {
		line, err := bufio.NewReader(in).ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			responses <- response{err: err}
			return
		}
		responses <- response{line: line}
	}()

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-timer.C:
		_, _ = fmt.Fprintln(out)
		return errors.New("timed out waiting for caveman confirmation")
	case resp := <-responses:
		if _, err := fmt.Fprintln(out); err != nil {
			return fmt.Errorf("can no longer write to output: %w", err)
		}
		if resp.err != nil {
			return fmt.Errorf("reading caveman confirmation: %w", resp.err)
		}

		answer := strings.ToLower(strings.TrimSpace(resp.line))
		if answer != "y" && answer != "yes" {
			return errors.New("caveman compaction aborted")
		}

		return nil
	}
}

func (c *CompactCmd) shouldConfirmCavemanMode(mode compact.CavemanMode) bool {
	return mode != compact.CavemanOff && !c.Yes
}

//goland:noinspection GoUnhandledErrorResult
func (c *CompactCmd) Run(logger *slog.Logger, conf config.Config) error {
	c.Embedding.applyConfig(&conf)

	cavemanMode := compact.ParseCavemanMode(c.Caveman)
	if c.shouldConfirmCavemanMode(cavemanMode) {
		// "You sure about that?" - Tim Robinson
		if err := confirmCavemanMode(os.Stdin, os.Stdout, cavemanMode, 30*time.Second); err != nil {
			return err
		}
	}

	scopes := createScopes(c.GlobalDir, c.LocalDir, c.Team)
	ys, err := yamlstore.New(scopes,
		logging.ForScope(conf, "store"),
		yamlstore.WithAutoHitCounting(false),
	)
	if err != nil {
		return fmt.Errorf("creating YAML store: %w", err)
	}

	ctrl, err := controller.New(conf,
		controller.WithStore(ys),
		controller.WithLogger(logger),
		controller.WithSkipInitialSync(true),
		controller.WithMnemonicDir(c.GlobalDir),
	)
	if err != nil {
		return fmt.Errorf("creating controller: %w", err)
	}
	defer ctrl.Close() //nolint:errcheck

	compacter := compact.New(embed.New(conf), c.BaseURL, c.ApiKey, c.Model,
		compact.WithLogger(logging.ForScope(conf, "compact")),
		compact.WithCavemanMode(cavemanMode),
	)
	defer compacter.Close() // nolint:errcheck

	entries, err := ctrl.All(slices.Collect(maps.Keys(scopes)))
	if err != nil {
		return fmt.Errorf("retrieving entries: %w", err)
	}

	for _, entry := range entries {
		maybeNew, err := compacter.Compact(entry.Content)
		if err != nil {
			logger.Warn("compaction failed for entry", "id", entry.ID, "err", err)
			continue
		}
		if maybeNew != entry.Content {
			entry.Content = maybeNew
			if err := ctrl.Save(&entry); err != nil {
				logger.Warn("failed to update entry after compaction", "id", entry.ID, "err", err)
				continue
			}
		}
	}

	return ctrl.BuildIndexes(true)
}
