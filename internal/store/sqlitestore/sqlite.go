package sqlitestore

import (
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"math"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	_ "github.com/ncruces/go-sqlite3/driver"

	"github.com/jimschubert/mnemonic/internal/store"
)

// Option is a functional option for configuring SQLiteStore.
type Option func(*options)

type options struct {
	autoHitCounting  bool
	configuredScopes []store.Scope
}

// WithAutoHitCounting controls whether queries automatically increment HitCount and LastHit.
// Enabled by default; pass false to disable.
func WithAutoHitCounting(enabled bool) Option {
	return func(o *options) {
		o.autoHitCounting = enabled
	}
}

// WithConfiguredScopes sets the allowed (or default) scopes for query methods.
// When set, queries which don't specify scopes will default to these, while queries providing scopes must be a subset of this set.
// This applies to reads/queries. Writes (e.g. Upsert and Promote) are not constrained.
func WithConfiguredScopes(scopes []store.Scope) Option {
	return func(o *options) {
		o.configuredScopes = append([]store.Scope(nil), scopes...)
	}
}

type SQLiteStore struct {
	mu               sync.RWMutex
	db               *sql.DB
	logger           *slog.Logger
	autoHitCounting  bool
	sqlWeightedSort  bool
	configuredScopes map[store.Scope]struct{}
}

var _ store.Store = (*SQLiteStore)(nil)

func New(path string, logger *slog.Logger, opts ...Option) (*SQLiteStore, error) {
	if path == "" {
		return nil, fmt.Errorf("path is required")
	}

	o := &options{
		autoHitCounting: true, // default value
	}
	for _, opt := range opts {
		opt(o)
	}
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}

	if path != ":memory:" {
		dir := filepath.Dir(path)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("creating sqlite store directory: %w", err)
		}
	}

	db, err := sql.Open("sqlite3", sqliteDSN(path))
	if err != nil {
		return nil, fmt.Errorf("opening sqlite database: %w", err)
	}

	s := &SQLiteStore{
		db:               db,
		logger:           logger,
		autoHitCounting:  o.autoHitCounting,
		configuredScopes: make(map[store.Scope]struct{}, len(o.configuredScopes)),
	}
	for _, scope := range o.configuredScopes {
		s.configuredScopes[scope] = struct{}{}
	}

	if err := s.ensureSchema(); err != nil {
		e := fmt.Errorf("initializing sqlite schema: %w", err)
		if err := db.Close(); err != nil {
			return nil, errors.Join(e, fmt.Errorf("failed to close sqlite database: %w", err))
		}
		return nil, e
	}
	s.sqlWeightedSort = s.supportsSQLWeightedSort()

	return s, nil
}

func (s *SQLiteStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil {
		return nil
	}
	err := s.db.Close()
	s.db = nil
	return err
}

func (s *SQLiteStore) ListHeads(scopes []store.Scope) ([]store.HeadInfo, error) {
	entries, err := s.entries(scopes, "")
	if err != nil {
		return nil, err
	}

	counts := map[string]int{}
	for _, entry := range entries {
		counts[entry.Category]++
	}

	headInfos := make([]store.HeadInfo, 0, len(counts))
	for category, count := range counts {
		headInfos = append(headInfos, store.HeadInfo{
			Name:      category,
			Count:     count,
			Mandatory: store.IsMandatoryCategory(category),
		})
	}
	sort.Slice(headInfos, func(i, j int) bool {
		return headInfos[i].Name < headInfos[j].Name
	})

	return headInfos, nil
}

func (s *SQLiteStore) All(scopes []store.Scope) ([]store.Entry, error) {
	entries, err := s.entries(scopes, "")
	if err != nil {
		return nil, err
	}
	s.sortEntries(entries)
	if err := s.markHits(entries); err != nil {
		return nil, err
	}
	return entries, nil
}

func (s *SQLiteStore) AllByCategory(category string, topK int, scopes []store.Scope) ([]store.Entry, error) {
	entries, err := s.entries(scopes, category)
	if err != nil {
		return nil, err
	}

	s.sortEntries(entries)
	if topK > 0 && len(entries) > topK {
		entries = entries[:topK]
	}
	if err := s.markHits(entries); err != nil {
		return nil, err
	}
	return entries, nil
}

func (s *SQLiteStore) Get(id string) (*store.Entry, error) {
	return s.getByID(id)
}

func (s *SQLiteStore) Query(category string, tags []string) ([]store.Entry, error) {
	requiredTags := make([]string, len(tags))
	for i, tag := range tags {
		requiredTags[i] = strings.ToLower(tag)
	}

	entries, err := s.entries(nil, category)
	if err != nil {
		return nil, err
	}

	hits := make([]store.Entry, 0, len(entries))
	for _, entry := range entries {
		if !tagsMatch(entry.Tags, requiredTags) {
			continue
		}
		hits = append(hits, entry)
	}

	s.sortEntries(hits)
	if err := s.markHits(hits); err != nil {
		return nil, err
	}

	return hits, nil
}

func (s *SQLiteStore) QueryByCategory(category, query string, topK int, scopes []store.Scope) ([]store.Entry, error) {
	terms := strings.Fields(strings.ToLower(query))
	entries, err := s.entries(scopes, category)
	if err != nil {
		return nil, err
	}

	candidates := make([]store.Entry, 0, len(entries))
	for _, entry := range entries {
		if !keywordMatch(entry, terms) {
			continue
		}
		candidates = append(candidates, entry)
	}

	s.sortEntries(candidates)
	if topK > 0 && len(candidates) > topK {
		candidates = candidates[:topK]
	}
	if err := s.markHits(candidates); err != nil {
		return nil, err
	}

	return candidates, nil
}

func (s *SQLiteStore) Upsert(entry *store.Entry) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if entry == nil {
		return fmt.Errorf("entry is required")
	}
	if !store.IsAllowedCategory(entry.Category) {
		return fmt.Errorf("category %q is not allowed; must be one of %v", entry.Category, store.AllowedCategories())
	}
	if entry.ID == "" {
		id, err := generateID()
		if err != nil {
			return err
		}
		entry.ID = id
	}
	if entry.Scope == "" {
		entry.Scope = store.ScopeGlobal.String()
	}
	if entry.Score == 0 {
		entry.Score = 1.0
	}
	if entry.Created.IsZero() {
		entry.Created = time.Now()
	}

	tags, err := json.Marshal(entry.Tags)
	if err != nil {
		return fmt.Errorf("encoding tags for entry %q: %w", entry.ID, err)
	}

	_, err = s.db.Exec(`
		INSERT INTO entries (
			id, content, tags, category, scope, score, hit_count, last_hit, created, source
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			content = excluded.content,
			tags = excluded.tags,
			category = excluded.category,
			scope = excluded.scope,
			score = excluded.score,
			hit_count = excluded.hit_count,
			last_hit = excluded.last_hit,
			created = excluded.created,
			source = excluded.source
	`,
		entry.ID,
		entry.Content,
		string(tags),
		entry.Category,
		entry.Scope,
		entry.Score,
		entry.HitCount,
		entry.LastHit,
		entry.Created,
		entry.Source,
	)
	if err != nil {
		return fmt.Errorf("upserting entry %q: %w", entry.ID, err)
	}

	return nil
}

func (s *SQLiteStore) Score(id string, delta float64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, err := s.getByID(id)
	if err != nil {
		return err
	}

	newScore := math.Max(0, entry.Score+delta)
	if s.autoHitCounting {
		_, err = s.db.Exec(`
			UPDATE entries
			SET score = ?, hit_count = hit_count + 1, last_hit = ?
			WHERE id = ?
		`, newScore, time.Now(), id)
	} else {
		_, err = s.db.Exec(`
			UPDATE entries
			SET score = ?
			WHERE id = ?
		`, newScore, id)
	}
	if err != nil {
		return fmt.Errorf("scoring entry %q: %w", id, err)
	}

	return nil
}

func (s *SQLiteStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	result, err := s.db.Exec(`DELETE FROM entries WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("deleting entry %q: %w", id, err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil && rowsAffected <= 0 {
		return fmt.Errorf("checking delete result for %q: %w", id, err)
	}
	if rowsAffected <= 0 {
		s.logger.Debug("requested entry was not deleted", "id", id)
	}
	return nil
}

func (s *SQLiteStore) Promote(id string, targetScope store.Scope) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	result, err := s.db.Exec(`UPDATE entries SET scope = ? WHERE id = ?`, targetScope.String(), id)
	if err != nil {
		return fmt.Errorf("promoting entry %q: %w", id, err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking promote result for %q: %w", id, err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("entry %q not found", id)
	}

	return nil
}

func (s *SQLiteStore) ensureSchema() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS entries (
			id TEXT PRIMARY KEY,
			content TEXT NOT NULL,
			tags TEXT NOT NULL,
			category TEXT NOT NULL,
			scope TEXT NOT NULL,
			score REAL NOT NULL,
			hit_count INTEGER NOT NULL,
			last_hit TIMESTAMP NOT NULL,
			created TIMESTAMP NOT NULL,
			source TEXT NOT NULL DEFAULT ''
		);
		CREATE INDEX IF NOT EXISTS idx_entries_category ON entries(category);
		CREATE INDEX IF NOT EXISTS idx_entries_scope ON entries(scope);
	`)
	if err != nil {
		return fmt.Errorf("initializing sqlite schema: %w", err)
	}
	return nil
}

// entries constructs and executes the SQL query for retrieving entries with optional scopes and categories clauses.
func (s *SQLiteStore) entries(scopes []store.Scope, category string) ([]store.Entry, error) {
	queryScopes, err := s.resolveQueryScopes(scopes)
	if err != nil {
		return nil, err
	}

	baseQuery := `
		SELECT id, content, tags, category, scope, score, hit_count, last_hit, created, source
		FROM entries
	`
	whereClauses := make([]string, 0, 2)
	args := make([]any, 0, len(queryScopes)+1)

	if category != "" {
		whereClauses = append(whereClauses, "category = ?")
		args = append(args, category)
	}
	if len(queryScopes) > 0 {
		placeholders := make([]string, len(queryScopes))
		for i, scope := range queryScopes {
			placeholders[i] = "?"
			args = append(args, scope.String())
		}
		whereClauses = append(whereClauses, "scope IN ("+strings.Join(placeholders, ",")+")")
	}
	if len(whereClauses) > 0 {
		baseQuery += " WHERE " + strings.Join(whereClauses, " AND ")
	}
	if s.sqlWeightedSort {
		baseQuery += `
			 ORDER BY
				score * exp(-0.05 * (julianday('now') - COALESCE(julianday(NULLIF(last_hit, '')), julianday(NULLIF(created, '')), julianday('now')))) DESC,
				id ASC
		`
	}

	rows, err := s.db.Query(baseQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("querying entries: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			s.logger.Debug("rows close error", "err", err)
		}
	}()

	entries := make([]store.Entry, 0)
	for rows.Next() {
		entry, err := scanEntry(rows)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating entries: %w", err)
	}

	return entries, nil
}

func (s *SQLiteStore) resolveQueryScopes(requested []store.Scope) ([]store.Scope, error) {
	if len(s.configuredScopes) == 0 {
		return requested, nil
	}
	if len(requested) == 0 {
		return slices.Collect(maps.Keys(s.configuredScopes)), nil
	}

	resolved := make([]store.Scope, 0, len(requested))
	seen := make(map[store.Scope]struct{}, len(requested))
	for _, scope := range requested {
		if _, ok := s.configuredScopes[scope]; !ok {
			return nil, fmt.Errorf("scope %q is not configured", scope)
		}
		if _, ok := seen[scope]; ok {
			continue
		}
		seen[scope] = struct{}{}
		resolved = append(resolved, scope)
	}

	return resolved, nil
}

func (s *SQLiteStore) sortEntries(entries []store.Entry) {
	if s.sqlWeightedSort {
		// was part of the query, so already sorted
		return
	}
	store.SortByWeightedScore(entries)
}

func (s *SQLiteStore) supportsSQLWeightedSort() bool {
	var v float64
	// when exp is available, we can move sort logic into the query itself
	err := s.db.QueryRow(`SELECT exp(0.0)`).Scan(&v)
	if err != nil {
		s.logger.Debug("sqlite exp() unavailable; falling back to Go-side weighted sorting", "err", err)
		return false
	}
	return true
}

func (s *SQLiteStore) getByID(id string) (*store.Entry, error) {
	row := s.db.QueryRow(`
		SELECT id, content, tags, category, scope, score, hit_count, last_hit, created, source
		FROM entries
		WHERE id = ?
	`, id)

	entry, err := scanEntry(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("entry %q not found", id)
		}
		return nil, err
	}

	return &entry, nil
}

func (s *SQLiteStore) markHits(entries []store.Entry) error {
	if !s.autoHitCounting || len(entries) == 0 {
		return nil
	}

	now := time.Now()
	seen := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		seen[entry.ID] = struct{}{}
	}

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("starting hit count transaction: %w", err)
	}
	defer func() {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			s.logger.Warn("tx rollback failed", "err", err)
		}
	}()

	for id := range seen {
		if _, err := tx.Exec(`
			UPDATE entries
			SET hit_count = hit_count + 1, last_hit = ?
			WHERE id = ?
		`, now, id); err != nil {
			return fmt.Errorf("updating hit count for %q: %w", id, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing hit count transaction: %w", err)
	}

	return nil
}

func tagsMatch(entryTags []string, required []string) bool {
	if len(required) == 0 {
		return true
	}

	set := make(map[string]struct{}, len(entryTags))
	for _, tag := range entryTags {
		set[strings.ToLower(tag)] = struct{}{}
	}
	for _, requirement := range required {
		if _, ok := set[requirement]; !ok {
			return false
		}
	}

	return true
}

func keywordMatch(entry store.Entry, terms []string) bool {
	if len(terms) == 0 {
		return true
	}

	content := strings.ToLower(entry.Content)
	for _, term := range terms {
		if strings.Contains(content, term) {
			return true
		}
	}
	for _, tag := range entry.Tags {
		tagLower := strings.ToLower(tag)
		for _, term := range terms {
			if strings.Contains(tagLower, term) {
				return true
			}
		}
	}

	return false
}

// rowScanner adapts both sql.Row and sql.Rows for extracting items from columns
type rowScanner interface {
	Scan(dest ...any) error
}

func scanEntry(scanner rowScanner) (store.Entry, error) {
	var entry store.Entry
	var tagsJSON string
	var lastHit time.Time
	var created time.Time

	err := scanner.Scan(
		&entry.ID,
		&entry.Content,
		&tagsJSON,
		&entry.Category,
		&entry.Scope,
		&entry.Score,
		&entry.HitCount,
		&lastHit,
		&created,
		&entry.Source,
	)
	if err != nil {
		return store.Entry{}, err
	}

	if tagsJSON == "" {
		entry.Tags = []string{}
	} else {
		if err := json.Unmarshal([]byte(tagsJSON), &entry.Tags); err != nil {
			return store.Entry{}, fmt.Errorf("decoding tags for entry %q: %w", entry.ID, err)
		}
	}

	entry.LastHit = lastHit
	entry.Created = created

	return entry, nil
}

func sqliteDSN(path string) string {
	if path == ":memory:" {
		return "file::memory:?_pragma=journal_mode(wal)"
	}
	return "file:" + path + "?_pragma=journal_mode(wal)"
}

func generateID() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generating ID: %w", err)
	}
	return fmt.Sprintf("%x", b), nil
}
