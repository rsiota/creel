// Package version reports the creel build version for :version and CLI use.
//
// Version is the authoritative version string. It is overwritten at release
// time by GoReleaser via ldflags:
//
//	-ldflags "-X github.com/rsiota/creel/internal/version.Version=v0.1.0"
//
// For local and CI builds without injection it stays empty, in which case
// String falls back to the module version embedded by `go install`, or
// "(devel)" for local `go build`/`go run`.
package version

import (
	"runtime/debug"
)

// Version is the current creel version, set via ldflags at release time.
// Empty when not injected.
var Version = ""

// String returns a short version label like "creel v1.2.3".
//
// Precedence: ldflags-injected Version, then the module version from
// debug.ReadBuildInfo() (populated by `go install`), then "(devel)".
func String() string {
	if Version != "" {
		return "creel " + Version
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "creel (unknown)"
	}
	v := info.Main.Version
	if v == "" || v == "(devel)" {
		return "creel (devel)"
	}
	return "creel " + v
}
