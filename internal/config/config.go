package config

import (
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/sethvargo/go-envconfig"
	"go.yaml.in/yaml/v4"
)

var Version = "0.1.0"

type Config struct {
	LogLevel string `yaml:"log_level" env:"LOG_LEVEL,default=warn"`
	// Logging allows for scoped logging, e.g. server=warn; scopes will be 1:1 with packages, e.g. server, store, etc.
	Logging          map[string]string `yaml:"logging" env:"MNEMONIC_LOGGING,separator=="`
	ClientTimeoutSec int               `yaml:"client_timeout_sec" env:"MNEMONIC_CLIENT_TIMEOUT_SEC,default=5"`
	ServerAddr       string            `yaml:"server_addr" env:"MNEMONIC_SERVER_ADDR,default=localhost:20001"`
	SocketPathRaw    string            `yaml:"socket_path" env:"MNEMONIC_SOCKET_PATH,default=~/.mnemonic/mnemonic.sock"`
}

func (c *Config) ClientTimeout() int {
	return max(5, c.ClientTimeoutSec)
}

func (c *Config) LogLevelFor(scope string) string {
	logLevel := c.LogLevel
	if value, ok := c.Logging[scope]; ok {
		logLevel = strings.TrimSpace(value)
	}
	return logLevel
}

// SocketPath returns the expanded Unix socket path for daemon IPC.
func (c *Config) SocketPath() string {
	p := c.SocketPathRaw
	if strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			p = filepath.Join(home, p[2:])
		}
	}
	return p
}

// Load reads config from the given YAML file paths (later paths win) and then
// overlays environment variables on top. Environment variables always take
// precedence over file values.
func Load(paths ...string) (Config, error) {
	fm := make(map[string]string)
	for _, p := range paths {
		if strings.HasPrefix(p, "~/") {
			if home, err := os.UserHomeDir(); err == nil {
				p = filepath.Join(home, p[2:])
			}
		}
		data, err := os.ReadFile(p)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return Config{}, fmt.Errorf("reading config %s: %w", p, err)
		}
		var fc Config
		if err = yaml.Unmarshal(data, &fc); err != nil {
			return Config{}, fmt.Errorf("parsing config %s: %w", p, err)
		}
		// paths[0] < paths[1] < ... < paths[n]
		maps.Copy(fm, fc.toEnvMap())
	}

	var cfg Config
	err := envconfig.ProcessWith(context.Background(), &envconfig.Config{
		Target: &cfg,
		// this looks up left-to-right. Order of precedence is: env vars > config files a...b > struct defaults.
		Lookuper: envconfig.MultiLookuper(envconfig.OsLookuper(), envconfig.MapLookuper(fm)),
	})
	return cfg, err
}

func (c *Config) AsMap() map[string]string {
	m := map[string]string{
		"log_level":          c.LogLevel,
		"client_timeout_sec": strconv.Itoa(c.ClientTimeoutSec),
		"server_addr":        c.ServerAddr,
		"socket_path":        c.SocketPathRaw,
	}
	if len(c.Logging) > 0 {
		m["logging"] = c.logString()
	}
	return m
}

func (c *Config) logString() string {
	if len(c.Logging) == 0 {
		return ""
	}

	parts := make([]string, 0, len(c.Logging))
	for k, v := range c.Logging {
		parts = append(parts, k+"="+v)
	}
	return strings.Join(parts, ",")
}

func (c *Config) toEnvMap() map[string]string {
	m := map[string]string{}
	if c.LogLevel != "" {
		m["LOG_LEVEL"] = c.LogLevel
	}
	if c.ServerAddr != "" {
		m["MNEMONIC_SERVER_ADDR"] = c.ServerAddr
	}
	if c.SocketPathRaw != "" {
		m["MNEMONIC_SOCKET_PATH"] = c.SocketPathRaw
	}
	if c.ClientTimeoutSec != 0 {
		m["MNEMONIC_CLIENT_TIMEOUT_SEC"] = strconv.Itoa(c.ClientTimeoutSec)
	}

	if len(c.Logging) > 0 {
		m["MNEMONIC_LOGGING"] = c.logString()
	}
	return m
}

// ApplyOverrides merges non-empty/non-zero fields from overrides into c.
// Use this to apply CLI flags or other sources that should take precedence.
func (c *Config) ApplyOverrides(overrides Config) {
	if overrides.LogLevel != "" {
		c.LogLevel = overrides.LogLevel
	}
	if overrides.ServerAddr != "" {
		c.ServerAddr = overrides.ServerAddr
	}
	if overrides.SocketPathRaw != "" {
		c.SocketPathRaw = overrides.SocketPathRaw
	}
	if overrides.ClientTimeoutSec != 0 {
		c.ClientTimeoutSec = overrides.ClientTimeoutSec
	}
	if len(overrides.Logging) > 0 {
		if c.Logging == nil {
			c.Logging = make(map[string]string)
		}
		maps.Copy(c.Logging, overrides.Logging)
	}
}
