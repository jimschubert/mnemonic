package config

import (
	"strings"
)

type Config struct {
	LogLevel string `env:"LOG_LEVEL,default=warn"`
	// Logging allows for scoped logging, e.g. server=warn; scopes will be 1:1 with packages, e.g. server, store, etc.
	Logging          map[string]string `env:"MNEMONIC_LOGGING,separator=="`
	ClientTimeoutSec int               `env:"MNEMONIC_CLIENT_TIMEOUT_SEC,default=5"`
	ServerAddr       string            `env:"MNEMONIC_SERVER_ADDR,default=localhost:20001"`
}

func (c Config) ClientTimeout() int {
	return max(5, c.ClientTimeoutSec)
}
func (c Config) LogLevelFor(scope string) string {
	logLevel := c.LogLevel
	if value, ok := c.Logging[scope]; ok {
		logLevel = strings.TrimSpace(value)
	}
	return logLevel
}
