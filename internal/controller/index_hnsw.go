package controller

import (
	"fmt"
	"log/slog"
	"os"
	"sync"

	"github.com/jimschubert/mnemonic/internal/config"
	"github.com/jimschubert/mnemonic/internal/index"
	"github.com/jimschubert/mnemonic/internal/index/hnsw"
	"github.com/jimschubert/mnemonic/internal/store"
)

type hnswManager struct {
	idx       *hnsw.Index
	indexPath string
	metaPath  string
	meta      *IndexMetadata
	logger    *slog.Logger
	conf      config.Config

	// mu guards indexer (coder/hnsw is not thread-safe)
	mu    sync.RWMutex
	dirty bool
	// metaMu guards dirty flag and meta
	metaMu sync.Mutex
}

func newHnswManager(indexPath, metaPath string, conf config.Config, logger *slog.Logger) *hnswManager {
	return &hnswManager{
		idx:       hnsw.New(conf),
		indexPath: indexPath,
		metaPath:  metaPath,
		meta:      newMetadata(),
		logger:    logger,
		conf:      conf,
	}
}

func (m *hnswManager) InsertVector(id string, vector []float32) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.idx.InsertVector(id, vector)
}

func (m *hnswManager) DeleteVector(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.idx.DeleteVector(id)
}

func (m *hnswManager) Search(target []float32, k int) ([]index.SearchResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.idx.Search(target, k)
}

func (m *hnswManager) LookupVector(id string) ([]float32, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.idx.LookupVector(id)
}

func (m *hnswManager) Close() error {
	return m.Flush()
}

func (m *hnswManager) markDirty() {
	m.metaMu.Lock()
	defer m.metaMu.Unlock()
	m.dirty = true
}

func (m *hnswManager) Load() error {
	meta, err := loadMetadata(m.metaPath)
	if err != nil {
		return fmt.Errorf("loading metadata: %w", err)
	}
	m.meta = meta

	f, err := os.Open(m.indexPath)
	if err != nil {
		if os.IsNotExist(err) {
			if len(m.meta.Entries) > 0 {
				m.logger.Warn("index file missing; clearing stale metadata", "meta_entries", len(m.meta.Entries), "path", m.indexPath)
				m.meta = newMetadata()
			}
			return nil
		}
		return fmt.Errorf("opening index: %w", err)
	}
	defer f.Close()

	if err := m.idx.Import(m.conf, f); err != nil {
		return fmt.Errorf("importing index: %w", err)
	}

	if err := m.Validate(); err != nil {
		m.logger.Warn("loaded index failed validation, rebuilding", "err", err)
		m.idx = hnsw.New(m.conf)
		m.meta = newMetadata()
	}

	return nil
}

func (m *hnswManager) Flush() error {
	m.metaMu.Lock()
	if !m.dirty {
		m.metaMu.Unlock()
		return nil
	}
	m.dirty = false
	m.metaMu.Unlock()

	f, err := os.Create(m.indexPath)
	if err != nil {
		return fmt.Errorf("creating index file: %w", err)
	}
	defer f.Close()

	m.mu.RLock()
	err = m.idx.Export(f)
	m.mu.RUnlock()
	if err != nil {
		return fmt.Errorf("exporting index: %w", err)
	}

	m.metaMu.Lock()
	err = m.meta.save(m.metaPath)
	m.metaMu.Unlock()

	if err != nil {
		return fmt.Errorf("saving metadata: %w", err)
	}

	return nil
}

// Validate makes sure the index is in a usable state.
// necessary because I was seeing panics lack of mutexes around hnsw graph, which is apparently not thread-safe.
func (m *hnswManager) Validate() (retErr error) {
	if m.meta == nil || len(m.meta.Entries) == 0 {
		return nil
	}

	defer func() {
		if r := recover(); r != nil {
			retErr = fmt.Errorf("index validation panicked (corrupted index file): %v", r)
		}
	}()

	testVec := make([]float32, m.conf.Index.Dimensions)
	for i := range testVec {
		testVec[i] = 0.5
	}

	m.mu.RLock()
	_, retErr = m.idx.Search(testVec, 1)
	m.mu.RUnlock()
	return retErr
}

func (m *hnswManager) IndexEntry(entry *store.Entry, embed func(text string) ([]float32, error)) {
	vec, err := embed(entry.Content)
	if err != nil {
		m.logger.Warn("failed to embed entry", "id", entry.ID, "err", err)
		return
	}
	m.mu.Lock()
	if err := m.idx.InsertVector(entry.ID, vec); err != nil {
		m.logger.Warn("failed to index entry", "id", entry.ID, "err", err)
		m.mu.Unlock()
		return
	}
	m.mu.Unlock()

	m.metaMu.Lock()
	m.meta.add(entry.ID)
	m.dirty = true
	m.metaMu.Unlock()
}

func (m *hnswManager) RemoveFromIndex(id string) {
	m.metaMu.Lock()
	if !m.meta.has(id) {
		m.metaMu.Unlock()
		return
	}
	m.meta.remove(id)
	m.dirty = true
	m.metaMu.Unlock()

	m.mu.Lock()
	_ = m.idx.DeleteVector(id)
	m.mu.Unlock()
}

func (m *hnswManager) BuildIndexes(entries []store.Entry, force bool, embed func(texts []string) ([][]float32, error)) error {
	if force {
		m.logger.Info("force rebuilding indexer",
			"type", "hnsw",
			"dimensions", m.conf.Index.Dimensions,
			"connections", m.conf.Index.Connections,
			"level_factor", m.conf.Index.LevelFactor,
			"ef_search", m.conf.Index.EfSearch)
		m.mu.Lock()
		m.idx = hnsw.New(m.conf)
		m.mu.Unlock()

		m.metaMu.Lock()
		m.meta = newMetadata()
		m.metaMu.Unlock()
	}

	m.metaMu.Lock()
	metaCopy := make(map[string]struct{}, len(m.meta.Entries))
	for k := range m.meta.Entries {
		metaCopy[k] = struct{}{}
	}
	m.metaMu.Unlock()

	activeIDs := make(map[string]struct{}, len(entries))
	var toEmbed []store.Entry
	for _, e := range entries {
		activeIDs[e.ID] = struct{}{}
		if _, ok := metaCopy[e.ID]; !ok {
			toEmbed = append(toEmbed, e)
		}
	}

	// remove stale entries
	for id := range metaCopy {
		if _, ok := activeIDs[id]; !ok {
			m.mu.Lock()
			_ = m.idx.DeleteVector(id)
			m.mu.Unlock()

			m.metaMu.Lock()
			m.meta.remove(id)
			m.metaMu.Unlock()
		}
	}

	if len(toEmbed) == 0 {
		m.markDirty()
		return m.Flush()
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
		m.mu.Lock()
		err := m.idx.InsertVector(e.ID, vectors[i])
		m.mu.Unlock()
		if err != nil {
			m.logger.Warn("failed to index entry", "id", e.ID, "err", err)
			continue
		}
		m.metaMu.Lock()
		m.meta.add(e.ID)
		m.metaMu.Unlock()
	}

	m.markDirty()
	return m.Flush()
}
