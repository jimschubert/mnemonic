package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/jimschubert/mnemonic/internal/config"
	"github.com/jimschubert/mnemonic/internal/daemon"
)

// StdioCmd serves MCP over stdio, auto-starting the daemon if it is not already running.
type StdioCmd struct {
	GlobalDir  string   `short:"g" default:"~/.mnemonic" help:"Directory for global data" env:"MNEMONIC_GLOBAL_DIR"`
	LocalDir   string   `short:"l" default:".mnemonic" help:"Directory for project data" env:"MNEMONIC_LOCAL_DIR"`
	Team       []string `short:"t" help:"Team data directories (repeatable); scope will become team:<basename>" env:"MNEMONIC_TEAM_DIRS" sep:","`
	Mandatory  []string `short:"m" help:"Additional mandatory categories beyond the defaults (avoidance, security)" env:"MNEMONIC_MANDATORY" sep:","`
	ServerAddr string   `short:"a" default:"${server_addr}" help:"Address to listen on for MCP requests" env:"MNEMONIC_SERVER_ADDR"`
}

func (c *StdioCmd) Run(logger *slog.Logger, conf config.Config) error {
	// explicitly assign because conf.ApplyOverrides ignores empty strings
	conf.ServerAddr = c.ServerAddr

	extraEnv := c.daemonEnv()

	if err := ensureDaemon(logger, conf, extraEnv); err != nil {
		return fmt.Errorf("ensuring daemon: %w", err)
	}

	logger.Info("starting stdio bridge")
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := daemon.RunStdioServer(ctx, conf); err != nil {
		logger.Error("stdio bridge exited with error", "err", err)
		return err
	}

	logger.Info("stdio bridge exited successfully")
	return nil
}

// daemonEnv builds config via environment variables for running the daemon.
func (c *StdioCmd) daemonEnv() []string {
	env := []string{
		"MNEMONIC_GLOBAL_DIR=" + c.GlobalDir,
		"MNEMONIC_LOCAL_DIR=" + c.LocalDir,
	}
	if len(c.Team) > 0 {
		env = append(env, "MNEMONIC_TEAM_DIRS="+strings.Join(c.Team, ","))
	}
	if len(c.Mandatory) > 0 {
		env = append(env, "MNEMONIC_MANDATORY="+strings.Join(c.Mandatory, ","))
	}
	if c.ServerAddr != "" {
		env = append(env, "MNEMONIC_SERVER_ADDR="+c.ServerAddr)
	}
	return env
}
