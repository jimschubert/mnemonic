package daemon

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/jimschubert/mnemonic/internal/config"
)

// RequestStop sends a graceful shutdown request to the daemon.
// It targets the Unix socket by default. Pass a non-empty tcpAddr to stop over TCP/HTTP instead.
func RequestStop(conf config.Config, tcpAddr string) error {
	var (
		client  *http.Client
		baseURL string
	)

	if tcpAddr != "" {
		client = &http.Client{Timeout: time.Duration(conf.ClientTimeout()) * time.Second}
		baseURL = "http://" + tcpAddr
	} else {
		if !IsRunning(conf) {
			return fmt.Errorf("daemon is not running (socket: %s)", conf.SocketPath())
		}
		socketPath := conf.SocketPath()
		client = &http.Client{
			Timeout: time.Duration(conf.ClientTimeout()) * time.Second,
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
					return (&net.Dialer{}).DialContext(ctx, "unix", socketPath)
				},
			},
		}
		baseURL = "http://unix"
	}

	resp, err := client.Post(baseURL+"/api/shutdown", "application/json", nil)
	if err != nil {
		return fmt.Errorf("sending shutdown request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("unexpected status from daemon: %s", resp.Status)
	}
	return nil
}
