package sqlitevec

import (
	"path/filepath"
	"sort"
	"testing"

	"github.com/alecthomas/assert/v2"
)

func TestSqliteVecIndex(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "index.db")

	idx, err := New(dbPath, 3)
	assert.NoError(t, err)
	t.Cleanup(func() {
		assert.NoError(t, idx.Close())
	})

	tests := []struct {
		name string
		fn   func(t *testing.T)
	}{
		{
			name: "insert",
			fn: func(t *testing.T) {
				assert.NoError(t, idx.InsertVector("a", []float32{1.0, 0.0, 0.0}))
				assert.NoError(t, idx.InsertVector("b", []float32{0.0, 1.0, 0.0}))
			},
		},
		{
			name: "lookup",
			fn: func(t *testing.T) {
				vec, ok := idx.LookupVector("a")
				assert.True(t, ok)
				assert.Equal(t, []float32{1.0, 0.0, 0.0}, vec)

				_, ok = idx.LookupVector("c")
				assert.False(t, ok)
			},
		},
		{
			name: "list ids",
			fn: func(t *testing.T) {
				ids, err := idx.ListIDs()
				assert.NoError(t, err)
				sort.Strings(ids)
				assert.Equal(t, []string{"a", "b"}, ids)
			},
		},
		{
			name: "search",
			fn: func(t *testing.T) {
				res, err := idx.Search([]float32{1.0, 0.1, 0.0}, 1)
				assert.NoError(t, err)
				assert.Equal(t, 1, len(res))
				assert.Equal(t, "a", res[0].ID)

				res, err = idx.Search([]float32{0.1, 1.0, 0.0}, 2)
				assert.NoError(t, err)
				assert.Equal(t, 2, len(res))
				assert.Equal(t, "b", res[0].ID)
				assert.Equal(t, "a", res[1].ID)
			},
		},
		{
			name: "delete",
			fn: func(t *testing.T) {
				assert.NoError(t, idx.DeleteVector("a"))

				_, ok := idx.LookupVector("a")
				assert.False(t, ok)

				assert.Error(t, idx.DeleteVector("a"))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.fn)
	}
}
