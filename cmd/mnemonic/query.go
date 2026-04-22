package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jimschubert/mnemonic/internal/config"
)

type QueryCmd struct {
	SocketPath string   `short:"s" default:"${socket_path}" help:"Path to daemon socket" env:"MNEMONIC_SOCKET_PATH"`
	Raw        bool     `help:"Output raw JSON response"`
	NoMeta     bool     `help:"Exclude metadata from output (when --raw is not set)"`
	Category   string   `short:"c" default:"syntax" enum:"avoidance,security,syntax,architecture,domain" help:"Limit results to a specific category: avoidance, security, syntax, architecture, or domain"`
	TopK       int      `short:"t" default:"5" help:"TopK number of entries to return (default: 5)"`
	Scopes     []string `help:"Limit to specific scopes: global, team, or project — empty returns all scopes"`
	Query      string   `arg:"" help:"Text to query for"`
}

func (c *QueryCmd) Run(logger *slog.Logger, conf config.Config) error {
	conf.ApplyOverrides(config.Config{
		SocketPathRaw: c.SocketPath,
	})

	payload := map[string]any{
		"query":    c.Query,
		"category": c.Category,
		"top_k":    c.TopK,
	}
	if len(c.Scopes) > 0 {
		payload["scopes"] = c.Scopes
	}

	res, err := socketSend(conf, "mnemonic_query", payload)
	if err != nil {
		return fmt.Errorf("querying daemon: %w (is it started?)", err)
	}

	if c.Raw {
		marshaled, err := json.MarshalIndent(res, "", "  ")
		if err != nil {
			return fmt.Errorf("marshaling response: %w", err)
		}
		fmt.Printf("%s\n", marshaled)
		return nil
	}

	type result struct {
		Entries []struct {
			ID       string   `json:"id,omitempty"`
			Content  string   `json:"content"`
			Category string   `json:"category"`
			Tags     []string `json:"tags,omitempty"`
			Scope    string   `json:"scope,omitempty"`
			Source   string   `json:"source,omitempty"`
		} `json:"entries"`
	}

	marshaled, err := json.Marshal(res)
	if err != nil {
		return fmt.Errorf("marshaling response: %w", err)
	}

	var r result
	err = json.Unmarshal(marshaled, &r)
	if err != nil {
		return fmt.Errorf("unmarshaling response: %w", err)
	}

	for i, entry := range r.Entries {
		if i > 0 {
			fmt.Println()
		}
		fmt.Printf("%d. [%s] %s\n", i+1, entry.Category, entry.Content)
		if !c.NoMeta {
			sb := &strings.Builder{}
			if entry.Scope != "" {
				fmt.Fprintf(sb, "scope=%s ", entry.Scope)
			}
			if len(entry.Tags) > 0 {
				fmt.Fprintf(sb, "tags=%v ", entry.Tags)
			}
			if entry.Source != "" {
				fmt.Fprintf(sb, "source=%s", entry.Source)
			}
			if sb.Len() > 0 {
				fmt.Fprintf(sb, "   (%s)\n", sb.String())
			}
		}
	}

	return nil
}
