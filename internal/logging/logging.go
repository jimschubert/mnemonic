package logging

import (
	"log/slog"
	"os"

	"golang.org/x/term"

	"github.com/jimschubert/mnemonic/internal/config"
)

// New returns a new slog.Logger configured appropriately for the environment.
// It uses a TextHandler if stdout is a terminal, and a JSONHandler otherwise.
// All logging is directed to os.Stdout to keep stdout clean for MCP stdio transport.
func New(level slog.Leveler) *slog.Logger {
	return NewWithWriter(level, os.Stdout)
}

// NewWithWriter is like New but allows specifying the output writer, which is useful for testing or stdio output.
func NewWithWriter(level slog.Leveler, writer *os.File) *slog.Logger {
	opts := &slog.HandlerOptions{
		Level: level,
	}

	var handler slog.Handler
	if term.IsTerminal(int(writer.Fd())) {
		handler = slog.NewTextHandler(writer, opts)
	} else {
		handler = slog.NewJSONHandler(writer, opts)
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
	return New(ParseLevel(conf.LogLevelFor(scope))).WithGroup(scope)
}
