package version

import "fmt"

// BuildInfo holds structured version information.
type BuildInfo struct {
	Version string
	Commit  string
	Date    string
}

// Build-time variables set via ldflags.
var (
	buildVersion = "dev"
	buildCommit  = "unknown"
	buildDate    = "unknown"
)

// Info returns the current build information.
func Info() BuildInfo {
	return BuildInfo{
		Version: buildVersion,
		Commit:  buildCommit,
		Date:    buildDate,
	}
}

// String returns a human-readable version string.
func (b BuildInfo) String() string {
	return fmt.Sprintf("version=%s commit=%s date=%s", b.Version, b.Commit, b.Date)
}
