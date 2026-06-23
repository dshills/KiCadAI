package fabrication

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"kicadai/internal/inspect"
	"kicadai/internal/kicadfiles"
	pcbfiles "kicadai/internal/kicadfiles/pcb"
	schematicfiles "kicadai/internal/kicadfiles/schematic"
	"kicadai/internal/reports"
)

type BOMRow struct {
	References            []string       `json:"references"`
	Quantity              int            `json:"quantity"`
	Value                 string         `json:"value"`
	SymbolID              string         `json:"symbol_id,omitempty"`
	FootprintID           string         `json:"footprint_id,omitempty"`
	ComponentID           string         `json:"component_id,omitempty"`
	Manufacturer          string         `json:"manufacturer,omitempty"`
	MPN                   string         `json:"mpn,omitempty"`
	Package               string         `json:"package,omitempty"`
	ComponentClass        string         `json:"component_class,omitempty"`
	Lifecycle             string         `json:"lifecycle,omitempty"`
	Confidence            string         `json:"confidence"`
	IdentityStatus        IdentityStatus `json:"identity_status,omitempty"`
	IdentitySource        IdentitySource `json:"identity_source,omitempty"`
	IdentityIssueCount    int            `json:"identity_issue_count,omitempty"`
	IdentityBlockingCount int            `json:"identity_blocking_count,omitempty"`
	ReadinessNote         string         `json:"readiness_note,omitempty"`
}

type CPLRow struct {
	Reference       string `json:"reference"`
	Footprint       string `json:"footprint"`
	XMM             string `json:"x_mm"`
	YMM             string `json:"y_mm"`
	RotationDegrees string `json:"rotation_degrees"`
	Layer           string `json:"layer"`
	PlacementSource string `json:"placement_source"`
	Fixed           bool   `json:"fixed"`
}

type ReportData struct {
	BOM    []BOMRow        `json:"bom"`
	CPL    []CPLRow        `json:"cpl"`
	Issues []reports.Issue `json:"issues"`
}

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
			cpl = BuildCPLRows(board)
		}
	}
	issues = dedupeIssues(issues)
	slices.SortFunc(issues, compareIssues)
	return ReportData{BOM: bom, CPL: cpl, Issues: issues}, nil
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
		manufacturer := firstNonEmpty(lookup(properties, "Manufacturer"), lookup(fields, "Manufacturer"))
		mpn := firstNonEmpty(lookup(properties, "MPN"), lookup(properties, "Manufacturer Part Number"), lookup(fields, "MPN"))
		componentID := firstNonEmpty(lookup(properties, "Component ID"), lookup(fields, "Component ID"))
		packageName := firstNonEmpty(lookup(properties, "Package"), lookup(fields, "Package"))
		componentClass := firstNonEmpty(lookup(properties, "Component Class"), lookup(fields, "Component Class"))
		lifecycle := firstNonEmpty(lookup(properties, "Lifecycle"), lookup(fields, "Lifecycle"))
		confidence := firstNonEmpty(lookup(properties, "Confidence"), lookup(fields, "Confidence"), "partial")
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
	rows := make([]CPLRow, 0, len(board.Footprints))
	for _, footprint := range board.Footprints {
		ref := strings.TrimSpace(footprint.Reference)
		if ref == "" || strings.HasPrefix(ref, "#") {
			continue
		}
		if footprintDNP(footprint) {
			continue
		}
		rows = append(rows, CPLRow{
			Reference:       ref,
			Footprint:       footprint.LibraryID,
			XMM:             kicadfiles.ToMMString(footprint.Position.X),
			YMM:             kicadfiles.ToMMString(footprint.Position.Y),
			RotationDegrees: fmt.Sprintf("%.3f", footprint.Rotation),
			Layer:           cplLayer(footprint.Layer),
			PlacementSource: "pcb",
			Fixed:           footprint.Locked,
		})
	}
	slices.SortFunc(rows, compareCPLRows)
	return rows
}

func MarshalBOMCSV(rows []BOMRow) ([]byte, error) {
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)
	if err := writer.Write([]string{"References", "Quantity", "Value", "SymbolID", "FootprintID", "ComponentID", "Manufacturer", "MPN", "Package", "ComponentClass", "Lifecycle", "Confidence", "IdentityStatus", "IdentitySource", "IdentityIssueCount", "IdentityBlockingCount", "ReadinessNote"}); err != nil {
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
	if err := writer.Write([]string{"Reference", "Footprint", "X(mm)", "Y(mm)", "Rotation", "Layer", "PlacementSource", "Fixed"}); err != nil {
		return nil, err
	}
	for _, row := range rows {
		if err := writer.Write([]string{row.Reference, row.Footprint, row.XMM, row.YMM, row.RotationDegrees, row.Layer, row.PlacementSource, fmt.Sprintf("%t", row.Fixed)}); err != nil {
			return nil, err
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
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
	if layer == kicadfiles.LayerBCu || strings.HasPrefix(string(layer), "B.") {
		return "bottom"
	}
	return "top"
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
