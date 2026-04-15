package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/jimschubert/mnemonic/internal/config"
	"github.com/jimschubert/mnemonic/internal/server"
	"github.com/jimschubert/mnemonic/internal/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Daemon manages the YAML store and exposes it via MCP over a Unix socket (and optionally TCP HTTP).
type Daemon struct {
	store      store.Store
	conf       config.Config
	mcpServer  *mcp.Server
	unixSrv    *http.Server
	tcpSrv     *http.Server
	logger     *slog.Logger
	shutdownCh chan struct{}
	startedAt  time.Time
}

func New(s store.Store, conf config.Config) *Daemon {
	srv := server.NewServer(s, conf)
	return &Daemon{
		store:      s,
		conf:       conf,
		mcpServer:  srv.McpServer(),
		logger:     slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})),
		shutdownCh: make(chan struct{}),
		startedAt:  time.Now(),
	}
}

func (d *Daemon) Start(ctx context.Context) error {
	socketPath := d.conf.SocketPath()

	if err := os.MkdirAll(filepath.Dir(socketPath), 0o700); err != nil {
		return fmt.Errorf("creating socket directory: %w", err)
	}
	// Remove stale socket from a previous run.
	_ = os.Remove(socketPath)

	unixLn, err := net.Listen("unix", socketPath)
	if err != nil {
		return fmt.Errorf("listening on unix socket %s: %w", socketPath, err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/status", d.handleStatus)
	mux.HandleFunc("POST /api/shutdown", d.handleShutdown)
	mux.Handle("/mcp", mcp.NewStreamableHTTPHandler(
		func(_ *http.Request) *mcp.Server { return d.mcpServer },
		nil,
	))

	d.unixSrv = &http.Server{Handler: mux}

	errCh := make(chan error, 2)
	go func() {
		d.logger.Info("daemon listening", "socket", socketPath)
		if err := d.unixSrv.Serve(unixLn); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		} else {
			errCh <- nil
		}
	}()

	if d.conf.ServerAddr != "" {
		tcpLn, err := net.Listen("tcp", d.conf.ServerAddr)
		if err != nil {
			return fmt.Errorf("listening on tcp %s: %w", d.conf.ServerAddr, err)
		}
		d.tcpSrv = &http.Server{Handler: mux}
		go func() {
			d.logger.Info("daemon HTTP listening", "addr", d.conf.ServerAddr, "path", "/mcp")
			if err := d.tcpSrv.Serve(tcpLn); err != nil && !errors.Is(err, http.ErrServerClosed) {
				errCh <- err
			} else {
				errCh <- nil
			}
		}()
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	select {
	case sig := <-sigCh:
		d.logger.Info("signal received, shutting down", "signal", sig)
	case <-d.shutdownCh:
		d.logger.Info("shutdown requested via API")
	case <-ctx.Done():
		d.logger.Info("context cancelled, shutting down")
	case err := <-errCh:
		return err
	}

	return d.shutdown()
}

func (d *Daemon) shutdown() error {
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if d.tcpSrv != nil {
		if err := d.tcpSrv.Shutdown(shutdownCtx); err != nil {
			d.logger.Warn("tcp server shutdown error", "err", err)
		}
	}

	if err := d.unixSrv.Shutdown(shutdownCtx); err != nil {
		d.logger.Warn("unix server shutdown error", "err", err)
	}

	if c, ok := d.store.(io.Closer); ok {
		if err := c.Close(); err != nil {
			d.logger.Warn("store close error", "err", err)
		}
	}

	_ = os.Remove(d.conf.SocketPath())
	return nil
}

func (d *Daemon) handleStatus(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":    "healthy",
		"timestamp": time.Now().Unix(),
		"version":   config.Version,
		"uptime":    time.Since(d.startedAt).String(),
	})
}

func (d *Daemon) handleShutdown(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusAccepted)
	// only allows call once
	select {
	case <-d.shutdownCh:
	default:
		close(d.shutdownCh)
	}
}
