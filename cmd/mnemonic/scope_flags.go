package main

import (
	"path/filepath"

	"github.com/jimschubert/mnemonic/internal/store"
)

type scopeFlags struct {
	GlobalDir string   `short:"g" default:"~/.mnemonic" help:"Directory for global data" env:"MNEMONIC_GLOBAL_DIR"`
	LocalDir  string   `short:"l" default:".mnemonic" help:"Directory for project data" env:"MNEMONIC_LOCAL_DIR"`
	Team      []string `short:"t" help:"Team data directories (repeatable); scope will become team:<basename>" env:"MNEMONIC_TEAM_DIRS" sep:","`
}

func (s *scopeFlags) createScopes() map[store.Scope]string {
	scopes := map[store.Scope]string{
		store.ScopeGlobal: filepath.Join(s.GlobalDir, "global"),
		"project":         filepath.Join(s.LocalDir, "project"),
	}
	for _, dir := range s.Team {
		scope := store.Scope("team:" + filepath.Base(dir))
		scopes[scope] = dir
	}
	return scopes
}
