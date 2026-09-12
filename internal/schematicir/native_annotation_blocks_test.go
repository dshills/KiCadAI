package schematicir

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"kicadai/internal/schematiclayout"
)

func TestNativeBlockMetadataAndProfileIsolation(t *testing.T) {
	d := validLEDDocument()
	d.Layout.NativeProfile = schematiclayout.NativeAnnotationV2
	d.Layout.FunctionalProfile = schematiclayout.FunctionalOwnershipV3
	d.Layout.Groups = []Group{{ID: "local", Label: "fragment:power input", Members: []string{"vin", "r_limit", "led"}}}
	d.Circuit.Components[0].Pins[0].Name = "SUPPLY"
	before, _ := json.Marshal(d.Circuit)
	blocks := nativeSchematicBlocks(d)
	if len(blocks) != 1 || blocks[0].Lines[0] != "POWER INPUT" {
		t.Fatal(blocks)
	}
	lines := strings.Join(blocks[0].Lines, "\n")
	for _, want := range []string{"J1.SUPPLY(1): R1.1", "J1.2: Ground (net role)"} {
		if !strings.Contains(lines, want) {
			t.Fatal(lines)
		}
	}
	if strings.Contains(lines, "3.3") || len(nativeSchematicNotes(d)) != 0 {
		t.Fatal("invented voltage or duplicate global guide")
	}
	after, _ := json.Marshal(d.Circuit)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("annotation mutated circuit")
	}
	for _, profile := range []string{"", schematiclayout.FunctionalOwnershipV1, schematiclayout.FunctionalOwnershipV2} {
		d.Layout.FunctionalProfile = profile
		if nativeSchematicBlocks(d) != nil {
			t.Fatal("blocks changed old profile")
		}
	}
	d.Layout.FunctionalProfile = schematiclayout.FunctionalOwnershipV3
	d.Layout.Groups[0].Label = ""
	if got := nativeSchematicBlocks(d)[0].Lines[0]; got != "LOCAL" {
		t.Fatal("missing label not grounded in group ID", got)
	}
}

func TestFunctionalPinAwareProvenanceValidation(t *testing.T) {
	d := validLEDDocument()
	d.Layout.NativeProfile = schematiclayout.NativeAnnotationV2
	d.Layout.FunctionalProfile = schematiclayout.FunctionalOwnershipV3
	d.Layout.Groups = []Group{{ID: "block"}}
	for _, c := range d.Circuit.Components {
		d.Layout.Groups[0].Members = append(d.Layout.Groups[0].Members, c.ID)
		d.Layout.FunctionalOwners = append(d.Layout.FunctionalOwners, FunctionalOwner{Component: c.ID, Group: "block", Source: "fragment:fixture"})
	}
	count := func() int { n := 0; validateFunctionalLayout(d, func(string, string) { n++ }); return n }
	if count() != 0 {
		t.Fatal("valid v3 ownership rejected")
	}
	d.Layout.FunctionalOwners[0].Source = "synthesis-support-parent"
	if count() == 0 {
		t.Fatal("missing parent accepted")
	}
}
