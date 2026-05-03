package sqlitestore

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/alecthomas/assert/v2"
	"github.com/jimschubert/mnemonic/internal/store"
)

func newTestStore(t *testing.T, opts ...Option) *SQLiteStore {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "store.db")
	s, err := New(dbPath, nil, opts...)
	assert.NoError(t, err)
	t.Cleanup(func() {
		assert.NoError(t, s.Close())
	})
	return s
}

func TestUpsertAndGet(t *testing.T) {
	s := newTestStore(t)

	tests := []struct {
		name      string
		entry     *store.Entry
		wantError bool
	}{
		{
			name: "allowed category",
			entry: &store.Entry{
				ID:       "syntax-1",
				Content:  "use wrapped errors",
				Tags:     []string{"go", "errors"},
				Category: "syntax",
				Scope:    "global",
				Score:    0.5,
				Created:  time.Now().Add(-1 * time.Hour),
				Source:   "test",
			},
			wantError: false,
		},
		{
			name: "disallowed category",
			entry: &store.Entry{
				ID:       "invalid-1",
				Content:  "bad category",
				Category: "invalid",
				Scope:    "global",
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := s.Upsert(tt.entry)
			if tt.wantError {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			got, getErr := s.Get(tt.entry.ID)
			assert.NoError(t, getErr)
			assert.Equal(t, tt.entry.ID, got.ID)
			assert.Equal(t, tt.entry.Category, got.Category)
		})
	}

	t.Run("defaults id score created and scope", func(t *testing.T) {
		entry := &store.Entry{
			Content:  "defaults",
			Category: "domain",
		}
		err := s.Upsert(entry)
		assert.NoError(t, err)
		assert.NotEqual(t, "", entry.ID)
		assert.Equal(t, 1.0, entry.Score)
		assert.Equal(t, "global", entry.Scope)
		assert.False(t, entry.Created.After(time.Now()))
	})
}

func TestQueryAndHitCounting(t *testing.T) {
	s := newTestStore(t)
	base := time.Now().Add(-24 * time.Hour)

	entries := []store.Entry{
		{
			ID:       "domain-high",
			Content:  "high score go error tip",
			Tags:     []string{"go", "errors"},
			Category: "domain",
			Scope:    "global",
			Score:    10.0,
			Created:  base,
			Source:   "test",
		},
		{
			ID:       "domain-low",
			Content:  "low score logging tip",
			Tags:     []string{"logging"},
			Category: "domain",
			Scope:    "project",
			Score:    1.0,
			Created:  base,
			Source:   "test",
		},
		{
			ID:       "syntax-1",
			Content:  "language syntax",
			Tags:     []string{"go"},
			Category: "syntax",
			Scope:    "global",
			Score:    5.0,
			Created:  base,
			Source:   "test",
		},
	}

	for i := range entries {
		err := s.Upsert(&entries[i])
		assert.NoError(t, err)
	}

	headInfos, err := s.ListHeads(nil)
	assert.NoError(t, err)
	assert.Equal(t, 2, len(headInfos))

	all, err := s.All([]store.Scope{"global"})
	assert.NoError(t, err)
	assert.Equal(t, 2, len(all))
	assert.Equal(t, "domain-high", all[0].ID)

	tagHits, err := s.Query("", []string{"GO"})
	assert.NoError(t, err)
	assert.Equal(t, 2, len(tagHits))

	allByCategory, err := s.AllByCategory("domain", 1, nil)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(allByCategory))
	assert.Equal(t, "domain-high", allByCategory[0].ID)

	queryByCategory, err := s.QueryByCategory("domain", "logging", 0, []store.Scope{"project"})
	assert.NoError(t, err)
	assert.Equal(t, 1, len(queryByCategory))
	assert.Equal(t, "domain-low", queryByCategory[0].ID)

	hitCountAfterQuery, err := s.Get("domain-high")
	assert.NoError(t, err)
	assert.True(t, hitCountAfterQuery.HitCount > 0)
	assert.False(t, hitCountAfterQuery.LastHit.IsZero())
}

func TestAutoHitCountingDisabled(t *testing.T) {
	s := newTestStore(t, WithAutoHitCounting(false))

	entry := &store.Entry{
		ID:       "no-hit",
		Content:  "query should not increment",
		Tags:     []string{"go"},
		Category: "domain",
		Scope:    "global",
		Score:    1.0,
		Created:  time.Now(),
		Source:   "test",
	}
	assert.NoError(t, s.Upsert(entry))

	_, err := s.Query("domain", []string{"go"})
	assert.NoError(t, err)

	got, err := s.Get("no-hit")
	assert.NoError(t, err)
	assert.Equal(t, 0, got.HitCount)
	assert.True(t, got.LastHit.IsZero())
}

func TestScoreDeletePromoteAndPersistence(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "persist.db")
	s, err := New(dbPath, nil)
	assert.NoError(t, err)

	entry := &store.Entry{
		ID:       "persisted",
		Content:  "persist me",
		Category: "domain",
		Scope:    "global",
		Score:    1.0,
		Created:  time.Now().Add(-1 * time.Hour),
		Source:   "test",
	}
	assert.NoError(t, s.Upsert(entry))

	assert.NoError(t, s.Score("persisted", 0.2))
	assert.NoError(t, s.Promote("persisted", "project"))
	assert.NoError(t, s.Close())

	s2, err := New(dbPath, nil)
	assert.NoError(t, err)
	t.Cleanup(func() {
		assert.NoError(t, s2.Close())
	})

	got, err := s2.Get("persisted")
	assert.NoError(t, err)
	assert.Equal(t, "project", got.Scope)
	assert.Equal(t, 1.2, got.Score)
	assert.True(t, got.HitCount >= 1)

	assert.NoError(t, s2.Delete("persisted"))
	_, err = s2.Get("persisted")
	assert.Error(t, err)
}

func TestSortFallbackWhenSQLWeightedSortDisabled(t *testing.T) {
	s := newTestStore(t)
	s.sqlWeightedSort = false

	now := time.Now().Add(-24 * time.Hour)
	entries := []store.Entry{
		{
			ID:       "low",
			Content:  "low",
			Category: "domain",
			Scope:    "global",
			Score:    1.0,
			Created:  now,
			Source:   "test",
		},
		{
			ID:       "high",
			Content:  "high",
			Category: "domain",
			Scope:    "global",
			Score:    100.0,
			Created:  now,
			Source:   "test",
		},
	}

	for i := range entries {
		assert.NoError(t, s.Upsert(&entries[i]))
	}

	all, err := s.AllByCategory("domain", 0, nil)
	assert.NoError(t, err)
	assert.Equal(t, 2, len(all))
	assert.Equal(t, "high", all[0].ID)
	assert.Equal(t, "low", all[1].ID)
}
