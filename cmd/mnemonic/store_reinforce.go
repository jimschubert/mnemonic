package main

import (
	"fmt"
	"log/slog"

	"github.com/jimschubert/mnemonic/internal/config"
)

type ReinforceCmd struct {
	ID    string  `help:"ID of the memory entry to reinforce"`
	Delta float64 `help:"Amount to increase the score by (e.g. 1.0)"`
}

func (c *ReinforceCmd) Run(logger *slog.Logger, conf config.Config) error {
	payload := map[string]any{
		"id":    c.ID,
		"delta": c.Delta,
	}

	_, err := socketSend(conf, "mnemonic_reinforce", payload)
	if err != nil {
		return fmt.Errorf("reinforcing entry in daemon: %w (is it started?)", err)
	}
	fmt.Println("Entry reinforced successfully")
	return nil
}
