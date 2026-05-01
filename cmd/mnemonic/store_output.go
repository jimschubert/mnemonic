package main

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/jimschubert/mnemonic/internal/store"
	"github.com/muesli/reflow/wordwrap"
)

type storeEntryPrinter struct {
	width      int
	labelWidth int
}

func (p storeEntryPrinter) contentWrapWidth() int {
	const minWrapWidth = 20
	wrapWidth := p.width - p.labelWidth
	if wrapWidth < minWrapWidth {
		return minWrapWidth
	}
	return wrapWidth
}

func (p storeEntryPrinter) printEntry(w io.Writer, entry store.Entry) {
	fmt.Fprintf(w, "id:\t%s\n", entry.ID)

	contentLines := p.wrapContent(entry.Content)

	fmt.Fprintf(w, "content:\t%s\n", contentLines[0])
	for _, line := range contentLines[1:] {
		// subsequent lines need a \t prefix to align properly under the content label
		fmt.Fprintf(w, "\t%s\n", line)
	}

	fmt.Fprintf(w, "category:\t%s\n", entry.Category)
	fmt.Fprintf(w, "scope:\t%s\n", entry.Scope)

	if len(entry.Tags) > 0 {
		fmt.Fprintf(w, "tags:\t%s\n", strings.Join(entry.Tags, ", "))
	}

	fmt.Fprintf(w, "score:\t%.4f\n", entry.Score)
	fmt.Fprintf(w, "hit_count:\t%d\n", entry.HitCount)

	if !entry.LastHit.IsZero() {
		fmt.Fprintf(w, "last_hit:\t%s\n", entry.LastHit.Format(time.RFC3339))
	}

	fmt.Fprintf(w, "created:\t%s\n", entry.Created.Format(time.RFC3339))

	if entry.Source != "" {
		fmt.Fprintf(w, "source:\t%s\n", entry.Source)
	}
}

func (p storeEntryPrinter) wrapContent(content string) []string {
	wrapWidth := p.contentWrapWidth()
	// tabs in content can disrupt tabwriter alignment… convert them to spaces
	normalized := strings.ReplaceAll(content, "\t", " ")
	// newline is a nwe paragraph which needs separate wrapping
	paragraphs := strings.Split(normalized, "\n")
	wrappedLines := make([]string, 0, len(paragraphs))
	for _, paragraph := range paragraphs {
		if paragraph == "" {
			wrappedLines = append(wrappedLines, "")
			continue
		}
		wrapped := wordwrap.String(paragraph, wrapWidth)
		wrappedLines = append(wrappedLines, strings.Split(wrapped, "\n")...)
	}

	return wrappedLines
}
