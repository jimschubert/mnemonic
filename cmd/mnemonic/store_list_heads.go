package main

import (
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/jimschubert/mnemonic/internal/config"
	"github.com/jimschubert/mnemonic/internal/store"
)

type ListHeadsCmd struct {
	SocketPath string `short:"s" default:"${socket_path}" help:"Path to daemon socket" env:"MNEMONIC_SOCKET_PATH"`
}

func (c *ListHeadsCmd) Run(logger *slog.Logger, conf config.Config) error {
	conf.ApplyOverrides(config.Config{
		SocketPathRaw: c.SocketPath,
	})

	res, err := socketSend(conf, "mnemonic_list_heads", nil)
	if err != nil {
		return err
	}

	type result struct {
		Heads []store.HeadInfo `json:"heads"`
	}
	var r result
	if res != nil {
		marshaled, err := json.Marshal(res)
		if err != nil {
			return fmt.Errorf("marshaling response: %w", err)
		}
		if err := json.Unmarshal(marshaled, &r); err != nil {
			return fmt.Errorf("unmarshaling response: %w", err)
		}
	}

	for _, head := range r.Heads {
		var prefix string
		if head.Mandatory {
			prefix = "* "
		} else {
			prefix = "  "
		}
		fmt.Printf("%s%s (count=%d)\n", prefix, head.Name, head.Count)
	}
	return nil
}
