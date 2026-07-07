package designworkflow

import (
	"fmt"
	"sort"
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
	switch strings.TrimSpace(footprintID) {
	case "Resistor_SMD:R_0603_1608Metric":
		return twoPadTemplate(1.6, 0.8, 0.6, 0.6, 1.0), true
	case "Resistor_SMD:R_0805_2012Metric",
		"Capacitor_SMD:C_0805_2012Metric",
		"LED_SMD:LED_0805_2012Metric",
		"Diode_SMD:D_SOD-323":
		return twoPadTemplate(2.0, 1.25, 0.7, 0.8, 1.2), true
	case "Resistor_SMD:R_1206_3216Metric":
		return twoPadTemplate(3.2, 1.6, 1.2, 1.2, 2.4), true
	case "Capacitor_SMD:C_1210_3225Metric":
		return twoPadTemplate(3.2, 2.5, 1.2, 2.5, 2.4), true
	case "Capacitor_SMD:C_0603_1608Metric":
		return twoPadTemplate(1.6, 0.8, 0.6, 0.6, 1.0), true
	case "Diode_SMD:D_SOD-123":
		return twoPadTemplate(3.7, 1.8, 1.2, 1.2, 2.4), true
	case "Diode_SMD:D_SMA":
		return twoPadTemplate(6.2, 3.0, 1.5, 1.7, 4.4), true
	case "Fuse:Fuse_1206_3216Metric":
		return twoPadTemplate(4.5, 2.6, 1.6, 1.6, 2.8), true
	case "Connector_USB:USB_C_Receptacle_GCT_USB4125-xx-x_6P_TopMnt_Horizontal":
		return usbCGCTUSB4125PowerOnlyTemplate(), true
	case "Connector_USB:USB_C_Receptacle_HRO_TYPE-C-31-M-12":
		return usbCHROTypeC31M12Template(), true
	case "Package_TO_SOT_SMD:SOT-23-5":
		return verifiedPadTemplateRecord{
			Bounds: centeredEstimatedBounds(3.7, 3.0),
			Pads: []placement.PadSummary{
				{Name: "1", XMM: -1.5, YMM: -0.95, WidthMM: 0.7, HeightMM: 0.8},
				{Name: "2", XMM: -1.5, YMM: 0, WidthMM: 0.7, HeightMM: 0.8},
				{Name: "3", XMM: -1.5, YMM: 0.95, WidthMM: 0.7, HeightMM: 0.8},
				{Name: "5", XMM: 1.5, YMM: -0.95, WidthMM: 0.7, HeightMM: 0.8},
				{Name: "4", XMM: 1.5, YMM: 0.95, WidthMM: 0.7, HeightMM: 0.8},
			},
		}, true
	case "Package_TO_SOT_SMD:SOT-23":
		return verifiedPadTemplateRecord{
			Bounds: centeredEstimatedBounds(3.0, 2.8),
			Pads: []placement.PadSummary{
				{Name: "1", XMM: -0.95, YMM: 0.95, WidthMM: 0.7, HeightMM: 0.8},
				{Name: "2", XMM: -0.95, YMM: -0.95, WidthMM: 0.7, HeightMM: 0.8},
				{Name: "3", XMM: 0.95, YMM: 0, WidthMM: 0.7, HeightMM: 0.8},
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
	case "Package_SO:SOIC-8_3.9x4.9mm_P1.27mm":
		return rowPadTemplate(5.9, 4.9, 1.27, 0.7, 0.9, []string{"1", "2", "3", "4"}, []string{"8", "7", "6", "5"}), true
	case "Connector_PinHeader_2.54mm:PinHeader_1x01_P2.54mm_Vertical":
		return pinHeaderTemplate(1), true
	case "Connector_PinHeader_2.54mm:PinHeader_1x02_P2.54mm_Vertical":
		return pinHeaderTemplate(2), true
	case "Connector_PinHeader_2.54mm:PinHeader_1x03_P2.54mm_Vertical":
		return pinHeaderTemplate(3), true
	case "Connector_PinHeader_2.54mm:PinHeader_1x04_P2.54mm_Vertical":
		return pinHeaderTemplate(4), true
	case "Connector_PinHeader_2.54mm:PinHeader_1x05_P2.54mm_Vertical":
		return pinHeaderTemplate(5), true
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
		{Name: "SH2", XMM: -4.32, YMM: 1.05, WidthMM: 1.0, HeightMM: 1.6},
		{Name: "SH3", XMM: 4.32, YMM: -3.13, WidthMM: 1.0, HeightMM: 2.1},
		{Name: "SH4", XMM: 4.32, YMM: 1.05, WidthMM: 1.0, HeightMM: 1.6},
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
		{Name: "SH2", XMM: -4.32, YMM: 0.8, WidthMM: 1.1, HeightMM: 1.7},
		{Name: "SH3", XMM: 4.32, YMM: -3.0, WidthMM: 1.1, HeightMM: 1.7},
		{Name: "SH4", XMM: 4.32, YMM: 0.8, WidthMM: 1.1, HeightMM: 1.7},
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
	height := max(2.54, float64(count)*2.54)
	for index := 0; index < count; index++ {
		pads = append(pads, placement.PadSummary{
			Name:     fmt.Sprintf("%d", index+1),
			XMM:      0,
			YMM:      (float64(index) - float64(count-1)/2) * 2.54,
			WidthMM:  1.7,
			HeightMM: 1.7,
		})
	}
	return verifiedPadTemplateRecord{Bounds: padEnvelopeBounds(pads, 2.54, height), Pads: pads}
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
