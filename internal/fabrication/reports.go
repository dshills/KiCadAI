package fabrication

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"math"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"kicadai/internal/componentprops"
	"kicadai/internal/inspect"
	"kicadai/internal/kicadfiles"
	pcbfiles "kicadai/internal/kicadfiles/pcb"
	schematicfiles "kicadai/internal/kicadfiles/schematic"
	"kicadai/internal/reports"
)

type BOMRow struct {
	References             []string       `json:"references"`
	Quantity               int            `json:"quantity"`
	Value                  string         `json:"value"`
	SymbolID               string         `json:"symbol_id,omitempty"`
	FootprintID            string         `json:"footprint_id,omitempty"`
	ComponentID            string         `json:"component_id,omitempty"`
	Manufacturer           string         `json:"manufacturer,omitempty"`
	MPN                    string         `json:"mpn,omitempty"`
	Package                string         `json:"package,omitempty"`
	ComponentClass         string         `json:"component_class,omitempty"`
	Lifecycle              string         `json:"lifecycle,omitempty"`
	ProcurementSourceID    string         `json:"procurement_source_id,omitempty"`
	LifecycleSourceDate    string         `json:"lifecycle_source_date,omitempty"`
	LifecycleFresh         *bool          `json:"lifecycle_fresh,omitempty"`
	AvailabilityStatus     string         `json:"availability_status,omitempty"`
	AvailabilitySourceDate string         `json:"availability_source_date,omitempty"`
	AvailabilityFresh      *bool          `json:"availability_fresh,omitempty"`
	ProcurementOutcome     string         `json:"procurement_outcome,omitempty"`
	Confidence             string         `json:"confidence"`
	IdentityStatus         IdentityStatus `json:"identity_status,omitempty"`
	IdentitySource         IdentitySource `json:"identity_source,omitempty"`
	IdentityIssueCount     int            `json:"identity_issue_count,omitempty"`
	IdentityBlockingCount  int            `json:"identity_blocking_count,omitempty"`
	ReadinessNote          string         `json:"readiness_note,omitempty"`
}

type CPLRow struct {
	Reference                 string `json:"reference"`
	Footprint                 string `json:"footprint"`
	ComponentID               string `json:"component_id,omitempty"`
	Manufacturer              string `json:"manufacturer,omitempty"`
	MPN                       string `json:"mpn,omitempty"`
	IdentityKey               string `json:"identity_key,omitempty"`
	XMM                       string `json:"x_mm"`
	YMM                       string `json:"y_mm"`
	RotationDegrees           string `json:"rotation_degrees"`
	Layer                     string `json:"layer"`
	NormalizedSide            string `json:"normalized_side,omitempty"`
	RawLayer                  string `json:"raw_layer,omitempty"`
	RawRotationDegrees        string `json:"raw_rotation_degrees,omitempty"`
	NormalizedRotationDegrees string `json:"normalized_rotation_degrees,omitempty"`
	BOMLinkageStatus          string `json:"bom_linkage_status,omitempty"`
	PlacementSource           string `json:"placement_source"`
	Fixed                     bool   `json:"fixed"`
	ReadinessNote             string `json:"readiness_note,omitempty"`
}

type ReportData struct {
	BOM         []BOMRow           `json:"bom"`
	CPL         []CPLRow           `json:"cpl"`
	Consistency ConsistencySummary `json:"consistency,omitempty"`
	Issues      []reports.Issue    `json:"issues"`
}

type ConsistencySummary struct {
	CheckedReferences int `json:"checked_references"`
	MatchedReferences int `json:"matched_references"`
	SkippedReferences int `json:"skipped_references,omitempty"`
	WarningCount      int `json:"warning_count,omitempty"`
	BlockingCount     int `json:"blocking_count,omitempty"`
}

const (
	cplSideTop     = "top"
	cplSideBottom  = "bottom"
	cplSideUnknown = "unknown"
)

func BuildReports(ctx context.Context, targetPath string) (ReportData, error) {
	target, err := resolveEvaluationTarget(targetPath)
	if err != nil {
		return ReportData{}, err
	}
	summary, err := inspect.ProjectContextWithProjectPath(ctx, target.Root, target.ProjectPath)
	if err != nil {
		return ReportData{}, err
	}
	schematicPath := summaryFilePath(target.Root, summary.Files, "schematic")
	boardPath := summaryFilePath(target.Root, summary.Files, "pcb")
	var issues []reports.Issue
	var bom []BOMRow
	var cpl []CPLRow
	if summary.Schematic == nil {
		issues = append(issues, missingReportDataIssue("schematic", "schematic is required to build a BOM"))
	} else {
		schematic, err := readSchematicsRecursive(schematicPath)
		if err != nil {
			issues = append(issues, reportDataIssue("schematic", err.Error()))
		} else {
			rows, rowIssues := BuildBOMRows(schematic)
			bom = rows
			issues = append(issues, rowIssues...)
		}
	}
	if summary.PCB == nil {
		issues = append(issues, missingReportDataIssue("pcb", "PCB is required to build a CPL"))
	} else {
		board, err := pcbfiles.ReadFile(boardPath)
		if err != nil {
			issues = append(issues, reportDataIssue("pcb", err.Error()))
		} else {
			rows, rowIssues := BuildCPLRowsWithBOM(board, bom)
			cpl = rows
			issues = append(issues, rowIssues...)
		}
	}
	consistency, consistencyIssues := ValidateBOMCPLConsistency(bom, cpl)
	issues = append(issues, consistencyIssues...)
	issues = dedupeIssues(issues)
	slices.SortFunc(issues, compareIssues)
	return ReportData{BOM: bom, CPL: cpl, Consistency: consistency, Issues: issues}, nil
}

func summaryFilePath(root string, files []inspect.FileSummary, kind string) string {
	for _, file := range files {
		if file.Kind == kind {
			if filepath.IsAbs(file.Path) {
				return file.Path
			}
			if strings.TrimSpace(file.Path) != "" {
				return filepath.Join(root, file.Path)
			}
			return file.Path
		}
	}
	return ""
}

func readSchematicsRecursive(rootPath string) (schematicfiles.SchematicFile, error) {
	if strings.TrimSpace(rootPath) == "" {
		return schematicfiles.SchematicFile{}, fmt.Errorf("schematic path is required")
	}
	stack := map[string]struct{}{}
	cache := map[string]schematicfiles.SchematicFile{}
	seen := map[string]int{}
	return readSchematicsRecursivePath(rootPath, stack, cache, seen)
}

func readSchematicsRecursivePath(path string, stack map[string]struct{}, cache map[string]schematicfiles.SchematicFile, seen map[string]int) (schematicfiles.SchematicFile, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return schematicfiles.SchematicFile{}, err
	}
	if _, ok := stack[absolute]; ok {
		return schematicfiles.SchematicFile{}, fmt.Errorf("circular sheet reference detected: %s", absolute)
	}
	if seen[absolute] > 0 {
		return schematicfiles.SchematicFile{}, fmt.Errorf("reused hierarchical sheet requires instance-specific references and is not yet supported: %s", absolute)
	}
	seen[absolute]++
	stack[absolute] = struct{}{}
	defer delete(stack, absolute)
	file, ok := cache[absolute]
	if !ok {
		loaded, err := schematicfiles.ReadFile(absolute)
		if err != nil {
			return schematicfiles.SchematicFile{}, fmt.Errorf("failed to read schematic %s: %w", absolute, err)
		}
		cache[absolute] = loaded
		file = loaded
	}
	merged := file
	merged.Symbols = slices.Clone(file.Symbols)
	for _, sheet := range file.Sheets {
		if sheet.DoNotPopulate || boolPtrValue(sheet.InBOM, true) == false || boolPtrValue(sheet.OnBoard, true) == false {
			continue
		}
		childPath := strings.TrimSpace(sheet.Filename)
		if childPath == "" {
			continue
		}
		if !filepath.IsAbs(childPath) {
			childPath = filepath.Join(filepath.Dir(absolute), filepath.FromSlash(childPath))
		}
		child, err := readSchematicsRecursivePath(childPath, stack, cache, seen)
		if err != nil {
			return schematicfiles.SchematicFile{}, fmt.Errorf("failed to read sheet %s: %w", childPath, err)
		}
		merged.Symbols = append(merged.Symbols, child.Symbols...)
	}
	return merged, nil
}

func BuildBOMRows(schematic schematicfiles.SchematicFile) ([]BOMRow, []reports.Issue) {
	type bomGroupKey struct {
		value          string
		symbolID       string
		footprintID    string
		componentID    string
		manufacturer   string
		mpn            string
		packageName    string
		componentClass string
	}
	groups := map[bomGroupKey]*BOMRow{}
	var issues []reports.Issue
	for _, symbol := range schematic.Symbols {
		if symbol.DoNotPopulate || boolPtrValue(symbol.InBOM, true) == false || boolPtrValue(symbol.OnBoard, true) == false {
			continue
		}
		ref := strings.TrimSpace(symbol.Reference)
		if ref == "" || strings.HasPrefix(ref, "#") {
			continue
		}
		properties := propertyMap(symbol.Properties)
		fields := fieldMap(symbol.Fields)
		value := firstNonEmpty(strings.TrimSpace(symbol.Value), lookup(properties, "Value"), lookup(fields, "Value"))
		footprint := firstNonEmpty(lookup(properties, "Footprint"), lookup(fields, "Footprint"))
		manufacturer := firstNonEmpty(lookup(properties, componentprops.PropertyManufacturer), lookup(properties, "Manufacturer"), lookup(fields, "Manufacturer"))
		mpn := firstNonEmpty(lookup(properties, componentprops.PropertyMPN), lookup(properties, "MPN"), lookup(properties, "Manufacturer Part Number"), lookup(fields, "MPN"))
		componentID := firstNonEmpty(lookup(properties, componentprops.PropertyComponentID), lookup(properties, "Component ID"), lookup(fields, "Component ID"))
		packageName := firstNonEmpty(lookup(properties, "Package"), lookup(fields, "Package"))
		componentClass := firstNonEmpty(lookup(properties, componentprops.PropertyComponentClass), lookup(properties, "Component Class"), lookup(fields, "Component Class"))
		lifecycle := firstNonEmpty(lookup(properties, componentprops.PropertyLifecycleStatus), lookup(properties, "Lifecycle"), lookup(fields, "Lifecycle"))
		availability := strings.ToLower(firstNonEmpty(lookup(properties, componentprops.PropertyAvailabilityStatus), lookup(properties, "Availability"), lookup(fields, "Availability")))
		confidence := firstNonEmpty(lookup(properties, componentprops.PropertyComponentConfidence), lookup(properties, "Confidence"), lookup(fields, "Confidence"), "partial")
		identity := NormalizeComponentIdentity(ComponentIdentity{
			Reference:      ref,
			ComponentID:    componentID,
			Value:          value,
			SymbolID:       symbol.LibraryID,
			FootprintID:    footprint,
			Manufacturer:   manufacturer,
			MPN:            mpn,
			Package:        packageName,
			ComponentClass: componentClass,
			Lifecycle:      lifecycle,
			Confidence:     confidence,
		})
		key := bomGroupKey{
			value:          identity.Value,
			symbolID:       identity.SymbolID,
			footprintID:    identity.FootprintID,
			componentID:    identity.ComponentID,
			manufacturer:   identity.Manufacturer,
			mpn:            identity.MPN,
			packageName:    identity.Package,
			componentClass: identity.ComponentClass,
		}
		row := groups[key]
		if row == nil {
			issueCount, blockingCount := IdentityIssueCounts(identity.Issues)
			row = &BOMRow{
				Value:                 identity.Value,
				SymbolID:              identity.SymbolID,
				FootprintID:           identity.FootprintID,
				ComponentID:           identity.ComponentID,
				Manufacturer:          identity.Manufacturer,
				MPN:                   identity.MPN,
				Package:               identity.Package,
				ComponentClass:        identity.ComponentClass,
				Lifecycle:             identity.Lifecycle,
				AvailabilityStatus:    availability,
				Confidence:            identity.Confidence,
				IdentityStatus:        identity.Status,
				IdentitySource:        identity.Source,
				IdentityIssueCount:    issueCount,
				IdentityBlockingCount: blockingCount,
			}
			groups[key] = row
		}
		row.References = append(row.References, ref)
		if identity.Manufacturer == "" || identity.MPN == "" {
			issue := reports.Issue{
				Code:       reports.CodeValidationFailed,
				Severity:   reports.SeverityWarning,
				Path:       "bom." + ref,
				Message:    fmt.Sprintf("%s is missing manufacturer or MPN data", ref),
				Refs:       []string{ref},
				Suggestion: "add Manufacturer and MPN properties before fabrication release",
			}
			addBOMRowReadinessIssue(row, &issues, "missing manufacturer or MPN", issue)
		}
		if identity.FootprintID == "" {
			issue := reports.Issue{
				Code:       reports.CodeMissingFootprint,
				Severity:   reports.SeverityError,
				Path:       "bom." + ref + ".footprint",
				Message:    fmt.Sprintf("%s has no footprint assignment", ref),
				Refs:       []string{ref},
				Suggestion: "assign a KiCad footprint before fabrication release",
			}
			addBOMRowReadinessIssue(row, &issues, "missing footprint", issue)
		}
		if identity.Manufacturer != "" && identity.MPN != "" && identity.FootprintID != "" {
			if row.Confidence == "" || strings.EqualFold(row.Confidence, "partial") {
				row.Confidence = "high"
			}
		}
	}
	rows := make([]BOMRow, 0, len(groups))
	for _, row := range groups {
		slices.SortFunc(row.References, compareReferences)
		row.Quantity = len(row.References)
		rows = append(rows, *row)
	}
	slices.SortFunc(rows, compareBOMRows)
	return rows, issues
}

func BuildCPLRows(board pcbfiles.PCBFile) []CPLRow {
	rows, _ := BuildCPLRowsWithBOM(board, nil)
	return rows
}

func BuildCPLRowsWithBOM(board pcbfiles.PCBFile, bom []BOMRow) ([]CPLRow, []reports.Issue) {
	bomByRef := bomRowsByReference(bom)
	rows := make([]CPLRow, 0, len(board.Footprints))
	var issues []reports.Issue
	for _, footprint := range board.Footprints {
		ref := strings.TrimSpace(footprint.Reference)
		if ref == "" || strings.HasPrefix(ref, "#") {
			continue
		}
		if footprintDNP(footprint) {
			continue
		}
		rawRotation := float64(footprint.Rotation)
		normalizedRotation := normalizeRotationDegrees(rawRotation)
		side, sideIssue := normalizeCPLSide(footprint.Layer, ref)
		row := CPLRow{
			Reference:                 ref,
			Footprint:                 footprint.LibraryID,
			XMM:                       kicadfiles.ToMMString(footprint.Position.X),
			YMM:                       kicadfiles.ToMMString(footprint.Position.Y),
			RotationDegrees:           formatDegrees(normalizedRotation),
			Layer:                     side,
			NormalizedSide:            side,
			RawLayer:                  string(footprint.Layer),
			RawRotationDegrees:        formatDegrees(rawRotation),
			NormalizedRotationDegrees: formatDegrees(normalizedRotation),
			BOMLinkageStatus:          "unlinked",
			PlacementSource:           "pcb",
			Fixed:                     footprint.Locked,
		}
		if bomRow, ok := bomByRef[ref]; ok {
			row.ComponentID = bomRow.ComponentID
			row.Manufacturer = bomRow.Manufacturer
			row.MPN = bomRow.MPN
			row.IdentityKey = bomRowIdentityKey(bomRow)
			row.BOMLinkageStatus = "linked"
		} else if len(bom) > 0 {
			row.BOMLinkageStatus = "missing_bom"
			row.ReadinessNote = appendReadinessNote(row.ReadinessNote, "missing BOM identity linkage")
		}
		if sideIssue != nil {
			row.ReadinessNote = appendReadinessNote(row.ReadinessNote, "unknown placement side")
			issues = append(issues, *sideIssue)
		}
		rows = append(rows, row)
	}
	slices.SortFunc(rows, compareCPLRows)
	return rows, issues
}

func ValidateBOMCPLConsistency(bom []BOMRow, cpl []CPLRow) (ConsistencySummary, []reports.Issue) {
	type bomRefEntry struct {
		row BOMRow
		ref string
	}
	bomRefs := map[string]bomRefEntry{}
	cplRefs := map[string]CPLRow{}
	var issues []reports.Issue
	for _, row := range bom {
		for _, ref := range row.References {
			refKey := referenceKey(ref)
			if refKey == "" {
				continue
			}
			if _, exists := bomRefs[refKey]; exists {
				issues = append(issues, consistencyIssue(
					reports.CodeDuplicateReference,
					reports.SeverityError,
					"bom."+ref,
					ref,
					fmt.Sprintf("%s appears in multiple BOM rows", ref),
					"ensure each assembled reference appears in exactly one BOM row",
				))
				continue
			}
			bomRefs[refKey] = bomRefEntry{row: row, ref: strings.TrimSpace(ref)}
		}
	}
	for _, row := range cpl {
		ref := strings.TrimSpace(row.Reference)
		refKey := referenceKey(ref)
		if refKey == "" {
			continue
		}
		if _, exists := cplRefs[refKey]; exists {
			issues = append(issues, consistencyIssue(
				reports.CodeDuplicateReference,
				reports.SeverityError,
				"cpl."+ref,
				ref,
				fmt.Sprintf("%s appears in multiple CPL rows", ref),
				"ensure each placed reference appears in exactly one CPL row",
			))
			continue
		}
		cplRefs[refKey] = row
	}
	matches := 0
	for refKey, bomEntry := range bomRefs {
		cplRow, ok := cplRefs[refKey]
		bomRow := bomEntry.row
		ref := bomEntry.ref
		if !ok {
			issues = append(issues, consistencyIssue(
				reports.CodeValidationFailed,
				reports.SeverityError,
				"cpl."+ref,
				ref,
				fmt.Sprintf("%s is present in the BOM but missing from the CPL", ref),
				"place the footprint on the PCB or mark it not-on-board/DNP",
			))
			continue
		}
		matches++
		if bomRow.FootprintID != "" && cplRow.Footprint != "" && bomRow.FootprintID != cplRow.Footprint {
			issues = append(issues, consistencyIssue(
				reports.CodeValidationFailed,
				reports.SeverityError,
				"cpl."+ref+".footprint",
				ref,
				fmt.Sprintf("%s footprint mismatch: BOM has %q, CPL has %q", ref, bomRow.FootprintID, cplRow.Footprint),
				"regenerate the PCB from the assigned schematic footprint or update the mismatched footprint",
			))
		}
		if strings.TrimSpace(cplRow.XMM) == "" || strings.TrimSpace(cplRow.YMM) == "" {
			issues = append(issues, consistencyIssue(
				reports.CodeValidationFailed,
				reports.SeverityError,
				"cpl."+ref+".position",
				ref,
				fmt.Sprintf("%s is missing placement coordinates", ref),
				"place the footprint before fabrication release",
			))
		}
		if cplRow.Layer == cplSideUnknown || cplRow.NormalizedSide == cplSideUnknown {
			issues = append(issues, consistencyIssue(
				reports.CodeValidationFailed,
				reports.SeverityError,
				"cpl."+ref+".layer",
				ref,
				fmt.Sprintf("%s has unknown assembly side", ref),
				"place assembled footprints on F.Cu or B.Cu",
			))
		}
	}
	for refKey, cplRow := range cplRefs {
		if _, ok := bomRefs[refKey]; !ok {
			ref := strings.TrimSpace(cplRow.Reference)
			issues = append(issues, consistencyIssue(
				reports.CodeValidationFailed,
				reports.SeverityError,
				"bom."+ref,
				ref,
				fmt.Sprintf("%s is present in the CPL but missing from the BOM", ref),
				"include the reference in the BOM or mark the footprint DNP",
			))
		}
	}
	summary := ConsistencySummary{
		CheckedReferences: len(bomRefs) + len(cplRefs) - matches,
		MatchedReferences: matches,
	}
	for _, issue := range issues {
		if issue.Blocking() {
			summary.BlockingCount++
		} else {
			summary.WarningCount++
		}
	}
	return summary, issues
}

func MarshalBOMCSV(rows []BOMRow) ([]byte, error) {
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)
	if err := writer.Write([]string{"References", "Quantity", "Value", "SymbolID", "FootprintID", "ComponentID", "Manufacturer", "MPN", "Package", "ComponentClass", "Lifecycle", "Confidence", "IdentityStatus", "IdentitySource", "IdentityIssueCount", "IdentityBlockingCount", "ReadinessNote", "ProcurementSourceID", "LifecycleSourceDate", "LifecycleFresh", "AvailabilityStatus", "AvailabilitySourceDate", "AvailabilityFresh", "ProcurementOutcome"}); err != nil {
		return nil, err
	}
	for _, row := range rows {
		if err := writer.Write([]string{
			strings.Join(row.References, " "),
			strconv.Itoa(row.Quantity),
			row.Value,
			row.SymbolID,
			row.FootprintID,
			row.ComponentID,
			row.Manufacturer,
			row.MPN,
			row.Package,
			row.ComponentClass,
			row.Lifecycle,
			row.Confidence,
			string(row.IdentityStatus),
			string(row.IdentitySource),
			strconv.Itoa(row.IdentityIssueCount),
			strconv.Itoa(row.IdentityBlockingCount),
			row.ReadinessNote,
			row.ProcurementSourceID,
			row.LifecycleSourceDate,
			boolPtrCSV(row.LifecycleFresh),
			row.AvailabilityStatus,
			row.AvailabilitySourceDate,
			boolPtrCSV(row.AvailabilityFresh),
			row.ProcurementOutcome,
		}); err != nil {
			return nil, err
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func MarshalCPLCSV(rows []CPLRow) ([]byte, error) {
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)
	if err := writer.Write(cplCSVHeader()); err != nil {
		return nil, err
	}
	for _, row := range rows {
		if err := writer.Write(cplCSVRecord(row)); err != nil {
			return nil, err
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func cplCSVHeader() []string {
	return []string{
		"Reference",
		"Footprint",
		"ComponentID",
		"Manufacturer",
		"MPN",
		"IdentityKey",
		"X(mm)",
		"Y(mm)",
		"Rotation",
		"Layer",
		"NormalizedSide",
		"RawLayer",
		"RawRotation",
		"NormalizedRotation",
		"BOMLinkageStatus",
		"PlacementSource",
		"Fixed",
		"ReadinessNote",
	}
}

func cplCSVRecord(row CPLRow) []string {
	return []string{
		row.Reference,
		row.Footprint,
		row.ComponentID,
		row.Manufacturer,
		row.MPN,
		row.IdentityKey,
		row.XMM,
		row.YMM,
		row.RotationDegrees,
		row.Layer,
		row.NormalizedSide,
		row.RawLayer,
		row.RawRotationDegrees,
		row.NormalizedRotationDegrees,
		row.BOMLinkageStatus,
		row.PlacementSource,
		fmt.Sprintf("%t", row.Fixed),
		row.ReadinessNote,
	}
}

func MarshalReportJSON(data ReportData) ([]byte, error) {
	normalized := data
	normalized.BOM = slices.Clone(data.BOM)
	normalized.CPL = slices.Clone(data.CPL)
	normalized.Issues = dedupeIssues(data.Issues)
	slices.SortFunc(normalized.BOM, compareBOMRows)
	slices.SortFunc(normalized.CPL, compareCPLRows)
	slices.SortFunc(normalized.Issues, compareIssues)
	out, err := json.MarshalIndent(normalized, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(out, '\n'), nil
}

func compareBOMRows(a, b BOMRow) int {
	aRef := ""
	bRef := ""
	if len(a.References) > 0 {
		aRef = a.References[0]
	}
	if len(b.References) > 0 {
		bRef = b.References[0]
	}
	if aRef != bRef {
		return compareReferences(aRef, bRef)
	}
	return strings.Compare(a.Value, b.Value)
}

func compareCPLRows(a, b CPLRow) int {
	return compareReferences(a.Reference, b.Reference)
}

func cplLayer(layer kicadfiles.BoardLayer) string {
	side, _ := normalizeCPLSide(layer, "")
	return side
}

func normalizeCPLSide(layer kicadfiles.BoardLayer, ref string) (string, *reports.Issue) {
	layerName := strings.TrimSpace(string(layer))
	switch layer {
	case kicadfiles.LayerFCu:
		return cplSideTop, nil
	case kicadfiles.LayerBCu:
		return cplSideBottom, nil
	default:
		path := "cpl"
		if ref != "" {
			path = "cpl." + ref + ".layer"
		}
		message := fmt.Sprintf("placement layer %q is not an assembly side", layerName)
		refs := []string{}
		if ref != "" {
			message = fmt.Sprintf("%s has unknown placement layer %q", ref, layerName)
			refs = []string{ref}
		}
		return cplSideUnknown, &reports.Issue{
			Code:       reports.CodeValidationFailed,
			Severity:   reports.SeverityError,
			Path:       path,
			Message:    message,
			Refs:       refs,
			Suggestion: "place assembled footprints on F.Cu or B.Cu before fabrication release",
		}
	}
}

func normalizeRotationDegrees(degrees float64) float64 {
	normalized := math.Mod(math.Mod(degrees, 360)+360, 360)
	if normalized == 0 {
		return 0
	}
	return normalized
}

func formatDegrees(degrees float64) string {
	return fmt.Sprintf("%.3f", degrees)
}

func bomRowsByReference(rows []BOMRow) map[string]BOMRow {
	byRef := map[string]BOMRow{}
	for _, row := range rows {
		for _, ref := range row.References {
			ref = strings.TrimSpace(ref)
			if ref != "" {
				byRef[ref] = row
			}
		}
	}
	return byRef
}

func bomRowIdentityKey(row BOMRow) string {
	if row.ComponentID != "" {
		return row.ComponentID
	}
	parts := []string{row.Value, row.SymbolID, row.FootprintID, row.Manufacturer, row.MPN}
	for index := range parts {
		parts[index] = strings.ReplaceAll(strings.TrimSpace(parts[index]), "|", `\|`)
	}
	return strings.Join(parts, "|")
}

func consistencyIssue(code reports.Code, severity reports.Severity, path string, ref string, message string, suggestion string) reports.Issue {
	return reports.Issue{
		Code:       code,
		Severity:   severity,
		Path:       path,
		Message:    message,
		Refs:       []string{ref},
		Suggestion: suggestion,
	}
}

func referenceKey(ref string) string {
	return strings.ToUpper(strings.TrimSpace(ref))
}

func footprintDNP(footprint pcbfiles.Footprint) bool {
	for _, attr := range footprint.Attributes {
		if strings.EqualFold(strings.TrimSpace(attr), "dnp") {
			return true
		}
	}
	return false
}

func appendReadinessNote(existing string, note string) string {
	if strings.TrimSpace(existing) == "" {
		return note
	}
	if strings.Contains(existing, note) {
		return existing
	}
	return existing + "; " + note
}

func addBOMRowReadinessIssue(row *BOMRow, issues *[]reports.Issue, note string, issue reports.Issue) {
	if row != nil {
		row.ReadinessNote = appendReadinessNote(row.ReadinessNote, note)
		addBOMRowIdentityIssue(row, issue)
	}
	*issues = append(*issues, issue)
}

func addBOMRowIdentityIssue(row *BOMRow, issue reports.Issue) {
	if row == nil {
		return
	}
	row.IdentityIssueCount++
	if issue.Blocking() {
		row.IdentityBlockingCount++
	}
	if issue.Blocking() {
		row.IdentityStatus = IdentityFail
		return
	}
	if row.IdentityStatus == "" || row.IdentityStatus == IdentityPass {
		row.IdentityStatus = IdentityWarning
	}
}

func propertyMap(properties []schematicfiles.Property) map[string]string {
	values := map[string]string{}
	for _, property := range properties {
		key := propertyKey(property.Name)
		if key != "" {
			values[key] = strings.TrimSpace(property.Value)
		}
	}
	return values
}

func fieldMap(fields []schematicfiles.Field) map[string]string {
	values := map[string]string{}
	for _, field := range fields {
		key := propertyKey(field.Name)
		if key != "" {
			values[key] = strings.TrimSpace(field.Value)
		}
	}
	return values
}

func lookup(values map[string]string, name string) string {
	return values[propertyKey(name)]
}

func propertyKey(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func compareReferences(a string, b string) int {
	aPrefix, aNumber, aHasNumber, aSuffix := splitReference(a)
	bPrefix, bNumber, bHasNumber, bSuffix := splitReference(b)
	if aPrefix != bPrefix {
		return strings.Compare(aPrefix, bPrefix)
	}
	if aHasNumber && bHasNumber && aNumber != bNumber {
		if aNumber < bNumber {
			return -1
		}
		return 1
	}
	if aHasNumber != bHasNumber {
		if aHasNumber {
			return -1
		}
		return 1
	}
	if aSuffix != bSuffix {
		return strings.Compare(aSuffix, bSuffix)
	}
	return strings.Compare(a, b)
}

func splitReference(reference string) (string, int, bool, string) {
	ref := strings.TrimSpace(reference)
	start := -1
	end := -1
	for index, char := range ref {
		if char >= '0' && char <= '9' {
			if start == -1 {
				start = index
			}
			end = index + 1
			continue
		}
		if start != -1 {
			break
		}
	}
	if start == -1 {
		return ref, 0, false, ""
	}
	number, err := strconv.Atoi(ref[start:end])
	if err != nil {
		return ref, 0, false, ""
	}
	return ref[:start], number, true, ref[end:]
}

func boolPtrValue(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
	}
	return *value
}

func boolPtrCSV(value *bool) string {
	if value == nil {
		return ""
	}
	return strconv.FormatBool(*value)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func missingReportDataIssue(path string, message string) reports.Issue {
	return reports.Issue{Code: reports.CodeMissingFile, Severity: reports.SeverityError, Path: path, Message: message}
}

func reportDataIssue(path string, message string) reports.Issue {
	return reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Path: path, Message: message}
}
