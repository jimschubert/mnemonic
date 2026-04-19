package yamlstore

import (
	"crypto/rand"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"math"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jimschubert/mnemonic/internal/store"
	"go.yaml.in/yaml/v4"
)

// file is the on-disk format for a single category.
type file struct {
	Version int           `yaml:"version"`
	Entries []store.Entry `yaml:"entries"`
}

var _ store.Store = (*YAMLStore)(nil)

// TODO: make flush configurable
const flushInterval = 30 * time.Second

// YAMLStore holds one or more scope directories, each containing one YAML file per category.
//
// Example layout:
//
//	~/.mnemonic/global/
//	 ├── avoidance.yaml
//	 └── security.yaml
//	~/.mnemonic/teams/acme/
//	 └── avoidance.yaml
//	.mnemonic/project/
//	 └── domain.yaml
type YAMLStore struct {
	mu sync.RWMutex
	// dirs maps scope -> directory path
	dirs map[store.Scope]string
	// data maps scope -> category -> file
	data map[store.Scope]map[string]*file
	// dirty maps scope -> category -> needs flush
	dirty  map[store.Scope]map[string]bool
	done   chan struct{}
	logger *slog.Logger
}

// New creates a YAMLStore. Call with a mapping of scope names to directory paths.
// Expands "~" to the home directory. Each directory is scanned for *.yaml files
// on startup; each file is treated as a category (filename without extension).
//
// Example:
//
//	store, err := yamlstore.New(map[Scope]string{
//	    "global":    "~/.mnemonic/global/",
//	    "team:acme": "~/.mnemonic/teams/acme/",
//	}, logger)
func New(scopeDirs map[store.Scope]string, logger *slog.Logger) (*YAMLStore, error) {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	s := &YAMLStore{
		dirs:   scopeDirs,
		data:   make(map[store.Scope]map[string]*file),
		dirty:  make(map[store.Scope]map[string]bool),
		done:   make(chan struct{}),
		logger: logger,
	}

	for scope, dir := range scopeDirs {
		if err := s.loadScope(scope, dir); err != nil {
			return nil, fmt.Errorf("loading scope %q from %s: %w", scope, dir, err)
		}
	}

	go s.flushLoop()
	return s, nil
}

func (s *YAMLStore) flushLoop() {
	t := time.NewTicker(flushInterval)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			_ = s.flush()
		case <-s.done:
			return
		}
	}
}

// Close stops the background flush goroutine and persists any dirty categories.
func (s *YAMLStore) Close() error {
	close(s.done)
	return s.flush()
}

func (s *YAMLStore) All(scopes []store.Scope) ([]store.Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries := s.allEntries(s.scopesForQuery(scopes))
	s.markHits(time.Now(), entries)
	return entries, nil
}

func (s *YAMLStore) ListHeads(scopes []store.Scope) ([]store.HeadInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	counts := map[string]int{}
	for _, e := range s.allEntries(s.scopesForQuery(scopes)) {
		counts[e.Category]++
	}
	out := make([]store.HeadInfo, 0, len(counts))
	for name, count := range counts {
		out = append(out, store.HeadInfo{Name: name, Count: count, Mandatory: store.IsMandatoryCategory(name)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (s *YAMLStore) Get(id string) (*store.Entry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, e := range s.allEntries(s.scopesForQuery(nil)) {
		if e.ID == id {
			return &e, nil
		}
	}
	return nil, fmt.Errorf("entry %q not found", id)
}

// AllByCategory returns entries in the given category, sorted by weighted score.
// Only the per-category file for each scope is accessed, not all files.
func (s *YAMLStore) AllByCategory(category string, topK int, scopes []store.Scope) ([]store.Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var candidates []store.Entry
	for _, sc := range s.scopesForQuery(scopes) {
		if files, ok := s.data[sc]; ok {
			if f, ok := files[category]; ok {
				candidates = append(candidates, f.Entries...)
			}
		}
	}
	sortByWeightedScore(candidates)
	if topK > 0 && len(candidates) > topK {
		candidates = candidates[:topK]
	}
	s.markHits(time.Now(), candidates)
	return candidates, nil
}

// QueryByCategory returns entries in the given category whose content or tags contain any query
// term (case-insensitive), sorted by weighted score. Only the per-category file is accessed per scope.
func (s *YAMLStore) QueryByCategory(category, query string, topK int, scopes []store.Scope) ([]store.Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	queryTerms := strings.Fields(strings.ToLower(query))
	var candidates []store.Entry
	for _, sc := range s.scopesForQuery(scopes) {
		if files, ok := s.data[sc]; ok {
			if f, ok := files[category]; ok {
				for _, e := range f.Entries {
					if keywordMatch(e, queryTerms) {
						candidates = append(candidates, e)
					}
				}
			}
		}
	}
	sortByWeightedScore(candidates)
	if topK > 0 && len(candidates) > topK {
		candidates = candidates[:topK]
	}
	s.markHits(time.Now(), candidates)
	return candidates, nil
}

func (s *YAMLStore) Query(category string, tags []string) ([]*store.Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	lower := make([]string, len(tags))
	for i, t := range tags {
		lower[i] = strings.ToLower(t)
	}
	var hits []store.Entry
	var out []*store.Entry
	for _, e := range s.allEntries(s.scopesForQuery(nil)) {
		if category != "" && e.Category != category {
			continue
		}
		if !tagsMatch(e.Tags, lower) {
			continue
		}
		hits = append(hits, e)
		out = append(out, new(e))
	}
	s.markHits(time.Now(), hits)
	return out, nil
}

func (s *YAMLStore) Upsert(entry *store.Entry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !store.IsAllowedCategory(entry.Category) {
		return fmt.Errorf("category %q is not allowed; must be one of %v", entry.Category, store.AllowedCategories())
	}

	scope := store.Scope(entry.Scope)
	if scope == "" {
		scope = store.ScopeGlobal
	}
	files, ok := s.data[scope]
	if !ok {
		return fmt.Errorf("scope %q is not configured", scope)
	}

	if entry.ID == "" {
		var b [8]byte
		if _, err := rand.Read(b[:]); err != nil {
			return fmt.Errorf("generating ID: %w", err)
		}
		entry.ID = fmt.Sprintf("%x", b)
	}
	if entry.Score == 0 {
		entry.Score = 1.0
	}
	if entry.Created.IsZero() {
		entry.Created = time.Now()
	}

	category := entry.Category
	f, ok := files[category]
	if !ok {
		f = &file{Version: 1}
		files[category] = f
	}

	for i, e := range f.Entries {
		// update existing entry
		if e.ID == entry.ID {
			f.Entries[i] = *entry
			return s.persist(scope, category)
		}
	}

	// add new entry
	f.Entries = append(f.Entries, *entry)
	return s.persist(scope, category)
}

func (s *YAMLStore) Promote(id string, targetScope store.Scope) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.data[targetScope]; !ok {
		return fmt.Errorf("target scope %q is not configured", targetScope)
	}
	for scope, files := range s.data {
		if scope == targetScope {
			continue
		}
		for category, f := range files {
			for i, e := range f.Entries {
				if e.ID != id {
					continue
				}
				// remove from source scope's category file
				f.Entries = append(f.Entries[:i], f.Entries[i+1:]...)
				if err := s.persist(scope, category); err != nil {
					// failed to persist removal, put back and return error
					f.Entries = append(f.Entries[:i], append([]store.Entry{e}, f.Entries[i:]...)...)
					return err
				}
				e.Scope = targetScope.String()
				target := s.data[targetScope]
				if target[category] == nil {
					target[category] = &file{Version: 1}
				}
				target[category].Entries = append(target[category].Entries, e)
				return s.persist(targetScope, category)
			}
		}
	}
	return fmt.Errorf("entry %q not found", id)
}

// markHits increments HitCount and sets LastHit for the entries in s.data that match the given slice.
// Caller must hold write lock.
func (s *YAMLStore) markHits(now time.Time, entries []store.Entry) {
	if len(entries) == 0 {
		return
	}
	ids := make(map[string]struct{}, len(entries))
	for _, e := range entries {
		ids[e.ID] = struct{}{}
	}
	for scope, files := range s.data {
		for category, f := range files {
			for i, e := range f.Entries {
				if _, ok := ids[e.ID]; ok {
					f.Entries[i].HitCount++
					f.Entries[i].LastHit = now
					s.dirty[scope][category] = true
				}
			}
		}
	}
}

// flush persists all dirty (scope, category) pairs.
func (s *YAMLStore) flush() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for scope, files := range s.dirty {
		for category := range files {
			if err := s.persist(scope, category); err != nil {
				s.logger.Warn("flush error", "scope", scope, "category", category, "err", err)
				return err
			}
			s.logger.Debug("flushed category", "scope", scope, "category", category)
			delete(files, category)
		}
	}
	return nil
}

func (s *YAMLStore) Score(id string, delta float64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for scope, files := range s.data {
		for category, f := range files {
			for idx, entry := range f.Entries {
				if entry.ID == id {
					f.Entries[idx].Score = math.Max(0, entry.Score+delta)
					f.Entries[idx].HitCount++
					f.Entries[idx].LastHit = time.Now()
					return s.persist(scope, category)
				}
			}
		}
	}
	return fmt.Errorf("entry %q not found", id)
}

func (s *YAMLStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for scope, files := range s.data {
		for category, f := range files {
			for i, e := range f.Entries {
				if e.ID == id {
					f.Entries = append(f.Entries[:i], f.Entries[i+1:]...)
					return s.persist(scope, category)
				}
			}
		}
	}
	return fmt.Errorf("entry %q not found", id)
}

// loadScope scans dir for *.yaml files and loads each as a separate category.
func (s *YAMLStore) loadScope(scope store.Scope, dir string) error {
	dir = expandHome(dir)
	s.data[scope] = make(map[string]*file)
	s.dirty[scope] = make(map[string]bool)

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	dirEntries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, dirEntry := range dirEntries {
		if dirEntry.IsDir() || !strings.HasSuffix(dirEntry.Name(), ".yaml") {
			continue
		}
		category := strings.TrimSuffix(dirEntry.Name(), ".yaml")
		f, err := readCategoryFile(filepath.Join(dir, dirEntry.Name()))
		if err != nil {
			return fmt.Errorf("loading category %q: %w", category, err)
		}
		s.data[scope][category] = f
		s.logger.Info("loaded category", "scope", scope, "category", category, "entries", len(f.Entries))
	}
	s.logger.Info("loaded scope", "scope", scope, "dir", dir, "categories", len(s.data[scope]))
	return nil
}

func readCategoryFile(path string) (*file, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var f file
	if err := yaml.Unmarshal(b, &f); err != nil {
		return nil, err
	}
	return &f, nil
}

func (s *YAMLStore) persist(scope store.Scope, category string) error {
	dir := expandHome(s.dirs[scope])
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	b, err := yaml.Marshal(s.data[scope][category])
	if err != nil {
		return err
	}
	path := filepath.Join(dir, category+".yaml")
	s.logger.Debug("persisting category", "scope", scope, "category", category, "path", path)
	return os.WriteFile(path, b, 0o644)
}

// scopesForQuery resolves which scopes to query.
func (s *YAMLStore) scopesForQuery(requested []store.Scope) []store.Scope {
	if len(requested) == 0 {
		return slices.Collect(maps.Keys(s.dirs))
	}
	return requested
}

// allEntries returns all entries from all category files in the requested scopes (caller must hold lock).
func (s *YAMLStore) allEntries(scopes []store.Scope) []store.Entry {
	var out []store.Entry
	for _, sc := range scopes {
		for _, f := range s.data[sc] {
			if f != nil {
				out = append(out, f.Entries...)
			}
		}
	}
	return out
}

// tagsMatch returns true if all required tags are present in entryTags (case-insensitive).
// If required is empty, it matches any entry.
// Caller must pass required tags in lowercase for efficiency.
func tagsMatch(entryTags []string, required []string) bool {
	if len(required) == 0 {
		return true
	}

	set := make(map[string]struct{}, len(entryTags))
	for _, t := range entryTags {
		set[strings.ToLower(t)] = struct{}{}
	}
	for _, r := range required {
		if _, ok := set[r]; !ok {
			return false
		}
	}
	return true
}

func keywordMatch(e store.Entry, terms []string) bool {
	if len(terms) == 0 {
		return true
	}
	content := strings.ToLower(e.Content)
	for _, t := range terms {
		if strings.Contains(content, t) {
			return true
		}
	}
	for _, tag := range e.Tags {
		tag = strings.ToLower(tag)
		for _, t := range terms {
			if strings.Contains(tag, t) {
				return true
			}
		}
	}
	return false
}

func sortByWeightedScore(entries []store.Entry) {
	sort.Slice(entries, func(i, j int) bool {
		return weightedScore(entries[i]) > weightedScore(entries[j])
	})
}

// weightedScore adds a recency bias to the entry's score. Recent hits increase the score, while old entries decay over time.
// The half-life is ~14 days (ln(2) / 0.05). LastHit is used as the recency reference; Created is the fallback for
// entries that have never been queried or reinforced.
func weightedScore(e store.Entry) float64 {
	ref := e.LastHit
	if ref.IsZero() {
		ref = e.Created
	}
	days := time.Since(ref).Hours() / 24
	decay := math.Exp(-0.05 * days)
	return e.Score * decay
}

func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}
