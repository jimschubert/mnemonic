package main

import (
	"log/slog"
	"maps"
	"slices"

	"github.com/jimschubert/mnemonic/internal/config"
	"github.com/jimschubert/mnemonic/internal/controller"
	"github.com/jimschubert/mnemonic/internal/logging"
	"github.com/jimschubert/mnemonic/internal/store"
	"github.com/jimschubert/mnemonic/internal/store/sqlitestore"
	"github.com/jimschubert/mnemonic/internal/store/yamlstore"
)

type embeddable struct {
	Endpoint      string  `default:"${embeddings_endpoint}" help:"Full embedding endpoint URL"`
	Model         string  `default:"${embeddings_model}" help:"Embedding model name"`
	AuthToken     string  `help:"Authentication token for embedding endpoint"`
	SkipPreflight bool    `default:"${embeddings_skip_preflight}" help:"Skip preflight check before building index"`
	IndexType     string  `default:"${index_type}" help:"Type of index to use (hnsw or sqlite)"`
	Dimensions    int     `default:"${index_dimensions}" help:"Expected embedding dimensions; auto-verified against test string response"`
	Connections   int     `default:"${index_connections}" help:"HNSW-only connections per node"`
	LevelFactor   float64 `default:"${index_level_factor}" help:"HNSW-only level multiplication factor (Ml)"`
	EfSearch      int     `default:"${index_ef_search}" help:"HNSW-only ef parameter for search"`
}

func (e *embeddable) applyConfig(conf *config.Config) {
	conf.ApplyOverrides(config.Config{
		Embeddings: config.Embeddings{
			Endpoint:      e.Endpoint,
			Model:         e.Model,
			AuthToken:     e.AuthToken,
			SkipPreflight: e.SkipPreflight,
		},
		Index: config.Index{
			Type:        e.IndexType,
			Dimensions:  e.Dimensions,
			Connections: e.Connections,
			LevelFactor: e.LevelFactor,
			EfSearch:    e.EfSearch,
		},
	})
}

// EmbedCmd fetches embeddings and builds the selected index.
type EmbedCmd struct {
	GlobalDir string   `short:"g" default:"~/.mnemonic" help:"Directory for global data" env:"MNEMONIC_GLOBAL_DIR"`
	LocalDir  string   `short:"l" default:".mnemonic" help:"Directory for project data" env:"MNEMONIC_LOCAL_DIR"`
	Team      []string `short:"t" help:"Team data directories (repeatable); scope will become team:<basename>" env:"MNEMONIC_TEAM_DIRS" sep:","`

	Force bool `help:"Overwrite existing index"`

	Embedding embeddable `embed:"" prefix:"embedding-"`
	Store     storeFlags `embed:"" prefix:"store-"`
}

func (e *EmbedCmd) Run(logger *slog.Logger, conf config.Config) error {
	e.Embedding.applyConfig(&conf)
	e.Store.applyConfig(&conf)

	scopes := createScopes(e.GlobalDir, e.LocalDir, e.Team)
	var st store.Store
	var err error
	switch conf.Store.Type {
	case "sqlite":
		sqlitePath := conf.SQLiteStorePath()
		logger.Info("using SQLite store for embedding", "store_type", "sqlite", "sqlite_path", sqlitePath)
		st, err = sqlitestore.New(sqlitePath,
			logging.ForScope(conf, "store"),
			sqlitestore.WithConfiguredScopes(slices.Collect(maps.Keys(scopes))),
			sqlitestore.WithAutoHitCounting(false),
		)
	default:
		logger.Info("using YAML store for embedding", "store_type", "yaml")
		st, err = yamlstore.New(scopes, logging.ForScope(conf, "store"), yamlstore.WithAutoHitCounting(false))
	}

	if err != nil {
		return err
	}

	logger.Info("embedding data", "endpoint", conf.Embeddings.Endpoint, "model", conf.Embeddings.Model)
	ctrl, err := controller.New(conf,
		controller.WithStore(st),
		controller.WithLogger(logger),
		controller.WithSkipInitialSync(true),
		controller.WithMnemonicDir(e.GlobalDir),
	)
	if err != nil {
		return err
	}
	defer func() {
		if err := ctrl.Close(); err != nil {
			logger.Warn("closing embed controller", "err", err)
		}
	}()

	return ctrl.BuildIndexes(e.Force)
}
