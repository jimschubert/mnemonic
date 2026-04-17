package config

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/alecthomas/assert/v2"
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

	assert.Equal(t, "warn", c.LogLevel)
	assert.Equal(t, 5, c.ClientTimeoutSec)
	assert.Equal(t, "localhost:20001", c.ServerAddr)
	assert.Equal(t, "~/.mnemonic/mnemonic.sock", c.SocketPathRaw)
	assert.Equal(t, 0, len(c.Logging))
}

func TestConfig_ClientTimeout(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		timeoutSec int
		expected   int
	}{
		{
			name:       "negative returns minimum",
			timeoutSec: -1,
			expected:   5,
		},
		{
			name:       "zero returns minimum",
			timeoutSec: 0,
			expected:   5,
		},
		{
			name:       "exactly 5 returns 5",
			timeoutSec: 5,
			expected:   5,
		},
		{
			name:       "greater than 5 returns value",
			timeoutSec: 10,
			expected:   10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c := Config{ClientTimeoutSec: tt.timeoutSec}
			assert.Equal(t, tt.expected, c.ClientTimeout())
		})
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
		{
			name:     "multiple scopes",
			env:      map[string]string{"LOG_LEVEL": "warn", "MNEMONIC_LOGGING": "store=debug,server=error"},
			scope:    "store",
			expected: "debug",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c := processWithMap(t, tt.env)
			assert.Equal(t, tt.expected, c.LogLevelFor(tt.scope))
		})
	}
}

func TestConfig_SocketPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		raw      string
		expected string
	}{
		{
			name:     "tilde expansion",
			raw:      "~/.mnemonic/mnemonic.sock",
			expected: "", // will check containment instead
		},
		{
			name:     "absolute path unchanged",
			raw:      "/var/run/mnemonic.sock",
			expected: "/var/run/mnemonic.sock",
		},
		{
			name:     "relative path unchanged",
			raw:      ".mnemonic/mnemonic.sock",
			expected: ".mnemonic/mnemonic.sock",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c := Config{SocketPathRaw: tt.raw}
			p := c.SocketPath()
			if tt.name == "tilde expansion" {
				assert.False(t, filepath.IsAbs(tt.raw))
				assert.True(t, filepath.IsAbs(p))
			} else {
				assert.Equal(t, tt.expected, p)
			}
		})
	}
}

func TestConfig_AsMap(t *testing.T) {
	t.Parallel()

	c := Config{
		LogLevel:         "debug",
		ClientTimeoutSec: 10,
		ServerAddr:       "localhost:9999",
		SocketPathRaw:    "~/.mnemonic/test.sock",
		Logging: map[string]string{
			"store":  "info",
			"server": "error",
		},
	}

	m := c.AsMap()
	assert.Equal(t, "debug", m["log_level"])
	assert.Equal(t, "10", m["client_timeout_sec"])
	assert.Equal(t, "localhost:9999", m["server_addr"])
	assert.Equal(t, "~/.mnemonic/test.sock", m["socket_path"])
	assert.Equal(t, true, len(m["logging"]) > 0)
}

func TestConfig_AsMapNoLogging(t *testing.T) {
	t.Parallel()

	c := Config{
		LogLevel:      "info",
		ServerAddr:    "localhost:20001",
		SocketPathRaw: "~/.mnemonic/mnemonic.sock",
	}

	m := c.AsMap()
	assert.Equal(t, 4, len(m))
	_, ok := m["logging"]
	assert.Equal(t, false, ok)
}

func TestLoad_FileOnly(t *testing.T) {
	t.Parallel()

	yaml := `
log_level: debug
server_addr: "localhost:9999"
client_timeout_sec: 10
`
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0600); err != nil {
		t.Fatalf("writing temp config: %v", err)
	}

	cfg, err := Load(path)
	assert.NoError(t, err)
	assert.Equal(t, "debug", cfg.LogLevel)
	assert.Equal(t, "localhost:9999", cfg.ServerAddr)
	assert.Equal(t, 10, cfg.ClientTimeoutSec)
}

func TestLoad_EnvOverridesFile(t *testing.T) {
	yaml := `
log_level: debug
server_addr: "localhost:9999"
`
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0600); err != nil {
		t.Fatalf("writing temp config: %v", err)
	}

	t.Setenv("MNEMONIC_SERVER_ADDR", "localhost:1111")

	cfg, err := Load(path)
	assert.NoError(t, err)
	assert.Equal(t, "localhost:1111", cfg.ServerAddr)
	assert.Equal(t, "debug", cfg.LogLevel)
}

func TestLoad_MissingFileUsesDefaults(t *testing.T) {
	t.Parallel()

	cfg, err := Load("/tmp/mnemonic-nonexistent-config-file.yaml")
	assert.NoError(t, err)
	assert.Equal(t, "warn", cfg.LogLevel)
	assert.Equal(t, "localhost:20001", cfg.ServerAddr)
}

func TestLoad_LaterFileWins(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	first := filepath.Join(dir, "global.yaml")
	second := filepath.Join(dir, "project.yaml")

	if err := os.WriteFile(first, []byte("server_addr: \"localhost:1000\"\n"), 0600); err != nil {
		t.Fatalf("writing first config: %v", err)
	}
	if err := os.WriteFile(second, []byte("server_addr: \"localhost:2000\"\n"), 0600); err != nil {
		t.Fatalf("writing second config: %v", err)
	}

	cfg, err := Load(first, second)
	assert.NoError(t, err)
	assert.Equal(t, "localhost:2000", cfg.ServerAddr)
}

func TestLoad_WithLogging(t *testing.T) {
	t.Parallel()

	yaml := `
log_level: info
logging:
  store: debug
  server: error
`
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0600); err != nil {
		t.Fatalf("writing temp config: %v", err)
	}

	cfg, err := Load(path)
	assert.NoError(t, err)
	assert.Equal(t, "info", cfg.LogLevel)
	assert.Equal(t, "debug", cfg.LogLevelFor("store"))
	assert.Equal(t, "error", cfg.LogLevelFor("server"))
	assert.Equal(t, "info", cfg.LogLevelFor("unknown"))
}

func TestConfig_ToEnvMap(t *testing.T) {
	t.Parallel()

	c := Config{
		LogLevel:         "debug",
		ServerAddr:       "localhost:9999",
		SocketPathRaw:    "/var/run/mnemonic.sock",
		ClientTimeoutSec: 15,
		Logging: map[string]string{
			"store": "info",
		},
	}

	m := c.toEnvMap()
	assert.Equal(t, "debug", m["LOG_LEVEL"])
	assert.Equal(t, "localhost:9999", m["MNEMONIC_SERVER_ADDR"])
	assert.Equal(t, "/var/run/mnemonic.sock", m["MNEMONIC_SOCKET_PATH"])
	assert.Equal(t, "15", m["MNEMONIC_CLIENT_TIMEOUT_SEC"])
	assert.Equal(t, "store=info", m["MNEMONIC_LOGGING"])
}

func TestConfig_ToEnvMapSkipsZeroValues(t *testing.T) {
	t.Parallel()

	c := Config{}
	m := c.toEnvMap()
	assert.Equal(t, 0, len(m))
}

func TestConfig_ApplyOverrides(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		base      Config
		overrides Config
		expected  Config
	}{
		{
			name: "override server_addr only",
			base: Config{
				LogLevel:   "debug",
				ServerAddr: "localhost:20001",
			},
			overrides: Config{
				ServerAddr: "localhost:9999",
			},
			expected: Config{
				LogLevel:   "debug",
				ServerAddr: "localhost:9999",
			},
		},
		{
			name: "override multiple fields",
			base: Config{
				LogLevel:         "warn",
				ServerAddr:       "localhost:20001",
				ClientTimeoutSec: 5,
			},
			overrides: Config{
				LogLevel:         "error",
				ClientTimeoutSec: 10,
			},
			expected: Config{
				LogLevel:         "error",
				ServerAddr:       "localhost:20001",
				ClientTimeoutSec: 10,
			},
		},
		{
			name: "zero values in overrides are ignored",
			base: Config{
				ClientTimeoutSec: 5,
			},
			overrides: Config{
				ClientTimeoutSec: 0,
			},
			expected: Config{
				ClientTimeoutSec: 5,
			},
		},
		{
			name: "empty strings in overrides are ignored",
			base: Config{
				ServerAddr: "localhost:20001",
			},
			overrides: Config{
				ServerAddr: "",
			},
			expected: Config{
				ServerAddr: "localhost:20001",
			},
		},
		{
			name: "merge logging maps",
			base: Config{
				Logging: map[string]string{
					"store": "info",
				},
			},
			overrides: Config{
				Logging: map[string]string{
					"server": "debug",
				},
			},
			expected: Config{
				Logging: map[string]string{
					"store":  "info",
					"server": "debug",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tt.base.ApplyOverrides(tt.overrides)
			assert.Equal(t, tt.expected.LogLevel, tt.base.LogLevel)
			assert.Equal(t, tt.expected.ServerAddr, tt.base.ServerAddr)
			assert.Equal(t, tt.expected.SocketPathRaw, tt.base.SocketPathRaw)
			assert.Equal(t, tt.expected.ClientTimeoutSec, tt.base.ClientTimeoutSec)
			assert.Equal(t, tt.expected.Logging, tt.base.Logging)
		})
	}
}
