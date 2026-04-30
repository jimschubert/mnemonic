package daemon

import (
	"context"
	"fmt"
	"strings"

	"github.com/jimschubert/mnemonic/internal/config"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// RunStdioServer connects to the daemon's MCP endpoint over the Unix socket and proxies it to stdio.
// Tools are queried from session then forwarded to avoid duplicating the implementation here and in the daemon.
func RunStdioServer(ctx context.Context, conf config.Config) error {
	if !IsRunning(conf) {
		return fmt.Errorf("mnemonic daemon is not running (socket: %s)", conf.SocketPath())
	}

	httpClient := NewSocketClient(conf)
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

	// Watch the daemon; cancel ctx when it stops or restarts.
	ctx, cancel := WatchDaemon(ctx, httpClient, conf.PollInterval())
	defer cancel()

	// proxy server forwards all tool calls through to the daemon
	proxySrv := mcp.NewServer(&mcp.Implementation{Name: "mnemonic", Version: config.Version}, nil)
	for _, t := range tools.Tools {
		proxySrv.AddTool(t, func(callCtx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			res, err := session.CallTool(callCtx, &mcp.CallToolParams{
				Name:      req.Params.Name,
				Arguments: req.Params.Arguments,
			})
			if err != nil && strings.Contains(err.Error(), "session not found") {
				cancel()
			}
			return res, err
		})
	}

	return proxySrv.Run(ctx, &mcp.StdioTransport{})
}
