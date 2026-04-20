package server

import (
	"net/http"
	"slices"
)

// BearerAuth creates middleware requiring a static bearer token on all requests.
// When token is empty or path is in exemptPaths, check is skipped (e.g. for /api/status).
func BearerAuth(token string, exemptPaths ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if token == "" {
			return next
		}
		expected := "Bearer " + token
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if slices.Contains(exemptPaths, r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}
			if r.Header.Get("Authorization") != expected {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// CORS creates middleware that require CORS on all requests and handles preflight requests.
// When allowedOrigins is empty, the check is skipped.
// Pass "*" in allowedOrigins to permit any origin (Access-Control-Allow-Origin: *).
func CORS(allowedOrigins []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if len(allowedOrigins) == 0 {
			return next
		}
		wildcard := slices.Contains(allowedOrigins, "*")
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" {
				allowed := wildcard || slices.Contains(allowedOrigins, origin)
				if allowed {
					// see https://security.stackexchange.com/questions/151590/vary-origin-response-header-and-cors-exploitation
					if wildcard {
						w.Header().Set("Access-Control-Allow-Origin", "*")
					} else {
						w.Header().Set("Access-Control-Allow-Origin", origin)
						w.Header().Add("Vary", "Origin")
					}
					w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
					w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
					if r.Method == http.MethodOptions {
						w.WriteHeader(http.StatusNoContent)
						return
					}
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// chain applies middleware in order: first(second(...(handler))).
func chain(h http.Handler, middlewares ...func(http.Handler) http.Handler) http.Handler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		h = middlewares[i](h)
	}
	return h
}

// TCPHandlerFromConfig builds the TCP middleware chain from config values.
func TCPHandlerFromConfig(base http.Handler, authToken string, allowedOrigins []string, unauthenticatedStatus bool) http.Handler {
	var exempt []string
	if unauthenticatedStatus {
		exempt = []string{"/api/status"}
	}
	return chain(base,
		CORS(allowedOrigins),
		BearerAuth(authToken, exempt...),
	)
}
