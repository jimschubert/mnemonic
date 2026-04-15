package daemon

import (
	"net"
	"time"

	"github.com/jimschubert/mnemonic/internal/config"
)

// IsRunning reports whether the daemon is accepting connections on its Unix socket.
func IsRunning(conf config.Config) bool {
	conn, err := net.DialTimeout("unix", conf.SocketPath(), 200*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}
