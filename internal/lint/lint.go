package lint

import (
	"github.com/jimschubert/mnemonic/internal/controller"
	"github.com/jimschubert/mnemonic/internal/store"
)

// ActionType defines the suggested operation for a lint issue.
type ActionType string

const (
	ActionMerge  ActionType = "MERGE"
	ActionDelete ActionType = "DELETE"

	nearDuplicateThreshold = 0.98
)

// Action represents a proposed change to the memory store.
type Action struct {
	Type       ActionType
	Left       store.Entry
	Right      store.Entry
	Similarity float64
}

// Linter analyzes the memory store for health issues.
type Linter struct {
	controller controllerInterface
}

// controllerInterface defines the method set required for linter, without creating a any circular dependencies with controller package.
// This also allows for easier mock testing.
type controllerInterface interface {
	All(scopes []store.Scope) ([]store.Entry, error)
	FindSimilar(id string, threshold float64) ([]controller.SimilarEntry, error)
}

// New creates a new Linter.
func New(c controllerInterface) *Linter {
	return &Linter{controller: c}
}

// Analyze scans all entries and identifies redundant or overlapping memories.
func (l *Linter) Analyze(threshold float64) ([]Action, error) {
	entries, err := l.controller.All(nil)
	if err != nil {
		return nil, err
	}

	var actions []Action
	seen := make(map[string]bool)

	for _, currentEntry := range entries {
		if seen[currentEntry.ID] {
			continue
		}

		similarEntries, err := l.controller.FindSimilar(currentEntry.ID, threshold)
		if err != nil {
			// ignore anything that errors
			continue
		}

		for _, similar := range similarEntries {
			if seen[similar.ID] {
				continue
			}

			similarity := float64(1.0 - similar.Distance)

			actionType := ActionMerge
			if similarity >= nearDuplicateThreshold && !hasUniqueTags(similar.Entry, currentEntry) {
				// extremely similar with same tags -> recommend delete
				actionType = ActionDelete
			}

			actions = append(actions, Action{
				Type:       actionType,
				Left:       currentEntry,
				Right:      similar.Entry,
				Similarity: similarity,
			})

			seen[similar.ID] = true
		}

		seen[currentEntry.ID] = true
	}

	return actions, nil
}

func hasUniqueTags(first store.Entry, second store.Entry) bool {
	tags := make(map[string]bool, len(second.Tags))
	for _, t := range second.Tags {
		tags[t] = true
	}
	for _, t := range first.Tags {
		if !tags[t] {
			return true
		}
	}
	return false
}
