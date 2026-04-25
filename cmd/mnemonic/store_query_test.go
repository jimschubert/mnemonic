package main

import (
	"testing"

	"github.com/alecthomas/assert/v2"
)

func TestParseCSV(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "empty input",
			input: "",
			want:  nil,
		},
		{
			name:  "single category",
			input: "security",
			want: []string{
				"security",
			},
		},
		{
			name:  "csv categories are trimmed and deduped",
			input: "security, architecture,security",
			want: []string{
				"security",
				"architecture",
			},
		},
		{
			name:  "trimming and deduping also works with spaces",
			input: "security, architecture, security \t, architecture \n",
			want: []string{
				"security",
				"architecture",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, parseCSV(tt.input))
		})
	}
}
