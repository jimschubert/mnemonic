package main

import (
	"runtime/debug"
	"testing"

	"github.com/alecthomas/assert/v2"
)

func TestResolvedVersionString(t *testing.T) {
	tests := []struct {
		name      string
		version   string
		commit    string
		buildInfo *debug.BuildInfo
		want      string
	}{
		{
			name:    "uses linker-provided version metadata first",
			version: "1.2.3",
			commit:  "abc1234",
			buildInfo: &debug.BuildInfo{
				Main: debug.Module{Version: "v9.9.9"},
				Settings: []debug.BuildSetting{{Key: "vcs.revision", Value: "deadbeefcafebabe"}},
			},
			want: "1.2.3 (abc1234)",
		},
		{
			name:      "falls back to build metadata when linker values are defaults",
			version:   defaultVersionValue,
			commit:    defaultCommitValue,
			buildInfo: &debug.BuildInfo{Main: debug.Module{Version: "v0.0.8"}, Settings: []debug.BuildSetting{{Key: "vcs.revision", Value: "f1c49052bcfde14f2866e43cdb24d7f6c9399e60"}}},
			want:      "v0.0.8 (f1c4905)",
		},
		{
			name:      "falls back to defaults when build info is unavailable",
			version:   defaultVersionValue,
			commit:    defaultCommitValue,
			buildInfo: nil,
			want:      "dev (unknown SHA)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			originalVersion := version
			originalCommit := commit
			originalReadBuildInfo := readBuildInfo
			defer func() {
				version = originalVersion
				commit = originalCommit
				readBuildInfo = originalReadBuildInfo
			}()

			version = tt.version
			commit = tt.commit
			readBuildInfo = func() (*debug.BuildInfo, bool) {
				if tt.buildInfo == nil {
					return nil, false
				}
				return tt.buildInfo, true
			}

			assert.Equal(t, tt.want, resolvedVersionString())
		})
	}
}


