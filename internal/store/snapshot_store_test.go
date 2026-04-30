package store

import (
	"testing"
	"time"

	"github.com/alecthomas/assert/v2"
)

func TestSnapshotStore(t *testing.T) {
	now := time.Now()
	entries := []Entry{
		{
			ID:       "1",
			Content:  "content one",
			Category: "domain",
			Scope:    "global",
			Tags:     []string{"go", "test"},
			Score:    10.0,
			LastHit:  now,
		},
		{
			ID:       "2",
			Content:  "content two",
			Category: "syntax",
			Scope:    "project",
			Tags:     []string{"go"},
			Score:    5.0,
			LastHit:  now.Add(-24 * time.Hour),
		},
	}

	t.Run("Get existing", func(t *testing.T) {
		s := NewSnapshotStore(entries)
		entry, err := s.Get("1")
		assert.NoError(t, err)
		assert.Equal(t, "1", entry.ID)
		assert.Equal(t, "content one", entry.Content)
	})

	t.Run("Get missing", func(t *testing.T) {
		s := NewSnapshotStore(entries)
		_, err := s.Get("3")
		assert.Error(t, err)
	})

	t.Run("All without scopes", func(t *testing.T) {
		s := NewSnapshotStore(entries)
		all, err := s.All(nil)
		assert.NoError(t, err)
		assert.Equal(t, 2, len(all))
	})

	t.Run("All with scopes", func(t *testing.T) {
		s := NewSnapshotStore(entries)
		all, err := s.All([]Scope{Scope("project")})
		assert.NoError(t, err)
		assert.Equal(t, 1, len(all))
		assert.Equal(t, "2", all[0].ID)
	})

	t.Run("AllByCategory", func(t *testing.T) {
		s := NewSnapshotStore(entries)
		res, err := s.AllByCategory("domain", 10, nil)
		assert.NoError(t, err)
		assert.Equal(t, 1, len(res))
		assert.Equal(t, "1", res[0].ID)
	})

	t.Run("Query by tags", func(t *testing.T) {
		s := NewSnapshotStore(entries)
		res, err := s.Query("", []string{"test"})
		assert.NoError(t, err)
		assert.Equal(t, 1, len(res))
		assert.Equal(t, "1", res[0].ID)
	})

	t.Run("QueryByCategory keyword search", func(t *testing.T) {
		s := NewSnapshotStore(entries)
		res, err := s.QueryByCategory("domain", "one", 10, nil)
		assert.NoError(t, err)
		assert.Equal(t, 1, len(res))
		assert.Equal(t, "1", res[0].ID)

		res, err = s.QueryByCategory("domain", "missing", 10, nil)
		assert.NoError(t, err)
		assert.Equal(t, 0, len(res))
	})

	t.Run("Upsert", func(t *testing.T) {
		s := NewSnapshotStore(nil)
		err := s.Upsert(&Entry{ID: "3", Content: "new"})
		assert.NoError(t, err)
		entry, err := s.Get("3")
		assert.NoError(t, err)
		assert.Equal(t, "new", entry.Content)
	})

	t.Run("Delete", func(t *testing.T) {
		s := NewSnapshotStore(entries)
		err := s.Delete("1")
		assert.NoError(t, err)
		_, err = s.Get("1")
		assert.Error(t, err)
	})

	t.Run("Score", func(t *testing.T) {
		s := NewSnapshotStore(entries)
		err := s.Score("1", 5.0)
		assert.NoError(t, err)
		entry, _ := s.Get("1")
		assert.Equal(t, 15.0, entry.Score)
	})

	t.Run("ListHeads", func(t *testing.T) {
		s := NewSnapshotStore(entries)
		heads, err := s.ListHeads(nil)
		assert.NoError(t, err)
		assert.Equal(t, 2, len(heads))
	})
}
