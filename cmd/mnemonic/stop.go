package main

import (
	"log"

	"github.com/jimschubert/mnemonic/internal/config"
	"github.com/jimschubert/mnemonic/internal/daemon"
)

// StopCmd sends a graceful shutdown event to a running daemon.
type StopCmd struct {
	ServerAddr string `short:"a" help:"Target TCP address instead of the Unix socket" env:"MNEMONIC_SERVER_ADDR"`
}

func (c *StopCmd) Run(logger *log.Logger, conf config.Config) error {
	if err := daemon.RequestStop(conf, c.ServerAddr); err != nil {
		return err
	}
	logger.Println("shutdown request sent")
	return nil
}
