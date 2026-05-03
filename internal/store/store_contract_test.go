package store_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/alecthomas/assert/v2"
	"github.com/jimschubert/mnemonic/internal/store"
	"github.com/jimschubert/mnemonic/internal/store/sqlitestore"
	"github.com/jimschubert/mnemonic/internal/store/yamlstore"
)

type storeFactory func(t *testing.T) (store.Store, func())

func TestStoreContract(t *testing.T) {
	tests := []struct {
		name    string
		factory storeFactory
	}{
		{
			name: "yamlstore",
			factory: func(t *testing.T) (store.Store, func()) {
				t.Helper()
				scopeDirs := map[store.Scope]string{
					store.ScopeGlobal: filepath.Join(t.TempDir(), "global"),
					"project":         filepath.Join(t.TempDir(), "project"),
				}
				ys, err := yamlstore.New(scopeDirs, nil, yamlstore.WithAutoHitCounting(false))
				assert.NoError(t, err)
				return ys, func() {
					assert.NoError(t, ys.Close())
				}
			},
		},
		{
			name: "sqlitestore",
			factory: func(t *testing.T) (store.Store, func()) {
				t.Helper()
				s, err := sqlitestore.New(filepath.Join(t.TempDir(), "contract.db"), nil, sqlitestore.WithAutoHitCounting(false))
				assert.NoError(t, err)
				return s, func() {
					assert.NoError(t, s.Close())
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runStoreContract(t, tt.factory)
		})
	}
}

func runStoreContract(t *testing.T, factory storeFactory) {
	t.Helper()

	t.Run("upsert and get", func(t *testing.T) {
		s, cleanup := factory(t)
		t.Cleanup(cleanup)

		entry := store.Entry{
			ID:       "syntax-1",
			Content:  "wrap errors with context",
			Tags:     []string{"go", "errors"},
			Category: "syntax",
			Scope:    "global",
			Score:    0.8,
			Created:  time.Now().Add(-1 * time.Hour),
			Source:   "test",
		}
		assert.NoError(t, s.Upsert(&entry))

		got, err := s.Get("syntax-1")
		assert.NoError(t, err)
		assert.Equal(t, entry.ID, got.ID)
		assert.Equal(t, entry.Content, got.Content)
		assert.Equal(t, entry.Category, got.Category)
		assert.Equal(t, entry.Scope, got.Scope)
		assert.Equal(t, entry.Tags, got.Tags)
		assert.Equal(t, entry.Score, got.Score)
		assert.True(t, got.Created.Sub(entry.Created) < time.Minute)
	})

	t.Run("all filters by scope", func(t *testing.T) {
		s, cleanup := factory(t)
		t.Cleanup(cleanup)
		seedEntries(t, s)

		all, err := s.All(nil)
		assert.NoError(t, err)
		assert.Equal(t, 3, len(all))

		globalOnly, err := s.All([]store.Scope{store.ScopeGlobal})
		assert.NoError(t, err)
		assert.Equal(t, 2, len(globalOnly))

		projectOnly, err := s.All([]store.Scope{"project"})
		assert.NoError(t, err)
		assert.Equal(t, 1, len(projectOnly))
	})

	t.Run("list heads sorted by mandatory flags", func(t *testing.T) {
		s, cleanup := factory(t)
		t.Cleanup(cleanup)
		seedEntries(t, s)

		heads, err := s.ListHeads(nil)
		assert.NoError(t, err)
		assert.Equal(t, 2, len(heads))
		assert.Equal(t, "avoidance", heads[0].Name)
		assert.Equal(t, "syntax", heads[1].Name)
		assert.True(t, heads[0].Mandatory)
		assert.False(t, heads[1].Mandatory)
	})

	t.Run("query matches tags case-insensitively with AND semantics", func(t *testing.T) {
		s, cleanup := factory(t)
		t.Cleanup(cleanup)
		seedEntries(t, s)

		hits, err := s.Query("", []string{"GO", "errors"})
		assert.NoError(t, err)
		assert.Equal(t, 1, len(hits))
		assert.Equal(t, "syntax-1", hits[0].ID)

		hits, err = s.Query("syntax", []string{"go"})
		assert.NoError(t, err)
		assert.Equal(t, 2, len(hits))
	})

	t.Run("weighted sort ordering is deterministic", func(t *testing.T) {
		s, cleanup := factory(t)
		t.Cleanup(cleanup)
		seedEntries(t, s)

		hits, err := s.Query("syntax", []string{"go"})
		assert.NoError(t, err)
		assert.Equal(t, 2, len(hits))
		assert.Equal(t, "syntax-1", hits[0].ID)
		assert.Equal(t, "syntax-2", hits[1].ID)

		allByCategory, err := s.AllByCategory("syntax", 0, nil)
		assert.NoError(t, err)
		assert.Equal(t, 2, len(allByCategory))
		assert.Equal(t, "syntax-1", allByCategory[0].ID)
		assert.Equal(t, "syntax-2", allByCategory[1].ID)

		queryByCategory, err := s.QueryByCategory("syntax", "go", 0, nil)
		assert.NoError(t, err)
		assert.Equal(t, 2, len(queryByCategory))
		assert.Equal(t, "syntax-1", queryByCategory[0].ID)
		assert.Equal(t, "syntax-2", queryByCategory[1].ID)
	})

	t.Run("score updates and clamps to zero", func(t *testing.T) {
		s, cleanup := factory(t)
		t.Cleanup(cleanup)
		seedEntries(t, s)

		assert.NoError(t, s.Score("syntax-2", -9.0))
		entry, err := s.Get("syntax-2")
		assert.NoError(t, err)
		assert.Equal(t, 0.0, entry.Score)
	})

	t.Run("delete removes entry", func(t *testing.T) {
		s, cleanup := factory(t)
		t.Cleanup(cleanup)
		seedEntries(t, s)

		assert.NoError(t, s.Delete("avoid-1"))
		_, err := s.Get("avoid-1")
		assert.Error(t, err)
	})

	t.Run("promote moves scope", func(t *testing.T) {
		s, cleanup := factory(t)
		t.Cleanup(cleanup)
		seedEntries(t, s)

		assert.NoError(t, s.Promote("syntax-1", "project"))

		globalOnly, err := s.All([]store.Scope{store.ScopeGlobal})
		assert.NoError(t, err)
		assert.Equal(t, 1, len(globalOnly))

		projectOnly, err := s.All([]store.Scope{"project"})
		assert.NoError(t, err)
		assert.Equal(t, 2, len(projectOnly))

		moved, err := s.Get("syntax-1")
		assert.NoError(t, err)
		assert.Equal(t, "project", moved.Scope)
	})
}

func seedEntries(t *testing.T, s store.Store) {
	t.Helper()

	entries := []store.Entry{
		{
			ID:       "syntax-1",
			Content:  "use wrapped go errors",
			Tags:     []string{"go", "errors"},
			Category: "syntax",
			Scope:    "global",
			Score:    2.0,
			Created:  time.Now().Add(-2 * time.Hour),
			Source:   "test",
		},
		{
			ID:       "syntax-2",
			Content:  "prefer small functions",
			Tags:     []string{"go", "style"},
			Category: "syntax",
			Scope:    "project",
			Score:    1.0,
			Created:  time.Now().Add(-1 * time.Hour),
			Source:   "test",
		},
		{
			ID:       "avoid-1",
			Content:  "avoid command injection from untrusted strings",
			Tags:     []string{"security", "shell"},
			Category: "avoidance",
			Scope:    "global",
			Score:    1.5,
			Created:  time.Now().Add(-3 * time.Hour),
			Source:   "test",
		},
	}

	for i := range entries {
		assert.NoError(t, s.Upsert(&entries[i]))
	}
}
