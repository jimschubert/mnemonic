package controller

import (
	"encoding/json"
	"os"
	"time"
)

// IndexMetadata tracks which entry IDs have been indexed.
type IndexMetadata struct {
	// Entries maps entry ID to the time it was indexed.
	Entries map[string]time.Time `json:"entries"`
}

func newMetadata() *IndexMetadata {
	return &IndexMetadata{Entries: make(map[string]time.Time)}
}

func loadMetadata(path string) (*IndexMetadata, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return newMetadata(), nil
		}
		return nil, err
	}
	m := newMetadata()
	if err := json.Unmarshal(b, m); err != nil {
		return nil, err
	}
	return m, nil
}

func (m *IndexMetadata) save(path string) error {
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

func (m *IndexMetadata) has(id string) bool {
	_, ok := m.Entries[id]
	return ok
}

func (m *IndexMetadata) add(id string) {
	m.Entries[id] = time.Now()
}

func (m *IndexMetadata) remove(id string) {
	delete(m.Entries, id)
}
