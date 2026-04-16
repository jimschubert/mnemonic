package main

import (
	"log"

	"github.com/jimschubert/mnemonic/internal/config"
	"github.com/jimschubert/mnemonic/internal/daemon"
)

// StopCmd sends a graceful shutdown event to a running daemon.
type StopCmd struct {
	ServerAddr string `short:"a" help:"TCP address for shutdown" env:"MNEMONIC_SERVER_ADDR"`
}

func (c *StopCmd) Run(logger *log.Logger, conf config.Config) error {
	if c.ServerAddr != "" {
		conf.ServerAddr = c.ServerAddr
	}
	return daemon.RequestStop(conf, logger)
}
