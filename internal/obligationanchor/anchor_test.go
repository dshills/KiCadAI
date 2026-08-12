package obligationanchor

import (
	"strings"
	"testing"
)

func TestDeriveIsDeterministicAndLengthDelimited(t *testing.T) {
	base := Input{
		CorpusManifestSHA256: strings.Repeat("a", 64), Role: "discovery", CaseID: "case_a",
		OperatingCaseID: "nominal", AssertionID: "output_bound", ObservationKind: "port",
		ObservationID: "signal_output", OutputID: "signal_output",
	}
	first, err := Derive(base)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Derive(base)
	if err != nil || first != second || len(first) != 64 {
		t.Fatalf("derive = %q, %v", second, err)
	}

	changed := base
	changed.AssertionID = "output_bound_a"
	changed.ObservationID, changed.OutputID = "signal_output_b", "signal_output_b"
	third, err := Derive(changed)
	if err != nil {
		t.Fatal(err)
	}
	if third == first {
		t.Fatal("distinct ordered fields produced the same anchor")
	}
}

func TestDeriveCircuitSentinelAndValidation(t *testing.T) {
	base := Input{
		CorpusManifestSHA256: strings.Repeat("b", 64), Role: "held_out", CaseID: "case_b",
		OperatingCaseID: "fault", AssertionID: "thermal_bound", ObservationKind: "circuit",
		ObservationID: "assembly_temperature", OutputID: CircuitOutput,
	}
	if _, err := Derive(base); err != nil {
		t.Fatal(err)
	}
	mutations := []func(*Input){
		func(value *Input) { value.CorpusManifestSHA256 = "bad" },
		func(value *Input) { value.Role = "unknown" },
		func(value *Input) { value.OperatingCaseID = "Bad" },
		func(value *Input) { value.ObservationKind = "node" },
		func(value *Input) { value.OutputID = "assembly_temperature" },
	}
	for index, mutate := range mutations {
		value := base
		mutate(&value)
		if _, err := Derive(value); err == nil {
			t.Fatalf("mutation %d was accepted", index)
		}
	}
}
