package blocks

import "testing"

func TestResolveCompositionNetAliasesMapsConnectedPortsToCanonicalNet(t *testing.T) {
	resolutions, issues := ResolveCompositionNetAliases(CompositionRequest{
		Connections: []CompositionConnection{
			{From: PortRef{InstanceID: "output", Port: "AMP_OUT"}, To: PortRef{InstanceID: "protection", Port: "AMP_OUT"}, NetAlias: "AMP_OUT_DC_BIASED"},
			{From: PortRef{InstanceID: "protection", Port: "HP_OUT"}, To: PortRef{InstanceID: "headphones", Port: "SIG"}, NetAlias: "HP_OUT"},
		},
	})
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	want := map[string]string{
		"output_AMP_OUT":     "AMP_OUT_DC_BIASED",
		"protection_AMP_OUT": "AMP_OUT_DC_BIASED",
		"protection_HP_OUT":  "HP_OUT",
		"headphones_SIG":     "HP_OUT",
	}
	got := map[string]string{}
	for _, resolution := range resolutions {
		got[resolution.LocalNet] = resolution.CanonicalNet
	}
	for local, canonical := range want {
		if got[local] != canonical {
			t.Fatalf("resolution[%s] = %q, want %q in %#v", local, got[local], canonical, got)
		}
	}
}

func TestResolveCompositionNetAliasesPreservesDistinctReferenceDomains(t *testing.T) {
	resolutions, issues := ResolveCompositionNetAliases(CompositionRequest{
		Connections: []CompositionConnection{
			{From: PortRef{InstanceID: "protection", Port: "LOAD_REF"}, To: PortRef{InstanceID: "headphones", Port: "LOAD_REF"}, NetAlias: "LOAD_REF"},
			{From: PortRef{InstanceID: "protection", Port: "LOAD_RET"}, To: PortRef{InstanceID: "headphones", Port: "RET"}, NetAlias: "HP_RET"},
			{From: PortRef{InstanceID: "power", Port: "VCC"}, To: PortRef{InstanceID: "output", Port: "VCC"}, NetAlias: "VCC"},
		},
	})
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	got := map[string]string{}
	for _, resolution := range resolutions {
		got[resolution.LocalNet] = resolution.CanonicalNet
	}
	for local, canonical := range map[string]string{
		"protection_LOAD_REF": "LOAD_REF",
		"headphones_LOAD_REF": "LOAD_REF",
		"protection_LOAD_RET": "HP_RET",
		"headphones_RET":      "HP_RET",
		"power_VCC":           "VCC",
		"output_VCC":          "VCC",
	} {
		if got[local] != canonical {
			t.Fatalf("resolution[%s] = %q, want %q in %#v", local, got[local], canonical, got)
		}
	}
	if got["protection_LOAD_REF"] == got["protection_LOAD_RET"] {
		t.Fatalf("LOAD_REF and LOAD_RET were merged in %#v", got)
	}
	if got["output_VCC"] != "VCC" {
		t.Fatalf("power rail alias changed: %#v", got)
	}
}

func TestResolveCompositionNetAliasesRejectsConflictingUserAliases(t *testing.T) {
	_, issues := ResolveCompositionNetAliases(CompositionRequest{
		Connections: []CompositionConnection{
			{From: PortRef{InstanceID: "a", Port: "SIG"}, To: PortRef{InstanceID: "b", Port: "SIG"}, NetAlias: "USER_A"},
			{From: PortRef{InstanceID: "b", Port: "SIG"}, To: PortRef{InstanceID: "c", Port: "SIG"}, NetAlias: "USER_B"},
		},
	})
	if len(issues) != 1 || issues[0].Path != "connections[1].net_alias" {
		t.Fatalf("issues = %#v", issues)
	}
}
