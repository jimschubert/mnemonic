package controller

import (
	"fmt"
	"log/slog"

	"github.com/jimschubert/mnemonic/internal/config"
	"github.com/jimschubert/mnemonic/internal/index"
	"github.com/jimschubert/mnemonic/internal/index/sqlitevec"
	"github.com/jimschubert/mnemonic/internal/store"
)

type sqliteManager struct {
	idx    *sqlitevec.Index
	logger *slog.Logger
	conf   config.Config
}

func newSqliteManager(path string, conf config.Config, logger *slog.Logger) (*sqliteManager, error) {
	idx, err := sqlitevec.New(path, conf.Index.Dimensions)
	if err != nil {
		return nil, fmt.Errorf("initializing sqlitevec index: %w", err)
	}
	return &sqliteManager{
		idx:    idx,
		logger: logger,
		conf:   conf,
	}, nil
}

func (m *sqliteManager) InsertVector(id string, vector []float32) error {
	return m.idx.InsertVector(id, vector)
}

func (m *sqliteManager) DeleteVector(id string) error {
	return m.idx.DeleteVector(id)
}

func (m *sqliteManager) Search(target []float32, k int) ([]index.SearchResult, error) {
	return m.idx.Search(target, k)
}

func (m *sqliteManager) LookupVector(id string) ([]float32, bool) {
	return m.idx.LookupVector(id)
}

func (m *sqliteManager) Close() error {
	return m.idx.Close()
}

func (m *sqliteManager) Flush() error {
	// sqlite persists immediately.
	return nil
}

func (m *sqliteManager) Validate() error {
	// touch the index to confirm it is reachable.
	_, _ = m.idx.LookupVector("noop")
	return nil
}

func (m *sqliteManager) IndexEntry(entry *store.Entry, embed func(text string) ([]float32, error)) {
	vec, err := embed(entry.Content)
	if err != nil {
		m.logger.Warn("failed to embed entry", "id", entry.ID, "err", err)
		return
	}
	if err := m.idx.InsertVector(entry.ID, vec); err != nil {
		m.logger.Warn("failed to index entry", "id", entry.ID, "err", err)
	}
}

func (m *sqliteManager) RemoveFromIndex(id string) {
	// ignore error if it wasn't in the index
	_ = m.idx.DeleteVector(id)
}

func (m *sqliteManager) BuildIndexes(entries []store.Entry, force bool, embed func(texts []string) ([][]float32, error)) error {
	if force {
		m.logger.Info("force rebuilding indexer", "type", "sqlite", "dimensions", m.conf.Index.Dimensions)
	}

	activeIDs := make(map[string]struct{}, len(entries))
	var toEmbed []store.Entry

	for _, e := range entries {
		activeIDs[e.ID] = struct{}{}
		if force {
			toEmbed = append(toEmbed, e)
			continue
		}

		_, found := m.idx.LookupVector(e.ID)
		if !found {
			toEmbed = append(toEmbed, e)
		}
	}

	ids, err := m.idx.ListIDs()
	if err != nil {
		return err
	}
	// prune vectors whose entries no longer exist.
	for _, id := range ids {
		if _, ok := activeIDs[id]; ok {
			continue
		}
		if err := m.idx.DeleteVector(id); err != nil {
			m.logger.Warn("failed to remove stale vector", "id", id, "err", err)
		}
	}

	if len(toEmbed) == 0 {
		return nil
	}

	m.logger.Info("indexing entries", "count", len(toEmbed), "force", force)

	texts := make([]string, len(toEmbed))
	for i, e := range toEmbed {
		texts[i] = e.Content
	}

	vectors, err := embed(texts)
	if err != nil {
		return fmt.Errorf("batch embedding: %w", err)
	}
	if len(vectors) != len(toEmbed) {
		return fmt.Errorf("batch embedding returned %d vectors for %d entries", len(vectors), len(toEmbed))
	}

	for i, e := range toEmbed {
		if err := m.idx.InsertVector(e.ID, vectors[i]); err != nil {
			m.logger.Warn("failed to index entry", "id", e.ID, "err", err)
		}
	}

	return nil
}
