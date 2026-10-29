package main

import (
	"bytes"
	"context"
	"encoding/json"
	"kicadai/internal/reproduction"
	"kicadai/internal/schematicassistant"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiagnosticCLIAndCapabilityExplorer(t *testing.T) {
	var output, stderr bytes.Buffer
	for _, args := range [][]string{{"capability", "explore"}, {"capability", "explore", "divider"}, {"capability", "docs"}} {
		output.Reset()
		if err := run(args, &output, &stderr); err != nil || output.Len() == 0 {
			t.Fatalf("%v: %v", args, err)
		}
	}
	for _, args := range [][]string{{"capability", "explore", "missing"}, {"conformance", "bad"}, {"repro", "replay"}, {"--overwrite", "conformance", "run"}} {
		if err := run(args, &output, &stderr); err == nil {
			t.Fatal("accepted", args)
		}
	}
	root := t.TempDir()
	report := filepath.Join(root, "report")
	output.Reset()
	if err := run([]string{"--output", report, "conformance", "run"}, &output, &stderr); err != nil || !strings.Contains(output.String(), `"external_status": "not_checked"`) {
		t.Fatalf("offline %v %s", err, output.String())
	}
	if err := run([]string{"--target", report, "conformance", "verify"}, &output, &stderr); err != nil {
		t.Fatal(err)
	}
	output.Reset()
	if err := run([]string{"--output", report, "conformance", "run"}, &output, &stderr); err == nil || output.Len() != 0 {
		t.Fatalf("early error emitted a misleading report: %v %s", err, output.String())
	}
	input := filepath.Join(root, "draft.json")
	if err := os.WriteFile(input, []byte("{malformed"), 0600); err != nil {
		t.Fatal(err)
	}
	repro := filepath.Join(root, "repro")
	if err := run([]string{"--request", input, "--output", repro, "repro", "schematic"}, &output, &stderr); err == nil {
		t.Fatal("bad schematic did not exit nonzero")
	}
	if err := run([]string{"--target", repro, "repro", "verify"}, &output, &stderr); err != nil {
		t.Fatal(err)
	}
	output.Reset()
	if err := run([]string{"--target", repro, "--output", filepath.Join(root, "replay"), "repro", "replay"}, &output, &stderr); err == nil || !strings.Contains(output.String(), "session_decode") {
		t.Fatalf("replay %v %s", err, output.String())
	}
}

func TestReproNativeReplayRequiresExplicitERCFlag(t *testing.T) {
	root := t.TempDir()
	session, err := schematicassistant.Example("divider")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(session)
	if err != nil {
		t.Fatal(err)
	}
	bundle := filepath.Join(root, "native")
	if _, err = reproduction.CaptureSchematic(context.Background(), raw, bundle, schematicassistant.Options{RequireERC: true, KiCadCLI: "/nonexistent/kicad-cli"}); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	out := filepath.Join(root, "guarded")
	err = run([]string{"--target", bundle, "--output", out, "--kicad-cli", "/another/nonexistent/tool", "repro", "replay"}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "explicitly pass --require-erc") {
		t.Fatal("native replay guard was bypassed", err)
	}
	if _, err = os.Stat(out); !os.IsNotExist(err) {
		t.Fatal("native replay ran before explicit opt-in")
	}
}
