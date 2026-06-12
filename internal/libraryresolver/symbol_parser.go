package libraryresolver

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"

	"kicadai/internal/kicadfiles"
	"kicadai/internal/kicadfiles/sexpr"
	"kicadai/internal/reports"
)

const maxSymbolLibraryBytes int64 = 64 << 20

func IndexSymbols(inventory LibraryInventory) (map[string]SymbolRecord, []reports.Issue) {
	return IndexSymbolsContext(context.Background(), inventory)
}

func IndexSymbolsContext(ctx context.Context, inventory LibraryInventory) (map[string]SymbolRecord, []reports.Issue) {
	records := make(map[string]SymbolRecord, len(inventory.SymbolFiles)*20)
	var issues []reports.Issue
	if issue, ok := contextIssue(ctx); ok {
		return records, []reports.Issue{issue}
	}
	results := parseSymbolFiles(ctx, inventory.SymbolFiles)
	for _, result := range results {
		issues = append(issues, result.issues...)
		for _, record := range result.records {
			if _, exists := records[record.LibraryID]; exists {
				issues = append(issues, reports.Issue{
					Code:     reports.CodeValidationFailed,
					Severity: reports.SeverityWarning,
					Path:     record.Path,
					Message:  "duplicate symbol ID " + record.LibraryID,
				})
				continue
			}
			records[record.LibraryID] = record
		}
	}
	if issue, ok := contextIssue(ctx); ok {
		issues = append(issues, issue)
	}
	return records, issues
}

type symbolParseResult struct {
	records []SymbolRecord
	issues  []reports.Issue
}

func parseSymbolFiles(ctx context.Context, files []LibraryFile) []symbolParseResult {
	results := make([]symbolParseResult, len(files))
	if len(files) == 0 {
		return results
	}
	if ctx == nil {
		ctx = context.Background()
	}
	workerCount := runtime.GOMAXPROCS(0)
	if workerCount > len(files) {
		workerCount = len(files)
	}
	jobs := make(chan int)
	var waitGroup sync.WaitGroup
	waitGroup.Add(workerCount)
	for range workerCount {
		go func() {
			defer waitGroup.Done()
			for index := range jobs {
				if ctx.Err() != nil {
					return
				}
				records, issues := parseSymbolFile(files[index])
				results[index] = symbolParseResult{records: records, issues: issues}
			}
		}()
	}
	for index := range files {
		select {
		case jobs <- index:
		case <-ctx.Done():
			close(jobs)
			waitGroup.Wait()
			return results
		}
	}
	close(jobs)
	waitGroup.Wait()
	return results
}

func ResolveSymbol(index LibraryIndex, libraryID string) (SymbolRecord, bool) {
	if index.Symbols == nil {
		return SymbolRecord{}, false
	}
	record, ok := index.Symbols[libraryID]
	return record, ok
}

func parseSymbolFile(file LibraryFile) ([]SymbolRecord, []reports.Issue) {
	sourcePath := filepath.FromSlash(file.Path)
	info, err := os.Stat(sourcePath)
	if err != nil {
		return nil, []reports.Issue{parseIssue(file.Path, err.Error())}
	}
	if info.Size() > maxSymbolLibraryBytes {
		return nil, []reports.Issue{parseIssue(file.Path, "symbol library exceeds 64 MiB parser limit")}
	}
	data, err := os.ReadFile(sourcePath)
	if err != nil {
		return nil, []reports.Issue{parseIssue(file.Path, err.Error())}
	}
	root, err := sexpr.Parse(data)
	if err != nil {
		return nil, []reports.Issue{parseIssue(file.Path, err.Error())}
	}
	if root.Head() != "kicad_symbol_lib" {
		return nil, []reports.Issue{parseIssue(file.Path, "expected kicad_symbol_lib root, got "+strconv.Quote(root.Head()))}
	}
	var records []SymbolRecord
	var issues []reports.Issue
	for _, child := range root.ChildrenByHead("symbol") {
		if len(child.Children) < 2 {
			issues = append(issues, parseIssue(file.Path, "symbol without name"))
			continue
		}
		name := child.ListValue(1)
		if strings.TrimSpace(name) == "" {
			issues = append(issues, parseIssue(file.Path, "symbol without name"))
			continue
		}
		records = append(records, readLibrarySymbol(file, child, name))
	}
	return records, issues
}

func readLibrarySymbol(file LibraryFile, node sexpr.ParsedNode, name string) SymbolRecord {
	record := SymbolRecord{
		LibraryID:       file.LibraryNickname + ":" + name,
		LibraryNickname: file.LibraryNickname,
		Name:            name,
		Path:            file.Path,
		Properties:      map[string]string{},
		Raw:             strings.Clone(strings.TrimSpace(node.Raw)),
	}
	for _, property := range node.ChildrenByHead("property") {
		if len(property.Children) < 3 {
			continue
		}
		record.Properties[property.ListValue(1)] = property.ListValue(2)
	}
	record.Description = firstSymbolText(node, "ki_description")
	record.Keywords = fields(firstSymbolText(node, "ki_keywords"))
	record.Datasheet = record.Properties["Datasheet"]
	if record.Datasheet == "" {
		record.Datasheet = record.Properties["ki_datasheet"]
	}
	record.FootprintFilter = symbolTextValues(node, "ki_fp_filters")
	record.Pins = collectSymbolPins(node, name, 1, 1)
	record.Units = collectSymbolUnits(record.Pins)
	record.SearchText = buildSymbolSearchText(record)
	return record
}

func collectSymbolPins(node sexpr.ParsedNode, symbolName string, unit int, bodyStyle int) []SymbolPin {
	var pins []SymbolPin
	for _, child := range node.Children {
		switch child.Head() {
		case "pin":
			pins = append(pins, readLibraryPin(child, unit, bodyStyle))
		case "symbol":
			childName := child.ListValue(1)
			childUnit, childBodyStyle := parseSymbolUnitBodyStyle(symbolName, childName)
			if childUnit == 0 {
				childUnit = unit
			}
			if childBodyStyle == 0 {
				childBodyStyle = bodyStyle
			}
			pins = append(pins, collectSymbolPins(child, symbolName, childUnit, childBodyStyle)...)
		}
	}
	return pins
}

func readLibraryPin(node sexpr.ParsedNode, unit int, bodyStyle int) SymbolPin {
	pin := SymbolPin{
		Unit:      unit,
		BodyStyle: bodyStyle,
		Hidden:    hasChildOrAtom(node, "hide"),
	}
	if len(node.Children) > 1 {
		pin.Electrical = node.ListValue(1)
	}
	if at, ok := node.Child("at"); ok {
		if x, ok := at.FloatValue(1); ok {
			pin.Position.X = kicadfiles.MM(x)
		}
		if y, ok := at.FloatValue(2); ok {
			pin.Position.Y = kicadfiles.MM(y)
		}
		if len(at.Children) > 3 {
			pin.Orientation = at.ListValue(3)
		}
	}
	if length, ok := node.Child("length"); ok {
		if value, ok := length.FloatValue(1); ok {
			pin.Length = kicadfiles.MM(value)
		}
	}
	if name, ok := node.Child("name"); ok && len(name.Children) > 1 {
		pin.Name = name.ListValue(1)
	}
	if number, ok := node.Child("number"); ok && len(number.Children) > 1 {
		pin.Number = number.ListValue(1)
	}
	return pin
}

func parseSymbolUnitBodyStyle(parentName string, childName string) (int, int) {
	prefix := parentName + "_"
	if !strings.HasPrefix(childName, prefix) {
		return 0, 0
	}
	parts := strings.Split(strings.TrimPrefix(childName, prefix), "_")
	if len(parts) < 2 {
		return 0, 0
	}
	unit, err := strconv.Atoi(parts[len(parts)-2])
	if err != nil {
		return 0, 0
	}
	bodyStyle, err := strconv.Atoi(parts[len(parts)-1])
	if err != nil {
		return 0, 0
	}
	return unit, bodyStyle
}

func collectSymbolUnits(pins []SymbolPin) []SymbolUnit {
	seen := map[SymbolUnit]struct{}{}
	for _, pin := range pins {
		unit := SymbolUnit{Unit: pin.Unit, BodyStyle: pin.BodyStyle}
		if unit.Unit == 0 {
			unit.Unit = 1
		}
		if unit.BodyStyle == 0 {
			unit.BodyStyle = 1
		}
		seen[unit] = struct{}{}
	}
	units := make([]SymbolUnit, 0, len(seen))
	for unit := range seen {
		units = append(units, unit)
	}
	sort.Slice(units, func(i, j int) bool {
		if units[i].Unit != units[j].Unit {
			return units[i].Unit < units[j].Unit
		}
		return units[i].BodyStyle < units[j].BodyStyle
	})
	return units
}

func firstSymbolText(node sexpr.ParsedNode, head string) string {
	values := symbolTextValues(node, head)
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func symbolTextValues(node sexpr.ParsedNode, head string) []string {
	child, ok := node.Child(head)
	if !ok || len(child.Children) < 2 {
		return nil
	}
	values := make([]string, 0, len(child.Children)-1)
	for _, value := range child.Children[1:] {
		if text := strings.TrimSpace(value.Value()); text != "" {
			values = append(values, text)
		}
	}
	return values
}

func fields(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return strings.Fields(value)
}

func hasChildOrAtom(node sexpr.ParsedNode, value string) bool {
	if _, ok := node.Child(value); ok {
		return true
	}
	for _, child := range node.Children {
		if child.Value() == value {
			return true
		}
	}
	return false
}
