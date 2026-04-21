package lint

import (
	"testing"

	"github.com/alecthomas/assert/v2"
	"github.com/jimschubert/mnemonic/internal/controller"
	"github.com/jimschubert/mnemonic/internal/store"
)

type mockController struct {
	entries []store.Entry
	similar map[string][]controller.SimilarEntry
}

func (m *mockController) All(scopes []store.Scope) ([]store.Entry, error) {
	return m.entries, nil
}

func (m *mockController) FindSimilar(id string, threshold float64) ([]controller.SimilarEntry, error) {
	return m.similar[id], nil
}

func TestLinter_Analyze(t *testing.T) {
	tests := []struct {
		name      string
		entries   []store.Entry
		similar   map[string][]controller.SimilarEntry
		threshold float64
		want      []Action
	}{
		{
			name: "chooses merge for near duplicates above the threshold",
			entries: []store.Entry{
				{ID: "1", Content: "Always use slog"},
				{ID: "2", Content: "Prefer slog for logging"},
			},
			similar: map[string][]controller.SimilarEntry{
				"1": {
					{Entry: store.Entry{ID: "2", Content: "Prefer slog for logging"}, Distance: 0.05},
				},
			},
			threshold: 0.90,
			want: []Action{
				{
					Type:       ActionMerge,
					Left:       store.Entry{ID: "1", Content: "Always use slog"},
					Right:      store.Entry{ID: "2", Content: "Prefer slog for logging"},
					Similarity: float64(float32(1.0) - float32(0.05)),
				},
			},
		},
		{
			name: "near-duplicate with no unique tags suggests delete",
			entries: []store.Entry{
				{ID: "1", Content: "Always use slog", Tags: []string{"go", "logging"}},
				{ID: "2", Content: "Always use slog for logging", Tags: []string{"go"}},
			},
			similar: map[string][]controller.SimilarEntry{
				"1": {
					// distance 0.01 -> similarity 0.99
					{
						Entry:    store.Entry{ID: "2", Content: "Always use slog for logging", Tags: []string{"go"}},
						Distance: 0.01,
					},
				},
			},
			threshold: 0.90,
			want: []Action{
				{
					Type:       ActionDelete,
					Left:       store.Entry{ID: "1", Content: "Always use slog", Tags: []string{"go", "logging"}},
					Right:      store.Entry{ID: "2", Content: "Always use slog for logging", Tags: []string{"go"}},
					Similarity: float64(float32(1.0) - float32(0.01)),
				},
			},
		},
		{
			name: "near-duplicate with unique tag on other entry suggests merge",
			entries: []store.Entry{
				{ID: "1", Content: "Always use slog", Tags: []string{"go"}},
				{ID: "2", Content: "Always use slog for logging", Tags: []string{"go", "slog"}},
			},
			similar: map[string][]controller.SimilarEntry{
				"1": {
					// distance 0.01 -> similarity 0.99, but extra tag
					{
						Entry: store.Entry{
							ID: "2", Content: "Always use slog for logging", Tags: []string{"go", "slog"},
						}, Distance: 0.01,
					},
				},
			},
			threshold: 0.90,
			want: []Action{
				{
					Type: ActionMerge,
					Left: store.Entry{ID: "1", Content: "Always use slog", Tags: []string{"go"}},
					Right: store.Entry{
						ID: "2", Content: "Always use slog for logging", Tags: []string{"go", "slog"},
					},
					Similarity: float64(float32(1.0) - float32(0.01)),
				},
			},
		},
		{
			name: "no action for dissimilar entries",
			entries: []store.Entry{
				{ID: "1", Content: "Always use slog"},
				{ID: "3", Content: "Use HTTPS for everything"},
			},
			similar:   map[string][]controller.SimilarEntry{},
			threshold: 0.90,
			want:      nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := New(&mockController{entries: tt.entries, similar: tt.similar})
			got, err := l.Analyze(tt.threshold)
			assert.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
