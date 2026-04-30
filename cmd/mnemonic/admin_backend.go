package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jimschubert/mnemonic/internal/config"
	"github.com/jimschubert/mnemonic/internal/store"
)

type entryStoreBackend interface {
	All(scopes []store.Scope) ([]store.Entry, error)
	Get(id string) (*store.Entry, error)
	ListHeads(scopes []store.Scope) ([]store.HeadInfo, error)
	Save(entry *store.Entry) error
	Delete(id string) error
	Merge(keepID, deleteID string) error
	Close() error
}

type daemonEntryStoreBackend struct {
	client *daemonAdminClient
}

func (m daemonEntryStoreBackend) Save(entry *store.Entry) error {
	return m.client.save(entry)
}

func (m daemonEntryStoreBackend) All(scopes []store.Scope) ([]store.Entry, error) {
	return m.client.entries(scopes)
}

func (m daemonEntryStoreBackend) Get(id string) (*store.Entry, error) {
	return m.client.entry(id)
}

func (m daemonEntryStoreBackend) ListHeads(scopes []store.Scope) ([]store.HeadInfo, error) {
	return m.client.heads(scopes)
}

func (m daemonEntryStoreBackend) Delete(id string) error {
	return m.client.delete(id)
}

func (m daemonEntryStoreBackend) Merge(keepID, deleteID string) error {
	return m.client.merge(keepID, deleteID)
}

func (m daemonEntryStoreBackend) Close() error {
	return nil
}

type daemonAdminClient struct {
	httpClient *http.Client
	timeout    time.Duration
}

func newDaemonAdminClient(conf config.Config) *daemonAdminClient {
	return &daemonAdminClient{
		httpClient: &http.Client{
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
					return (&net.Dialer{}).DialContext(ctx, "unix", conf.SocketPath())
				},
			},
		},
		timeout: time.Duration(conf.ClientTimeout()) * time.Second,
	}
}

func (c *daemonAdminClient) save(entry *store.Entry) error {
	if entry == nil {
		return fmt.Errorf("entry is required")
	}
	if entry.ID == "" {
		return fmt.Errorf("entry id is required")
	}
	return c.doJSON(http.MethodPut, "/api/admin/entries/"+url.PathEscape(entry.ID), entry, nil)
}

func (c *daemonAdminClient) entries(scopes []store.Scope) ([]store.Entry, error) {
	var res struct {
		Entries []store.Entry `json:"entries"`
	}
	if err := c.doJSON(http.MethodGet, c.path("/api/admin/entries", scopes), nil, &res); err != nil {
		return nil, err
	}
	return res.Entries, nil
}

func (c *daemonAdminClient) entry(id string) (*store.Entry, error) {
	var res store.Entry
	if err := c.doJSON(http.MethodGet, "/api/admin/entries/"+url.PathEscape(id), nil, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

func (c *daemonAdminClient) heads(scopes []store.Scope) ([]store.HeadInfo, error) {
	var res struct {
		Heads []store.HeadInfo `json:"heads"`
	}
	if err := c.doJSON(http.MethodGet, c.path("/api/admin/heads", scopes), nil, &res); err != nil {
		return nil, err
	}
	return res.Heads, nil
}

func (c *daemonAdminClient) delete(id string) error {
	if id == "" {
		return fmt.Errorf("entry id is required")
	}
	return c.doJSON(http.MethodDelete, "/api/admin/entries/"+url.PathEscape(id), nil, nil)
}

func (c *daemonAdminClient) merge(keepID, deleteID string) error {
	if keepID == "" || deleteID == "" {
		return fmt.Errorf("both entry ids are required")
	}
	payload := map[string]string{
		"keep_id":   keepID,
		"delete_id": deleteID,
	}
	return c.doJSON(http.MethodPost, "/api/admin/entries/merge", payload, nil)
}

func (c *daemonAdminClient) path(base string, scopes []store.Scope) string {
	u := url.URL{Scheme: "http", Host: "unix", Path: base}
	if len(scopes) > 0 {
		q := u.Query()
		parts := make([]string, 0, len(scopes))
		for _, scope := range scopes {
			parts = append(parts, scope.String())
		}
		q.Set("scopes", strings.Join(parts, ","))
		u.RawQuery = q.Encode()
	}
	return u.String()
}

// doJSON sends an HTTP request (with optional JSON body) parsed the JSON response into out (if provided).
func (c *daemonAdminClient) doJSON(method, path string, body any, out any) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	var reqBody io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encoding request body: %w", err)
		}
		reqBody = bytes.NewReader(encoded)
	}

	reqTarget := path
	if !strings.HasPrefix(path, "http://") && !strings.HasPrefix(path, "https://") {
		reqURL := url.URL{Scheme: "http", Host: "unix", Path: path}
		reqTarget = reqURL.String()
	}
	req, err := http.NewRequestWithContext(ctx, method, reqTarget, reqBody)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("sending request: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode >= http.StatusMultipleChoices {
		details, _ := io.ReadAll(resp.Body)
		message := strings.TrimSpace(string(details))
		if message == "" {
			message = resp.Status
		}
		return fmt.Errorf("daemon admin %s %s: %s", method, path, message)
	}

	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decoding response: %w", err)
	}
	return nil
}
