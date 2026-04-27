package main

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/jimschubert/mnemonic/internal/config"
	"github.com/jimschubert/mnemonic/internal/daemon"
)

type daemonEnvOptions struct {
	GlobalDir string
	LocalDir  string
	Team      []string
	Mandatory []string

	IncludeServerAddr bool
}

// startDaemonBackground spawns the daemon subcommand as a background process.
// extraEnv is appended to the current environment (e.g. to forward CLI flag values).
func startDaemonBackground(logger *slog.Logger, extraEnv []string) error {
	program, err := os.Executable()
	if err != nil {
		return fmt.Errorf("could not determine executable path: %w", err)
	}

	cmd := newDaemonCommand(program, extraEnv)

	logger.Info("spawning daemon", "command", program+" daemon")
	return cmd.Start()
}

func newDaemonCommand(program string, extraEnv []string) *exec.Cmd {
	cmd := exec.Command(program, "daemon")
	cmd.Stdin = nil
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr

	// detach the child process from the parent session so daemon can keep running if server/stdio stops.
	// the daemon still requires an explicit stop command.
	cmd.SysProcAttr = &syscall.SysProcAttr{
		// start the daemon in a new session so it does not share the parent terminal's process group
		Setsid: true,
	}

	cmd.Env = append(os.Environ(), extraEnv...)
	return cmd
}

// ensureDaemon starts the daemon if it is not already running and waits for it to become ready.
func ensureDaemon(logger *slog.Logger, conf config.Config, extraEnv []string) error {
	if daemon.IsRunning(conf) {
		return nil
	}

	logger.Info("daemon not running, starting it now...")
	if err := startDaemonBackground(logger, extraEnv); err != nil {
		return fmt.Errorf("could not start daemon: %w", err)
	}

	logger.Info("daemon process started, waiting for socket availability...")
	for range 20 {
		time.Sleep(250 * time.Millisecond)
		if daemon.IsRunning(conf) {
			break
		}
	}

	if !daemon.IsRunning(conf) {
		return fmt.Errorf("daemon failed to start (socket: %s)", conf.SocketPath())
	}

	logger.Info("daemon is ready")
	return nil
}

// daemonEnv builds environment variables for the background daemon process.
func daemonEnv(conf config.Config, opts daemonEnvOptions) []string {
	m := conf.ToEnvMap()

	m["MNEMONIC_GLOBAL_DIR"] = opts.GlobalDir
	m["MNEMONIC_LOCAL_DIR"] = opts.LocalDir

	if len(opts.Team) > 0 {
		m["MNEMONIC_TEAM_DIRS"] = strings.Join(opts.Team, ",")
	}

	if len(opts.Mandatory) > 0 {
		m["MNEMONIC_MANDATORY"] = strings.Join(opts.Mandatory, ",")
	}

	if !opts.IncludeServerAddr {
		delete(m, "MNEMONIC_SERVER_ADDR")
	}

	env := make([]string, 0, len(m))
	for k, v := range m {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	return env
}
