package main

import (
	"io"
	"strings"
	"testing"
	"time"

	"github.com/alecthomas/assert/v2"
	"github.com/jimschubert/mnemonic/internal/compact"
)

func TestConfirmCavemanMode(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantErr     string
		wantWarning string
	}{
		{
			name:        "accepts lowercase y",
			input:       "y\n",
			wantWarning: "Caveman mode (lite) is DESTRUCTIVE and can't be undone.",
		},
		{
			name:        "accepts yes without newline",
			input:       "yes",
			wantWarning: "Caveman mode (lite) is DESTRUCTIVE and can't be undone.",
		},
		{
			name:        "rejects default response",
			input:       "\n",
			wantErr:     "caveman compaction aborted",
			wantWarning: "Ensure you have committed your global/project/team memories before continuing.",
		},
		{
			name:        "rejects no",
			input:       "n\n",
			wantErr:     "caveman compaction aborted",
			wantWarning: "ARE YOU SURE? y/N",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out strings.Builder

			err := confirmCavemanMode(strings.NewReader(tt.input), &out, compact.CavemanLite, time.Second)
			if tt.wantErr == "" {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
				assert.Equal(t, tt.wantErr, err.Error())
			}

			assert.True(t, strings.Contains(out.String(), tt.wantWarning))
		})
	}
}

func TestCompactCmd_ShouldConfirmCavemanMode(t *testing.T) {
	tests := []struct {
		name string
		cmd  CompactCmd
		mode compact.CavemanMode
		want bool
	}{
		{
			name: "does not confirm when caveman is off",
			cmd: CompactCmd{
				Yes: false,
			},
			mode: compact.CavemanOff,
			want: false,
		},
		{
			name: "confirms when caveman is enabled without yes",
			cmd: CompactCmd{
				Yes: false,
			},
			mode: compact.CavemanUltra,
			want: true,
		},
		{
			name: "skips confirmation when yes is set",
			cmd: CompactCmd{
				Yes: true,
			},
			mode: compact.CavemanLite,
			want: false,
		},
		{
			name: "yes is ignored when caveman is off",
			cmd: CompactCmd{
				Yes: true,
			},
			mode: compact.CavemanOff,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.cmd.shouldConfirmCavemanMode(tt.mode))
		})
	}
}

func TestConfirmCavemanMode_Timeout(t *testing.T) {
	reader, writer := io.Pipe()
	defer func() {
		_ = writer.Close()
	}()

	var out strings.Builder

	err := confirmCavemanMode(reader, &out, compact.CavemanFull, 50*time.Millisecond)
	assert.Error(t, err)
	assert.Equal(t, "timed out waiting for caveman confirmation", err.Error())
	assert.True(t, strings.Contains(out.String(), "Caveman mode (full) is DESTRUCTIVE and can't be undone."))
}
