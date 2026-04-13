package main

import (
	"context"
	"fmt"
	"log"
	"path"

	"github.com/jimschubert/mnemonic/internal/config"
	"github.com/jimschubert/mnemonic/internal/server"
	"github.com/jimschubert/mnemonic/internal/store"
	"github.com/jimschubert/mnemonic/internal/store/yamlstore"
)

// ServerCmd runs when the user doesn't specify a command.
type ServerCmd struct {
	GlobalDir  string   `default:"~/.mnemonic" help:"Directory for global data" env:"MNEMONIC_GLOBAL_DIR"`
	LocalDir   string   `default:".mnemonic" help:"Directory for project data" env:"MNEMONIC_LOCAL_DIR"`
	Mandatory  []string `help:"Additional mandatory categories to use beyond the defaults (avoidance, security)" env:"MNEMONIC_MANDATORY" sep:","`
	ServerAddr string   `help:"Address to listen on for MCP requests" default:"localhost:20001" env:"MNEMONIC_SERVER_ADDR"`
}

func (c *ServerCmd) Run(logger *log.Logger, conf config.Config) error {
	if c.ServerAddr != "" {
		if conf.ServerAddr != "" && conf.ServerAddr != c.ServerAddr {
			logger.Printf("warning: MCP address specified in both config and CLI, using CLI value: %s", c.ServerAddr)
		}
		conf.ServerAddr = c.ServerAddr
	}

	store.WithAdditionalMandatoryCategories(c.Mandatory)

	ys, err := yamlstore.New(map[store.Scope]string{
		store.ScopeGlobal: path.Join(c.GlobalDir, "global.yaml"),
		"project":         path.Join(c.LocalDir, "project.yaml"),
	})

	if err != nil {
		return fmt.Errorf("creating YAML store: %w", err)
	}

	mcpServer := server.NewServer(ys, conf)
	return mcpServer.Serve(context.Background())
}
