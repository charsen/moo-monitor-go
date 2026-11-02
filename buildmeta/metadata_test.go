package buildmeta

import (
	"runtime"
	"runtime/debug"
	"strings"
	"testing"
)

func TestResolveBuildMetadata(t *testing.T) {
	sdk := debug.Module{Path: sdkModule, Version: "v1.2.3"}
	framework := &debug.Module{Path: goravelModule, Version: "v1.18.0"}
	info := &debug.BuildInfo{GoVersion: "go1.27.3", Main: sdk, Deps: []*debug.Module{framework}, Settings: []debug.BuildSetting{{Key: "vcs.revision", Value: "must-not-be-version"}}}
	got := Resolve(info, true)
	if got.SDKVersion != "v1.2.3" || got.GoravelVersion != "v1.18.0" || got.GoVersion != "go1.27.3" {
		t.Fatal(got)
	}
	for _, test := range []struct {
		name   string
		module debug.Module
		want   string
	}{
		{"workspace", debug.Module{Path: sdkModule, Version: "(devel)"}, "development"},
		{"empty", debug.Module{Path: sdkModule}, "development"},
		{"local", debug.Module{Path: sdkModule, Version: "v1.2.3", Replace: &debug.Module{Path: "../local-sdk"}}, "development"},
		{"foreign-version", debug.Module{Path: sdkModule, Version: "v1.2.3", Replace: &debug.Module{Path: "example.invalid/fork", Version: "v9.0.0"}}, "development"},
		{"same-version", debug.Module{Path: sdkModule, Version: "v1.2.3", Replace: &debug.Module{Path: sdkModule, Version: "v1.2.4"}}, "v1.2.4"},
		{"too-long", debug.Module{Path: sdkModule, Version: strings.Repeat("v", 65)}, "unknown"},
		{"bad-utf8", debug.Module{Path: sdkModule, Version: string([]byte{255})}, "unknown"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if v := Resolve(&debug.BuildInfo{Deps: []*debug.Module{nil, &test.module}}, true).SDKVersion; v != test.want {
				t.Fatal(v)
			}
		})
	}
	for _, input := range []struct {
		info *debug.BuildInfo
		ok   bool
	}{{nil, false}, {nil, true}, {info, false}, {&debug.BuildInfo{}, true}} {
		value := Resolve(input.info, input.ok)
		if value.SDKVersion != "unknown" || value.GoravelVersion != "unknown" || value.GoVersion != runtime.Version() {
			t.Fatal(value)
		}
	}
}
func TestCurrentBuildMetadata(t *testing.T) {
	current := Current()
	if current.GoVersion != runtime.Version() || current.SDKVersion == "" || current.GoravelVersion == "" {
		t.Fatal(current)
	}
}
