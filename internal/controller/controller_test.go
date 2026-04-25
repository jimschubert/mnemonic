package controller

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alecthomas/assert/v2"
	"github.com/jimschubert/mnemonic/internal/config"
	"github.com/jimschubert/mnemonic/internal/index"
	"github.com/jimschubert/mnemonic/internal/store"
)

type mockEmbedder struct {
	available bool
	dim       int
	calls     int
}

func (m *mockEmbedder) Embed(texts []string) ([][]float32, error) {
	m.calls++
	out := make([][]float32, len(texts))
	for i := range texts {
		out[i] = make([]float32, m.dim)
		// simple deterministic vector: first element is float of text length
		out[i][0] = float32(len(texts[i]))
	}
	return out, nil
}

func (m *mockEmbedder) EmbedSingle(text string) ([]float32, error) {
	vecs, err := m.Embed([]string{text})
	if err != nil {
		return nil, err
	}
	return vecs[0], nil
}

func (m *mockEmbedder) Available() bool {
	return m.available
}

type mockIndexer struct {
	vectors map[string][]float32
	results []index.SearchResult
}

func (m *mockIndexer) LookupVector(id string) ([]float32, bool) {
	return m.vectors[id], true
}

func newMockIndexer() *mockIndexer {
	return &mockIndexer{vectors: make(map[string][]float32)}
}

func (m *mockIndexer) InsertVector(id string, vec []float32) error {
	m.vectors[id] = vec
	return nil
}

func (m *mockIndexer) DeleteVector(id string) error {
	delete(m.vectors, id)
	return nil
}

func (m *mockIndexer) Search(_ []float32, k int) ([]index.SearchResult, error) {
	if len(m.results) > 0 {
		if len(m.results) > k {
			return m.results[:k], nil
		}
		return m.results, nil
	}

	var out []index.SearchResult
	for id := range m.vectors {
		out = append(out, index.SearchResult{ID: id, Distance: 0.1})
		if len(out) >= k {
			break
		}
	}
	return out, nil
}

func (m *mockIndexer) Export(_ any) error { return nil }

type mockStore struct {
	entries map[string]*store.Entry
}

func newMockStore() *mockStore {
	return &mockStore{entries: make(map[string]*store.Entry)}
}

func (m *mockStore) ListHeads(_ []store.Scope) ([]store.HeadInfo, error) { return nil, nil }

func (m *mockStore) All(_ []store.Scope) ([]store.Entry, error) {
	var out []store.Entry
	for _, e := range m.entries {
		out = append(out, *e)
	}
	return out, nil
}

func (m *mockStore) AllByCategory(_ string, _ int, _ []store.Scope) ([]store.Entry, error) {
	return nil, nil
}

func (m *mockStore) Get(id string) (*store.Entry, error) {
	e, ok := m.entries[id]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	return e, nil
}

func (m *mockStore) Query(_ string, _ []string) ([]store.Entry, error) { return nil, nil }

func (m *mockStore) QueryByCategory(_, _ string, _ int, _ []store.Scope) ([]store.Entry, error) {
	return nil, nil
}

func (m *mockStore) Upsert(entry *store.Entry) error {
	if entry.ID == "" {
		entry.ID = fmt.Sprintf("auto-%d", len(m.entries))
	}
	m.entries[entry.ID] = entry
	return nil
}

func (m *mockStore) Score(_ string, _ float64) error       { return nil }
func (m *mockStore) Delete(id string) error                { delete(m.entries, id); return nil }
func (m *mockStore) Promote(_ string, _ store.Scope) error { return nil }

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
}

func testConfig() config.Config {
	return config.Config{
		Index: config.Index{
			Dimensions:  4,
			Connections: 16,
			LevelFactor: 0.25,
			EfSearch:    50,
		},
	}
}

//goland:noinspection GoUnhandledErrorResult
func TestNew_SyncIndexesExistingEntries(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	ms := newMockStore()
	ms.entries["a"] = &store.Entry{ID: "a", Content: "hello world", Category: "domain"}
	ms.entries["b"] = &store.Entry{ID: "b", Content: "go idioms", Category: "syntax"}

	idx := newMockIndexer()
	emb := &mockEmbedder{available: true, dim: 4}

	mc, err := New(testConfig(),
		WithStore(ms),
		WithIndexer(idx),
		WithEmbedder(emb),
		WithMnemonicDir(dir),
		WithLogger(testLogger()),
	)
	assert.NoError(t, err)
	defer mc.Close() // nolint:errcheck

	assert.Equal(t, 2, len(idx.vectors))
	assert.True(t, mc.meta.has("a"))
	assert.True(t, mc.meta.has("b"))
}

//goland:noinspection GoUnhandledErrorResult
func TestNew_EmbedderUnavailable_Passthrough(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	ms := newMockStore()
	ms.entries["a"] = &store.Entry{ID: "a", Content: "test"}

	idx := newMockIndexer()
	emb := &mockEmbedder{available: false, dim: 4}

	mc, err := New(testConfig(),
		WithStore(ms),
		WithIndexer(idx),
		WithEmbedder(emb),
		WithMnemonicDir(dir),
	)
	assert.NoError(t, err)
	defer mc.Close() // nolint:errcheck

	assert.Equal(t, 0, len(idx.vectors))
	assert.Equal(t, 0, emb.calls)
}

//goland:noinspection GoUnhandledErrorResult
func TestUpsert_IndexesNewEntry(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	ms := newMockStore()
	idx := newMockIndexer()
	emb := &mockEmbedder{available: true, dim: 4}

	mc, err := New(testConfig(),
		WithStore(ms),
		WithIndexer(idx),
		WithEmbedder(emb),
		WithMnemonicDir(dir),
	)
	assert.NoError(t, err)
	defer mc.Close() // nolint:errcheck

	entry := &store.Entry{ID: "new1", Content: "new entry", Category: "domain"}
	assert.NoError(t, mc.Upsert(entry))

	assert.True(t, mc.meta.has("new1"))
	_, ok := idx.vectors["new1"]
	assert.True(t, ok)
}

//goland:noinspection GoUnhandledErrorResult
func TestNew_MissingIndexFileClearsStaleMetadata(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	ms := newMockStore()
	ms.entries["live"] = &store.Entry{ID: "live", Content: "alive", Category: "domain"}

	meta := newMetadata()
	meta.add("live")
	metaPath := filepath.Join(dir, "index.hnsw.json")
	assert.NoError(t, meta.save(metaPath))

	idx := newMockIndexer()
	emb := &mockEmbedder{available: true, dim: 4}

	mc, err := New(testConfig(),
		WithStore(ms),
		WithIndexer(idx),
		WithEmbedder(emb),
		WithMnemonicDir(dir),
	)
	assert.NoError(t, err)
	defer mc.Close() // nolint:errcheck

	assert.True(t, mc.meta.has("live"))
	_, ok := idx.vectors["live"]
	assert.True(t, ok)
	assert.Equal(t, 1, emb.calls)
}

//goland:noinspection GoUnhandledErrorResult
func TestUpsert_DeduplicatesOnlyWithinCategoryAndScope(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		existingCategory string
		existingScope    string
		entryCategory    string
		entryScope       string
		wantID           string
	}{
		{
			name:             "reuses id for same category and scope",
			existingCategory: "domain",
			existingScope:    "project",
			entryCategory:    "domain",
			entryScope:       "project",
			wantID:           "existing",
		},
		{
			name:             "does not reuse id across categories",
			existingCategory: "avoidance",
			existingScope:    "project",
			entryCategory:    "domain",
			entryScope:       "project",
			wantID:           "new-entry",
		},
		{
			name:             "does not reuse id across scopes",
			existingCategory: "domain",
			existingScope:    "global",
			entryCategory:    "domain",
			entryScope:       "project",
			wantID:           "new-entry",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()

			ms := newMockStore()
			ms.entries["existing"] = &store.Entry{
				ID:       "existing",
				Content:  "existing memory",
				Category: tt.existingCategory,
				Scope:    tt.existingScope,
				Score:    1.0,
			}

			idx := newMockIndexer()
			idx.results = []index.SearchResult{
				{
					ID:       "existing",
					Distance: 0.01,
				},
			}
			emb := &mockEmbedder{available: true, dim: 4}

			mc, err := New(testConfig(),
				WithStore(ms),
				WithIndexer(idx),
				WithEmbedder(emb),
				WithMnemonicDir(dir),
				WithSkipInitialSync(true),
			)
			assert.NoError(t, err)
			defer mc.Close() // nolint:errcheck

			entry := &store.Entry{
				ID:       "new-entry",
				Content:  "compacted memory",
				Category: tt.entryCategory,
				Scope:    tt.entryScope,
			}

			assert.NoError(t, mc.Upsert(entry))
			assert.Equal(t, tt.wantID, entry.ID)
		})
	}
}

//goland:noinspection GoUnhandledErrorResult
func TestSave_SkipsSemanticDeduplication(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	ms := newMockStore()
	ms.entries["existing"] = &store.Entry{
		ID:       "existing",
		Content:  "original memory",
		Category: "domain",
		Scope:    "project",
		Score:    1.0,
	}

	idx := newMockIndexer()
	idx.results = []index.SearchResult{
		{
			ID:       "existing",
			Distance: 0.01,
		},
	}
	emb := &mockEmbedder{available: true, dim: 4}

	mc, err := New(testConfig(),
		WithStore(ms),
		WithIndexer(idx),
		WithEmbedder(emb),
		WithMnemonicDir(dir),
		WithSkipInitialSync(true),
	)
	assert.NoError(t, err)
	defer mc.Close() // nolint:errcheck

	entry := &store.Entry{
		ID:       "entry-to-compact",
		Content:  "compacted memory",
		Category: "domain",
		Scope:    "project",
	}

	assert.NoError(t, mc.Save(entry))
	assert.Equal(t, "entry-to-compact", entry.ID)
	_, ok := ms.entries["entry-to-compact"]
	assert.True(t, ok)
}

//goland:noinspection GoUnhandledErrorResult
func TestDelete_RemovesFromIndex(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	ms := newMockStore()
	ms.entries["del1"] = &store.Entry{ID: "del1", Content: "to delete", Category: "domain"}

	idx := newMockIndexer()
	emb := &mockEmbedder{available: true, dim: 4}

	mc, err := New(testConfig(),
		WithStore(ms),
		WithIndexer(idx),
		WithEmbedder(emb),
		WithMnemonicDir(dir),
	)
	assert.NoError(t, err)
	defer mc.Close() // nolint:errcheck

	assert.True(t, mc.meta.has("del1"))
	assert.NoError(t, mc.Delete("del1"))
	assert.False(t, mc.meta.has("del1"))
	_, ok := idx.vectors["del1"]
	assert.False(t, ok)
}

//goland:noinspection GoUnhandledErrorResult
func TestSyncIndex_RemovesStaleMetadata(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	ms := newMockStore()
	ms.entries["live"] = &store.Entry{ID: "live", Content: "alive", Category: "domain"}

	idx := newMockIndexer()
	idx.vectors["stale"] = []float32{1, 2, 3, 4}

	emb := &mockEmbedder{available: true, dim: 4}

	meta := newMetadata()
	meta.add("stale")
	meta.add("live")
	indexPath := filepath.Join(dir, "index.hnsw")
	assert.NoError(t, os.WriteFile(indexPath, []byte("test-index"), 0o644))
	metaPath := filepath.Join(dir, "index.hnsw.json")
	assert.NoError(t, meta.save(metaPath))

	mc, err := New(testConfig(),
		WithStore(ms),
		WithIndexer(idx),
		WithEmbedder(emb),
		WithMnemonicDir(dir),
	)
	assert.NoError(t, err)
	defer mc.Close() // nolint:errcheck

	assert.False(t, mc.meta.has("stale"))
	assert.True(t, mc.meta.has("live"))
	_, ok := idx.vectors["stale"]
	assert.False(t, ok)
}

//goland:noinspection GoUnhandledErrorResult
func TestSemanticSearch_ReturnsResults(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	ms := newMockStore()
	ms.entries["s1"] = &store.Entry{ID: "s1", Content: "golang patterns", Category: "syntax", Scope: "global"}
	ms.entries["s2"] = &store.Entry{ID: "s2", Content: "security rules", Category: "security", Scope: "global"}

	idx := newMockIndexer()
	emb := &mockEmbedder{available: true, dim: 4}

	mc, err := New(testConfig(),
		WithStore(ms),
		WithIndexer(idx),
		WithEmbedder(emb),
		WithMnemonicDir(dir),
	)
	assert.NoError(t, err)
	defer mc.Close() // nolint:errcheck

	results, err := mc.SemanticSearch("go patterns", 5, nil, nil)
	assert.NoError(t, err)
	assert.True(t, len(results) > 0)
}

//goland:noinspection GoUnhandledErrorResult
func TestSemanticSearch_UnavailableReturnsNil(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	ms := newMockStore()
	idx := newMockIndexer()
	emb := &mockEmbedder{available: false, dim: 4}

	mc, err := New(testConfig(),
		WithStore(ms),
		WithIndexer(idx),
		WithEmbedder(emb),
		WithMnemonicDir(dir),
	)
	assert.NoError(t, err)
	defer mc.Close() // nolint:errcheck

	results, err := mc.SemanticSearch("anything", 5, nil, nil)
	assert.NoError(t, err)
	assert.Equal(t, 0, len(results))
}

//goland:noinspection GoUnhandledErrorResult
func TestSemanticSearch_FiltersByScope(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	ms := newMockStore()
	ms.entries["g1"] = &store.Entry{ID: "g1", Content: "global entry", Category: "domain", Scope: "global"}
	ms.entries["p1"] = &store.Entry{ID: "p1", Content: "project entry", Category: "domain", Scope: "project"}

	idx := newMockIndexer()
	emb := &mockEmbedder{available: true, dim: 4}

	mc, err := New(testConfig(),
		WithStore(ms),
		WithIndexer(idx),
		WithEmbedder(emb),
		WithMnemonicDir(dir),
	)
	assert.NoError(t, err)
	defer mc.Close() // nolint:errcheck

	results, err := mc.SemanticSearch("entry", 5, []store.Scope{"project"}, nil)
	assert.NoError(t, err)
	for _, e := range results {
		assert.Equal(t, "project", e.Scope)
	}
}

//goland:noinspection GoUnhandledErrorResult
func TestSemanticSearch_FiltersByCategory(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	ms := newMockStore()
	ms.entries["syn"] = &store.Entry{ID: "syn", Content: "syntax entry", Category: "syntax", Scope: "global", Score: 1, Created: time.Now()}
	ms.entries["sec"] = &store.Entry{ID: "sec", Content: "security entry", Category: "security", Scope: "global", Score: 1, Created: time.Now()}

	idx := newMockIndexer()
	idx.results = []index.SearchResult{
		{ID: "syn", Distance: 0.01},
		{ID: "sec", Distance: 0.50},
	}
	emb := &mockEmbedder{available: true, dim: 4}

	mc, err := New(testConfig(),
		WithStore(ms),
		WithIndexer(idx),
		WithEmbedder(emb),
		WithMnemonicDir(dir),
	)
	assert.NoError(t, err)
	defer mc.Close() // nolint:errcheck

	results, err := mc.SemanticSearch("entry", 5, nil, []string{"security"})
	assert.NoError(t, err)
	assert.Equal(t, 1, len(results))
	assert.Equal(t, "sec", results[0].ID)
}

//goland:noinspection GoUnhandledErrorResult
func TestSemanticSearch_PrefersHigherWeightedScoreOnDistanceTie(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	now := time.Now()

	ms := newMockStore()
	ms.entries["low"] = &store.Entry{ID: "low", Content: "tie entry one", Category: "domain", Scope: "global", Score: 1, Created: now}
	ms.entries["high"] = &store.Entry{ID: "high", Content: "tie entry two", Category: "domain", Scope: "global", Score: 3, Created: now}

	idx := newMockIndexer()
	idx.results = []index.SearchResult{
		{ID: "low", Distance: 0.10},
		{ID: "high", Distance: 0.10},
	}
	emb := &mockEmbedder{available: true, dim: 4}

	mc, err := New(testConfig(),
		WithStore(ms),
		WithIndexer(idx),
		WithEmbedder(emb),
		WithMnemonicDir(dir),
	)
	assert.NoError(t, err)
	defer mc.Close() // nolint:errcheck

	results, err := mc.SemanticSearch("tie", 5, nil, nil)
	assert.NoError(t, err)
	assert.Equal(t, 2, len(results))
	assert.Equal(t, "high", results[0].ID)
	assert.Equal(t, "low", results[1].ID)
}

//goland:noinspection GoUnhandledErrorResult
func TestFlushIndex_WritesFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	ms := newMockStore()
	ms.entries["f1"] = &store.Entry{ID: "f1", Content: "flush test", Category: "domain"}

	idx := newMockIndexer()
	emb := &mockEmbedder{available: true, dim: 4}

	mc, err := New(testConfig(),
		WithStore(ms),
		WithIndexer(idx),
		WithEmbedder(emb),
		WithMnemonicDir(dir),
	)
	assert.NoError(t, err)

	mc.markDirty()
	assert.NoError(t, mc.flushIndex())

	metaPath := filepath.Join(dir, "index.hnsw.json")
	b, err := os.ReadFile(metaPath)
	assert.NoError(t, err)

	var meta IndexMetadata
	assert.NoError(t, json.Unmarshal(b, &meta))
	assert.True(t, meta.has("f1"))

	mc.Close() // nolint:errcheck
}

//goland:noinspection GoUnhandledErrorResult
func TestFlushIndex_SkipsWhenNotDirty(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	ms := newMockStore()
	idx := newMockIndexer()
	emb := &mockEmbedder{available: false, dim: 4}

	mc, err := New(testConfig(),
		WithStore(ms),
		WithIndexer(idx),
		WithEmbedder(emb),
		WithMnemonicDir(dir),
	)
	assert.NoError(t, err)
	defer mc.Close() // nolint:errcheck

	assert.NoError(t, mc.flushIndex())

	metaPath := filepath.Join(dir, "index.hnsw.json")
	_, err = os.Stat(metaPath)
	assert.True(t, os.IsNotExist(err))
}

func TestMetadata_LoadSaveRoundtrip(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "meta.json")

	m := newMetadata()
	m.add("abc")
	m.add("def")
	assert.NoError(t, m.save(path))

	loaded, err := loadMetadata(path)
	assert.NoError(t, err)
	assert.True(t, loaded.has("abc"))
	assert.True(t, loaded.has("def"))
	assert.False(t, loaded.has("ghi"))
}

func TestMetadata_LoadNonexistent(t *testing.T) {
	t.Parallel()
	m, err := loadMetadata("/tmp/nonexistent-mnemonic-meta.json")
	assert.NoError(t, err)
	assert.Equal(t, 0, len(m.Entries))
}
