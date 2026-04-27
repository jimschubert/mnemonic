package main

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alecthomas/assert/v2"
	"github.com/jimschubert/mnemonic/internal/config"
)

func TestServerProxyReturnsBadGatewayWhenDaemonSocketMissing(t *testing.T) {
	t.Parallel()

	conf := config.Config{
		SocketPathRaw: filepath.Join(t.TempDir(), "missing.sock"),
	}

	proxy := &httputil.ReverseProxy{
		Rewrite: func(req *httputil.ProxyRequest) {
			req.Out.URL.Scheme = "http"
			req.Out.URL.Host = "localhost"
			req.Out.Host = "localhost"
		},
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, _ error) {
			http.Error(w, "daemon unavailable", http.StatusBadGateway)
		},
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return net.Dial("unix", conf.SocketPath())
			},
		},
	}

	req := httptest.NewRequest(http.MethodGet, "http://example.com/mcp", nil)
	w := httptest.NewRecorder()

	proxy.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadGateway, w.Code, "missing daemon socket should fail with bad gateway")
	assert.Contains(t, strings.ToLower(w.Body.String()), "daemon unavailable", "proxy failure should be visible to clients")
}
