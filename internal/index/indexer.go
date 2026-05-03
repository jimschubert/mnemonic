package index

import (
	"io"

	_ "github.com/asg017/sqlite-vec-go-bindings/ncruces"
)

// Indexer defines an interface for managing and searching vector indices.
type Indexer interface {
	// InsertVector adds or updates a vector in the index.
	InsertVector(id string, vector []float32) error
	// DeleteVector removes a vector from the index by ID.
	DeleteVector(id string) error
	// Search returns the k nearest neighbors to the target vector.
	Search(target []float32, k int) ([]SearchResult, error)
	// LookupVector returns the stored vector for the given ID, if present.
	LookupVector(id string) ([]float32, bool)
}

// Exporter is an optional interface for indexes that support binary serialization.
type Exporter interface {
	Export(w io.Writer) error
}

// SearchResult contains the ID of a matching vector and its distance to the query target.
type SearchResult struct {
	ID       string
	Distance float32
}
