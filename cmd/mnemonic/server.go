package main

import (
	"context"
	"fmt"
	"log"
	"path/filepath"

	"github.com/jimschubert/mnemonic/internal/config"
	"github.com/jimschubert/mnemonic/internal/daemon"
	"github.com/jimschubert/mnemonic/internal/store"
	"github.com/jimschubert/mnemonic/internal/store/yamlstore"
)

// ServerCmd starts the MCP server in-process: serves the store over both a Unix socket and TCP HTTP.
type ServerCmd struct {
	GlobalDir  string   `short:"g" default:"~/.mnemonic" help:"Directory for global data" env:"MNEMONIC_GLOBAL_DIR"`
	LocalDir   string   `short:"l" default:".mnemonic" help:"Directory for project data" env:"MNEMONIC_LOCAL_DIR"`
	Mandatory  []string `short:"m" help:"Additional mandatory categories beyond the defaults (avoidance, security)" env:"MNEMONIC_MANDATORY" sep:","`
	ServerAddr string   `short:"a" default:"localhost:20001" help:"Address to listen on for MCP requests"  env:"MNEMONIC_SERVER_ADDR"`
}

func (c *ServerCmd) Run(logger *log.Logger, conf config.Config) error {
	store.WithAdditionalMandatoryCategories(c.Mandatory)

	if c.ServerAddr != "" {
		if conf.ServerAddr != "" && conf.ServerAddr != c.ServerAddr {
			logger.Printf("warning: MCP address specified in both config and CLI, using CLI value: %s", c.ServerAddr)
		}
		conf.ServerAddr = c.ServerAddr
	}

	ys, err := yamlstore.New(map[store.Scope]string{
		store.ScopeGlobal: filepath.Join(c.GlobalDir, "global.yaml"),
		"project":         filepath.Join(c.LocalDir, "project.yaml"),
	})

	if err != nil {
		return fmt.Errorf("creating YAML store: %w", err)
	}

	d := daemon.New(ys, conf)
	logger.Printf("starting server (socket: %s, MCP: %s/mcp)", conf.SocketPath(), conf.ServerAddr)
	return d.Start(context.Background())
}
