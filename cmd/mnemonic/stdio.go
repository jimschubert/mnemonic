package main

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/jimschubert/mnemonic/internal/config"
	"github.com/jimschubert/mnemonic/internal/daemon"
)

// StdioCmd serves MCP over stdio, auto-starting the daemon if it is not already running.
type StdioCmd struct {
	GlobalDir  string   `short:"g" default:"~/.mnemonic" help:"Directory for global data" env:"MNEMONIC_GLOBAL_DIR"`
	LocalDir   string   `short:"l" default:".mnemonic" help:"Directory for project data" env:"MNEMONIC_LOCAL_DIR"`
	Team       []string `short:"t" help:"Team data directories (repeatable); scope will become team:<basename>" env:"MNEMONIC_TEAM_DIRS" sep:","`
	Mandatory  []string `short:"m" help:"Additional mandatory categories beyond the defaults (avoidance, security)" env:"MNEMONIC_MANDATORY" sep:","`
	ServerAddr string   `short:"a" default:"localhost:20001" help:"Address to listen on for MCP requests" env:"MNEMONIC_SERVER_ADDR"`
}

func (c *StdioCmd) Run(logger *log.Logger, conf config.Config) error {
	extraEnv := c.daemonEnv()

	if err := ensureDaemon(logger, conf, extraEnv); err != nil {
		return fmt.Errorf("ensuring daemon: %w", err)
	}

	logger.Println("starting stdio bridge")
	if err := daemon.RunStdioServer(context.Background(), conf); err != nil {
		logger.Printf("stdio bridge exited with error: %v", err)
		return err
	}

	logger.Println("stdio bridge exited successfully")
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
