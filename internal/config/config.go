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

// Index holds configuration for the vector index.
type Index struct {
	Type        string  `yaml:"type" env:"INDEX_TYPE,default=hnsw"`
	Dimensions  int     `yaml:"dimensions" env:"DIMENSIONS,default=768"`
	Connections int     `yaml:"connections" env:"CONNECTIONS,default=16"`
	LevelFactor float64 `yaml:"level_factor" env:"LEVEL_FACTOR,default=0.25"`
	EfSearch    int     `yaml:"ef_search" env:"EF_SEARCH,default=50"`
}

// Embeddings holds configuration for the embedding endpoint and model. Defaults are LM Studio specifics.
type Embeddings struct {
	Endpoint  string `yaml:"endpoint" env:"ENDPOINT,default=http://127.0.0.1:1234/v1/embeddings"`
	Model     string `yaml:"model" env:"MODEL,default=nomic-ai/nomic-embed-text-v1.5"`
	AuthToken string `yaml:"auth_token" env:"AUTH_TOKEN"`
	// SkipPreflight is a boolean flag to skip the preflight check before building the index.
	// This is useful for testing or if the user is confident that the embedding endpoint is working correctly.
	SkipPreflight bool `yaml:"skip_preflight" env:"SKIP_PREFLIGHT"`
}

type Config struct {
	LogLevel string `yaml:"log_level" env:"LOG_LEVEL,default=warn"`
	// Logging allows for scoped logging, e.g. server=warn; scopes will be 1:1 with packages, e.g. server, store, etc.
	Logging          map[string]string `yaml:"logging" env:"MNEMONIC_LOGGING,separator=="`
	ClientTimeoutSec int               `yaml:"client_timeout_sec" env:"MNEMONIC_CLIENT_TIMEOUT_SEC,default=5"`
	ServerAddr       string            `yaml:"server_addr" env:"MNEMONIC_SERVER_ADDR,default=localhost:20001"`
	SocketPathRaw    string            `yaml:"socket_path" env:"MNEMONIC_SOCKET_PATH,default=~/.mnemonic/mnemonic.sock"`
	// AuthToken, if non-empty, requires all TCP HTTP requests to present "Authorization: Bearer <token>".
	AuthToken string `yaml:"auth_token" env:"MNEMONIC_AUTH_TOKEN"`
	// AllowedOrigins enables CORS for the listed origins. Use "*" to permit any origin.
	AllowedOrigins []string `yaml:"allowed_origins" env:"MNEMONIC_ALLOWED_ORIGINS"`
	// UnauthenticatedStatus exempts GET /api/status from bearer-token enforcement on TCP.
	UnauthenticatedStatus bool       `yaml:"unauthenticated_status" env:"MNEMONIC_UNAUTHENTICATED_STATUS"`
	Embeddings            Embeddings `yaml:"embeddings" env:", prefix=MNEMONIC_EMBEDDINGS_"`
	Index                 Index      `yaml:"index" env:", prefix=MNEMONIC_INDEX_"`
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
	// note: put everything with relevant zero values in the map initializer.
	m := map[string]string{
		"log_level":                 c.LogLevel,
		"client_timeout_sec":        strconv.Itoa(c.ClientTimeoutSec),
		"server_addr":               c.ServerAddr,
		"socket_path":               c.SocketPathRaw,
		"embeddings_skip_preflight": strconv.FormatBool(c.Embeddings.SkipPreflight),
		"unauthenticated_status":    strconv.FormatBool(c.UnauthenticatedStatus),
	}
	putIfNotZero(m, "logging", c.logString())
	putIfNotZero(m, "auth_token", c.AuthToken)
	if len(c.AllowedOrigins) > 0 {
		m["allowed_origins"] = strings.Join(c.AllowedOrigins, ",")
	}
	putIfNotZero(m, "embeddings_endpoint", c.Embeddings.Endpoint)
	putIfNotZero(m, "embeddings_model", c.Embeddings.Model)
	putIfNotZero(m, "embeddings_auth_token", c.Embeddings.AuthToken)
	putIfNotZero(m, "index_type", c.Index.Type)
	putIfNotZero(m, "index_dimensions", c.Index.Dimensions, strconv.Itoa)
	putIfNotZero(m, "index_connections", c.Index.Connections, strconv.Itoa)
	putIfNotZero(m, "index_level_factor", c.Index.LevelFactor, formatFloat64)
	putIfNotZero(m, "index_ef_search", c.Index.EfSearch, strconv.Itoa)
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
	m := make(map[string]string)
	putIfNotZero(m, "LOG_LEVEL", c.LogLevel)
	putIfNotZero(m, "MNEMONIC_SERVER_ADDR", c.ServerAddr)
	putIfNotZero(m, "MNEMONIC_SOCKET_PATH", c.SocketPathRaw)
	putIfNotZero(m, "MNEMONIC_CLIENT_TIMEOUT_SEC", c.ClientTimeoutSec, strconv.Itoa)
	putIfNotZero(m, "MNEMONIC_LOGGING", c.logString())
	putIfNotZero(m, "MNEMONIC_AUTH_TOKEN", c.AuthToken)
	if len(c.AllowedOrigins) > 0 {
		m["MNEMONIC_ALLOWED_ORIGINS"] = strings.Join(c.AllowedOrigins, ",")
	}
	putIfNotZero(m, "MNEMONIC_EMBEDDINGS_ENDPOINT", c.Embeddings.Endpoint)
	putIfNotZero(m, "MNEMONIC_EMBEDDINGS_MODEL", c.Embeddings.Model)
	putIfNotZero(m, "MNEMONIC_EMBEDDINGS_AUTH_TOKEN", c.Embeddings.AuthToken)
	putIfNotZero(m, "MNEMONIC_INDEX_TYPE", c.Index.Type)
	putIfNotZero(m, "MNEMONIC_INDEX_DIMENSIONS", c.Index.Dimensions, strconv.Itoa)
	putIfNotZero(m, "MNEMONIC_INDEX_CONNECTIONS", c.Index.Connections, strconv.Itoa)
	putIfNotZero(m, "MNEMONIC_INDEX_LEVEL_FACTOR", c.Index.LevelFactor, formatFloat64)
	putIfNotZero(m, "MNEMONIC_INDEX_EF_SEARCH", c.Index.EfSearch, strconv.Itoa)

	// keep zero value, always
	m["MNEMONIC_EMBEDDINGS_SKIP_PREFLIGHT"] = strconv.FormatBool(c.Embeddings.SkipPreflight)
	m["MNEMONIC_UNAUTHENTICATED_STATUS"] = strconv.FormatBool(c.UnauthenticatedStatus)
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
	if overrides.AuthToken != "" {
		c.AuthToken = overrides.AuthToken
	}
	if len(overrides.AllowedOrigins) > 0 {
		c.AllowedOrigins = overrides.AllowedOrigins
	}
	if overrides.UnauthenticatedStatus {
		c.UnauthenticatedStatus = overrides.UnauthenticatedStatus
	}

	if overrides.Embeddings.Endpoint != "" {
		c.Embeddings.Endpoint = overrides.Embeddings.Endpoint
	}
	if overrides.Embeddings.Model != "" {
		c.Embeddings.Model = overrides.Embeddings.Model
	}
	if overrides.Embeddings.AuthToken != "" {
		c.Embeddings.AuthToken = overrides.Embeddings.AuthToken
	}
	if overrides.Embeddings.SkipPreflight {
		c.Embeddings.SkipPreflight = overrides.Embeddings.SkipPreflight
	}

	if overrides.Index.Type != "" {
		c.Index.Type = overrides.Index.Type
	}
	if overrides.Index.Dimensions != 0 {
		c.Index.Dimensions = overrides.Index.Dimensions
	}
	if overrides.Index.Connections != 0 {
		c.Index.Connections = overrides.Index.Connections
	}
	if overrides.Index.LevelFactor != 0 {
		c.Index.LevelFactor = overrides.Index.LevelFactor
	}
	if overrides.Index.EfSearch != 0 {
		c.Index.EfSearch = overrides.Index.EfSearch
	}
}

// formatFloat64 formats a float64 to string without trailing zeros.
func formatFloat64(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}

// putIfNotZero adds value to map if it's not the zero value.
// For non-string types, pass an optional stringify conversion function.
func putIfNotZero[T comparable](m map[string]string, key string, value T, stringify ...func(T) string) {
	var zero T
	if value != zero {
		if len(stringify) > 0 {
			m[key] = stringify[0](value)
		} else {
			var s any = value
			if str, ok := s.(string); ok {
				m[key] = str
			}
		}
	}
}
