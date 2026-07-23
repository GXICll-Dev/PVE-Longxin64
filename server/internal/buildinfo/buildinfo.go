package buildinfo

// These values are replaced by linker flags in release container builds.
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildTime = "unknown"
)
