package store

import (
	"sync"
)

var onceMandatory = sync.Once{}
var mandatory = []string{
	"avoidance",
	"security",
}

func IsMandatoryCategory(head string) bool {
	for _, m := range mandatory {
		if head == m {
			return true
		}
	}
	return false
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
