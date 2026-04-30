package main

import (
	"fmt"
	"log/slog"

	"github.com/jimschubert/mnemonic/internal/config"
)

type DeleteCmd struct {
	ID string `arg:"" help:"ID of the memory entry to delete"`
}

func (c *DeleteCmd) Run(_ *slog.Logger, conf config.Config) error {
	if err := newDaemonAdminClient(conf).delete(c.ID); err != nil {
		return fmt.Errorf("deleting entry: %w (is daemon started?)", err)
	}
	fmt.Printf("Entry %s deleted\n", c.ID)
	return nil
}
