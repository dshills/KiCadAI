package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestParseBundlePathsRequiresExactAuthorSet(t *testing.T) {
	authors := []string{"author_1", "author_2", "author_3"}
	got, err := parseBundlePaths([]string{"author_3=/three", "author_1=/one", "author_2=/two"}, authors)
	if err != nil {
		t.Fatal(err)
	}
	if got["author_1"] != "/one" || got["author_2"] != "/two" || got["author_3"] != "/three" {
		t.Fatalf("bundle paths = %#v", got)
	}
	for name, arguments := range map[string][]string{
		"missing":   {"author_1=/one", "author_2=/two"},
		"duplicate": {"author_1=/one", "author_1=/other", "author_2=/two", "author_3=/three"},
		"unknown":   {"author_1=/one", "author_2=/two", "author_4=/four"},
		"empty":     {"author_1=/one", "author_2=", "author_3=/three"},
		"malformed": {"author_1=/one", "author_2=/two", "author_3"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parseBundlePaths(arguments, authors); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestRunRejectsIncompleteInvocationBeforeReadingInputs(t *testing.T) {
	var stdout bytes.Buffer
	err := run([]string{"-packet-root", "missing"}, &stdout)
	if err == nil || !strings.Contains(err.Error(), "usage:") {
		t.Fatalf("error = %v", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestRunReportsFlagErrorsOnce(t *testing.T) {
	var stdout bytes.Buffer
	err := run([]string{"-unknown"}, &stdout)
	if err == nil || strings.Count(err.Error(), "flag provided but not defined") != 1 || !strings.Contains(err.Error(), "usage:") {
		t.Fatalf("error = %v", err)
	}
}

func TestRunReportsHelpAsSingleUsageDiagnostic(t *testing.T) {
	var stdout bytes.Buffer
	err := run([]string{"-h"}, &stdout)
	if err == nil || strings.Count(err.Error(), "usage:") != 1 || strings.Count(err.Error(), "flag: help requested") != 1 {
		t.Fatalf("error = %v", err)
	}
}
