package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"kicadai/internal/architecturesearch"
	"kicadai/internal/reports"
)

func TestExclusiveEvidenceAndBoundedModes(t *testing.T) {
	dir := t.TempDir()
	if err := writeBytes(dir, "evidence", []byte("original")); err != nil {
		t.Fatal(err)
	}
	if err := writeBytes(dir, "evidence", []byte("replacement")); err == nil {
		t.Fatal("overwritten evidence")
	}
	b, err := os.ReadFile(filepath.Join(dir, "evidence"))
	if err != nil || string(b) != "original" {
		t.Fatal("changed original")
	}
	for _, key := range []string{"OPENAI_API_KEY", "ANTHROPIC_API_KEY", "GEMINI_API_KEY", "GOOGLE_API_KEY", "KICADAI_OPENAI_LIVE_TEST"} {
		t.Setenv(key, "")
	}
	oldSource, oldSHA := engineSource, adapterSHA256
	t.Cleanup(func() { engineSource, adapterSHA256 = oldSource, oldSHA })
	engineSource, adapterSHA256 = strings.Repeat("a", 40), strings.Repeat("b", 64)
	for _, mode := range []string{"live", "", "final"} {
		if run(mode, "", filepath.Join(dir, "unused"), time.Minute) == nil {
			t.Fatal("unsupported mode accepted")
		}
	}
	for _, limit := range []time.Duration{0, -time.Second, 21 * time.Minute} {
		if run("snapshot", "", filepath.Join(dir, "unused"), limit) == nil {
			t.Fatal("unbounded deadline accepted")
		}
	}
	t.Setenv("OPENAI_API_KEY", "synthetic-not-a-real-key")
	if run("snapshot", "", filepath.Join(dir, "unused"), time.Minute) == nil {
		t.Fatal("provider credential admitted to offline engine")
	}
}

func TestStrictInputNeverRepairsUnknownOrTrailingData(t *testing.T) {
	b, err := os.ReadFile("../../../internal/architecturesearch/testdata/power_interface_synthesis_corpus/regulated_mcu_sensor_subsystem.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, issues := architecturesearch.DecodeStrict(bytes.NewReader(b)); reports.HasBlockingIssue(issues) {
		t.Fatal(issues)
	}
	var raw map[string]any
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatal(err)
	}
	raw["invented_answer"] = "do not ignore"
	unknown, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range [][]byte{unknown, append(append([]byte(nil), b...), []byte(" {}")...)} {
		if _, issues := architecturesearch.DecodeStrict(bytes.NewReader(bad)); !reports.HasBlockingIssue(issues) {
			t.Fatal("invalid input admitted")
		}
	}
}

func TestInputReadIsBounded(t *testing.T) {
	dir := t.TempDir()
	if _, err := readInput(dir); err == nil {
		t.Fatal("directory accepted")
	}
	for _, size := range []int{architecturesearch.MaxRequirementBytes, architecturesearch.MaxRequirementBytes + 1} {
		name := filepath.Join(t.TempDir(), "input")
		if err := os.WriteFile(name, bytes.Repeat([]byte(" "), size), 0o600); err != nil {
			t.Fatal(err)
		}
		b, err := readInput(name)
		if size <= architecturesearch.MaxRequirementBytes && (err != nil || len(b) != size) {
			t.Fatal("bounded input not retained exactly", err)
		}
		if size > architecturesearch.MaxRequirementBytes && (err == nil || b != nil) {
			t.Fatal("oversize input read or accepted")
		}
	}
}
