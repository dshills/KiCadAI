package libraryresolver

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"kicadai/internal/kicadfiles"
	"kicadai/internal/kicadfiles/sexpr"
	"kicadai/internal/reports"
)

const maxSymbolLibraryBytes int64 = 64 << 20

const codeSymbolInheritanceCycle reports.Code = "SYMBOL_INHERITANCE_CYCLE"

const (
	SymbolElectricalInput         = "input"
	SymbolElectricalOutput        = "output"
	SymbolElectricalBidirectional = "bidirectional"
	SymbolElectricalTriState      = "tri_state"
	SymbolElectricalPassive       = "passive"
	SymbolElectricalFree          = "free"
	SymbolElectricalUnspecified   = "unspecified"
	SymbolElectricalPowerIn       = "power_in"
	SymbolElectricalPowerOut      = "power_out"
	SymbolElectricalOpenCollector = "open_collector"
	SymbolElectricalOpenEmitter   = "open_emitter"
	SymbolElectricalNoConnect     = "no_connect"
)

var knownSymbolElectricalTypes = map[string]struct{}{
	SymbolElectricalInput:         {},
	SymbolElectricalOutput:        {},
	SymbolElectricalBidirectional: {},
	SymbolElectricalTriState:      {},
	SymbolElectricalPassive:       {},
	SymbolElectricalFree:          {},
	SymbolElectricalUnspecified:   {},
	SymbolElectricalPowerIn:       {},
	SymbolElectricalPowerOut:      {},
	SymbolElectricalOpenCollector: {},
	SymbolElectricalOpenEmitter:   {},
	SymbolElectricalNoConnect:     {},
}

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
	records = resolveCrossFileInheritedSymbols(records)
	for _, record := range records {
		issues = append(issues, record.Diagnostics...)
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
	return parallelMap(ctx, len(files), func(index int) symbolParseResult {
		records, issues := parseSymbolFile(files[index])
		return symbolParseResult{records: records, issues: issues}
	})
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
		if !validSymbolIDPart(file.LibraryNickname) || !validSymbolIDPart(name) {
			issues = append(issues, parseIssue(file.Path, "invalid symbol ID "+strconv.Quote(file.LibraryNickname+":"+name)))
			continue
		}
		records = append(records, readLibrarySymbol(file, child, name))
	}
	records = resolveInheritedSymbols(records)
	return records, issues
}

func resolveCrossFileInheritedSymbols(records map[string]SymbolRecord) map[string]SymbolRecord {
	byLibraryAndName := make(map[string]string, len(records))
	for id, record := range records {
		byLibraryAndName[record.LibraryNickname+"\x00"+record.Name] = id
	}
	resolving := make(map[string]bool, len(records))
	resolved := make(map[string]bool, len(records))
	var resolve func(string) SymbolRecord
	resolve = func(id string) SymbolRecord {
		record := records[id]
		if resolved[id] || strings.TrimSpace(record.Extends) == "" || record.Inherited {
			resolved[id] = true
			return record
		}
		if resolving[id] {
			return record
		}
		baseID, ok := byLibraryAndName[record.LibraryNickname+"\x00"+record.Extends]
		if !ok {
			resolved[id] = true
			return record
		}
		resolving[id] = true
		base := resolve(baseID)
		resolving[id] = false
		if hasCyclicInheritanceDiagnostic(base.Diagnostics) {
			resolved[id] = true
			return record
		}
		if hasUnresolvedBaseDiagnostic(base.Diagnostics) {
			record.Diagnostics = appendSymbolIssueOnce(record.Diagnostics, unresolvedInheritedBaseIssue(record))
			records[id] = record
			resolved[id] = true
			return record
		}
		inherited := inheritSymbolRecord(base, record)
		inherited.Diagnostics = mergeInheritedSymbolDiagnostics(record.Diagnostics, validateParsedSymbol(inherited))
		inherited.SearchText = buildSymbolSearchText(inherited)
		records[id] = inherited
		resolved[id] = true
		return inherited
	}
	for id := range records {
		resolve(id)
	}
	return records
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
	if extends, ok := node.Child("extends"); ok && len(extends.Children) > 1 {
		record.Extends = strings.TrimSpace(extends.ListValue(1))
	}
	record.Description = symbolMetadataText(record.Properties, node, "ki_description", "Description")
	record.Keywords = fields(symbolMetadataText(record.Properties, node, "ki_keywords", "Keywords"))
	record.Datasheet = symbolMetadataText(record.Properties, node, "ki_datasheet", "Datasheet")
	record.FootprintFilter = fields(record.Properties["ki_fp_filters"])
	if len(record.FootprintFilter) == 0 {
		record.FootprintFilter = symbolTextValues(node, "ki_fp_filters")
	}
	record.Pins = collectSymbolPins(node, name, 1, 1)
	record.Graphics = collectSymbolGraphics(file.Path, node, name, 1, 1)
	record.Units = collectSymbolUnits(record.Pins)
	record.PowerSymbol = isPowerSymbol(record)
	record.Diagnostics = validateParsedSymbol(record)
	record.SearchText = buildSymbolSearchText(record)
	return record
}

func resolveInheritedSymbols(records []SymbolRecord) []SymbolRecord {
	byName := make(map[string]int, len(records))
	for index, record := range records {
		byName[record.Name] = index
	}
	resolved := make([]bool, len(records))
	resolving := make([]bool, len(records))
	var resolve func(int) SymbolRecord
	resolve = func(index int) SymbolRecord {
		if resolved[index] {
			return records[index]
		}
		record := records[index]
		if strings.TrimSpace(record.Extends) == "" {
			resolved[index] = true
			return record
		}
		baseIndex, ok := byName[record.Extends]
		if !ok {
			issue := symbolIssue(record, reports.SeverityBlocked, "unresolved base symbol "+strconv.Quote(record.Extends))
			record.Diagnostics = append(record.Diagnostics, issue)
			records[index] = record
			resolved[index] = true
			return record
		}
		if resolving[index] {
			issue := cyclicSymbolInheritanceIssue(record)
			record.Diagnostics = appendSymbolIssueOnce(record.Diagnostics, issue)
			records[index] = record
			resolving[index] = false
			resolved[index] = true
			return record
		}
		resolving[index] = true
		base := resolve(baseIndex)
		resolving[index] = false
		if hasCyclicInheritanceDiagnostic(base.Diagnostics) {
			record.Diagnostics = appendSymbolIssueOnce(record.Diagnostics, cyclicSymbolInheritanceIssue(record))
			records[index] = record
			resolved[index] = true
			return record
		}
		if hasUnresolvedBaseDiagnostic(base.Diagnostics) {
			record.Diagnostics = appendSymbolIssueOnce(record.Diagnostics, unresolvedInheritedBaseIssue(record))
			records[index] = record
			resolved[index] = true
			return record
		}
		record = inheritSymbolRecord(base, record)
		record.Diagnostics = mergeInheritedSymbolDiagnostics(record.Diagnostics, validateParsedSymbol(record))
		record.SearchText = buildSymbolSearchText(record)
		records[index] = record
		resolved[index] = true
		return record
	}
	for index := range records {
		resolve(index)
	}
	return records
}

func symbolMetadataText(properties map[string]string, node sexpr.ParsedNode, legacyHead string, propertyNames ...string) string {
	for _, propertyName := range propertyNames {
		if value := strings.TrimSpace(properties[propertyName]); value != "" {
			return value
		}
	}
	if value := strings.TrimSpace(properties[legacyHead]); value != "" {
		return value
	}
	if value := strings.TrimSpace(firstSymbolText(node, legacyHead)); value != "" {
		return value
	}
	return ""
}

func hasCyclicInheritanceDiagnostic(issues []reports.Issue) bool {
	for _, issue := range issues {
		if issue.Code == codeSymbolInheritanceCycle {
			return true
		}
	}
	return false
}

func hasUnresolvedBaseDiagnostic(issues []reports.Issue) bool {
	for _, issue := range issues {
		if strings.HasPrefix(issue.Message, "unresolved base symbol ") || strings.HasPrefix(issue.Message, "unresolved inherited base symbol ") {
			return true
		}
	}
	return false
}

func unresolvedInheritedBaseIssue(record SymbolRecord) reports.Issue {
	return symbolIssue(record, reports.SeverityBlocked, "unresolved inherited base symbol "+strconv.Quote(record.Extends))
}

func cyclicSymbolInheritanceIssue(record SymbolRecord) reports.Issue {
	issue := symbolIssue(record, reports.SeverityBlocked, "cyclic symbol inheritance involving "+record.LibraryID)
	issue.Code = codeSymbolInheritanceCycle
	return issue
}

func mergeInheritedSymbolDiagnostics(existing []reports.Issue, validation []reports.Issue) []reports.Issue {
	merged := make([]reports.Issue, 0, len(existing)+len(validation))
	for _, issue := range existing {
		if issue.Code == reports.CodeValidationFailed {
			continue
		}
		merged = append(merged, issue)
	}
	return append(merged, validation...)
}

func appendSymbolIssueOnce(issues []reports.Issue, issue reports.Issue) []reports.Issue {
	for _, existing := range issues {
		if existing.Code == issue.Code &&
			existing.Severity == issue.Severity &&
			existing.Path == issue.Path &&
			existing.Message == issue.Message &&
			existing.Suggestion == issue.Suggestion {
			return issues
		}
	}
	return append(issues, issue)
}

func inheritSymbolRecord(base SymbolRecord, child SymbolRecord) SymbolRecord {
	inherited := child
	inherited.Inherited = true
	if raw, ok := mergeInheritedSymbolRaw(base.Raw, child.Raw, child.Name); ok {
		inherited.Raw = raw
	}
	inherited.Properties = mergeSymbolProperties(base.Properties, child.Properties)
	if inherited.Description == "" {
		inherited.Description = base.Description
	}
	if len(inherited.Keywords) == 0 {
		inherited.Keywords = append([]string(nil), base.Keywords...)
	}
	if inherited.Datasheet == "" {
		inherited.Datasheet = base.Datasheet
	}
	if len(inherited.FootprintFilter) == 0 {
		inherited.FootprintFilter = append([]string(nil), base.FootprintFilter...)
	}
	inherited.Pins = append(append([]SymbolPin(nil), base.Pins...), child.Pins...)
	inherited.Graphics = append(append([]SymbolGraphic(nil), base.Graphics...), child.Graphics...)
	inherited.Units = collectSymbolUnits(inherited.Pins)
	inherited.PowerSymbol = isPowerSymbol(inherited)
	return inherited
}

func mergeInheritedSymbolRaw(baseRaw, childRaw, childName string) (string, bool) {
	baseNode, err := sexpr.Parse([]byte(baseRaw))
	if err != nil || !baseNode.IsList {
		return "", false
	}
	childNode, err := sexpr.Parse([]byte(childRaw))
	if err != nil || !childNode.IsList {
		return "", false
	}
	merged := mergeInheritedSymbolList(baseNode.Node().(sexpr.List), childNode.Node().(sexpr.List), true)
	if len(merged) < 2 {
		return "", false
	}
	merged[1] = sexpr.S(childName)
	rendered, err := sexpr.Format(merged)
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(rendered), true
}

func mergeInheritedSymbolList(base, child sexpr.List, root bool) sexpr.List {
	merged := append(sexpr.List(nil), base...)
	if root && len(merged) > 1 {
		merged[1] = child[1]
	}
	for _, childNode := range child {
		if head := sexprNodeHead(childNode); head == "" || (root && head == "symbol") || head == "extends" {
			continue
		}
		if index := inheritedNodeIndex(merged, childNode); index >= 0 {
			if baseList, ok := merged[index].(sexpr.List); ok {
				if childList, ok := childNode.(sexpr.List); ok && sexprNodeHead(childNode) == "symbol" {
					merged[index] = mergeInheritedSymbolList(baseList, childList, false)
					continue
				}
			}
			merged[index] = childNode
			continue
		}
		merged = append(merged, childNode)
	}
	return merged
}

func inheritedNodeIndex(nodes sexpr.List, candidate sexpr.Node) int {
	head := sexprNodeHead(candidate)
	if head == "" || head == "pin" || head == "rectangle" || head == "circle" || head == "arc" || head == "polyline" || head == "text" {
		return -1
	}
	key := sexprNodeKey(candidate)
	for index, node := range nodes {
		if sexprNodeHead(node) != head || sexprNodeKey(node) != key {
			continue
		}
		return index
	}
	return -1
}

func sexprNodeHead(node sexpr.Node) string {
	list, ok := node.(sexpr.List)
	if !ok || len(list) == 0 {
		return ""
	}
	atom, ok := list[0].(sexpr.Atom)
	if !ok {
		return ""
	}
	return string(atom)
}

func sexprNodeKey(node sexpr.Node) string {
	list, ok := node.(sexpr.List)
	if !ok || len(list) < 2 {
		return ""
	}
	switch value := list[1].(type) {
	case sexpr.Atom:
		return string(value)
	case sexpr.String:
		return string(value)
	default:
		return ""
	}
}

func mergeSymbolProperties(base map[string]string, child map[string]string) map[string]string {
	if len(base) == 0 && len(child) == 0 {
		return nil
	}
	merged := make(map[string]string, len(base)+len(child))
	for key, value := range base {
		merged[key] = value
	}
	for key, value := range child {
		merged[key] = value
	}
	return merged
}

func collectSymbolPins(node sexpr.ParsedNode, symbolName string, unit int, bodyStyle int) []SymbolPin {
	var pins []SymbolPin
	for _, child := range node.Children {
		switch child.Head() {
		case "pin":
			pins = append(pins, readLibraryPin(child, unit, bodyStyle))
		case "symbol":
			if len(child.Children) < 2 {
				continue
			}
			childName := child.ListValue(1)
			childUnit, childBodyStyle, ok := parseSymbolUnitBodyStyle(symbolName, childName)
			if !ok {
				childUnit = unit
				childBodyStyle = bodyStyle
			}
			pins = append(pins, collectSymbolPins(child, symbolName, childUnit, childBodyStyle)...)
		}
	}
	return pins
}

func collectSymbolGraphics(path string, node sexpr.ParsedNode, symbolName string, unit int, bodyStyle int) []SymbolGraphic {
	var graphics []SymbolGraphic
	for _, child := range node.Children {
		switch child.Head() {
		case "rectangle", "circle", "arc", "polyline", "text":
			if graphic, ok := readLibrarySymbolGraphic(path, child, unit, bodyStyle); ok {
				graphics = append(graphics, graphic)
			}
		case "symbol":
			if len(child.Children) < 2 {
				continue
			}
			childName := child.ListValue(1)
			childUnit, childBodyStyle, ok := parseSymbolUnitBodyStyle(symbolName, childName)
			if !ok {
				childUnit = unit
				childBodyStyle = bodyStyle
			}
			graphics = append(graphics, collectSymbolGraphics(path, child, symbolName, childUnit, childBodyStyle)...)
		}
	}
	return graphics
}

func readLibrarySymbolGraphic(path string, node sexpr.ParsedNode, unit int, bodyStyle int) (SymbolGraphic, bool) {
	bounds := newBounds()
	switch node.Head() {
	case "rectangle":
		bounds.includeNamedPoint(node, "start")
		bounds.includeNamedPoint(node, "end")
	case "circle":
		center, centerOK := readNamedPointOK(node, "center")
		radius, radiusOK := firstNumericMMFromChild(node, "radius")
		if centerOK && radiusOK {
			bounds.includePoint(kicadfiles.Point{X: center.X - radius, Y: center.Y - radius})
			bounds.includePoint(kicadfiles.Point{X: center.X + radius, Y: center.Y + radius})
		}
	case "arc":
		start, mid, end, ok := readLibraryArcPointsOK(node)
		if ok {
			bounds.includeArc(start, mid, end)
		}
	case "polyline":
		points, _ := readPolyPoints(path, node)
		for _, point := range points {
			bounds.includePoint(point)
		}
	case "text":
		bounds.includeNamedPoint(node, "at")
	default:
		return SymbolGraphic{}, false
	}
	if !bounds.initialized {
		return SymbolGraphic{}, false
	}
	return SymbolGraphic{Kind: node.Head(), Unit: unit, BodyStyle: bodyStyle, Bounds: bounds.box()}, true
}

func firstNumericMMFromChild(node sexpr.ParsedNode, name string) (kicadfiles.IU, bool) {
	child, ok := node.Child(name)
	if !ok {
		return 0, false
	}
	return firstNumericMM(child, 1)
}

func readLibraryPin(node sexpr.ParsedNode, unit int, bodyStyle int) SymbolPin {
	pin := SymbolPin{
		Unit:      unit,
		BodyStyle: bodyStyle,
		Common:    unit == 0,
		Hidden:    hasChildOrAtom(node, "hide"),
	}
	if len(node.Children) > 1 {
		pin.ElectricalType = strings.ToLower(strings.TrimSpace(node.ListValue(1)))
		pin.Electrical = pin.ElectricalType
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

func parseSymbolUnitBodyStyle(parentName string, childName string) (int, int, bool) {
	prefix := parentName + "_"
	if !strings.HasPrefix(childName, prefix) {
		return 0, 0, false
	}
	parts := strings.Split(strings.TrimPrefix(childName, prefix), "_")
	if len(parts) < 2 {
		return 0, 0, false
	}
	unit, err := strconv.Atoi(parts[len(parts)-2])
	if err != nil {
		return 0, 0, false
	}
	bodyStyle, err := strconv.Atoi(parts[len(parts)-1])
	if err != nil {
		return 0, 0, false
	}
	return unit, bodyStyle, true
}

func collectSymbolUnits(pins []SymbolPin) []SymbolUnit {
	type unitKey struct {
		unit      int
		bodyStyle int
	}
	byUnit := map[unitKey]*SymbolUnit{}
	var commonPinIndexes []int
	for index, pin := range pins {
		if pin.Unit == 0 {
			commonPinIndexes = append(commonPinIndexes, index)
			continue
		}
		unit := pin.Unit
		bodyStyle := pin.BodyStyle
		key := unitKey{unit: unit, bodyStyle: bodyStyle}
		entry := byUnit[key]
		if entry == nil {
			entry = &SymbolUnit{Unit: unit, BodyStyle: bodyStyle}
			byUnit[key] = entry
		}
		entry.PinIndexes = append(entry.PinIndexes, index)
	}
	if len(byUnit) == 0 && len(commonPinIndexes) > 0 {
		byUnit[unitKey{}] = &SymbolUnit{}
	}
	for _, unit := range byUnit {
		unit.CommonPinIndexes = append(unit.CommonPinIndexes, commonPinIndexes...)
	}
	units := make([]SymbolUnit, 0, len(byUnit))
	for _, unit := range byUnit {
		units = append(units, *unit)
	}
	sort.Slice(units, func(i, j int) bool {
		if units[i].Unit != units[j].Unit {
			return units[i].Unit < units[j].Unit
		}
		return units[i].BodyStyle < units[j].BodyStyle
	})
	return units
}

func validSymbolIDPart(value string) bool {
	return value != "" && strings.TrimSpace(value) == value && !strings.Contains(value, ":")
}

func validateParsedSymbol(record SymbolRecord) []reports.Issue {
	var issues []reports.Issue
	type pinKey struct {
		unit      int
		bodyStyle int
		number    string
	}
	duplicatePins := map[pinKey]int{}
	for index, pin := range record.Pins {
		if _, ok := knownSymbolElectricalTypes[pin.ElectricalType]; pin.ElectricalType != "" && !ok {
			issues = append(issues, symbolIssue(record, reports.SeverityWarning, "unknown electrical type "+strconv.Quote(pin.ElectricalType)+" on pin "+pin.Number))
		}
		if strings.TrimSpace(pin.Number) == "" {
			continue
		}
		key := pinKey{unit: pin.Unit, bodyStyle: pin.BodyStyle, number: pin.Number}
		if first, exists := duplicatePins[key]; exists {
			if !allowsStackedSymbolPins(record) {
				issues = append(issues, symbolIssue(record, reports.SeverityError, "duplicate pin number "+pin.Number+" in unit "+strconv.Itoa(pin.Unit)+" body style "+strconv.Itoa(pin.BodyStyle)+" at pin indexes "+strconv.Itoa(first)+" and "+strconv.Itoa(index)))
			}
			continue
		}
		duplicatePins[key] = index
	}
	issues = append(issues, SymbolConnectivityAcceptanceIssues(record)...)
	return issues
}

func allowsStackedSymbolPins(record SymbolRecord) bool {
	hasStacked := false
	hasPins := false
	for _, keyword := range record.Keywords {
		switch strings.ToLower(strings.TrimSpace(keyword)) {
		case "stacked":
			hasStacked = true
		case "pin", "pins":
			hasPins = true
		}
	}
	return hasStacked && hasPins
}

func isPowerSymbol(record SymbolRecord) bool {
	if strings.EqualFold(record.LibraryNickname, "power") {
		return true
	}
	for key, value := range record.Properties {
		if strings.EqualFold(key, "Reference") && strings.HasPrefix(strings.TrimSpace(value), "#") {
			return true
		}
	}
	return false
}

func SymbolConnectivityAcceptanceIssues(record SymbolRecord) []reports.Issue {
	var issues []reports.Issue
	for _, pin := range record.Pins {
		if pin.Hidden && (pin.ElectricalType == SymbolElectricalPowerIn || pin.ElectricalType == SymbolElectricalPowerOut) {
			issues = append(issues, symbolIssue(record, reports.SeverityBlocked, "hidden power pin "+pin.Number+" requires explicit connectivity policy"))
		}
	}
	return issues
}

func symbolIssue(record SymbolRecord, severity reports.Severity, message string) reports.Issue {
	return reports.Issue{
		Code:     reports.CodeValidationFailed,
		Severity: severity,
		Path:     "library.symbol." + record.LibraryID,
		Message:  message,
	}
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
