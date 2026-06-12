package inspect

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	pcbfiles "kicadai/internal/kicadfiles/pcb"
	schematicfiles "kicadai/internal/kicadfiles/schematic"
	"kicadai/internal/manifest"
	"kicadai/internal/reports"
)

const inspectSampleLimit = 20

func Project(path string) (ProjectSummary, error) {
	if strings.TrimSpace(path) == "" {
		return ProjectSummary{}, fmt.Errorf("project path required")
	}
	info, err := os.Stat(path)
	if err != nil {
		return ProjectSummary{}, err
	}
	if !info.IsDir() {
		return ProjectSummary{}, fmt.Errorf("project path must be a directory")
	}
	root, err := filepath.Abs(path)
	if err != nil {
		return ProjectSummary{}, err
	}
	name := filepath.Base(root)
	projectPath := filepath.Join(root, name+".kicad_pro")
	if discoveredProject, ok, err := discoverProjectFile(root); err != nil {
		return ProjectSummary{}, err
	} else if ok {
		projectPath = discoveredProject
		name = strings.TrimSuffix(filepath.Base(projectPath), ".kicad_pro")
	}
	summary := ProjectSummary{
		Root:   root,
		Name:   name,
		Issues: []reports.Issue{},
	}
	_, manifestStatus, _ := manifest.Read(root)
	summary.Manifest = manifestStatus
	schematicPath := filepath.Join(root, name+".kicad_sch")
	pcbPath := filepath.Join(root, name+".kicad_pcb")
	schematicExists := false
	pcbExists := false
	for _, file := range []FileSummary{
		{Kind: "project", Path: projectPath},
		{Kind: "schematic", Path: schematicPath},
		{Kind: "pcb", Path: pcbPath},
	} {
		if _, err := os.Stat(file.Path); err == nil {
			file.Exists = true
			switch file.Kind {
			case "schematic":
				schematicExists = true
			case "pcb":
				pcbExists = true
			}
		} else if os.IsNotExist(err) {
			summary.Issues = append(summary.Issues, reports.Issue{
				Code:     reports.CodeMissingFile,
				Severity: reports.SeverityWarning,
				Path:     file.Kind,
				Message:  file.Kind + " file not found",
			})
		} else {
			summary.Issues = append(summary.Issues, issueFromError(err, file.Kind))
		}
		summary.Files = append(summary.Files, file)
	}
	if schematicExists {
		schematicSummary, err := Schematic(schematicPath)
		if err != nil {
			summary.Issues = append(summary.Issues, issueFromError(err, "schematic"))
		} else {
			summary.Schematic = &schematicSummary
		}
	}
	if pcbExists {
		pcbSummary, err := PCB(pcbPath)
		if err != nil {
			summary.Issues = append(summary.Issues, issueFromError(err, "pcb"))
		} else {
			summary.PCB = &pcbSummary
			summary.Unsupported = pcbSummary.Unsupported
			summary.PreservationOnly = pcbSummary.PreservationOnly
		}
	}
	return summary, nil
}

func PCB(path string) (PCBSummary, error) {
	if strings.TrimSpace(path) == "" {
		return PCBSummary{}, fmt.Errorf("pcb path required")
	}
	board, err := pcbfiles.ReadFile(path)
	if err != nil {
		return PCBSummary{}, err
	}
	layerUsage := map[string]int{}
	unsupported := pcbUnsupported(board.Preserved)
	preservationOnly := []UnsupportedNode{}
	objectCounts := map[string]int{
		"footprint": len(board.Footprints),
		"segment":   len(board.Tracks) + len(board.TrackArcs),
		"via":       len(board.Vias),
		"zone":      len(board.Zones),
		"drawing":   len(board.Drawings),
		"dimension": len(board.Dimensions),
	}
	padCount := 0
	for _, footprint := range board.Footprints {
		padCount += len(footprint.Pads)
	}
	outline := false
	for _, drawing := range board.Drawings {
		if drawing.Layer != "" {
			layerUsage[string(drawing.Layer)]++
		}
		if drawing.Layer == "Edge.Cuts" {
			outline = true
		}
	}
	nets, netsTruncated := boundedStrings(boardNetNames(board), inspectSampleLimit)
	footprints, footprintsTruncated := boundedStrings(boardFootprintRefs(board), inspectSampleLimit)
	summary := PCBSummary{
		Path:             path,
		FilesScanned:     1,
		NetCount:         len(board.Nets),
		FootprintCount:   len(board.Footprints),
		PadCount:         padCount,
		TrackCount:       len(board.Tracks) + len(board.TrackArcs),
		ViaCount:         len(board.Vias),
		ZoneCount:        len(board.Zones),
		DrawingCount:     len(board.Drawings),
		DimensionCount:   len(board.Dimensions),
		HasBoardOutline:  outline,
		Nets:             nets,
		Footprints:       footprints,
		Truncated:        netsTruncated || footprintsTruncated,
		ObjectCounts:     objectCounts,
		LayerUsage:       layerUsage,
		Unsupported:      unsupported,
		PreservationOnly: preservationOnly,
		Issues:           []reports.Issue{},
	}
	if !summary.HasBoardOutline {
		summary.Issues = append(summary.Issues, reports.Issue{
			Code:     reports.CodeMissingBoardOutline,
			Severity: reports.SeverityWarning,
			Path:     "pcb.board_outline",
			Message:  "no Edge.Cuts board outline detected by scanner",
		})
	}
	return summary, nil
}

func Schematic(path string) (SchematicSummary, error) {
	if strings.TrimSpace(path) == "" {
		return SchematicSummary{}, fmt.Errorf("schematic path required")
	}
	file, err := schematicfiles.ReadFile(path)
	if err != nil {
		return SchematicSummary{}, err
	}
	symbols, truncated := boundedStrings(schematicSymbolRefs(file), inspectSampleLimit)
	objectCounts := map[string]int{
		"symbol":       len(file.Symbols),
		"wire":         len(file.Wires),
		"label":        len(file.Labels),
		"junction":     len(file.Junctions),
		"no_connect":   len(file.NoConnects),
		"sheet":        len(file.Sheets),
		"raw_item":     len(file.RawItems),
		"lib_symbols":  len(file.LibSymbols),
		"sheet_symbol": len(file.SheetInstances),
	}
	summary := SchematicSummary{
		Path:            path,
		FormatVersion:   string(file.Version),
		Generator:       file.Generator,
		SymbolCount:     len(file.Symbols),
		WireCount:       len(file.Wires),
		LabelCount:      len(file.Labels),
		JunctionCount:   len(file.Junctions),
		NoConnectCount:  len(file.NoConnects),
		SheetCount:      len(file.Sheets),
		Symbols:         symbols,
		Truncated:       truncated,
		ObjectCounts:    objectCounts,
		InspectionDepth: "structured",
		Unsupported:     schematicUnsupported(file.RawItems),
		Issues:          []reports.Issue{},
	}
	return summary, nil
}

func discoverProjectFile(root string) (string, bool, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", false, err
	}
	matches := []string{}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.HasSuffix(entry.Name(), ".kicad_pro") {
			matches = append(matches, filepath.Join(root, entry.Name()))
		}
	}
	if len(matches) == 0 {
		return "", false, nil
	}
	sort.Strings(matches)
	return matches[0], true, nil
}

func copyCounts(source map[string]int) map[string]int {
	if len(source) == 0 {
		return nil
	}
	result := make(map[string]int, len(source))
	for key, value := range source {
		if value != 0 {
			result[key] = value
		}
	}
	return result
}

func boundedStrings(values []string, limit int) ([]string, bool) {
	if len(values) == 0 {
		return nil, false
	}
	values = append([]string(nil), values...)
	sort.Strings(values)
	if len(values) <= limit {
		return values, false
	}
	return append([]string(nil), values[:limit]...), true
}

func boardNetNames(board pcbfiles.PCBFile) []string {
	seen := map[string]struct{}{}
	for _, net := range board.Nets {
		if net.Name != "" {
			seen[net.Name] = struct{}{}
		}
	}
	for _, track := range board.Tracks {
		if track.NetName != "" {
			seen[track.NetName] = struct{}{}
		}
	}
	for _, arc := range board.TrackArcs {
		if arc.NetName != "" {
			seen[arc.NetName] = struct{}{}
		}
	}
	for _, zone := range board.Zones {
		if zone.NetName != "" {
			seen[zone.NetName] = struct{}{}
		}
	}
	for _, footprint := range board.Footprints {
		for _, pad := range footprint.Pads {
			if pad.NetName != "" {
				seen[pad.NetName] = struct{}{}
			}
		}
	}
	values := make([]string, 0, len(seen))
	for value := range seen {
		values = append(values, value)
	}
	return values
}

func boardFootprintRefs(board pcbfiles.PCBFile) []string {
	values := make([]string, 0, len(board.Footprints))
	for _, footprint := range board.Footprints {
		label := footprint.Reference
		if label == "" {
			label = footprint.LibraryID
		}
		if label != "" {
			values = append(values, label)
		}
	}
	return values
}

func schematicSymbolRefs(file schematicfiles.SchematicFile) []string {
	values := make([]string, 0, len(file.Symbols))
	for _, symbol := range file.Symbols {
		label := symbol.Reference
		if label == "" {
			label = symbol.LibraryID
		}
		if label != "" {
			values = append(values, label)
		}
	}
	return values
}

func schematicUnsupported(items []schematicfiles.RawSchematicItem) []UnsupportedNode {
	counts := map[string]int{}
	for _, item := range items {
		kind := string(item.Kind)
		if kind == "" {
			kind = "raw_item"
		}
		counts[kind]++
	}
	return unsupportedNodes(counts)
}

func pcbUnsupported(items []pcbfiles.PreservedNode) []UnsupportedNode {
	counts := map[string]int{}
	for _, item := range items {
		kind := item.Family
		if kind == "" {
			kind = "unknown"
		}
		counts[kind]++
	}
	return unsupportedNodes(counts)
}

func unsupportedNodes(counts map[string]int) []UnsupportedNode {
	nodes := make([]UnsupportedNode, 0, len(counts))
	for kind, count := range counts {
		if count > 0 {
			nodes = append(nodes, UnsupportedNode{Kind: kind, Count: count})
		}
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].Kind < nodes[j].Kind })
	return nodes
}

type schematicScan struct {
	Counts   map[string]int
	Metadata map[string]string
}

func scanSchematic(r io.Reader) (schematicScan, error) {
	scanner := bufio.NewReader(r)
	scan := schematicScan{
		Counts:   map[string]int{},
		Metadata: map[string]string{},
	}
	depth := 0
	for {
		b, err := scanner.ReadByte()
		if err == io.EOF {
			return scan, nil
		}
		if err != nil {
			return schematicScan{}, err
		}
		switch b {
		case ';':
			if err := discardLine(scanner); err != nil {
				return schematicScan{}, err
			}
		case '"':
			if err := discardQuoted(scanner); err != nil {
				return schematicScan{}, err
			}
		case '(':
			token, err := readAtom(scanner)
			if err != nil {
				return schematicScan{}, err
			}
			depth++
			if token == "" {
				continue
			}
			if depth == 2 {
				scan.Counts[token]++
				switch token {
				case "version", "generator":
					value, err := readScalar(scanner)
					if err != nil {
						return schematicScan{}, err
					}
					scan.Metadata[token] = value
				}
			}
		case ')':
			if depth > 0 {
				depth--
			}
		}
	}
}

func discardLine(r *bufio.Reader) error {
	for {
		b, err := r.ReadByte()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if b == '\n' {
			return nil
		}
	}
}

func discardQuoted(r *bufio.Reader) error {
	escaped := false
	for {
		b, err := r.ReadByte()
		if err != nil {
			return err
		}
		if escaped {
			escaped = false
			continue
		}
		if b == '\\' {
			escaped = true
			continue
		}
		if b == '"' {
			return nil
		}
	}
}

func readAtom(r *bufio.Reader) (string, error) {
	var builder strings.Builder
	for {
		b, err := r.ReadByte()
		if err != nil {
			if err == io.EOF {
				return builder.String(), nil
			}
			return "", err
		}
		if b == ' ' || b == '\t' || b == '\n' || b == '\r' {
			if builder.Len() == 0 {
				continue
			}
			return builder.String(), nil
		}
		if b == '(' || b == ')' || b == ';' {
			if b == ')' || b == ';' || builder.Len() > 0 {
				if err := r.UnreadByte(); err != nil {
					return "", err
				}
			}
			return builder.String(), nil
		}
		builder.WriteByte(b)
	}
}

func readScalar(r *bufio.Reader) (string, error) {
	for {
		b, err := r.ReadByte()
		if err != nil {
			if err == io.EOF {
				return "", nil
			}
			return "", err
		}
		switch b {
		case ' ', '\t', '\r', '\n':
			continue
		case ';':
			if err := discardLine(r); err != nil {
				return "", err
			}
		case ')':
			if err := r.UnreadByte(); err != nil {
				return "", err
			}
			return "", nil
		case '"':
			return readQuotedScalar(r)
		default:
			if err := r.UnreadByte(); err != nil {
				return "", err
			}
			return readAtom(r)
		}
	}
}

func readQuotedScalar(r *bufio.Reader) (string, error) {
	var builder strings.Builder
	escaped := false
	for {
		b, err := r.ReadByte()
		if err != nil {
			return "", err
		}
		if escaped {
			builder.WriteByte(b)
			escaped = false
			continue
		}
		if b == '\\' {
			escaped = true
			continue
		}
		if b == '"' {
			return builder.String(), nil
		}
		builder.WriteByte(b)
	}
}

func issueFromError(err error, path string) reports.Issue {
	if issue, ok := reports.IssueFromError(err); ok {
		issue.Path = path
		return issue
	}
	return reports.Issue{
		Code:     reports.CodeUnknown,
		Severity: reports.SeverityError,
		Path:     path,
		Message:  err.Error(),
	}
}
