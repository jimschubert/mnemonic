package main

import "github.com/jimschubert/mnemonic/internal/config"

// storeFlags provides reusable dashed CLI flags for selecting the store backend and SQLite path.
type storeFlags struct {
	Type       string `default:"${store_type}" help:"Type of store to use (yaml or sqlite)"`
	SQLitePath string `default:"${store_path}" help:"Path to the SQLite store file"`
}

func (s *storeFlags) applyConfig(conf *config.Config) {
	conf.ApplyOverrides(config.Config{
		Store: config.Store{
			Type:          s.Type,
			SQLitePathRaw: s.SQLitePath,
		},
	})
}
