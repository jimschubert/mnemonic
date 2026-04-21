package hnsw

import (
	"bufio"
	"fmt"
	"io"
	"math/rand"
	"time"

	"github.com/coder/hnsw"
	"github.com/jimschubert/mnemonic/internal/config"
	"github.com/jimschubert/mnemonic/internal/index"
)

var _ index.Indexer = (*Index)(nil)

// Index wraps the coder/hnsw graph implementing the index.Indexer interface.
type Index struct {
	graph      *hnsw.Graph[string]
	dimensions int
	m          int
	efSearch   int
}

// New creates a new, empty HNSW index.
func New(conf config.Config) *Index {
	g := hnsw.NewGraph[string]()
	g.Distance = hnsw.CosineDistance
	g.M = conf.Index.Connections
	g.Ml = conf.Index.LevelFactor
	g.EfSearch = conf.Index.EfSearch
	g.Rng = rand.New(rand.NewSource(time.Now().UnixNano()))

	return &Index{
		graph:      g,
		dimensions: conf.Index.Dimensions,
		m:          conf.Index.Connections,
		efSearch:   conf.Index.EfSearch,
	}
}

// Export writes the graph to w using hnsw's binary encoding.
func (idx *Index) Export(w io.Writer) error {
	bw := bufio.NewWriter(w)
	if err := idx.graph.Export(bw); err != nil {
		return fmt.Errorf("exporting hnsw graph: %w", err)
	}
	return bw.Flush()
}

// Import replaces the graph contents with data from r.
// Various parameters need to match between cfg and the graph, if any mismatch then an error is returned.
func (idx *Index) Import(conf config.Config, r io.Reader) error {
	if err := idx.graph.Import(bufio.NewReader(r)); err != nil {
		return fmt.Errorf("importing hnsw graph: %w", err)
	}

	// only validate dimensions on a non-empty graph
	if dims := idx.graph.Dims(); dims != 0 && dims != conf.Index.Dimensions {
		return fmt.Errorf("dimension mismatch: graph has %d, config has %d. rebuild required", dims, conf.Index.Dimensions)
	}

	if idx.graph.M != conf.Index.Connections {
		return fmt.Errorf("connections (M) mismatch: graph has %d, config has %d. rebuild required", idx.graph.M, conf.Index.Connections)
	}

	if idx.graph.Ml != conf.Index.LevelFactor {
		return fmt.Errorf("level factor (Ml) mismatch: graph has %g, config has %g. rebuild required", idx.graph.Ml, conf.Index.LevelFactor)
	}

	idx.graph.Distance = hnsw.CosineDistance
	idx.graph.EfSearch = conf.Index.EfSearch
	idx.dimensions = conf.Index.Dimensions
	idx.m = conf.Index.Connections
	idx.efSearch = conf.Index.EfSearch
	return nil
}

// InsertVector adds a new vector to the graph. If the ID exists, it is replaced.
func (idx *Index) InsertVector(id string, vector []float32) error {
	if len(vector) != idx.dimensions {
		return fmt.Errorf("vector dimension mismatch: expected %d, got %d. you will need to use the expected model or set expected index", idx.dimensions, len(vector))
	}

	node := hnsw.MakeNode(id, vector)
	idx.graph.Add(node)

	return nil
}

// DeleteVector removes a node from the graph.
func (idx *Index) DeleteVector(id string) error {
	deleted := idx.graph.Delete(id)
	if !deleted {
		return fmt.Errorf("node %q not found in index", id)
	}
	return nil
}

// Search finds the k nearest neighbors.
func (idx *Index) Search(target []float32, k int) ([]index.SearchResult, error) {
	if len(target) != idx.dimensions {
		return nil, fmt.Errorf("target dimension mismatch: expected %d, got %d", idx.dimensions, len(target))
	}

	results := idx.graph.SearchWithDistance(target, k)

	out := make([]index.SearchResult, len(results))
	for i, res := range results {
		out[i] = index.SearchResult{
			ID:       res.Key,
			Distance: res.Distance,
		}
	}

	return out, nil
}

// LookupVector returns the id's vector, plus bool indicating if it was found.
func (idx *Index) LookupVector(id string) ([]float32, bool) {
	return idx.graph.Lookup(id)
}
