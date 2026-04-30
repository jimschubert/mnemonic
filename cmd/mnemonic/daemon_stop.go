package main

import (
	"log/slog"

	"github.com/jimschubert/mnemonic/internal/config"
	"github.com/jimschubert/mnemonic/internal/daemon"
)

// DaemonStopCmd sends a shutdown event to a running daemon.
type DaemonStopCmd struct {
	ServerAddr string `short:"a" help:"TCP address for shutdown" env:"MNEMONIC_SERVER_ADDR"`
}

func (c *DaemonStopCmd) Run(logger *slog.Logger, conf config.Config) error {
	conf.ApplyOverrides(config.Config{
		ServerAddr: c.ServerAddr,
	})
	return daemon.RequestStop(conf, logger)
}
