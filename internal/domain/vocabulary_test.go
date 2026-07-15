package domain

import "testing"

func TestVocabularyPreservesWireSpellings(t *testing.T) {
	tests := []struct {
		name string
		got  string
		want string
	}{
		{"acceptance erc-drc", string(AcceptanceERCDRC), "erc-drc"},
		{"acceptance erc_drc", string(AcceptanceERCDRCUnderscore), "erc_drc"},
		{"acceptance fabrication-candidate", string(AcceptanceFabricationCandidate), "fabrication-candidate"},
		{"acceptance fabrication_candidate", string(AcceptanceFabricationCandidateUnderscore), "fabrication_candidate"},
		{"component indicator", string(ComponentRoleIndicatorLED), "indicator_led"},
		{"net power positive", string(NetRolePowerPos), "power_pos"},
		{"net no-connect", string(NetRoleNoConnect), "no_connect"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.got != test.want {
				t.Fatalf("got %q, want %q", test.got, test.want)
			}
		})
	}
}

func TestVocabularyAliasesAreAssignable(t *testing.T) {
	var acceptance AcceptanceLevel = AcceptanceERCDRC
	var component ComponentRole = ComponentRoleIC
	var role NetRole = NetRoleSignal
	if acceptance != "erc-drc" || component != "ic" || role != "signal" {
		t.Fatalf("unexpected vocabulary values: %q, %q, %q", acceptance, component, role)
	}
}
