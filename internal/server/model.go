package server

import "github.com/jimschubert/mnemonic/internal/store"

type QueryInput struct {
	Query    string   `json:"query"              jsonschema:"describe the current task or question to retrieve relevant lessons"`
	Category string   `json:"category,omitempty" jsonschema:"limit results to a specific category: avoidance, security, syntax, architecture, or domain"`
	TopK     int      `json:"top_k,omitempty"    jsonschema:"maximum number of entries to return (default: 5)"`
	Scopes   []string `json:"scopes,omitempty"   jsonschema:"limit to specific scopes: global, team, or project — empty returns all scopes"`
}

type QueryResult struct {
	ID       string   `json:"id"`
	Content  string   `json:"content"`
	Category string   `json:"category"`
	Tags     []string `json:"tags,omitempty"`
	Scope    string   `json:"scope"`
	Source   string   `json:"source,omitempty"`
}

type QueryOutput struct {
	Entries []QueryResult `json:"entries"`
}

type AddInput struct {
	Content  string   `json:"content"  jsonschema:"the knowledge to store"`
	Category string   `json:"category" jsonschema:"memory category: avoidance, security, syntax, architecture, or domain"`
	Tags     []string `json:"tags,omitempty" jsonschema:"2-5 inferred tags: lowercase words or hyphenated phrases drawn from language names, frameworks, patterns, or domain concepts"`
	Scope    string   `json:"scope,omitempty"  jsonschema:"global, team, or project (default: project)"`
	Source   string   `json:"source,omitempty" jsonschema:"audit label, e.g. agent:2025-04-12 or manual"`
}

type AddOutput struct {
	Status   string `json:"status"`
	ID       string `json:"id"`
	Scope    string `json:"scope"`
	Category string `json:"category"`
}

type ReinforceInput struct {
	ID    string  `json:"id"    jsonschema:"ID of the entry to adjust, as returned by mnemonic_query"`
	Delta float64 `json:"delta" jsonschema:"score adjustment: +0.1 for confirmed/approved patterns, -0.2 for rejected ones"`
}

type ReinforceOutput struct {
	Status string  `json:"status"`
	ID     string  `json:"id"`
	Delta  float64 `json:"delta"`
}

type ListHeadsInput struct{}

type ListHeadsOutput struct {
	Heads []store.HeadInfo `json:"heads"`
}
