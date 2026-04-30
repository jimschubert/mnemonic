package daemon

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"time"

	"github.com/jimschubert/mnemonic/internal/config"
)

// NewSocketClient returns an HTTP client that dials the daemon's Unix socket.
func NewSocketClient(conf config.Config) *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, "unix", conf.SocketPath())
			},
		},
	}
}

// WatchDaemon polls GET /api/status via httpClient every interval and cancels the
// returned context when the daemon becomes unreachable or restarts (detected via
// started_at changing). The caller must call the returned CancelFunc to release
// resources when the watch is no longer needed.
func WatchDaemon(ctx context.Context, httpClient *http.Client, interval time.Duration) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(ctx)
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		var startedAt int64

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
				if resp.StatusCode != http.StatusOK {
					_ = resp.Body.Close()
					cancel()
					return
				}

				var st struct {
					StartedAt int64 `json:"started_at"`
				}
				if err := json.NewDecoder(resp.Body).Decode(&st); err == nil {
					if startedAt == 0 {
						startedAt = st.StartedAt
					} else if startedAt != st.StartedAt {
						_ = resp.Body.Close()
						cancel()
						return
					}
				}
				_ = resp.Body.Close()
			}
		}
	}()
	return ctx, cancel
}
