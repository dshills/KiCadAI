package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"kicadai/internal/circuitgraph"
	"kicadai/internal/kicadfiles/checks"
	"kicadai/internal/reports"
)

func TestCircuitPreflightReadyAndDeterministic(t *testing.T) {
	graph := filepath.Join("..", "..", "examples", "circuit-graph", "rc_filter.json")
	output := filepath.Join(t.TempDir(), "must-not-be-written")
	args := []string{"--request", graph, "--output", output, "circuit", "preflight"}
	first := runCircuitPreflightCLI(t, args)
	second := runCircuitPreflightCLI(t, args)
	if !reflect.DeepEqual(first, second) || !first.OK {
		t.Fatalf("preflight is not deterministic or ready: first=%#v second=%#v", first, second)
	}
	data := preflightResultData(t, first)
	if !data.ReadyForWrite || data.CapabilityProfile != "generic-circuit-v1" || data.InputContract == "" || data.Routing == nil || len(data.Gates) == 0 {
		t.Fatalf("preflight data = %#v", data)
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("preflight wrote output: %v", err)
	}
}

func TestAppendUniquePreflightIssuesPreservesOrder(t *testing.T) {
	first := reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityWarning, Path: "selection", Message: "selection warning"}
	second := reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityWarning, Path: "evidence", Message: "evidence warning"}
	got := appendUniquePreflightIssues([]reports.Issue{first}, []reports.Issue{first, second})
	if !reflect.DeepEqual(got, []reports.Issue{first, second}) {
		t.Fatalf("issues=%#v", got)
	}
}

func TestCircuitPreflightAcceptsDocumentedArgumentOrder(t *testing.T) {
	graph := filepath.Join("..", "..", "examples", "circuit-graph", "rc_filter.json")
	result := runCircuitPreflightCLI(t, []string{"circuit", "preflight", "--request", graph, "--json"})
	if !result.OK || !preflightResultData(t, result).ReadyForWrite {
		t.Fatalf("documented argument order result = %#v", result)
	}
}

func TestCircuitPreflightIgnoresUnreferencedLibraryDiagnostics(t *testing.T) {
	graph := writeCircuitCreateRCGraph(t)
	symbolsRoot, footprintsRoot := writeCircuitCreateLibraryFixture(t)
	var unrelated strings.Builder
	unrelated.WriteString(`(kicad_symbol_lib
  (version 20220914)
  (generator "kicadai-test")
  (symbol "HiddenPower"
    (property "Reference" "#PWR" (at 0 -2.54 0) hide)
    (property "Value" "HiddenPower" (at 0 2.54 0))
    (symbol "HiddenPower_1_1"
`)
	for index := 0; index < 160; index++ {
		fmt.Fprintf(&unrelated, "      (pin power_in line (at 0 0 90) (length 0) hide (name \"PWR\") (number \"%d\"))\n", index+1)
	}
	unrelated.WriteString("    )\n  )\n)\n")
	writeTestFile(t, filepath.Join(symbolsRoot, "Unrelated.kicad_sym"), unrelated.String())

	args := []string{
		"--symbols-root", symbolsRoot,
		"--footprints-root", footprintsRoot,
		"--request", graph,
		"circuit", "preflight",
	}
	var stdout, stderr bytes.Buffer
	_ = run(args, &stdout, &stderr)
	result := decodeSingleResultDocument(t, stdout.Bytes())
	if !result.OK || !preflightResultData(t, result).ReadyForWrite {
		t.Fatalf("unreferenced library diagnostic blocked design: %#v", result)
	}
	if stdout.Len() > 512_000 {
		t.Fatalf("stock-library regression output = %d bytes, want <= 512000", stdout.Len())
	}

	audit := runCircuitPreflightCLI(t, []string{
		"--json",
		"--catalog-dir", testComponentCatalogDir(t),
		"--symbols-root", symbolsRoot,
		"--footprints-root", footprintsRoot,
		"component", "validate",
	})
	if !resultHasIssueMessage(audit, "hidden power pin") {
		t.Fatalf("explicit library audit omitted unrelated diagnostic: %#v", audit)
	}
}

func TestCircuitPreflightBlocksReferencedLibraryDefects(t *testing.T) {
	for _, test := range []struct {
		name string
		edit func(t *testing.T, symbolsRoot string, footprintsRoot string)
		want string
	}{
		{
			name: "symbol_semantic_diagnostic",
			edit: func(t *testing.T, symbolsRoot string, _ string) {
				writeTestFile(t, filepath.Join(symbolsRoot, "Device.kicad_sym"), `
(kicad_symbol_lib (version 20220914) (generator "kicadai-test")
  (symbol "R"
    (property "Reference" "R" (at 0 0 0))
    (property "Value" "R" (at 0 -2.54 0))
    (symbol "R_1_1"
      (pin power_in line (at -5.08 0 0) (length 2.54) hide (name "PWR") (number "1"))
      (pin passive line (at 5.08 0 180) (length 2.54) (name "~") (number "2"))))
  (symbol "C"
    (property "Reference" "C" (at 0 0 0))
    (property "Value" "C" (at 0 -2.54 0))
    (symbol "C_1_1"
      (pin passive line (at -5.08 0 0) (length 2.54) (name "~") (number "1"))
      (pin passive line (at 5.08 0 180) (length 2.54) (name "~") (number "2")))))`)
			},
			want: "hidden power pin",
		},
		{
			name: "symbol_file_syntax",
			edit: func(t *testing.T, symbolsRoot string, _ string) {
				writeTestFile(t, filepath.Join(symbolsRoot, "Device.kicad_sym"), `(kicad_symbol_lib (symbol "R"`)
			},
			want: "referenced symbol is absent",
		},
		{
			name: "footprint_file_syntax",
			edit: func(t *testing.T, _ string, footprintsRoot string) {
				writeTestFile(t, filepath.Join(footprintsRoot, "Resistor_SMD.pretty", "R_0805_2012Metric.kicad_mod"), `(footprint "R_0805_2012Metric"`)
			},
			want: "referenced footprint is absent",
		},
		{
			name: "missing_inherited_base",
			edit: func(t *testing.T, symbolsRoot string, _ string) {
				writeTestFile(t, filepath.Join(symbolsRoot, "Device.kicad_sym"), `
(kicad_symbol_lib (version 20220914) (generator "kicadai-test")
  (symbol "R" (extends "Missing"))
  (symbol "C"
    (property "Reference" "C" (at 0 0 0))
    (property "Value" "C" (at 0 -2.54 0))
    (symbol "C_1_1"
      (pin passive line (at -5.08 0 0) (length 2.54) (name "~") (number "1"))
      (pin passive line (at 5.08 0 180) (length 2.54) (name "~") (number "2")))))`)
			},
			want: "referenced symbol is absent",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			graph := writeCircuitCreateRCGraph(t)
			symbolsRoot, footprintsRoot := writeCircuitCreateLibraryFixture(t)
			test.edit(t, symbolsRoot, footprintsRoot)
			result := runCircuitPreflightCLI(t, []string{
				"--symbols-root", symbolsRoot,
				"--footprints-root", footprintsRoot,
				"--request", graph,
				"circuit", "preflight",
			})
			if result.OK || preflightResultData(t, result).ReadyForWrite || !resultHasIssueMessage(result, test.want) {
				t.Fatalf("referenced defect did not block with %q: %#v", test.want, result)
			}
		})
	}
}

func TestCircuitPreflightIgnoresUnreferencedMalformedInheritance(t *testing.T) {
	graph := writeCircuitCreateRCGraph(t)
	symbolsRoot, footprintsRoot := writeCircuitCreateLibraryFixture(t)
	writeTestFile(t, filepath.Join(symbolsRoot, "Unrelated.kicad_sym"), `
(kicad_symbol_lib (version 20220914) (generator "kicadai-test")
  (symbol "BrokenChild" (extends "MissingBase")))`)
	result := runCircuitPreflightCLI(t, []string{
		"--symbols-root", symbolsRoot,
		"--footprints-root", footprintsRoot,
		"--request", graph,
		"circuit", "preflight",
	})
	if !result.OK || !preflightResultData(t, result).ReadyForWrite {
		t.Fatalf("unreferenced inherited symbol blocked design: %#v", result)
	}
}

func TestCircuitPreflightIncludesProjectSymbolTableInClosure(t *testing.T) {
	graph := writeCircuitCreateRCGraph(t)
	symbolsRoot, footprintsRoot := writeCircuitCreateLibraryFixture(t)
	projectDir := filepath.Dir(graph)
	writeTestFile(t, filepath.Join(projectDir, "sym-lib-table"), `
(sym_lib_table
  (version 7)
  (lib (name "Device")(type "KiCad")(uri "${KIPRJMOD}/local-device.kicad_sym")(options "")(descr "local generated symbols")))`)
	writeTestFile(t, filepath.Join(projectDir, "local-device.kicad_sym"), `
(kicad_symbol_lib (version 20220914) (generator "kicadai-test")
  (symbol "R"
    (property "Reference" "R" (at 0 0 0))
    (property "Value" "R" (at 0 -2.54 0))
    (symbol "R_1_1"
      (pin power_in line (at -5.08 0 0) (length 2.54) hide (name "PWR") (number "1"))
      (pin passive line (at 5.08 0 180) (length 2.54) (name "~") (number "2"))))
  (symbol "C"
    (property "Reference" "C" (at 0 0 0))
    (property "Value" "C" (at 0 -2.54 0))
    (symbol "C_1_1"
      (pin passive line (at -5.08 0 0) (length 2.54) (name "~") (number "1"))
      (pin passive line (at 5.08 0 180) (length 2.54) (name "~") (number "2")))))`)
	result := runCircuitPreflightCLI(t, []string{
		"--symbols-root", symbolsRoot,
		"--footprints-root", footprintsRoot,
		"--request", graph,
		"circuit", "preflight",
	})
	if result.OK || !resultHasIssueMessage(result, "hidden power pin") {
		t.Fatalf("project symbol table did not participate in closure: %#v", result)
	}
}

func TestCircuitHelpIsSuccessful(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
		want []string
	}{
		{name: "circuit", args: []string{"circuit", "--help"}, want: []string{"circuit <preflight|create", "circuit preflight --request"}},
		{name: "preflight", args: []string{"circuit", "preflight", "--help"}, want: []string{"circuit preflight --request", "--symbols-root"}},
		{name: "create", args: []string{"circuit", "create", "--help"}, want: []string{"circuit create --request", "--require-kicad-roundtrip"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if err := run(test.args, &stdout, &stderr); err != nil {
				t.Fatalf("help error: %v; stdout=%s stderr=%s", err, stdout.String(), stderr.String())
			}
			for _, want := range test.want {
				if !strings.Contains(stdout.String(), want) {
					t.Fatalf("help omitted %q: %s", want, stdout.String())
				}
			}
			if stderr.Len() != 0 {
				t.Fatalf("help wrote stderr: %s", stderr.String())
			}
		})
	}
}

func TestCircuitPreflightFailsClosedBeforeWrite(t *testing.T) {
	graph := filepath.Join("..", "..", "examples", "circuit-graph", "unsupported_unknown_component.json")
	output := filepath.Join(t.TempDir(), "must-not-be-written")
	result := runCircuitPreflightCLI(t, []string{"--request", graph, "--output", output, "circuit", "preflight"})
	data := preflightResultData(t, result)
	if result.OK || data.ReadyForWrite || len(result.Issues) == 0 || result.Issues[0].RetryScope == "" || len(data.RepairOptions) == 0 || data.RepairOptions[0].OperationTemplate.Op != "replace_component" {
		t.Fatalf("unsupported preflight = %#v", result)
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("preflight wrote output: %v", err)
	}
}

func TestCircuitPreflightFailsClosedForInvalidPinAndPlacement(t *testing.T) {
	base, err := os.ReadFile(filepath.Join("..", "..", "examples", "circuit-graph", "rc_filter.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name string
		edit func(string) string
	}{
		{name: "unknown_pin", edit: func(input string) string { return strings.Replace(input, `"selector": "1"`, `"selector": "999"`, 1) }},
		{name: "invalid_region", edit: func(input string) string { return strings.Replace(input, `"width_mm": 24,`, `"width_mm": 120,`, 1) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			graph := filepath.Join(t.TempDir(), test.name+".json")
			if err := os.WriteFile(graph, []byte(test.edit(string(base))), 0o600); err != nil {
				t.Fatal(err)
			}
			output := filepath.Join(t.TempDir(), "must-not-be-written")
			result := runCircuitPreflightCLI(t, []string{"--request", graph, "--output", output, "circuit", "preflight"})
			data := preflightResultData(t, result)
			if result.OK || data.ReadyForWrite || len(result.Issues) == 0 || len(data.RepairOptions) == 0 {
				t.Fatalf("invalid %s preflight = %#v", test.name, result)
			}
			if _, err := os.Stat(output); !os.IsNotExist(err) {
				t.Fatalf("preflight wrote output: %v", err)
			}
		})
	}
}

func TestCircuitPreflightFailsClosedForInvalidMultiUnitAssignment(t *testing.T) {
	recorded, err := os.ReadFile(filepath.Join("..", "..", "examples", "ai", "generic_lm358_buffered_signal_conditioner", "recorded-response.json"))
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		Intent json.RawMessage `json:"intent"`
	}
	if err := json.Unmarshal(recorded, &envelope); err != nil {
		t.Fatal(err)
	}
	invalid := strings.Replace(string(envelope.Intent), `"unit":"B"`, `"unit":"Z"`, 1)
	if invalid == string(envelope.Intent) {
		t.Fatal("LM358 fixture does not contain unit B")
	}
	graph := filepath.Join(t.TempDir(), "invalid_multi_unit.json")
	if err := os.WriteFile(graph, []byte(invalid), 0o600); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "must-not-be-written")
	result := runCircuitPreflightCLI(t, []string{"--request", graph, "--output", output, "circuit", "preflight"})
	data := preflightResultData(t, result)
	if result.OK || data.ReadyForWrite || len(result.Issues) == 0 || len(data.RepairOptions) == 0 || data.RepairOptions[0].OperationTemplate.Op != "replace_endpoint" {
		t.Fatalf("invalid multi-unit preflight = %#v", result)
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("preflight wrote output: %v", err)
	}
}

func TestCircuitCreateWritesPreflightReadyRCGraph(t *testing.T) {
	graph := writeCircuitCreateRCGraph(t)
	symbolsRoot, footprintsRoot := writeCircuitCreateLibraryFixture(t)
	cli := fakeWorkflowKiCadCLI(t, 0, passingWorkflowKiCadReport)
	output := filepath.Join(t.TempDir(), "project")
	result, err := runCircuitCreateCLI(t, []string{
		"--kicad-cli", cli, "--symbols-root", symbolsRoot, "--footprints-root", footprintsRoot,
		"circuit", "create", "--request", graph, "--output", output, "--overwrite",
	})
	if err != nil || !result.OK {
		t.Fatalf("circuit create err=%v result=%#v", err, result)
	}
	data := circuitCreateResultData(t, result)
	if !data.Preflight.ReadyForWrite || data.Workflow == nil || len(data.ProjectPaths) == 0 {
		t.Fatalf("circuit create data = %#v", data)
	}
	for _, name := range []string{"generic_rc_filter.kicad_pro", "generic_rc_filter.kicad_sch", "generic_rc_filter.kicad_pcb", ".kicadai/transaction.json"} {
		if _, statErr := os.Stat(filepath.Join(output, name)); statErr != nil {
			t.Fatalf("missing written %s: %v", name, statErr)
		}
	}
}

func TestCircuitCreateWritesSharedCoreEvidence(t *testing.T) {
	t.Skip("known F6: remove after circuit create uses the shared core evidence writer")
	graph := writeCircuitCreateRCGraph(t)
	symbolsRoot, footprintsRoot := writeCircuitCreateLibraryFixture(t)
	cli := fakeWorkflowKiCadCLI(t, 0, passingWorkflowKiCadReport)
	output := filepath.Join(t.TempDir(), "project")
	result, err := runCircuitCreateCLI(t, []string{
		"--kicad-cli", cli, "--symbols-root", symbolsRoot, "--footprints-root", footprintsRoot,
		"circuit", "create", "--request", graph, "--output", output, "--overwrite",
	})
	if err != nil || !result.OK {
		t.Fatalf("circuit create err=%v result=%#v", err, result)
	}
	for _, name := range []string{
		"design-request.json",
		"transaction.json",
		"workflow-result.json",
		"validation-summary.json",
		"design-promotion.json",
		"manifest.json",
	} {
		if _, statErr := os.Stat(filepath.Join(output, ".kicadai", name)); statErr != nil {
			t.Fatalf("missing shared core evidence %s: %v", name, statErr)
		}
	}
}

func TestCircuitCreateFailsClosedBeforeWrite(t *testing.T) {
	for _, graph := range []string{
		filepath.Join("..", "..", "examples", "circuit-graph", "unsupported_unknown_component.json"),
		writeInvalidCircuitCreateGraph(t),
	} {
		output := filepath.Join(t.TempDir(), "must-not-be-written")
		result, err := runCircuitCreateCLI(t, []string{"circuit", "create", "--request", graph, "--output", output})
		if err == nil || result.OK || len(result.Issues) == 0 {
			t.Fatalf("blocked circuit create graph=%s err=%v result=%#v", graph, err, result)
		}
		if _, statErr := os.Stat(output); !os.IsNotExist(statErr) {
			t.Fatalf("blocked circuit create wrote output: %v", statErr)
		}
	}
}

func TestCircuitCreateRCGraphIsDeterministic(t *testing.T) {
	graph := writeCircuitCreateRCGraph(t)
	symbolsRoot, footprintsRoot := writeCircuitCreateLibraryFixture(t)
	cli := fakeWorkflowKiCadCLI(t, 0, passingWorkflowKiCadReport)
	create := func(name string) (circuitCreateData, string) {
		t.Helper()
		output := filepath.Join(t.TempDir(), name)
		result, err := runCircuitCreateCLI(t, []string{
			"--kicad-cli", cli, "--symbols-root", symbolsRoot, "--footprints-root", footprintsRoot,
			"circuit", "create", "--request", graph, "--output", output, "--overwrite",
		})
		if err != nil || !result.OK {
			t.Fatalf("circuit create %s err=%v result=%#v", name, err, result)
		}
		data := circuitCreateResultData(t, result)
		return data, output
	}
	first, firstOutput := create("first")
	second, secondOutput := create("second")
	if string(mustJSON(t, first.Preflight.Graph)) != string(mustJSON(t, second.Preflight.Graph)) ||
		string(mustJSON(t, first.Preflight.Request)) != string(mustJSON(t, second.Preflight.Request)) ||
		string(mustJSON(t, first.Preflight.Placement)) != string(mustJSON(t, second.Preflight.Placement)) ||
		string(mustJSON(t, first.Preflight.Routing)) != string(mustJSON(t, second.Preflight.Routing)) {
		t.Fatalf("direct circuit create preflight evidence is not deterministic")
	}
	for _, file := range []string{"generic_rc_filter.kicad_pro", "generic_rc_filter.kicad_sch", "generic_rc_filter.kicad_pcb"} {
		firstContents, err := os.ReadFile(filepath.Join(firstOutput, file))
		if err != nil {
			t.Fatal(err)
		}
		secondContents, err := os.ReadFile(filepath.Join(secondOutput, file))
		if err != nil {
			t.Fatal(err)
		}
		if string(firstContents) != string(secondContents) {
			t.Fatalf("generated %s is not deterministic", file)
		}
	}
}

func TestCircuitCreateOptionalKiCadBMP280(t *testing.T) {
	cliPath := strings.TrimSpace(os.Getenv(checks.EnvKiCadCLI))
	symbolsRoot := strings.TrimSpace(os.Getenv("KICADAI_SYMBOLS_ROOT"))
	footprintsRoot := strings.TrimSpace(os.Getenv("KICADAI_FOOTPRINTS_ROOT"))
	if cliPath == "" || symbolsRoot == "" || footprintsRoot == "" {
		t.Skip("set KICADAI_KICAD_CLI, KICADAI_SYMBOLS_ROOT, and KICADAI_FOOTPRINTS_ROOT to run direct BMP280 KiCad promotion")
	}
	output := filepath.Join(t.TempDir(), "project")
	result, err := runCircuitCreateCLI(t, []string{
		"--kicad-cli", cliPath, "--symbols-root", symbolsRoot, "--footprints-root", footprintsRoot,
		"--catalog-dir", filepath.Join("..", "..", "data", "components"),
		"--require-kicad-roundtrip", "--strict-diffs",
		"circuit", "create", "--request", filepath.Join("..", "..", "examples", "circuit-graph", "usb_c_bmp280_breakout.json"), "--output", output, "--overwrite",
	})
	if err != nil || !result.OK {
		t.Fatalf("direct BMP280 KiCad promotion err=%v result=%#v", err, result)
	}
	if !circuitCreateResultData(t, result).Preflight.ReadyForWrite {
		t.Fatal("direct BMP280 create did not preserve preflight readiness")
	}
}

func TestCircuitPatchRepairsThenPreflights(t *testing.T) {
	base, err := os.ReadFile(filepath.Join("..", "..", "examples", "circuit-graph", "rc_filter.json"))
	if err != nil {
		t.Fatal(err)
	}
	broken := filepath.Join(t.TempDir(), "broken.json")
	if err := os.WriteFile(broken, []byte(strings.Replace(string(base), `"selector": "1"`, `"selector": "999"`, 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	patch := filepath.Join(t.TempDir(), "patch.json")
	if err := os.WriteFile(patch, []byte(`{"schema":"kicadai.circuit-patch.v1","version":1,"operations":[{"op":"replace_endpoint","net":"FILTER_IN","endpoint":{"component":"input","selector_kind":"symbol_pin","selector":"999"},"replacement":{"component":"input","selector_kind":"symbol_pin","selector":"1"}}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	corrected := filepath.Join(t.TempDir(), "corrected.json")
	result, err := runCircuitPatchCLI(t, []string{"circuit", "patch", "--request", broken, "--patch", patch, "--output", corrected})
	if err != nil || !result.OK {
		t.Fatalf("patch err=%v result=%#v", err, result)
	}
	if _, err := os.Stat(corrected); err != nil {
		t.Fatal(err)
	}
	patchData := circuitPatchResultData(t, result)
	if len(patchData.NormalizedPatchOperations) != 1 || patchData.CriticalDesignProjection == nil || patchData.CriticalDesignProjection.Before.Nets[0].Endpoints[0].Selector != "999" || patchData.CriticalDesignProjection.After.Nets[0].Endpoints[0].Selector != "1" {
		t.Fatalf("patch result omitted deterministic corrected projection: %#v", patchData)
	}
	preflight := runCircuitPreflightCLI(t, []string{"circuit", "preflight", "--request", corrected})
	if !preflight.OK || !preflightResultData(t, preflight).ReadyForWrite {
		t.Fatalf("corrected preflight=%#v", preflight)
	}
	symbolsRoot, footprintsRoot := writeCircuitCreateLibraryFixture(t)
	cli := fakeWorkflowKiCadCLI(t, 0, passingWorkflowKiCadReport)
	project := filepath.Join(t.TempDir(), "project")
	created, createErr := runCircuitCreateCLI(t, []string{
		"--kicad-cli", cli, "--symbols-root", symbolsRoot, "--footprints-root", footprintsRoot,
		"circuit", "create", "--request", corrected, "--output", project, "--overwrite",
	})
	if createErr != nil || !created.OK {
		t.Fatalf("corrected circuit create err=%v result=%#v", createErr, created)
	}
}

func TestCircuitPatchRepairsCatalogSelectorAndPlacement(t *testing.T) {
	t.Run("catalog_selector", func(t *testing.T) {
		graph := filepath.Join("..", "..", "examples", "circuit-graph", "unsupported_unknown_component.json")
		patch := filepath.Join(t.TempDir(), "catalog-selector.json")
		if err := os.WriteFile(patch, []byte(`{"schema":"kicadai.circuit-patch.v1","version":1,"operations":[{"op":"replace_component","component":"r1","component_patch":{"component_id":"resistor.generic.0805","variant_id":"0805"}}]}`), 0o600); err != nil {
			t.Fatal(err)
		}
		corrected := filepath.Join(t.TempDir(), "corrected.json")
		result, err := runCircuitPatchCLI(t, []string{"circuit", "patch", "--request", graph, "--patch", patch, "--output", corrected})
		if err != nil || !result.OK || !circuitPatchResultData(t, result).ReadyForWrite {
			t.Fatalf("catalog selector patch err=%v result=%#v", err, result)
		}
	})
	t.Run("placement_region", func(t *testing.T) {
		base, err := os.ReadFile(filepath.Join("..", "..", "examples", "circuit-graph", "rc_filter.json"))
		if err != nil {
			t.Fatal(err)
		}
		broken := filepath.Join(t.TempDir(), "invalid-placement.json")
		if err := os.WriteFile(broken, []byte(strings.Replace(string(base), `"width_mm": 12,`, `"width_mm": 120,`, 1)), 0o600); err != nil {
			t.Fatal(err)
		}
		patch := filepath.Join(t.TempDir(), "placement.json")
		if err := os.WriteFile(patch, []byte(`{"schema":"kicadai.circuit-patch.v1","version":1,"operations":[{"op":"replace_pcb_region","region":"filter","bounds":{"x_mm":8,"y_mm":0,"width_mm":24,"height_mm":25}}]}`), 0o600); err != nil {
			t.Fatal(err)
		}
		corrected := filepath.Join(t.TempDir(), "corrected.json")
		result, err := runCircuitPatchCLI(t, []string{"circuit", "patch", "--request", broken, "--patch", patch, "--output", corrected})
		if err != nil || !result.OK || !circuitPatchResultData(t, result).ReadyForWrite {
			t.Fatalf("placement patch err=%v result=%#v", err, result)
		}
	})
}

func TestCircuitPatchRepairsInvalidMultiUnitAssignment(t *testing.T) {
	recorded, err := os.ReadFile(filepath.Join("..", "..", "examples", "ai", "generic_lm358_buffered_signal_conditioner", "recorded-response.json"))
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		Intent json.RawMessage `json:"intent"`
	}
	if err := json.Unmarshal(recorded, &envelope); err != nil {
		t.Fatal(err)
	}
	brokenContents := strings.Replace(string(envelope.Intent), `"unit":"B"`, `"unit":"Z"`, 1)
	broken := filepath.Join(t.TempDir(), "invalid-multi-unit.json")
	if err := os.WriteFile(broken, []byte(brokenContents), 0o600); err != nil {
		t.Fatal(err)
	}
	patch := filepath.Join(t.TempDir(), "multi-unit.json")
	if err := os.WriteFile(patch, []byte(`{"schema":"kicadai.circuit-patch.v1","version":1,"operations":[{"op":"replace_endpoint","net":"GAIN_INPUT","endpoint":{"component":"amplifier","unit":"Z","selector_kind":"function","selector":"IN_PLUS"},"replacement":{"component":"amplifier","unit":"B","selector_kind":"function","selector":"IN_PLUS"}}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	corrected := filepath.Join(t.TempDir(), "corrected.json")
	result, err := runCircuitPatchCLI(t, []string{"circuit", "patch", "--request", broken, "--patch", patch, "--output", corrected})
	if err != nil || !result.OK || !circuitPatchResultData(t, result).ReadyForWrite {
		t.Fatalf("multi-unit patch err=%v result=%#v", err, result)
	}
}

func TestCircuitPatchFailsClosedWithoutOutput(t *testing.T) {
	graph := filepath.Join("..", "..", "examples", "circuit-graph", "rc_filter.json")
	patch := filepath.Join(t.TempDir(), "unsafe.json")
	if err := os.WriteFile(patch, []byte(`{"schema":"kicadai.circuit-patch.v1","version":1,"operations":[{"op":"replace_project"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "must-not-exist.json")
	result, err := runCircuitPatchCLI(t, []string{"circuit", "patch", "--request", graph, "--patch", patch, "--output", output})
	if err == nil || result.OK {
		t.Fatalf("unsafe patch err=%v result=%#v", err, result)
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("patch wrote output: %v", err)
	}
}

func TestCircuitRepairPlanReadyAndRepeatedHash(t *testing.T) {
	graph := filepath.Join("..", "..", "examples", "circuit-graph", "rc_filter.json")
	ready := runCircuitPreflightCLI(t, []string{"circuit", "repair-plan", "--request", graph})
	data := circuitRepairPlanResultData(t, ready)
	if !ready.OK || data.Plan.State != circuitgraph.RepairPlanReady || data.Plan.Patch != nil || data.Plan.InputHash == "" {
		t.Fatalf("ready repair plan = %#v", ready)
	}
	repeated := runCircuitPreflightCLI(t, []string{"circuit", "repair-plan", "--request", graph, "--previous-hash", data.Plan.InputHash})
	if got := circuitRepairPlanResultData(t, repeated).Plan; got.State != circuitgraph.RepairPlanBlocked || got.StopReason != "repeated_input_hash" || got.Patch != nil {
		t.Fatalf("repeated repair plan = %#v", got)
	}
}

func TestCircuitRepairPlanRecordedGenericCorpusAndReviewStop(t *testing.T) {
	for _, name := range []string{
		"generic_usb_c_bmp280_breakout",
		"generic_lmv321_ac_gain_stage",
		"generic_dual_lmv321_signal_conditioner",
		"generic_lm358_buffered_signal_conditioner",
	} {
		t.Run(name, func(t *testing.T) {
			graph := recordedGenericCircuitGraph(t, name)
			result := runCircuitPreflightCLI(t, []string{"circuit", "repair-plan", "--request", graph})
			plan := circuitRepairPlanResultData(t, result).Plan
			if !result.OK || plan.State != circuitgraph.RepairPlanReady || plan.Patch != nil {
				t.Fatalf("ready recorded graph result=%#v plan=%#v", result, plan)
			}
		})
	}
	t.Run("protected_led_routing_ready", func(t *testing.T) {
		graph := recordedGenericCircuitGraph(t, "generic_usb_c_led_indicator_protected")
		result := runCircuitPreflightCLI(t, []string{"circuit", "repair-plan", "--request", graph})
		plan := circuitRepairPlanResultData(t, result).Plan
		if !result.OK || plan.State != circuitgraph.RepairPlanReady || plan.StopReason != "preflight_ready" || plan.Patch != nil {
			t.Fatalf("ready protected LED plan result=%#v plan=%#v", result, plan)
		}
	})
}

func TestCircuitRepairPlanDoesNotWriteFiles(t *testing.T) {
	tmpDir := t.TempDir()
	graph := filepath.Join(tmpDir, "graph.json")
	contents, err := os.ReadFile(filepath.Join("..", "..", "examples", "circuit-graph", "rc_filter.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(graph, contents, 0o600); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	result := runCircuitPreflightCLI(t, []string{"circuit", "repair-plan", "--request", graph})
	if !result.OK || circuitRepairPlanResultData(t, result).Plan.State != circuitgraph.RepairPlanReady {
		t.Fatalf("read-only repair plan=%#v", result)
	}
	after, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != 1 || len(after) != 1 || before[0].Name() != "graph.json" || after[0].Name() != "graph.json" {
		t.Fatalf("repair-plan wrote files: before=%#v after=%#v", before, after)
	}
}

func TestCircuitRepairPlanDerivesUniqueSelectorPatch(t *testing.T) {
	base, err := os.ReadFile(filepath.Join("..", "..", "examples", "circuit-graph", "rc_filter.json"))
	if err != nil {
		t.Fatal(err)
	}
	broken := filepath.Join(t.TempDir(), "broken.json")
	if err := os.WriteFile(broken, []byte(strings.Replace(string(base), `"selector": "1"`, `"selector": "999"`, 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	result := runCircuitPreflightCLI(t, []string{"circuit", "repair-plan", "--request", broken})
	plan := circuitRepairPlanResultData(t, result).Plan
	if plan.State != circuitgraph.RepairPlanAvailable || plan.Patch == nil || len(plan.Patch.Operations) != 1 || plan.Patch.Operations[0].Replacement == nil || plan.Patch.Operations[0].Replacement.Selector != "1" {
		t.Fatalf("selector repair plan = %#v", plan)
	}
}

func TestCircuitRepairPlanConvergesRCGraphToDirectCreate(t *testing.T) {
	base, err := os.ReadFile(filepath.Join("..", "..", "examples", "circuit-graph", "rc_filter.json"))
	if err != nil {
		t.Fatal(err)
	}
	graph, issues := circuitgraph.DecodeStrict(bytes.NewReader(base))
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("decode base graph issues=%#v", issues)
	}
	modified := false
	for netIndex := range graph.Nets {
		if graph.Nets[netIndex].Name != "FILTER_IN" {
			continue
		}
		for endpointIndex := range graph.Nets[netIndex].Endpoints {
			if graph.Nets[netIndex].Endpoints[endpointIndex].Component == "input" {
				graph.Nets[netIndex].Endpoints[endpointIndex].Selector = "999"
				modified = true
			}
		}
	}
	if !modified {
		t.Fatal("RC fixture no longer exposes FILTER_IN input endpoint")
	}
	brokenContents, err := json.Marshal(graph)
	if err != nil {
		t.Fatal(err)
	}
	tmpDir := t.TempDir()
	broken := filepath.Join(tmpDir, "broken.json")
	if err := os.WriteFile(broken, brokenContents, 0o600); err != nil {
		t.Fatal(err)
	}
	planResult := runCircuitPreflightCLI(t, []string{"circuit", "repair-plan", "--request", broken})
	plan := circuitRepairPlanResultData(t, planResult).Plan
	if plan.State != circuitgraph.RepairPlanAvailable || plan.Patch == nil {
		t.Fatalf("repair plan = %#v", plan)
	}
	patchContents, err := json.Marshal(plan.Patch)
	if err != nil {
		t.Fatal(err)
	}
	patchPath := filepath.Join(tmpDir, "repair.json")
	if err := os.WriteFile(patchPath, patchContents, 0o600); err != nil {
		t.Fatal(err)
	}
	corrected := filepath.Join(tmpDir, "corrected.json")
	patchResult, patchErr := runCircuitPatchCLI(t, []string{"circuit", "patch", "--request", broken, "--patch", patchPath, "--output", corrected})
	if patchErr != nil || !patchResult.OK || !circuitPatchResultData(t, patchResult).ReadyForWrite {
		t.Fatalf("apply plan err=%v result=%#v", patchErr, patchResult)
	}
	symbolsRoot, footprintsRoot := writeCircuitCreateLibraryFixture(t)
	cli := fakeWorkflowKiCadCLI(t, 0, passingWorkflowKiCadReport)
	project := filepath.Join(tmpDir, "project")
	created, createErr := runCircuitCreateCLI(t, []string{"--kicad-cli", cli, "--symbols-root", symbolsRoot, "--footprints-root", footprintsRoot, "circuit", "create", "--request", corrected, "--output", project, "--overwrite"})
	if createErr != nil || !created.OK {
		t.Fatalf("create corrected graph err=%v result=%#v", createErr, created)
	}
}

func TestCircuitRepairPlanDerivesLM358UnitPatch(t *testing.T) {
	recorded, err := os.ReadFile(filepath.Join("..", "..", "examples", "ai", "generic_lm358_buffered_signal_conditioner", "recorded-response.json"))
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		Intent json.RawMessage `json:"intent"`
	}
	if err := json.Unmarshal(recorded, &envelope); err != nil {
		t.Fatal(err)
	}
	brokenContents := strings.Replace(string(envelope.Intent), `"unit":"B"`, `"unit":"Z"`, 1)
	if brokenContents == string(envelope.Intent) {
		t.Fatal("LM358 fixture no longer exposes a unit B endpoint")
	}
	tmpDir := t.TempDir()
	broken := filepath.Join(tmpDir, "invalid-multi-unit.json")
	if err := os.WriteFile(broken, []byte(brokenContents), 0o600); err != nil {
		t.Fatal(err)
	}
	planned := runCircuitPreflightCLI(t, []string{"circuit", "repair-plan", "--request", broken})
	plan := circuitRepairPlanResultData(t, planned).Plan
	if plan.State != circuitgraph.RepairPlanAvailable || plan.Patch == nil || len(plan.Patch.Operations) != 1 || plan.Patch.Operations[0].Replacement == nil || plan.Patch.Operations[0].Replacement.Unit != "B" {
		t.Fatalf("LM358 repair plan = %#v", plan)
	}
	patch, err := json.Marshal(plan.Patch)
	if err != nil {
		t.Fatal(err)
	}
	patchPath := filepath.Join(tmpDir, "repair.json")
	if err := os.WriteFile(patchPath, patch, 0o600); err != nil {
		t.Fatal(err)
	}
	corrected := filepath.Join(tmpDir, "corrected.json")
	patched, patchErr := runCircuitPatchCLI(t, []string{"circuit", "patch", "--request", broken, "--patch", patchPath, "--output", corrected})
	if patchErr != nil || !patched.OK || !circuitPatchResultData(t, patched).ReadyForWrite {
		t.Fatalf("apply LM358 plan err=%v result=%#v", patchErr, patched)
	}
}

func TestCircuitRepairPlanDerivesBoundedRegionPatch(t *testing.T) {
	base, err := os.ReadFile(filepath.Join("..", "..", "examples", "circuit-graph", "rc_filter.json"))
	if err != nil {
		t.Fatal(err)
	}
	graph, issues := circuitgraph.DecodeStrict(bytes.NewReader(base))
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("decode RC graph issues=%#v", issues)
	}
	modified := false
	for index := range graph.PCB.Regions {
		if graph.PCB.Regions[index].ID == "filter" {
			graph.PCB.Regions[index].Bounds.WidthMM = 120
			modified = true
		}
	}
	if !modified {
		t.Fatal("RC fixture no longer exposes the bounded filter region")
	}
	brokenContents, err := json.Marshal(graph)
	if err != nil {
		t.Fatal(err)
	}
	tmpDir := t.TempDir()
	broken := filepath.Join(tmpDir, "invalid-region.json")
	if err := os.WriteFile(broken, brokenContents, 0o600); err != nil {
		t.Fatal(err)
	}
	planned := runCircuitPreflightCLI(t, []string{"circuit", "repair-plan", "--request", broken})
	plan := circuitRepairPlanResultData(t, planned).Plan
	if plan.State != circuitgraph.RepairPlanAvailable || plan.Patch == nil || len(plan.Patch.Operations) != 1 || plan.Patch.Operations[0].Bounds == nil {
		t.Fatalf("region repair plan = %#v", plan)
	}
	patch, err := json.Marshal(plan.Patch)
	if err != nil {
		t.Fatal(err)
	}
	patchPath := filepath.Join(tmpDir, "repair.json")
	if err := os.WriteFile(patchPath, patch, 0o600); err != nil {
		t.Fatal(err)
	}
	corrected := filepath.Join(tmpDir, "corrected.json")
	patched, patchErr := runCircuitPatchCLI(t, []string{"circuit", "patch", "--request", broken, "--patch", patchPath, "--output", corrected})
	if patchErr != nil || !patched.OK || !circuitPatchResultData(t, patched).ReadyForWrite {
		t.Fatalf("apply region plan err=%v result=%#v", patchErr, patched)
	}
}

func TestCircuitPatchRejectsUnsupportedSubstitutionWithoutOutput(t *testing.T) {
	graph := filepath.Join("..", "..", "examples", "circuit-graph", "rc_filter.json")
	patch := filepath.Join(t.TempDir(), "unsupported-substitution.json")
	if err := os.WriteFile(patch, []byte(`{"schema":"kicadai.circuit-patch.v1","version":1,"operations":[{"op":"replace_component","component":"r1","component_patch":{"component_id":"unsupported.component"}}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "must-not-exist.json")
	result, err := runCircuitPatchCLI(t, []string{"circuit", "patch", "--request", graph, "--patch", patch, "--output", output})
	if err == nil || result.OK || circuitPatchResultData(t, result).ReadyForWrite || len(result.Issues) == 0 {
		t.Fatalf("unsupported substitution err=%v result=%#v", err, result)
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("patch wrote unsupported substitution: %v", err)
	}
}

func runCircuitPreflightCLI(t *testing.T, args []string) reports.Result {
	t.Helper()
	var stdout, stderr bytes.Buffer
	_ = run(args, &stdout, &stderr)
	var result reports.Result
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode preflight: %v\n%s", err, stdout.String())
	}
	return result
}

func resultHasIssueMessage(result reports.Result, fragment string) bool {
	for _, issue := range result.Issues {
		if strings.Contains(issue.Message, fragment) {
			return true
		}
	}
	if result.Diagnostics != nil {
		for _, group := range result.Diagnostics.Groups {
			for _, issue := range group.Samples {
				if strings.Contains(issue.Message, fragment) {
					return true
				}
			}
		}
	}
	return false
}

func preflightResultData(t *testing.T, result reports.Result) circuitPreflightData {
	t.Helper()
	data, err := json.Marshal(result.Data)
	if err != nil {
		t.Fatal(err)
	}
	var preflight circuitPreflightData
	if err := json.Unmarshal(data, &preflight); err != nil {
		t.Fatal(err)
	}
	return preflight
}

func circuitPatchResultData(t *testing.T, result reports.Result) circuitPatchData {
	t.Helper()
	data, err := json.Marshal(result.Data)
	if err != nil {
		t.Fatal(err)
	}
	var patch circuitPatchData
	if err := json.Unmarshal(data, &patch); err != nil {
		t.Fatal(err)
	}
	return patch
}

func circuitRepairPlanResultData(t *testing.T, result reports.Result) circuitRepairPlanData {
	t.Helper()
	data, err := json.Marshal(result.Data)
	if err != nil {
		t.Fatal(err)
	}
	var plan circuitRepairPlanData
	if err := json.Unmarshal(data, &plan); err != nil {
		t.Fatal(err)
	}
	return plan
}

func recordedGenericCircuitGraph(t *testing.T, name string) string {
	t.Helper()
	recorded, err := os.ReadFile(filepath.Join("..", "..", "examples", "ai", name, "recorded-response.json"))
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		Intent json.RawMessage `json:"intent"`
	}
	if err := json.Unmarshal(recorded, &envelope); err != nil {
		t.Fatal(err)
	}
	if len(envelope.Intent) == 0 {
		t.Fatal("recorded response omitted generic circuit intent")
	}
	path := filepath.Join(t.TempDir(), "graph.json")
	if err := os.WriteFile(path, envelope.Intent, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func runCircuitCreateCLI(t *testing.T, args []string) (reports.Result, error) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	err := run(args, &stdout, &stderr)
	var result reports.Result
	if decodeErr := json.Unmarshal(stdout.Bytes(), &result); decodeErr != nil {
		t.Fatalf("decode circuit create: %v\n%s", decodeErr, stdout.String())
	}
	return result, err
}

func runCircuitPatchCLI(t *testing.T, args []string) (reports.Result, error) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	err := run(args, &stdout, &stderr)
	var result reports.Result
	if decodeErr := json.Unmarshal(stdout.Bytes(), &result); decodeErr != nil {
		t.Fatalf("decode circuit patch: %v\n%s", decodeErr, stdout.String())
	}
	return result, err
}

func circuitCreateResultData(t *testing.T, result reports.Result) circuitCreateData {
	t.Helper()
	data, err := json.Marshal(result.Data)
	if err != nil {
		t.Fatal(err)
	}
	var created circuitCreateData
	if err := json.Unmarshal(data, &created); err != nil {
		t.Fatal(err)
	}
	return created
}

func writeCircuitCreateRCGraph(t *testing.T) string {
	t.Helper()
	contents, err := os.ReadFile(filepath.Join("..", "..", "examples", "circuit-graph", "rc_filter.json"))
	if err != nil {
		t.Fatal(err)
	}
	contents = []byte(strings.Replace(string(contents), `"acceptance": "erc-drc"`, `"acceptance": "structural"`, 1))
	path := filepath.Join(t.TempDir(), "rc_filter.json")
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeInvalidCircuitCreateGraph(t *testing.T) string {
	t.Helper()
	contents, err := os.ReadFile(filepath.Join("..", "..", "examples", "circuit-graph", "rc_filter.json"))
	if err != nil {
		t.Fatal(err)
	}
	contents = []byte(strings.Replace(string(contents), `"selector": "1"`, `"selector": "999"`, 1))
	path := filepath.Join(t.TempDir(), "invalid_pin.json")
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeCircuitCreateLibraryFixture(t *testing.T) (string, string) {
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
    (symbol "R_1_1"
      (pin passive line (at -5.08 0 0) (length 2.54) (name "~") (number "1"))
      (pin passive line (at 5.08 0 180) (length 2.54) (name "~") (number "2"))
    )
  )
  (symbol "C"
    (property "Reference" "C" (at 0 0 0))
    (property "Value" "C" (at 0 -2.54 0))
    (symbol "C_1_1"
      (pin passive line (at -5.08 0 0) (length 2.54) (name "~") (number "1"))
      (pin passive line (at 5.08 0 180) (length 2.54) (name "~") (number "2"))
    )
  )
)`)
	writeTestFile(t, filepath.Join(symbols, "Connector_Generic.kicad_sym"), `
(kicad_symbol_lib
  (version 20220914)
  (generator "kicadai-test")
  (symbol "Conn_01x02"
    (property "Reference" "J" (at 0 0 0))
    (property "Value" "Conn_01x02" (at 0 -5.08 0))
    (symbol "Conn_01x02_1_1"
      (pin passive line (at -5.08 2.54 0) (length 2.54) (name "Pin_1") (number "1"))
      (pin passive line (at -5.08 -2.54 0) (length 2.54) (name "Pin_2") (number "2"))
    )
  )
)`)
	for _, fixture := range []struct{ path, body string }{
		{"Resistor_SMD.pretty/R_0805_2012Metric.kicad_mod", `(footprint "R_0805_2012Metric" (version 20240108) (generator "kicadai-test") (attr smd) (fp_rect (start -1.2 -0.8) (end 1.2 0.8) (layer "F.CrtYd")) (pad "1" smd rect (at -0.95 0) (size 1 1.2) (layers "F.Cu" "F.Paste" "F.Mask")) (pad "2" smd rect (at 0.95 0) (size 1 1.2) (layers "F.Cu" "F.Paste" "F.Mask")))`},
		{"Capacitor_SMD.pretty/C_0805_2012Metric.kicad_mod", `(footprint "C_0805_2012Metric" (version 20240108) (generator "kicadai-test") (attr smd) (fp_rect (start -1.2 -0.8) (end 1.2 0.8) (layer "F.CrtYd")) (pad "1" smd rect (at -0.95 0) (size 1 1.2) (layers "F.Cu" "F.Paste" "F.Mask")) (pad "2" smd rect (at 0.95 0) (size 1 1.2) (layers "F.Cu" "F.Paste" "F.Mask")))`},
		{"Connector_PinHeader_2.54mm.pretty/PinHeader_1x02_P2.54mm_Vertical.kicad_mod", `(footprint "PinHeader_1x02_P2.54mm_Vertical" (version 20240108) (generator "kicadai-test") (fp_rect (start -1.5 -3) (end 1.5 3) (layer "F.CrtYd")) (pad "1" thru_hole rect (at 0 1.27) (size 1.8 1.8) (drill 1) (layers "*.Cu" "*.Mask")) (pad "2" thru_hole circle (at 0 -1.27) (size 1.8 1.8) (drill 1) (layers "*.Cu" "*.Mask")))`},
	} {
		writeTestFile(t, filepath.Join(footprints, fixture.path), fixture.body)
	}
	return symbols, footprints
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}
