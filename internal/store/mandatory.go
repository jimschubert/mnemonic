package store

import (
	"slices"
	"sync"
)

var onceMandatory = sync.Once{}
var mandatory = []string{
	"avoidance",
	"security",
}

func IsMandatoryCategory(head string) bool {
	return slices.Contains(mandatory, head)
}

// WithAdditionalMandatoryCategories allows extending mandatory categories (heads).
//
//goland:noinspection GoUnusedExportedFunction
func WithAdditionalMandatoryCategories(categories []string) {
	onceMandatory.Do(func() {
		// only allow adding
		mandatory = append(mandatory, categories...)
	})
}
