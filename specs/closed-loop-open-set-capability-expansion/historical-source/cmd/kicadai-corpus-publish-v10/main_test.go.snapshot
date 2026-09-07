package main

import (
	"bytes"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"kicadai/internal/corpuspublication"
)

func TestRunRequiresCompleteV10Boundary(t *testing.T) {
	var stdout bytes.Buffer
	if err := run(nil, &stdout); err == nil || !strings.Contains(err.Error(), "-packet-root is required: usage:") {
		t.Fatalf("error=%v", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout=%q", stdout.String())
	}
}

func TestRunV10Help(t *testing.T) {
	var stdout bytes.Buffer
	if err := run([]string{"-h"}, &stdout); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "Usage of kicadai-corpus-publish-v10:") || !strings.Contains(stdout.String(), "-freeze-parent-commit") {
		t.Fatalf("help=%q", stdout.String())
	}
}

func TestV10BundlePathsExact(t *testing.T) {
	authors := []string{"author_1", "author_2"}
	usage := errors.New("usage")
	got, err := parseBundlePaths([]string{"author_2=/two", "author_1=/one"}, authors, usage)
	if err != nil || got["author_1"] != "/one" {
		t.Fatalf("got=%v err=%v", got, err)
	}
	for _, values := range [][]string{{"author_1=/one"}, {"author_1=/one", "author_1=/again"}, {"author_1=/one", "author_3=/three"}} {
		if _, err := parseBundlePaths(values, authors, usage); err == nil {
			t.Fatalf("accepted %v", values)
		}
	}
}

func TestV10FrozenCommits(t *testing.T) {
	if err := verifyFrozenCommits(v10StartingCommit, v10ContractFreezeCommit, v10AuthorPacketCommit, v10ValidatorCommit); err != nil {
		t.Fatal(err)
	}
	if err := verifyFrozenCommits(v10StartingCommit, v10ContractFreezeCommit, v10AuthorPacketCommit, strings.Repeat("0", 40)); err == nil || !strings.Contains(err.Error(), "validator commit") {
		t.Fatal("validator substitution accepted")
	}
}

func TestV10FrozenManifests(t *testing.T) {
	repositoryRoot := filepath.Clean("../..")
	specRoot := filepath.Join(repositoryRoot, "specs", "closed-loop-open-set-capability-expansion")
	for _, name := range []string{"V10_VALIDATOR.sha256", "V10_PUBLISHER.sha256"} {
		if _, err := corpuspublication.VerifyChecksumManifest(repositoryRoot, filepath.Join(specRoot, name)); err != nil {
			t.Fatalf("verify %s: %v", name, err)
		}
	}
	if _, err := corpuspublication.VerifyV6ContractManifest(repositoryRoot, filepath.Join(specRoot, "V10_CONTRACT.sha256")); err != nil {
		t.Fatalf("verify V10_CONTRACT.sha256: %v", err)
	}
}
