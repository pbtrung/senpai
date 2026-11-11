package senpai

import (
	"runtime/debug"
)

// version is set via -ldflags "-X git.sr.ht/~delthas/senpai.version=...".
var version string

func BuildVersion() (string, bool) {
	if version != "" {
		return version, true
	}
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		return bi.Main.Version, true
	}
	return "", false
}
