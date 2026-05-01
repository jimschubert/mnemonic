package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"text/tabwriter"
	"time"

	"github.com/jimschubert/mnemonic/internal/config"
	"github.com/jimschubert/mnemonic/internal/daemon"
)

// DaemonStatusCmd reports whether the daemon is running and shows details from the status endpoint.
type DaemonStatusCmd struct{}

//goland:noinspection GoUnhandledErrorResult
func (c *DaemonStatusCmd) Run(_ *slog.Logger, conf config.Config) error {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 4, ' ', 0)
	defer w.Flush()

	// may differ from config used to start the daemon, but likely never changes in the config file so okay for now
	fmt.Fprintf(w, "socket:\t%s\n", conf.SocketPath())

	if !daemon.IsRunning(conf) {
		fmt.Fprintf(w, "status:\tnot running\n")
		return nil
	}

	client := daemon.NewSocketClient(conf)
	resp, err := client.Get("http://unix/api/status")
	if err != nil {
		fmt.Fprintf(w, "status:\tunreachable (%s)\n", err)
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(w, "status:\terror connecting... response code was %d\n", resp.StatusCode)
		return nil
	}

	var statusResp struct {
		Status    string `json:"status"`
		Version   string `json:"version"`
		Uptime    string `json:"uptime"`
		StartedAt int64  `json:"started_at"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&statusResp); err != nil {
		return fmt.Errorf("decoding status response: %w", err)
	}

	startedAt := time.Unix(statusResp.StartedAt, 0).Format(time.RFC3339)
	fmt.Fprintf(w, "status:\t%s\n", statusResp.Status)
	fmt.Fprintf(w, "version:\t%s\n", statusResp.Version)
	fmt.Fprintf(w, "uptime:\t%s\n", statusResp.Uptime)
	fmt.Fprintf(w, "started:\t%s\n", startedAt)

	return nil
}
