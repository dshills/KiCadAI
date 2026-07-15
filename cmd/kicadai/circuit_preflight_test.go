package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

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

func TestCircuitPreflightAcceptsDocumentedArgumentOrder(t *testing.T) {
	graph := filepath.Join("..", "..", "examples", "circuit-graph", "rc_filter.json")
	result := runCircuitPreflightCLI(t, []string{"circuit", "preflight", "--request", graph, "--json"})
	if !result.OK || !preflightResultData(t, result).ReadyForWrite {
		t.Fatalf("documented argument order result = %#v", result)
	}
}

func TestCircuitPreflightFailsClosedBeforeWrite(t *testing.T) {
	graph := filepath.Join("..", "..", "examples", "circuit-graph", "unsupported_unknown_component.json")
	output := filepath.Join(t.TempDir(), "must-not-be-written")
	result := runCircuitPreflightCLI(t, []string{"--request", graph, "--output", output, "circuit", "preflight"})
	data := preflightResultData(t, result)
	if result.OK || data.ReadyForWrite || len(result.Issues) == 0 || result.Issues[0].RetryScope == "" {
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
		{name: "invalid_region", edit: func(input string) string { return strings.Replace(input, `"width_mm": 12,`, `"width_mm": 120,`, 1) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			graph := filepath.Join(t.TempDir(), test.name+".json")
			if err := os.WriteFile(graph, []byte(test.edit(string(base))), 0o600); err != nil {
				t.Fatal(err)
			}
			output := filepath.Join(t.TempDir(), "must-not-be-written")
			result := runCircuitPreflightCLI(t, []string{"circuit", "preflight", "--request", graph, "--output", output})
			if result.OK || preflightResultData(t, result).ReadyForWrite || len(result.Issues) == 0 {
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
	result := runCircuitPreflightCLI(t, []string{"circuit", "preflight", "--request", graph, "--output", output})
	if result.OK || preflightResultData(t, result).ReadyForWrite || len(result.Issues) == 0 {
		t.Fatalf("invalid multi-unit preflight = %#v", result)
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("preflight wrote output: %v", err)
	}
}

func TestCircuitCreateWritesPreflightReadyRCGraph(t *testing.T) {
	graph := writeCircuitCreateRCGraph(t)
	symbolsRoot, footprintsRoot := writeCircuitCreateLibraryFixture(t)
	output := filepath.Join(t.TempDir(), "project")
	result, err := runCircuitCreateCLI(t, []string{
		"--symbols-root", symbolsRoot, "--footprints-root", footprintsRoot,
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
	create := func(name string) (circuitCreateData, string) {
		t.Helper()
		output := filepath.Join(t.TempDir(), name)
		result, err := runCircuitCreateCLI(t, []string{
			"--symbols-root", symbolsRoot, "--footprints-root", footprintsRoot,
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
	preflight := runCircuitPreflightCLI(t, []string{"circuit", "preflight", "--request", corrected})
	if !preflight.OK || !preflightResultData(t, preflight).ReadyForWrite {
		t.Fatalf("corrected preflight=%#v", preflight)
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
