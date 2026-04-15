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
	Mandatory  []string `short:"m" help:"Additional mandatory categories beyond the defaults (avoidance, security)" env:"MNEMONIC_MANDATORY" sep:","`
	ServerAddr string   `short:"a" default:"localhost:20001" help:"Address to listen on for MCP requests" env:"MNEMONIC_SERVER_ADDR"`
}

func (c *StdioCmd) Run(logger *log.Logger, conf config.Config) error {
	extraEnv := daemonEnv(c.GlobalDir, c.LocalDir, c.Mandatory, c.ServerAddr)

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
func daemonEnv(globalDir, localDir string, mandatory []string, serverAddr string) []string {
	env := []string{
		"MNEMONIC_GLOBAL_DIR=" + globalDir,
		"MNEMONIC_LOCAL_DIR=" + localDir,
	}
	if len(mandatory) > 0 {
		env = append(env, "MNEMONIC_MANDATORY="+strings.Join(mandatory, ","))
	}
	if serverAddr != "" {
		env = append(env, "MNEMONIC_SERVER_ADDR="+serverAddr)
	}
	return env
}
