package config

import (
	"context"
	"fmt"
	"testing"

	"github.com/sethvargo/go-envconfig"
)

func processWithMap(t *testing.T, env map[string]string) Config {
	t.Helper()
	var c Config
	if err := envconfig.ProcessWith(context.Background(), &envconfig.Config{
		Target:   &c,
		Lookuper: envconfig.MapLookuper(env),
	}); err != nil {
		t.Fatalf("envconfig.ProcessWith: %v", err)
	}
	return c
}

func TestConfig_Defaults(t *testing.T) {
	t.Parallel()

	c := processWithMap(t, map[string]string{})

	if c.LogLevel != "warn" {
		t.Errorf("LogLevel: got %q, want %q", c.LogLevel, "warn")
	}
	if len(c.Logging) != 0 {
		t.Errorf("Logging: got %v, want empty map", c.Logging)
	}
	if c.ClientTimeoutSec != 5 {
		t.Errorf("ClientTimeoutSec: got %v, want %d", c.ClientTimeoutSec, 5)
	}
	if c.ServerAddr != "localhost:20001" {
		t.Errorf("ServerAddr: got %q, want %q", c.ServerAddr, "localhost:20001")
	}
}

func TestConfig_LogLevelFor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		env      map[string]string
		scope    string
		expected string
	}{
		{
			name:     "unknown scope returns global level",
			env:      map[string]string{"LOG_LEVEL": "error"},
			scope:    "unknown",
			expected: "error",
		},
		{
			name:     "known scope overrides global",
			env:      map[string]string{"LOG_LEVEL": "error", "MNEMONIC_LOGGING": "myservice=debug"},
			scope:    "myservice",
			expected: "debug",
		},
		{
			name:     "scoped value is trimmed",
			env:      map[string]string{"MNEMONIC_LOGGING": "myservice=  info  "},
			scope:    "myservice",
			expected: "info",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := processWithMap(t, tc.env)
			if got := c.LogLevelFor(tc.scope); got != tc.expected {
				t.Errorf("LogLevelFor(%q): got %q, want %q", tc.scope, got, tc.expected)
			}
		})
	}
}

func TestConfig_ClientTimeout(t *testing.T) {
	t.Parallel()

	defaultValue := 5
	defaultStr := fmt.Sprintf("%d", defaultValue)

	tests := []struct {
		name               string
		hiClientTimeoutSec int
		want               int
	}{
		{
			name:               "negative value returns min of " + defaultStr,
			hiClientTimeoutSec: -1,
			want:               defaultValue,
		},
		{
			name:               "zero value returns min of " + defaultStr,
			hiClientTimeoutSec: 0,
			want:               defaultValue,
		},
		{
			name:               defaultStr + " returns " + defaultStr,
			hiClientTimeoutSec: defaultValue,
			want:               defaultValue,
		},

		{
			name:               "value > default returns that value",
			hiClientTimeoutSec: defaultValue + 1,
			want:               defaultValue + 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := Config{
				ClientTimeoutSec: tt.hiClientTimeoutSec,
			}
			if got := c.ClientTimeout(); got != tt.want {
				t.Errorf("ClientTimeout() = %v, want %v", got, tt.want)
			}
		})
	}
}
