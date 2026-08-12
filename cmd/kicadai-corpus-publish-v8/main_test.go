package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestRunRequiresCompleteV8Boundary(t *testing.T) {
	var stdout bytes.Buffer
	if err := run(nil, &stdout); err == nil || !strings.Contains(err.Error(), "-packet-root is required: usage:") {
		t.Fatalf("error=%v", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout=%q", stdout.String())
	}
}
func TestRunV8Help(t *testing.T) {
	var stdout bytes.Buffer
	if err := run([]string{"-h"}, &stdout); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "Usage of kicadai-corpus-publish-v8:") || !strings.Contains(stdout.String(), "-freeze-parent-commit") {
		t.Fatalf("help=%q", stdout.String())
	}
}
func TestV8BundlePathsExact(t *testing.T) {
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
func TestV8FrozenCommits(t *testing.T) {
	if err := verifyFrozenCommits(v8StartingCommit, v8ContractFreezeCommit, v8AuthorPacketCommit, v8ValidatorCommit); err != nil {
		t.Fatal(err)
	}
	if err := verifyFrozenCommits(v8StartingCommit, v8ContractFreezeCommit, v8AuthorPacketCommit, strings.Repeat("0", 40)); err == nil {
		t.Fatal("validator substitution accepted")
	}
}
