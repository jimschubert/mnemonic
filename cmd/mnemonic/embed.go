package main

import (
	"log"
	"log/slog"
	"os"

	"github.com/jimschubert/mnemonic/internal/config"
	"github.com/jimschubert/mnemonic/internal/controller"
	"github.com/jimschubert/mnemonic/internal/store/yamlstore"
)

// EmbedCmd fetches embeddings and builds the HNSW index.
type EmbedCmd struct {
	GlobalDir string   `short:"g" default:"~/.mnemonic" help:"Directory for global data" env:"MNEMONIC_GLOBAL_DIR"`
	LocalDir  string   `short:"l" default:".mnemonic" help:"Directory for project data" env:"MNEMONIC_LOCAL_DIR"`
	Team      []string `short:"t" help:"Team data directories (repeatable); scope will become team:<basename>" env:"MNEMONIC_TEAM_DIRS" sep:","`

	Endpoint      string  `short:"e" default:"${embeddings_endpoint}" help:"Full embedding endpoint URL"`
	Model         string  `short:"m" default:"${embeddings_model}" help:"Embedding model name"`
	AuthToken     string  `short:"a" help:"Authentication token for embedding endpoint"`
	SkipPreflight bool    `default:"${embeddings_skip_preflight}" help:"Skip preflight check before building index"`
	Dimensions    int     `short:"d" default:"${index_dimensions}" help:"Expected embedding dimensions; auto-verified against test string response"`
	Connections   int     `short:"c" default:"${index_connections}" help:"Number of connections per node in HNSW graph"`
	LevelFactor   float64 `short:"b" default:"${index_level_factor}" help:"HNSW level multiplication factor (Ml) for index building"`
	EfSearch      int     `short:"s" default:"${index_ef_search}" help:"HNSW ef parameter for index searching"`
	Force         bool    `help:"Overwrite existing index."`
}

func (e *EmbedCmd) Run(logger *log.Logger, conf config.Config) error {
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

	scopes := createScopes(e.GlobalDir, e.LocalDir, e.Team)
	ys, err := yamlstore.New(scopes)
	if err != nil {
		return err
	}

	logger.Printf("embedding data (endpoint: %s, model: %s)", conf.Embeddings.Endpoint, conf.Embeddings.Model)
	ctrl, err := controller.New(conf,
		controller.WithStore(ys),
		controller.WithLogger(slog.New(slog.NewTextHandler(os.Stderr, nil))),
		controller.WithSkipInitialSync(true),
		controller.WithMnemonicDir(e.GlobalDir),
	)
	if err != nil {
		return err
	}
	defer ctrl.Close() //nolint:errcheck

	return ctrl.BuildIndexes(e.Force)
}
