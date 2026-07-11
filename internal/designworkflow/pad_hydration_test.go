package designworkflow

import (
	"reflect"
	"strings"
	"testing"

	"kicadai/internal/kicadfiles"
	"kicadai/internal/libraryresolver"
	"kicadai/internal/placement"
	"kicadai/internal/reports"
)

func TestSummarizePadHydrationOrdersMissingRefsAndCountsSources(t *testing.T) {
	summary := summarizePadHydration([]PadHydrationEntry{
		{Ref: "D1", FootprintID: "LED:D", Source: PadHydrationSourceResolver, PadCount: 2},
		{Ref: "R1", FootprintID: "R:R", Source: PadHydrationSourceInput, PadCount: 2},
		{Ref: "J1", FootprintID: "Connector:J", Source: PadHydrationSourceMissing, MissingReason: "not found"},
	}, []reports.Issue{
		{Severity: reports.SeverityBlocked, Refs: []string{"U1"}},
		{Severity: reports.SeverityWarning, Refs: []string{"J1"}},
	})

	if summary.ComponentCount != 3 || summary.HydratedComponents != 2 || summary.MissingComponents != 1 || summary.PadCount != 4 {
		t.Fatalf("summary counts = %#v", summary)
	}
	wantSources := map[PadHydrationSource]int{PadHydrationSourceResolver: 1, PadHydrationSourceInput: 1, PadHydrationSourceMissing: 1}
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

func TestVerifiedPadTemplateUsesPackageSpecificRowPinOrder(t *testing.T) {
	soic, ok := verifiedPadTemplate("Package_SO:SOIC-8_3.9x4.9mm_P1.27mm")
	if !ok {
		t.Fatal("missing SOIC-8 template")
	}
	if got := padTemplateNames(soic.Pads); !reflect.DeepEqual(got, []string{"1", "2", "3", "4", "8", "7", "6", "5"}) {
		t.Fatalf("SOIC pad order = %#v", got)
	}
	sot, ok := verifiedPadTemplate("Package_TO_SOT_SMD:SOT-23-5")
	if !ok {
		t.Fatal("missing SOT-23-5 template")
	}
	if got := padTemplateNames(sot.Pads); !reflect.DeepEqual(got, []string{"1", "2", "3", "5", "4"}) {
		t.Fatalf("SOT pad order = %#v", got)
	}
	if len(sot.Pads) != 5 {
		t.Fatalf("SOT pad count = %d, want 5", len(sot.Pads))
	}
	if sot.Pads[2].Name != "3" || sot.Pads[4].Name != "4" || sot.Pads[2].YMM != sot.Pads[4].YMM {
		t.Fatalf("SOT pin 4 should align with pin 3: %#v", sot.Pads)
	}
	if sot.Bounds.WidthMM < 3.7 {
		t.Fatalf("SOT bounds too narrow: %#v", sot.Bounds)
	}
	if sot.Pads[0].XMM != -1.1375 || sot.Pads[0].WidthMM != 1.325 || sot.Pads[0].HeightMM != 0.6 {
		t.Fatalf("SOT-23-5 pad geometry does not match KiCad library: %#v", sot.Pads[0])
	}
	if sot.Pads[0].YMM == soic.Pads[0].YMM {
		t.Fatalf("expected package-specific pitch, sot=%#v soic=%#v", sot.Pads[0], soic.Pads[0])
	}
	if soic.Bounds.WidthMM < 6.6 {
		t.Fatalf("SOIC bounds too narrow: %#v", soic.Bounds)
	}
}

func TestVerifiedBMP280PadTemplateMatchesKiCadFootprint(t *testing.T) {
	template, ok := verifiedPadTemplate("Package_LGA:Bosch_LGA-8_2x2.5mm_P0.65mm_ClockwisePinNumbering")
	if !ok {
		t.Fatal("missing BMP280 LGA-8 template")
	}
	if got := padTemplateNames(template.Pads); !reflect.DeepEqual(got, []string{"1", "2", "3", "4", "5", "6", "7", "8"}) {
		t.Fatalf("BMP280 pad order = %#v", got)
	}
	wantCenters := []placement.Point{
		{XMM: -0.975, YMM: -0.8}, {XMM: -0.325, YMM: -0.8},
		{XMM: 0.325, YMM: -0.8}, {XMM: 0.975, YMM: -0.8},
		{XMM: 0.975, YMM: 0.8}, {XMM: 0.325, YMM: 0.8},
		{XMM: -0.325, YMM: 0.8}, {XMM: -0.975, YMM: 0.8},
	}
	for index, pad := range template.Pads {
		if pad.XMM != wantCenters[index].XMM || pad.YMM != wantCenters[index].YMM || pad.WidthMM != 0.35 || pad.HeightMM != 0.5 {
			t.Fatalf("BMP280 pad %s = %#v, want center %#v and size 0.35x0.5", pad.Name, pad, wantCenters[index])
		}
	}
}

func TestVerifiedTwoPadTemplatesDoNotOverlap(t *testing.T) {
	for _, footprintID := range []string{
		"Resistor_SMD:R_0603_1608Metric",
		"Resistor_SMD:R_0805_2012Metric",
		"Capacitor_SMD:C_0603_1608Metric",
		"Capacitor_SMD:C_0805_2012Metric",
		"LED_SMD:LED_0805_2012Metric",
	} {
		t.Run(footprintID, func(t *testing.T) {
			template, ok := verifiedPadTemplate(footprintID)
			if !ok {
				t.Fatal("missing template")
			}
			if len(template.Pads) != 2 {
				t.Fatalf("pad count = %d, want 2", len(template.Pads))
			}
			gap := template.Pads[1].XMM - template.Pads[1].WidthMM/2 - (template.Pads[0].XMM + template.Pads[0].WidthMM/2)
			if gap <= 0 {
				t.Fatalf("pads overlap or touch, gap=%v pads=%#v", gap, template.Pads)
			}
		})
	}
}

func TestVerifiedPadTemplateBoundsAreCenteredOnPads(t *testing.T) {
	for _, footprintID := range []string{
		"Connector_PinHeader_2.54mm:PinHeader_1x03_P2.54mm_Vertical",
		"Resistor_SMD:R_0805_2012Metric",
		"Package_SO:SOIC-8_3.9x4.9mm_P1.27mm",
		"Package_TO_SOT_SMD:SOT-223-3_TabPin2",
	} {
		t.Run(footprintID, func(t *testing.T) {
			template, ok := verifiedPadTemplate(footprintID)
			if !ok {
				t.Fatal("missing template")
			}
			if template.Bounds.AnchorOffset.XMM <= 0 || template.Bounds.AnchorOffset.YMM <= 0 {
				t.Fatalf("bounds are not centered: %#v", template.Bounds)
			}
			minX := -template.Bounds.AnchorOffset.XMM
			maxX := minX + template.Bounds.WidthMM
			minY := -template.Bounds.AnchorOffset.YMM
			maxY := minY + template.Bounds.HeightMM
			for _, pad := range template.Pads {
				if pad.XMM-pad.WidthMM/2 < minX || pad.XMM+pad.WidthMM/2 > maxX || pad.YMM-pad.HeightMM/2 < minY || pad.YMM+pad.HeightMM/2 > maxY {
					t.Fatalf("pad outside centered bounds: bounds=%#v pad=%#v", template.Bounds, pad)
				}
			}
		})
	}
}

func TestSOT223TemplateMapsDuplicatePinTwoPads(t *testing.T) {
	template, ok := verifiedPadTemplate("Package_TO_SOT_SMD:SOT-223-3_TabPin2")
	if !ok {
		t.Fatal("missing SOT-223 template")
	}
	index := buildPadNetAssignmentIndex([]placement.Net{{Name: "VOUT", Endpoints: []placement.Endpoint{{Ref: "U1", Pin: "2"}}}})
	pads, issues := assignPadNetsFromIndex("U1", template.Pads, index)
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	var pinTwoCount int
	for _, pad := range pads {
		if pad.Name == "2" && pad.Net == "VOUT" {
			pinTwoCount++
		}
	}
	if pinTwoCount != 2 {
		t.Fatalf("pin 2 pads assigned = %d, pads=%#v", pinTwoCount, pads)
	}
}

func TestUSBCHROTemplateHydratesPowerOnlyPads(t *testing.T) {
	template, ok := verifiedPadTemplate("Connector_USB:USB_C_Receptacle_HRO_TYPE-C-31-M-12")
	if !ok {
		t.Fatal("missing USB-C HRO template")
	}
	if len(template.Pads) != 20 {
		t.Fatalf("pad count = %d, want 20", len(template.Pads))
	}
	for _, name := range []string{"A1", "A4", "A5", "A9", "A12", "B1", "B4", "B5", "B9", "B12", "SH"} {
		if !padTemplateHasName(template.Pads, name) {
			t.Fatalf("missing USB-C pad %s in %#v", name, padTemplateNames(template.Pads))
		}
	}
	if template.Bounds.WidthMM < 9.6 || template.Bounds.HeightMM < 7.0 {
		t.Fatalf("USB-C bounds too small: %#v", template.Bounds)
	}

	index := buildPadNetAssignmentIndex([]placement.Net{{Name: "SHIELD", Endpoints: []placement.Endpoint{{Ref: "J1", Pin: "SH"}}}})
	pads, issues := assignPadNetsFromIndex("J1", template.Pads, index)
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	var shieldCount int
	for _, pad := range pads {
		if strings.HasPrefix(pad.Name, "SH") && pad.Net == "SHIELD" {
			shieldCount++
		}
	}
	if shieldCount != 4 {
		t.Fatalf("shield pads assigned = %d, pads=%#v", shieldCount, pads)
	}
	if duplicated := duplicatePadTemplateNames(template.Pads); len(duplicated) != 0 {
		t.Fatalf("routing template contains duplicate pad names: %#v", duplicated)
	}
}

func TestUSBCGCTPowerOnlyTemplateHydratesVerifiedPads(t *testing.T) {
	template, ok := verifiedPadTemplate("Connector_USB:USB_C_Receptacle_GCT_USB4125-xx-x_6P_TopMnt_Horizontal")
	if !ok {
		t.Fatal("missing USB-C GCT template")
	}
	if len(template.Pads) != 10 {
		t.Fatalf("pad count = %d, want 10", len(template.Pads))
	}
	for _, name := range []string{"A5", "A9", "A12", "B5", "B9", "B12", "SH", "SH2", "SH3", "SH4"} {
		if !padTemplateHasName(template.Pads, name) {
			t.Fatalf("missing USB-C GCT pad %s in %#v", name, padTemplateNames(template.Pads))
		}
	}
	if duplicated := duplicatePadTemplateNames(template.Pads); len(duplicated) != 0 {
		t.Fatalf("routing template contains duplicate pad names: %#v", duplicated)
	}

	index := buildPadNetAssignmentIndex([]placement.Net{{Name: "SHIELD", Endpoints: []placement.Endpoint{{Ref: "J1", Pin: "SH"}}}})
	pads, issues := assignPadNetsFromIndex("J1", template.Pads, index)
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	var shieldCount int
	for _, pad := range pads {
		if strings.HasPrefix(pad.Name, "SH") && pad.Net == "SHIELD" {
			shieldCount++
		}
	}
	if shieldCount != 4 {
		t.Fatalf("shield pads assigned = %d, pads=%#v", shieldCount, pads)
	}
}

func TestUSBShieldPadMatcherRequiresNumericSuffix(t *testing.T) {
	padByName := map[string][]int{
		"SH":      {1},
		"SH2":     {2},
		"SHIELD":  {3},
		"SHIELD1": {4},
		"SHUNT":   {5},
	}
	got := matchingPadIndexesForPin(padByName, " sh ")
	if !reflect.DeepEqual(got, []int{1, 2, 3, 4}) {
		t.Fatalf("shield pad matches = %#v, want SH/SH2/SHIELD only", got)
	}
	got = matchingPadIndexesForPin(padByName, "SHIELD")
	if !reflect.DeepEqual(got, []int{1, 2, 3, 4}) {
		t.Fatalf("shield alias pad matches = %#v, want SH/SH2/SHIELD only", got)
	}
}

func padTemplateNames(pads []placement.PadSummary) []string {
	names := make([]string, 0, len(pads))
	for _, pad := range pads {
		names = append(names, pad.Name)
	}
	return names
}

func padTemplateHasName(pads []placement.PadSummary, name string) bool {
	for _, pad := range pads {
		if pad.Name == name {
			return true
		}
	}
	return false
}

func duplicatePadTemplateNames(pads []placement.PadSummary) []string {
	seen := map[string]struct{}{}
	duplicates := []string{}
	for _, pad := range pads {
		if _, exists := seen[pad.Name]; exists {
			duplicates = append(duplicates, pad.Name)
			continue
		}
		seen[pad.Name] = struct{}{}
	}
	return duplicates
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
