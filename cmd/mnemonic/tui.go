package main

import (
	"fmt"
	"os"

	"github.com/charmbracelet/lipgloss"
)

var (
	titleStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7D56F4")).Padding(0, 1)
	entryMetaStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#AAAAAA")).Padding(0, 1)
	entryStyle     = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#444444")).
			Padding(0, 1).
			Width(80)
	simStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00")).Bold(true)
)

type compactProgress struct {
	total       int
	updated     int
	failed      int
	interactive bool
}

//goland:noinspection GoUnhandledErrorResult
func (p *compactProgress) write(i int, done bool, status string) {
	if !p.interactive {
		os.Stdout.WriteString(".")
		return
	}
	current := i
	if done {
		current = i + 1
	}
	_, _ = fmt.Fprintf(
		os.Stdout,
		"\rcompacting %d/%d updated=%d failed=%d status=%s",
		current,
		p.total,
		p.updated,
		p.failed,
		status,
	)
}

func (p *compactProgress) writeLine(i int, status string) {
	if !p.interactive {
		return
	}
	p.write(i, true, status)
	_, _ = fmt.Fprintln(os.Stdout)
}
