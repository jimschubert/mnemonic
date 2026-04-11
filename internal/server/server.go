package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jimschubert/mnemonic/internal/config"
	"github.com/jimschubert/mnemonic/internal/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var serverVersion = "0.1.0"

type Server struct {
	mcpServer *mcp.Server
	store     store.Store
	conf      config.Config
	startedAt time.Time
	logger    *slog.Logger
	server    *http.Server
}

// NewServer injects dependencies and registers the tools.
func NewServer(store store.Store, conf config.Config) *Server {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	mcpSrv := mcp.NewServer(&mcp.Implementation{
		Name:    "mnemonic",
		Version: serverVersion,
	}, &mcp.ServerOptions{Logger: logger})

	s := &Server{
		mcpServer: mcpSrv,
		store:     store,
		conf:      conf,
		startedAt: time.Now(),
		logger:    logger,
	}

	s.registerTools()
	return s
}

func (s *Server) Serve(ctx context.Context) error {
	ctx = config.StoreMcpAddress(ctx, s.conf.ServerAddr)
	ctx = config.StoreServerVersion(ctx, serverVersion)

	mux := http.NewServeMux()

	s.server = &http.Server{Handler: mux}

	mux.Handle("/mcp", mcp.NewStreamableHTTPHandler(
		func(_ *http.Request) *mcp.Server { return s.mcpServer },
		&mcp.StreamableHTTPOptions{Logger: s.logger},
	))

	mux.HandleFunc("/api/status", func(writer http.ResponseWriter, request *http.Request) {
		// taken from https://mcp-go.dev/transports/http/
		writer.WriteHeader(http.StatusOK)
		status := map[string]any{
			"status":    "healthy",
			"timestamp": time.Now().Unix(),
			"version":   serverVersion,
			"uptime":    time.Since(s.startedAt).String(),
		}

		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(status)
	})

	errCh := make(chan error, 1)
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigChan)

	// listen _first_ so we don't fail in goroutine
	ln, err := net.Listen("tcp", s.conf.ServerAddr)
	if err != nil {
		s.logger.Warn("Failed to listen on address", "address", s.conf.ServerAddr, "error", err)
		return fmt.Errorf("cannot bind MCP address: %s %w", s.conf.ServerAddr, err)
	}

	go func() {
		s.logger.Info("MCP server listening", "addr", s.conf.ServerAddr, "path", "/mcp")
		if err := s.server.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.logger.Error("MCP HTTP server error", "err", err)
		} else {
			errCh <- err
		}
	}()

	select {
	case sig := <-sigChan:
		s.logger.Warn("Signal received", "signal", sig)
		// gracefully shutdown
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return s.Shutdown(shutdownCtx)
	case <-ctx.Done():
		// gracefully shutdown
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return s.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}

// Shutdown gracefully shuts down the server.
func (s *Server) Shutdown(ctx context.Context) error {
	if s.server == nil {
		return fmt.Errorf("server not running")
	}
	return s.server.Shutdown(ctx)
}

func (s *Server) registerTools() {
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "mnemonic_query",
		Description: "Retrieve relevant project rules, style guides, and lessons-learned based on the current task.",
	}, s.handleQuery)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "mnemonic_add",
		Description: "Add a new lesson, rule, or architectural decision to the project memory.",
	}, s.handleAdd)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "mnemonic_reinforce",
		Description: "Adjust the relevance score of an entry based on human-in-the-loop approval or rejection.",
	}, s.handleReinforce)
}

// handleQuery processes the mnemonic_query tool execution.
func (s *Server) handleQuery(ctx context.Context, req *mcp.CallToolRequest, input QueryInput) (*mcp.CallToolResult, QueryOutput, error) {
	return nil, QueryOutput{}, nil
}

func (s *Server) handleAdd(ctx context.Context, req *mcp.CallToolRequest, input AddInput) (*mcp.CallToolResult, AddOutput, error) {
	return nil, AddOutput{}, nil
}

func (s *Server) handleReinforce(ctx context.Context, req *mcp.CallToolRequest, input ReinforceInput) (*mcp.CallToolResult, ReinforceOutput, error) {
	return nil, ReinforceOutput{}, nil
}
