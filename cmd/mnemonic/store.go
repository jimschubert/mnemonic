package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/jimschubert/mnemonic/internal/config"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type StoreCmd struct {
	Query     QueryCmd     `cmd:"" help:"Query the memory store"`
	Add       AddCmd       `cmd:"" help:"Add an entry to the memory store"`
	ListHeads ListHeadsCmd `cmd:"" help:"List all memory heads in the store"`
}

//goland:noinspection GoUnhandledErrorResult
func socketSend(conf config.Config, tool string, payload map[string]any) (map[string]any, error) {
	httpClient := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, "unix", conf.SocketPath())
			},
		},
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "mnemonic-cli", Version: config.Version}, nil)
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{
		Endpoint:   "http://unix/mcp",
		HTTPClient: httpClient,
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("connecting to MCP server: %w", err)
	}
	defer session.Close() // nolint:errcheck

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      tool,
		Arguments: payload,
	})
	if err != nil {
		return nil, fmt.Errorf("calling query tool: %w", err)
	}

	if result.StructuredContent == nil {
		return nil, nil
	}
	return result.StructuredContent.(map[string]any), nil
}
