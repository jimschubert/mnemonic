package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/alecthomas/kong"
	"github.com/jimschubert/mnemonic/internal/config"
	"github.com/jimschubert/mnemonic/internal/server"
	"github.com/jimschubert/mnemonic/internal/store"
	"github.com/sethvargo/go-envconfig"
)

var (
	projectName = "mnemonic"
	version     = "dev"
	commit      = "unknown SHA"
)

var CLI struct {
	Default DefaultCmd       `hidden:"" cmd:"" default:"withargs" help:"Ensures the daemon is running, starting if it's not (default)"`
	Version kong.VersionFlag `short:"v" help:"Print version information"`
}

// DefaultCmd runs when the user doesn't specify a command.
type DefaultCmd struct {
	ServerAddr string `help:"Address to listen on for MCP requests" default:"localhost:20001" env:"MNEMONIC_SERVER_ADDR"`
}

func (c *DefaultCmd) Run(logger *log.Logger, conf config.Config) error {
	if c.ServerAddr != "" {
		if conf.ServerAddr != "" && conf.ServerAddr != c.ServerAddr {
			logger.Printf("warning: MCP address specified in both config and CLI, using CLI value: %s", c.ServerAddr)
		}
		conf.ServerAddr = c.ServerAddr
	}

	noop := &store.NoopStore{}
	mcpServer := server.NewServer(noop, conf)
	return mcpServer.Serve(context.Background())
}

func main() {
	logger := log.New(os.Stdout, "["+projectName+"] ", 0)

	conf := processConfig()
	ctx := kong.Parse(&CLI,
		kong.Name(projectName),
		kong.Description("Attention-based MCP memory controller for LLM coding agents."),
		kong.UsageOnError(),
		kong.Vars{
			"version": fmt.Sprintf("%s (%s)", version, commit),
		},
		kong.Bind(
			logger,
			conf,
		),
	)

	err := ctx.Run(context.Background())
	ctx.FatalIfErrorf(err)
}

func processConfig() config.Config {
	c := config.Config{}
	err := envconfig.Process(context.Background(), &c)
	if err != nil {
		fmt.Printf("error processing config: %s\n", err)
		os.Exit(1)
	}
	return c
}
