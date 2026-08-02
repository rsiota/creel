// Package version reports the creel build version for :version and CLI use.
package version

import (
	"runtime/debug"
)

// String returns a short version label like "creel v1.2.3" or "creel (devel)"
// when built without module version metadata (typical for `go run` / local
// builds).
func String() string {
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
