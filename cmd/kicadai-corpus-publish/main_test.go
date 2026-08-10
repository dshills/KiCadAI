package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunRequiresCompletePublicationBoundary(t *testing.T) {
	var stdout bytes.Buffer
	if err := run(nil, &stdout); err == nil || !strings.HasPrefix(err.Error(), "usage:") {
		t.Fatalf("empty invocation error = %v", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("usage failure wrote stdout: %q", stdout.String())
	}
}

func TestParseBundlePathsRequiresEachFrozenAuthorOnce(t *testing.T) {
	authors := []string{"author_1", "author_2", "author_3"}
	paths, err := parseBundlePaths([]string{"author_3=/three", "author_1=/one", "author_2=/two"}, authors)
	if err != nil {
		t.Fatal(err)
	}
	if paths["author_1"] != "/one" || paths["author_2"] != "/two" || paths["author_3"] != "/three" {
		t.Fatalf("bundle paths = %#v", paths)
	}
	for _, invalid := range [][]string{
		{"author_1=/one", "author_2=/two"},
		{"author_1=/one", "author_1=/again", "author_2=/two", "author_3=/three"},
		{"author_1=/one", "author_2=/two", "author_4=/four"},
	} {
		if _, err := parseBundlePaths(invalid, authors); err == nil {
			t.Fatalf("invalid bundle set accepted: %v", invalid)
		}
	}
}

func TestDigestIsLowercaseSHA256(t *testing.T) {
	value := digest([]byte("manifest"))
	if len(value) != 64 || value != strings.ToLower(value) {
		t.Fatalf("digest = %q", value)
	}
}
