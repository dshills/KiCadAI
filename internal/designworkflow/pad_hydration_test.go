package designworkflow

import (
	"reflect"
	"testing"

	"kicadai/internal/kicadfiles"
	"kicadai/internal/libraryresolver"
	"kicadai/internal/placement"
	"kicadai/internal/reports"
)

func TestSummarizePadHydrationOrdersMissingRefsAndCountsSources(t *testing.T) {
	summary := summarizePadHydration([]PadHydrationEntry{
		{Ref: "D1", FootprintID: "LED:D", Source: PadHydrationSourceResolver, PadCount: 2},
		{Ref: "R1", FootprintID: "R:R", Source: PadHydrationSourceVerifiedTemplate, PadCount: 2},
		{Ref: "J1", FootprintID: "Connector:J", Source: PadHydrationSourceMissing, MissingReason: "not found"},
	}, []reports.Issue{
		{Severity: reports.SeverityBlocked, Refs: []string{"U1"}},
		{Severity: reports.SeverityWarning, Refs: []string{"J1"}},
	})

	if summary.ComponentCount != 3 || summary.HydratedComponents != 2 || summary.MissingComponents != 1 || summary.PadCount != 4 {
		t.Fatalf("summary counts = %#v", summary)
	}
	wantSources := map[PadHydrationSource]int{PadHydrationSourceResolver: 1, PadHydrationSourceVerifiedTemplate: 1, PadHydrationSourceMissing: 1}
	if !reflect.DeepEqual(summary.SourceCounts, wantSources) {
		t.Fatalf("source counts = %#v, want %#v", summary.SourceCounts, wantSources)
	}
	if !reflect.DeepEqual(summary.MissingRefs, []string{"J1"}) {
		t.Fatalf("missing refs = %#v", summary.MissingRefs)
	}
	if summary.BlockingIssues != 1 {
		t.Fatalf("blocking issues = %d", summary.BlockingIssues)
	}
}

func TestSummarizePadHydrationTreatsEmptySourceAsMissing(t *testing.T) {
	summary := summarizePadHydration([]PadHydrationEntry{{Ref: "X1"}}, nil)
	if summary.SourceCounts["missing"] != 1 || summary.MissingComponents != 1 {
		t.Fatalf("summary = %#v", summary)
	}
}

func TestHydratePadsFromResolverExtractsBoundsAndPads(t *testing.T) {
	index := libraryresolver.LibraryIndex{Footprints: map[string]libraryresolver.FootprintRecord{
		"Test:R": {
			FootprintID: "Test:R",
			BoundingBox: libraryresolver.BoundingBox{
				Min: kicadfiles.Point{X: -1_000_000, Y: -500_000},
				Max: kicadfiles.Point{X: 1_000_000, Y: 500_000},
			},
			Pads: []libraryresolver.FootprintPad{
				{Name: " 1 ", Position: kicadfiles.Point{X: -600_000}, Size: kicadfiles.Point{X: 500_000, Y: 600_000}},
				{Name: "2", Position: kicadfiles.Point{X: 600_000}, Size: kicadfiles.Point{X: 500_000, Y: 600_000}},
			},
		},
	}}

	resolver := padHydrationResolver{index: index}
	result := resolver.Hydrate("R1", "Test:R")
	if len(result.Issues) != 0 {
		t.Fatalf("issues = %#v", result.Issues)
	}
	if result.Entry.Source != PadHydrationSourceResolver || result.Entry.PadCount != 2 {
		t.Fatalf("entry = %#v", result.Entry)
	}
	if result.Bounds.WidthMM != 2 || result.Bounds.HeightMM != 1 {
		t.Fatalf("bounds = %#v", result.Bounds)
	}
	if result.Pads[0].Name != "1" || result.Pads[0].WidthMM != 0.5 || result.Pads[0].HeightMM != 0.6 {
		t.Fatalf("pads = %#v", result.Pads)
	}
}

func TestHydratePadsFromResolverBlocksMissingFootprint(t *testing.T) {
	resolver := padHydrationResolver{index: libraryresolver.LibraryIndex{}}
	result := resolver.Hydrate("U1", "Package:Missing")
	if result.Entry.Source != PadHydrationSourceMissing || result.Entry.MissingReason == "" {
		t.Fatalf("entry = %#v", result.Entry)
	}
	if len(result.Issues) != 1 || !result.Issues[0].Blocking() {
		t.Fatalf("issues = %#v", result.Issues)
	}
}

func TestHydratePadsFromResolverRejectsInvalidPadGeometry(t *testing.T) {
	index := libraryresolver.LibraryIndex{Footprints: map[string]libraryresolver.FootprintRecord{
		"Test:Bad": {
			FootprintID: "Test:Bad",
			BoundingBox: libraryresolver.BoundingBox{
				Min: kicadfiles.Point{X: -1_000_000, Y: -500_000},
				Max: kicadfiles.Point{X: 1_000_000, Y: 500_000},
			},
			Pads: []libraryresolver.FootprintPad{{Name: "", Size: kicadfiles.Point{X: 500_000, Y: 600_000}}},
		},
	}}

	resolver := padHydrationResolver{index: index}
	result := resolver.Hydrate("X1", "Test:Bad")
	if result.Entry.Source != PadHydrationSourceMissing || result.Entry.MissingReason != "no routable footprint pads" {
		t.Fatalf("entry = %#v", result.Entry)
	}
	if len(result.Pads) != 0 || len(result.Issues) == 0 {
		t.Fatalf("result = %#v", result)
	}
}

func TestPadHydrationResolverReusesFootprintDataWithPerRefContext(t *testing.T) {
	index := libraryresolver.LibraryIndex{Footprints: map[string]libraryresolver.FootprintRecord{
		"Test:R": {
			FootprintID: "Test:R",
			BoundingBox: libraryresolver.BoundingBox{
				Min: kicadfiles.Point{X: -1_000_000, Y: -500_000},
				Max: kicadfiles.Point{X: 1_000_000, Y: 500_000},
			},
			Pads: []libraryresolver.FootprintPad{{Name: "1", Size: kicadfiles.Point{X: 500_000, Y: 600_000}}},
		},
	}}
	resolver := padHydrationResolver{index: index}

	first := resolver.Hydrate("R1", "Test:R")
	second := resolver.Hydrate("R2", "Test:R")
	if len(resolver.cache) != 1 {
		t.Fatalf("cache size = %d, want 1", len(resolver.cache))
	}
	if first.Entry.Ref != "R1" || second.Entry.Ref != "R2" {
		t.Fatalf("entries did not get per-ref context: first=%#v second=%#v", first.Entry, second.Entry)
	}
	second.Pads[0].Net = "SIG"
	if first.Pads[0].Net == "SIG" {
		t.Fatalf("cached pad summaries were not copied per result")
	}
}

func TestAssignPadNetsMapsEndpointPinsToPads(t *testing.T) {
	index := buildPadNetAssignmentIndex([]placement.Net{{
		Name: "LED_SERIES",
		Endpoints: []placement.Endpoint{
			{Ref: "R1", Pin: "2"},
			{Ref: "D1", Pin: "1"},
		},
	}})
	pads, issues := assignPadNetsFromIndex("D1", []placement.PadSummary{{Name: "1"}, {Name: "2"}}, index)
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	if pads[0].Net != "LED_SERIES" || pads[1].Net != "" {
		t.Fatalf("pads = %#v", pads)
	}
}

func TestAssignPadNetsMapsSharedPadNames(t *testing.T) {
	index := buildPadNetAssignmentIndex([]placement.Net{{
		Name:      "GND",
		Endpoints: []placement.Endpoint{{Ref: "U1", Pin: "GND"}},
	}})
	pads, issues := assignPadNetsFromIndex("U1", []placement.PadSummary{{Name: "GND"}, {Name: "GND"}, {Name: "VCC"}}, index)
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	if pads[0].Net != "GND" || pads[1].Net != "GND" || pads[2].Net != "" {
		t.Fatalf("pads = %#v", pads)
	}
}

func TestBuildPadNetAssignmentIndexGroupsByRef(t *testing.T) {
	index := buildPadNetAssignmentIndex([]placement.Net{
		{Name: "A", Endpoints: []placement.Endpoint{{Ref: "R1", Pin: "1"}, {Ref: "R2", Pin: "1"}}},
		{Name: "B", Endpoints: []placement.Endpoint{{Ref: "R1", Pin: "2"}}},
	})
	if len(index["R1"]) != 2 || len(index["R2"]) != 1 {
		t.Fatalf("index = %#v", index)
	}
}

func TestAssignPadNetsBlocksMissingPadMapping(t *testing.T) {
	index := buildPadNetAssignmentIndex([]placement.Net{{
		Name:      "LED_SERIES",
		Endpoints: []placement.Endpoint{{Ref: "D1", Pin: "2"}},
	}})
	_, issues := assignPadNetsFromIndex("D1", []placement.PadSummary{{Name: "1"}}, index)
	if len(issues) != 1 || !issues[0].Blocking() {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestAssignPadNetsBlocksConflictingNetAssignment(t *testing.T) {
	index := buildPadNetAssignmentIndex([]placement.Net{
		{Name: "A", Endpoints: []placement.Endpoint{{Ref: "D1", Pin: "1"}}},
		{Name: "B", Endpoints: []placement.Endpoint{{Ref: "D1", Pin: "1"}}},
	})
	_, issues := assignPadNetsFromIndex("D1", []placement.PadSummary{{Name: "1"}}, index)
	if len(issues) != 1 || !issues[0].Blocking() {
		t.Fatalf("issues = %#v", issues)
	}
}
