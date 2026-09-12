package schematicir

import (
	"kicadai/internal/schematiclayout"
	"testing"
)

func TestFunctionalOwnershipValidation(t *testing.T) {
	d := validLEDDocument()
	d.Layout.NativeProfile = schematiclayout.NativeAnnotationV2
	d.Layout.FunctionalProfile = schematiclayout.FunctionalOwnershipV1
	d.Layout.Groups = []Group{{ID: "block", Members: []string{}}}
	for _, c := range d.Circuit.Components {
		d.Layout.Groups[0].Members = append(d.Layout.Groups[0].Members, c.ID)
		d.Layout.FunctionalOwners = append(d.Layout.FunctionalOwners, FunctionalOwner{Component: c.ID, Group: "block", Source: "explicit-test"})
	}
	count := func(doc Document) int { n := 0; validateFunctionalLayout(doc, func(string, string) { n++ }); return n }
	if count(d) != 0 {
		t.Fatal("valid ownership rejected")
	}
	for name, change := range map[string]func(*Document){
		"missing": func(d *Document) { d.Layout.FunctionalOwners = d.Layout.FunctionalOwners[1:] },
		"duplicate": func(d *Document) {
			d.Layout.FunctionalOwners = append(d.Layout.FunctionalOwners, d.Layout.FunctionalOwners[0])
		},
		"group":    func(d *Document) { d.Layout.FunctionalOwners[0].Group = "absent" },
		"source":   func(d *Document) { d.Layout.FunctionalOwners[0].Source = "" },
		"cycle":    func(d *Document) { d.Layout.FunctionalOwners[0].Parent = d.Layout.FunctionalOwners[0].Component },
		"parent":   func(d *Document) { d.Layout.FunctionalOwners[0].Parent = "absent" },
		"profile":  func(d *Document) { d.Layout.FunctionalProfile = "unknown" },
		"legacy":   func(d *Document) { d.Layout.FunctionalProfile = "" },
		"inferred": func(d *Document) { d.Layout.Groups[0].RankPolicy = "inferred-v1" },
	} {
		t.Run(name, func(t *testing.T) {
			copy := d
			copy.Layout = CloneLayout(d.Layout)
			change(&copy)
			if count(copy) == 0 {
				t.Fatal("invalid ownership accepted")
			}
		})
	}
}

func TestFunctionalOwnershipV2SourceValidation(t *testing.T) {
	d := validLEDDocument()
	d.Layout.NativeProfile = schematiclayout.NativeAnnotationV2
	d.Layout.FunctionalProfile = schematiclayout.FunctionalOwnershipV2
	d.Layout.Groups = []Group{{ID: "block"}}
	for _, c := range d.Circuit.Components {
		d.Layout.Groups[0].Members = append(d.Layout.Groups[0].Members, c.ID)
		d.Layout.FunctionalOwners = append(d.Layout.FunctionalOwners, FunctionalOwner{Component: c.ID, Group: "block", Source: "fragment:fixture"})
	}
	count := func(doc Document) int { n := 0; validateFunctionalLayout(doc, func(string, string) { n++ }); return n }
	if count(d) != 0 {
		t.Fatal("valid v2 ownership rejected")
	}
	for _, source := range []string{"unknown", "fragment:", "fragment: ", "synthesis-support-parent"} {
		copy := d
		copy.Layout = CloneLayout(d.Layout)
		copy.Layout.FunctionalOwners[0].Source = source
		if count(copy) == 0 {
			t.Fatal("inconsistent source accepted", source)
		}
	}
	copy := d
	copy.Layout = CloneLayout(d.Layout)
	copy.Layout.FunctionalOwners[0].Parent = copy.Layout.FunctionalOwners[1].Component
	if count(copy) == 0 {
		t.Fatal("fragment parent conflict accepted")
	}
}
