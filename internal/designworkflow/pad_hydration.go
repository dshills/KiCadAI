package designworkflow

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"

	"kicadai/internal/libraryresolver"
	"kicadai/internal/placement"
	"kicadai/internal/reports"
)

type PadHydrationSource string

const (
	PadHydrationSourceResolver         PadHydrationSource = "resolver"
	PadHydrationSourceVerifiedTemplate PadHydrationSource = "verified_template"
	PadHydrationSourceInput            PadHydrationSource = "input"
	PadHydrationSourceMissing          PadHydrationSource = "missing"
)

type PadHydrationEntry struct {
	Ref           string             `json:"ref"`
	FootprintID   string             `json:"footprint_id,omitempty"`
	Source        PadHydrationSource `json:"source"`
	PadCount      int                `json:"pad_count,omitempty"`
	MissingReason string             `json:"missing_reason,omitempty"`
}

type PadHydrationSummary struct {
	ComponentCount     int                        `json:"component_count"`
	HydratedComponents int                        `json:"hydrated_components"`
	MissingComponents  int                        `json:"missing_components"`
	PadCount           int                        `json:"pad_count"`
	SourceCounts       map[PadHydrationSource]int `json:"source_counts,omitempty"`
	MissingRefs        []string                   `json:"missing_refs,omitempty"`
	BlockingIssues     int                        `json:"blocking_issues,omitempty"`
}

type padHydrationResult struct {
	Bounds placement.Bounds
	Pads   []placement.PadSummary
	Entry  PadHydrationEntry
	Issues []reports.Issue
}

type padHydrationResolver struct {
	index libraryresolver.LibraryIndex
	cache map[string]padHydrationResult
	mu    sync.Mutex
}

func routingPadNames(pads []placement.PadSummary) []string {
	names := make([]string, len(pads))
	for index, pad := range pads {
		names[index] = pad.Name
	}
	return uniqueRoutingPadNames(names)
}

func uniqueRoutingPadNames(names []string) []string {
	out := make([]string, len(names))
	reserved := make(map[string]struct{}, len(names))
	for _, name := range names {
		if name := strings.ToUpper(strings.TrimSpace(name)); name != "" {
			reserved[name] = struct{}{}
		}
	}
	assigned := make(map[string]struct{}, len(names))
	occurrences := map[string]int{}
	for index, name := range names {
		base := strings.TrimSpace(name)
		key := strings.ToUpper(base)
		if key == "" {
			continue
		}
		occurrences[key]++
		if _, exists := assigned[key]; !exists {
			out[index] = base
			assigned[key] = struct{}{}
			continue
		}
		for suffix := max(2, occurrences[key]); ; suffix++ {
			candidate := base + "#" + strconv.Itoa(suffix)
			candidateKey := strings.ToUpper(candidate)
			if _, conflicts := reserved[candidateKey]; conflicts {
				continue
			}
			if _, conflicts := assigned[candidateKey]; conflicts {
				continue
			}
			out[index] = candidate
			assigned[candidateKey] = struct{}{}
			occurrences[key] = suffix
			break
		}
	}
	return out
}

func (resolver *padHydrationResolver) Hydrate(ref string, footprintID string) padHydrationResult {
	ref = strings.TrimSpace(ref)
	footprintID = strings.TrimSpace(footprintID)
	resolver.mu.Lock()
	resolver.ensureCache()
	base, ok := resolver.cache[footprintID]
	resolver.mu.Unlock()
	if ok {
		return padHydrationResultForRef(base, ref)
	}

	base = hydratePadsFromResolverRecord(resolver.index, "", footprintID)
	resolver.mu.Lock()
	resolver.ensureCache()
	if cached, ok := resolver.cache[footprintID]; ok {
		base = cached
	} else {
		resolver.cache[footprintID] = base
	}
	resolver.mu.Unlock()
	return padHydrationResultForRef(base, ref)
}

func (resolver *padHydrationResolver) ensureCache() {
	if resolver.cache == nil {
		resolver.cache = map[string]padHydrationResult{}
	}
}

func hydratePadsFromResolverRecord(index libraryresolver.LibraryIndex, ref string, footprintID string) padHydrationResult {
	result := padHydrationResult{Entry: PadHydrationEntry{Ref: ref, FootprintID: footprintID, Source: PadHydrationSourceMissing}}
	if footprintID == "" {
		result.Entry.MissingReason = "missing footprint id"
		result.Issues = append(result.Issues, padHydrationIssue(ref, footprintID, "footprint_id", "component has no footprint id for pad hydration"))
		return result
	}
	record, ok := libraryresolver.ResolveFootprint(index, footprintID)
	if !ok {
		result.Entry.MissingReason = "footprint not resolved"
		result.Issues = append(result.Issues, padHydrationIssue(ref, footprintID, "footprint_id", "footprint library record not found: "+footprintID))
		return result
	}
	bounds, pads, issues := placement.BoundsFromFootprint(record)
	result.Bounds = bounds
	result.Issues = append(result.Issues, contextualizePadHydrationIssues(ref, issues)...)
	for padIndex, pad := range pads {
		pad.Name = strings.TrimSpace(pad.Name)
		if pad.Name == "" {
			result.Issues = append(result.Issues, padHydrationWarning(ref, footprintID, fmt.Sprintf("pads[%d].name", padIndex), "unnamed footprint pad skipped during routing summary hydration"))
			continue
		}
		if pad.WidthMM <= 0 || pad.HeightMM <= 0 {
			result.Issues = append(result.Issues, padHydrationIssue(ref, footprintID, fmt.Sprintf("pads[%d].size", padIndex), "footprint pad size must be positive"))
			continue
		}
		result.Pads = append(result.Pads, pad)
	}
	if len(result.Pads) == 0 {
		result.Entry.MissingReason = "no routable footprint pads"
		result.Issues = append(result.Issues, padHydrationIssue(ref, footprintID, "pads", "footprint has no routable pads"))
		return result
	}
	result.Entry.Source = PadHydrationSourceResolver
	result.Entry.PadCount = len(result.Pads)
	return result
}

func hydratePadsFromVerifiedTemplate(ref string, footprintID string) padHydrationResult {
	ref = strings.TrimSpace(ref)
	footprintID = strings.TrimSpace(footprintID)
	result := padHydrationResult{Entry: PadHydrationEntry{Ref: ref, FootprintID: footprintID, Source: PadHydrationSourceMissing}}
	template, ok := verifiedPadTemplate(footprintID)
	if !ok {
		result.Entry.MissingReason = "no verified pad template"
		result.Issues = append(result.Issues, padHydrationIssue(ref, footprintID, "footprint_id", "no verified pad template for footprint: "+footprintID))
		return result
	}
	result.Bounds = template.Bounds
	result.Pads = append([]placement.PadSummary(nil), template.Pads...)
	result.Entry.Source = PadHydrationSourceVerifiedTemplate
	result.Entry.PadCount = len(result.Pads)
	return result
}

type verifiedPadTemplateRecord struct {
	Bounds placement.Bounds
	Pads   []placement.PadSummary
}

func verifiedPadTemplate(footprintID string) (verifiedPadTemplateRecord, bool) {
	footprintID = strings.TrimSpace(footprintID)
	const pinHeaderPrefix = "Connector_PinHeader_2.54mm:PinHeader_1x"
	const pinHeaderSuffix = "_P2.54mm_Vertical"
	if strings.HasPrefix(footprintID, pinHeaderPrefix) && strings.HasSuffix(footprintID, pinHeaderSuffix) {
		countText := strings.TrimSuffix(strings.TrimPrefix(footprintID, pinHeaderPrefix), pinHeaderSuffix)
		if count, err := strconv.Atoi(countText); err == nil && count > 0 {
			return pinHeaderTemplate(count), true
		}
	}
	switch footprintID {
	case "Resistor_SMD:R_0603_1608Metric":
		return twoPadTemplate(1.6, 0.8, 0.6, 0.6, 1.0), true
	case "Resistor_SMD:R_0805_2012Metric":
		return verifiedPadTemplateRecord{
			Bounds: verifiedCourtyardBounds(3.36, 1.9, 1.68, 0.95),
			Pads: []placement.PadSummary{
				smdPad("1", -0.9125, 0, 1.025, 1.4, "roundrect"),
				smdPad("2", 0.9125, 0, 1.025, 1.4, "roundrect"),
			},
		}, true
	case "Capacitor_SMD:C_0805_2012Metric",
		"LED_SMD:LED_0805_2012Metric",
		"Diode_SMD:D_SOD-323":
		return twoPadTemplate(2.0, 1.25, 0.7, 0.8, 1.2), true
	case "Resistor_SMD:R_1206_3216Metric":
		return twoPadTemplate(3.2, 1.6, 1.2, 1.2, 2.4), true
	case "Resistor_SMD:R_2512_6332Metric":
		return verifiedPadTemplateRecord{
			Bounds: verifiedCourtyardBoundsFromExtents(-3.83, -1.93, 3.83, 1.93),
			Pads: []placement.PadSummary{
				smdPad("1", -2.9625, 0, 1.225, 3.35, "roundrect"),
				smdPad("2", 2.9625, 0, 1.225, 3.35, "roundrect"),
			},
		}, true
	case "Capacitor_SMD:C_1210_3225Metric":
		return twoPadTemplate(3.2, 2.5, 1.2, 2.5, 2.4), true
	case "Capacitor_Tantalum_SMD:CP_EIA-3216-18_Kemet-A":
		pads := []placement.PadSummary{
			{Name: "1", XMM: -1.3525, WidthMM: 1.415, HeightMM: 1.39, Type: "smd", Shape: "roundrect", Layers: []string{"F.Cu", "F.Mask", "F.Paste"}},
			{Name: "2", XMM: 1.3525, WidthMM: 1.415, HeightMM: 1.39, Type: "smd", Shape: "roundrect", Layers: []string{"F.Cu", "F.Mask", "F.Paste"}},
		}
		return verifiedPadTemplateRecord{Bounds: verifiedCourtyardBounds(4.62, 2.1, 2.31, 1.05), Pads: pads}, true
	case "Capacitor_SMD:C_0603_1608Metric":
		return twoPadTemplate(1.6, 0.8, 0.6, 0.6, 1.0), true
	case "Capacitor_THT:C_Rect_L7.2mm_W3.0mm_P5.00mm_FKS2_FKP2_MKS2_MKP2":
		template := throughHoleRowTemplate([]float64{0, 5}, []string{"1", "2"}, 1.6, 1.6, 0.8, []string{"circle", "circle"}, 7.2, 3.0)
		template.Bounds = verifiedCourtyardBoundsFromExtents(-1.35, -1.75, 6.35, 1.75)
		return template, true
	case "Capacitor_THT:CP_Radial_D8.0mm_P3.50mm":
		template := throughHoleRowTemplate([]float64{0, 3.5}, []string{"1", "2"}, 1.6, 1.6, 0.8, []string{"roundrect", "circle"}, 8.0, 8.0)
		template.Bounds = verifiedCourtyardBounds(8.5, 8.5, 2.5, 4.25)
		return template, true
	case "Diode_SMD:D_SOD-123":
		return verifiedPadTemplateRecord{
			Bounds: verifiedCourtyardBounds(4.7, 2.3, 2.35, 1.15),
			Pads: []placement.PadSummary{
				smdPad("1", -1.65, 0, 0.9, 1.2, "roundrect"),
				smdPad("2", 1.65, 0, 0.9, 1.2, "roundrect"),
			},
		}, true
	case "Diode_SMD:D_SMA":
		return twoPadTemplate(6.2, 3.0, 1.5, 1.7, 4.4), true
	case "Diode_SMD:D_SMC":
		return verifiedPadTemplateRecord{
			Bounds: verifiedCourtyardBoundsFromExtents(-4.9, -3.35, 4.9, 3.35),
			Pads: []placement.PadSummary{
				smdPad("1", -3.4, 0, 3.3, 2.5, "roundrect"),
				smdPad("2", 3.4, 0, 3.3, 2.5, "roundrect"),
			},
		}, true
	case "Fuse:Fuse_1206_3216Metric":
		return twoPadTemplate(4.5, 2.6, 1.6, 1.6, 2.8), true
	case "Resistor_THT:R_Axial_DIN0414_L11.9mm_D4.5mm_P20.32mm_Horizontal":
		template := throughHoleRowTemplate([]float64{0, 20.32}, []string{"1", "2"}, 2.4, 2.4, 1.2, []string{"circle", "circle"}, 23.0, 4.5)
		template.Bounds = verifiedCourtyardBounds(23.22, 5, 1.45, 2.5)
		return template, true
	case "Package_TO_SOT_THT:TO-126-3_Vertical":
		template := throughHoleRowTemplate([]float64{0, 2.28, 4.56}, []string{"1", "2", "3"}, 1.71, 1.8, 1.0, []string{"rect", "oval", "oval"}, 8.0, 2.5)
		template.Bounds = verifiedCourtyardBounds(8.5, 3.75, 1.97, 2.25)
		return template, true
	case "Package_TO_SOT_THT:TO-3P-3_Vertical":
		template := throughHoleRowTemplate([]float64{0, 5.45, 10.9}, []string{"1", "2", "3"}, 2.5, 4.5, 1.5, []string{"rect", "oval", "oval"}, 16.0, 5.0)
		template.Bounds = verifiedCourtyardBounds(16, 5.75, 2.55, 3.25)
		return template, true
	case "Relay_THT:Relay_SPST_Omron-G5Q-1A":
		pads := []placement.PadSummary{
			throughHolePad("1", 0, 0, 2.3, 2.3, 1.3, "rect"),
			throughHolePad("2", 10.16, 0, 2.3, 2.3, 1.3, "circle"),
			throughHolePad("3", 17.78, 0, 2.3, 2.3, 1.3, "circle"),
			throughHolePad("5", 0, -7.62, 2.3, 2.3, 1.3, "circle"),
		}
		for index := range pads {
			pads[index].RotationDeg = 180
		}
		return verifiedPadTemplateRecord{Bounds: verifiedCourtyardBounds(21.65, 11.5, 1.95, 9.55), Pads: pads}, true
	case "Converter_DCDC:Converter_DCDC_Murata_MEE1SxxxxSC_THT":
		pads := make([]placement.PadSummary, 0, 4)
		for index, name := range []string{"1", "2", "3", "4"} {
			shape := "oval"
			if index == 0 {
				shape = "rect"
			}
			pad := throughHolePad(name, 0, float64(index)*2.54, 1.75, 2.25, 1.075, shape)
			pad.RotationDeg = 270
			pads = append(pads, pad)
		}
		return verifiedPadTemplateRecord{Bounds: verifiedCourtyardBoundsFromExtents(-1.38, -2.33, 5.22, 9.7), Pads: pads}, true
	case "Converter_DCDC:Converter_DCDC_TRACO_TEL12-xxxx_THT":
		pads := []placement.PadSummary{
			throughHolePad("1", 0, 0, 2, 1.5, 0.7, "rect"),
			throughHolePad("8", 0, 17.78, 2, 1.5, 0.7, "oval"),
			throughHolePad("9", 10.16, 17.78, 2, 1.5, 0.7, "oval"),
			throughHolePad("10", 10.16, 15.24, 2, 1.5, 0.7, "oval"),
			throughHolePad("16", 10.16, 0, 2, 1.5, 0.7, "oval"),
		}
		return verifiedPadTemplateRecord{Bounds: verifiedCourtyardBoundsFromExtents(-2.02, -3.25, 12.18, 21.05), Pads: pads}, true
	case "Connector_USB:USB_C_Receptacle_GCT_USB4125-xx-x_6P_TopMnt_Horizontal":
		return usbCGCTUSB4125PowerOnlyTemplate(), true
	case "Connector_USB:USB_C_Receptacle_HRO_TYPE-C-31-M-12":
		return usbCHROTypeC31M12Template(), true
	case "RF_Module:ESP32-WROOM-32E":
		return esp32WROOM32ETemplate(), true
	case "Button_Switch_SMD:SW_SPST_SKQG_WithStem":
		return verifiedPadTemplateRecord{
			Bounds: centeredEstimatedBounds(8.0, 6.2),
			Pads: []placement.PadSummary{
				{Name: "1", XMM: -3.1, YMM: -1.85, WidthMM: 1.8, HeightMM: 1.1},
				{Name: "1", XMM: 3.1, YMM: -1.85, WidthMM: 1.8, HeightMM: 1.1},
				{Name: "2", XMM: -3.1, YMM: 1.85, WidthMM: 1.8, HeightMM: 1.1},
				{Name: "2", XMM: 3.1, YMM: 1.85, WidthMM: 1.8, HeightMM: 1.1},
			},
		}, true
	case "Button_Switch_SMD:SW_SPST_B3U-1000P":
		return verifiedPadTemplateRecord{
			Bounds: centeredEstimatedBounds(4.3, 3.2),
			Pads: []placement.PadSummary{
				{Name: "1", XMM: -1.7, WidthMM: 0.9, HeightMM: 1.7},
				{Name: "2", XMM: 1.7, WidthMM: 0.9, HeightMM: 1.7},
			},
		}, true
	case "Package_TO_SOT_SMD:SOT-23-5":
		return verifiedPadTemplateRecord{
			Bounds: verifiedCourtyardBounds(4.1, 3.4, 2.05, 1.7),
			Pads: []placement.PadSummary{
				smdPad("1", -1.1375, -0.95, 1.325, 0.6, "roundrect"),
				smdPad("2", -1.1375, 0, 1.325, 0.6, "roundrect"),
				smdPad("3", -1.1375, 0.95, 1.325, 0.6, "roundrect"),
				smdPad("4", 1.1375, 0.95, 1.325, 0.6, "roundrect"),
				smdPad("5", 1.1375, -0.95, 1.325, 0.6, "roundrect"),
			},
		}, true
	case "Package_TO_SOT_SMD:SOT-23-6":
		return verifiedPadTemplateRecord{
			Bounds: verifiedCourtyardBoundsFromExtents(-2.05, -1.7, 2.05, 1.7),
			Pads: []placement.PadSummary{
				smdPad("1", -1.1375, -0.95, 1.325, 0.6, "roundrect"),
				smdPad("2", -1.1375, 0, 1.325, 0.6, "roundrect"),
				smdPad("3", -1.1375, 0.95, 1.325, 0.6, "roundrect"),
				smdPad("4", 1.1375, 0.95, 1.325, 0.6, "roundrect"),
				smdPad("5", 1.1375, 0, 1.325, 0.6, "roundrect"),
				smdPad("6", 1.1375, -0.95, 1.325, 0.6, "roundrect"),
			},
		}, true
	case "Package_TO_SOT_SMD:SOT-23":
		return verifiedPadTemplateRecord{
			Bounds: verifiedCourtyardBounds(3.86, 3.4, 1.93, 1.7),
			Pads: []placement.PadSummary{
				smdPad("1", -0.9375, -0.95, 1.475, 0.6, "roundrect"),
				smdPad("2", -0.9375, 0.95, 1.475, 0.6, "roundrect"),
				smdPad("3", 0.9375, 0, 1.475, 0.6, "roundrect"),
			},
		}, true
	case "Package_TO_SOT_SMD:SOT-223-3_TabPin2":
		pads := []placement.PadSummary{
			{Name: "1", XMM: -2.3, YMM: 2.4, WidthMM: 1.2, HeightMM: 1.5},
			{Name: "2", XMM: 0, YMM: 2.4, WidthMM: 1.2, HeightMM: 1.5},
			{Name: "3", XMM: 2.3, YMM: 2.4, WidthMM: 1.2, HeightMM: 1.5},
			{Name: "2", XMM: 0, YMM: -2.1, WidthMM: 3.8, HeightMM: 2.4},
		}
		return verifiedPadTemplateRecord{Bounds: padEnvelopeBounds(pads, 6.7, 7.0), Pads: pads}, true
	case "Package_TO_SOT_THT:TO-220-3_Vertical":
		return verifiedPadTemplateRecord{
			Bounds: verifiedCourtyardBoundsFromExtents(-2.71, -3.4, 7.79, 1.5),
			Pads: []placement.PadSummary{
				throughHolePad("1", 0, 0, 1.905, 2, 1.1, "rect"),
				throughHolePad("2", 2.54, 0, 1.905, 2, 1.1, "oval"),
				throughHolePad("3", 5.08, 0, 1.905, 2, 1.1, "oval"),
			},
		}, true
	case "Package_SO:SOIC-8_3.9x4.9mm_P1.27mm":
		pads := []placement.PadSummary{
			smdPad("1", -2.475, -1.905, 1.95, 0.6, "roundrect"),
			smdPad("2", -2.475, -0.635, 1.95, 0.6, "roundrect"),
			smdPad("3", -2.475, 0.635, 1.95, 0.6, "roundrect"),
			smdPad("4", -2.475, 1.905, 1.95, 0.6, "roundrect"),
			smdPad("5", 2.475, 1.905, 1.95, 0.6, "roundrect"),
			smdPad("6", 2.475, 0.635, 1.95, 0.6, "roundrect"),
			smdPad("7", 2.475, -0.635, 1.95, 0.6, "roundrect"),
			smdPad("8", 2.475, -1.905, 1.95, 0.6, "roundrect"),
		}
		return verifiedPadTemplateRecord{Bounds: verifiedCourtyardBounds(7.4, 5.4, 3.7, 2.7), Pads: pads}, true
	case "Package_SO:VSSOP-8_2.3x2mm_P0.5mm":
		pads := []placement.PadSummary{
			smdPad("1", -1.4, -0.75, 1.25, 0.35, "roundrect"),
			smdPad("2", -1.4, -0.25, 1.25, 0.35, "roundrect"),
			smdPad("3", -1.4, 0.25, 1.25, 0.35, "roundrect"),
			smdPad("4", -1.4, 0.75, 1.25, 0.35, "roundrect"),
			smdPad("5", 1.4, 0.75, 1.25, 0.35, "roundrect"),
			smdPad("6", 1.4, 0.25, 1.25, 0.35, "roundrect"),
			smdPad("7", 1.4, -0.25, 1.25, 0.35, "roundrect"),
			smdPad("8", 1.4, -0.75, 1.25, 0.35, "roundrect"),
		}
		return verifiedPadTemplateRecord{Bounds: verifiedCourtyardBoundsFromExtents(-2.28, -1.25, 2.28, 1.25), Pads: pads}, true
	case "Package_QFP:TQFP-32_7x7mm_P0.8mm":
		return tqfp32Template(), true
	case "Package_LGA:Bosch_LGA-8_2x2.5mm_P0.65mm_ClockwisePinNumbering":
		pads := []placement.PadSummary{
			{Name: "1", XMM: -0.975, YMM: -0.8, WidthMM: 0.35, HeightMM: 0.5},
			{Name: "2", XMM: -0.325, YMM: -0.8, WidthMM: 0.35, HeightMM: 0.5},
			{Name: "3", XMM: 0.325, YMM: -0.8, WidthMM: 0.35, HeightMM: 0.5},
			{Name: "4", XMM: 0.975, YMM: -0.8, WidthMM: 0.35, HeightMM: 0.5},
			{Name: "5", XMM: 0.975, YMM: 0.8, WidthMM: 0.35, HeightMM: 0.5},
			{Name: "6", XMM: 0.325, YMM: 0.8, WidthMM: 0.35, HeightMM: 0.5},
			{Name: "7", XMM: -0.325, YMM: 0.8, WidthMM: 0.35, HeightMM: 0.5},
			{Name: "8", XMM: -0.975, YMM: 0.8, WidthMM: 0.35, HeightMM: 0.5},
		}
		return verifiedPadTemplateRecord{Bounds: padEnvelopeBounds(pads, 2.0, 2.5), Pads: pads}, true
	case "Package_LGA:Bosch_LGA-8_2.5x2.5mm_P0.65mm_ClockwisePinNumbering":
		return boschLGA8_2_5Template(), true
	case "Sensor_Humidity:Sensirion_DFN-8-1EP_2.5x2.5mm_P0.5mm_EP1.1x1.7mm":
		return sensirionDFN8Template(), true
	case "TestPoint:TestPoint_Pad_D1.0mm":
		return verifiedPadTemplateRecord{
			Bounds: centeredEstimatedBounds(1.6, 1.6),
			Pads: []placement.PadSummary{
				{Name: "1", XMM: 0, YMM: 0, WidthMM: 1.0, HeightMM: 1.0},
			},
		}, true
	default:
		return verifiedPadTemplateRecord{}, false
	}
}

func throughHoleRowTemplate(xPositions []float64, names []string, widthMM, heightMM, drillMM float64, shapes []string, bodyWidthMM, bodyHeightMM float64) verifiedPadTemplateRecord {
	count := min(len(xPositions), len(names), len(shapes))
	pads := make([]placement.PadSummary, 0, count)
	for index := 0; index < count; index++ {
		pads = append(pads, throughHolePad(names[index], xPositions[index], 0, widthMM, heightMM, drillMM, shapes[index]))
	}
	return verifiedPadTemplateRecord{Bounds: padEnvelopeBounds(pads, bodyWidthMM, bodyHeightMM), Pads: pads}
}

func throughHolePad(name string, xMM, yMM, widthMM, heightMM, drillMM float64, shape string) placement.PadSummary {
	return placement.PadSummary{
		Name: name, XMM: xMM, YMM: yMM, WidthMM: widthMM, HeightMM: heightMM,
		Type: "thru_hole", Shape: shape, DrillMM: drillMM, Layers: []string{"*.Cu", "*.Mask"},
	}
}

func smdPad(name string, xMM, yMM, widthMM, heightMM float64, shape string) placement.PadSummary {
	return placement.PadSummary{
		Name: name, XMM: xMM, YMM: yMM, WidthMM: widthMM, HeightMM: heightMM,
		Type: "smd", Shape: shape, Layers: []string{"F.Cu", "F.Mask", "F.Paste"},
	}
}

func verifiedCourtyardBounds(widthMM, heightMM, anchorXMM, anchorYMM float64) placement.Bounds {
	return placement.Bounds{
		WidthMM: widthMM, HeightMM: heightMM,
		AnchorOffset: placement.Point{XMM: anchorXMM, YMM: anchorYMM},
		Source:       placement.BoundsLibraryCourtyard,
	}
}

func verifiedCourtyardBoundsFromExtents(minXMM, minYMM, maxXMM, maxYMM float64) placement.Bounds {
	return verifiedCourtyardBounds(maxXMM-minXMM, maxYMM-minYMM, -minXMM, -minYMM)
}

func esp32WROOM32ETemplate() verifiedPadTemplateRecord {
	pads := make([]placement.PadSummary, 0, 39)
	for number := 1; number <= 14; number++ {
		pads = append(pads, placement.PadSummary{Name: strconv.Itoa(number), XMM: -8.75, YMM: -5.26 + float64(number-1)*1.27, WidthMM: 1.5, HeightMM: 0.9})
	}
	for number := 15; number <= 24; number++ {
		pads = append(pads, placement.PadSummary{Name: strconv.Itoa(number), XMM: -5.715 + float64(number-15)*1.27, YMM: 12.5, WidthMM: 0.9, HeightMM: 1.5})
	}
	for number := 25; number <= 38; number++ {
		pads = append(pads, placement.PadSummary{Name: strconv.Itoa(number), XMM: 8.75, YMM: 11.25 - float64(number-25)*1.27, WidthMM: 1.5, HeightMM: 0.9})
	}
	pads = append(pads, placement.PadSummary{Name: "39", XMM: -0.1, YMM: 2.46, WidthMM: 5.8, HeightMM: 5.8})
	return verifiedPadTemplateRecord{Bounds: centeredEstimatedBounds(18.0, 25.5), Pads: pads}
}

func tqfp32Template() verifiedPadTemplateRecord {
	pads := make([]placement.PadSummary, 0, 32)
	for number := 1; number <= 8; number++ {
		pads = append(pads, placement.PadSummary{Name: strconv.Itoa(number), XMM: -4.1625, YMM: -2.8 + float64(number-1)*0.8, WidthMM: 1.475, HeightMM: 0.55})
	}
	for number := 9; number <= 16; number++ {
		pads = append(pads, placement.PadSummary{Name: strconv.Itoa(number), XMM: -2.8 + float64(number-9)*0.8, YMM: 4.1625, WidthMM: 0.55, HeightMM: 1.475})
	}
	for number := 17; number <= 24; number++ {
		pads = append(pads, placement.PadSummary{Name: strconv.Itoa(number), XMM: 4.1625, YMM: 2.8 - float64(number-17)*0.8, WidthMM: 1.475, HeightMM: 0.55})
	}
	for number := 25; number <= 32; number++ {
		pads = append(pads, placement.PadSummary{Name: strconv.Itoa(number), XMM: 2.8 - float64(number-25)*0.8, YMM: -4.1625, WidthMM: 0.55, HeightMM: 1.475})
	}
	return verifiedPadTemplateRecord{Bounds: padEnvelopeBounds(pads, 10.3, 10.3), Pads: pads}
}

func boschLGA8_2_5Template() verifiedPadTemplateRecord {
	pads := []placement.PadSummary{
		{Name: "1", XMM: -0.975, YMM: -1.025, WidthMM: 0.35, HeightMM: 0.5},
		{Name: "2", XMM: -0.325, YMM: -1.025, WidthMM: 0.35, HeightMM: 0.5},
		{Name: "3", XMM: 0.325, YMM: -1.025, WidthMM: 0.35, HeightMM: 0.5},
		{Name: "4", XMM: 0.975, YMM: -1.025, WidthMM: 0.35, HeightMM: 0.5},
		{Name: "5", XMM: 0.975, YMM: 1.025, WidthMM: 0.35, HeightMM: 0.5},
		{Name: "6", XMM: 0.325, YMM: 1.025, WidthMM: 0.35, HeightMM: 0.5},
		{Name: "7", XMM: -0.325, YMM: 1.025, WidthMM: 0.35, HeightMM: 0.5},
		{Name: "8", XMM: -0.975, YMM: 1.025, WidthMM: 0.35, HeightMM: 0.5},
	}
	return verifiedPadTemplateRecord{Bounds: padEnvelopeBounds(pads, 2.5, 2.5), Pads: pads}
}

func sensirionDFN8Template() verifiedPadTemplateRecord {
	pads := []placement.PadSummary{
		{Name: "1", XMM: -1.175, YMM: -0.75, WidthMM: 0.55, HeightMM: 0.25},
		{Name: "2", XMM: -1.175, YMM: -0.25, WidthMM: 0.55, HeightMM: 0.25},
		{Name: "3", XMM: -1.175, YMM: 0.25, WidthMM: 0.55, HeightMM: 0.25},
		{Name: "4", XMM: -1.175, YMM: 0.75, WidthMM: 0.55, HeightMM: 0.25},
		{Name: "5", XMM: 1.175, YMM: 0.75, WidthMM: 0.55, HeightMM: 0.25},
		{Name: "6", XMM: 1.175, YMM: 0.25, WidthMM: 0.55, HeightMM: 0.25},
		{Name: "7", XMM: 1.175, YMM: -0.25, WidthMM: 0.55, HeightMM: 0.25},
		{Name: "8", XMM: 1.175, YMM: -0.75, WidthMM: 0.55, HeightMM: 0.25},
		{Name: "9", WidthMM: 1.0, HeightMM: 1.7},
	}
	return verifiedPadTemplateRecord{Bounds: padEnvelopeBounds(pads, 2.5, 2.5), Pads: pads}
}

func usbCHROTypeC31M12Template() verifiedPadTemplateRecord {
	pads := []placement.PadSummary{
		{Name: "A1", XMM: -3.25, YMM: -4.045, WidthMM: 0.6, HeightMM: 1.45},
		{Name: "A4", XMM: -2.45, YMM: -4.045, WidthMM: 0.6, HeightMM: 1.45},
		{Name: "A5", XMM: -1.25, YMM: -4.045, WidthMM: 0.3, HeightMM: 1.45},
		{Name: "A6", XMM: -0.25, YMM: -4.045, WidthMM: 0.3, HeightMM: 1.45},
		{Name: "A7", XMM: 0.25, YMM: -4.045, WidthMM: 0.3, HeightMM: 1.45},
		{Name: "A8", XMM: 1.25, YMM: -4.045, WidthMM: 0.3, HeightMM: 1.45},
		{Name: "A9", XMM: 2.45, YMM: -4.045, WidthMM: 0.6, HeightMM: 1.45},
		{Name: "A12", XMM: 3.25, YMM: -4.045, WidthMM: 0.6, HeightMM: 1.45},
		{Name: "B1", XMM: 3.25, YMM: -4.045, WidthMM: 0.6, HeightMM: 1.45},
		{Name: "B4", XMM: 2.45, YMM: -4.045, WidthMM: 0.6, HeightMM: 1.45},
		{Name: "B5", XMM: 1.75, YMM: -4.045, WidthMM: 0.3, HeightMM: 1.45},
		{Name: "B6", XMM: 0.75, YMM: -4.045, WidthMM: 0.3, HeightMM: 1.45},
		{Name: "B7", XMM: -0.75, YMM: -4.045, WidthMM: 0.3, HeightMM: 1.45},
		{Name: "B8", XMM: -1.75, YMM: -4.045, WidthMM: 0.3, HeightMM: 1.45},
		{Name: "B9", XMM: -2.45, YMM: -4.045, WidthMM: 0.6, HeightMM: 1.45},
		{Name: "B12", XMM: -3.25, YMM: -4.045, WidthMM: 0.6, HeightMM: 1.45},
		{Name: "SH", XMM: -4.32, YMM: -3.13, WidthMM: 1.0, HeightMM: 2.1},
		{Name: "SH", XMM: -4.32, YMM: 1.05, WidthMM: 1.0, HeightMM: 1.6},
		{Name: "SH", XMM: 4.32, YMM: -3.13, WidthMM: 1.0, HeightMM: 2.1},
		{Name: "SH", XMM: 4.32, YMM: 1.05, WidthMM: 1.0, HeightMM: 1.6},
	}
	return verifiedPadTemplateRecord{Bounds: padEnvelopeBounds(pads, 10.0, 7.5), Pads: pads}
}

func usbCGCTUSB4125PowerOnlyTemplate() verifiedPadTemplateRecord {
	pads := []placement.PadSummary{
		{Name: "A5", XMM: -0.5, YMM: -3.08, WidthMM: 0.7, HeightMM: 1.2},
		{Name: "A9", XMM: 1.52, YMM: -3.08, WidthMM: 0.76, HeightMM: 1.2},
		{Name: "A12", XMM: 2.75, YMM: -3.08, WidthMM: 0.8, HeightMM: 1.2},
		{Name: "B5", XMM: 0.5, YMM: -3.08, WidthMM: 0.7, HeightMM: 1.2},
		{Name: "B9", XMM: -1.52, YMM: -3.08, WidthMM: 0.76, HeightMM: 1.2},
		{Name: "B12", XMM: -2.75, YMM: -3.08, WidthMM: 0.8, HeightMM: 1.2},
		{Name: "SH", XMM: -4.32, YMM: -3.0, WidthMM: 1.1, HeightMM: 1.7},
		{Name: "SH", XMM: -4.32, YMM: 0.8, WidthMM: 1.1, HeightMM: 1.7},
		{Name: "SH", XMM: 4.32, YMM: -3.0, WidthMM: 1.1, HeightMM: 1.7},
		{Name: "SH", XMM: 4.32, YMM: 0.8, WidthMM: 1.1, HeightMM: 1.7},
	}
	return verifiedPadTemplateRecord{Bounds: padEnvelopeBounds(pads, 10.0, 6.9), Pads: pads}
}

func twoPadTemplate(envelopeWidth, envelopeHeight, padWidth, padHeight, pitch float64) verifiedPadTemplateRecord {
	return verifiedPadTemplateRecord{
		Bounds: centeredEstimatedBounds(envelopeWidth, envelopeHeight),
		Pads: []placement.PadSummary{
			{Name: "1", XMM: -pitch / 2, YMM: 0, WidthMM: padWidth, HeightMM: padHeight},
			{Name: "2", XMM: pitch / 2, YMM: 0, WidthMM: padWidth, HeightMM: padHeight},
		},
	}
}

func rowPadTemplate(centerSpanWidth, bodyHeight, pitch, padWidth, padHeight float64, leftNames []string, rightNames []string) verifiedPadTemplateRecord {
	pads := make([]placement.PadSummary, 0, len(leftNames)+len(rightNames))
	rows := max(len(leftNames), len(rightNames))
	for index, name := range leftNames {
		pads = append(pads, placement.PadSummary{Name: name, XMM: -centerSpanWidth / 2, YMM: rowPadY(index, rows, pitch), WidthMM: padWidth, HeightMM: padHeight})
	}
	for index, name := range rightNames {
		pads = append(pads, placement.PadSummary{Name: name, XMM: centerSpanWidth / 2, YMM: rowPadY(index, rows, pitch), WidthMM: padWidth, HeightMM: padHeight})
	}
	return verifiedPadTemplateRecord{Bounds: centeredEstimatedBounds(centerSpanWidth+padWidth, bodyHeight+padHeight), Pads: pads}
}

func rowPadY(index int, count int, pitch float64) float64 {
	if count <= 1 {
		return 0
	}
	return (float64(index) - float64(count-1)/2) * pitch
}

func pinHeaderTemplate(count int) verifiedPadTemplateRecord {
	pads := make([]placement.PadSummary, 0, count)
	for index := 0; index < count; index++ {
		shape := "circle"
		if index == 0 {
			shape = "rect"
		}
		pads = append(pads, placement.PadSummary{
			Name:     fmt.Sprintf("%d", index+1),
			XMM:      0,
			YMM:      float64(index) * 2.54,
			WidthMM:  1.7,
			HeightMM: 1.7,
			Type:     "thru_hole",
			Shape:    shape,
			DrillMM:  1.0,
			Layers:   []string{"*.Cu", "*.Mask"},
		})
	}
	maxYMM := float64(count-1)*2.54 + 1.77
	// KiCad's two-position member of this generated footprint family snaps its
	// positive courtyard edge outward by one hundredth of a millimeter.
	if count == 2 {
		maxYMM = 4.32
	}
	return verifiedPadTemplateRecord{Bounds: verifiedCourtyardBoundsFromExtents(-1.77, -1.77, 1.77, maxYMM), Pads: pads}
}

// centeredEstimatedBounds matches verified fallback templates whose pads are centered around the footprint origin.
func centeredEstimatedBounds(width, height float64) placement.Bounds {
	return placement.Bounds{
		WidthMM:      width,
		HeightMM:     height,
		AnchorOffset: placement.Point{XMM: width / 2, YMM: height / 2},
		Source:       placement.BoundsEstimated,
	}
}

func padEnvelopeBounds(pads []placement.PadSummary, minWidth, minHeight float64) placement.Bounds {
	if len(pads) == 0 {
		return centeredEstimatedBounds(minWidth, minHeight)
	}
	minX := pads[0].XMM - pads[0].WidthMM/2
	maxX := pads[0].XMM + pads[0].WidthMM/2
	minY := pads[0].YMM - pads[0].HeightMM/2
	maxY := pads[0].YMM + pads[0].HeightMM/2
	for _, pad := range pads[1:] {
		minX = min(minX, pad.XMM-pad.WidthMM/2)
		maxX = max(maxX, pad.XMM+pad.WidthMM/2)
		minY = min(minY, pad.YMM-pad.HeightMM/2)
		maxY = max(maxY, pad.YMM+pad.HeightMM/2)
	}
	width := max(minWidth, maxX-minX)
	height := max(minHeight, maxY-minY)
	left := minX - (width-(maxX-minX))/2
	bottom := minY - (height-(maxY-minY))/2
	return placement.Bounds{
		WidthMM:      width,
		HeightMM:     height,
		AnchorOffset: placement.Point{XMM: -left, YMM: -bottom},
		Source:       placement.BoundsEstimated,
	}
}

type padNetAssignment struct {
	NetName string
	Pin     string
}

type padNetAssignmentIndex map[string][]padNetAssignment

func buildPadNetAssignmentIndex(nets []placement.Net) padNetAssignmentIndex {
	index := padNetAssignmentIndex{}
	for _, net := range nets {
		netName := strings.TrimSpace(net.Name)
		if netName == "" {
			continue
		}
		for _, endpoint := range net.Endpoints {
			ref := strings.ToUpper(strings.TrimSpace(endpoint.Ref))
			if ref == "" {
				continue
			}
			index[ref] = append(index[ref], padNetAssignment{NetName: netName, Pin: strings.TrimSpace(endpoint.Pin)})
		}
	}
	return index
}

func assignPadNetsFromIndex(ref string, pads []placement.PadSummary, assignments padNetAssignmentIndex) ([]placement.PadSummary, []reports.Issue) {
	ref = strings.TrimSpace(ref)
	out := append([]placement.PadSummary(nil), pads...)
	padByName := map[string][]int{}
	for index, pad := range out {
		name := strings.TrimSpace(pad.Name)
		out[index].Name = name
		if name != "" {
			normalizedName := strings.ToUpper(name)
			padByName[normalizedName] = append(padByName[normalizedName], index)
		}
	}
	var issues []reports.Issue
	for _, assignment := range assignments[strings.ToUpper(ref)] {
		pin := strings.TrimSpace(assignment.Pin)
		if pin == "" {
			issues = append(issues, padHydrationIssue(ref, "", "nets."+assignment.NetName, "net endpoint pin is required for pad assignment"))
			continue
		}
		padIndexes := matchingPadIndexesForPin(padByName, pin)
		if len(padIndexes) == 0 {
			issues = append(issues, padHydrationIssue(ref, "", "nets."+assignment.NetName+"."+pin, "net endpoint pin has no matching footprint pad"))
			continue
		}
		for _, padIndex := range padIndexes {
			if out[padIndex].Net != "" && out[padIndex].Net != assignment.NetName {
				issues = append(issues, padHydrationIssue(ref, "", "pads."+pin+".net", fmt.Sprintf("footprint pad maps to multiple generated nets: %s and %s", out[padIndex].Net, assignment.NetName)))
				continue
			}
			out[padIndex].Net = assignment.NetName
		}
	}
	return out, issues
}

func matchingPadIndexesForPin(padByName map[string][]int, pin string) []int {
	normalizedPin := strings.ToUpper(strings.TrimSpace(pin))
	members := groupedPinMembers(normalizedPin)
	if len(members) > 1 {
		matches := []int{}
		for _, member := range members {
			matches = append(matches, padByName[member]...)
		}
		sort.Ints(matches)
		return matches
	}
	if normalizedPin != "SH" && normalizedPin != "SHIELD" {
		return padByName[normalizedPin]
	}
	matches := []int{}
	for padName, padIndexes := range padByName {
		if isUSBShieldPadName(padName) {
			matches = append(matches, padIndexes...)
		}
	}
	sort.Ints(matches)
	return matches
}

func groupedPinMembers(pin string) []string {
	return libraryresolver.GroupedPinMembers(strings.ToUpper(strings.TrimSpace(pin)))
}

func isUSBShieldPadName(name string) bool {
	if name == "SH" || name == "SHIELD" {
		return true
	}
	prefix := "SH"
	if strings.HasPrefix(name, "SHIELD") {
		prefix = "SHIELD"
	} else if !strings.HasPrefix(name, "SH") {
		return false
	}
	for _, char := range name[len(prefix):] {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}

func padHydrationResultForRef(base padHydrationResult, ref string) padHydrationResult {
	result := padHydrationResult{
		Bounds: base.Bounds,
		Pads:   append([]placement.PadSummary(nil), base.Pads...),
		Entry:  base.Entry,
		Issues: append([]reports.Issue(nil), base.Issues...),
	}
	result.Entry.Ref = ref
	for index := range result.Issues {
		result.Issues[index] = contextualizePadHydrationIssue(ref, result.Issues[index])
	}
	return result
}

func padHydrationIssue(ref string, footprintID string, path string, message string) reports.Issue {
	return newPadHydrationIssue(ref, footprintID, path, message, reports.SeverityBlocked)
}

func padHydrationWarning(ref string, footprintID string, path string, message string) reports.Issue {
	return newPadHydrationIssue(ref, footprintID, path, message, reports.SeverityWarning)
}

func newPadHydrationIssue(ref string, footprintID string, path string, message string, severity reports.Severity) reports.Issue {
	issuePath := "pad_hydration"
	if ref != "" {
		issuePath += "." + ref
	}
	if path != "" {
		issuePath += "." + path
	}
	issue := reports.Issue{
		Code:     reports.CodeInvalidArgument,
		Severity: severity,
		Path:     issuePath,
		Message:  message,
	}
	if ref != "" {
		issue.Refs = []string{ref}
	}
	if footprintID != "" {
		issue.Suggestion = "resolve footprint pad metadata for " + footprintID
	}
	return issue
}

func contextualizePadHydrationIssues(ref string, issues []reports.Issue) []reports.Issue {
	if ref == "" || len(issues) == 0 {
		return issues
	}
	out := make([]reports.Issue, 0, len(issues))
	for _, issue := range issues {
		out = append(out, contextualizePadHydrationIssue(ref, issue))
	}
	return out
}

func contextualizePadHydrationIssue(ref string, issue reports.Issue) reports.Issue {
	if ref == "" {
		return issue
	}
	if issue.Path != "" {
		if !strings.HasPrefix(issue.Path, "pad_hydration.") {
			issue.Path = "pad_hydration." + ref + "." + issue.Path
		} else if !strings.HasPrefix(issue.Path, "pad_hydration."+ref+".") {
			issue.Path = "pad_hydration." + ref + strings.TrimPrefix(issue.Path, "pad_hydration")
		}
	} else {
		issue.Path = "pad_hydration." + ref
	}
	if len(issue.Refs) == 0 {
		issue.Refs = []string{ref}
	}
	return issue
}

func summarizePadHydration(entries []PadHydrationEntry, issues []reports.Issue) PadHydrationSummary {
	summary := PadHydrationSummary{
		ComponentCount: len(entries),
		SourceCounts:   map[PadHydrationSource]int{},
	}
	missing := map[string]struct{}{}
	for _, entry := range entries {
		source := normalizePadHydrationSource(entry.Source)
		summary.SourceCounts[source]++
		if entry.PadCount > 0 && source != PadHydrationSourceMissing {
			summary.HydratedComponents++
			summary.PadCount += entry.PadCount
			continue
		}
		summary.MissingComponents++
		ref := strings.TrimSpace(entry.Ref)
		if ref != "" {
			missing[ref] = struct{}{}
		}
	}
	for _, issue := range issues {
		if issue.Blocking() {
			summary.BlockingIssues++
		}
	}
	summary.MissingRefs = sortedKeys(missing)
	if len(summary.SourceCounts) == 0 {
		summary.SourceCounts = nil
	}
	return summary
}

func normalizePadHydrationSource(source PadHydrationSource) PadHydrationSource {
	if source == "" {
		return PadHydrationSourceMissing
	}
	return source
}

func sortedKeys(values map[string]struct{}) []string {
	if len(values) == 0 {
		return nil
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
