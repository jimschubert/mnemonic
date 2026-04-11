package store

import (
	"time"
)

// You can regenerate Entry using yq:
//
//	yq '.entries.[0]' -P -o json  entries.example.yaml | pbcopy
//
// Then, in Goland, you can use CMD+N -> "Type from JSON" and paste it.
// You may need to clean up (Id->ID, Score is float64, etc.) and add YAML tags.

// Entry represents a single knowledge nugget.
type Entry struct {
	ID       string    `json:"id" yaml:"id"`
	Content  string    `json:"content" yaml:"content"`
	Tags     []string  `json:"tags" yaml:"tags"`
	Category string    `json:"category" yaml:"category"`
	Scope    string    `json:"scope" yaml:"scope"`
	Score    float64   `json:"score" yaml:"score"`
	HitCount int       `json:"hit_count" yaml:"hit_count"`
	LastHit  time.Time `json:"last_hit" yaml:"last_hit"`
	Created  time.Time `json:"created" yaml:"created"`
	Source   string    `json:"source" yaml:"source"`
}
