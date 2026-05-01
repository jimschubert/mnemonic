package main

import (
	"testing"

	"github.com/alecthomas/assert/v2"
	"github.com/jimschubert/mnemonic/internal/store"
)

func TestParseScopeFilters(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   []string
		want    []store.Scope
		wantErr bool
	}{
		{
			name:  "empty input returns nil",
			input: nil,
			want:  nil,
		},
		{
			name: "single valid scope",
			input: []string{
				"project",
			},
			want: []store.Scope{
				"project",
			},
		},
		{
			name: "invalid scope returns an error",
			input: []string{
				"workspace",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseScopeFilters(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestFilterEntriesByCategory(t *testing.T) {
	t.Parallel()

	entries := []store.Entry{
		{
			ID:       "a",
			Category: "security",
		},
		{
			ID:       "b",
			Category: "domain",
		},
		{
			ID:       "c",
			Category: "architecture",
		},
	}

	tests := []struct {
		name       string
		categories []string
		wantIDs    []string
	}{
		{
			name:       "no category filters returns all entries",
			categories: nil,
			wantIDs: []string{
				"a",
				"b",
				"c",
			},
		},
		{
			name: "single category",
			categories: []string{
				"security",
			},
			wantIDs: []string{
				"a",
			},
		},
		{
			name: "multiple categories with case differences",
			categories: []string{
				"DOMAIN",
				"architecture",
			},
			wantIDs: []string{
				"b",
				"c",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filtered := filterEntriesByCategory(entries, tt.categories)
			ids := make([]string, 0, len(filtered))
			for _, entry := range filtered {
				ids = append(ids, entry.ID)
			}
			assert.Equal(t, tt.wantIDs, ids)
		})
	}
}
