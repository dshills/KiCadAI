package architecturesearch

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestV5EventObservationMustUseDeclaringOperatingCase(t *testing.T) {
	contents, err := os.ReadFile(filepath.Join(frozenDynamicElectrothermalCorpusRoot(), "class_ab_dynamic_output_stage.json"))
	if err != nil {
		t.Fatal(err)
	}
	requirement, issues := DecodeStrict(strings.NewReader(string(contents)))
	if len(issues) != 0 {
		t.Fatalf("decode fixture: %#v", issues)
	}
	found := false
	for index := range requirement.Requirements.BehavioralRequirements {
		behavior := &requirement.Requirements.BehavioralRequirements[index]
		if behavior.ID == "short_fault_response" {
			behavior.OperatingCases = []string{"reactive_audio_load"}
			found = true
			break
		}
	}
	if !found {
		t.Fatal("fixture no longer contains short_fault_response")
	}
	issues = Validate(requirement)
	if len(issues) == 0 {
		t.Fatal("cross-case event observation was accepted")
	}
	found = false
	for _, issue := range issues {
		if strings.Contains(issue.Message, "operating case that declares the event") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("cross-case event diagnostic is missing: %#v", issues)
	}
}
