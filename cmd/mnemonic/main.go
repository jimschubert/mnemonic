package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/alecthomas/kong"
	kongyaml "github.com/alecthomas/kong-yaml"
	"github.com/jimschubert/mnemonic/internal/config"
	"github.com/sethvargo/go-envconfig"
)

var (
	projectName = "mnemonic"
	version     = "dev"
	commit      = "unknown SHA"
)

var CLI struct {
	Default StdioCmd         `hidden:"" cmd:"" default:"withargs" help:"Serve MCP over stdio, starting the daemon if needed (default)"`
	Stdio   StdioCmd         `cmd:"" help:"Serve MCP over stdio, starting the daemon if needed"`
	Server  ServerCmd        `cmd:"" help:"Start the MCP HTTP server, starting the daemon if needed"`
	Stop    StopCmd          `cmd:"" help:"Send a graceful shutdown request to a running daemon"`
	Version kong.VersionFlag `short:"v" help:"Print version information"`
}

func main() {
	logger := log.New(os.Stdout, "["+projectName+"] ", 0)

	conf := processConfig()
	ctx := kong.Parse(&CLI,
		kong.Name(projectName),
		kong.Description("Attention-based MCP memory controller for LLM coding agents."),
		kong.Configuration(kongyaml.Loader, "~/.mnemonic/config.yaml", ".mnemonic/config.yaml"),
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
