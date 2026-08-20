// Package buildinfo exposes the reproducible identity of a KiCadAI binary.
package buildinfo

import (
	"runtime"
	"runtime/debug"
	"strings"
)

const (
	Name            = "kicadai"
	Development     = "dev"
	Unknown         = "unknown"
	versionSetting  = "vcs.revision"
	timeSetting     = "vcs.time"
	modifiedSetting = "vcs.modified"
)

// These values are replaced by -ldflags for release builds. Development
// builds fall back to the VCS data embedded by the Go toolchain.
var (
	Version   = Development
	Commit    = Unknown
	BuildDate = Unknown
	platform  = runtime.GOOS + "/" + runtime.GOARCH
)

// Info is the stable, machine-readable application identity.
type Info struct {
	Name      string `json:"name"`
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildDate string `json:"build_date"`
	GoVersion string `json:"go_version"`
	Platform  string `json:"platform"`
	Modified  bool   `json:"modified"`
}

// Current returns release metadata without consulting the network or the
// working tree. Linker-provided values take precedence over embedded VCS data.
func Current() Info {
	info := Info{
		Name:      Name,
		Version:   normalized(Version, Development),
		Commit:    normalized(Commit, Unknown),
		BuildDate: normalized(BuildDate, Unknown),
		GoVersion: runtime.Version(),
		Platform:  platform,
	}

	if build, ok := debug.ReadBuildInfo(); ok {
		if info.Version == Development && build.Main.Version != "" && build.Main.Version != "(devel)" {
			info.Version = strings.TrimPrefix(build.Main.Version, "v")
		}
		for _, setting := range build.Settings {
			switch setting.Key {
			case versionSetting:
				if info.Commit == Unknown {
					info.Commit = normalized(setting.Value, Unknown)
				}
			case timeSetting:
				if info.BuildDate == Unknown {
					info.BuildDate = normalized(setting.Value, Unknown)
				}
			case modifiedSetting:
				info.Modified = setting.Value == "true"
			}
		}
	}

	return info
}

func normalized(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}
