package main

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
	"text/tabwriter"
	"time"

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
	defer w.Flush() //nolint:errcheck

	fmt.Fprintf(w, "id:\t%s\n", entry.ID)             //nolint:errcheck
	fmt.Fprintf(w, "content:\t%s\n", entry.Content)   //nolint:errcheck
	fmt.Fprintf(w, "category:\t%s\n", entry.Category) //nolint:errcheck
	fmt.Fprintf(w, "scope:\t%s\n", entry.Scope)       //nolint:errcheck
	if len(entry.Tags) > 0 {
		fmt.Fprintf(w, "tags:\t%s\n", strings.Join(entry.Tags, ", ")) //nolint:errcheck
	}
	fmt.Fprintf(w, "score:\t%.4f\n", entry.Score)      //nolint:errcheck
	fmt.Fprintf(w, "hit_count:\t%d\n", entry.HitCount) //nolint:errcheck
	if !entry.LastHit.IsZero() {
		fmt.Fprintf(w, "last_hit:\t%s\n", entry.LastHit.Format(time.RFC3339)) //nolint:errcheck
	}
	fmt.Fprintf(w, "created:\t%s\n", entry.Created.Format(time.RFC3339)) //nolint:errcheck
	if entry.Source != "" {
		fmt.Fprintf(w, "source:\t%s\n", entry.Source) //nolint:errcheck
	}

	return nil
}
