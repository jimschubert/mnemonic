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
	Team       []string `short:"t" help:"Team data directories (repeatable); scope will become team:<basename>" env:"MNEMONIC_TEAM_DIRS" sep:","`
	Mandatory  []string `short:"m" help:"Additional mandatory categories beyond the defaults (avoidance, security)" env:"MNEMONIC_MANDATORY" sep:","`
	ServerAddr string   `short:"a" default:"${server_addr}" help:"Address to listen on for MCP requests"  env:"MNEMONIC_SERVER_ADDR"`
}

func (c *ServerCmd) Run(logger *log.Logger, conf config.Config) error {
	conf.ApplyOverrides(config.Config{
		ServerAddr: c.ServerAddr,
	})

	store.WithAdditionalMandatoryCategories(c.Mandatory)

	scopes := createScopes(c.GlobalDir, c.LocalDir, c.Team)
	ys, err := yamlstore.New(scopes)
	if err != nil {
		return fmt.Errorf("creating YAML store: %w", err)
	}

	d := daemon.New(ys, conf)
	logger.Printf("starting server (socket: %s, MCP: %s/mcp)", conf.SocketPath(), conf.ServerAddr)
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
