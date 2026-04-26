package main

import (
	"fmt"
	"runtime/debug"
)

var readBuildInfo = debug.ReadBuildInfo

const (
	defaultVersionValue = "dev"
	defaultCommitValue  = "unknown SHA"
)

func resolvedVersionString() string {
	versionValue, commitValue := version, commit

	if isDefaultVersionMetadata(versionValue, commitValue) {
		if info, ok := readBuildInfo(); ok && info != nil {
			if buildVersion := info.Main.Version; buildVersion != "" && buildVersion != "(devel)" {
				versionValue = buildVersion
			}
			for _, setting := range info.Settings {
				if setting.Key == "vcs.revision" {
					commitValue = setting.Value
					if len(commitValue) > 7 {
						commitValue = commitValue[:7]
					}
					break
				}
			}
		}
	}

	return fmt.Sprintf("%s (%s)", versionValue, commitValue)
}

func isDefaultVersionMetadata(versionValue, commitValue string) bool {
	return versionValue == defaultVersionValue && commitValue == defaultCommitValue
}




