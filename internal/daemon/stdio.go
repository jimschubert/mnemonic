package daemon

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/jimschubert/mnemonic/internal/config"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func dialSocketContext(conf config.Config) func(ctx context.Context, _, _ string) (net.Conn, error) {
	return func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", conf.SocketPath())
	}
}

// RunStdioServer connects to the daemon's MCP endpoint over the Unix socket and proxies it to stdio.
// Tools are queried from session then forwarded to avoid duplicating the implementation here and in the daemon.
func RunStdioServer(ctx context.Context, conf config.Config) error {
	if !IsRunning(conf) {
		return fmt.Errorf("mnemonic daemon is not running (socket: %s)", conf.SocketPath())
	}

	httpClient := &http.Client{Transport: &http.Transport{DialContext: dialSocketContext(conf)}}
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "mnemonic-stdio", Version: config.Version}, nil)

	session, err := mcpClient.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint:   "http://unix/mcp",
		HTTPClient: httpClient,
	}, nil)
	if err != nil {
		return fmt.Errorf("connecting to daemon MCP: %w", err)
	}
	defer func() {
		_ = session.Close()
	}()

	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		return fmt.Errorf("listing daemon tools: %w", err)
	}

	// proxy server forwards all tool calls through to the daemon
	proxySrv := mcp.NewServer(&mcp.Implementation{Name: "mnemonic", Version: config.Version}, nil)
	for _, t := range tools.Tools {
		proxySrv.AddTool(t, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return session.CallTool(ctx, &mcp.CallToolParams{
				Name:      req.Params.Name,
				Arguments: req.Params.Arguments,
			})
		})
	}

	// this sets up a 10s polling loop to see if the daemon died, so the stdio server doesn't remain open indefinitely.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		interval := 10 * time.Second
		if i, err := time.ParseDuration(os.Getenv("MNEMONIC_POLL_INTERVAL")); err == nil {
			interval = i
		}
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				resp, err := httpClient.Get("http://unix/api/status")
				if err != nil {
					cancel()
					return
				}
				_ = resp.Body.Close()
				if resp.StatusCode != 200 {
					cancel()
					return
				}
			}
		}
	}()

	return proxySrv.Run(ctx, &mcp.StdioTransport{})
}
