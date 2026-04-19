package daemon

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alecthomas/assert/v2"
	"github.com/jimschubert/mnemonic/internal/config"
	"github.com/jimschubert/mnemonic/internal/logging"
	"github.com/jimschubert/mnemonic/internal/store"
)

func waitForCondition(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}

	t.Fatalf("condition not met within %s", timeout)
}

func shortSocketPath(t *testing.T) string {
	t.Helper()

	path := filepath.Join(os.TempDir(), fmt.Sprintf("mnemonic-%d.sock", time.Now().UnixNano()))
	t.Cleanup(func() {
		_ = os.Remove(path)
	})

	return path
}

func TestDaemonStart_ShutdownRequestStopsServer(t *testing.T) {
	conf := config.Config{
		SocketPathRaw:    shortSocketPath(t),
		ClientTimeoutSec: 1,
	}

	logger := logging.New(slog.LevelInfo)
	d := New(&store.NoopStore{}, conf, logger)
	errCh := make(chan error, 1)

	go func() {
		errCh <- d.Start(context.Background())
	}()

	waitForCondition(t, time.Second, func() bool {
		return IsRunning(conf)
	})

	assert.NoError(t, sendShutdown(conf, ""))

	select {
	case err := <-errCh:
		assert.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("daemon did not stop after shutdown request")
	}

	assert.False(t, IsRunning(conf))
}
