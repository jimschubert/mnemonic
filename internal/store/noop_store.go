package store

type NoopStore struct{}

func (s *NoopStore) ListHeads(scopes []Scope) ([]HeadInfo, error) {
	return nil, nil
}

func (s *NoopStore) All(scopes []Scope) ([]Entry, error) {
	return nil, nil
}

func (s *NoopStore) Delete(id string) error {
	return nil
}

func (s *NoopStore) Get(id string) (*Entry, error) {
	return nil, nil
}

func (s *NoopStore) Query(category string, tags []string) ([]Entry, error) {
	return nil, nil
}

func (s *NoopStore) Upsert(entry *Entry) error {
	return nil
}

func (s *NoopStore) Score(id string, delta float64) error {
	return nil
}

func (s *NoopStore) AllByCategory(category string, topK int, scopes []Scope) ([]Entry, error) {
	return nil, nil
}

func (s *NoopStore) QueryByCategory(category, query string, topK int, scopes []Scope) ([]Entry, error) {
	return nil, nil
}

func (s *NoopStore) Promote(id string, targetScope Scope) error {
	return nil
}

var _ Store = (*NoopStore)(nil)
