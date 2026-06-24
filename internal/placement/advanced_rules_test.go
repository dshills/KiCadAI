package placement

import "testing"

func TestAdvancedPlacementRulesMixedRegression(t *testing.T) {
	req := Request{
		Board: BoardPlacementArea{WidthMM: 60, HeightMM: 35, MarginMM: 1},
		Components: []Component{
			{Ref: "J1", Role: "connector", FootprintID: "Connector:Conn_01x04", Bounds: Bounds{WidthMM: 4, HeightMM: 8, Source: BoundsExplicit}, Fixed: true, Position: &Placement{XMM: 5, YMM: 15, Layer: "F.Cu"}, Pads: []PadSummary{{Name: "1"}, {Name: "2"}, {Name: "3"}}},
			{Ref: "U1", Role: "mcu", FootprintID: "Package_QFP:TQFP-32", Bounds: Bounds{WidthMM: 7, HeightMM: 7, Source: BoundsExplicit}, Pads: []PadSummary{{Name: "1"}, {Name: "2"}, {Name: "3"}}},
			{Ref: "U2", Role: "regulator", FootprintID: "Package_TO_SOT_SMD:SOT-223", Bounds: Bounds{WidthMM: 6, HeightMM: 7, Source: BoundsExplicit}, Pads: []PadSummary{{Name: "1"}, {Name: "2"}}},
			{Ref: "S1", Role: "sensor", FootprintID: "Package_SO:SOIC-8", Bounds: Bounds{WidthMM: 5, HeightMM: 4, Source: BoundsExplicit}, Pads: []PadSummary{{Name: "1"}, {Name: "2"}}},
		},
		Nets: []Net{
			{Name: "VBUS", Role: NetPower, Endpoints: []Endpoint{{Ref: "J1", Pin: "1"}, {Ref: "U2", Pin: "1"}}},
			{Name: "USB_P", Role: NetDifferential, Endpoints: []Endpoint{{Ref: "J1", Pin: "2"}, {Ref: "U1", Pin: "1"}}},
			{Name: "USB_N", Role: NetDifferential, Endpoints: []Endpoint{{Ref: "J1", Pin: "3"}, {Ref: "U1", Pin: "2"}}},
			{Name: "SENSE", Role: NetAnalog, Endpoints: []Endpoint{{Ref: "S1", Pin: "1"}, {Ref: "U1", Pin: "3"}}},
		},
		AdvancedRules: AdvancedPlacementRules{
			Thermal: []ThermalPlacementRule{{
				ID:            "regulator-thermal",
				Refs:          []string{"U2"},
				KeepAwayRefs:  []string{"S1"},
				PreferredEdge: EdgeRight,
				ThermalRole:   ThermalRoleRegulator,
			}},
			HighCurrent: []HighCurrentPlacementRule{{
				ID:         "vbus-path",
				Nets:       []string{"VBUS"},
				SourceRefs: []string{"J1"},
				SinkRefs:   []string{"U2"},
			}},
			CreepageClearance: []CreepageClearancePlacementRule{{
				ID:             "power-analog-gap",
				DomainA:        PlacementRuleDomain{Nets: []string{"VBUS"}},
				DomainB:        PlacementRuleDomain{Nets: []string{"SENSE"}},
				MinClearanceMM: 3,
				MinCreepageMM:  4,
			}},
			DifferentialPair: []DifferentialPairPlacementRule{{
				ID:          "usb-pair",
				PositiveNet: "USB_P",
				NegativeNet: "USB_N",
				SourceRefs:  []string{"J1"},
				SinkRefs:    []string{"U1"},
			}},
			ControlledImpedance: []ControlledImpedancePlacementRule{{
				ID:              "usb-impedance",
				Nets:            []string{"USB_P", "USB_N"},
				SourceRefs:      []string{"J1"},
				SinkRefs:        []string{"U1"},
				PreferredLayers: []string{"F.Cu"},
			}},
		},
		Rules: DefaultRules(),
		Seed:  "advanced-regression",
	}
	req.Rules.CandidateScoring.Enabled = true

	first := Place(CloneRequest(req))
	second := Place(CloneRequest(req))
	if first.Status != StatusPlaced {
		t.Fatalf("status = %s issues=%#v", first.Status, first.Issues)
	}
	if !placementsNearlyEqual(first.Placements, second.Placements) {
		t.Fatalf("advanced placement not deterministic: first=%#v second=%#v", first.Placements, second.Placements)
	}
	if first.CandidateScoring == nil || len(first.CandidateScoring.WinningCandidates) == 0 {
		t.Fatalf("candidate scoring missing: %#v", first.CandidateScoring)
	}
	dimensions := map[CandidateScoreDimensionName]bool{}
	for _, winner := range first.CandidateScoring.WinningCandidates {
		for _, dimension := range winner.Dimensions {
			dimensions[dimension.Name] = true
		}
	}
	var missing []CandidateScoreDimensionName
	for _, want := range []CandidateScoreDimensionName{
		CandidateScoreThermal,
		CandidateScoreHighCurrent,
		CandidateScoreCreepageClearance,
		CandidateScoreDifferentialPair,
		CandidateScoreControlledImpedance,
	} {
		if !dimensions[want] {
			missing = append(missing, want)
		}
	}
	if len(missing) > 0 {
		t.Fatalf("missing advanced dimensions %v; found %v", missing, dimensionNames(dimensions))
	}
}

func dimensionNames(values map[CandidateScoreDimensionName]bool) []CandidateScoreDimensionName {
	names := make([]CandidateScoreDimensionName, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	return names
}
