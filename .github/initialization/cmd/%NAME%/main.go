package main

import (
	"fmt"
	"log"
	"os"

	"github.com/alecthomas/kong"
)

var (
	programName = "%NAME%"
	version     = "dev"
	commit      = "unknown SHA"
)

var CLI struct {
	Version kong.VersionFlag `short:"v" help:"Print version information"`
}

func main() {
	formattedVersion := fmt.Sprintf("%s (%s)", version, commit)

	kong.Parse(&CLI,
		kong.Name(programName),
		kong.Description("[placeholder description]"),
		kong.UsageOnError(),
		kong.Vars{
			"version": formattedVersion,
		},
	)

	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	_, _ = fmt.Fprint(os.Stderr, "Not yet implemented")
	os.Exit(1)
	return nil
}
