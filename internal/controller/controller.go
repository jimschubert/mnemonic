package controller

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/jimschubert/mnemonic/internal/config"
	"github.com/jimschubert/mnemonic/internal/embed"
	"github.com/jimschubert/mnemonic/internal/index"
	"github.com/jimschubert/mnemonic/internal/index/hnsw"
	"github.com/jimschubert/mnemonic/internal/logging"
	"github.com/jimschubert/mnemonic/internal/store"
	"github.com/jimschubert/mnemonic/internal/store/yamlstore"
)

// TODO: make flush configurable
const flushInterval = 30 * time.Second

var (
	// ErrEmbedderNotAvailable is returned when an operation requires an embedder but one is not available.
	ErrEmbedderNotAvailable = errors.New("embedder not available")
)

// SemanticSearcher implements vector-based semantic search.
// This is separate from store.Store because SemanticSearch is a responsibility of the embed.Embedder + index.Indexer
// control structure, not necessarily every future store implementation.
type SemanticSearcher interface {
	SemanticSearch(query string, k int, scopes []store.Scope) ([]store.Entry, error)
}

// MemoryController wraps a Store with optional semantic indexing via HNSW.
//
// When an Embedder is available, it keeps vectors in sync with store entries and supports SemanticSearch.
// When no Embedder is available, it passes through to the inner store.
type MemoryController struct {
	store    store.Store
	embedder embed.Embedder
	indexer  index.Indexer
	// meta tracks which files have been indexed so we can efficiently sync new/deleted entries without full re-index.
	meta   *IndexMetadata
	logger *slog.Logger
	conf   config.Config

	indexPath string
	metaPath  string

	// mu guards dirty flag and meta
	mu sync.Mutex
	// indexMu guards indexer (coder/hnsw is not thread-safe)
	indexMu sync.RWMutex
	dirty   bool
	done    chan struct{}
}

var _ store.Store = (*MemoryController)(nil)
var _ SemanticSearcher = (*MemoryController)(nil)

// New creates a MemoryController. cfg is required; Embedder, Indexer, and
// Store are constructed from cfg unless overridden via options.
// mnemonicDir defaults to ~/.mnemonic unless overridden.
func New(conf config.Config, opts ...Option) (*MemoryController, error) {
	o := &options{
		mnemonicDir: "~/.mnemonic",
		logger:      logging.ForScope(conf, "controller"),
	}
	for _, opt := range opts {
		opt(o)
	}

	// build defaults if not overridden
	if o.embedder == nil {
		o.logger.Info("creating embedder", "type", "http", "endpoint", conf.Embeddings.Endpoint, "model", conf.Embeddings.Model)
		o.embedder = embed.New(conf)
	}
	if o.indexer == nil {
		o.logger.Info("creating indexer", "type", "hnsw", "dimensions", conf.Index.Dimensions, "connections", conf.Index.Connections, "level_factor", conf.Index.LevelFactor, "ef_search", conf.Index.EfSearch)
		o.indexer = hnsw.New(conf)
	}
	if o.store == nil {
		s, err := yamlstore.New(map[store.Scope]string{
			store.ScopeGlobal: filepath.Join(o.mnemonicDir, "global"),
			"project":         ".mnemonic/project",
		}, logging.ForScope(conf, "store"))
		if err != nil {
			return nil, fmt.Errorf("constructing store: %w", err)
		}
		o.logger.Info("creating default store (global, project)", "type", "yaml", "scopes", []string{
			store.ScopeGlobal.String(), "project",
		})
		o.store = s
	}

	dir := expandHome(o.mnemonicDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("creating mnemonic directory: %w", err)
	}

	mc := &MemoryController{
		store:     o.store,
		embedder:  o.embedder,
		indexer:   o.indexer,
		conf:      conf,
		indexPath: filepath.Join(dir, "index.hnsw"),
		metaPath:  filepath.Join(dir, "index.hnsw.json"),
		logger:    o.logger,
		done:      make(chan struct{}),
	}

	if o.embedder.Available() {
		if err := mc.loadIndex(); err != nil {
			mc.logger.Warn("failed to load index, starting fresh", "err", err)
			mc.meta = newMetadata()
			// Rebuild the indexer to ensure a clean state after import failure
			o.logger.Info("rebuilding indexer after failed import", "type", "hnsw", "dimensions", conf.Index.Dimensions, "connections", conf.Index.Connections, "level_factor", conf.Index.LevelFactor, "ef_search", conf.Index.EfSearch)
			mc.indexer = hnsw.New(conf)
		}
		if !o.skipInitialSync {
			// BuildIndexes incrementally syncs then immediately flushes when force=false.
			if err := mc.BuildIndexes(false); err != nil && !errors.Is(err, ErrEmbedderNotAvailable) {
				mc.logger.Warn("index sync error", "err", err)
			}
		}
	} else {
		mc.logger.Info("embedder unavailable, skipping index load and sync")
		mc.meta = newMetadata()
	}

	go mc.flushLoop()
	return mc, nil
}

// Close stops the flush loop, persists the index, and closes the inner store.
func (mc *MemoryController) Close() error {
	close(mc.done)
	var errs error
	if err := mc.flushIndex(); err != nil {
		errs = err
	}
	if c, ok := mc.store.(io.Closer); ok {
		if err := c.Close(); err != nil {
			errs = fmt.Errorf("%w; store close: %w", errs, err)
		}
	}
	return errs
}

func (mc *MemoryController) ListHeads(scopes []store.Scope) ([]store.HeadInfo, error) {
	return mc.store.ListHeads(scopes)
}

func (mc *MemoryController) All(scopes []store.Scope) ([]store.Entry, error) {
	return mc.store.All(scopes)
}

func (mc *MemoryController) AllByCategory(category string, topK int, scopes []store.Scope) ([]store.Entry, error) {
	return mc.store.AllByCategory(category, topK, scopes)
}

func (mc *MemoryController) Get(id string) (*store.Entry, error) {
	return mc.store.Get(id)
}

func (mc *MemoryController) Query(category string, tags []string) ([]store.Entry, error) {
	return mc.store.Query(category, tags)
}

func (mc *MemoryController) QueryByCategory(category, query string, topK int, scopes []store.Scope) ([]store.Entry, error) {
	return mc.store.QueryByCategory(category, query, topK, scopes)
}

func (mc *MemoryController) Score(id string, delta float64) error {
	return mc.store.Score(id, delta)
}

func (mc *MemoryController) Promote(id string, targetScope store.Scope) error {
	return mc.store.Promote(id, targetScope)
}

func (mc *MemoryController) doUpsert(entry *store.Entry) error {
	err := mc.store.Upsert(entry)
	if err != nil {
		return err
	}
	mc.indexEntry(entry)
	return nil
}

// Save allows for re-save without all the embedding and deduplication logic of Upsert. DOES _NOT_ UPDATE INDEX.
func (mc *MemoryController) Save(entry *store.Entry) error {
	return mc.store.Upsert(entry)
}

func (mc *MemoryController) Upsert(entry *store.Entry) error {
	if !mc.embedder.Available() {
		return mc.doUpsert(entry)
	}

	// find any semantically equivalent existing entry and reuse its ID to avoid duplicates in the index.
	query := entry.Content
	vec, err := mc.embedder.EmbedSingle(query)
	if err != nil {
		return mc.doUpsert(entry)
	}

	mc.indexMu.RLock()
	results, err := mc.indexer.Search(vec, 3)
	mc.indexMu.RUnlock()
	if err != nil {
		mc.logger.Warn("index search error during upsert; skipping deduplication", "err", err)
		return mc.doUpsert(entry)
	}

	entryScope := normalizedScope(entry.Scope)
	for _, r := range results {
		// TODO: make duplication threshold configurable?
		if r.Distance < 0.05 {
			existing, err := mc.store.Get(r.ID)
			if err != nil {
				continue
			}

			existingScope := normalizedScope(existing.Scope)
			// NOTE: avoids saving the same entry.ID to different categories/scopes. Can be cleaned up later.
			if existing.Category != entry.Category || existingScope != entryScope {
				mc.logger.Debug("skipping dedup candidate from different category or scope",
					"entry_id", entry.ID,
					"candidate_id", existing.ID,
					"entry_category", entry.Category,
					"candidate_category", existing.Category,
					"entry_scope", entryScope,
					"candidate_scope", existingScope,
				)
				continue
			}

			entry.ID = existing.ID
			// bump score because it's obviously an important memory
			entry.Score = existing.Score + 0.1
			entry.HitCount = existing.HitCount
			entry.LastHit = time.Now()
			entry.Created = existing.Created
			entry.Source = existing.Source

			// just sent if empty, but maybe merge these later?
			if len(entry.Tags) == 0 && len(existing.Tags) > 0 {
				entry.Tags = existing.Tags
			}
			break
		}
	}

	return mc.doUpsert(entry)
}

func normalizedScope(scope string) string {
	if scope == "" {
		return store.ScopeGlobal.String()
	}
	return scope
}

func (mc *MemoryController) Delete(id string) error {
	if err := mc.store.Delete(id); err != nil {
		return err
	}
	mc.removeFromIndex(id)
	return nil
}

// SemanticSearch embeds the query and returns the k nearest entries.
// Returns nil when the embedder is unavailable.
func (mc *MemoryController) SemanticSearch(query string, k int, scopes []store.Scope) ([]store.Entry, error) {
	if !mc.embedder.Available() {
		return nil, nil
	}

	vec, err := mc.embedder.EmbedSingle(query)
	if err != nil {
		return nil, fmt.Errorf("embedding query: %w", err)
	}

	// k*2 to allow for filtering by scope (may result in <k results)
	mc.indexMu.RLock()
	results, err := mc.indexer.Search(vec, k*2)
	mc.indexMu.RUnlock()
	if err != nil {
		return nil, fmt.Errorf("index search: %w", err)
	}

	scopeSet := make(map[string]bool, len(scopes))
	for _, s := range scopes {
		scopeSet[s.String()] = true
	}

	var entries []store.Entry
	for _, r := range results {
		entry, err := mc.store.Get(r.ID)
		if err != nil {
			mc.removeFromIndex(r.ID)
			continue
		}
		if len(scopeSet) > 0 && !scopeSet[entry.Scope] {
			continue
		}
		entries = append(entries, *entry)
		if len(entries) >= k {
			break
		}
	}

	return entries, nil
}

type SimilarEntry struct {
	store.Entry
	Distance float32
}

// FindSimilar finds semantically similar entries by id with a threshold of similarity. A higher threshold means more similar results.
func (mc *MemoryController) FindSimilar(id string, threshold float64) ([]SimilarEntry, error) {
	if !mc.embedder.Available() {
		return nil, ErrEmbedderNotAvailable
	}

	// Use the indexed vector, as embedding can return slightly different values each time you call it
	mc.indexMu.RLock()
	vec, ok := mc.indexer.LookupVector(id)
	mc.indexMu.RUnlock()

	if !ok {
		// entry not yet indexed; fall back to embedding
		entry, err := mc.store.Get(id)
		if err != nil {
			return nil, fmt.Errorf("getting entry for similarity check: %w", err)
		}
		vec, err = mc.embedder.EmbedSingle(entry.Content)
		if err != nil {
			return nil, fmt.Errorf("embedding entry for similarity check: %w", err)
		}
	}

	// threshold of 0.9 (90% similar) means a distance of 0.1
	targetDistance := float32(1.0 - threshold)

	mc.indexMu.RLock()
	// k=10 should be good, but maybe this could be configurable?
	results, err := mc.indexer.Search(vec, 10)
	mc.indexMu.RUnlock()
	if err != nil {
		return nil, fmt.Errorf("index search: %w", err)
	}

	var similar []SimilarEntry
	for _, r := range results {
		if r.ID == id {
			continue
		}
		if r.Distance > targetDistance {
			continue
		}
		e, err := mc.store.Get(r.ID)
		if err != nil {
			continue
		}
		similar = append(similar, SimilarEntry{
			Entry:    *e,
			Distance: r.Distance,
		})
	}

	return similar, nil
}

// Merge takes the text of the entry (keepId) and combines other metadata from the other entry (deleteId), then deletes the other entry.
// Text is only keept from the first (keepId), and fully discarded from the second (deleteId).
func (mc *MemoryController) Merge(keepId string, deleteId string) error {
	keep, err := mc.store.Get(keepId)
	if err != nil {
		return fmt.Errorf("getting first entry %q: %w", keepId, err)
	}
	del, err := mc.store.Get(deleteId)
	if err != nil {
		return fmt.Errorf("getting second entry %q: %w", deleteId, err)
	}

	tagSet := make(map[string]bool)
	for _, t := range keep.Tags {
		tagSet[t] = true
	}
	for _, t := range del.Tags {
		if !tagSet[t] {
			keep.Tags = append(keep.Tags, t)
			tagSet[t] = true
		}
	}

	keep.Score = math.Max(keep.Score, del.Score)
	keep.HitCount += del.HitCount
	if del.LastHit.After(keep.LastHit) {
		keep.LastHit = del.LastHit
	}

	// TODO: rather than keeping one content and deleting the other, is there value in merging the text?

	if err := mc.store.Upsert(keep); err != nil {
		return fmt.Errorf("updating kept entry after merge: %w", err)
	}

	if err := mc.store.Delete(deleteId); err != nil {
		return fmt.Errorf("deleting other entry after merge: %w", err)
	}

	return nil
}

func (mc *MemoryController) loadIndex() error {
	meta, err := loadMetadata(mc.metaPath)
	if err != nil {
		return fmt.Errorf("loading metadata: %w", err)
	}
	mc.meta = meta

	f, err := os.Open(mc.indexPath)
	if err != nil {
		if os.IsNotExist(err) {
			if len(mc.meta.Entries) > 0 {
				mc.logger.Warn("index file missing; clearing stale metadata", "meta_entries", len(mc.meta.Entries), "path", mc.indexPath)
				mc.meta = newMetadata()
			}
			return nil
		}
		return fmt.Errorf("opening index: %w", err)
	}
	defer f.Close() // nolint:errcheck

	type importer interface {
		Import(conf config.Config, r io.Reader) error
	}
	if imp, ok := mc.indexer.(importer); ok {
		if err := imp.Import(mc.conf, f); err != nil {
			return fmt.Errorf("importing index: %w", err)
		}
	}

	if err := mc.validateIndex(); err != nil {
		mc.logger.Warn("loaded index failed validation, rebuilding", "err", err)
		mc.indexer = hnsw.New(mc.conf)
		mc.meta = newMetadata()
		return nil
	}

	return nil
}

// validateIndex makes sure the index is usable.
// necessary because I was seeing panics lack of mutexes around hnsw graph, which is apparently not thread-safe.
func (mc *MemoryController) validateIndex() (retErr error) {
	if mc.indexer == nil || mc.meta == nil || len(mc.meta.Entries) == 0 {
		return nil
	}

	defer func() {
		if r := recover(); r != nil {
			retErr = fmt.Errorf("index validation panicked (corrupted index file): %v", r)
		}
	}()

	testVec := make([]float32, mc.conf.Index.Dimensions)
	for i := range testVec {
		testVec[i] = 0.5
	}

	mc.indexMu.RLock()
	_, retErr = mc.indexer.Search(testVec, 1)
	mc.indexMu.RUnlock()
	return retErr
}

// BuildIndexes builds the index or force rebuilds the entire index from scratch.
func (mc *MemoryController) BuildIndexes(force bool) error {
	if !mc.embedder.Available() {
		return ErrEmbedderNotAvailable
	}

	entries, err := mc.store.All(nil)
	if err != nil {
		return err
	}

	if force {
		mc.logger.Info("force rebuilding indexer",
			"type", "hnsw",
			"dimensions", mc.conf.Index.Dimensions,
			"connections", mc.conf.Index.Connections,
			"level_factor", mc.conf.Index.LevelFactor,
			"ef_search", mc.conf.Index.EfSearch)
		mc.indexer = hnsw.New(mc.conf)
		mc.meta = newMetadata()
	}

	// embed all entries not yet in the index
	activeIDs := make(map[string]struct{}, len(entries))
	var toEmbed []store.Entry
	for _, e := range entries {
		activeIDs[e.ID] = struct{}{}
		if !mc.meta.has(e.ID) {
			toEmbed = append(toEmbed, e)
		}
	}

	// remove stale entries
	for id := range mc.meta.Entries {
		if _, ok := activeIDs[id]; !ok {
			mc.indexMu.Lock()
			_ = mc.indexer.DeleteVector(id)
			mc.indexMu.Unlock()
			mc.meta.remove(id)
		}
	}

	if len(toEmbed) == 0 {
		mc.markDirty()
		return mc.flushIndex()
	}

	mc.logger.Info("indexing entries", "count", len(toEmbed), "force", force)

	texts := make([]string, len(toEmbed))
	for i, e := range toEmbed {
		texts[i] = e.Content
	}

	vectors, err := mc.embedder.Embed(texts)
	if err != nil {
		return fmt.Errorf("batch embedding: %w", err)
	}
	if len(vectors) != len(toEmbed) {
		return fmt.Errorf("batch embedding returned %d vectors for %d entries", len(vectors), len(toEmbed))
	}

	for i, e := range toEmbed {
		mc.indexMu.Lock()
		err := mc.indexer.InsertVector(e.ID, vectors[i])
		mc.indexMu.Unlock()
		if err != nil {
			mc.logger.Warn("failed to index entry", "id", e.ID, "err", err)
			continue
		}
		mc.meta.add(e.ID)
	}

	mc.markDirty()
	return mc.flushIndex()
}

func (mc *MemoryController) indexEntry(entry *store.Entry) {
	if !mc.embedder.Available() {
		return
	}
	vec, err := mc.embedder.EmbedSingle(entry.Content)
	if err != nil {
		mc.logger.Warn("failed to embed entry", "id", entry.ID, "err", err)
		return
	}
	mc.indexMu.Lock()
	defer mc.indexMu.Unlock()
	if err := mc.indexer.InsertVector(entry.ID, vec); err != nil {
		mc.logger.Warn("failed to index entry", "id", entry.ID, "err", err)
		return
	}
	mc.mu.Lock()
	mc.meta.add(entry.ID)
	mc.dirty = true
	mc.mu.Unlock()
}

func (mc *MemoryController) removeFromIndex(id string) {
	mc.mu.Lock()
	if !mc.meta.has(id) {
		mc.mu.Unlock()
		return
	}
	mc.meta.remove(id)
	mc.dirty = true
	mc.mu.Unlock()

	mc.indexMu.Lock()
	_ = mc.indexer.DeleteVector(id)
	mc.indexMu.Unlock()
}

func (mc *MemoryController) markDirty() {
	mc.mu.Lock()
	mc.dirty = true
	mc.mu.Unlock()
}

func (mc *MemoryController) flushLoop() {
	// TODO: maybe it makes sense to move flushing logic like this to shared struct (share here and yamlstore, plus any future stores)
	t := time.NewTicker(flushInterval)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			if err := mc.flushIndex(); err != nil {
				mc.logger.Warn("index flush error", "err", err)
			}
		case <-mc.done:
			return
		}
	}
}

func (mc *MemoryController) flushIndex() error {
	mc.mu.Lock()
	if !mc.dirty {
		mc.mu.Unlock()
		return nil
	}
	mc.dirty = false
	mc.mu.Unlock()

	if exp, ok := mc.indexer.(index.Exporter); ok {
		f, err := os.Create(mc.indexPath)
		if err != nil {
			return fmt.Errorf("creating index file: %w", err)
		}
		defer func(f *os.File) {
			if err := f.Close(); err != nil {
				mc.logger.Warn("error closing index file", "err", err)
			}
		}(f)
		mc.indexMu.RLock()
		err = exp.Export(f)
		mc.indexMu.RUnlock()
		if err != nil {
			return fmt.Errorf("exporting index: %w", err)
		}
	}

	if err := mc.meta.save(mc.metaPath); err != nil {
		return fmt.Errorf("saving metadata: %w", err)
	}

	return nil
}

func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}
