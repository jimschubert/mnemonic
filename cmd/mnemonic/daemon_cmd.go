package main

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"

	"github.com/jimschubert/mnemonic/internal/config"
	"github.com/jimschubert/mnemonic/internal/controller"
	"github.com/jimschubert/mnemonic/internal/daemon"
	"github.com/jimschubert/mnemonic/internal/logging"
	"github.com/jimschubert/mnemonic/internal/store"
	"github.com/jimschubert/mnemonic/internal/store/yamlstore"
	"github.com/muesli/reflow/wordwrap"
)

// DaemonCmd groups daemon lifecycle subcommands. Running `daemon` alone starts the daemon (default).
type DaemonCmd struct {
	Start  DaemonStartCmd  `cmd:"start" default:"withargs" help:"Start the background daemon process (default)"`
	Stop   DaemonStopCmd   `cmd:"" help:"Send a shutdown request to a running daemon"`
	Status DaemonStatusCmd `cmd:"" help:"Show daemon status (socket path, uptime, version)"`
}

func (c *DaemonCmd) Help() string {
	help := `
The daemon manages the YAML store and exposes it via MCP and admin APIs over a Unix socket. 
It starts automatically for commands like 'mnemonic server' or 'mnemonic stdio', and can 
be manually started with 'mnemonic daemon start'. Attach a stateless frontend like server 
or stdio later. 
	
Running the command without a subcommand defaults to starting the daemon.
`

	return wordwrap.String(help, 80)
}

// DaemonStartCmd starts the background daemon managing the YAML store and Unix socket.
type DaemonStartCmd struct {
	GlobalDir     string   `short:"g" default:"~/.mnemonic" help:"Directory for global data" env:"MNEMONIC_GLOBAL_DIR"`
	LocalDir      string   `short:"l" default:".mnemonic" help:"Directory for project data" env:"MNEMONIC_LOCAL_DIR"`
	Team          []string `short:"t" help:"Team data directories (repeatable); scope will become team:<basename>" env:"MNEMONIC_TEAM_DIRS" sep:","`
	Mandatory     []string `help:"Additional mandatory categories beyond the defaults (avoidance, security)" env:"MNEMONIC_MANDATORY" sep:","`
	SkipIndexSync bool     `help:"Skip initial index sync on startup; use when restarting or invoking embedding manually" env:"MNEMONIC_SKIP_INDEX_SYNC"`

	Embedding embeddable `embed:"" prefix:"embedding-"`
}

func (c *DaemonStartCmd) Run(logger *slog.Logger, conf config.Config) error {
	c.Embedding.applyConfig(&conf)

	store.WithAdditionalMandatoryCategories(c.Mandatory)

	scopes := createScopes(c.GlobalDir, c.LocalDir, c.Team)
	ys, err := yamlstore.New(scopes, logging.ForScope(conf, "store"))
	if err != nil {
		return fmt.Errorf("creating YAML store: %w", err)
	}

	ctrl, err := controller.New(conf,
		controller.WithStore(ys),
		controller.WithLogger(logging.ForScope(conf, "controller")),
		controller.WithSkipInitialSync(c.SkipIndexSync),
		controller.WithMnemonicDir(c.GlobalDir),
	)
	if err != nil {
		return err
	}

	// For the daemon command, we do not listen on TCP by default.
	// We clear ServerAddr so the daemon package only listens on the Unix socket.
	conf.ServerAddr = ""

	d := daemon.New(ctrl, conf, logging.ForScope(conf, "daemon"))
	logger.Info("starting daemon", "socket", conf.SocketPath())
	return d.Start(context.Background())
}

func createScopes(globalDir string, localDir string, teams []string) map[store.Scope]string {
	scopes := map[store.Scope]string{
		store.ScopeGlobal: filepath.Join(globalDir, "global"),
		"project":         filepath.Join(localDir, "project"),
	}
	for _, dir := range teams {
		scope := store.Scope("team:" + filepath.Base(dir))
		scopes[scope] = dir
	}
	return scopes
}
