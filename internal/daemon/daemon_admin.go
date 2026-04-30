package daemon

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strings"

	"github.com/jimschubert/mnemonic/internal/store"
)

type adminMergeRequest struct {
	KeepID   string `json:"keep_id"`
	DeleteID string `json:"delete_id"`
}

func (d *Daemon) handleAdminEntries(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	entries, err := d.store.All(parseAdminScopes(r.URL.Query().Get("scopes")))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"entries": entries})
}

func (d *Daemon) handleAdminEntryGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	entryID := r.PathValue("id")
	if entryID == "" {
		http.Error(w, "entry id is required", http.StatusBadRequest)
		return
	}

	entry, err := d.store.Get(entryID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(entry)
}

func (d *Daemon) handleAdminEntryUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	entryID := r.PathValue("id")
	if entryID == "" {
		http.Error(w, "entry id is required", http.StatusBadRequest)
		return
	}

	var entry store.Entry
	if err := json.NewDecoder(r.Body).Decode(&entry); err != nil {
		http.Error(w, fmt.Sprintf("decoding entry: %v", err), http.StatusBadRequest)
		return
	}
	entry.ID = entryID

	if saver, ok := d.store.(interface{ Save(*store.Entry) error }); ok {
		if err := saver.Save(&entry); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	} else if upserter, ok := d.store.(interface{ Upsert(*store.Entry) error }); ok {
		if err := upserter.Upsert(&entry); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	} else {
		http.Error(w, "store does not support updates", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "updated", "id": entryID})
}

func (d *Daemon) handleAdminEntryDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	entryID := r.PathValue("id")
	if entryID == "" {
		http.Error(w, "entry id is required", http.StatusBadRequest)
		return
	}

	if err := d.store.Delete(entryID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "deleted", "id": entryID})
}

func (d *Daemon) handleAdminHeads(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	heads, err := d.store.ListHeads(parseAdminScopes(r.URL.Query().Get("scopes")))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"heads": heads})
}

func (d *Daemon) handleAdminMerge(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req adminMergeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("decoding merge request: %v", err), http.StatusBadRequest)
		return
	}
	if req.KeepID == "" || req.DeleteID == "" {
		http.Error(w, "keep_id and delete_id are required", http.StatusBadRequest)
		return
	}

	keep, err := d.store.Get(req.KeepID)
	if err != nil {
		http.Error(w, fmt.Sprintf("getting kept entry: %v", err), http.StatusNotFound)
		return
	}
	del, err := d.store.Get(req.DeleteID)
	if err != nil {
		http.Error(w, fmt.Sprintf("getting deleted entry: %v", err), http.StatusNotFound)
		return
	}

	tagSet := make(map[string]bool, len(keep.Tags))
	for _, tag := range keep.Tags {
		tagSet[tag] = true
	}
	for _, tag := range del.Tags {
		if !tagSet[tag] {
			keep.Tags = append(keep.Tags, tag)
			tagSet[tag] = true
		}
	}

	keep.Score = math.Max(keep.Score, del.Score)
	keep.HitCount += del.HitCount
	if del.LastHit.After(keep.LastHit) {
		keep.LastHit = del.LastHit
	}

	if updater, ok := d.store.(interface{ Save(*store.Entry) error }); ok {
		if err := updater.Save(keep); err != nil {
			http.Error(w, fmt.Sprintf("updating kept entry: %v", err), http.StatusInternalServerError)
			return
		}
	} else if upserter, ok := d.store.(interface{ Upsert(*store.Entry) error }); ok {
		if err := upserter.Upsert(keep); err != nil {
			http.Error(w, fmt.Sprintf("updating kept entry: %v", err), http.StatusInternalServerError)
			return
		}
	} else {
		http.Error(w, "store does not support updates", http.StatusInternalServerError)
		return
	}

	if err := d.store.Delete(req.DeleteID); err != nil {
		http.Error(w, fmt.Sprintf("deleting merged entry: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":    "merged",
		"keep_id":   req.KeepID,
		"delete_id": req.DeleteID,
	})
}

func parseAdminScopes(raw string) []store.Scope {
	if raw == "" {
		return nil
	}
	parts := strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == ' ' })
	scopes := make([]store.Scope, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if _, ok := seen[part]; ok {
			continue
		}
		seen[part] = struct{}{}
		scopes = append(scopes, store.Scope(part))
	}
	return scopes
}
