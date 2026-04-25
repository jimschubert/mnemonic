package server

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/alecthomas/assert/v2"
	"github.com/jimschubert/mnemonic/internal/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type queryTestStore struct {
	semanticResults    []store.Entry
	semanticScopes     []store.Scope
	semanticCategories []string
	queryByCategory    map[string][]store.Entry
	allByCategory      map[string][]store.Entry
	allEntries         []store.Entry
}

func (s *queryTestStore) ListHeads(_ []store.Scope) ([]store.HeadInfo, error) { return nil, nil }

func (s *queryTestStore) All(_ []store.Scope) ([]store.Entry, error) {
	return s.allEntries, nil
}

func (s *queryTestStore) AllByCategory(category string, _ int, _ []store.Scope) ([]store.Entry, error) {
	return s.allByCategory[category], nil
}

func (s *queryTestStore) Get(id string) (*store.Entry, error) {
	for _, entry := range s.semanticResults {
		if entry.ID == id {
			entryCopy := entry
			return &entryCopy, nil
		}
	}
	for _, entries := range s.queryByCategory {
		for _, entry := range entries {
			if entry.ID == id {
				entryCopy := entry
				return &entryCopy, nil
			}
		}
	}
	return nil, nil
}

func (s *queryTestStore) Query(_ string, _ []string) ([]store.Entry, error) { return nil, nil }

func (s *queryTestStore) QueryByCategory(category, _ string, _ int, _ []store.Scope) ([]store.Entry, error) {
	return s.queryByCategory[category], nil
}

func (s *queryTestStore) Upsert(_ *store.Entry) error           { return nil }
func (s *queryTestStore) Score(_ string, _ float64) error       { return nil }
func (s *queryTestStore) Delete(_ string) error                 { return nil }
func (s *queryTestStore) Promote(_ string, _ store.Scope) error { return nil }
func (s *queryTestStore) SemanticSearch(_ string, _ int, scopes []store.Scope, categories []string) ([]store.Entry, error) {
	s.semanticScopes = scopes
	s.semanticCategories = categories
	return s.semanticResults, nil
}

func testServerLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestHandleQuery_MergesCategoriesAndBackfillsToOverallTopK(t *testing.T) {
	t.Parallel()
	now := time.Now()
	testStore := &queryTestStore{
		semanticResults: []store.Entry{
			{ID: "sem-security", Content: "semantic security hit", Category: "security", Score: 1, Created: now},
		},
		queryByCategory: map[string][]store.Entry{
			"security": {
				{ID: "sem-security", Content: "semantic security hit", Category: "security", Score: 1, Created: now},
				{ID: "kw-security", Content: "keyword security hit", Category: "security", Score: 2, Created: now},
			},
			"architecture": {
				{ID: "kw-architecture", Content: "keyword architecture hit", Category: "architecture", Score: 5, Created: now},
			},
		},
	}
	srv := &Server{store: testStore, logger: testServerLogger()}

	_, output, err := srv.handleQuery(t.Context(), &mcp.CallToolRequest{}, QueryInput{
		Query:      "workflow safety",
		Category:   "security",
		Categories: []string{"architecture", "security"},
		TopK:       2,
	})
	assert.NoError(t, err)
	assert.Equal(t, []string{"security", "architecture"}, testStore.semanticCategories)
	assert.Equal(t, 2, len(output.Entries))
	assert.Equal(t, "sem-security", output.Entries[0].ID)
	assert.Equal(t, "kw-architecture", output.Entries[1].ID)
}

func TestHandleQuery_CategoryMergesUseOverallTopK(t *testing.T) {
	t.Parallel()
	now := time.Now()
	testStore := &queryTestStore{
		allByCategory: map[string][]store.Entry{
			"security": {
				{ID: "security-low", Content: "security low", Category: "security", Score: 1, Created: now},
			},
			"architecture": {
				{ID: "architecture-high", Content: "architecture high", Category: "architecture", Score: 4, Created: now},
			},
		},
	}
	srv := &Server{store: testStore, logger: testServerLogger()}

	_, output, err := srv.handleQuery(t.Context(), &mcp.CallToolRequest{}, QueryInput{
		Categories: []string{"security", "architecture"},
		TopK:       1,
	})
	assert.NoError(t, err)
	assert.Equal(t, 1, len(output.Entries))
	assert.Equal(t, "architecture-high", output.Entries[0].ID)
}

func TestNormalizeCategories_RejectsUnknownCategory(t *testing.T) {
	t.Parallel()

	_, err := normalizeCategories("", []string{"wat"})
	assert.Error(t, err)
}
