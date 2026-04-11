package store

// Store defines the persistent storage contract (YAML, Chroma, etc.).
type Store interface {
	Get(id string) (*Entry, error)
	Query(category string, tags []string) ([]*Entry, error)
	Upsert(entry *Entry) error
	Score(id string, delta float64) error
}

type NoopStore struct{}

func (s *NoopStore) Get(id string) (*Entry, error) {
	return nil, nil
}

func (s *NoopStore) Query(category string, tags []string) ([]*Entry, error) {
	return nil, nil
}

func (s *NoopStore) Upsert(entry *Entry) error {
	return nil
}

func (s *NoopStore) Score(id string, delta float64) error {
	return nil
}
