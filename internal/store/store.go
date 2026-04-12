package store

// Scope represents a logical grouping for entries.
// Examples of scopes include "global", "team:acme", "project:mnemonic".
type Scope string

func (s Scope) String() string {
	return string(s)
}

const ScopeGlobal Scope = "global"

type HeadInfo struct {
	Name      string `json:"name" yaml:"name"`
	Count     int    `json:"count" yaml:"count"`
	Mandatory bool   `json:"mandatory" yaml:"mandatory"`
}

type QueryOptions struct {
}

// Store defines the persistent storage contract (YAML, Chroma, etc.).
type Store interface {
	ListHeads(scopes []Scope) ([]HeadInfo, error)
	All(scopes []Scope) ([]Entry, error)
	Get(id string) (*Entry, error)
	Query(category string, tags []string) ([]*Entry, error)
	Upsert(entry *Entry) error
	Score(id string, delta float64) error
	Delete(id string) error
}
