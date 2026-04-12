package store

import (
	"testing"

	"github.com/alecthomas/assert/v2"
)

func TestIsMandatoryCategory(t *testing.T) {
	tests := []struct {
		name     string
		category string
		expected bool
	}{
		{
			name:     "avoidance is mandatory",
			category: "avoidance",
			expected: true,
		},
		{
			name:     "security is mandatory",
			category: "security",
			expected: true,
		},
		{
			name:     "syntax is not mandatory",
			category: "syntax",
			expected: false,
		},
		{
			name:     "architecture is not mandatory",
			category: "architecture",
			expected: false,
		},
		{
			name:     "domain is not mandatory",
			category: "domain",
			expected: false,
		},
		{
			name:     "unknown category is not mandatory",
			category: "unknown",
			expected: false,
		},
		{
			name:     "empty string is not mandatory",
			category: "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsMandatoryCategory(tt.category)
			assert.Equal(t, tt.expected, result)
		})
	}
}
