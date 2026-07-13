package writercorrectness

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"kicadai/internal/kicadfiles"
	kschematic "kicadai/internal/kicadfiles/schematic"
	"kicadai/internal/libraryresolver"
	"kicadai/internal/reports"
)

type SchematicSnapshot struct {
	Files             []SchematicFileSnapshot `json:"files"`
	SymbolCount       int                     `json:"symbol_count"`
	WireCount         int                     `json:"wire_count"`
	LabelCount        int                     `json:"label_count"`
	SheetCount        int                     `json:"sheet_count"`
	NoConnectCount    int                     `json:"no_connect_count"`
	JunctionCount     int                     `json:"junction_count"`
	BoardSymbolCount  int                     `json:"board_symbol_count"`
	MissingFootprints []string                `json:"missing_footprints,omitempty"`
}

type SchematicFileSnapshot struct {
	Path               string           `json:"path"`
	Symbols            []SymbolSnapshot `json:"symbols,omitempty"`
	LocalLabels        []string         `json:"local_labels,omitempty"`
	GlobalLabels       []string         `json:"global_labels,omitempty"`
	HierarchicalLabels []string         `json:"hierarchical_labels,omitempty"`
	SheetPins          []string         `json:"sheet_pins,omitempty"`
	Sheets             []string         `json:"sheets,omitempty"`
}

type SymbolSnapshot struct {
	Reference string `json:"reference"`
	LibraryID string `json:"library_id,omitempty"`
	Footprint string `json:"footprint,omitempty"`
	UUID      string `json:"uuid,omitempty"`
	File      string `json:"file"`
}

func CheckSchematics(target Target) (SchematicSnapshot, []CheckResult) {
	return CheckSchematicsWithOptions(target, Options{})
}

func CheckSchematicsWithOptions(target Target, opts Options) (SchematicSnapshot, []CheckResult) {
	snapshot := SchematicSnapshot{}
	files := target.SchematicFiles
	if len(files) == 0 && target.SchematicPath != "" {
		files = []string{target.SchematicPath}
	}
	if len(files) == 0 {
		return snapshot, []CheckResult{{
			Name:     CheckSchematicParse,
			Status:   CheckSkipped,
			Required: false,
			Summary:  "no schematic files resolved",
		}, {
			Name:     CheckSchematicConnectivity,
			Status:   CheckSkipped,
			Required: false,
			Summary:  "no schematic files resolved",
		}}
	}

	var parseIssues []reports.Issue
	var connectivityIssues []reports.Issue
	seenRefs := map[string]string{}
	hierarchicalLabelsByFile := map[string]map[string]struct{}{}
	var sheetPinExpectations []sheetPinExpectation

	for _, path := range files {
		file, err := kschematic.ReadFile(filepath.FromSlash(path))
		if err != nil {
			parseIssues = append(parseIssues, BlockingIssue(reports.CodeValidationFailed, path, err.Error()))
			continue
		}
		fileSnapshot := SchematicFileSnapshot{Path: slashPath(path)}
		anchors := schematicAnchors(file)
		embeddedLibraryIDs := make(map[string]struct{}, len(file.LibSymbols))
		for _, embedded := range file.LibSymbols {
			if libraryID := strings.TrimSpace(embedded.LibraryID); libraryID != "" && len(embedded.Body) > 0 {
				embeddedLibraryIDs[libraryID] = struct{}{}
			}
		}
		for _, symbol := range file.Symbols {
			ref := strings.TrimSpace(symbol.Reference)
			footprint := symbolFootprint(symbol)
			fileSnapshot.Symbols = append(fileSnapshot.Symbols, SymbolSnapshot{
				Reference: ref,
				LibraryID: strings.TrimSpace(symbol.LibraryID),
				Footprint: footprint,
				UUID:      string(symbol.UUID),
				File:      slashPath(path),
			})
			snapshot.SymbolCount++
			if ref == "" {
				connectivityIssues = append(connectivityIssues, BlockingIssue(reports.CodeValidationFailed, path, "schematic symbol is missing a reference"))
				continue
			}
			libraryID := strings.TrimSpace(symbol.LibraryID)
			if libraryID == "" {
				connectivityIssues = append(connectivityIssues, reports.Issue{
					Code:     reports.CodeValidationFailed,
					Severity: reports.SeverityError,
					Path:     slashPath(path) + ".symbols." + ref + ".library_id",
					Message:  "schematic symbol has no library ID",
					Refs:     []string{ref},
				})
			} else if opts.HasLibraryIndex {
				connectivityIssues = append(connectivityIssues, resolverSymbolIssues(embeddedLibraryIDs, path, ref, libraryID, opts)...)
			}
			if previous, ok := seenRefs[strings.ToUpper(ref)]; ok && !strings.HasPrefix(ref, "#") {
				connectivityIssues = append(connectivityIssues, reports.Issue{
					Code:     reports.CodeDuplicateReference,
					Severity: reports.SeverityError,
					Path:     slashPath(path),
					Message:  fmt.Sprintf("duplicate schematic reference %s also appears in %s", ref, previous),
					Refs:     []string{ref},
				})
			} else {
				seenRefs[strings.ToUpper(ref)] = slashPath(path)
			}
			if boardBearingSymbol(symbol) {
				snapshot.BoardSymbolCount++
				if footprint == "" {
					snapshot.MissingFootprints = append(snapshot.MissingFootprints, ref)
					connectivityIssues = append(connectivityIssues, reports.Issue{
						Code:     reports.CodeMissingFootprint,
						Severity: reports.SeverityError,
						Path:     slashPath(path) + ".symbols." + ref,
						Message:  "PCB-bearing schematic symbol has no footprint assignment",
						Refs:     []string{ref},
					})
				}
			}
		}
		labelsForFile := map[string]struct{}{}
		for _, label := range file.Labels {
			text := strings.TrimSpace(label.Text)
			switch label.Kind {
			case kschematic.LabelGlobal:
				fileSnapshot.GlobalLabels = append(fileSnapshot.GlobalLabels, text)
			case kschematic.LabelHierarchical:
				fileSnapshot.HierarchicalLabels = append(fileSnapshot.HierarchicalLabels, text)
				if text != "" {
					labelsForFile[text] = struct{}{}
				}
			default:
				fileSnapshot.LocalLabels = append(fileSnapshot.LocalLabels, text)
			}
			if text != "" && !labelAttached(file, anchors, label.Position) {
				connectivityIssues = append(connectivityIssues, reports.Issue{
					Code:     reports.CodeValidationFailed,
					Severity: reports.SeverityWarning,
					Path:     slashPath(path),
					Message:  "schematic label is not attached to a parsed wire or bus, junction, or no-connect marker",
					Nets:     []string{text},
				})
			}
		}
		hierarchicalLabelsByFile[slashPath(path)] = labelsForFile
		for _, sheet := range file.Sheets {
			if strings.TrimSpace(sheet.Filename) != "" {
				fileSnapshot.Sheets = append(fileSnapshot.Sheets, sheet.Filename)
			}
			for _, pin := range sheet.Pins {
				text := strings.TrimSpace(pin.Text)
				if text == "" {
					continue
				}
				fileSnapshot.SheetPins = append(fileSnapshot.SheetPins, text)
				childPath := filepath.Clean(filepath.Join(filepath.Dir(filepath.FromSlash(path)), sheet.Filename))
				sheetPinExpectations = append(sheetPinExpectations, sheetPinExpectation{
					ParentFile: slashPath(path),
					ChildFile:  slashPath(childPath),
					Text:       text,
				})
			}
		}
		snapshot.WireCount += len(file.Wires)
		snapshot.LabelCount += len(file.Labels)
		snapshot.SheetCount += len(file.Sheets)
		snapshot.NoConnectCount += len(file.NoConnects)
		snapshot.JunctionCount += len(file.Junctions)
		sortFileSnapshot(&fileSnapshot)
		snapshot.Files = append(snapshot.Files, fileSnapshot)
	}
	for _, expectation := range sheetPinExpectations {
		labels := hierarchicalLabelsByFile[expectation.ChildFile]
		if _, ok := labels[expectation.Text]; !ok {
			connectivityIssues = append(connectivityIssues, reports.Issue{
				Code:     reports.CodeValidationFailed,
				Severity: reports.SeverityWarning,
				Path:     expectation.ParentFile + ".sheet_pins." + expectation.Text,
				Message:  "sheet pin has no matching hierarchical label in parsed schematic tree",
				Nets:     []string{expectation.Text},
			})
		}
	}
	slices.Sort(snapshot.MissingFootprints)
	slices.SortFunc(snapshot.Files, func(a, b SchematicFileSnapshot) int {
		return strings.Compare(a.Path, b.Path)
	})
	return snapshot, []CheckResult{{
		Name:     CheckSchematicParse,
		Required: true,
		Issues:   parseIssues,
		Summary:  fmt.Sprintf("parsed %d schematic file(s)", len(snapshot.Files)),
	}, {
		Name:     CheckSchematicConnectivity,
		Required: true,
		Issues:   connectivityIssues,
		Summary:  fmt.Sprintf("%d symbol(s), %d label(s), %d wire(s)", snapshot.SymbolCount, snapshot.LabelCount, snapshot.WireCount),
	}}
}

func resolverSymbolIssues(embeddedLibraryIDs map[string]struct{}, path string, ref string, libraryID string, opts Options) []reports.Issue {
	if _, ok := embeddedLibraryIDs[libraryID]; ok {
		return nil
	}
	if _, ok := libraryresolver.ResolveSymbol(opts.LibraryIndex, libraryID); !ok {
		return []reports.Issue{{
			Code:     reports.CodeMissingFile,
			Severity: reports.SeverityError,
			Path:     slashPath(path) + ".symbols." + ref + ".library_id",
			Message:  "symbol library record not found: " + libraryID,
			Refs:     []string{ref},
		}}
	}
	return nil
}

type sheetPinExpectation struct {
	ParentFile string
	ChildFile  string
	Text       string
}

func symbolFootprint(symbol kschematic.SchematicSymbol) string {
	for _, property := range symbol.Properties {
		if strings.EqualFold(property.Name, "Footprint") {
			return strings.TrimSpace(property.Value)
		}
	}
	for _, field := range symbol.Fields {
		if strings.EqualFold(field.Name, "Footprint") {
			return strings.TrimSpace(field.Value)
		}
	}
	return ""
}

func boardBearingSymbol(symbol kschematic.SchematicSymbol) bool {
	ref := strings.TrimSpace(symbol.Reference)
	if ref == "" || strings.HasPrefix(ref, "#") {
		return false
	}
	if symbol.OnBoard != nil && !*symbol.OnBoard {
		return false
	}
	lib := strings.ToLower(strings.TrimSpace(symbol.LibraryID))
	return !strings.HasPrefix(lib, "power:")
}

func schematicAnchors(file kschematic.SchematicFile) map[kicadfiles.Point]bool {
	anchors := map[kicadfiles.Point]bool{}
	for _, wire := range file.Wires {
		for _, point := range wire.Points {
			anchors[point] = true
		}
	}
	for _, bus := range file.Buses {
		for _, point := range bus.Points {
			anchors[point] = true
		}
	}
	for _, junction := range file.Junctions {
		anchors[junction.Position] = true
	}
	for _, noConnect := range file.NoConnects {
		anchors[noConnect.Position] = true
	}
	for _, symbol := range file.Symbols {
		for _, point := range symbol.PinAnchors {
			anchors[point] = true
		}
	}
	return anchors
}

func labelAttached(file kschematic.SchematicFile, anchors map[kicadfiles.Point]bool, point kicadfiles.Point) bool {
	if anchors[point] {
		return true
	}
	for _, wire := range file.Wires {
		for i := 1; i < len(wire.Points); i++ {
			if pointOnSegment(point, wire.Points[i-1], wire.Points[i]) {
				return true
			}
		}
	}
	for _, bus := range file.Buses {
		for i := 1; i < len(bus.Points); i++ {
			if pointOnSegment(point, bus.Points[i-1], bus.Points[i]) {
				return true
			}
		}
	}
	return false
}

func pointOnSegment(point, a, b kicadfiles.Point) bool {
	cross := (point.Y-a.Y)*(b.X-a.X) - (point.X-a.X)*(b.Y-a.Y)
	if cross != 0 {
		return false
	}
	minX, maxX := minMaxIU(a.X, b.X)
	minY, maxY := minMaxIU(a.Y, b.Y)
	return point.X >= minX && point.X <= maxX && point.Y >= minY && point.Y <= maxY
}

func minMaxIU(a, b kicadfiles.IU) (kicadfiles.IU, kicadfiles.IU) {
	if a <= b {
		return a, b
	}
	return b, a
}

func sortFileSnapshot(snapshot *SchematicFileSnapshot) {
	slices.SortFunc(snapshot.Symbols, func(a, b SymbolSnapshot) int {
		return strings.Compare(a.Reference, b.Reference)
	})
	slices.Sort(snapshot.LocalLabels)
	slices.Sort(snapshot.GlobalLabels)
	slices.Sort(snapshot.HierarchicalLabels)
	slices.Sort(snapshot.SheetPins)
	slices.Sort(snapshot.Sheets)
}
