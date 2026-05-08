package sqlitevec

import (
	"math"
	"path/filepath"
	"sort"
	"testing"

	"github.com/alecthomas/assert/v2"
)

// cosineDist computes 1 - cosine_similarity for two equal-length vectors.
func cosineDist(a, b []float32) float32 {
	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}
	return float32(1.0 - dot/(math.Sqrt(normA)*math.Sqrt(normB)))
}

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
			name: "update existing id",
			fn: func(t *testing.T) {
				assert.NoError(t, idx.InsertVector("a", []float32{0.0, 0.0, 1.0}))

				vec, ok := idx.LookupVector("a")
				assert.True(t, ok)
				assert.Equal(t, []float32{0.0, 0.0, 1.0}, vec)

				res, err := idx.Search([]float32{0.0, 0.0, 1.0}, 1)
				assert.NoError(t, err)
				assert.Equal(t, 1, len(res))
				assert.Equal(t, "a", res[0].ID)
				assert.True(t, math.Abs(float64(res[0].Distance)) < 1e-6, "distance should be almost zero for identical vectors")
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
				assert.NoError(t, idx.InsertVector("a", []float32{1.0, 0.0, 0.0}))

				res, err := idx.Search([]float32{1.0, 0.0, 0.0}, 1)
				assert.NoError(t, err)
				assert.Equal(t, 1, len(res))
				assert.Equal(t, "a", res[0].ID)
				assert.True(t, math.Abs(float64(res[0].Distance)) < 1e-6, "distance should be almost zero for identical vectors")

				res, err = idx.Search([]float32{1.0, 0.1, 0.0}, 1)
				assert.NoError(t, err)
				assert.Equal(t, 1, len(res))
				assert.Equal(t, "a", res[0].ID)
				expectedDistance := cosineDist([]float32{1.0, 0.0, 0.0}, []float32{1.0, 0.1, 0.0})
				assert.True(t, math.Abs(float64(res[0].Distance-expectedDistance)) < 1e-6, "distance should match cosine distance within floating point precision")

				res, err = idx.Search([]float32{0.1, 1.0, 0.0}, 2)
				assert.NoError(t, err)
				assert.Equal(t, 2, len(res))
				assert.Equal(t, "b", res[0].ID)
				assert.Equal(t, "a", res[1].ID)
				assert.True(t, res[0].Distance < res[1].Distance)
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
