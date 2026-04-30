package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alecthomas/assert/v2"
)

func TestNewSnapshotTempDir(t *testing.T) {
	tests := []struct {
		name string
	}{
		{
			name: "creates and removes a temporary lint index directory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir, cleanup, err := newSnapshotTempDir("mnemonic-lint-")
			assert.NoError(t, err)
			assert.True(t, strings.HasPrefix(filepath.Base(dir), "mnemonic-lint-"))

			info, statErr := os.Stat(dir)
			assert.NoError(t, statErr)
			assert.True(t, info.IsDir())

			cleanup()

			_, statErr = os.Stat(dir)
			assert.Error(t, statErr)
			assert.True(t, os.IsNotExist(statErr))
		})
	}
}
