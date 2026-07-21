package designworkflow

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"slices"
	"testing"

	"kicadai/internal/closedloopsynthesis"
	"kicadai/internal/reports"
)

func TestAutonomousCorrectionDiagnosticCategories(t *testing.T) {
	tests := []struct {
		name      string
		issue     reports.Issue
		want      AutonomousCorrectionCategory
		supported bool
	}{
		{name: "overlap", issue: correctionIssue(reports.CodePlacementCollision, "placement.components[0]", []string{"U1", "R1"}, nil, "collision"), want: CorrectionComponentOverlap, supported: true},
		{name: "pad access", issue: correctionIssue(reports.CodeValidationFailed, "routing.nets[0]", []string{"U1"}, []string{"SIG"}, "pad access point is blocked"), want: CorrectionInaccessiblePad, supported: true},
		{name: "blocked escape", issue: correctionIssue(reports.CodePlacementOutsideBoard, "placement.components[0]", []string{"J1"}, nil, "outside board"), want: CorrectionBlockedEscapeDirection, supported: true},
		{name: "branch order", issue: correctionIssue(reports.CodeValidationFailed, `design.inter_block_route_groups["SIG"].branches[0]`, []string{"U1", "J1"}, []string{"SIG"}, "no legal route"), want: CorrectionRouteTreeBranchOrder, supported: false},
		{name: "layer transition", issue: correctionIssue(reports.CodeRouteContactLayerMismatch, "routing.contacts[0]", []string{"U1"}, []string{"SIG"}, "routing layer is not available"), want: CorrectionMissingLayerTransition, supported: false},
		{name: "same net merge", issue: correctionIssue(reports.CodeRouteGraphIncomplete, "design.inter_block_contact.nets[0]", []string{"U1"}, []string{"SIG"}, "separate same-net graph component"), want: CorrectionSameNetBranchMerge, supported: true},
		{name: "disconnected endpoint", issue: correctionIssue(reports.CodeDisconnectedPad, "routing.nets[0]", []string{"U1", "J1"}, []string{"SIG"}, "route does not connect all intended endpoints"), want: CorrectionRequiredNetDisconnectedEndpoint, supported: true},
		{name: "region exhaustion", issue: correctionIssue(reports.CodeValidationFailed, "routing.nets[0]", []string{"U1", "J1"}, []string{"SIG"}, "no legal route"), want: CorrectionRoutingRegionExhaustion, supported: true},
		{name: "unsupported", issue: correctionIssue(reports.CodeRouteContactUnsupported, "routing.contacts[0]", []string{"U1"}, []string{"SIG"}, "unsupported geometry"), want: CorrectionUnsupportedGeometry, supported: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			diagnostics := BuildAutonomousCorrectionDiagnostics(nil, []reports.Issue{test.issue})
			if len(diagnostics) != 1 || diagnostics[0].Category != test.want || diagnostics[0].AutomaticAction != test.supported {
				t.Fatalf("diagnostic = %#v, want category=%s supported=%t", diagnostics, test.want, test.supported)
			}
			if slices.Contains(diagnostics[0].Evidence, test.issue.Message) {
				t.Fatalf("raw issue message leaked into evidence: %#v", diagnostics[0].Evidence)
			}
		})
	}
}

func TestAutonomousCorrectionDiagnosticsAreDeterministicAndSanitized(t *testing.T) {
	left := correctionIssue(reports.CodeDisconnectedPad, filepath.Join(string(filepath.Separator), "private", "tmp", "routing.nets[1]"), []string{"U2", "U1", "U1"}, []string{"GND", "GND"}, "first external message")
	right := left
	right.Message = "different external message"
	right.Path = filepath.Join(string(filepath.Separator), "another", "workspace", "routing.nets[1]")
	first := BuildAutonomousCorrectionDiagnostics(nil, []reports.Issue{left})
	second := BuildAutonomousCorrectionDiagnostics(nil, []reports.Issue{right})
	if len(first) != 1 || len(second) != 1 {
		t.Fatalf("diagnostics: first=%#v second=%#v", first, second)
	}
	if first[0].Path != "routing.nets[1]" || second[0].Path != first[0].Path {
		t.Fatalf("normalized paths: first=%q second=%q", first[0].Path, second[0].Path)
	}
	firstKey := AutonomousCorrectionRetryKey(first, []string{"reduce_endpoint_distance"}, "invariant", "state")
	secondKey := AutonomousCorrectionRetryKey(second, []string{"reduce_endpoint_distance"}, "invariant", "state")
	if firstKey != secondKey {
		t.Fatalf("workspace/message changed retry key: %s != %s", firstKey, secondKey)
	}
}

func TestNormalizeAutonomousCorrectionPathIsHostIndependent(t *testing.T) {
	for input, want := range map[string]string{
		`/private/tmp/routing.nets[1]`:        "routing.nets[1]",
		`C:\work\project\routing.nets[1]`:     "routing.nets[1]",
		`\\server\share\routing.nets[1]`:      "routing.nets[1]",
		`design\route_groups["SIG"]\branches`: `design/route_groups["SIG"]/branches`,
	} {
		if got := normalizeAutonomousCorrectionPath(input); got != want {
			t.Fatalf("normalize %q = %q, want %q", input, got, want)
		}
	}
}

func TestAutonomousCorrectionRetryKeyChangesWithProtectedInputs(t *testing.T) {
	diagnostics := BuildAutonomousCorrectionDiagnostics(nil, []reports.Issue{
		correctionIssue(reports.CodePlacementCollision, "placement.components[0]", []string{"U1", "R1"}, []string{"SIG"}, "collision"),
	})
	base := AutonomousCorrectionRetryKey(diagnostics, []string{"adjust_relative_spacing"}, "invariant-a", "state-a")
	for name, key := range map[string]string{
		"action":    AutonomousCorrectionRetryKey(diagnostics, []string{"improve_endpoint_fanout"}, "invariant-a", "state-a"),
		"invariant": AutonomousCorrectionRetryKey(diagnostics, []string{"adjust_relative_spacing"}, "invariant-b", "state-a"),
		"state":     AutonomousCorrectionRetryKey(diagnostics, []string{"adjust_relative_spacing"}, "invariant-a", "state-b"),
	} {
		if key == base {
			t.Fatalf("%s did not change retry key", name)
		}
	}
}

func TestAutonomousCorrectionDiagnosticDedupeUsesFramedFields(t *testing.T) {
	first := correctionIssue(reports.CodePlacementCollision, "placement|components", []string{"A,B"}, nil, "collision")
	second := correctionIssue(reports.CodePlacementCollision, "placement|components", []string{"A", "B"}, nil, "collision")
	diagnostics := BuildAutonomousCorrectionDiagnostics([]reports.Issue{first, second}, nil)
	if len(diagnostics) != 2 {
		t.Fatalf("delimiter-bearing diagnostics collapsed: %#v", diagnostics)
	}
}

func TestAutonomousCorrectionInvariantFingerprint(t *testing.T) {
	request := correctionExplicitRequest()
	originalRequest, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	base, err := AutonomousCorrectionInvariantFingerprint(request)
	if err != nil {
		t.Fatal(err)
	}
	afterRequest, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(afterRequest, originalRequest) {
		t.Fatal("fingerprinting mutated caller request")
	}
	renamed := request
	renamed.Name = "another_project_name"
	renamed.RoutingRetry.MaxAttempts = 3
	renameHash, err := AutonomousCorrectionInvariantFingerprint(renamed)
	if err != nil {
		t.Fatal(err)
	}
	if renameHash != base {
		t.Fatalf("workspace-neutral fields changed invariant: %s != %s", renameHash, base)
	}
	changed := request
	changed.ExplicitCircuit = cloneExplicitCircuit(request.ExplicitCircuit)
	changed.ExplicitCircuit.Nets[0].WidthMM = 0.5
	changedHash, err := AutonomousCorrectionInvariantFingerprint(changed)
	if err != nil {
		t.Fatal(err)
	}
	if changedHash == base {
		t.Fatal("protected net width did not change invariant fingerprint")
	}
}

func TestAutonomousCorrectionInvariantFingerprintUsesCompactClosedLoopBinding(t *testing.T) {
	request := correctionExplicitRequest()
	request.ExplicitCircuit.ClosedLoop = &closedloopsynthesis.Report{
		Schema: "closed-loop-v1", PolicyVersion: "policy-v1",
		PolicyHash: "policy", RequirementHash: "requirement", RegistryHash: "registry",
		CatalogHash: "catalog", FormulaLibraryHash: "formula", ModelRegistryHash: "models",
		SelectedCircuitHash: "circuit", StopReason: closedloopsynthesis.StopPassed, Status: "pass",
		Diagnostics: []closedloopsynthesis.Diagnostic{{Path: "attempts[0]", Message: "large runtime transcript"}},
	}
	base, err := AutonomousCorrectionInvariantFingerprint(request)
	if err != nil {
		t.Fatal(err)
	}

	transcriptChanged := request
	transcriptChanged.ExplicitCircuit = cloneExplicitCircuit(request.ExplicitCircuit)
	transcriptChanged.ExplicitCircuit.ClosedLoop.Diagnostics[0].Message = "different runtime transcript"
	transcriptHash, err := AutonomousCorrectionInvariantFingerprint(transcriptChanged)
	if err != nil {
		t.Fatal(err)
	}
	if transcriptHash != base {
		t.Fatalf("runtime transcript changed design invariant: %s != %s", transcriptHash, base)
	}

	bindingChanged := request
	bindingChanged.ExplicitCircuit = cloneExplicitCircuit(request.ExplicitCircuit)
	bindingChanged.ExplicitCircuit.ClosedLoop.SelectedCircuitHash = "different-circuit"
	bindingHash, err := AutonomousCorrectionInvariantFingerprint(bindingChanged)
	if err != nil {
		t.Fatal(err)
	}
	if bindingHash == base {
		t.Fatal("selected closed-loop circuit binding did not change invariant fingerprint")
	}
}

func TestIsGenericAutonomousCorrectionRequest(t *testing.T) {
	request := correctionExplicitRequest()
	if !IsGenericAutonomousCorrectionRequest(request) {
		t.Fatal("explicit graph request was not eligible")
	}
	request.Intent.Category = "other"
	if IsGenericAutonomousCorrectionRequest(request) {
		t.Fatal("non-generic explicit request was eligible")
	}
}

func correctionIssue(code reports.Code, path string, refs, nets []string, message string) reports.Issue {
	return reports.Issue{Code: code, Severity: reports.SeverityBlocked, Path: path, Refs: refs, Nets: nets, Message: message}
}

func correctionExplicitRequest() Request {
	return Request{
		Version: RequestVersion,
		Name:    "generic_correction",
		Intent:  Intent{Summary: "generic correction test", Category: "explicit_circuit_graph"},
		Board:   BoardSpec{WidthMM: 40, HeightMM: 30, Layers: 2, EdgeClearanceMM: 0.5},
		Constraints: ConstraintSpec{
			RouteWidthMM: 0.25, ClearanceMM: 0.2, AllowBackLayer: true,
		},
		Validation: ValidationSpec{Acceptance: AcceptanceConnectivity, StrictUnrouted: true},
		ExplicitCircuit: &ExplicitCircuitSpec{
			ResolutionHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			CatalogID:      "catalog:test",
			CatalogHash:    "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
			Components: []ExplicitComponentSpec{
				{ID: "source", Reference: "J1", Role: "input", Value: "IN", FootprintID: "Connector:Test", Pads: []ExplicitPadSpec{{Name: "1", SymbolPin: "1", Net: "SIG"}}},
				{ID: "load", Reference: "R1", Role: "load", Value: "1k", FootprintID: "Resistor:Test", Pads: []ExplicitPadSpec{{Name: "1", SymbolPin: "1", Net: "SIG"}}},
			},
			Nets: []ExplicitNetSpec{{
				Name: "SIG", Role: "signal", NetClass: "signal", Required: true, WidthMM: 0.25, ClearanceMM: 0.2,
				Endpoints: []ExplicitNetEndpoint{{Component: "source", Pad: "1"}, {Component: "load", Pad: "1"}},
			}},
		},
	}
}
