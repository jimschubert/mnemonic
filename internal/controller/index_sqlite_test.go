package controller

import (
	"path/filepath"
	"testing"

	"github.com/alecthomas/assert/v2"
	"github.com/jimschubert/mnemonic/internal/store"
)

func TestSqliteManagerBuildIndexes_ForceRemovesStaleVectors(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	conf := testConfig()
	conf.Index.Type = "sqlite"

	mgr, err := newSqliteManager(filepath.Join(dir, "index.db"), conf, testLogger())
	assert.NoError(t, err)
	t.Cleanup(func() {
		assert.NoError(t, mgr.Close())
	})

	assert.NoError(t, mgr.InsertVector("live", []float32{1, 0, 0, 0}))
	assert.NoError(t, mgr.InsertVector("stale", []float32{0, 1, 0, 0}))

	embedCalls := 0
	entries := []store.Entry{{ID: "live", Content: "alive"}}
	err = mgr.BuildIndexes(entries, true, func(texts []string) ([][]float32, error) {
		embedCalls++
		return [][]float32{{1, 0, 0, 0}}, nil
	})
	assert.NoError(t, err)
	assert.Equal(t, 1, embedCalls)

	_, ok := mgr.LookupVector("live")
	assert.True(t, ok, "live entry should remain indexed")
	_, ok = mgr.LookupVector("stale")
	assert.False(t, ok, "stale entry should be removed during rebuild")
}
