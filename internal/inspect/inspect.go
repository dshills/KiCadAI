package inspect

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"kicadai/internal/kicadfiles/pcb"
	"kicadai/internal/reports"
)

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
	report, err := pcb.ScanCorpus(path)
	if err != nil {
		return PCBSummary{}, err
	}
	summary := PCBSummary{
		Path:             path,
		FilesScanned:     report.Files,
		NetCount:         report.TopLevelObjects["net"],
		FootprintCount:   report.TopLevelObjects["footprint"],
		PadCount:         report.FootprintChildTypes["pad"],
		TrackCount:       report.TopLevelObjects["segment"] + report.TopLevelObjects["arc"],
		ViaCount:         report.TopLevelObjects["via"],
		ZoneCount:        report.TopLevelObjects["zone"],
		DrawingCount:     report.TopLevelObjects["gr_line"] + report.TopLevelObjects["gr_rect"] + report.TopLevelObjects["gr_circle"] + report.TopLevelObjects["gr_arc"] + report.TopLevelObjects["gr_poly"] + report.TopLevelObjects["gr_text"],
		DimensionCount:   report.SupportedObjects["dimension"],
		HasBoardOutline:  report.LayerUsage["Edge.Cuts"] > 0,
		ObjectCounts:     copyCounts(report.ObjectCount),
		LayerUsage:       copyCounts(report.LayerUsage),
		Unsupported:      unsupportedNodes(report.UnsupportedObjects),
		PreservationOnly: unsupportedNodes(report.PreservationOnly),
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
	file, err := os.Open(path)
	if err != nil {
		return SchematicSummary{}, err
	}
	defer file.Close()
	scan, err := scanSchematic(file)
	if err != nil {
		return SchematicSummary{}, err
	}
	summary := SchematicSummary{
		Path:            path,
		FormatVersion:   scan.Metadata["version"],
		Generator:       scan.Metadata["generator"],
		SymbolCount:     scan.Counts["symbol"],
		WireCount:       scan.Counts["wire"],
		LabelCount:      scan.Counts["label"] + scan.Counts["global_label"] + scan.Counts["hierarchical_label"],
		JunctionCount:   scan.Counts["junction"],
		NoConnectCount:  scan.Counts["no_connect"],
		SheetCount:      scan.Counts["sheet"],
		ObjectCounts:    scan.Counts,
		InspectionDepth: "shallow",
		Issues: []reports.Issue{{
			Code:     reports.CodeUnsupportedOperation,
			Severity: reports.SeverityWarning,
			Path:     "schematic",
			Message:  "full structured schematic reader is not implemented; summary uses shallow object counts",
		}},
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
