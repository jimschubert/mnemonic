package yamlstore

import (
	"fmt"
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

// file is the on-disk format.
type file struct {
	Version int           `yaml:"version"`
	Entries []store.Entry `yaml:"entries"`
}

var _ store.Store = (*YAMLStore)(nil)

// YAMLStore holds one or more YAML files (one per scope path).
type YAMLStore struct {
	mu    sync.RWMutex
	files map[store.Scope]string
	data  map[store.Scope]*file
}

// New creates a YAMLStore. The caller provides a mapping of scope names to a absolute or relative file path.
// The home directory can be referenced with "~". The file will be loaded or created.
//
// Example:
//
//	store, err := yamlstore.New(map[Scope]string{
//	    "global": "~/.mnemonic/global.yaml",
//	    "team:acme": "~/.mnemonic/data/team_acme.yaml",
//	})
func New(scopePaths map[store.Scope]string) (*YAMLStore, error) {
	s := &YAMLStore{
		files: scopePaths,
		data:  make(map[store.Scope]*file),
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	for scope, path := range scopePaths {
		if err := s.load(scope, path); err != nil {
			return nil, fmt.Errorf("loading scope %q from %s: %w", scope, path, err)
		}
	}

	return s, nil
}

func (s *YAMLStore) All(scopes []store.Scope) ([]store.Entry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.allEntries(s.scopesForQuery(scopes)), nil
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

func (s *YAMLStore) Query(category string, tags []string) ([]*store.Entry, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *YAMLStore) Upsert(entry *store.Entry) error {
	return fmt.Errorf("not implemented")
}

func (s *YAMLStore) Score(id string, delta float64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for scope, f := range s.data {
		for idx, entry := range f.Entries {
			if entry.ID == id {
				f.Entries[idx].Score = math.Max(0, entry.Score+delta)
				f.Entries[idx].HitCount++
				f.Entries[idx].LastHit = time.Now()
				return s.persist(scope)
			}
		}
	}
	return fmt.Errorf("entry %q not found", id)
}

func (s *YAMLStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for scope, wf := range s.data {
		for i, e := range wf.Entries {
			if e.ID == id {
				wf.Entries = append(wf.Entries[:i], wf.Entries[i+1:]...)
				return s.persist(scope)
			}
		}
	}
	return fmt.Errorf("entry %q not found", id)
}

// load reads or inits a file for a scope.
func (s *YAMLStore) load(scope store.Scope, path string) error {
	path = expandHome(path)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		s.data[scope] = new(file{Version: 1})
		return s.persist(scope)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var wf file
	if err := yaml.Unmarshal(b, &wf); err != nil {
		return err
	}
	s.data[scope] = &wf
	return nil
}

func (s *YAMLStore) persist(scope store.Scope) error {
	path := expandHome(s.files[scope])
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := yaml.Marshal(s.data[scope])
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

// scopesForQuery resolves which scopes to query.
func (s *YAMLStore) scopesForQuery(requested []store.Scope) []store.Scope {
	if len(requested) == 0 {
		return slices.Collect(maps.Keys(s.files))
	}
	return requested
}

// allEntries returns entries from requested scopes (caller must hold read lock).
func (s *YAMLStore) allEntries(scopes []store.Scope) []store.Entry {
	var out []store.Entry
	for _, sc := range scopes {
		if f, ok := s.data[sc]; ok && f != nil && len(f.Entries) > 0 {
			out = append(out, f.Entries...)
		}
	}
	return out
}

func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}
