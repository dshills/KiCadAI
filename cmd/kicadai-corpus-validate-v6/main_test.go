package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunRequiresCompleteArguments(t *testing.T) {
	for _, arguments := range [][]string{
		nil,
		{"-packet-root", "packet", "-history", "history", "-output", "report"},
		{"-unknown"},
	} {
		if err := run(arguments, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "usage: kicadai-corpus-validate-v6") {
			t.Fatalf("run(%q) error = %v", arguments, err)
		}
	}
}

func TestParseBundlePathsRequiresExactAuthorSet(t *testing.T) {
	authors := []string{"author_1", "author_2", "author_3"}
	got, err := parseBundlePaths([]string{"author_3=/three", "author_1=/one", "author_2=/two"}, authors)
	if err != nil {
		t.Fatal(err)
	}
	if got["author_1"] != "/one" || got["author_2"] != "/two" || got["author_3"] != "/three" {
		t.Fatalf("bundle paths = %#v", got)
	}
	for _, arguments := range [][]string{
		{"author_1=/one", "author_2=/two"},
		{"author_1=/one", "author_2=/two", "author_3=/three", "author_3=/again"},
		{"author_1=/one", "author_2=/two", "unknown=/three"},
	} {
		if _, err := parseBundlePaths(arguments, authors); err == nil {
			t.Fatalf("parseBundlePaths(%q) unexpectedly passed", arguments)
		}
	}
}
