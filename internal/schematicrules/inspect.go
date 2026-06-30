package schematicrules

import (
	"sort"
	"strconv"
	"strings"

	"kicadai/internal/kicadfiles"
	"kicadai/internal/kicadfiles/schematic"
	"kicadai/internal/reports"
)

// Inspect evaluates file-local schematic electrical rules that do not require
// block or component catalog context.
func Inspect(file schematic.SchematicFile, opts Options) Report {
	wireIndex := newWireSegmentIndex(file.Wires)
	pinAnchors, pinAnchorCounts := symbolPinAnchorMetadata(file)
	inspector := fileInspector{
		file:                file,
		opts:                opts,
		pinAnchors:          pinAnchors,
		pinAnchorCounts:     pinAnchorCounts,
		noConnectCounts:     noConnectPositionCounts(file),
		pinConnectionCounts: pinConnectionCounts(file, wireIndex, pinAnchors),
		labelAnchorCounts:   labelAnchorCounts(file, wireIndex),
		connectedComponents: wireConnectedComponents(file, wireIndex),
		pointLabels:         map[kicadfiles.Point][]indexedLabel{},
		acceptedExternal:    acceptedExternalRailSet(opts),
	}
	inspector.inspectReferences()
	inspector.inspectLabels()
	inspector.inspectNoConnects()
	inspector.inspectPinIntents()
	report := Report{
		CheckedSymbols:      len(file.Symbols),
		CheckedNets:         len(inspector.connectedComponents),
		CheckedRequiredPins: countRequiredPinIntents(opts.PinIntents),
		Findings:            inspector.findings,
	}
	return NewReport(report)
}

type fileInspector struct {
	file                schematic.SchematicFile
	opts                Options
	pinAnchors          map[kicadfiles.Point]struct{}
	pinAnchorCounts     map[kicadfiles.Point]int
	noConnectCounts     map[kicadfiles.Point]int
	pinConnectionCounts map[kicadfiles.Point]int
	labelAnchorCounts   map[kicadfiles.Point]int
	pointLabels         map[kicadfiles.Point][]indexedLabel
	connectedComponents [][]kicadfiles.Point
	acceptedExternal    map[string]struct{}
	findings            []Finding
}

type indexedLabel struct {
	label schematic.Label
	path  string
}

func (inspector *fileInspector) inspectReferences() {
	seen := map[string][]string{}
	for index, symbol := range inspector.file.Symbols {
		reference := strings.TrimSpace(symbol.Reference)
		path := "symbols[" + strconv.Itoa(index) + "].reference"
		if reference == "" {
			inspector.add(Finding{
				RuleID:   RuleReferenceEmpty,
				Severity: reports.SeverityError,
				Category: CategoryReference,
				Path:     path,
				Message:  "schematic symbol reference is empty",
				Repair:   "assign a stable reference before schematic-to-PCB transfer",
			})
			continue
		}
		if isPowerReference(reference) || isUnannotatedReference(reference) {
			continue
		}
		seen[strings.ToLower(reference)] = append(seen[strings.ToLower(reference)], path)
	}
	for reference, paths := range seen {
		if len(paths) < 2 {
			continue
		}
		for _, path := range paths {
			inspector.add(Finding{
				RuleID:    RuleReferenceDuplicate,
				Severity:  reports.SeverityBlocked,
				Category:  CategoryReference,
				Path:      path,
				Reference: reference,
				Message:   "duplicate non-power schematic reference " + reference,
				Repair:    "renumber generated schematic references before footprint assignment and BOM generation",
			})
		}
	}
}

func (inspector *fileInspector) inspectLabels() {
	for index, label := range allSchematicLabels(inspector.file) {
		text := strings.TrimSpace(label.Text)
		path := "labels[" + strconv.Itoa(index) + "]"
		if text == "" {
			inspector.add(Finding{
				RuleID:   RuleLabelEmpty,
				Severity: reports.SeverityError,
				Category: CategoryNet,
				Path:     path + ".text",
				Message:  "schematic label text is empty",
				Repair:   "remove the label or assign the intended net name",
			})
		}
		if inspector.labelAnchorCounts[label.Position] == 0 {
			inspector.add(Finding{
				RuleID:   RuleLabelFloating,
				Severity: reports.SeverityError,
				Category: CategoryNet,
				Path:     path + ".position",
				Net:      text,
				Message:  "schematic label is not attached to a known wire, pin, or sheet pin anchor",
				Repair:   "move the label onto the intended net anchor or route a wire to it",
			})
		}
		inspector.pointLabels[label.Position] = append(inspector.pointLabels[label.Position], indexedLabel{label: label, path: path})
	}
	inspector.inspectLabelNormalizationCollisions()
	inspector.inspectConnectedLabelConflicts()
}

func (inspector *fileInspector) inspectLabelNormalizationCollisions() {
	byNormalized := map[string]map[string][]string{}
	for index, label := range allSchematicLabels(inspector.file) {
		raw := strings.TrimSpace(label.Text)
		if raw == "" {
			continue
		}
		normalized := normalizeLabelText(raw)
		if byNormalized[normalized] == nil {
			byNormalized[normalized] = map[string][]string{}
		}
		byNormalized[normalized][raw] = append(byNormalized[normalized][raw], "labels["+strconv.Itoa(index)+"]")
	}
	for normalized, rawPaths := range byNormalized {
		if len(rawPaths) < 2 {
			continue
		}
		raws := make([]string, 0, len(rawPaths))
		var paths []string
		for raw, labelPaths := range rawPaths {
			raws = append(raws, raw)
			paths = append(paths, labelPaths...)
		}
		sort.Strings(raws)
		sort.Strings(paths)
		for _, path := range paths {
			inspector.add(Finding{
				RuleID:   RuleLabelNormalizationCollision,
				Severity: reports.SeverityWarning,
				Category: CategoryNet,
				Path:     path,
				Net:      normalized,
				Message:  "schematic labels differ only by case or whitespace: " + strings.Join(raws, ", "),
				Repair:   "normalize label spelling or declare explicit aliases before relying on shared-net behavior",
			})
		}
	}
}

func (inspector *fileInspector) inspectConnectedLabelConflicts() {
	for _, points := range inspector.connectedComponents {
		rawSet := map[string]struct{}{}
		pathSet := map[string]struct{}{}
		for _, point := range points {
			for _, indexed := range inspector.pointLabels[point] {
				text := strings.TrimSpace(indexed.label.Text)
				if text != "" {
					rawSet[text] = struct{}{}
					pathSet[indexed.path] = struct{}{}
				}
			}
		}
		if len(rawSet) < 2 {
			continue
		}
		labels := make([]string, 0, len(rawSet))
		for label := range rawSet {
			labels = append(labels, label)
		}
		sort.Strings(labels)
		paths := make([]string, 0, len(pathSet))
		for path := range pathSet {
			paths = append(paths, path)
		}
		sort.Strings(paths)
		joinedLabels := strings.Join(labels, ", ")
		for _, path := range paths {
			inspector.add(Finding{
				RuleID:   RuleLabelConflict,
				Severity: reports.SeverityError,
				Category: CategoryNet,
				Path:     path,
				Net:      strings.Join(labels, ","),
				Message:  "connected schematic net has conflicting labels: " + joinedLabels,
				Repair:   "split the nets or keep a single intentional net label on the connected segment",
			})
		}
	}
}

func (inspector *fileInspector) inspectNoConnects() {
	for index, noConnect := range inspector.file.NoConnects {
		path := "no_connects[" + strconv.Itoa(index) + "].position"
		if _, ok := inspector.pinAnchors[noConnect.Position]; !ok {
			inspector.add(Finding{
				RuleID:   RulePinNoConnectMissing,
				Severity: reports.SeverityError,
				Category: CategoryPin,
				Path:     path,
				Message:  "no-connect marker is not placed on a known symbol pin anchor",
				Repair:   "move the no-connect marker to the intentionally unused symbol pin",
			})
			continue
		}
		if inspector.pinConnected(noConnect.Position) {
			inspector.add(Finding{
				RuleID:   RulePinNoConnectViolated,
				Severity: reports.SeverityError,
				Category: CategoryPin,
				Path:     path,
				Message:  "no-connect marker is placed on a pin that also has schematic connectivity",
				Repair:   "remove the no-connect marker or disconnect the pin if it is intentionally unused",
			})
		}
	}
}

func (inspector *fileInspector) inspectPinIntents() {
	if len(inspector.opts.PinIntents) == 0 {
		if inspector.opts.RequireConfidence && len(inspector.file.Symbols) > 0 {
			inspector.add(Finding{
				RuleID:   RulePinMetadataMissing,
				Severity: reports.SeverityBlocked,
				Category: CategoryPin,
				Message:  "schematic pin intent metadata is required but missing",
				Repair:   "provide block or resolver pin intent before claiming schematic electrical confidence",
			})
		}
		return
	}
	for index, intent := range inspector.opts.PinIntents {
		path := "pin_intents[" + strconv.Itoa(index) + "]"
		point := pointFromIntent(intent.Position)
		if _, ok := inspector.pinAnchors[point]; !ok {
			inspector.add(Finding{
				RuleID:    RulePinMetadataMissing,
				Severity:  metadataMissingSeverity(inspector.opts),
				Category:  CategoryPin,
				Path:      path + ".position",
				Reference: strings.TrimSpace(intent.Reference),
				Pin:       strings.TrimSpace(intent.Pin),
				Net:       strings.TrimSpace(intent.Net),
				Message:   "pin intent does not match a known schematic symbol pin anchor",
				Repair:    "derive pin intent from resolver-backed symbol metadata or update the generated pin anchor",
			})
			continue
		}
		hasNoConnect := inspector.noConnectCounts[point] > 0
		connected := inspector.pinConnected(point)
		switch intent.Kind {
		case PinIntentRequired:
			if hasNoConnect {
				inspector.add(pinIntentFinding(RulePinNoConnectOnRequired, reports.SeverityError, path, intent, "required pin has an explicit no-connect marker", "remove the no-connect marker or change the pin intent if it is truly unused"))
			} else if !connected {
				inspector.add(pinIntentFinding(RulePinRequiredOpen, reports.SeverityBlocked, path, intent, "required pin is not connected", "connect the required pin to its intended net or represent it as an accepted external port"))
			}
		case PinIntentOptional:
			if !connected && !hasNoConnect {
				inspector.add(pinIntentFinding(RulePinOptionalOpen, reports.SeverityWarning, path, intent, "optional pin is open without no-connect evidence", "add a no-connect marker or document why this optional pin may float"))
			}
		case PinIntentNoConnect:
			if connected {
				inspector.add(pinIntentFinding(RulePinNoConnectViolated, reports.SeverityError, path, intent, "pin intended as no-connect is connected", "remove the connection or change the pin intent"))
			}
			if !hasNoConnect {
				inspector.add(pinIntentFinding(RulePinNoConnectMissing, reports.SeverityError, path, intent, "pin intended as no-connect lacks a no-connect marker", "place a no-connect marker on the intentionally unused pin"))
			}
		case PinIntentExternal:
			if !connected && !intent.AcceptedExternal && !inspector.externalNetAccepted(intent.Net) {
				inspector.add(pinIntentFinding(RulePinRequiredOpen, reports.SeverityBlocked, path, intent, "external pin is open without accepted external policy", "attach the external pin to a connector, label, sheet pin, or explicit external-driver policy"))
			}
		default:
			inspector.add(pinIntentFinding(RulePinMetadataMissing, metadataMissingSeverity(inspector.opts), path, intent, "pin intent kind is unknown", "use required, optional, external, or no_connect pin intent"))
		}
	}
}

func (inspector *fileInspector) pinConnected(point kicadfiles.Point) bool {
	return inspector.pinConnectionCounts[point] > 0 || inspector.pinAnchorCounts[point] > 1
}

func pinIntentFinding(rule RuleID, severity reports.Severity, path string, intent PinIntent, message string, repair string) Finding {
	return Finding{
		RuleID:    rule,
		Severity:  severity,
		Category:  CategoryPin,
		Path:      path,
		Reference: strings.TrimSpace(intent.Reference),
		Pin:       strings.TrimSpace(intent.Pin),
		Net:       strings.TrimSpace(intent.Net),
		Message:   message,
		Repair:    repair,
	}
}

func metadataMissingSeverity(opts Options) reports.Severity {
	if opts.RequireConfidence || acceptanceRequiresConfidence(opts.Acceptance) {
		return reports.SeverityBlocked
	}
	return reports.SeverityWarning
}

func acceptanceRequiresConfidence(acceptance Acceptance) bool {
	switch acceptance {
	case AcceptanceERCDRC, AcceptanceFabricationCandidate:
		return true
	default:
		return false
	}
}

func (inspector *fileInspector) externalNetAccepted(net string) bool {
	net = normalizeLabelText(net)
	if net == "" {
		return false
	}
	_, ok := inspector.acceptedExternal[net]
	return ok
}

func acceptedExternalRailSet(opts Options) map[string]struct{} {
	accepted := map[string]struct{}{}
	for _, rail := range opts.AcceptedExternalRails {
		normalized := normalizeLabelText(rail)
		if normalized != "" {
			accepted[normalized] = struct{}{}
		}
	}
	return accepted
}

func noConnectPositionCounts(file schematic.SchematicFile) map[kicadfiles.Point]int {
	positions := map[kicadfiles.Point]int{}
	for _, noConnect := range file.NoConnects {
		positions[noConnect.Position]++
	}
	return positions
}

func pointFromIntent(point Point) kicadfiles.Point {
	return kicadfiles.Point{X: iuFromIntentCoordinate(point.X), Y: iuFromIntentCoordinate(point.Y)}
}

func iuFromIntentCoordinate(value int64) kicadfiles.IU {
	// kicadfiles.IU is int64 in this repository; keep the conversion in one
	// place so any future IU representation change has a single audit point.
	return kicadfiles.IU(value)
}

func countRequiredPinIntents(intents []PinIntent) int {
	count := 0
	for _, intent := range intents {
		if intent.Kind == PinIntentRequired || intent.Kind == PinIntentExternal {
			count++
		}
	}
	return count
}

func normalizeLabelText(text string) string {
	return strings.ToLower(strings.Join(strings.Fields(text), " "))
}

func allSchematicLabels(file schematic.SchematicFile) []schematic.Label {
	// The schematic model stores local, global, hierarchical, and directive
	// labels in one slice. The KiCad label node is represented by Label.Kind.
	return file.Labels
}

func (inspector *fileInspector) add(finding Finding) {
	inspector.findings = append(inspector.findings, finding)
}

func isPowerReference(reference string) bool {
	return strings.HasPrefix(reference, "#")
}

func isUnannotatedReference(reference string) bool {
	return strings.HasSuffix(reference, "?")
}

func symbolPinAnchorMetadata(file schematic.SchematicFile) (map[kicadfiles.Point]struct{}, map[kicadfiles.Point]int) {
	anchors := map[kicadfiles.Point]struct{}{}
	counts := map[kicadfiles.Point]int{}
	for _, symbol := range file.Symbols {
		for _, point := range symbol.PinAnchors {
			anchors[point] = struct{}{}
			counts[point]++
		}
	}
	return anchors, counts
}

func pinConnectionCounts(file schematic.SchematicFile, wireIndex wireSegmentIndex, pinAnchors map[kicadfiles.Point]struct{}) map[kicadfiles.Point]int {
	counts := map[kicadfiles.Point]int{}
	candidatePins := map[kicadfiles.Point]struct{}{}
	for point := range pinAnchors {
		candidatePins[point] = struct{}{}
	}
	for _, noConnect := range file.NoConnects {
		candidatePins[noConnect.Position] = struct{}{}
	}
	for point := range candidatePins {
		if wireIndex.contains(point) {
			counts[point]++
		}
	}
	for _, label := range allSchematicLabels(file) {
		if _, ok := candidatePins[label.Position]; ok {
			counts[label.Position]++
		}
	}
	for _, sheet := range file.Sheets {
		for _, pin := range sheet.Pins {
			if _, ok := candidatePins[pin.Position]; ok {
				counts[pin.Position]++
			}
		}
	}
	return counts
}

func labelAnchorCounts(file schematic.SchematicFile, wireIndex wireSegmentIndex) map[kicadfiles.Point]int {
	anchors := map[kicadfiles.Point]int{}
	seen := map[kicadfiles.Point]struct{}{}
	for _, wire := range file.Wires {
		clear(seen)
		for _, point := range wire.Points {
			seen[point] = struct{}{}
		}
		for point := range seen {
			anchors[point]++
		}
	}
	for _, symbol := range file.Symbols {
		for _, point := range symbol.PinAnchors {
			anchors[point]++
		}
	}
	for _, sheet := range file.Sheets {
		for _, pin := range sheet.Pins {
			anchors[pin.Position]++
		}
	}
	for _, label := range allSchematicLabels(file) {
		if wireIndex.contains(label.Position) {
			anchors[label.Position]++
		}
	}
	return anchors
}

func wireConnectedComponents(file schematic.SchematicFile, wireIndex wireSegmentIndex) [][]kicadfiles.Point {
	parent := map[kicadfiles.Point]kicadfiles.Point{}
	find := func(point kicadfiles.Point) kicadfiles.Point {
		root, ok := parent[point]
		if !ok {
			parent[point] = point
			return point
		}
		for root != parent[root] {
			root = parent[root]
		}
		for point != root {
			next := parent[point]
			parent[point] = root
			point = next
		}
		return root
	}
	union := func(a, b kicadfiles.Point) {
		ra := find(a)
		rb := find(b)
		if ra != rb {
			parent[rb] = ra
		}
	}
	for _, label := range allSchematicLabels(file) {
		find(label.Position)
	}
	for _, junction := range file.Junctions {
		find(junction.Position)
	}
	for _, noConnect := range file.NoConnects {
		find(noConnect.Position)
	}
	for _, symbol := range file.Symbols {
		for _, point := range symbol.PinAnchors {
			find(point)
		}
	}
	for _, sheet := range file.Sheets {
		for _, pin := range sheet.Pins {
			find(pin.Position)
		}
	}
	for _, wire := range file.Wires {
		for _, point := range wire.Points {
			find(point)
		}
		for i := 1; i < len(wire.Points); i++ {
			union(wire.Points[i-1], wire.Points[i])
		}
	}
	for point := range parent {
		for _, segment := range wireIndex.segmentsContaining(point) {
			union(point, segment.a)
			union(point, segment.b)
		}
	}
	componentsByRoot := map[kicadfiles.Point][]kicadfiles.Point{}
	for point := range parent {
		root := find(point)
		componentsByRoot[root] = append(componentsByRoot[root], point)
	}
	components := make([][]kicadfiles.Point, 0, len(componentsByRoot))
	for _, points := range componentsByRoot {
		components = append(components, points)
	}
	return components
}

type wireSegment struct {
	a kicadfiles.Point
	b kicadfiles.Point
}

type wireSegmentIndex struct {
	horizontal map[kicadfiles.IU][]wireSegment
	vertical   map[kicadfiles.IU][]wireSegment
	diagonal   []wireSegment
}

func newWireSegmentIndex(wires []schematic.Wire) wireSegmentIndex {
	index := wireSegmentIndex{
		horizontal: map[kicadfiles.IU][]wireSegment{},
		vertical:   map[kicadfiles.IU][]wireSegment{},
	}
	for _, wire := range wires {
		for i := 1; i < len(wire.Points); i++ {
			segment := wireSegment{a: wire.Points[i-1], b: wire.Points[i]}
			switch {
			case segment.a.Y == segment.b.Y:
				index.horizontal[segment.a.Y] = append(index.horizontal[segment.a.Y], segment)
			case segment.a.X == segment.b.X:
				index.vertical[segment.a.X] = append(index.vertical[segment.a.X], segment)
			default:
				index.diagonal = append(index.diagonal, segment)
			}
		}
	}
	return index
}

func (index wireSegmentIndex) contains(point kicadfiles.Point) bool {
	return len(index.segmentsContaining(point)) > 0
}

func (index wireSegmentIndex) segmentsContaining(point kicadfiles.Point) []wireSegment {
	var segments []wireSegment
	for _, segment := range index.horizontal[point.Y] {
		if pointOnSegment(point, segment.a, segment.b) {
			segments = append(segments, segment)
		}
	}
	for _, segment := range index.vertical[point.X] {
		if pointOnSegment(point, segment.a, segment.b) {
			segments = append(segments, segment)
		}
	}
	for _, segment := range index.diagonal {
		if pointOnSegment(point, segment.a, segment.b) {
			segments = append(segments, segment)
		}
	}
	return segments
}

func pointOnSegment(point, a, b kicadfiles.Point) bool {
	if !pointInSegmentBounds(point, a, b) {
		return false
	}
	dxSegment := b.X - a.X
	dySegment := b.Y - a.Y
	dxPoint := point.X - a.X
	dyPoint := point.Y - a.Y
	return dxPoint*dySegment == dyPoint*dxSegment
}

func pointInSegmentBounds(point, a, b kicadfiles.Point) bool {
	return betweenInclusive(point.X, a.X, b.X) && betweenInclusive(point.Y, a.Y, b.Y)
}

func betweenInclusive(value, a, b kicadfiles.IU) bool {
	if a > b {
		a, b = b, a
	}
	return value >= a && value <= b
}
