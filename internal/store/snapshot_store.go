package store

import (
	"fmt"
	"slices"
	"strings"
	"time"
)

// SnapshotStore is an in-memory implementation of Store.
// It is used when a daemon is running to cache all entries fetched from the daemon.
// This allows local components (like the lint command) to wrap it in a MemoryController
// and build ephemeral, client-side vector indexes for fast local analysis,
// without interfering with the daemon's state or file locks.
type SnapshotStore struct {
	entries map[string]*Entry
}

var _ Store = (*SnapshotStore)(nil)

func NewSnapshotStore(entries []Entry) *SnapshotStore {
	out := make(map[string]*Entry, len(entries))
	for _, entry := range entries {
		copyEntry := entry
		out[entry.ID] = &copyEntry
	}
	return &SnapshotStore{entries: out}
}

func (s *SnapshotStore) ListHeads(scopes []Scope) ([]HeadInfo, error) {
	counts := map[string]int{}
	for _, entry := range s.entriesForScopes(scopes) {
		counts[entry.Category]++
	}
	heads := make([]HeadInfo, 0, len(counts))
	for name, count := range counts {
		heads = append(heads, HeadInfo{Name: name, Count: count, Mandatory: IsMandatoryCategory(name)})
	}
	slices.SortStableFunc(heads, func(i, j HeadInfo) int { return strings.Compare(i.Name, j.Name) })
	return heads, nil
}

func (s *SnapshotStore) All(scopes []Scope) ([]Entry, error) {
	entries := s.entriesForScopes(scopes)
	SortByWeightedScore(entries)
	return entries, nil
}

func (s *SnapshotStore) AllByCategory(category string, topK int, scopes []Scope) ([]Entry, error) {
	var out []Entry
	for _, entry := range s.entriesForScopes(scopes) {
		if entry.Category != category {
			continue
		}
		out = append(out, entry)
	}
	SortByWeightedScore(out)
	if topK > 0 && len(out) > topK {
		out = out[:topK]
	}
	return out, nil
}

func (s *SnapshotStore) Get(id string) (*Entry, error) {
	entry, ok := s.entries[id]
	if !ok {
		return nil, fmt.Errorf("entry %q not found", id)
	}
	copyEntry := *entry
	return &copyEntry, nil
}

func (s *SnapshotStore) Query(category string, tags []string) ([]Entry, error) {
	var out []Entry
	for _, entry := range s.entries {
		if category != "" && entry.Category != category {
			continue
		}
		if !hasAllTags(entry.Tags, tags) {
			continue
		}
		out = append(out, *entry)
	}
	SortByWeightedScore(out)
	return out, nil
}

func (s *SnapshotStore) QueryByCategory(category, query string, topK int, scopes []Scope) ([]Entry, error) {
	terms := strings.Fields(strings.ToLower(query))
	var out []Entry
	for _, entry := range s.entriesForScopes(scopes) {
		if entry.Category != category {
			continue
		}
		if !keywordMatchSnapshot(entry, terms) {
			continue
		}
		out = append(out, entry)
	}
	SortByWeightedScore(out)
	if topK > 0 && len(out) > topK {
		out = out[:topK]
	}
	return out, nil
}

func (s *SnapshotStore) Upsert(entry *Entry) error {
	if entry == nil {
		return fmt.Errorf("entry is required")
	}
	copyEntry := *entry
	s.entries[entry.ID] = &copyEntry
	return nil
}

func (s *SnapshotStore) Score(id string, delta float64) error {
	entry, ok := s.entries[id]
	if !ok {
		return fmt.Errorf("entry %q not found", id)
	}
	entry.Score += delta
	if entry.Score < 0 {
		entry.Score = 0
	}
	entry.LastHit = time.Now()
	return nil
}

func (s *SnapshotStore) Delete(id string) error {
	delete(s.entries, id)
	return nil
}

func (s *SnapshotStore) Promote(id string, targetScope Scope) error {
	entry, ok := s.entries[id]
	if !ok {
		return fmt.Errorf("entry %q not found", id)
	}
	entry.Scope = targetScope.String()
	return nil
}

func (s *SnapshotStore) entriesForScopes(scopes []Scope) []Entry {
	if len(scopes) == 0 {
		out := make([]Entry, 0, len(s.entries))
		for _, entry := range s.entries {
			out = append(out, *entry)
		}
		return out
	}

	allowed := make(map[string]struct{}, len(scopes))
	for _, scope := range scopes {
		allowed[scope.String()] = struct{}{}
	}

	out := make([]Entry, 0, len(s.entries))
	for _, entry := range s.entries {
		if _, ok := allowed[entry.Scope]; ok {
			out = append(out, *entry)
		}
	}
	return out
}

func hasAllTags(entryTags []string, required []string) bool {
	if len(required) == 0 {
		return true
	}
	set := make(map[string]struct{}, len(entryTags))
	for _, tag := range entryTags {
		set[strings.ToLower(tag)] = struct{}{}
	}
	for _, requiredTag := range required {
		if _, ok := set[strings.ToLower(requiredTag)]; !ok {
			return false
		}
	}
	return true
}

func keywordMatchSnapshot(e Entry, terms []string) bool {
	if len(terms) == 0 {
		return true
	}
	content := strings.ToLower(e.Content)
	for _, term := range terms {
		if strings.Contains(content, term) {
			return true
		}
	}
	for _, tag := range e.Tags {
		tag = strings.ToLower(tag)
		for _, term := range terms {
			if strings.Contains(tag, term) {
				return true
			}
		}
	}
	return false
}
