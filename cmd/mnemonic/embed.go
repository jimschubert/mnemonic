package main

import (
	"log/slog"

	"github.com/jimschubert/mnemonic/internal/config"
	"github.com/jimschubert/mnemonic/internal/controller"
	"github.com/jimschubert/mnemonic/internal/logging"
	"github.com/jimschubert/mnemonic/internal/store/yamlstore"
)

type embeddable struct {
	Endpoint      string  `default:"${embeddings_endpoint}" help:"Full embedding endpoint URL"`
	Model         string  `default:"${embeddings_model}" help:"Embedding model name"`
	AuthToken     string  `help:"Authentication token for embedding endpoint"`
	SkipPreflight bool    `default:"${embeddings_skip_preflight}" help:"Skip preflight check before building index"`
	Dimensions    int     `default:"${index_dimensions}" help:"Expected embedding dimensions; auto-verified against test string response"`
	Connections   int     `default:"${index_connections}" help:"Number of connections per node in HNSW graph"`
	LevelFactor   float64 `default:"${index_level_factor}" help:"HNSW level multiplication factor (Ml) for index building"`
	EfSearch      int     `default:"${index_ef_search}" help:"HNSW ef parameter for index searching"`
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
			Dimensions:  e.Dimensions,
			Connections: e.Connections,
			LevelFactor: e.LevelFactor,
			EfSearch:    e.EfSearch,
		},
	})
}

// EmbedCmd fetches embeddings and builds the HNSW index.
type EmbedCmd struct {
	GlobalDir string   `short:"g" default:"~/.mnemonic" help:"Directory for global data" env:"MNEMONIC_GLOBAL_DIR"`
	LocalDir  string   `short:"l" default:".mnemonic" help:"Directory for project data" env:"MNEMONIC_LOCAL_DIR"`
	Team      []string `short:"t" help:"Team data directories (repeatable); scope will become team:<basename>" env:"MNEMONIC_TEAM_DIRS" sep:","`

	Force bool `help:"Overwrite existing index."`

	Embedding embeddable `embed:"" prefix:"embedding-"`
}

func (e *EmbedCmd) Run(logger *slog.Logger, conf config.Config) error {
	e.Embedding.applyConfig(&conf)

	scopes := createScopes(e.GlobalDir, e.LocalDir, e.Team)
	ys, err := yamlstore.New(scopes, logging.ForScope(conf, "store"), yamlstore.WithAutoHitCounting(false))
	if err != nil {
		return err
	}

	logger.Info("embedding data", "endpoint", conf.Embeddings.Endpoint, "model", conf.Embeddings.Model)
	ctrl, err := controller.New(conf,
		controller.WithStore(ys),
		controller.WithLogger(logger),
		controller.WithSkipInitialSync(true),
		controller.WithMnemonicDir(e.GlobalDir),
	)
	if err != nil {
		return err
	}
	defer ctrl.Close() //nolint:errcheck

	return ctrl.BuildIndexes(e.Force)
}
