package main

import (
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
