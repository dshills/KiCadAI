package buildinfo

import (
	"runtime"
	"testing"
)

func TestCurrentUsesLinkerIdentity(t *testing.T) {
	originalVersion, originalCommit, originalBuildDate := Version, Commit, BuildDate
	t.Cleanup(func() {
		Version, Commit, BuildDate = originalVersion, originalCommit, originalBuildDate
	})

	Version = "1.0.0-rc.1"
	Commit = "0123456789abcdef"
	BuildDate = "2026-08-20T12:00:00Z"

	got := Current()
	if got.Name != Name || got.Version != Version || got.Commit != Commit || got.BuildDate != BuildDate {
		t.Fatalf("Current() identity = %+v", got)
	}
	if got.GoVersion != runtime.Version() || got.Platform != runtime.GOOS+"/"+runtime.GOARCH {
		t.Fatalf("Current() runtime = %+v", got)
	}
}

func TestCurrentNormalizesEmptyLinkerValues(t *testing.T) {
	originalVersion, originalCommit, originalBuildDate := Version, Commit, BuildDate
	t.Cleanup(func() {
		Version, Commit, BuildDate = originalVersion, originalCommit, originalBuildDate
	})

	Version = " "
	Commit = " "
	BuildDate = " "

	got := Current()
	if got.Version != Development {
		t.Fatalf("Version = %q, want %q", got.Version, Development)
	}
	if got.Commit == "" || got.BuildDate == "" {
		t.Fatalf("Current() contains empty identity: %+v", got)
	}
}
