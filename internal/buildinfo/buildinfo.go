// Package buildinfo holds variables injected by ldflags at build time.
package buildinfo

import "fmt"

// Variables set via -ldflags at compile time.
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
	Dirty     = "false"
)

// String returns a human-readable build info string.
func String() string {
	dirty := ""
	if Dirty == "true" {
		dirty = " (dirty)"
	}
	return fmt.Sprintf("shiori-server %s (commit %s, built %s%s)",
		Version, short(Commit), BuildDate, dirty)
}

func short(s string) string {
	if len(s) > 8 {
		return s[:8]
	}
	return s
}
