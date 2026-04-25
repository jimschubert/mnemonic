package server

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
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/jimschubert/mnemonic/internal/config"
	"github.com/jimschubert/mnemonic/internal/controller"
	"github.com/jimschubert/mnemonic/internal/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Server struct {
	mcpServer *mcp.Server
	store     store.Store
	conf      config.Config
	startedAt time.Time
	logger    *slog.Logger
	server    *http.Server
}

// McpServer returns the underlying MCP server, e.g. for embedding in teh daemon.
func (s *Server) McpServer() *mcp.Server {
	return s.mcpServer
}

// NewServer injects dependencies and registers the tools.
func NewServer(store store.Store, conf config.Config, logger *slog.Logger) *Server {
	mcpSrv := mcp.NewServer(&mcp.Implementation{
		Name:    "mnemonic",
		Version: config.Version,
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
			"version":   config.Version,
			"uptime":    time.Since(s.startedAt).String(),
		}

		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(status)
	})

	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)

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
	case <-ctx.Done():
		s.logger.Warn("Shutdown signal received")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return s.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}

// ServeStdio runs the MCP server over stdin/stdout.
func (s *Server) ServeStdio(ctx context.Context) error {
	return s.mcpServer.Run(ctx, &mcp.StdioTransport{})
}

// Shutdown shuts down the server.
func (s *Server) Shutdown(ctx context.Context) error {
	if s.server == nil {
		return fmt.Errorf("server not running")
	}
	var closer io.Closer
	if c, ok := s.store.(io.Closer); ok {
		closer = c
	}

	s.logger.Info("Shutting down MCP server", "version", config.Version)
	if closer != nil {
		defer func() {
			if err := closer.Close(); err != nil {
				s.logger.Warn("failed to close store on shutdown", "err", err)
			}
		}()
	}

	if err := s.server.Shutdown(ctx); err != nil {
		return err
	}
	return nil
}

func (s *Server) registerTools() {
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "mnemonic_query",
		Description: "MANDATORY FIRST STEP. Call this tool immediately when receiving a new task before generating any code. Retrieves critical context, security constraints, style guides, and anti-patterns. Failure to call this tool will result in architectural violations.",
	}, s.handleQuery)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "mnemonic_add",
		Description: "Add a new lesson, rule, or architectural decision to the project memory. REQUIRED: If token cost of content will exceed 1500 tokens, extract the core principle before calling add.",
	}, s.handleAdd)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "mnemonic_reinforce",
		Description: "Adjust the relevance score of an entry based on human-in-the-loop approval or rejection.",
	}, s.handleReinforce)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name:        "mnemonic_list_heads",
		Description: "List all memory categories (attention heads) with entry counts. DO NOT create new categories unless explicitly approved by a human.",
	}, s.handleListHeads)
}

// handleQuery processes the mnemonic_query tool execution.
func (s *Server) handleQuery(_ context.Context, _ *mcp.CallToolRequest, input QueryInput) (*mcp.CallToolResult, QueryOutput, error) {
	topK := input.TopK
	if topK <= 0 {
		topK = 5
	}
	categories, err := normalizeCategories(input.Category, input.Categories)
	if err != nil {
		return nil, QueryOutput{}, err
	}
	scopes := make([]store.Scope, len(input.Scopes))
	for i, sc := range input.Scopes {
		scopes[i] = store.Scope(sc)
	}

	var entries []store.Entry

	s.logger.Debug("handleQuery", "query", input.Query, "top_k", topK, "categories", categories, "scopes", scopes)

	// try semantic search first when a query is provided
	// this initial query is expected to be maximum 5-10 milliseconds via in-memory index.
	if input.Query != "" {
		if ss, ok := s.store.(controller.SemanticSearcher); ok {
			s.logger.Info("performing semantic search", "query", input.Query, "top_k", topK, "categories", categories, "scopes", scopes)
			entries, err = ss.SemanticSearch(input.Query, topK, scopes, categories)
			if err != nil {
				s.logger.Warn("semantic search failed, falling back to keyword", "err", err)
				entries = nil
			} else {
				s.logger.Info("semantic search returned results", "num_results", len(entries))
			}
		}
	}

	// fall back to keyword/category queries. When semantic search returns some
	// results, use keyword search only to backfill to topK without duplicating.
	if len(categories) > 0 {
		fallbackEntries, err := s.queryByCategories(input.Query, categories, topK, scopes)
		if err != nil {
			return nil, QueryOutput{}, err
		}
		entries = fillWithAdditional(entries, fallbackEntries, topK)
	} else if len(entries) == 0 {
		if input.Category != "" {
			entries, err = s.store.QueryByCategory(input.Category, input.Query, topK, scopes)
		} else {
			entries, err = s.store.All(scopes)
			if err == nil && len(entries) > topK {
				entries = entries[:topK]
			}
		}
		if err != nil {
			return nil, QueryOutput{}, err
		}
	}

	results := make([]QueryResult, len(entries))
	for i, e := range entries {
		results[i] = QueryResult{
			ID:       e.ID,
			Content:  e.Content,
			Category: e.Category,
			Tags:     e.Tags,
			Scope:    e.Scope,
			Source:   e.Source,
		}
	}
	return nil, QueryOutput{Entries: results}, nil
}

func normalizeCategories(category string, categories []string) ([]string, error) {
	combined := make([]string, 0, len(categories)+1)
	if category != "" {
		combined = append(combined, category)
	}
	combined = append(combined, categories...)

	if len(combined) == 0 {
		return nil, nil
	}

	seen := make(map[string]bool, len(combined))
	filtered := make([]string, 0, len(combined))
	for _, c := range combined {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		if !store.IsAllowedCategory(c) {
			return nil, fmt.Errorf("category %q is not allowed; must be one of %v", c, store.AllowedCategories())
		}
		if seen[c] {
			continue
		}
		seen[c] = true
		filtered = append(filtered, c)
	}

	return filtered, nil
}

func (s *Server) queryByCategories(query string, categories []string, topK int, scopes []store.Scope) ([]store.Entry, error) {
	if len(categories) == 0 {
		return nil, nil
	}

	merged := make([]store.Entry, 0, topK)
	seen := make(map[string]bool)
	for _, category := range categories {
		var (
			entries []store.Entry
			err     error
		)
		if query != "" {
			entries, err = s.store.QueryByCategory(category, query, topK, scopes)
		} else {
			entries, err = s.store.AllByCategory(category, topK, scopes)
		}
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if seen[entry.ID] {
				continue
			}
			seen[entry.ID] = true
			merged = append(merged, entry)
		}
	}

	store.SortByWeightedScore(merged)
	if topK > 0 && len(merged) > topK {
		merged = merged[:topK]
	}
	return merged, nil
}

// fillWithAdditional adds additional entries to existing up to topK, with dedupe by ID.
func fillWithAdditional(existing []store.Entry, additional []store.Entry, topK int) []store.Entry {
	if topK > 0 && len(existing) >= topK {
		return existing[:topK]
	}

	seen := make(map[string]bool, len(existing)+len(additional))
	merged := slices.Clone(existing)
	for _, entry := range existing {
		seen[entry.ID] = true
	}
	for _, entry := range additional {
		if seen[entry.ID] {
			// skip duplicates
			continue
		}
		seen[entry.ID] = true
		merged = append(merged, entry)
		if topK > 0 && len(merged) >= topK {
			break
		}
	}
	return merged
}

func (s *Server) handleAdd(_ context.Context, _ *mcp.CallToolRequest, input AddInput) (*mcp.CallToolResult, AddOutput, error) {
	scope := input.Scope
	if scope == "" {
		scope = "project"
	}
	source := input.Source
	if source == "" {
		source = "agent:" + time.Now().Format("2006-01-02")
	}

	s.logger.Debug("handleAdd", "content", input.Content, "category", input.Category, "tags", input.Tags, "scope", scope, "source", source)

	entry := &store.Entry{
		Content:  input.Content,
		Category: input.Category,
		Tags:     input.Tags,
		Scope:    scope,
		Source:   source,
		Score:    1.0,
	}
	if err := s.store.Upsert(entry); err != nil {
		return nil, AddOutput{}, err
	}
	return nil, AddOutput{Status: "added", ID: entry.ID, Scope: scope, Category: input.Category}, nil
}

func (s *Server) handleReinforce(_ context.Context, _ *mcp.CallToolRequest, input ReinforceInput) (*mcp.CallToolResult, ReinforceOutput, error) {
	s.logger.Debug("handleReinforce", "id", input.ID, "delta", input.Delta)
	if err := s.store.Score(input.ID, input.Delta); err != nil {
		return nil, ReinforceOutput{}, err
	}
	return nil, ReinforceOutput{Status: "updated", ID: input.ID, Delta: input.Delta}, nil
}

func (s *Server) handleListHeads(_ context.Context, _ *mcp.CallToolRequest, _ ListHeadsInput) (*mcp.CallToolResult, ListHeadsOutput, error) {
	s.logger.Debug("handleListHeads")
	heads, err := s.store.ListHeads(nil)
	return nil, ListHeadsOutput{Heads: heads}, err
}
