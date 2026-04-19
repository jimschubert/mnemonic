package main

import (
	"context"
	"fmt"
	"log"
	"maps"
	"os"

	"github.com/alecthomas/kong"
	kongyaml "github.com/alecthomas/kong-yaml"
	"github.com/jimschubert/mnemonic/internal/config"
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
	Stop    StopCmd          `cmd:"" help:"Send a shutdown request to a running daemon"`
	Embed   EmbedCmd         `cmd:"" help:"Fetch embeddings and build the HNSW index"`
	Version kong.VersionFlag `short:"v" help:"Print version information"`
}

func main() {
	logger := log.New(os.Stdout, "["+projectName+"] ", 0)

	conf, err := config.Load("~/.mnemonic/config.yaml", ".mnemonic/config.yaml")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading config: %s\n", err)
		os.Exit(1)
	}

	vars := kong.Vars{}
	maps.Copy(vars, conf.AsMap())
	vars["version"] = fmt.Sprintf("%s (%s)", version, commit)

	ctx := kong.Parse(&CLI,
		kong.Name(projectName),
		kong.Description("Attention-based MCP memory controller for LLM coding agents."),
		kong.Configuration(kongyaml.Loader, "~/.mnemonic/config.yaml", ".mnemonic/config.yaml"),
		kong.UsageOnError(),
		kong.Bind(
			logger,
			conf,
		),
		vars,
	)

	err = ctx.Run(context.Background())
	ctx.FatalIfErrorf(err)
}
