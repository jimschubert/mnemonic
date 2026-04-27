package controller

import (
	"cmp"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/jimschubert/mnemonic/internal/config"
	"github.com/jimschubert/mnemonic/internal/embed"
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
	SemanticSearch(query string, k int, scopes []store.Scope, categories []string) ([]store.Entry, error)
}

// MemoryController wraps a Store with optional semantic indexing via HNSW.
//
// When an Embedder is available, it keeps vectors in sync with store entries and supports SemanticSearch.
// When no Embedder is available, it passes through to the inner store.
type MemoryController struct {
	store        store.Store
	embedder     embed.Embedder
	indexManager IndexManager
	logger       *slog.Logger
	conf         config.Config

	done chan struct{}
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
		store:    o.store,
		embedder: o.embedder,
		conf:     conf,
		logger:   o.logger,
		done:     make(chan struct{}),
	}

	if o.indexManager != nil {
		mc.indexManager = o.indexManager
	} else {
		var err error
		if conf.Index.Type == "sqlite" {
			mc.logger.Info("creating sqlite index manager", "path", filepath.Join(dir, "index.db"))
			mc.indexManager, err = newSqliteManager(filepath.Join(dir, "index.db"), conf, o.logger)
			if err != nil {
				return nil, fmt.Errorf("initializing sqlite index manager: %w", err)
			}
		} else {
			mc.logger.Info("creating HNSW index manager", "path", filepath.Join(dir, "index.hnsw"), "config", conf.Index)
			mc.indexManager = newHnswManager(filepath.Join(dir, "index.hnsw"), filepath.Join(dir, "index.hnsw.json"), conf, o.logger)
		}
	}

	if o.embedder.Available() {
		// only HNSW-backed managers expose Load.
		if mgr, ok := mc.indexManager.(interface{ Load() error }); ok {
			if err := mgr.Load(); err != nil {
				mc.logger.Warn("failed to load index", "err", err)
			}
		}

		if !o.skipInitialSync {
			mc.logger.Info("syncing index with store entries on startup")
			if err := mc.BuildIndexes(false); err != nil && !errors.Is(err, ErrEmbedderNotAvailable) {
				mc.logger.Warn("index sync error", "err", err)
			}
		}
	} else {
		mc.logger.Info("embedder unavailable, skipping index load and sync")
	}

	go mc.flushLoop()
	return mc, nil
}

// Close stops the flush loop, persists the index, and closes the inner store.
func (mc *MemoryController) Close() error {
	close(mc.done)
	var errs error
	if err := mc.indexManager.Close(); err != nil {
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
	if mc.embedder.Available() {
		mc.indexManager.IndexEntry(entry, mc.embedder.EmbedSingle)
	}
	return nil
}

// Save persists an entry without embedding or deduplication.
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

	results, err := mc.indexManager.Search(vec, 3)
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
	if mc.embedder.Available() {
		mc.indexManager.RemoveFromIndex(id)
	}
	return nil
}

// SemanticSearch embeds the query and returns the k nearest entries.
// When categories are provided, semantic candidates category-limited so query filters apply consistently across semantic and keyword search.
// Returns nil when embedder is not available, or an error if embedding or index search fails.
func (mc *MemoryController) SemanticSearch(query string, k int, scopes []store.Scope, categories []string) ([]store.Entry, error) {
	if !mc.embedder.Available() {
		return nil, nil
	}

	vec, err := mc.embedder.EmbedSingle(query)
	if err != nil {
		return nil, fmt.Errorf("embedding query: %w", err)
	}

	var searchK int
	switch {
	case k <= 0:
		searchK = 10
	case len(categories) > 0:
		searchK = k * 4
	default:
		searchK = k * 2
	}

	results, err := mc.indexManager.Search(vec, searchK)
	if err != nil {
		return nil, fmt.Errorf("index search: %w", err)
	}

	scopeSet := make(map[string]bool, len(scopes))
	for _, s := range scopes {
		scopeSet[s.String()] = true
	}
	categorySet := make(map[string]bool, len(categories))
	for _, category := range categories {
		categorySet[category] = true
	}

	type semanticCandidate struct {
		entry    store.Entry
		distance float32
	}

	candidates := make([]semanticCandidate, 0, len(results))
	for _, r := range results {
		entry, err := mc.store.Get(r.ID)
		if err != nil {
			mc.indexManager.RemoveFromIndex(r.ID)
			continue
		}
		if len(scopeSet) > 0 && !scopeSet[entry.Scope] {
			continue
		}
		if len(categorySet) > 0 && !categorySet[entry.Category] {
			continue
		}
		candidates = append(candidates, semanticCandidate{entry: *entry, distance: r.Distance})
	}

	slices.SortStableFunc(candidates, func(a, b semanticCandidate) int {
		if math.Abs(float64(a.distance-b.distance)) > 1e-10 {
			return cmp.Compare(a.distance, b.distance)
		}
		aw, bw := store.WeightedScore(a.entry), store.WeightedScore(b.entry)
		if math.Abs(aw-bw) > 1e-10 {
			return cmp.Compare(bw, aw)
		}
		return strings.Compare(a.entry.ID, b.entry.ID)
	})

	var entries []store.Entry
	for _, candidate := range candidates {
		entries = append(entries, candidate.entry)
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
	vec, ok := mc.indexManager.LookupVector(id)

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

	// k=10 should be good, but maybe this could be configurable?
	results, err := mc.indexManager.Search(vec, 10)
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

// BuildIndexes builds the index or force rebuilds the entire index from scratch.
func (mc *MemoryController) BuildIndexes(force bool) error {
	if !mc.embedder.Available() {
		return ErrEmbedderNotAvailable
	}

	entries, err := mc.store.All(nil)
	if err != nil {
		return err
	}

	return mc.indexManager.BuildIndexes(entries, force, mc.embedder.Embed)
}

func (mc *MemoryController) flushLoop() {
	t := time.NewTicker(flushInterval)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			if err := mc.indexManager.Flush(); err != nil {
				mc.logger.Warn("index flush error", "err", err)
			}
		case <-mc.done:
			return
		}
	}
}

func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}
