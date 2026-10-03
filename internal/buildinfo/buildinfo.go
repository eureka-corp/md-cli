// Package buildinfo holds values injected at build time with -ldflags.
package buildinfo

import (
	"fmt"
	"io"
	"strconv"
	"time"
)

var (
	Version = "dev"
	Commit  = ""
	Time    = ""
)

// Repository is where releases are published and self-update looks.
const (
	RepoOwner = "eureka-corp"
	RepoName  = "md-cli"
)

// IsRelease reports whether this binary came from a tagged release.
func IsRelease() bool {
	return Version != "dev" && Version != ""
}

func Print(w io.Writer) {
	built := Time
	if ts, err := strconv.ParseInt(Time, 10, 64); err == nil {
		built = time.Unix(ts, 0).UTC().Format(time.RFC3339)
	}
	fmt.Fprintf(w, "Version:    %s\nCommit:     %s\nBuild time: %s\n", Version, Commit, built)
}
