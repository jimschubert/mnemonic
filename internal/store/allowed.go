package store

import (
	"maps"
	"slices"
)

var allowedCategories = map[string]bool{
	"avoidance":    true,
	"security":     true,
	"syntax":       true,
	"architecture": true,
	"domain":       true,
}

// IsAllowedCategory checks if a category is allowed (i.e. can be used as a head).
func IsAllowedCategory(head string) bool {
	return allowedCategories[head]
}

// AllowedCategories returns the list of allowed categories (heads).
func AllowedCategories() []string {
	return slices.Collect(maps.Keys(allowedCategories))
}
