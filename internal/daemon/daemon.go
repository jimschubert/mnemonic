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
	"github.com/jimschubert/mnemonic/internal/logging"
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

func New(s store.Store, conf config.Config, logger *slog.Logger) *Daemon {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}

	srv := server.NewServer(s, conf, logging.ForScope(conf, "server"))
	return &Daemon{
		store:      s,
		conf:       conf,
		mcpServer:  srv.McpServer(),
		logger:     logger,
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
	mux.HandleFunc("GET /api/admin/entries", d.handleAdminEntries)
	mux.HandleFunc("GET /api/admin/entries/{id}", d.handleAdminEntryGet)
	mux.HandleFunc("PUT /api/admin/entries/{id}", d.handleAdminEntryUpdate)
	mux.HandleFunc("DELETE /api/admin/entries/{id}", d.handleAdminEntryDelete)
	mux.HandleFunc("GET /api/admin/heads", d.handleAdminHeads)
	mux.HandleFunc("POST /api/admin/entries/merge", d.handleAdminMerge)
	mux.Handle("/mcp", mcp.NewStreamableHTTPHandler(
		func(_ *http.Request) *mcp.Server { return d.mcpServer },
		nil,
	))

	d.unixSrv = &http.Server{Handler: mux}

	errCh := make(chan error, 2)
	numServers := 1
	go func() {
		d.logger.Info("daemon listening", "socket", socketPath)
		if err := d.unixSrv.Serve(unixLn); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		} else {
			errCh <- nil
		}
	}()

	// TBD: server and daemon subcommands no longer support this flow…
	// determine whether to keep or remove (later).
	// Similarly: shutdown no longer shuts down servers, which will fail until the daemon comes back up.
	// May need to introduce a `--broadcast` flag to the stop command so we can stop all servers.
	if d.conf.ServerAddr != "" {
		numServers++
		tcpLn, err := net.Listen("tcp", d.conf.ServerAddr)
		if err != nil {
			return fmt.Errorf("listening on tcp %s: %w", d.conf.ServerAddr, err)
		}
		tcpHandler := server.TCPHandlerFromConfig(mux, d.conf.AuthToken, d.conf.AllowedOrigins, d.conf.UnauthenticatedStatus)
		d.tcpSrv = &http.Server{Handler: tcpHandler}
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
		d.logger.Info("context canceled, shutting down")
	case err := <-errCh:
		return err
	}

	if err := d.shutdown(); err != nil {
		return err
	}

	var errs error

	for i := 0; i < numServers; i++ {
		errs = errors.Join(errs, <-errCh)
	}

	return errs
}

func (d *Daemon) shutdown() error {
	var errs error

	if d.tcpSrv != nil {
		if err := d.shutdownHTTPServer("tcp", d.tcpSrv, 5*time.Second); err != nil {
			errs = errors.Join(errs, err)
		}
	}

	if err := d.shutdownHTTPServer("unix", d.unixSrv, 5*time.Second); err != nil {
		errs = errors.Join(errs, err)
	}

	if c, ok := d.store.(io.Closer); ok {
		if err := c.Close(); err != nil {
			d.logger.Warn("store close error", "err", err)
			errs = errors.Join(errs, err)
		}
	}

	if err := os.Remove(d.conf.SocketPath()); err != nil && !errors.Is(err, os.ErrNotExist) {
		errs = errors.Join(errs, err)
	}

	return errs
}

func (d *Daemon) shutdownHTTPServer(name string, srv *http.Server, timeout time.Duration) error {
	if srv == nil {
		return nil
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		d.logger.Warn("graceful server shutdown failed; forcing close", "server", name, "err", err)

		if closeErr := srv.Close(); closeErr != nil && !errors.Is(closeErr, http.ErrServerClosed) {
			return errors.Join(err, closeErr)
		}
	}

	return nil
}

func (d *Daemon) handleStatus(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":     "healthy",
		"timestamp":  time.Now().Unix(),
		"version":    config.Version,
		"uptime":     time.Since(d.startedAt).String(),
		"started_at": d.startedAt.Unix(),
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
