package sqlitevec

import (
	"database/sql"
	"fmt"
	"math"

	sqlite_vec "github.com/asg017/sqlite-vec-go-bindings/ncruces"
	_ "github.com/ncruces/go-sqlite3/driver"

	"github.com/jimschubert/mnemonic/internal/index"
)

var _ index.Indexer = (*Index)(nil)

// Index implements index.Indexer using SQLite and sqlite-vec.
type Index struct {
	db         *sql.DB
	dimensions int
}

// New creates or opens a SQLite database and initializes the vector table.
func New(dbPath string, dimensions int) (*Index, error) {
	db, err := sql.Open("sqlite3", "file:"+dbPath+"?_pragma=journal_mode(wal)")
	if err != nil {
		return nil, fmt.Errorf("opening sqlite index database: %w", err)
	}

	schema := fmt.Sprintf(`
		CREATE VIRTUAL TABLE IF NOT EXISTS vec_index USING vec0(
			id TEXT PRIMARY KEY,
			embedding float[%d]
		);
	`, dimensions)

	if _, err := db.Exec(schema); err != nil {
		if closeErr := db.Close(); closeErr != nil {
			return nil, fmt.Errorf("closing sqlite index database after schema failure: %w", closeErr)
		}
		return nil, fmt.Errorf("creating vec_index table: %w", err)
	}

	return &Index{
		db:         db,
		dimensions: dimensions,
	}, nil
}

func (idx *Index) Close() error {
	return idx.db.Close()
}

func (idx *Index) InsertVector(id string, vector []float32) error {
	if len(vector) != idx.dimensions {
		return fmt.Errorf("vector dimension mismatch: expected %d, got %d", idx.dimensions, len(vector))
	}

	blob, err := sqlite_vec.SerializeFloat32(vector)
	if err != nil {
		return fmt.Errorf("serializing vector: %w", err)
	}

	query := `INSERT OR REPLACE INTO vec_index(id, embedding) VALUES (?, ?)`
	_, err = idx.db.Exec(query, id, blob)
	if err != nil {
		return fmt.Errorf("inserting vector: %w", err)
	}
	return nil
}

func (idx *Index) DeleteVector(id string) error {
	query := `DELETE FROM vec_index WHERE id = ?`
	res, err := idx.db.Exec(query, id)
	if err != nil {
		return fmt.Errorf("deleting vector: %w", err)
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return fmt.Errorf("node %q not found in index", id)
	}
	return nil
}

func (idx *Index) ListIDs() (ids []string, err error) {
	rows, err := idx.db.Query(`SELECT id FROM vec_index`)
	if err != nil {
		return nil, fmt.Errorf("listing vector ids: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("closing id rows: %w", closeErr)
		}
	}()

	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scanning vector id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating vector ids: %w", err)
	}

	return ids, nil
}

func (idx *Index) Search(target []float32, k int) (results []index.SearchResult, err error) {
	if len(target) != idx.dimensions {
		return nil, fmt.Errorf("target dimension mismatch: expected %d, got %d", idx.dimensions, len(target))
	}
	if k <= 0 {
		return nil, fmt.Errorf("k must be greater than 0, got %d", k)
	}

	blob, err := sqlite_vec.SerializeFloat32(target)
	if err != nil {
		return nil, fmt.Errorf("serializing target vector: %w", err)
	}

	query := `
		SELECT id, distance
		FROM vec_index
		WHERE embedding MATCH ?
		  AND k = ?
		ORDER BY distance
	`
	rows, err := idx.db.Query(query, blob, k)
	if err != nil {
		return nil, fmt.Errorf("searching index: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("closing search rows: %w", closeErr)
			results = nil
		}
	}()

	for rows.Next() {
		var res index.SearchResult
		if err := rows.Scan(&res.ID, &res.Distance); err != nil {
			return nil, fmt.Errorf("scanning search result: %w", err)
		}
		results = append(results, res)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating search results: %w", err)
	}

	return results, nil
}

func (idx *Index) LookupVector(id string) ([]float32, bool) {
	query := `SELECT embedding FROM vec_index WHERE id = ?`

	var blob []byte
	err := idx.db.QueryRow(query, id).Scan(&blob)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, false
		}
		return nil, false
	}

	vec := make([]float32, len(blob)/4)
	for i := range vec {
		importBinary := uint32(blob[i*4]) | uint32(blob[i*4+1])<<8 | uint32(blob[i*4+2])<<16 | uint32(blob[i*4+3])<<24
		vec[i] = math.Float32frombits(importBinary)
	}
	return vec, true
}
