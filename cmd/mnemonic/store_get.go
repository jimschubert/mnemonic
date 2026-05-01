package main

import (
	"fmt"
	"log/slog"
	"os"
	"text/tabwriter"

	"github.com/jimschubert/mnemonic/internal/config"
)

type GetCmd struct {
	ID string `arg:"" help:"ID of the memory entry to retrieve"`
}

//goland:noinspection GoUnhandledErrorResult
func (c *GetCmd) Run(_ *slog.Logger, conf config.Config) error {
	entry, err := newDaemonAdminClient(conf).entry(c.ID)
	if err != nil {
		return fmt.Errorf("fetching entry: %w (is daemon started?)", err)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 4, ' ', 0)
	defer w.Flush()

	printer := storeEntryPrinter{width: 80, labelWidth: 18}
	printer.printEntry(w, *entry)

	return nil
}
