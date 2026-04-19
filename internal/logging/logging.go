package logging

import (
	"log/slog"
	"os"

	"golang.org/x/term"

	"github.com/jimschubert/mnemonic/internal/config"
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

// ParseLevel parses a level string (debug, info, warn, error) into a slog.Level. Default is slog.LevelWarn.
func ParseLevel(s string) slog.Level {
	var level slog.Level
	if err := level.UnmarshalText([]byte(s)); err != nil {
		return slog.LevelWarn
	}
	return level
}

// ForScope creates a logger for the given package scope, according to user config.
// Falls back to conf.LogLevel (see config.Config#LogLevelFor).
func ForScope(conf config.Config, scope string) *slog.Logger {
	return New(ParseLevel(conf.LogLevelFor(scope)))
}
