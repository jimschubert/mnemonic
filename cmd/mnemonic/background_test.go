package main

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/alecthomas/assert/v2"
	"github.com/jimschubert/mnemonic/internal/config"
)

func TestNewDaemonCommandDetached(t *testing.T) {
	t.Parallel()

	extraEnv := []string{"MNEMONIC_TEST_FLAG=1", "MNEMONIC_TEST_NAME=daemon"}
	cmd := newDaemonCommand("/tmp/mnemonic-test-binary", extraEnv)

	assert.Equal(t, []string{"/tmp/mnemonic-test-binary", "daemon"}, cmd.Args)
	assert.NotZero(t, cmd.SysProcAttr, "SysProcAttr should be configured to detach the daemon from the parent terminal")
	assert.True(t, cmd.SysProcAttr.Setsid, "Setsid should be enabled so the daemon starts in a new session")
	assert.Zero(t, cmd.Stdin)
	assert.Equal(t, io.Writer(os.Stderr), cmd.Stdout, "daemon stdout should be visible in the parent stderr")
	assert.Equal(t, io.Writer(os.Stderr), cmd.Stderr, "daemon stderr should be visible in the parent stderr")
	assert.Equal(t, extraEnv, cmd.Env[len(cmd.Env)-len(extraEnv):])
}

func TestDaemonEnv(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		conf   config.Config
		opts   daemonEnvOptions
		want   map[string]string
		absent []string
	}{
		{
			name: "includes command values and server address for stdio",
			conf: config.Config{
				LogLevel:      "debug",
				ServerAddr:    "localhost:20001",
				SocketPathRaw: "/tmp/mnemonic.sock",
			},
			opts: daemonEnvOptions{
				GlobalDir:         "/Users/me/.mnemonic",
				LocalDir:          ".mnemonic",
				Team:              []string{"/repos/team-a", "/repos/team-b"},
				Mandatory:         []string{"avoidance", "security"},
				IncludeServerAddr: true,
			},
			want: map[string]string{
				"LOG_LEVEL":                          "debug",
				"MNEMONIC_SERVER_ADDR":               "localhost:20001",
				"MNEMONIC_SOCKET_PATH":               "/tmp/mnemonic.sock",
				"MNEMONIC_GLOBAL_DIR":                "/Users/me/.mnemonic",
				"MNEMONIC_LOCAL_DIR":                 ".mnemonic",
				"MNEMONIC_TEAM_DIRS":                 "/repos/team-a,/repos/team-b",
				"MNEMONIC_MANDATORY":                 "avoidance,security",
				"MNEMONIC_EMBEDDINGS_SKIP_PREFLIGHT": "false",
				"MNEMONIC_UNAUTHENTICATED_STATUS":    "false",
			},
		},
		{
			name: "drops server address for daemon proxy",
			conf: config.Config{
				ServerAddr: "localhost:20001",
			},
			opts: daemonEnvOptions{
				GlobalDir:         "~/.mnemonic",
				LocalDir:          ".mnemonic",
				IncludeServerAddr: false,
			},
			want: map[string]string{
				"MNEMONIC_GLOBAL_DIR":                "~/.mnemonic",
				"MNEMONIC_LOCAL_DIR":                 ".mnemonic",
				"MNEMONIC_EMBEDDINGS_SKIP_PREFLIGHT": "false",
				"MNEMONIC_UNAUTHENTICATED_STATUS":    "false",
			},
			absent: []string{"MNEMONIC_SERVER_ADDR"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := envSliceToMap(daemonEnv(tt.conf, tt.opts))
			for key, want := range tt.want {
				assert.Equal(t, want, got[key], key)
			}
			for _, key := range tt.absent {
				_, ok := got[key]
				assert.False(t, ok, key)
			}
		})
	}
}

func envSliceToMap(env []string) map[string]string {
	result := make(map[string]string, len(env))
	for _, item := range env {
		key, value, ok := strings.Cut(item, "=")
		if !ok {
			continue
		}
		result[key] = value
	}
	return result
}
