package controller

import (
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

type mockIndexManager struct {
	vectors map[string][]float32
	results []index.SearchResult
}

func (m *mockIndexManager) LookupVector(id string) ([]float32, bool) {
	vec, ok := m.vectors[id]
	return vec, ok
}

func newMockIndexManager() *mockIndexManager {
	return &mockIndexManager{vectors: make(map[string][]float32)}
}

func (m *mockIndexManager) InsertVector(id string, vec []float32) error {
	m.vectors[id] = vec
	return nil
}

func (m *mockIndexManager) DeleteVector(id string) error {
	delete(m.vectors, id)
	return nil
}

func (m *mockIndexManager) Search(_ []float32, k int) ([]index.SearchResult, error) {
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

func (m *mockIndexManager) Close() error { return nil }
func (m *mockIndexManager) Flush() error { return nil }

func (m *mockIndexManager) BuildIndexes(entries []store.Entry, force bool, embed func([]string) ([][]float32, error)) error {
	if force {
		m.vectors = make(map[string][]float32)
	}

	activeIDs := make(map[string]struct{}, len(entries))
	var toEmbed []store.Entry
	for _, e := range entries {
		activeIDs[e.ID] = struct{}{}
		if force {
			toEmbed = append(toEmbed, e)
			continue
		}
		if _, ok := m.vectors[e.ID]; !ok {
			toEmbed = append(toEmbed, e)
		}
	}

	for id := range m.vectors {
		if _, ok := activeIDs[id]; !ok {
			delete(m.vectors, id)
		}
	}

	if len(toEmbed) == 0 {
		return nil
	}

	texts := make([]string, len(toEmbed))
	for i, e := range toEmbed {
		texts[i] = e.Content
	}
	vecs, err := embed(texts)
	if err != nil {
		return err
	}
	for i, e := range toEmbed {
		m.vectors[e.ID] = vecs[i]
	}
	return nil
}

func (m *mockIndexManager) IndexEntry(entry *store.Entry, embed func(string) ([]float32, error)) {
	vec, err := embed(entry.Content)
	if err == nil {
		m.vectors[entry.ID] = vec
	}
}

func (m *mockIndexManager) RemoveFromIndex(id string) {
	delete(m.vectors, id)
}

func (m *mockIndexManager) Validate() error { return nil }

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

	idx := newMockIndexManager()
	emb := &mockEmbedder{available: true, dim: 4}

	mc, err := New(testConfig(),
		WithStore(ms),
		WithIndexManager(idx),
		WithEmbedder(emb),
		WithMnemonicDir(dir),
		WithLogger(testLogger()),
	)
	assert.NoError(t, err)
	defer mc.Close() // nolint:errcheck

	assert.Equal(t, 2, len(idx.vectors))
	_, ok := mc.indexManager.LookupVector("a")
	assert.True(t, ok, "entry a should be present in the index")
	_, ok = mc.indexManager.LookupVector("b")
	assert.True(t, ok, "entry b should be present in the index")
}

//goland:noinspection GoUnhandledErrorResult
func TestNew_EmbedderUnavailable_Passthrough(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	ms := newMockStore()
	ms.entries["a"] = &store.Entry{ID: "a", Content: "test"}

	idx := newMockIndexManager()
	emb := &mockEmbedder{available: false, dim: 4}

	mc, err := New(testConfig(),
		WithStore(ms),
		WithIndexManager(idx),
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
	idx := newMockIndexManager()
	emb := &mockEmbedder{available: true, dim: 4}

	mc, err := New(testConfig(),
		WithStore(ms),
		WithIndexManager(idx),
		WithEmbedder(emb),
		WithMnemonicDir(dir),
	)
	assert.NoError(t, err)
	defer mc.Close() // nolint:errcheck

	entry := &store.Entry{ID: "new1", Content: "new entry", Category: "domain"}
	assert.NoError(t, mc.Upsert(entry))

	_, ok := mc.indexManager.LookupVector("new1")
	assert.True(t, ok, "upserted entry should be indexed")
	_, ok = idx.vectors["new1"]
	assert.True(t, ok, "mock indexer should receive the new vector")
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

	idx := newMockIndexManager()
	emb := &mockEmbedder{available: true, dim: 4}

	mc, err := New(testConfig(),
		WithStore(ms),
		WithIndexManager(idx),
		WithEmbedder(emb),
		WithMnemonicDir(dir),
	)
	assert.NoError(t, err)
	defer mc.Close() // nolint:errcheck
	_, ok := mc.indexManager.LookupVector("live")
	assert.True(t, ok, "live entry should remain indexed")
	_, ok = idx.vectors["live"]
	assert.True(t, ok, "mock indexer should contain the live entry")
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

			idx := newMockIndexManager()
			idx.results = []index.SearchResult{
				{
					ID:       "existing",
					Distance: 0.01,
				},
			}
			emb := &mockEmbedder{available: true, dim: 4}

			mc, err := New(testConfig(),
				WithStore(ms),
				WithIndexManager(idx),
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
			assert.Equal(t, tt.wantID, entry.ID, "deduplication should preserve the expected entry id")
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

	idx := newMockIndexManager()
	idx.results = []index.SearchResult{
		{
			ID:       "existing",
			Distance: 0.01,
		},
	}
	emb := &mockEmbedder{available: true, dim: 4}

	mc, err := New(testConfig(),
		WithStore(ms),
		WithIndexManager(idx),
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
	assert.Equal(t, "entry-to-compact", entry.ID, "save should preserve the provided id")
	_, ok := ms.entries["entry-to-compact"]
	assert.True(t, ok, "save should persist the entry in the store")
}

//goland:noinspection GoUnhandledErrorResult
func TestSave_UpdatesIndexWithoutDeduplication(t *testing.T) {
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

	idx := newMockIndexManager()
	emb := &mockEmbedder{available: true, dim: 4}

	mc, err := New(testConfig(),
		WithStore(ms),
		WithIndexManager(idx),
		WithEmbedder(emb),
		WithMnemonicDir(dir),
		WithSkipInitialSync(true),
	)
	assert.NoError(t, err)
	defer mc.Close() // nolint:errcheck

	entry := &store.Entry{
		ID:       "existing",
		Content:  "compacted memory",
		Category: "domain",
		Scope:    "project",
	}

	assert.NoError(t, mc.Save(entry))
	assert.Equal(t, "existing", entry.ID)
	assert.Equal(t, "compacted memory", ms.entries["existing"].Content)
	_, ok := mc.indexManager.LookupVector("existing")
	assert.True(t, ok, "save should refresh the index for the stored entry")
}

//goland:noinspection GoUnhandledErrorResult
func TestDelete_RemovesFromIndex(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	ms := newMockStore()
	ms.entries["del1"] = &store.Entry{ID: "del1", Content: "to delete", Category: "domain"}

	idx := newMockIndexManager()
	emb := &mockEmbedder{available: true, dim: 4}

	mc, err := New(testConfig(),
		WithStore(ms),
		WithIndexManager(idx),
		WithEmbedder(emb),
		WithMnemonicDir(dir),
	)
	assert.NoError(t, err)
	defer mc.Close() // nolint:errcheck

	_, ok1 := mc.indexManager.LookupVector("del1")
	assert.True(t, ok1, "entry should be indexed before delete")
	assert.NoError(t, mc.Delete("del1"))
	_, ok2 := mc.indexManager.LookupVector("del1")
	assert.False(t, ok2, "deleted entry should be removed from the index")
	_, ok := idx.vectors["del1"]
	assert.False(t, ok, "mock indexer should not retain deleted vectors")
}

//goland:noinspection GoUnhandledErrorResult
func TestSemanticSearch_ReturnsResults(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	ms := newMockStore()
	ms.entries["s1"] = &store.Entry{ID: "s1", Content: "golang patterns", Category: "syntax", Scope: "global"}
	ms.entries["s2"] = &store.Entry{ID: "s2", Content: "security rules", Category: "security", Scope: "global"}

	idx := newMockIndexManager()
	emb := &mockEmbedder{available: true, dim: 4}

	mc, err := New(testConfig(),
		WithStore(ms),
		WithIndexManager(idx),
		WithEmbedder(emb),
		WithMnemonicDir(dir),
	)
	assert.NoError(t, err)
	defer mc.Close() // nolint:errcheck

	results, err := mc.SemanticSearch("go patterns", 5, nil, nil)
	assert.NoError(t, err)
	assert.True(t, len(results) > 0, "semantic search should return results")
}

//goland:noinspection GoUnhandledErrorResult
func TestSemanticSearch_UnavailableReturnsNil(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	ms := newMockStore()
	idx := newMockIndexManager()
	emb := &mockEmbedder{available: false, dim: 4}

	mc, err := New(testConfig(),
		WithStore(ms),
		WithIndexManager(idx),
		WithEmbedder(emb),
		WithMnemonicDir(dir),
	)
	assert.NoError(t, err)
	defer mc.Close() // nolint:errcheck

	results, err := mc.SemanticSearch("anything", 5, nil, nil)
	assert.NoError(t, err)
	assert.Equal(t, 0, len(results), "semantic search should return no results without an embedder")
}

//goland:noinspection GoUnhandledErrorResult
func TestSemanticSearch_FiltersByScope(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	ms := newMockStore()
	ms.entries["g1"] = &store.Entry{ID: "g1", Content: "global entry", Category: "domain", Scope: "global"}
	ms.entries["p1"] = &store.Entry{ID: "p1", Content: "project entry", Category: "domain", Scope: "project"}

	idx := newMockIndexManager()
	emb := &mockEmbedder{available: true, dim: 4}

	mc, err := New(testConfig(),
		WithStore(ms),
		WithIndexManager(idx),
		WithEmbedder(emb),
		WithMnemonicDir(dir),
	)
	assert.NoError(t, err)
	defer mc.Close() // nolint:errcheck

	results, err := mc.SemanticSearch("entry", 5, []store.Scope{"project"}, nil)
	assert.NoError(t, err)
	for _, e := range results {
		assert.Equal(t, "project", e.Scope, "semantic search should only return the requested scope")
	}
}

//goland:noinspection GoUnhandledErrorResult
func TestSemanticSearch_FiltersByCategory(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	ms := newMockStore()
	ms.entries["syn"] = &store.Entry{
		ID: "syn", Content: "syntax entry", Category: "syntax", Scope: "global", Score: 1, Created: time.Now(),
	}
	ms.entries["sec"] = &store.Entry{
		ID: "sec", Content: "security entry", Category: "security", Scope: "global", Score: 1, Created: time.Now(),
	}

	idx := newMockIndexManager()
	idx.results = []index.SearchResult{
		{ID: "syn", Distance: 0.01},
		{ID: "sec", Distance: 0.50},
	}
	emb := &mockEmbedder{available: true, dim: 4}

	mc, err := New(testConfig(),
		WithStore(ms),
		WithIndexManager(idx),
		WithEmbedder(emb),
		WithMnemonicDir(dir),
	)
	assert.NoError(t, err)
	defer mc.Close() // nolint:errcheck

	results, err := mc.SemanticSearch("entry", 5, nil, []string{"security"})
	assert.NoError(t, err)
	assert.Equal(t, 1, len(results), "only matching category entries should be returned")
	assert.Equal(t, "sec", results[0].ID, "security result should be returned first")
}

//goland:noinspection GoUnhandledErrorResult
func TestSemanticSearch_PrefersHigherWeightedScoreOnDistanceTie(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	now := time.Now()

	ms := newMockStore()
	ms.entries["low"] = &store.Entry{
		ID: "low", Content: "tie entry one", Category: "domain", Scope: "global", Score: 1, Created: now,
	}
	ms.entries["high"] = &store.Entry{
		ID: "high", Content: "tie entry two", Category: "domain", Scope: "global", Score: 3, Created: now,
	}

	idx := newMockIndexManager()
	idx.results = []index.SearchResult{
		{ID: "low", Distance: 0.10},
		{ID: "high", Distance: 0.10},
	}
	emb := &mockEmbedder{available: true, dim: 4}

	mc, err := New(testConfig(),
		WithStore(ms),
		WithIndexManager(idx),
		WithEmbedder(emb),
		WithMnemonicDir(dir),
	)
	assert.NoError(t, err)
	defer mc.Close() // nolint:errcheck

	results, err := mc.SemanticSearch("tie", 5, nil, nil)
	assert.NoError(t, err)
	assert.Equal(t, 2, len(results), "both tied entries should be returned")
	assert.Equal(t, "high", results[0].ID, "higher weighted score should win tie-breaker")
	assert.Equal(t, "low", results[1].ID, "lower weighted score should come after higher one")
}

//goland:noinspection GoUnhandledErrorResult
func TestFlushIndex_SkipsWhenNotDirty(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	ms := newMockStore()
	idx := newMockIndexManager()
	emb := &mockEmbedder{available: false, dim: 4}

	mc, err := New(testConfig(),
		WithStore(ms),
		WithIndexManager(idx),
		WithEmbedder(emb),
		WithMnemonicDir(dir),
	)
	assert.NoError(t, err)
	defer mc.Close() // nolint:errcheck

	assert.NoError(t, mc.indexManager.Flush())

	metaPath := filepath.Join(dir, "index.hnsw.json")
	_, err = os.Stat(metaPath)
	assert.True(t, os.IsNotExist(err), "flush should not create metadata when index is clean")
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
	assert.Equal(t, 0, len(m.Entries), "missing metadata file should produce an empty set")
}
