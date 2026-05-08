package sqlitevec

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/ncruces/go-sqlite3/driver"
	"github.com/ncruces/go-sqlite3/ext/vec1"

	"github.com/jimschubert/mnemonic/internal/index"
)

var _ index.Indexer = (*Index)(nil)

// Index implements index.Indexer using SQLite vec1 extension.
type Index struct {
	db         *sql.DB
	dimensions int
}

// New creates or opens a SQLite database and initializes the vec1 tables.
func New(dbPath string, dimensions int) (*Index, error) {
	db, err := driver.Open("file:"+dbPath+"?_pragma=journal_mode(wal)", vec1.Register)
	if err != nil {
		return nil, fmt.Errorf("opening sqlite index database: %w", err)
	}

	// vec1 does not support TEXT primary keys directly; use a mapping table (vec_ids)
	// that maps integer rowids to string IDs, and a separate virtual table for embeddings.
	schema := `
		CREATE TABLE IF NOT EXISTS vec_ids (
			rowid INTEGER PRIMARY KEY,
			id TEXT NOT NULL UNIQUE
		);

		CREATE VIRTUAL TABLE IF NOT EXISTS vec_embeddings USING vec1(embedding);
	`

	if _, err := db.Exec(schema); err != nil {
		if closeErr := db.Close(); closeErr != nil {
			return nil, fmt.Errorf("closing sqlite index database after schema failure: %w", closeErr)
		}
		return nil, fmt.Errorf("creating vec1 tables: %w", err)
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

	// vector -> JSON -> vec1_from_json on insert
	jsonVector, err := json.Marshal(vector)
	if err != nil {
		return fmt.Errorf("marshaling vector: %w", err)
	}

	tx, err := idx.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	// ensure an id -> rowid mapping exists, then upsert the embedding for rowid
	if _, err := tx.Exec(`INSERT OR IGNORE INTO vec_ids(id) VALUES (?)`, id); err != nil {
		return fmt.Errorf("inserting vec id mapping: %w", err)
	}

	var rowid int64
	if err := tx.QueryRow(`SELECT rowid FROM vec_ids WHERE id = ?`, id).Scan(&rowid); err != nil {
		return fmt.Errorf("selecting rowid for id %q: %w", id, err)
	}

	// vec1 can't upsert directly, so attempt an update and insert of zero rows were affected
	result, err := tx.Exec(`UPDATE vec_embeddings SET embedding = vec1_from_json(?) WHERE rowid = ?`, string(jsonVector), rowid)
	if err != nil {
		return fmt.Errorf("updating embedding: %w", err)
	}

	// ncruces never errors here
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		if _, err := tx.Exec(`INSERT INTO vec_embeddings(rowid, embedding) VALUES (?, vec1_from_json(?))`, rowid, string(jsonVector)); err != nil {
			return fmt.Errorf("inserting embedding: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit insert vector: %w", err)
	}

	return nil
}

func (idx *Index) DeleteVector(id string) error {
	tx, err := idx.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// get id then cascade delete the embedding
	var rowid int64
	if err := tx.QueryRow(`SELECT rowid FROM vec_ids WHERE id = ?`, id).Scan(&rowid); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("node %q not found in index", id)
		}
		return fmt.Errorf("finding rowid for delete: %w", err)
	}

	if _, err := tx.Exec(`DELETE FROM vec_embeddings WHERE rowid = ?`, rowid); err != nil {
		return fmt.Errorf("deleting embedding: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM vec_ids WHERE rowid = ?`, rowid); err != nil {
		return fmt.Errorf("deleting id mapping: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit delete: %w", err)
	}
	return nil
}

func (idx *Index) ListIDs() (ids []string, err error) {
	rows, err := idx.db.Query(`SELECT id FROM vec_ids`)
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

	jsonVector, err := json.Marshal(target)
	if err != nil {
		return nil, fmt.Errorf("marshaling target vector: %w", err)
	}

	// vec1 uses a table-valued query. first arg is target vector, second arg is options as a JSON object.
	// k belongs in that options, not as a LIMIT on the query (vec1 optimizes the ANN query based on the opts).
	// v.distance is metadata (don't return it), so we need vec1_cos_distance to get a user-facing distance value.
	query := `
		SELECT
			vi.id,
			vec1_cos_distance(v.embedding, vec1_from_json(?)) AS distance
		FROM vec_embeddings(vec1_from_json(?), json_object('k', ?)) AS v
		JOIN vec_ids vi ON vi.rowid = v.rowid
		ORDER BY distance
	`
	rows, err := idx.db.Query(query, string(jsonVector), string(jsonVector), k)
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
		var distance float64
		if err := rows.Scan(&res.ID, &distance); err != nil {
			return nil, fmt.Errorf("scanning search result: %w", err)
		}
		res.Distance = float32(distance)
		results = append(results, res)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating search results: %w", err)
	}

	return results, nil
}

func (idx *Index) LookupVector(id string) ([]float32, bool) {
	query := `
		SELECT vec1_to_json(v.embedding)
		FROM vec_embeddings v
		JOIN vec_ids vi ON vi.rowid = v.rowid
		WHERE vi.id = ?
	`

	var raw string
	if err := idx.db.QueryRow(query, id).Scan(&raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, false
		}
		return nil, false
	}
	var vector []float32
	if err := json.Unmarshal([]byte(raw), &vector); err != nil {
		return nil, false
	}
	return vector, true
}
