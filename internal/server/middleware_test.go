package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alecthomas/assert/v2"
)

func TestBearerAuth(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	tests := []struct {
		name        string
		token       string
		authHeader  string
		path        string
		exemptPaths []string
		wantStatus  int
	}{
		{
			name:       "no token configured is passthrough",
			token:      "",
			authHeader: "",
			wantStatus: http.StatusOK,
		},
		{
			name:       "valid bearer token",
			token:      "secret",
			authHeader: "Bearer secret",
			wantStatus: http.StatusOK,
		},
		{
			name:       "missing auth header",
			token:      "secret",
			authHeader: "",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "wrong token",
			token:      "secret",
			authHeader: "Bearer wrongtoken",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "wrong scheme",
			token:      "secret",
			authHeader: "Basic secret",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:        "exempt path bypasses auth",
			token:       "secret",
			authHeader:  "",
			path:        "/api/status",
			exemptPaths: []string{"/api/status"},
			wantStatus:  http.StatusOK,
		},
		{
			name:        "non-exempt path still requires auth",
			token:       "secret",
			authHeader:  "",
			path:        "/mcp",
			exemptPaths: []string{"/api/status"},
			wantStatus:  http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := tt.path
			if path == "" {
				path = "/mcp"
			}
			req := httptest.NewRequest(http.MethodGet, path, nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			w := httptest.NewRecorder()
			BearerAuth(tt.token, tt.exemptPaths...)(next).ServeHTTP(w, req)
			assert.Equal(t, tt.wantStatus, w.Code)
		})
	}
}

func TestCORS(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	tests := []struct {
		name           string
		allowedOrigins []string
		origin         string
		method         string
		wantStatus     int
		wantACAO       string // Access-Control-Allow-Origin
	}{
		{
			name:           "no origins configured is passthrough",
			allowedOrigins: nil,
			origin:         "http://evil.example.com",
			wantStatus:     http.StatusOK,
			wantACAO:       "",
		},
		{
			name:           "exact origin match",
			allowedOrigins: []string{"http://localhost:3000"},
			origin:         "http://localhost:3000",
			wantStatus:     http.StatusOK,
			wantACAO:       "http://localhost:3000",
		},
		{
			name:           "origin not in list gets no header",
			allowedOrigins: []string{"http://localhost:3000"},
			origin:         "http://evil.example.com",
			wantStatus:     http.StatusOK,
			wantACAO:       "",
		},
		{
			name:           "wildcard allows any origin",
			allowedOrigins: []string{"*"},
			origin:         "http://anything.example.com",
			wantStatus:     http.StatusOK,
			wantACAO:       "*",
		},
		{
			name:           "preflight OPTIONS returns 204",
			allowedOrigins: []string{"http://localhost:3000"},
			origin:         "http://localhost:3000",
			method:         http.MethodOptions,
			wantStatus:     http.StatusNoContent,
			wantACAO:       "http://localhost:3000",
		},
		{
			name:           "preflight with wildcard returns 204",
			allowedOrigins: []string{"*"},
			origin:         "http://anything.example.com",
			method:         http.MethodOptions,
			wantStatus:     http.StatusNoContent,
			wantACAO:       "*",
		},
		{
			name:           "no Origin header skips CORS headers",
			allowedOrigins: []string{"http://localhost:3000"},
			origin:         "",
			wantStatus:     http.StatusOK,
			wantACAO:       "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			method := tt.method
			if method == "" {
				method = http.MethodGet
			}
			req := httptest.NewRequest(method, "/mcp", nil)
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}
			w := httptest.NewRecorder()
			CORS(tt.allowedOrigins)(next).ServeHTTP(w, req)
			assert.Equal(t, tt.wantStatus, w.Code)
			assert.Equal(t, tt.wantACAO, w.Header().Get("Access-Control-Allow-Origin"))
		})
	}
}

func TestTCPHandlerFromConfig(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	tests := []struct {
		name                  string
		authToken             string
		allowedOrigins        []string
		unauthenticatedStatus bool
		path                  string
		authHeader            string
		origin                string
		wantStatus            int
		wantACAO              string
	}{
		{
			name:       "no auth + no cors: passthrough",
			path:       "/mcp",
			wantStatus: http.StatusOK,
		},
		{
			name:                  "auth required, /api/status exempted when flag set",
			authToken:             "tok",
			unauthenticatedStatus: true,
			path:                  "/api/status",
			wantStatus:            http.StatusOK,
		},
		{
			name:       "auth required, /api/status NOT exempted when flag unset",
			authToken:  "tok",
			path:       "/api/status",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:           "cors header present for allowed origin",
			allowedOrigins: []string{"http://localhost:3000"},
			origin:         "http://localhost:3000",
			path:           "/mcp",
			wantStatus:     http.StatusOK,
			wantACAO:       "http://localhost:3000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}
			w := httptest.NewRecorder()
			TCPHandlerFromConfig(next, tt.authToken, tt.allowedOrigins, tt.unauthenticatedStatus).ServeHTTP(w, req)
			assert.Equal(t, tt.wantStatus, w.Code)
			assert.Equal(t, tt.wantACAO, w.Header().Get("Access-Control-Allow-Origin"))
		})
	}
}
