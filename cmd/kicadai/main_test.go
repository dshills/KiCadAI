package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"kicadai/internal/config"
	"kicadai/internal/kiapi"
	commontypes "kicadai/internal/kiapi/gen/common/types"
	"kicadai/internal/reports"
	"kicadai/internal/workflows"
)

func TestRunDefaultsToHelp(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	if err := run(nil, &stdout, &stderr); err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	if !strings.Contains(stdout.String(), "Usage:") {
		t.Fatalf("expected help output, got %q", stdout.String())
	}
}

func TestRunConfigJSON(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := run([]string{
		"--socket", "ipc:///tmp/kicad/api.sock",
		"--token", "secret-token",
		"--client-name", "test-client",
		"--timeout-ms", "3000",
		"--json",
		"config",
	}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	output := stdout.String()

	for _, want := range []string{
		`"socket_path": "ipc:///tmp/kicad/api.sock"`,
		`"client_name": "test-client"`,
		`"timeout_ms": 3000`,
		`"token": "<redacted>"`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected output to contain %q, got %s", want, output)
		}
	}

	if strings.Contains(output, "secret-token") {
		t.Fatalf("token leaked in output: %s", output)
	}
}

func TestRunConfigTextRedactsToken(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := run([]string{
		"--socket", "ipc:///tmp/kicad/api.sock",
		"--token", "secret-token",
		"--client-name", "test-client",
		"config",
	}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	output := stdout.String()
	for _, want := range []string{
		"socket_path: ipc:///tmp/kicad/api.sock",
		"client_name: test-client",
		"token: <redacted>",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected output to contain %q, got %s", want, output)
		}
	}
	if strings.Contains(output, "secret-token") {
		t.Fatalf("token leaked in output: %s", output)
	}
}

func TestRunPlaceRequestJSON(t *testing.T) {
	dir := t.TempDir()
	request := filepath.Join(dir, "placement.json")
	if err := os.WriteFile(request, []byte(`{
	  "Board": {"WidthMM": 40, "HeightMM": 25, "MarginMM": 1},
	  "Components": [{
	    "Ref": "R1",
	    "FootprintID": "Resistor_SMD:R_0805_2012Metric",
	    "Bounds": {"WidthMM": 2, "HeightMM": 1.25, "Source": "explicit"},
	    "Pads": [{"Name": "1"}, {"Name": "2"}]
	  }],
	  "Nets": [{"Name": "N1", "Endpoints": [{"Ref": "R1", "Pin": "1"}]}]
	}`), 0o644); err != nil {
		t.Fatalf("write request: %v", err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	if err := run([]string{"--json", "--request", request, "place", "request"}, &stdout, &stderr); err != nil {
		t.Fatalf("run returned error: %v\nstdout=%s", err, stdout.String())
	}
	output := stdout.String()
	for _, want := range []string{`"command": "place"`, `"status": "placed"`, `"op": "place_footprint"`, `"quality":`, `"ready": true`} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected output to contain %q, got %s", want, output)
		}
	}
}

func TestRunPlaceRequestRequiresRequestPath(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	if err := run([]string{"--json", "place", "request"}, &stdout, &stderr); err == nil || !strings.Contains(err.Error(), "place request requires --request") {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(stdout.String(), "place request requires --request") {
		t.Fatalf("expected missing request issue, got %s", stdout.String())
	}
}

func TestRunUnknownCommandReturnsUsage(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := run([]string{"bogus"}, &stdout, &stderr)
	if err == nil {
		t.Fatalf("run returned nil error")
	}
	if !strings.Contains(err.Error(), `unknown command "bogus"`) || !strings.Contains(err.Error(), "Usage:") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunLibraryRequiresJSON(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{"library", "index"}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "library requires --json") {
		t.Fatalf("err = %v", err)
	}
}

func TestRunLibraryIndexJSON(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	symbols, footprints := writeCLILibraryFixture(t)
	err := run([]string{"--json", "--symbols-root", symbols, "--footprints-root", footprints, "library", "index"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run returned error: %v\n%s", err, stdout.String())
	}
	for _, want := range []string{`"symbol_count": 1`, `"footprint_count": 1`, `"symbol_file_count": 1`, `"footprint_file_count": 1`} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("output missing %q:\n%s", want, stdout.String())
		}
	}
}

func TestRunLibrarySymbolAndFootprintJSON(t *testing.T) {
	symbols, footprints := writeCLILibraryFixture(t)
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{name: "symbol", args: []string{"library", "symbol", "Device:R"}, want: `"library_id": "Device:R"`},
		{name: "footprint", args: []string{"library", "footprint", "Resistor_SMD:R_0805_2012Metric"}, want: `"footprint_id": "Resistor_SMD:R_0805_2012Metric"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			args := append([]string{"--json", "--symbols-root", symbols, "--footprints-root", footprints}, tc.args...)
			err := run(args, &stdout, &stderr)
			if err != nil {
				t.Fatalf("run returned error: %v\n%s", err, stdout.String())
			}
			if !strings.Contains(stdout.String(), tc.want) {
				t.Fatalf("output missing %q:\n%s", tc.want, stdout.String())
			}
		})
	}
}

func TestRunLibrarySearchJSON(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	symbols, footprints := writeCLILibraryFixture(t)
	err := run([]string{"--json", "--symbols-root", symbols, "--footprints-root", footprints, "library", "search-symbols", "resistor"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run returned error: %v\n%s", err, stdout.String())
	}
	if !strings.Contains(stdout.String(), `"library_id": "Device:R"`) {
		t.Fatalf("unexpected output:\n%s", stdout.String())
	}
}

func TestRunLibraryCompatibilityJSON(t *testing.T) {
	symbols, footprints := writeCLILibraryFixture(t)
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{name: "compatible-footprints", args: []string{"library", "compatible-footprints", "Device:R"}, want: `"status": "compatible"`},
		{name: "validate-assignment", args: []string{"library", "validate-assignment", "Device:R", "Resistor_SMD:R_0805_2012Metric"}, want: `"status": "compatible"`},
		{name: "pinmap-candidate", args: []string{"library", "pinmap-candidate", "Device:R", "Resistor_SMD:R_0805_2012Metric"}, want: `"pinmap_candidate"`},
		{name: "klc-symbol", args: []string{"library", "klc-symbol", "Device:R"}, want: `"kind": "symbol"`},
		{name: "klc-footprint", args: []string{"library", "klc-footprint", "Resistor_SMD:R_0805_2012Metric"}, want: `"kind": "footprint"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			args := append([]string{"--json", "--symbols-root", symbols, "--footprints-root", footprints}, tc.args...)
			err := run(args, &stdout, &stderr)
			if err != nil {
				t.Fatalf("run returned error: %v\n%s", err, stdout.String())
			}
			if !strings.Contains(stdout.String(), tc.want) {
				t.Fatalf("output missing %q:\n%s", tc.want, stdout.String())
			}
		})
	}
}

func TestRunLibraryTemplatesJSON(t *testing.T) {
	templates := writeCLITemplateFixture(t)
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{name: "templates", args: []string{"library", "templates"}, want: `"name": "Demo"`},
		{name: "template", args: []string{"library", "template", "Demo"}, want: `"project_files": [`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			args := append([]string{"--json", "--templates-root", templates}, tc.args...)
			err := run(args, &stdout, &stderr)
			if err != nil {
				t.Fatalf("run returned error: %v\n%s", err, stdout.String())
			}
			if !strings.Contains(stdout.String(), tc.want) {
				t.Fatalf("output missing %q:\n%s", tc.want, stdout.String())
			}
		})
	}
}

func TestRunLibraryUnsupportedSubcommandJSON(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{"--json", "library", "bogus"}, &stdout, &stderr)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(stdout.String(), `"unsupported library subcommand bogus"`) {
		t.Fatalf("unexpected output:\n%s", stdout.String())
	}
}

func TestRunLibraryMissingIDJSON(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	symbols, footprints := writeCLILibraryFixture(t)
	err := run([]string{"--json", "--symbols-root", symbols, "--footprints-root", footprints, "library", "symbol", "Device:Missing"}, &stdout, &stderr)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(stdout.String(), `"code": "MISSING_FILE"`) || !strings.Contains(stdout.String(), `library record not found: Device:Missing`) {
		t.Fatalf("unexpected output:\n%s", stdout.String())
	}
}

func TestWriteReportJSON(t *testing.T) {
	var stdout bytes.Buffer

	err := writeReportJSON(&stdout, reports.ErrorResult("inspect-project", reports.Issue{
		Code:     reports.CodeMissingFile,
		Severity: reports.SeverityError,
		Path:     "project",
		Message:  "project path does not exist",
	}))
	if err != nil {
		t.Fatalf("writeReportJSON returned error: %v", err)
	}
	output := stdout.String()
	for _, want := range []string{
		`"ok": false`,
		`"command": "inspect-project"`,
		`"code": "MISSING_FILE"`,
		`"path": "project"`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func writeCLILibraryFixture(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	symbols := filepath.Join(root, "symbols")
	footprints := filepath.Join(root, "footprints")
	writeTestFile(t, filepath.Join(symbols, "Device.kicad_sym"), `
(kicad_symbol_lib
  (version 20220914)
  (generator "kicadai-test")
  (symbol "R"
    (property "Reference" "R" (at 0 0 0))
    (property "Value" "R" (at 0 -2.54 0))
    (ki_keywords "resistor R")
    (ki_description "Resistor")
    (ki_fp_filters "R_*" "Resistor_*")
    (symbol "R_1_1"
      (pin passive line (at -5.08 0 0) (length 2.54) (name "~") (number "1"))
      (pin passive line (at 5.08 0 180) (length 2.54) (name "~") (number "2"))
    )
  )
)`)
	writeTestFile(t, filepath.Join(footprints, "Resistor_SMD.pretty", "R_0805_2012Metric.kicad_mod"), `
(footprint "R_0805_2012Metric"
  (version 20240108)
  (generator "kicadai-test")
  (descr "Resistor SMD 0805")
  (tags "resistor 0805")
  (attr smd)
  (pad "1" smd roundrect (at -0.95 0) (size 1.0 1.2) (layers "F.Cu" "F.Paste" "F.Mask") (roundrect_rratio 0.25))
  (pad "2" smd roundrect (at 0.95 0) (size 1.0 1.2) (layers "F.Cu" "F.Paste" "F.Mask") (roundrect_rratio 0.25))
)`)
	return symbols, footprints
}

func writeCLITemplateFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	project := filepath.Join(root, "Projects", "Demo")
	writeTestFile(t, filepath.Join(project, "Demo.kicad_pro"), `{}`)
	writeTestFile(t, filepath.Join(project, "Demo.kicad_sch"), `(kicad_sch)`)
	writeTestFile(t, filepath.Join(project, "Demo.kicad_pcb"), `(kicad_pcb)`)
	writeTestFile(t, filepath.Join(project, "fp-lib-table"), ``)
	writeTestFile(t, filepath.Join(project, "meta", "info.html"), `<p>demo</p>`)
	return root
}

func writeTestFile(t *testing.T, path string, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRunHelpFlagPrintsUsage(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := run([]string{"--help"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run returned error: %v", err)
	}
	if !strings.Contains(stdout.String(), "Usage:") {
		t.Fatalf("expected usage, got %s", stdout.String())
	}
}

func TestRunHelpIncludesGenerateCommands(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	if err := run([]string{"help"}, &stdout, &stderr); err != nil {
		t.Fatalf("run returned error: %v", err)
	}
	for _, want := range []string{"generate-led-demo", "generate-project", "--output", "--with-pcb", "--overwrite"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("help missing %q:\n%s", want, stdout.String())
		}
	}
}

func TestRunHelpIncludesStructuredCommandFamilies(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	if err := run([]string{"help"}, &stdout, &stderr); err != nil {
		t.Fatalf("run returned error: %v", err)
	}
	for _, want := range []string{"inspect", "evaluate", "pinmap", "transaction", "roundtrip", "export", "generate", "block"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("help missing %q:\n%s", want, stdout.String())
		}
	}
}

func TestRunStructuredCommandRequiresJSON(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := run([]string{"inspect", "project", "demo"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "inspect requires --json") {
		t.Fatalf("error = %v", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("expected no stdout, got %s", stdout.String())
	}
}

func TestRunBlockRequiresJSON(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := run([]string{"block", "list"}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "block requires --json") {
		t.Fatalf("err = %v stdout=%s", err, stdout.String())
	}
}

func TestRunBlockListShowValidate(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	if err := run([]string{"--json", "block", "list"}, &stdout, &stderr); err != nil {
		t.Fatalf("list err = %v stdout=%s", err, stdout.String())
	}
	if !strings.Contains(stdout.String(), `"id": "connector_breakout"`) || !strings.Contains(stdout.String(), `"id": "led_indicator"`) {
		t.Fatalf("unexpected list output: %s", stdout.String())
	}

	stdout.Reset()
	if err := run([]string{"--json", "block", "show", "led_indicator"}, &stdout, &stderr); err != nil {
		t.Fatalf("show err = %v stdout=%s", err, stdout.String())
	}
	if !strings.Contains(stdout.String(), `"name": "LED Indicator"`) || !strings.Contains(stdout.String(), `"required_libraries"`) {
		t.Fatalf("unexpected show output: %s", stdout.String())
	}

	request := filepath.Join(t.TempDir(), "request.json")
	if err := os.WriteFile(request, []byte(`{"block_id":"led_indicator","instance_id":"status","params":{"active_high":true}}`), 0o644); err != nil {
		t.Fatalf("write request: %v", err)
	}
	stdout.Reset()
	if err := run([]string{"--json", "--request", request, "block", "validate", "led_indicator"}, &stdout, &stderr); err != nil {
		t.Fatalf("validate err = %v stdout=%s", err, stdout.String())
	}
	if !strings.Contains(stdout.String(), `"ok": true`) || !strings.Contains(stdout.String(), `"block_id": "led_indicator"`) {
		t.Fatalf("unexpected validate output: %s", stdout.String())
	}

	stdout.Reset()
	if err := run([]string{"--json", "--request", request, "block", "instantiate", "led_indicator"}, &stdout, &stderr); err != nil {
		t.Fatalf("instantiate err = %v stdout=%s", err, stdout.String())
	}
	for _, want := range []string{`"operations": [`, `"op": "add_symbol"`, `"op": "connect"`, `"refs": [`} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("instantiate output missing %q: %s", want, stdout.String())
		}
	}
}

func TestRunBlockReportsStructuredFailures(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := run([]string{"--json", "block", "show", "missing_block"}, &stdout, &stderr)
	if err == nil || !strings.Contains(stdout.String(), `"code": "MISSING_FILE"`) {
		t.Fatalf("missing err = %v stdout=%s", err, stdout.String())
	}

	stdout.Reset()
	request := filepath.Join(t.TempDir(), "request.json")
	if err := os.WriteFile(request, []byte(`{"block_id":"led_indicator","params":{"active_high":"yes"}}`), 0o644); err != nil {
		t.Fatalf("write request: %v", err)
	}
	err = run([]string{"--json", "--request", request, "block", "validate", "led_indicator"}, &stdout, &stderr)
	if err == nil || !strings.Contains(stdout.String(), `"code": "VALIDATION_FAILED"`) || !strings.Contains(stdout.String(), "active_high must be a bool") {
		t.Fatalf("invalid err = %v stdout=%s", err, stdout.String())
	}
}

func TestRunStructuredCommandRequiresSubcommand(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := run([]string{"--json", "inspect"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error")
	}
	output := stdout.String()
	for _, want := range []string{
		`"ok": false`,
		`"command": "inspect"`,
		`"code": "INVALID_ARGUMENT"`,
		`"message": "inspect subcommand required"`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func TestRunStructuredCommandRejectsUnknownSubcommand(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := run([]string{"--json", "evaluate", "gerbers", "demo"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error")
	}
	output := stdout.String()
	if !strings.Contains(output, `"code": "INVALID_ARGUMENT"`) ||
		!strings.Contains(output, `"unsupported evaluate subcommand gerbers"`) {
		t.Fatalf("unexpected output:\n%s", output)
	}
}

func TestRunStructuredCommandRejectsMissingTarget(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := run([]string{"--json", "roundtrip", "pcb"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error")
	}
	output := stdout.String()
	if !strings.Contains(output, `"code": "INVALID_ARGUMENT"`) ||
		!strings.Contains(output, `"roundtrip pcb requires 1 argument(s)"`) {
		t.Fatalf("unexpected output:\n%s", output)
	}
}

func TestRunStructuredCommandReturnsUnsupportedStub(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := run([]string{"--json", "export", "preview", "demo"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error")
	}
	output := stdout.String()
	for _, want := range []string{
		`"ok": false`,
		`"command": "export"`,
		`"code": "UNSUPPORTED_OPERATION"`,
		`"severity": "blocked"`,
		`"export command family is not implemented yet"`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func TestRunEvaluatePCBJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "board.kicad_pcb")
	if err := os.WriteFile(path, []byte(`(kicad_pcb (gr_rect (layer "Edge.Cuts")) (footprint "Test:One" (pad "1" smd rect (layers "F.Cu"))))`), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{"--json", "evaluate", "pcb", path}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run returned error: %v", err)
	}
	output := stdout.String()
	for _, want := range []string{
		`"ok": true`,
		`"command": "evaluate"`,
		`"target": "` + path + `"`,
		`"name": "pcb_validation"`,
		`"status": "passed"`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func TestRunEvaluateMissingTargetJSON(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := run([]string{"--json", "evaluate", "pcb", filepath.Join(t.TempDir(), "missing.kicad_pcb")}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error")
	}
	output := stdout.String()
	for _, want := range []string{
		`"ok": false`,
		`"command": "evaluate"`,
		`"code": "MISSING_FILE"`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func TestRunInspectProjectJSON(t *testing.T) {
	root := filepath.Join(t.TempDir(), "demo")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "demo.kicad_pro"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "demo.kicad_sch"), []byte(`(kicad_sch (version 20260306) (symbol))`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "demo.kicad_pcb"), []byte(`(kicad_pcb (gr_rect (layer "Edge.Cuts")) (footprint "Test:One" (pad "1" smd rect (layers "F.Cu"))))`), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{"--json", "inspect", "project", root}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run returned error: %v", err)
	}
	output := stdout.String()
	for _, want := range []string{
		`"ok": true`,
		`"command": "inspect"`,
		`"name": "demo"`,
		`"symbol_count": 1`,
		`"footprint_count": 1`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func TestRunPinmapListJSON(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{"--json", "pinmap", "list"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run returned error: %v", err)
	}
	output := stdout.String()
	for _, want := range []string{
		`"ok": true`,
		`"command": "pinmap"`,
		`"source": "human_verified"`,
		`"symbol": "Device:R"`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func TestRunPinmapValidateReportsBlockingIssues(t *testing.T) {
	root := filepath.Join(t.TempDir(), "demo")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "demo.kicad_pro"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "demo.kicad_sch"), []byte(`(kicad_sch
  (version 20260306)
  (generator "kicadai")
  (generator_version "10.0")
  (uuid "11111111-1111-5111-8111-111111111111")
  (paper A4)
  (symbol
    (lib_id "Device:U")
    (property "Reference" "U1")
    (property "Footprint" "Package_SO:SOIC-8")
    (uuid "33333333-3333-5333-8333-333333333333"))
)`), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{"--json", "pinmap", "validate", root}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error")
	}
	output := stdout.String()
	for _, want := range []string{
		`"ok": false`,
		`"command": "pinmap"`,
		`"code": "PINMAP_UNVERIFIED"`,
		`"fabrication_ready": false`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func TestRunEvaluateProjectWithBlockingIssuesReturnsError(t *testing.T) {
	root := filepath.Join(t.TempDir(), "demo")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "demo.kicad_pro"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{"--json", "evaluate", "project", root}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error")
	}
	output := stdout.String()
	for _, want := range []string{
		`"ok": false`,
		`"command": "evaluate"`,
		`"code": "MISSING_FILE"`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func TestRunRoundTripSkipsWhenKiCadCLIUnavailable(t *testing.T) {
	t.Setenv("KICADAI_KICAD_CLI", filepath.Join(t.TempDir(), "missing-kicad-cli"))
	path := filepath.Join(t.TempDir(), "demo.kicad_sch")
	if err := os.WriteFile(path, []byte(`(kicad_sch (version 20260306))`), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{"--json", "roundtrip", "schematic", path}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("skip should not fail command: %v", err)
	}
	output := stdout.String()
	for _, want := range []string{
		`"ok": true`,
		`"command": "roundtrip"`,
		`"code": "SKIPPED_EXTERNAL_TOOL"`,
		`"skipped": true`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func TestRunCheckSkipsWhenKiCadCLIUnavailable(t *testing.T) {
	t.Setenv("KICADAI_KICAD_CLI", filepath.Join(t.TempDir(), "missing-kicad-cli"))
	path := filepath.Join(t.TempDir(), "demo.kicad_sch")
	if err := os.WriteFile(path, []byte(`(kicad_sch)`), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{"--json", "check", "erc", path}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("skip should not fail command: %v", err)
	}
	output := stdout.String()
	for _, want := range []string{
		`"ok": true`,
		`"command": "check"`,
		`"code": "SKIPPED_EXTERNAL_TOOL"`,
		`"status": "skipped"`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func TestRunCheckERCWithFakeCLIReportsFindings(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "demo.kicad_sch")
	if err := os.WriteFile(path, []byte(`(kicad_sch)`), 0o644); err != nil {
		t.Fatal(err)
	}
	cli := fakeCheckCLI(t, 1, `{"coordinate_units":"mm","sheets":[{"path":"/","violations":[{"description":"Input Power pin not driven by any Output Power pins","severity":"error","type":"power_pin_not_driven","items":[{"description":"Symbol #PWR01 Pin 1","pos":{"x":1,"y":2},"uuid":"11111111-1111-4111-8111-111111111111"}]}]}]}`)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{"--json", "--kicad-cli", cli, "--keep-artifacts", "--artifact-dir", filepath.Join(dir, "artifacts"), "check", "erc", path}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected validation failure")
	}
	output := stdout.String()
	for _, want := range []string{
		`"ok": false`,
		`"command": "check"`,
		`"kind": "erc"`,
		`"status": "fail"`,
		`"power_pin_not_driven"`,
		`"erc_report"`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func TestRunCheckProjectWithFakeCLIReportsBothChecks(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "demo")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, contents := range map[string]string{
		"demo.kicad_pro": "{}",
		"demo.kicad_sch": `(kicad_sch)`,
		"demo.kicad_pcb": `(kicad_pcb)`,
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cli := fakeCheckCLI(t, 0, `{"coordinate_units":"mm","violations":[],"sheets":[]}`)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{"--json", "--kicad-cli", cli, "check", "project", dir}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run returned error: %v\n%s", err, stdout.String())
	}
	output := stdout.String()
	if strings.Count(output, `"status": "pass"`) != 2 {
		t.Fatalf("expected two passing checks:\n%s", output)
	}
}

func TestRunRoundTripSchematicWithFakeCLI(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "demo.kicad_sch")
	if err := os.WriteFile(path, []byte(`(kicad_sch (version 20260306))`), 0o644); err != nil {
		t.Fatal(err)
	}
	cli := fakeRoundTripCLI(t, filepath.Join(dir, "cli.log"), 0)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{"--json", "--kicad-cli", cli, "--keep-artifacts", "--artifact-dir", filepath.Join(dir, "artifacts"), "roundtrip", "schematic", path}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run returned error: %v\n%s", err, stdout.String())
	}
	output := stdout.String()
	for _, want := range []string{
		`"ok": true`,
		`"file_type": "schematic"`,
		`"equal": true`,
		`"roundtrip_report"`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func TestRunRoundTripReportsKiCadFailure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "board.kicad_pcb")
	if err := os.WriteFile(path, []byte(`(kicad_pcb)`), 0o644); err != nil {
		t.Fatal(err)
	}
	cli := fakeRoundTripCLI(t, filepath.Join(dir, "cli.log"), 3)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{"--json", "--kicad-cli", cli, "roundtrip", "pcb", path}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error")
	}
	output := stdout.String()
	for _, want := range []string{
		`"ok": false`,
		`"code": "KICAD_CLI_FAILED"`,
		`"file_type": "pcb"`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func TestRunRoundTripRejectsNonExecutableKiCadCLI(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("windows does not use Unix execute bits")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "board.kicad_pcb")
	if err := os.WriteFile(path, []byte(`(kicad_pcb)`), 0o644); err != nil {
		t.Fatal(err)
	}
	cli := filepath.Join(dir, "not-executable")
	if err := os.WriteFile(cli, []byte("#!/bin/sh\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{"--json", "--kicad-cli", cli, "roundtrip", "pcb", path}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(stdout.String(), `"code": "KICAD_CLI_FAILED"`) || !strings.Contains(stdout.String(), "not executable") {
		t.Fatalf("unexpected output:\n%s", stdout.String())
	}
}

func TestRunRoundTripProjectDiscoversRootFiles(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "demo")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, contents := range map[string]string{
		"demo.kicad_pro": "{}",
		"demo.kicad_sch": `(kicad_sch (version 20260306))`,
		"demo.kicad_pcb": `(kicad_pcb)`,
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cli := fakeRoundTripCLI(t, filepath.Join(dir, "cli.log"), 0)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{"--json", "--kicad-cli", cli, "roundtrip", "project", dir}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run returned error: %v\n%s", err, stdout.String())
	}
	output := stdout.String()
	if strings.Count(output, `"equal": true`) != 2 {
		t.Fatalf("expected two successful checks:\n%s", output)
	}
}

func TestRunGenerateStructuredCommandAllowsNoTarget(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := run([]string{"--json", "generate", "example"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(stdout.String(), `"code": "UNSUPPORTED_OPERATION"`) {
		t.Fatalf("unexpected output:\n%s", stdout.String())
	}
}

func TestRunGenerateBreakoutJSON(t *testing.T) {
	dir := t.TempDir()
	request := filepath.Join(dir, "request.json")
	output := filepath.Join(dir, "out")
	body := `{
	  "kind":"breakout_board",
	  "name":"sensor_breakout",
	  "board":{"width_mm":50,"height_mm":30},
	  "connectors":[
	    {"ref":"J1","pins":["VCC","GND","SCL","SDA"]},
	    {"ref":"J2","pins":["VCC","GND","SCL","SDA"]}
	  ]
	}`
	if err := os.WriteFile(request, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := run([]string{"--json", "--request", request, "--output", output, "generate", "breakout"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run returned error: %v\n%s", err, stdout.String())
	}
	text := stdout.String()
	for _, want := range []string{`"ok": true`, `"command": "generate"`, `"kind": "pcb"`, `sensor_breakout.kicad_pcb`} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected output to contain %q:\n%s", want, text)
		}
	}
	if _, err := os.Stat(filepath.Join(output, "sensor_breakout.kicad_pcb")); err != nil {
		t.Fatalf("expected generated PCB: %v", err)
	}
}

func TestRunBlockInstantiateWritesProject(t *testing.T) {
	output := filepath.Join(t.TempDir(), "status_led")
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := run([]string{"--json", "--output", output, "--name", "status_led", "block", "instantiate", "led_indicator"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run returned error: %v\n%s", err, stdout.String())
	}
	text := stdout.String()
	for _, want := range []string{`"ok": true`, `"command": "block"`, `"apply_result"`, `"kicad_project"`} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected output to contain %q:\n%s", want, text)
		}
	}
	for _, name := range []string{"status_led.kicad_pro", "status_led.kicad_sch", "status_led.kicad_pcb"} {
		if _, err := os.Stat(filepath.Join(output, name)); err != nil {
			t.Fatalf("expected generated %s: %v", name, err)
		}
	}
}

func TestRunBlockComposeWritesProject(t *testing.T) {
	dir := t.TempDir()
	request := filepath.Join(dir, "request.json")
	output := filepath.Join(dir, "composed")
	body := `{
	  "project_name": "composed",
	  "instances": [
	    {"id": "header", "block_id": "connector_breakout", "params": {"pin_names": ["SIG", "GND"]}},
	    {"id": "status", "block_id": "led_indicator"}
	  ],
	  "connections": [
	    {"from": {"instance_id": "header", "port": "SIG"}, "to": {"instance_id": "status", "port": "IN"}, "net_alias": "LED_EN"}
	  ]
	}`
	if err := os.WriteFile(request, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := run([]string{"--json", "--request", request, "--output", output, "block", "compose"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run returned error: %v\n%s", err, stdout.String())
	}
	text := stdout.String()
	for _, want := range []string{`"ok": true`, `"command": "block"`, `"project_name": "composed"`, `"kind": "schematic"`} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected output to contain %q:\n%s", want, text)
		}
	}
	for _, name := range []string{"composed.kicad_pro", "composed.kicad_sch", "composed.kicad_pcb"} {
		if _, err := os.Stat(filepath.Join(output, name)); err != nil {
			t.Fatalf("expected generated %s: %v", name, err)
		}
	}
}

func TestRunBlockComposeRequiresRequest(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := run([]string{"--json", "block", "compose"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(stdout.String(), `"block compose requires --request"`) {
		t.Fatalf("unexpected output:\n%s", stdout.String())
	}
}

func TestRunBlockComposeRejectsEmptyRequest(t *testing.T) {
	request := filepath.Join(t.TempDir(), "request.json")
	if err := os.WriteFile(request, []byte(`{"project_name":"empty"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := run([]string{"--json", "--request", request, "block", "compose"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(stdout.String(), `"composition request must contain at least one instance"`) {
		t.Fatalf("unexpected output:\n%s", stdout.String())
	}
}

func TestRunBlockRequiresSubcommand(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := run([]string{"--json", "block"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(stdout.String(), `"block requires a subcommand"`) {
		t.Fatalf("unexpected output:\n%s", stdout.String())
	}
}

func TestRunBlockMissingBlockReturnsError(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := run([]string{"--json", "block", "show", "missing_block"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(stdout.String(), `"block not found: missing_block"`) {
		t.Fatalf("unexpected output:\n%s", stdout.String())
	}
}

func fakeRoundTripCLI(t *testing.T, logPath string, upgradeExit int) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "kicad-cli")
	var body string
	if runtime.GOOS == "windows" {
		path += ".bat"
		body = "@echo off\r\n" +
			"if \"%1\"==\"--version\" echo 10.0.0& exit /b 0\r\n" +
			"echo %* > \"" + logPath + "\"\r\n" +
			"if not \"" + strconv.Itoa(upgradeExit) + "\"==\"0\" echo failed 1>&2\r\n" +
			"exit /b " + strconv.Itoa(upgradeExit) + "\r\n"
	} else {
		body = "#!/bin/sh\n" +
			"if [ \"$1\" = \"--version\" ]; then echo 10.0.0; exit 0; fi\n" +
			"printf '%s\\n' \"$*\" >> '" + logPath + "'\n"
		if upgradeExit != 0 {
			body += "printf '%s\\n' failed >&2\n"
		}
		body += "exit " + strconv.Itoa(upgradeExit) + "\n"
	}
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("write fake KiCad CLI: %v", err)
	}
	return path
}

func fakeCheckCLI(t *testing.T, exitCode int, reportJSON string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "kicad-cli")
	if runtime.GOOS == "windows" {
		path += ".bat"
		t.Skip("fake check CLI helper is implemented for POSIX shells")
	}
	body := "#!/bin/sh\n" +
		"if [ \"$1\" = \"--version\" ]; then echo 10.0.0; exit 0; fi\n" +
		"out=''\n" +
		"prev=''\n" +
		"for arg in \"$@\"; do\n" +
		"  if [ \"$prev\" = \"--output\" ]; then out=\"$arg\"; fi\n" +
		"  prev=\"$arg\"\n" +
		"done\n" +
		"if [ -n \"$out\" ]; then cat > \"$out\" <<'EOF'\n" +
		reportJSON + "\nEOF\n" +
		"fi\n" +
		"exit " + strconv.Itoa(exitCode) + "\n"
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("write fake check CLI: %v", err)
	}
	return path
}

func TestRunTransactionPlanRequiresProjectAndTransaction(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := run([]string{"--json", "transaction", "plan", "demo"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(stdout.String(), `"code": "INVALID_ARGUMENT"`) ||
		!strings.Contains(stdout.String(), `"transaction plan requires 2 argument(s)"`) {
		t.Fatalf("unexpected output:\n%s", stdout.String())
	}
}

func TestRunTransactionValidateJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tx.json")
	if err := os.WriteFile(path, []byte(`{"operations":[{"op":"create_project","name":"demo"},{"op":"write_project"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{"--json", "transaction", "validate", path}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run returned error: %v\n%s", err, stdout.String())
	}
	output := stdout.String()
	for _, want := range []string{
		`"ok": true`,
		`"command": "transaction"`,
		`"operation_count": 2`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func TestRunRouteRequestJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "route.json")
	request := `{
  "board":{"width_mm":30,"height_mm":20,"layers":[{"name":"F.Cu","kind":"copper","routable":true}]},
  "components":[
    {"ref":"J1","position":{"x_mm":5,"y_mm":10,"layer":"F.Cu"},"pads":[{"name":"1","net":"SIG","shape":"circle","type":"smd","size":{"width_mm":1,"height_mm":1},"layers":["F.Cu"]}]},
    {"ref":"J2","position":{"x_mm":20,"y_mm":10,"layer":"F.Cu"},"pads":[{"name":"1","net":"SIG","shape":"circle","type":"smd","size":{"width_mm":1,"height_mm":1},"layers":["F.Cu"]}]}
  ],
  "nets":[{"name":"SIG","endpoints":[{"ref":"J1","pin":"1"},{"ref":"J2","pin":"1"}]}],
  "rules":{"grid_mm":1,"trace_width_mm":0.1,"clearance_mm":0.01,"edge_clearance_mm":0.01},
  "strategy":{"mode":"single_layer"}
}`
	if err := os.WriteFile(path, []byte(request), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{"--json", "--request", path, "route", "request"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run returned error: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
	output := stdout.String()
	for _, want := range []string{`"ok": true`, `"command": "route"`, `"status": "routed"`, `"operations"`} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func TestRunRouteRejectsInvalidMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "route.json")
	if err := os.WriteFile(path, []byte(`{"board":{"width_mm":1,"height_mm":1},"rules":{},"strategy":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{"--json", "--request", path, "--mode", "diagonal", "route", "request"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected invalid mode error")
	}
	if !strings.Contains(stdout.String(), `unsupported route mode`) {
		t.Fatalf("unexpected output:\n%s", stdout.String())
	}
}

func TestRunTransactionValidateReportsIssues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tx.json")
	if err := os.WriteFile(path, []byte(`{"operations":[{"op":"route","net_name":"","points":[{"x_mm":0,"y_mm":0}]}]}`), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{"--json", "transaction", "validate", path}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error")
	}
	output := stdout.String()
	for _, want := range []string{
		`"ok": false`,
		`"code": "INVALID_ARGUMENT"`,
		`"path": "operations[0].net_name"`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func TestRunTransactionPlanJSON(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "out")
	path := filepath.Join(t.TempDir(), "tx.json")
	if err := os.WriteFile(path, []byte(`{"operations":[{"op":"create_project","name":"demo"},{"op":"add_symbol","ref":"R1","library_id":"Device:R","at":{"x_mm":0,"y_mm":0}},{"op":"write_project"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{"--json", "transaction", "plan", dir, path}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run returned error: %v\n%s", err, stdout.String())
	}
	output := stdout.String()
	for _, want := range []string{
		`"ok": true`,
		`"op": "add_symbol"`,
		`"refs": [`,
		`demo.kicad_sch`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func TestRunTransactionPlanBlocksExistingProject(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "demo.kicad_pro"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "tx.json")
	if err := os.WriteFile(path, []byte(`{"operations":[{"op":"create_project","name":"demo"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{"--json", "transaction", "plan", dir, path}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(stdout.String(), `"path": "operations[0].op"`) {
		t.Fatalf("unexpected output:\n%s", stdout.String())
	}
}

func TestRunTransactionApplyJSON(t *testing.T) {
	output := filepath.Join(t.TempDir(), "demo")
	path := filepath.Join(t.TempDir(), "tx.json")
	if err := os.WriteFile(path, []byte(`{"operations":[
	  {"op":"create_project","name":"demo"},
	  {"op":"add_symbol","ref":"R1","library_id":"Device:R","at":{"x_mm":10,"y_mm":10},"pins":[{"number":"1","x_mm":-2.54},{"number":"2","x_mm":2.54}]},
	  {"op":"assign_footprint","ref":"R1","footprint_id":"Resistor_SMD:R_0805_2012Metric"},
	  {"op":"place_footprint","ref":"R1","at":{"x_mm":20,"y_mm":20}},
	  {"op":"write_project"}
	]}`), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run([]string{"--json", "transaction", "apply", output, path}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run returned error: %v\n%s", err, stdout.String())
	}
	if !strings.Contains(stdout.String(), `"ok": true`) || !strings.Contains(stdout.String(), `"kicad_project"`) {
		t.Fatalf("unexpected output:\n%s", stdout.String())
	}
	if _, err := os.Stat(filepath.Join(output, "demo.kicad_pcb")); err != nil {
		t.Fatalf("PCB not written: %v", err)
	}
}

func TestRunStructuredCommandRejectsExtraArguments(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := run([]string{"--json", "inspect", "project", "demo", "extra"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(stdout.String(), `"code": "INVALID_ARGUMENT"`) ||
		!strings.Contains(stdout.String(), `"inspect project received 1 unexpected argument(s)"`) {
		t.Fatalf("unexpected output:\n%s", stdout.String())
	}
}

func TestRunGenerateLEDDemoJSON(t *testing.T) {
	root := filepath.Join(t.TempDir(), "demo-output")
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := run([]string{
		"--output", root,
		"--name", "led_indicator",
		"--seed", "cli-test",
		"--lib-resistor", "Custom:R_US",
		"--with-pcb",
		"--json",
		"generate-led-demo",
	}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	var result generationResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, stdout.String())
	}
	if result.ProjectName != "led_indicator" || len(result.WrittenFiles) != 3 {
		t.Fatalf("unexpected result: %+v", result)
	}
	if _, err := os.Stat(filepath.Join(root, "led_indicator.kicad_pcb")); err != nil {
		t.Fatalf("PCB not written: %v", err)
	}
	schematicBytes, err := os.ReadFile(filepath.Join(root, "led_indicator.kicad_sch"))
	if err != nil {
		t.Fatalf("schematic not readable: %v", err)
	}
	if !strings.Contains(string(schematicBytes), "Custom:R_US") {
		t.Fatalf("custom library ID missing from schematic:\n%s", schematicBytes)
	}
}

func TestRunGenerateLEDDemoRefusesOverwrite(t *testing.T) {
	root := filepath.Join(t.TempDir(), "led_indicator")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := run([]string{
		"--output", root,
		"--json",
		"generate-led-demo",
	}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(stdout.String(), `"target exists`) {
		t.Fatalf("expected target exists JSON, got %s", stdout.String())
	}
}

func TestRunGenerateProjectOmitsPCBByDefault(t *testing.T) {
	root := filepath.Join(t.TempDir(), "led_indicator")
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := run([]string{
		"--output", root,
		"--json",
		"generate-project",
	}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run returned error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "led_indicator.kicad_pcb")); !os.IsNotExist(err) {
		t.Fatalf("PCB should be omitted by default, stat error: %v", err)
	}
}

func TestRunGenerateRejectsCurrentDirectoryOutput(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := run([]string{
		"--output", ".",
		"--json",
		"generate-project",
	}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(stdout.String(), "project directory") {
		t.Fatalf("expected project directory error JSON, got %s", stdout.String())
	}
}

func TestRunPingJSON(t *testing.T) {
	app := appWithClientFactory(func(ctx context.Context, cfg config.Config) (apiClient, error) {
		return &fakeAPIClient{}, nil
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := app.run([]string{
		"--socket", "ipc:///tmp/kicad/api.sock",
		"--client-name", "test-client",
		"--json",
		"ping",
	}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	output := stdout.String()
	for _, want := range []string{
		`"socket_path": "ipc:///tmp/kicad/api.sock"`,
		`"client_name": "test-client"`,
		`"reachable": true`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected output to contain %q, got %s", want, output)
		}
	}
}

func TestRunPingJSONConnectFailure(t *testing.T) {
	want := errors.New("dial failed")
	app := appWithClientFactory(func(ctx context.Context, cfg config.Config) (apiClient, error) {
		return nil, want
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := app.run([]string{
		"--socket", "ipc:///tmp/kicad/api.sock",
		"--client-name", "test-client",
		"--json",
		"ping",
	}, &stdout, &stderr)
	if !errors.Is(err, want) {
		t.Fatalf("run error = %v, want %v", err, want)
	}
	output := stdout.String()
	for _, wantText := range []string{
		`"socket_path": "ipc:///tmp/kicad/api.sock"`,
		`"client_name": "test-client"`,
		`"reachable": false`,
		`"error": "dial failed"`,
	} {
		if !strings.Contains(output, wantText) {
			t.Fatalf("expected output to contain %q, got %s", wantText, output)
		}
	}
}

func TestRunPingTextConnectFailureDoesNotWriteJSON(t *testing.T) {
	want := errors.New("dial failed")
	app := appWithClientFactory(func(ctx context.Context, cfg config.Config) (apiClient, error) {
		return nil, want
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := app.run([]string{
		"--socket", "ipc:///tmp/kicad/api.sock",
		"ping",
	}, &stdout, &stderr)
	if !errors.Is(err, want) {
		t.Fatalf("run error = %v, want %v", err, want)
	}
	if stdout.Len() != 0 {
		t.Fatalf("expected no text output on connect failure, got %s", stdout.String())
	}
}

func TestRunVersionJSON(t *testing.T) {
	app := appWithClientFactory(func(ctx context.Context, cfg config.Config) (apiClient, error) {
		return &fakeAPIClient{
			version: &commontypes.KiCadVersion{Major: 9, Minor: 1, Patch: 0, FullVersion: "9.1.0"},
		}, nil
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := app.run([]string{
		"--socket", "ipc:///tmp/kicad/api.sock",
		"--client-name", "test-client",
		"--json",
		"version",
	}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	output := stdout.String()
	for _, want := range []string{
		`"reachable": true`,
		`"full_version": "9.1.0"`,
		`"major": 9`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected output to contain %q, got %s", want, output)
		}
	}
}

func TestRunVersionJSONFailureReturnsStructuredResult(t *testing.T) {
	want := errors.New("version failed")
	app := appWithClientFactory(func(ctx context.Context, cfg config.Config) (apiClient, error) {
		return &fakeAPIClient{versionErr: want}, nil
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := app.run([]string{
		"--socket", "ipc:///tmp/kicad/api.sock",
		"--json",
		"version",
	}, &stdout, &stderr)
	if !errors.Is(err, want) {
		t.Fatalf("run error = %v, want %v", err, want)
	}
	if !strings.Contains(stdout.String(), `"reachable": false`) || !strings.Contains(stdout.String(), `"error": "version failed"`) {
		t.Fatalf("expected structured version failure, got %s", stdout.String())
	}
}

func TestRunPingJSONFailureReturnsError(t *testing.T) {
	want := errors.New("not reachable")
	app := appWithClientFactory(func(ctx context.Context, cfg config.Config) (apiClient, error) {
		return &fakeAPIClient{pingErr: want}, nil
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := app.run([]string{
		"--socket", "ipc:///tmp/kicad/api.sock",
		"--json",
		"ping",
	}, &stdout, &stderr)
	if !errors.Is(err, want) {
		t.Fatalf("run error = %v, want %v", err, want)
	}
	if !strings.Contains(stdout.String(), `"reachable": false`) {
		t.Fatalf("expected failure JSON, got %s", stdout.String())
	}
}

func TestRunDocumentsInvalidType(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := run([]string{
		"--document-type", "invalid",
		"documents",
	}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "unsupported document type") {
		t.Fatalf("run error = %v, want unknown document type", err)
	}
}

func TestRunDocumentsJSON(t *testing.T) {
	app := appWithClientFactory(func(ctx context.Context, cfg config.Config) (apiClient, error) {
		return &fakeAPIClient{
			documents: []kiapi.Document{{
				Type:       kiapi.DocumentTypeSchematic,
				Identifier: "/",
				SheetPath:  "/",
			}},
		}, nil
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := app.run([]string{
		"--socket", "ipc:///tmp/kicad/api.sock",
		"--document-type", "schematic",
		"--json",
		"documents",
	}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	output := stdout.String()
	for _, want := range []string{
		`"documents": [`,
		`"type": "schematic"`,
		`"sheet_path": "/"`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected output to contain %q, got %s", want, output)
		}
	}
}

func TestRunDocumentsJSONFailureReturnsStructuredResult(t *testing.T) {
	want := errors.New("documents failed")
	app := appWithClientFactory(func(ctx context.Context, cfg config.Config) (apiClient, error) {
		return &fakeAPIClient{documentsErr: want}, nil
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := app.run([]string{
		"--socket", "ipc:///tmp/kicad/api.sock",
		"--json",
		"documents",
	}, &stdout, &stderr)
	if !errors.Is(err, want) {
		t.Fatalf("run error = %v, want %v", err, want)
	}
	output := stdout.String()
	if !strings.Contains(output, `"documents": []`) || !strings.Contains(output, `"error": "documents failed"`) {
		t.Fatalf("expected structured documents failure, got %s", output)
	}
}

func TestRunCapabilitiesJSON(t *testing.T) {
	app := appWithClientFactory(func(ctx context.Context, cfg config.Config) (apiClient, error) {
		return &fakeAPIClient{
			version: &commontypes.KiCadVersion{Major: 9, Minor: 1, Patch: 0, FullVersion: "9.1.0"},
		}, nil
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := app.run([]string{
		"--socket", "ipc:///tmp/kicad/api.sock",
		"--json",
		"capabilities",
	}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	var capabilities kiapi.Capabilities
	if err := json.Unmarshal(stdout.Bytes(), &capabilities); err != nil {
		t.Fatalf("unmarshal capabilities JSON: %v", err)
	}
	if capabilities.KiCadVersion != "9.1.0" {
		t.Fatalf("KiCadVersion = %q", capabilities.KiCadVersion)
	}
	if !capabilities.Supports(kiapi.CapabilitySchematicRead) {
		t.Fatalf("expected schematic read in supported capabilities")
	}
	for _, missing := range []kiapi.Capability{
		kiapi.CapabilitySchematicWrite,
		kiapi.CapabilitySymbolPlace,
	} {
		if capabilities.Supports(missing) {
			t.Fatalf("expected %s to be missing, got supported", missing)
		}
	}
}

func TestRunCapabilitiesJSONVersionFailure(t *testing.T) {
	want := errors.New("version failed")
	app := appWithClientFactory(func(ctx context.Context, cfg config.Config) (apiClient, error) {
		return &fakeAPIClient{versionErr: want}, nil
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := app.run([]string{
		"--socket", "ipc:///tmp/kicad/api.sock",
		"--json",
		"capabilities",
	}, &stdout, &stderr)
	if !errors.Is(err, want) {
		t.Fatalf("run error = %v, want %v", err, want)
	}
	if !strings.Contains(stdout.String(), `"kicad_version": "unknown"`) || !strings.Contains(stdout.String(), `"error": "version failed"`) {
		t.Fatalf("expected structured capabilities failure, got %s", stdout.String())
	}
}

func TestRunPlanLEDDemoJSON(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := run([]string{
		"--document", "/",
		"--origin-x", "1000",
		"--origin-y", "2000",
		"--prefix", "STATUS",
		"--lib-resistor", "Custom:R_US",
		"--json",
		"plan-led-demo",
	}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	output := stdout.String()
	for _, want := range []string{
		`"operations": [`,
		`"kind": "add_symbol"`,
		`Custom:R_US`,
		`STATUS_OUT`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected output to contain %q, got %s", want, output)
		}
	}
}

func TestRunPlanLEDDemoJSONMissingDocument(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := run([]string{
		"--json",
		"plan-led-demo",
	}, &stdout, &stderr)
	if err == nil {
		t.Fatalf("run returned nil error")
	}
	if !strings.Contains(stdout.String(), `"success": false`) || !strings.Contains(stdout.String(), `"document is required"`) {
		t.Fatalf("expected structured planning error, got %s", stdout.String())
	}
}

func TestRunPlanLEDDemoTextOutput(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := run([]string{
		"--document", "/",
		"plan-led-demo",
	}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run returned error: %v", err)
	}
	if !strings.Contains(stdout.String(), "1. add_symbol") {
		t.Fatalf("expected text plan output, got %s", stdout.String())
	}
}

func TestRunDrawLEDDemoJSONRequiresExecute(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := run([]string{
		"--document", "/",
		"--json",
		"draw-led-demo",
	}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "requires --execute") {
		t.Fatalf("run error = %v, want --execute requirement", err)
	}
	if !strings.Contains(stdout.String(), `"success": false`) || !strings.Contains(stdout.String(), `"draw-led-demo requires --execute"`) {
		t.Fatalf("expected structured error result, got %s", stdout.String())
	}
}

func TestRunDrawLEDDemoJSONReportsMissingWriteCapability(t *testing.T) {
	app := appWithClientFactory(func(ctx context.Context, cfg config.Config) (apiClient, error) {
		return &fakeAPIClient{
			version: &commontypes.KiCadVersion{Major: 9, Minor: 1, Patch: 0, FullVersion: "9.1.0"},
		}, nil
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := app.run([]string{
		"--socket", "ipc:///tmp/kicad/api.sock",
		"--document", "/",
		"--execute",
		"--json",
		"draw-led-demo",
	}, &stdout, &stderr)
	if !errors.Is(err, workflows.ErrMissingSchematicWriteCapability) {
		t.Fatalf("run error = %v, want %v", err, workflows.ErrMissingSchematicWriteCapability)
	}
	if !strings.Contains(stdout.String(), `"operations_completed": 0`) {
		t.Fatalf("expected structured result, got %s", stdout.String())
	}
}

type fakeAPIClient struct {
	pingErr      error
	version      *commontypes.KiCadVersion
	versionErr   error
	documents    []kiapi.Document
	documentsErr error
	closeErr     error
}

func (c *fakeAPIClient) Ping(context.Context) error {
	return c.pingErr
}

func (c *fakeAPIClient) GetVersion(context.Context) (*commontypes.KiCadVersion, error) {
	if c.versionErr != nil {
		return nil, c.versionErr
	}
	return c.version, nil
}

func (c *fakeAPIClient) GetOpenDocuments(context.Context, kiapi.DocumentType) ([]kiapi.Document, error) {
	if c.documentsErr != nil {
		return nil, c.documentsErr
	}
	return c.documents, nil
}

func (c *fakeAPIClient) Close() error {
	return c.closeErr
}

func appWithClientFactory(factory func(context.Context, config.Config) (apiClient, error)) app {
	return app{newClient: factory}
}
