package controller

import (
	"log/slog"

	"github.com/jimschubert/mnemonic/internal/embed"
	"github.com/jimschubert/mnemonic/internal/store"
)

type options struct {
	embedder        embed.Embedder
	indexManager    IndexManager
	store           store.Store
	logger          *slog.Logger
	mnemonicDir     string
	skipInitialSync bool
}

// Option defines a functional option for configuring MemoryController.
type Option func(*options)

// WithEmbedder overrides the default embedder.
func WithEmbedder(e embed.Embedder) Option {
	return func(o *options) {
		o.embedder = e
	}
}

// WithIndexManager overrides the default index manager.
func WithIndexManager(m IndexManager) Option {
	return func(o *options) {
		o.indexManager = m
	}
}

// WithStore overrides the default store.
func WithStore(s store.Store) Option {
	return func(o *options) {
		o.store = s
	}
}

// WithLogger overrides the default logger.
func WithLogger(l *slog.Logger) Option {
	return func(o *options) {
		o.logger = l
	}
}

// WithMnemonicDir sets the mnemonic directory (default: ~/.mnemonic).
func WithMnemonicDir(dir string) Option {
	return func(o *options) {
		o.mnemonicDir = dir
	}
}

// WithSkipInitialSync skips the initial index sync on startup.
// Use this when restarting, or when invoking embedding manually.
func WithSkipInitialSync(skip bool) Option {
	return func(o *options) {
		o.skipInitialSync = skip
	}
}
