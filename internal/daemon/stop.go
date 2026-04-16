package daemon

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/jimschubert/mnemonic/internal/config"
)

// sendShutdown posts a shutdown request to the daemon via Unix socket (tcpAddr == "") or TCP.
func sendShutdown(conf config.Config, tcpAddr string) error {
	var (
		client  *http.Client
		baseURL string
	)

	if tcpAddr != "" {
		client = &http.Client{Timeout: time.Duration(conf.ClientTimeout()) * time.Second}
		baseURL = "http://" + tcpAddr
	} else {
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

// isTCPRunning reports whether something is accepting TCP connections on conf.ServerAddr.
func isTCPRunning(conf config.Config) bool {
	if conf.ServerAddr == "" {
		return false
	}
	conn, err := net.DialTimeout("tcp", conf.ServerAddr, 200*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// RequestStop attempts a graceful shutdown via Unix socket then TCP, and verifies each transport
// is no longer reachable afterwards.
func RequestStop(conf config.Config, logger *log.Logger) error {
	socketSent := false
	tcpSent := false

	// try to stop unix socket
	if IsRunning(conf) {
		if err := sendShutdown(conf, ""); err != nil {
			logger.Printf("warning: socket shutdown request failed: %v", err)
		} else {
			logger.Printf("shutdown request sent via socket")
			socketSent = true
		}
	}

	// try to stop tcp
	if isTCPRunning(conf) {
		if err := sendShutdown(conf, conf.ServerAddr); err != nil {
			logger.Printf("warning: TCP shutdown request failed: %v", err)
		} else {
			logger.Printf("shutdown request sent via TCP")
			tcpSent = true
		}
	}

	if !socketSent && !tcpSent {
		return fmt.Errorf("daemon does not appear to be running (socket: %s, addr: %s)", conf.SocketPath(), conf.ServerAddr)
	}

	time.Sleep(300 * time.Millisecond)

	// check socket has stopped
	if IsRunning(conf) {
		logger.Printf("warning: socket still reachable (%s)", conf.SocketPath())
	} else {
		logger.Printf("socket: stopped")
	}

	// check tcp has stopped, if applicable
	if conf.ServerAddr != "" {
		if isTCPRunning(conf) {
			logger.Printf("warning: TCP still reachable (%s)", conf.ServerAddr)
		} else {
			logger.Printf("TCP: stopped (%s)", conf.ServerAddr)
		}
	}

	return nil
}
