package main

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"os"

	"github.com/alecthomas/kong"
	kongyaml "github.com/alecthomas/kong-yaml"
	"github.com/jimschubert/mnemonic/internal/config"
	"github.com/jimschubert/mnemonic/internal/logging"
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
	Lint    LintCmd          `cmd:"" help:"Analyze memory store for redundancy and resolve interactively"`
	Store   StoreCmd         `cmd:"" help:"Interact with the memory store over unix socket (daemon must be running)"`
	Version kong.VersionFlag `short:"v" help:"Print version information"`
}

func main() {
	// root logger. sub-components will use logging.ForScope to pick up user configurations (if available).
	logger := logging.New(slog.LevelInfo)

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
