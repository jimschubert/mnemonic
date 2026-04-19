package logging

import (
	"log/slog"
	"os"

	"golang.org/x/term"
)

// New returns a new slog.Logger configured appropriately for the environment.
// It uses a TextHandler if stderr is a terminal, and a JSONHandler otherwise.
// All logging is directed to os.Stderr to keep stdout clean for MCP stdio transport.
func New(level slog.Leveler) *slog.Logger {
	opts := &slog.HandlerOptions{
		Level: level,
	}

	var handler slog.Handler
	if term.IsTerminal(int(os.Stderr.Fd())) {
		handler = slog.NewTextHandler(os.Stderr, opts)
	} else {
		handler = slog.NewJSONHandler(os.Stderr, opts)
	}

	return slog.New(handler)
}
