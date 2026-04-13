package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/alecthomas/kong"
	"github.com/jimschubert/mnemonic/internal/config"
	"github.com/sethvargo/go-envconfig"
)

var (
	projectName = "mnemonic"
	version     = "dev"
	commit      = "unknown SHA"
)

var CLI struct {
	// Default command does not yet do any daemon stuff. I copied this from jimschubert/hi, and plan to implement the daemon logic later.
	Default ServerCmd        `hidden:"" cmd:"" default:"withargs" help:"Ensures the daemon is running, starting if it's not (default)"`
	Server  ServerCmd        `cmd:"" help:"Start the mnemonic server"`
	Version kong.VersionFlag `short:"v" help:"Print version information"`
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
