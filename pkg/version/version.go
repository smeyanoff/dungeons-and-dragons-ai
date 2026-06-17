package version

import (
	"fmt"
	"runtime"
)

var (
	// Version is the application version
	Version = "dev"
	// Commit is the git commit SHA
	Commit = "unknown"
	// BuildTime is the build timestamp
	BuildTime = "unknown"
	// GoVersion is the Go version used to build
	GoVersion = runtime.Version()
)

// Info contains version information
type Info struct {
	Version   string
	Commit    string
	BuildTime string
	GoVersion string
}

// Get returns version information
func Get() Info {
	return Info{
		Version:   Version,
		Commit:    Commit,
		BuildTime: BuildTime,
		GoVersion: GoVersion,
	}
}

// String returns a formatted version string
func String() string {
	return fmt.Sprintf("Version: %s\nCommit: %s\nBuildTime: %s\nGoVersion: %s",
		Version, Commit, BuildTime, GoVersion)
}
