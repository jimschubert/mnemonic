package yamlstore

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alecthomas/assert/v2"
	"github.com/jimschubert/mnemonic/internal/store"
)

const (
	singleEntryYAML = `version: 1
entries:
  - id: test-entry
    content: "Test"
    tags: []
    category: test
    scope: global
    score: 0.5
    hit_count: 0
    last_hit: 2025-01-01T00:00:00Z
    created: 2025-01-01T00:00:00Z
    source: manual
`

	multiEntryYAML = `version: 1
entries:
  - id: first
    content: "First"
    tags: []
    category: test
    scope: global
    score: 1.0
    hit_count: 0
    last_hit: 2025-01-01T00:00:00Z
    created: 2025-01-01T00:00:00Z
    source: manual
  - id: second
    content: "Second"
    tags: []
    category: test
    scope: global
    score: 1.0
    hit_count: 0
    last_hit: 2025-01-02T00:00:00Z
    created: 2025-01-02T00:00:00Z
    source: manual
  - id: third
    content: "Third"
    tags: []
    category: test
    scope: global
    score: 1.0
    hit_count: 0
    last_hit: 2025-01-03T00:00:00Z
    created: 2025-01-03T00:00:00Z
    source: manual
`

	oldTimestampYAML = `version: 1
entries:
  - id: test-entry
    content: "Test"
    tags: []
    category: test
    scope: global
    score: 0.5
    hit_count: 0
    last_hit: 2020-01-01T00:00:00Z
    created: 2020-01-01T00:00:00Z
    source: manual
`

	globalScopeYAML = `version: 1
entries:
  - id: global-entry
    content: "Global content"
    tags: [global]
    category: misc
    scope: global
    score: 1.0
    hit_count: 0
    last_hit: 2025-01-01T00:00:00Z
    created: 2025-01-01T00:00:00Z
    source: manual
`

	teamScopeYAML = `version: 1
entries:
  - id: team-entry
    content: "Team content"
    tags: [team]
    category: misc
    scope: team
    score: 0.5
    hit_count: 1
    last_hit: 2025-01-02T00:00:00Z
    created: 2025-01-02T00:00:00Z
    source: manual
`
)

func loadTestdata(t *testing.T) string {
	t.Helper()
	content, err := os.ReadFile("testdata/entries.yaml")
	if err != nil {
		t.Fatalf("failed to read testdata/entries.yaml: %v", err)
	}
	return string(content)
}

// setupTempYAML is used for tests which modify in-place
func setupTempYAML(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write temp YAML: %v", err)
	}
	return path
}

func newTestStore(t *testing.T, content string) (*YAMLStore, string) {
	t.Helper()
	path := setupTempYAML(t, content)
	s, err := New(map[store.Scope]string{store.ScopeGlobal: path})
	if err != nil {
		t.Fatalf("failed to create test store: %v", err)
	}
	return s, path
}

func TestNew_LoadExistingFile(t *testing.T) {
	s, _ := newTestStore(t, loadTestdata(t))

	entries, err := s.All(nil)
	assert.NoError(t, err)
	assert.Equal(t, 5, len(entries))
}

func TestNew_CreatesNonExistentFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "new.yaml")

	s, err := New(map[store.Scope]string{store.ScopeGlobal: path})
	assert.NoError(t, err)

	entries, err := s.All(nil)
	assert.NoError(t, err)
	assert.Equal(t, 0, len(entries))

	_, err = os.Stat(path)
	assert.NoError(t, err)
}

func TestNew_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yaml")
	err := os.WriteFile(path, []byte("{{{invalid yaml}}}"), 0o644)
	assert.NoError(t, err)

	_, err = New(map[store.Scope]string{store.ScopeGlobal: path})
	assert.Error(t, err)
}

func TestAll(t *testing.T) {
	dir := t.TempDir()
	globalPath := filepath.Join(dir, "global.yaml")
	projectPath := filepath.Join(dir, "project.yaml")

	globalYAML := `version: 1
entries:
  - id: entry-global-1
    content: "Global entry 1"
    tags: []
    category: test
    scope: global
    score: 1.0
    hit_count: 0
    last_hit: 2025-01-01T00:00:00Z
    created: 2025-01-01T00:00:00Z
    source: manual
  - id: entry-global-2
    content: "Global entry 2"
    tags: []
    category: test
    scope: global
    score: 1.0
    hit_count: 0
    last_hit: 2025-01-01T00:00:00Z
    created: 2025-01-01T00:00:00Z
    source: manual
`

	projectYAML := `version: 1
entries:
  - id: entry-project-1
    content: "Project entry 1"
    tags: []
    category: test
    scope: project
    score: 1.0
    hit_count: 0
    last_hit: 2025-01-01T00:00:00Z
    created: 2025-01-01T00:00:00Z
    source: manual
`

	err := os.WriteFile(globalPath, []byte(globalYAML), 0o644)
	assert.NoError(t, err)
	err = os.WriteFile(projectPath, []byte(projectYAML), 0o644)
	assert.NoError(t, err)

	s, err := New(map[store.Scope]string{
		store.ScopeGlobal: globalPath,
		"project":         projectPath,
	})
	assert.NoError(t, err)

	t.Run("all scopes", func(t *testing.T) {
		entries, err := s.All(nil)
		assert.NoError(t, err)
		assert.Equal(t, 3, len(entries))
	})

	t.Run("specific scope global", func(t *testing.T) {
		entries, err := s.All([]store.Scope{store.ScopeGlobal})
		assert.NoError(t, err)
		assert.Equal(t, 2, len(entries))
	})

	t.Run("specific scope project", func(t *testing.T) {
		entries, err := s.All([]store.Scope{"project"})
		assert.NoError(t, err)
		assert.Equal(t, 1, len(entries))
	})

	t.Run("non-existent scope", func(t *testing.T) {
		entries, err := s.All([]store.Scope{"nonexistent"})
		assert.NoError(t, err)
		assert.Equal(t, 0, len(entries))
	})
}

func TestListHeads(t *testing.T) {
	s, _ := newTestStore(t, loadTestdata(t))

	heads, err := s.ListHeads(nil)
	assert.NoError(t, err)
	assert.Equal(t, 5, len(heads))

	for i := 1; i < len(heads); i++ {
		assert.True(t, heads[i-1].Name < heads[i].Name, "heads should be sorted")
	}

	expected := map[string]int{
		"avoidance":    1,
		"security":     1,
		"syntax":       1,
		"architecture": 1,
		"domain":       1,
	}
	for _, h := range heads {
		assert.Equal(t, expected[h.Name], h.Count)
	}
}

func TestGet(t *testing.T) {
	s, _ := newTestStore(t, loadTestdata(t))

	t.Run("existing entry", func(t *testing.T) {
		entry, err := s.Get("go-error-wrapping")
		assert.NoError(t, err)
		assert.NotZero(t, entry)
		assert.Equal(t, "go-error-wrapping", entry.ID)
		assert.Equal(t, "syntax", entry.Category)
	})

	t.Run("non-existent entry", func(t *testing.T) {
		_, err := s.Get("does-not-exist")
		assert.Error(t, err)
	})
}

func TestScore(t *testing.T) {
	s, _ := newTestStore(t, loadTestdata(t))

	t.Run("positive delta", func(t *testing.T) {
		err := s.Score("go-error-wrapping", 0.1)
		assert.NoError(t, err)

		entry, err := s.Get("go-error-wrapping")
		assert.NoError(t, err)
		assert.Equal(t, 1.0, entry.Score)
		assert.Equal(t, 13, entry.HitCount)
		assert.False(t, entry.LastHit.IsZero())
	})

	t.Run("negative delta clamped to zero", func(t *testing.T) {
		err := s.Score("go-error-wrapping", -999.0)
		assert.NoError(t, err)

		entry, err := s.Get("go-error-wrapping")
		assert.NoError(t, err)
		assert.Equal(t, 0.0, entry.Score)
	})

	t.Run("non-existent entry", func(t *testing.T) {
		err := s.Score("does-not-exist", 0.5)
		assert.Error(t, err)
	})

	t.Run("persistence", func(t *testing.T) {
		path := setupTempYAML(t, loadTestdata(t))
		s, err := New(map[store.Scope]string{store.ScopeGlobal: path})
		assert.NoError(t, err)

		err = s.Score("stencil-custom-funcs", 0.25)
		assert.NoError(t, err)

		s2, err := New(map[store.Scope]string{store.ScopeGlobal: path})
		assert.NoError(t, err)
		entry, err := s2.Get("stencil-custom-funcs")
		assert.NoError(t, err)
		assert.Equal(t, 1.0, entry.Score)
		assert.Equal(t, 4, entry.HitCount)
	})
}

func TestDelete(t *testing.T) {
	s, _ := newTestStore(t, loadTestdata(t))

	t.Run("existing entry", func(t *testing.T) {
		err := s.Delete("avoid-left-recursion")
		assert.NoError(t, err)

		_, err = s.Get("avoid-left-recursion")
		assert.Error(t, err)

		entries, err := s.All(nil)
		assert.NoError(t, err)
		assert.Equal(t, 4, len(entries))
	})

	t.Run("non-existent entry", func(t *testing.T) {
		err := s.Delete("does-not-exist")
		assert.Error(t, err)
	})

	t.Run("persistence", func(t *testing.T) {
		path := setupTempYAML(t, loadTestdata(t))
		s, err := New(map[store.Scope]string{store.ScopeGlobal: path})
		assert.NoError(t, err)

		err = s.Delete("service-layer-pattern")
		assert.NoError(t, err)

		s2, err := New(map[store.Scope]string{store.ScopeGlobal: path})
		assert.NoError(t, err)
		_, err = s2.Get("service-layer-pattern")
		assert.Error(t, err)
	})
}

func TestScopesForQuery(t *testing.T) {
	s, _ := newTestStore(t, loadTestdata(t))

	t.Run("nil returns all scopes", func(t *testing.T) {
		scopes := s.scopesForQuery(nil)
		assert.Equal(t, 1, len(scopes))
	})

	t.Run("empty returns all scopes", func(t *testing.T) {
		scopes := s.scopesForQuery([]store.Scope{})
		assert.Equal(t, 1, len(scopes))
	})

	t.Run("specific scope", func(t *testing.T) {
		scopes := s.scopesForQuery([]store.Scope{"global"})
		assert.Equal(t, 1, len(scopes))
		assert.Equal(t, "global", scopes[0])
	})
}

func TestAllEntries(t *testing.T) {
	s, _ := newTestStore(t, loadTestdata(t))

	t.Run("global scope", func(t *testing.T) {
		entries := s.allEntries([]store.Scope{store.ScopeGlobal})
		assert.Equal(t, 5, len(entries))
	})

	t.Run("non-existent scope", func(t *testing.T) {
		entries := s.allEntries([]store.Scope{"nonexistent"})
		assert.Equal(t, 0, len(entries))
	})

	t.Run("all scopes", func(t *testing.T) {
		entries := s.allEntries(s.scopesForQuery(nil))
		assert.Equal(t, 5, len(entries))
	})
}

func TestExpandHome(t *testing.T) {
	t.Run("no tilde", func(t *testing.T) {
		assert.Equal(t, "/absolute/path.yaml", expandHome("/absolute/path.yaml"))
	})

	t.Run("relative path", func(t *testing.T) {
		assert.Equal(t, "relative/path.yaml", expandHome("relative/path.yaml"))
	})

	t.Run("tilde path", func(t *testing.T) {
		home, err := os.UserHomeDir()
		assert.NoError(t, err)
		assert.Equal(t, filepath.Join(home, "test.yaml"), expandHome("~/test.yaml"))
	})
}

func TestMultipleScopes(t *testing.T) {
	dir := t.TempDir()
	globalPath := filepath.Join(dir, "global.yaml")
	teamPath := filepath.Join(dir, "team.yaml")

	err := os.WriteFile(globalPath, []byte(globalScopeYAML), 0o644)
	assert.NoError(t, err)
	err = os.WriteFile(teamPath, []byte(teamScopeYAML), 0o644)
	assert.NoError(t, err)

	s, err := New(map[store.Scope]string{
		"global":    globalPath,
		"team:acme": teamPath,
	})
	assert.NoError(t, err)

	t.Run("all entries from all scopes", func(t *testing.T) {
		entries, err := s.All(nil)
		assert.NoError(t, err)
		assert.Equal(t, 2, len(entries))
	})

	t.Run("entries from global only", func(t *testing.T) {
		entries, err := s.All([]store.Scope{"global"})
		assert.NoError(t, err)
		assert.Equal(t, 1, len(entries))
		assert.Equal(t, "global-entry", entries[0].ID)
	})

	t.Run("entries from team only", func(t *testing.T) {
		entries, err := s.All([]store.Scope{"team:acme"})
		assert.NoError(t, err)
		assert.Equal(t, 1, len(entries))
		assert.Equal(t, "team-entry", entries[0].ID)
	})

	t.Run("get global entry", func(t *testing.T) {
		entry, err := s.Get("global-entry")
		assert.NoError(t, err)
		assert.Equal(t, "global-entry", entry.ID)
	})

	t.Run("get team entry", func(t *testing.T) {
		entry, err := s.Get("team-entry")
		assert.NoError(t, err)
		assert.Equal(t, "team-entry", entry.ID)
	})

	t.Run("score global entry", func(t *testing.T) {
		err := s.Score("global-entry", 0.1)
		assert.NoError(t, err)
		entry, err := s.Get("global-entry")
		assert.NoError(t, err)
		assert.Equal(t, 1.1, entry.Score)
	})

	t.Run("delete team entry", func(t *testing.T) {
		err := s.Delete("team-entry")
		assert.NoError(t, err)
		_, err = s.Get("team-entry")
		assert.Error(t, err)
	})
}

func TestScore_MaxClamp(t *testing.T) {
	s, _ := newTestStore(t, singleEntryYAML)

	err := s.Score("test-entry", -1.0)
	assert.NoError(t, err)

	entry, err := s.Get("test-entry")
	assert.NoError(t, err)
	assert.Equal(t, 0.0, entry.Score)
}

func TestPersist_CreatesDirectoryIfNeeded(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "nested.yaml")

	_, err := New(map[store.Scope]string{store.ScopeGlobal: path})
	assert.NoError(t, err)

	_, err = os.Stat(filepath.Dir(path))
	assert.NoError(t, err)
}

func TestScore_UpdatesLastHit(t *testing.T) {
	s, _ := newTestStore(t, oldTimestampYAML)

	before := time.Now()
	err := s.Score("test-entry", 0.1)
	assert.NoError(t, err)
	after := time.Now()

	entry, err := s.Get("test-entry")
	assert.NoError(t, err)

	assert.True(t, !entry.LastHit.Before(before) && !entry.LastHit.After(after),
		"LastHit should be between before and after times")
}

func TestListHeads_EmptyStore(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.yaml")

	s, err := New(map[store.Scope]string{store.ScopeGlobal: path})
	assert.NoError(t, err)

	heads, err := s.ListHeads(nil)
	assert.NoError(t, err)
	assert.Equal(t, 0, len(heads))
}

func TestScore_MultipleEntries(t *testing.T) {
	s, _ := newTestStore(t, loadTestdata(t))

	err := s.Score("avoid-left-recursion", 0.5)
	assert.NoError(t, err)

	err = s.Score("sensitive-data-redaction", -0.5)
	assert.NoError(t, err)

	entry1, err := s.Get("avoid-left-recursion")
	assert.NoError(t, err)
	assert.Equal(t, 1.5, entry1.Score)
	assert.Equal(t, 8, entry1.HitCount)

	entry2, err := s.Get("sensitive-data-redaction")
	assert.NoError(t, err)
	assert.Equal(t, 0.5, entry2.Score)
	assert.Equal(t, 24, entry2.HitCount)

	entry3, err := s.Get("go-error-wrapping")
	assert.NoError(t, err)
	assert.Equal(t, 0.9, entry3.Score)
	assert.Equal(t, 12, entry3.HitCount)
}

func TestDelete_FirstAndLastEntry(t *testing.T) {
	s, _ := newTestStore(t, multiEntryYAML)

	t.Run("delete first entry", func(t *testing.T) {
		err := s.Delete("first")
		assert.NoError(t, err)
		entries, err := s.All(nil)
		assert.NoError(t, err)
		assert.Equal(t, 2, len(entries))
	})

	t.Run("delete last entry", func(t *testing.T) {
		err := s.Delete("third")
		assert.NoError(t, err)
		entries, err := s.All(nil)
		assert.NoError(t, err)
		assert.Equal(t, 1, len(entries))
		assert.Equal(t, "second", entries[0].ID)
	})
}

func TestListHeads_MandatoryCategories(t *testing.T) {
	s, _ := newTestStore(t, loadTestdata(t))

	heads, err := s.ListHeads(nil)
	assert.NoError(t, err)

	expectedMandatory := map[string]bool{
		"avoidance":    true,
		"security":     true,
		"syntax":       false,
		"architecture": false,
		"domain":       false,
	}

	for _, head := range heads {
		expected, ok := expectedMandatory[head.Name]
		assert.True(t, ok, "unexpected category %q", head.Name)
		assert.Equal(t, expected, head.Mandatory, "category %q mandatory should be %v", head.Name, expected)
	}
}

