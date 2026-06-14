package transactions

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kicadai/internal/reports"
)

func TestOperationPreservesRawJSON(t *testing.T) {
	input := []byte(`{"operations":[{"op":"add_symbol","ref":"R1","library_id":"Device:R","extra":{"keep":true}}]}`)
	tx, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if len(tx.Operations) != 1 || !strings.Contains(string(tx.Operations[0].Raw), `"extra"`) {
		t.Fatalf("raw operation not preserved: %#v", tx.Operations)
	}
	encoded, err := json.Marshal(tx.Operations[0])
	if err != nil {
		t.Fatalf("Marshal operation: %v", err)
	}
	if !strings.Contains(string(encoded), `"extra"`) {
		t.Fatalf("marshal did not preserve raw payload: %s", encoded)
	}
}

func TestValidateValidTransaction(t *testing.T) {
	tx := mustParse(t, `{"operations":[
	  {"op":"create_project","name":"demo"},
	  {"op":"add_symbol","ref":"R1","library_id":"Device:R","at":{"x_mm":10,"y_mm":20},"pins":[{"number":"1"}]},
	  {"op":"connect","from":{"ref":"R1","pin":"1"},"to":{"ref":"J1","pin":"1"},"net_name":"SIG"},
	  {"op":"assign_footprint","ref":"R1","footprint_id":"Resistor_SMD:R_0805_2012Metric"},
	  {"op":"place_footprint","ref":"R1","at":{"x_mm":5,"y_mm":5},"pads":[{"name":"1"}]},
	  {"op":"route","net_name":"SIG","points":[{"x_mm":0,"y_mm":0},{"x_mm":1,"y_mm":1}]},
	  {"op":"add_zone","net_name":null,"polygon":[{"x_mm":0,"y_mm":0},{"x_mm":1,"y_mm":0},{"x_mm":1,"y_mm":1}]},
	  {"op":"write_project"}
	]}`)
	result := Validate(tx)
	if len(result.Issues) != 0 || result.OperationCount != 8 {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestValidateRejectsEmptyOperations(t *testing.T) {
	result := Validate(mustParse(t, `{"operations":[]}`))
	if len(result.Issues) != 1 || result.Issues[0].Path != "operations" {
		t.Fatalf("unexpected issues: %#v", result.Issues)
	}
}

func TestValidateRejectsUnknownOperation(t *testing.T) {
	result := Validate(mustParse(t, `{"operations":[{"op":"bogus"}]}`))
	if len(result.Issues) != 1 || result.Issues[0].Code != reports.CodeUnsupportedOperation || result.Issues[0].Path != "operations[0].op" {
		t.Fatalf("unexpected issues: %#v", result.Issues)
	}
}

func TestValidateSetBoardOutlineRequiresBoardOrPoints(t *testing.T) {
	result := Validate(mustParse(t, `{"operations":[{"op":"set_board_outline"}]}`))
	if len(result.Issues) != 1 || result.Issues[0].Path != "operations[0].board" {
		t.Fatalf("unexpected issues: %#v", result.Issues)
	}
}

func TestValidateSetBoardOutlineRejectsBoardAndPoints(t *testing.T) {
	result := Validate(mustParse(t, `{"operations":[{"op":"set_board_outline","board":{"width_mm":10,"height_mm":10},"points":[{"x_mm":0,"y_mm":0},{"x_mm":1,"y_mm":0},{"x_mm":1,"y_mm":1}]}]}`))
	if len(result.Issues) != 1 || result.Issues[0].Path != "operations[0]" {
		t.Fatalf("unexpected issues: %#v", result.Issues)
	}
}

func TestValidateReportsOperationIndex(t *testing.T) {
	result := Validate(mustParse(t, `{"operations":[{"op":"write_project"},{"op":"route","net_name":"SIG","points":[{"x_mm":0,"y_mm":0},{"x_mm":0,"y_mm":0}]}]}`))
	found := false
	for _, issue := range result.Issues {
		if issue.Path == "operations[1].points[1]" {
			found = true
		}
	}
	if !found {
		t.Fatalf("operation index issue missing: %#v", result.Issues)
	}
}

func TestValidateRejectsInvalidZone(t *testing.T) {
	result := Validate(mustParse(t, `{"operations":[{"op":"add_zone","net_name":"","polygon":[{"x_mm":0,"y_mm":0}]}]}`))
	if len(result.Issues) < 2 {
		t.Fatalf("expected net and polygon issues: %#v", result.Issues)
	}
}

func TestValidateRejectsZeroLengthPolygonSegment(t *testing.T) {
	result := Validate(mustParse(t, `{"operations":[{"op":"add_zone","net_name":null,"polygon":[{"x_mm":0,"y_mm":0},{"x_mm":0,"y_mm":0},{"x_mm":1,"y_mm":1}]}]}`))
	found := false
	for _, issue := range result.Issues {
		if issue.Path == "operations[0].polygon[1]" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected zero-length polygon issue: %#v", result.Issues)
	}
}

func TestValidateAcceptsViaOnlyRoute(t *testing.T) {
	result := Validate(mustParse(t, `{"operations":[{"op":"route","net_name":"SIG","vias":[{"at":{"x_mm":1,"y_mm":2},"diameter_mm":0.6,"drill_mm":0.3,"layers":["F.Cu","B.Cu"]}]}]}`))
	if len(result.Issues) != 0 {
		t.Fatalf("issues = %#v", result.Issues)
	}
}

func TestValidateRejectsMalformedWriteProject(t *testing.T) {
	_, err := Parse([]byte(`{"operations":[{"op":"write_project","output_dir":42}]}`))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	result := Validate(mustParse(t, `{"operations":[{"op":"write_project","output_dir":42}]}`))
	if len(result.Issues) != 1 || result.Issues[0].Path != "operations[0]" {
		t.Fatalf("unexpected issues: %#v", result.Issues)
	}
}

func TestValidateRejectsInvalidLibraryID(t *testing.T) {
	result := Validate(mustParse(t, `{"operations":[{"op":"add_symbol","ref":"R1","library_id":"Device:","at":{"x_mm":0,"y_mm":0}}]}`))
	if len(result.Issues) != 1 || result.Issues[0].Path != "operations[0].library_id" {
		t.Fatalf("unexpected issues: %#v", result.Issues)
	}
}

func TestValidateAllowsColonInLibraryItemName(t *testing.T) {
	result := Validate(mustParse(t, `{"operations":[{"op":"add_symbol","ref":"R1","library_id":"Device:R:Variant","at":{"x_mm":0,"y_mm":0}}]}`))
	if len(result.Issues) != 0 {
		t.Fatalf("unexpected issues: %#v", result.Issues)
	}
}

func TestValidateRejectsInvalidPinCoordinate(t *testing.T) {
	result := Validate(mustParse(t, `{"operations":[{"op":"add_symbol","ref":"R1","library_id":"Device:R","at":{"x_mm":0,"y_mm":0},"pins":[{"number":"1","x_mm":1e999}]}]}`))
	if len(result.Issues) != 1 || result.Issues[0].Path != "operations[0]" {
		t.Fatalf("unexpected issues: %#v", result.Issues)
	}
}

func TestLoadFileInvalidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tx.json")
	if err := os.WriteFile(path, []byte(`{`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadFile(path); err == nil {
		t.Fatal("expected invalid JSON error")
	}
}

func mustParse(t *testing.T, input string) Transaction {
	t.Helper()
	tx, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	return tx
}
