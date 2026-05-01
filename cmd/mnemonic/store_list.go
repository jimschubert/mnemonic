package main

import (
	"fmt"
	"log/slog"
	"maps"
	"os"
	"slices"
	"strings"
	"text/tabwriter"

	"github.com/jimschubert/mnemonic/internal/config"
	"github.com/jimschubert/mnemonic/internal/store"
)

type ListCmd struct {
	Scope    []string `help:"Limit results to one or more scopes (repeat flag): global, team, project"`
	Category []string `short:"c" help:"Limit results to one or more categories (repeat flag)"`
}

//goland:noinspection GoUnhandledErrorResult
func (c *ListCmd) Run(_ *slog.Logger, conf config.Config) error {
	scopes, err := parseScopeFilters(c.Scope)
	if err != nil {
		return err
	}

	// TODO: pagination? pass categories as filter to backend?
	entries, err := newDaemonAdminClient(conf).entries(scopes)
	if err != nil {
		return fmt.Errorf("listing entries: %w (is daemon started?)", err)
	}

	entries = filterEntriesByCategory(entries, c.Category)
	if len(entries) == 0 {
		fmt.Println("No entries found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 4, ' ', 0)
	defer w.Flush() //nolint:errcheck
	printer := storeEntryPrinter{width: 80, labelWidth: 18}

	for i, entry := range entries {
		if i > 0 {
			fmt.Fprintf(w, "%s\n", strings.Repeat("-", printer.width)) //nolint:errcheck
		}
		printer.printEntry(w, entry)
	}

	return nil
}

func parseScopeFilters(values []string) ([]store.Scope, error) {
	if len(values) == 0 {
		return nil, nil
	}

	found := make(map[store.Scope]struct{})
	for _, input := range values {
		if !store.IsAllowedScope(input) {
			return nil, fmt.Errorf("invalid scope %q: expected one of %v", input, store.AllowedScopes())
		}
		found[store.Scope(input)] = struct{}{}
	}

	return slices.Collect(maps.Keys(found)), nil
}

func filterEntriesByCategory(entries []store.Entry, categories []string) []store.Entry {
	if len(categories) == 0 {
		return entries
	}
	allowed := make(map[string]struct{}, len(categories))
	for _, category := range categories {
		allowed[strings.ToLower(strings.TrimSpace(category))] = struct{}{}
	}

	filtered := make([]store.Entry, 0, len(entries))
	for _, entry := range entries {
		if _, ok := allowed[strings.ToLower(strings.TrimSpace(entry.Category))]; ok {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}
