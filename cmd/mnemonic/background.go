package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"time"

	"github.com/jimschubert/mnemonic/internal/config"
	"github.com/jimschubert/mnemonic/internal/daemon"
)

// startDaemonBackground spawns the daemon subcommand as a background process.
// extraEnv is appended to the current environment (e.g. to forward CLI flag values).
func startDaemonBackground(logger *log.Logger, extraEnv []string) error {
	program, err := os.Executable()
	if err != nil {
		return fmt.Errorf("could not determine executable path: %w", err)
	}

	cmd := exec.Command(program, "server")
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil

	cmd.Env = append(os.Environ(), extraEnv...)

	logger.Printf("spawning server: %s server", program)
	return cmd.Start()
}

// ensureDaemon starts the daemon if it is not already running and waits for it to become ready.
func ensureDaemon(logger *log.Logger, conf config.Config, extraEnv []string) error {
	if daemon.IsRunning(conf) {
		return nil
	}

	logger.Println("daemon not running, starting it now...")
	if err := startDaemonBackground(logger, extraEnv); err != nil {
		return fmt.Errorf("could not start daemon: %w", err)
	}

	logger.Println("daemon process started, waiting for socket availability...")
	for range 20 {
		time.Sleep(250 * time.Millisecond)
		if daemon.IsRunning(conf) {
			break
		}
	}

	if !daemon.IsRunning(conf) {
		return fmt.Errorf("daemon failed to start (socket: %s)", conf.SocketPath())
	}

	logger.Println("daemon is ready")
	return nil
}
