// Package version reports the gsql build version for :version and CLI use.
package version

import (
	"runtime/debug"
)

// String returns a short version label like "gsql v1.2.3" or "gsql (devel)"
// when built without module version metadata (typical for `go run` / local
// builds).
func String() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "gsql (unknown)"
	}
	v := info.Main.Version
	if v == "" || v == "(devel)" {
		return "gsql (devel)"
	}
	return "gsql " + v
}
