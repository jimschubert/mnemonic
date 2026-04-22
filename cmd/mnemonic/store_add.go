package main

import (
	"fmt"
	"log/slog"

	"github.com/jimschubert/mnemonic/internal/config"
)

type AddCmd struct {
	Scope    string   `short:"s" default:"project" enum:"global,team,project" help:"Scope for the entry: global, team, or project"`
	Category string   `short:"c"  enum:"avoidance,security,syntax,architecture,domain" default:"domain" help:"Category for the entry"`
	Tags     []string `short:"t" help:"Comma-separated tags for the entry"`
	Content  string   `arg:"" help:"Text content to add to the memory store"`
}

func (c *AddCmd) Run(logger *slog.Logger, conf config.Config) error {
	payload := map[string]any{
		"scope":    c.Scope,
		"category": c.Category,
		"content":  c.Content,
	}
	if len(c.Tags) > 0 {
		payload["tags"] = c.Tags
	}

	_, err := socketSend(conf, "mnemonic_add", payload)
	if err != nil {
		return fmt.Errorf("adding entry to daemon: %w (is it started?)", err)
	}
	logger.Info("Entry added successfully")
	return nil
}
