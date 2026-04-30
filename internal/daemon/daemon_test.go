package daemon

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alecthomas/assert/v2"
	"github.com/jimschubert/mnemonic/internal/config"
	"github.com/jimschubert/mnemonic/internal/logging"
	"github.com/jimschubert/mnemonic/internal/store"
)

func waitForCondition(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}

	t.Fatalf("condition not met within %s", timeout)
}

func shortSocketPath(t *testing.T) string {
	t.Helper()

	path := filepath.Join(os.TempDir(), fmt.Sprintf("mnemonic-%d.sock", time.Now().UnixNano()))
	t.Cleanup(func() {
		_ = os.Remove(path)
	})

	return path
}

func TestDaemonStart_ShutdownRequestStopsServer(t *testing.T) {
	conf := config.Config{
		SocketPathRaw:    shortSocketPath(t),
		ClientTimeoutSec: 1,
	}

	logger := logging.New(slog.LevelInfo)
	d := New(&store.NoopStore{}, conf, logger)
	errCh := make(chan error, 1)

	go func() {
		errCh <- d.Start(context.Background())
	}()

	waitForCondition(t, time.Second, func() bool {
		return IsRunning(conf)
	})

	assert.NoError(t, sendShutdown(conf, ""))

	select {
	case err := <-errCh:
		assert.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("daemon did not stop after shutdown request")
	}

	assert.False(t, IsRunning(conf))
}

type adminMockStore struct {
	entries map[string]*store.Entry
}

func newAdminMockStore() *adminMockStore {
	return &adminMockStore{entries: make(map[string]*store.Entry)}
}

func (s *adminMockStore) ListHeads(scopes []store.Scope) ([]store.HeadInfo, error) {
	counts := map[string]int{}
	for _, entry := range s.AllForScopes(scopes) {
		counts[entry.Category]++
	}
	out := make([]store.HeadInfo, 0, len(counts))
	for name, count := range counts {
		out = append(out, store.HeadInfo{Name: name, Count: count, Mandatory: store.IsMandatoryCategory(name)})
	}
	return out, nil
}

func (s *adminMockStore) All(scopes []store.Scope) ([]store.Entry, error) {
	return s.AllForScopes(scopes), nil
}

func (s *adminMockStore) AllForScopes(scopes []store.Scope) []store.Entry {
	if len(scopes) == 0 {
		out := make([]store.Entry, 0, len(s.entries))
		for _, entry := range s.entries {
			out = append(out, *entry)
		}
		return out
	}

	allowed := make(map[string]struct{}, len(scopes))
	for _, scope := range scopes {
		allowed[scope.String()] = struct{}{}
	}

	out := make([]store.Entry, 0, len(s.entries))
	for _, entry := range s.entries {
		if _, ok := allowed[entry.Scope]; ok {
			out = append(out, *entry)
		}
	}
	return out
}

func (s *adminMockStore) AllByCategory(_ string, _ int, _ []store.Scope) ([]store.Entry, error) {
	return nil, nil
}

func (s *adminMockStore) Get(id string) (*store.Entry, error) {
	entry, ok := s.entries[id]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	return entry, nil
}

func (s *adminMockStore) Query(_ string, _ []string) ([]store.Entry, error) { return nil, nil }

func (s *adminMockStore) QueryByCategory(_, _ string, _ int, _ []store.Scope) ([]store.Entry, error) {
	return nil, nil
}

func (s *adminMockStore) Upsert(entry *store.Entry) error {
	copyEntry := *entry
	s.entries[entry.ID] = &copyEntry
	return nil
}

func (s *adminMockStore) Score(_ string, _ float64) error       { return nil }
func (s *adminMockStore) Delete(id string) error                { delete(s.entries, id); return nil }
func (s *adminMockStore) Promote(_ string, _ store.Scope) error { return nil }

func (s *adminMockStore) Save(entry *store.Entry) error {
	return s.Upsert(entry)
}

var _ store.Store = (*adminMockStore)(nil)

func TestAdminHandlers_UpdateDeleteAndMerge(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		wantStatus int
		assertFn   func(t *testing.T, store *adminMockStore, body string)
	}{
		{
			name:       "updates entry by id",
			method:     http.MethodPut,
			path:       "/api/admin/entries/keep",
			body:       `{"content":"updated","category":"domain","scope":"project"}`,
			wantStatus: http.StatusOK,
			assertFn: func(t *testing.T, store *adminMockStore, body string) {
				t.Helper()
				assert.Equal(t, "updated", store.entries["keep"].Content)
				assert.Contains(t, strings.ToLower(body), "updated")
			},
		},
		{
			name:       "lists entries by scope",
			method:     http.MethodGet,
			path:       "/api/admin/entries?scopes=project",
			wantStatus: http.StatusOK,
			assertFn: func(t *testing.T, store *adminMockStore, body string) {
				t.Helper()
				assert.Contains(t, strings.ToLower(body), "keep me")
				assert.NotContains(t, body, "remove me")
			},
		},
		{
			name:       "gets entry by id",
			method:     http.MethodGet,
			path:       "/api/admin/entries/keep",
			wantStatus: http.StatusOK,
			assertFn: func(t *testing.T, store *adminMockStore, body string) {
				t.Helper()
				assert.Contains(t, body, `"id":"keep"`)
				assert.Contains(t, body, `"content":"keep me"`)
			},
		},
		{
			name:       "lists heads by scope",
			method:     http.MethodGet,
			path:       "/api/admin/heads?scopes=project",
			wantStatus: http.StatusOK,
			assertFn: func(t *testing.T, store *adminMockStore, body string) {
				t.Helper()
				assert.Contains(t, body, `"heads"`)
				assert.Contains(t, body, `"name":"domain"`)
			},
		},
		{
			name:       "deletes entry by id",
			method:     http.MethodDelete,
			path:       "/api/admin/entries/delete",
			wantStatus: http.StatusOK,
			assertFn: func(t *testing.T, store *adminMockStore, body string) {
				t.Helper()
				_, ok := store.entries["delete"]
				assert.False(t, ok)
				assert.Contains(t, strings.ToLower(body), "deleted")
			},
		},
		{
			name:       "merges entries",
			method:     http.MethodPost,
			path:       "/api/admin/entries/merge",
			body:       `{"keep_id":"keep","delete_id":"delete"}`,
			wantStatus: http.StatusOK,
			assertFn: func(t *testing.T, store *adminMockStore, body string) {
				t.Helper()
				assert.Equal(t, []string{"a", "b"}, store.entries["keep"].Tags)
				_, ok := store.entries["delete"]
				assert.False(t, ok)
				assert.Contains(t, strings.ToLower(body), "merged")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adminStore := newAdminMockStore()
			adminStore.entries["keep"] = &store.Entry{ID: "keep", Content: "keep me", Category: "domain", Tags: []string{"a"}, Scope: "project"}
			adminStore.entries["delete"] = &store.Entry{ID: "delete", Content: "remove me", Category: "domain", Tags: []string{"b"}, Scope: "global"}

			d := &Daemon{store: adminStore}
			mux := http.NewServeMux()
			mux.HandleFunc("GET /api/admin/entries", d.handleAdminEntries)
			mux.HandleFunc("GET /api/admin/entries/{id}", d.handleAdminEntryGet)
			mux.HandleFunc("PUT /api/admin/entries/{id}", d.handleAdminEntryUpdate)
			mux.HandleFunc("DELETE /api/admin/entries/{id}", d.handleAdminEntryDelete)
			mux.HandleFunc("GET /api/admin/heads", d.handleAdminHeads)
			mux.HandleFunc("POST /api/admin/entries/merge", d.handleAdminMerge)

			req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			rec := httptest.NewRecorder()

			mux.ServeHTTP(rec, req)

			assert.Equal(t, tt.wantStatus, rec.Code)
			tt.assertFn(t, adminStore, rec.Body.String())
		})
	}
}
