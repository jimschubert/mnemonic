package config

import "context"

var serverKey = configCtxKey{name: "serverVersion"}
var mcpAddrKey = configCtxKey{name: "mcpAddress"}

// configCtxKey stores individual values on context
// TODO: consider maybe storing everything in *Config and setting that on context?
type configCtxKey struct {
	name string
}

func StoreMcpAddress(ctx context.Context, addr string) context.Context {
	ctx = context.WithValue(ctx, mcpAddrKey, addr)
	return ctx
}

func GetMcpAddress(ctx context.Context) string {
	if v := ctx.Value(mcpAddrKey); v != nil {
		if value, ok := v.(string); ok {
			return value
		}
	}
	return ""
}

func StoreServerVersion(ctx context.Context, version string) context.Context {
	ctx = context.WithValue(ctx, serverKey, version)
	return ctx
}

func GetServerVersion(ctx context.Context) string {
	if v := ctx.Value(serverKey); v != nil {
		if value, ok := v.(string); ok {
			return value
		}
	}
	return ""
}
