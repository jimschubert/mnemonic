package main

import (
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"slices"

	"github.com/jimschubert/mnemonic/internal/config"
	"github.com/jimschubert/mnemonic/internal/logging"
	"github.com/jimschubert/mnemonic/internal/store"
	"github.com/jimschubert/mnemonic/internal/store/sqlitestore"
	"github.com/jimschubert/mnemonic/internal/store/yamlstore"
)

type SQLiteExportCmd struct {
	scopeFlags
	StorePath string `default:"${store_path}" required:"" help:"Path to the SQLite store file"`
}

func (s *SQLiteExportCmd) Run(logger *slog.Logger, conf config.Config) error {
	conf.ApplyOverrides(config.Config{
		Store: config.Store{
			SQLitePathRaw: s.StorePath,
		},
	})

	scopes := s.createScopes()

	logger.Debug("starting SQLite export", "sqlite_path", conf.SQLiteStorePath(), "scopes", maps.Keys(scopes))
	yst, err := yamlstore.New(scopes, logging.ForScope(conf, "store"), yamlstore.WithAutoHitCounting(false))
	if err != nil {
		return fmt.Errorf("creating yamlstore: %w", err)
	}
	defer func(yst *yamlstore.YAMLStore) {
		if err := yst.Close(); err != nil {
			fmt.Printf("error closing yamlstore: %v\n", err)
			return
		}
	}(yst)

	st, err := sqlitestore.New(conf.SQLiteStorePath(),
		logging.ForScope(conf, "store"),
		sqlitestore.WithConfiguredScopes(slices.Collect(maps.Keys(scopes))),
		sqlitestore.WithAutoHitCounting(false),
	)
	if err != nil {
		return fmt.Errorf("creating sqlitestore: %w", err)
	}
	defer func(st *sqlitestore.SQLiteStore) {
		if err := st.Close(); err != nil {
			fmt.Printf("error closing sqlitestore: %v\n", err)
			return
		}
	}(st)

	entries, err := st.All([]store.Scope{})
	if err != nil {
		return fmt.Errorf("retrieving entries from sqlitestore: %w", err)
	}
	logger.Debug("retrieved entries from sqlitestore", "entry_count", len(entries))

	errCount := 0
	var aggErr error
	for _, entry := range entries {
		thisErr := yst.Upsert(&entry)
		if thisErr != nil {
			aggErr = errors.Join(aggErr, thisErr)
			fmt.Print("X")
			errCount++
			continue
		}
		fmt.Print(".")
	}
	fmt.Println()

	logger.Info("finished SQLite export", "total_entries", len(entries), "errors", errCount)

	return aggErr
}
