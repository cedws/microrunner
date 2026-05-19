package version

// injected by goreleaser
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func Version() string {
	return version
}

func Commit() string {
	return commit
}

func Date() string {
	return date
}
