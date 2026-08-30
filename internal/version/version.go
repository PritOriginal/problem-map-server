// Package version identifies the running build. The value is stamped at
// link time and is used wherever a redeploy must invalidate state that
// outlives the process — currently the keys of the response cache.
package version

import (
	"runtime/debug"
	"sync"
)

// Version is set at link time:
//
//	go build -ldflags "-X github.com/PritOriginal/problem-map-server/internal/version.Version=$(git rev-parse HEAD)"
//
// When it is empty the build id falls back to the VCS revision recorded by
// the toolchain, and then to "dev".
var Version string

// maxBuildLen keeps the build id short enough to stay readable inside a
// cache key; a commit hash is unique long before its 12th character.
const maxBuildLen = 12

var build = sync.OnceValue(func() string {
	if v := truncate(Version); v != "" {
		return v
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			if setting.Key == "vcs.revision" {
				if v := truncate(setting.Value); v != "" {
					return v
				}
			}
		}
	}
	return "dev"
})

// Build returns the identifier of the running build. It is stable for the
// life of the process and equal across every replica of the same build, so
// replicas share their cache entries while a redeploy starts a fresh
// namespace.
func Build() string {
	return build()
}

func truncate(v string) string {
	if len(v) > maxBuildLen {
		return v[:maxBuildLen]
	}
	return v
}
