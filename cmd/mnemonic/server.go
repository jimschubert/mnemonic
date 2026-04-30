package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jimschubert/mnemonic/internal/config"
	"github.com/jimschubert/mnemonic/internal/daemon"
	"github.com/jimschubert/mnemonic/internal/server"
	"github.com/muesli/reflow/wordwrap"
)

// ServerCmd starts the MCP server as a stateless proxy to the background daemon.
type ServerCmd struct {
	GlobalDir             string   `short:"g" default:"~/.mnemonic" help:"Directory for global data" env:"MNEMONIC_GLOBAL_DIR"`
	LocalDir              string   `short:"l" default:".mnemonic" help:"Directory for project data" env:"MNEMONIC_LOCAL_DIR"`
	Team                  []string `short:"t" help:"Team data directories (repeatable); scope will become team:<basename>" env:"MNEMONIC_TEAM_DIRS" sep:","`
	ServerAddr            string   `short:"a" default:"${server_addr}" help:"Address to listen on for MCP requests"  env:"MNEMONIC_SERVER_ADDR"`
	Mandatory             []string `help:"Additional mandatory categories beyond the defaults (avoidance, security)" env:"MNEMONIC_MANDATORY" sep:","`
	AuthToken             string   `help:"Bearer token required for all TCP HTTP requests (empty = no auth)" env:"MNEMONIC_AUTH_TOKEN"`
	AllowedOrigins        []string `help:"Allowed CORS origins; use * to permit any origin" env:"MNEMONIC_ALLOWED_ORIGINS" sep:","`
	UnauthenticatedStatus bool     `help:"Allow unauthenticated access to GET /api/status" env:"MNEMONIC_UNAUTHENTICATED_STATUS"`
	SkipIndexSync         bool     `help:"Skip initial index sync on startup; use when restarting or invoking embedding manually" env:"MNEMONIC_SKIP_INDEX_SYNC"`

	Embedding embeddable `embed:"" prefix:"embedding-"`
}

func (c *ServerCmd) Help() string {
	help := `
The MCP server as a stateless proxy to the background daemon. The daemon starts automatically if not running.
The server can be terminated without impacting the daemon or YAML store. To stop the daemon, use 'mnemonic daemon stop'.
	
Multiple servers can be attached to the same daemon, allowing for different ports, auth tokens, etc.
`
	return wordwrap.String(help, 80)
}

func (c *ServerCmd) Run(logger *slog.Logger, conf config.Config) error {
	c.Embedding.applyConfig(&conf)
	conf.ApplyOverrides(config.Config{
		AuthToken:             c.AuthToken,
		AllowedOrigins:        c.AllowedOrigins,
		UnauthenticatedStatus: c.UnauthenticatedStatus,
	})

	// explicitly assign because conf.ApplyOverrides ignores empty strings
	conf.ServerAddr = c.ServerAddr

	extraEnv := daemonEnv(conf, daemonEnvOptions{
		GlobalDir:         c.GlobalDir,
		LocalDir:          c.LocalDir,
		Team:              c.Team,
		Mandatory:         c.Mandatory,
		IncludeServerAddr: false,
		SkipIndexSync:     c.SkipIndexSync,
	})

	if err := ensureDaemon(logger, conf, extraEnv); err != nil {
		return fmt.Errorf("ensuring daemon: %w", err)
	}

	watchCtx, watchCancel := daemon.WatchDaemon(context.Background(), daemon.NewSocketClient(conf), conf.PollInterval())
	defer watchCancel()

	proxy := &httputil.ReverseProxy{
		Rewrite: func(req *httputil.ProxyRequest) {
			req.Out.URL.Scheme = "http"
			// Host is ignored because of the custom dialer, but some HTTP clients require it to be non-empty
			req.Out.URL.Host = "localhost"
			req.Out.Host = "localhost"
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			logger.Warn("proxy request failed", "path", r.URL.Path, "err", err)
			http.Error(w, "daemon unavailable", http.StatusBadGateway)
		},
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return net.Dial("unix", conf.SocketPath())
			},
		},
	}

	mux := http.NewServeMux()
	mux.Handle("/mcp", proxy)
	mux.Handle("/api/status", proxy)
	mux.Handle("/api/shutdown", proxy)

	tcpHandler := server.TCPHandlerFromConfig(mux, conf.AuthToken, conf.AllowedOrigins, conf.UnauthenticatedStatus)
	tcpSrv := &http.Server{Handler: tcpHandler}

	tcpLn, err := net.Listen("tcp", conf.ServerAddr)
	if err != nil {
		return fmt.Errorf("listening on tcp %s: %w", conf.ServerAddr, err)
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("stateless proxy listening", "addr", conf.ServerAddr, "target_socket", conf.SocketPath())
		if err := tcpSrv.Serve(tcpLn); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		} else {
			errCh <- nil
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	select {
	case sig := <-sigCh:
		logger.Info("signal received, shutting down proxy", "signal", sig)
	case <-watchCtx.Done():
		logger.Info("daemon stopped, shutting down proxy")
	case err := <-errCh:
		return err
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := tcpSrv.Shutdown(shutdownCtx); err != nil {
		logger.Warn("graceful proxy shutdown failed; forcing close", "err", err)
		_ = tcpSrv.Close()
	}

	return nil
}
