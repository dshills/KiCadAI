package circuitgraph

import (
	"strings"
	"testing"

	"kicadai/internal/reports"
)

func TestDecodePatchStrict(t *testing.T) {
	patch, issues := DecodePatchStrict(strings.NewReader(`{"schema":"kicadai.circuit-patch.v1","version":1,"operations":[{"op":"replace_endpoint","net":"N","endpoint":{"component":"r1","selector_kind":"symbol_pin","selector":"2"},"replacement":{"component":"r1","selector_kind":"symbol_pin","selector":"1"}}]}`))
	if reports.HasBlockingIssue(issues) || len(patch.Operations) != 1 {
		t.Fatalf("patch=%#v issues=%#v", patch, issues)
	}
}

func TestDecodePatchStrictRejectsUnsafeOperations(t *testing.T) {
	for _, input := range []string{
		`{"schema":"kicadai.circuit-patch.v1","version":1,"operations":[{"op":"replace_project"}]}`,
		`{"schema":"kicadai.circuit-patch.v1","version":1,"operations":[{"op":"replace_endpoint","net":"N","endpoint":{"component":"r1","selector_kind":"symbol_pin","selector":"1"},"replacement":{"component":"r2","selector_kind":"symbol_pin","selector":"1"}}]}`,
		`{"schema":"kicadai.circuit-patch.v1","version":1,"operations":[{"op":"replace_policy","policy":"require_drc","enabled":true}]}`,
	} {
		_, issues := DecodePatchStrict(strings.NewReader(input))
		if !reports.HasBlockingIssue(issues) || issues[0].Code != CodePatchInvalid {
			t.Fatalf("input=%s issues=%#v", input, issues)
		}
	}
}
