// Package buildmeta reports bounded build metadata, not release or deployment attestation.
package buildmeta

import (
	"runtime"
	"runtime/debug"
	"unicode/utf8"
)

const sdkModule = "gitee.com/charsen/moo-monitor-go"
const goravelModule = "github.com/goravel/framework"

type Metadata struct{ SDKVersion, GoVersion, GoravelVersion string }

func Current() Metadata { info, ok := debug.ReadBuildInfo(); return Resolve(info, ok) }

// Resolve consumes the standard build-info shape without exposing module paths,
// checksums, VCS revision or local replacement locations.
func Resolve(info *debug.BuildInfo, available bool) Metadata {
	result := Metadata{SDKVersion: "unknown", GoVersion: bounded(runtime.Version()), GoravelVersion: "unknown"}
	if !available || info == nil {
		return result
	}
	if info.GoVersion != "" {
		result.GoVersion = bounded(info.GoVersion)
	}
	result.SDKVersion = moduleVersion(info, sdkModule)
	result.GoravelVersion = moduleVersion(info, goravelModule)
	return result
}
func moduleVersion(info *debug.BuildInfo, path string) string {
	var found *debug.Module
	if info.Main.Path == path {
		found = &info.Main
	} else {
		for _, module := range info.Deps {
			if module != nil && module.Path == path {
				found = module
				break
			}
		}
	}
	if found == nil {
		return "unknown"
	}
	if found.Replace != nil {
		if found.Replace.Path != path || found.Replace.Version == "" || found.Replace.Version == "(devel)" {
			return "development"
		}
		return bounded(found.Replace.Version)
	}
	if found.Version == "" || found.Version == "(devel)" {
		return "development"
	}
	return bounded(found.Version)
}
func bounded(value string) string {
	if value == "" || len(value) > 64 || !utf8.ValidString(value) {
		return "unknown"
	}
	return value
}
