package schematicir

import "kicadai/internal/schematiclayout"

func validateFunctionalLayout(document Document, add func(string, string)) {
	layout := document.Layout
	if layout.FunctionalProfile == "" {
		if len(layout.FunctionalOwners) != 0 {
			add("layout.functional_owners", "owners require a functional profile")
		}
		return
	}
	if layout.FunctionalProfile != schematiclayout.FunctionalOwnershipV1 || layout.NativeProfile != schematiclayout.NativeAnnotationV2 {
		add("layout.functional_profile", "unsupported functional profile or missing annotation-v2")
	}
	components := indexComponentsByID(document.Circuit.Components)
	membership := map[string]string{}
	for _, group := range layout.Groups {
		if group.Inferred || group.RankPolicy != "" {
			add("layout.groups", "functional groups require explicit membership")
		}
		for _, member := range group.Members {
			if membership[member] != "" {
				add("layout.groups", "duplicate functional membership")
			}
			membership[member] = group.ID
		}
	}
	owners := map[string]FunctionalOwner{}
	for _, owner := range layout.FunctionalOwners {
		_, known := components[owner.Component]
		_, duplicate := owners[owner.Component]
		if !known || duplicate || owner.Group == "" || membership[owner.Component] != owner.Group || owner.Source == "" {
			add("layout.functional_owners", "invalid functional membership or provenance")
		}
		owners[owner.Component] = owner
	}
	for id := range components {
		if _, ok := owners[id]; !ok {
			add("layout.functional_owners", "every component requires explicit ownership")
		}
	}
	for _, owner := range owners {
		seen := map[string]bool{owner.Component: true}
		for parent := owner.Parent; parent != ""; {
			p, ok := owners[parent]
			if !ok || seen[parent] || p.Group != owner.Group {
				add("layout.functional_owners", "invalid or cyclic support parent")
				break
			}
			seen[parent] = true
			parent = p.Parent
		}
	}
}
