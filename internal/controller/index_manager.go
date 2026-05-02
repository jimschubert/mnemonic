package controller

import (
	"io"

	"github.com/jimschubert/mnemonic/internal/index"
	"github.com/jimschubert/mnemonic/internal/store"
)

// IndexManager wraps an index.Indexer to manage its lifecycle, including synchronization and persistence.
type IndexManager interface {
	index.Indexer
	io.Closer

	// Flush persists any dirty state to disk.
	Flush() error

	// BuildIndexes updates the index from the given entries. If force is true, the index is rebuilt from scratch.
	// embed is used to retrieve vector representations for texts.
	BuildIndexes(entries []store.Entry, force bool, embed func(texts []string) ([][]float32, error)) error

	// IndexEntry inserts or updates an entry in the index.
	IndexEntry(entry *store.Entry, embed func(text string) ([]float32, error))

	// RemoveFromIndex deletes an entry from the index.
	RemoveFromIndex(id string)

	// LookupVector returns the stored vector for the given entry ID, if present.
	LookupVector(id string) ([]float32, bool)

	// Validate checks that the index is healthy.
	Validate() error
}
