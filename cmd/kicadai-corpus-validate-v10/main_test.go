package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestUsageFailsClosed(t *testing.T) {
	for _, arguments := range [][]string{nil, {"-packet-root", "x"}, {"extra"}} {
		if err := run(arguments, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "usage:") {
			t.Fatalf("arguments %v did not fail with usage", arguments)
		}
	}
}

func TestParseBundlePathsRequiresExactAuthors(t *testing.T) {
	authors := []string{"author_1", "author_2"}
	got, err := parseBundlePaths([]string{"author_1=/a", "author_2=/b"}, authors)
	if err != nil || got["author_1"] != "/a" || got["author_2"] != "/b" {
		t.Fatalf("parse = %v, %v", got, err)
	}
	for _, values := range [][]string{{"author_1=/a"}, {"author_1=/a", "author_1=/b"}, {"author_3=/c", "author_2=/b"}} {
		if _, err := parseBundlePaths(values, authors); err == nil {
			t.Fatalf("accepted invalid bundles %v", values)
		}
	}
}
