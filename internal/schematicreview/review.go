// Package schematicreview reviews immutable snapshots of existing schematics.
// It never repairs or rewrites the source and never claims hardware validation.
package schematicreview

import (
	"context"
	"errors"
	"fmt"
	"kicadai/internal/evaluate"
	"kicadai/internal/evidencebundle"
	"kicadai/internal/inspect"
	"kicadai/internal/kicadfiles/checks"
	"kicadai/internal/kicadfiles/schematic"
	"kicadai/internal/reports"
	"kicadai/internal/schematicassistant"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const Schema = "kicadai.schematic-review.v1"
const MaxSheets = 32
const MaxSymbols = 2048
const MaxSourceBytes = 16 << 20

type Options struct {
	KiCadCLI   string
	RequireERC bool
	Runner     checks.Runner
}
type Part struct {
	Reference string `json:"reference"`
	Library   string `json:"library"`
	Value     string `json:"value"`
	Purpose   string `json:"purpose"`
}
type Sheet struct {
	Path       string                   `json:"path"`
	SHA256     string                   `json:"sha256"`
	Inspection inspect.SchematicSummary `json:"inspection"`
	Evaluation evaluate.Report          `json:"evaluation"`
	Parts      []Part                   `json:"parts"`
}
type Report struct {
	Schema          string   `json:"schema"`
	Status          string   `json:"status"`
	RootSheet       string   `json:"root_sheet"`
	Sheets          []Sheet  `json:"sheets"`
	NativeERC       string   `json:"native_erc"`
	SourceUnchanged bool     `json:"source_unchanged"`
	Limitations     []string `json:"limitations"`
}

func rootSheet(target string) (string, error) {
	info, err := os.Lstat(target)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		if project, ok, err := inspect.DiscoverProjectFile(target); err != nil {
			return "", err
		} else if ok {
			return strings.TrimSuffix(project, filepath.Ext(project)) + ".kicad_sch", nil
		}
		entries, err := os.ReadDir(target)
		if err != nil {
			return "", err
		}
		var matches []string
		for _, e := range entries {
			if strings.EqualFold(filepath.Ext(e.Name()), ".kicad_sch") {
				matches = append(matches, filepath.Join(target, e.Name()))
			}
		}
		if len(matches) != 1 {
			return "", errors.New("select an explicit root .kicad_sch; directory has no unambiguous project/root")
		}
		return matches[0], nil
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("review target must be a regular file or real directory")
	}
	switch strings.ToLower(filepath.Ext(target)) {
	case ".kicad_sch":
		return target, nil
	case ".kicad_pro":
		return strings.TrimSuffix(target, filepath.Ext(target)) + ".kicad_sch", nil
	}
	return "", errors.New("review requires .kicad_sch, .kicad_pro, or project directory")
}

// Analyze returns a bounded snapshot and report; native tools, when requested,
// run in a temporary workspace rather than the user's project.
func Analyze(ctx context.Context, target string, options Options) (Report, map[string][]byte, error) {
	report := Report{Schema: Schema, Status: "reviewed", NativeERC: "not_checked", Sheets: []Sheet{}, Limitations: []string{"Read-only structural/electrical-rule review, not a design approval or physical validation.", "Internal checks run per sheet. Cross-sheet electrical interpretation requires optional native ERC; project rule overrides/exclusions and external libraries are not imported.", "Component purposes are conservative reference-prefix descriptions, not inferred functional correctness. No operating-point, tolerance, thermal, PCB or manufacturing checks are performed.", "Unsupported and preservation-only constructs remain explicitly listed; passing supported checks does not assess those constructs."}}
	files := map[string][]byte{}
	if err := ctx.Err(); err != nil {
		return report, nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	selected, err := rootSheet(target)
	if err != nil {
		return report, nil, err
	}
	selected, err = filepath.Abs(selected)
	if err != nil {
		return report, nil, err
	}
	root, err := filepath.EvalSymlinks(filepath.Dir(selected))
	if err != nil {
		return report, nil, err
	}
	report.RootSheet = filepath.Base(selected)
	work, err := os.MkdirTemp("", "kicadai-schematic-review-")
	if err != nil {
		return report, nil, err
	}
	defer func() { _ = os.RemoveAll(work) }()
	queue := []string{report.RootSheet}
	visited := map[string]bool{}
	caseNames := map[string]string{}
	total, symbols := 0, 0
	for len(queue) > 0 {
		if err := ctx.Err(); err != nil {
			return report, nil, err
		}
		name := queue[0]
		queue = queue[1:]
		folded := strings.ToLower(name)
		if previous, ok := caseNames[folded]; ok && previous != name {
			return report, nil, errors.New("case-aliased sheet names are not portable")
		}
		caseNames[folded] = name
		if visited[name] {
			continue
		}
		visited[name] = true
		if len(visited) > MaxSheets {
			return report, nil, errors.New("review exceeds 32 distinct sheets")
		}
		path := filepath.Join(root, filepath.FromSlash(name))
		resolved, err := filepath.EvalSymlinks(path)
		if err != nil {
			return report, nil, err
		}
		if resolved != path {
			return report, nil, errors.New("symbolic sheet paths are not reviewed")
		}
		raw, err := evidencebundle.Read(root, name)
		if err != nil {
			return report, nil, err
		}
		total += len(raw)
		if total > MaxSourceBytes {
			return report, nil, errors.New("review source exceeds 16 MiB")
		}
		file, err := schematic.Read(raw)
		if err != nil {
			return report, nil, fmt.Errorf("parse %s: %w", name, err)
		}
		symbols += len(file.Symbols)
		if symbols > MaxSymbols {
			return report, nil, errors.New("review exceeds 2048 symbols")
		}
		files["source/"+name] = raw
		destination := filepath.Join(work, filepath.FromSlash(name))
		if err = os.MkdirAll(filepath.Dir(destination), 0700); err != nil {
			return report, nil, err
		}
		if err = os.WriteFile(destination, raw, 0600); err != nil {
			return report, nil, err
		}
		parts := []Part{}
		for _, s := range file.Symbols {
			part := Part{Reference: s.Reference, Library: s.LibraryID, Value: s.Value}
			for _, p := range s.Properties {
				if p.Name == "Value" {
					part.Value = p.Value
				}
			}
			part.Purpose = purpose(part.Reference)
			parts = append(parts, part)
		}
		sort.Slice(parts, func(i, j int) bool { return parts[i].Reference < parts[j].Reference })
		report.Sheets = append(report.Sheets, Sheet{Path: name, SHA256: evidencebundle.Digest(raw), Parts: parts})
		if len(file.Sheets) > 128 {
			return report, nil, errors.New("review exceeds 128 child instances per sheet")
		}
		for _, child := range file.Sheets {
			if child.Filename == "" || filepath.IsAbs(child.Filename) || strings.ContainsAny(child.Filename, `\$:`) {
				return report, nil, errors.New("child sheet requires a relative literal filename")
			}
			next := filepath.Clean(filepath.Join(filepath.Dir(name), child.Filename))
			rel, err := filepath.Rel(root, filepath.Join(root, next))
			if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || strings.ToLower(filepath.Ext(rel)) != ".kicad_sch" {
				return report, nil, errors.New("child sheet escapes the selected project root or is not a schematic")
			}
			queue = append(queue, filepath.ToSlash(rel))
		}
	}
	sort.Slice(report.Sheets, func(i, j int) bool { return report.Sheets[i].Path < report.Sheets[j].Path })
	for i := range report.Sheets {
		s := &report.Sheets[i]
		path := filepath.Join(work, filepath.FromSlash(s.Path))
		s.Inspection, err = inspect.SchematicContext(ctx, path)
		if err != nil {
			return report, nil, err
		}
		s.Inspection.Path = s.Path
		if s.Inspection.Preservation != nil {
			for j := range s.Inspection.Preservation.Files {
				s.Inspection.Preservation.Files[j].Path = s.Path
			}
		}
		s.Evaluation, err = evaluate.Schematic(path)
		if err != nil {
			return report, nil, err
		}
		s.Evaluation.Target = s.Path
		if s.Evaluation.Preservation != nil {
			for j := range s.Evaluation.Preservation.Files {
				s.Evaluation.Preservation.Files[j].Path = s.Path
			}
		}
		if reports.HasBlockingIssue(s.Evaluation.Issues) {
			report.Status = "findings"
		}
		if len(s.Inspection.Unsupported) > 0 || len(s.Inspection.PreservationOnly) > 0 {
			report.Status = "findings"
		}
	}
	if options.RequireERC || options.KiCadCLI != "" {
		report.NativeERC = "blocked"
		cli, err := checks.DiscoverCLI(options.KiCadCLI)
		if err != nil {
			return report, nil, err
		}
		runner := options.Runner
		if runner == nil {
			runner = schematicassistant.SafeRunner{}
		}
		native, runErr := checks.RunERCWithRunner(ctx, runner, cli, filepath.Join(work, report.RootSheet), checks.Options{Timeout: 30 * time.Second})
		raw, encodeErr := evidencebundle.JSON(native)
		if encodeErr != nil {
			return report, nil, encodeErr
		}
		files["native-erc.json"] = raw
		if runErr != nil || native.Status != checks.CheckStatusPass || native.KiCadVersion == "" {
			report.Status = "findings"
			report.NativeERC = "failed"
		} else {
			report.NativeERC = "checked"
		}
	}
	for _, s := range report.Sheets {
		raw, err := evidencebundle.Read(root, s.Path)
		if err != nil || evidencebundle.Digest(raw) != s.SHA256 {
			return report, nil, errors.New("source changed during review; discard and rerun")
		}
	}
	report.SourceUnchanged = true
	return report, files, nil
}

func Write(ctx context.Context, target, output string, options Options) (Report, error) {
	if err := evidencebundle.CheckDestination(output); err != nil {
		return Report{}, err
	}
	selected, err := rootSheet(target)
	if err != nil {
		return Report{}, err
	}
	root, err := filepath.Abs(filepath.Dir(selected))
	if err != nil {
		return Report{}, err
	}
	dest, err := filepath.Abs(output)
	if err != nil {
		return Report{}, err
	}
	// Resolve the input root before checking containment, including symlinked parents.
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return Report{}, err
	}
	// Resolve the closest existing output ancestor so a symlinked parent cannot
	// bypass the source-containment check, even with several new subdirectories.
	ancestor := dest
	var suffix []string
	for {
		if _, e := os.Lstat(ancestor); e == nil {
			break
		} else if !os.IsNotExist(e) {
			return Report{}, e
		}
		suffix = append(suffix, filepath.Base(ancestor))
		parent := filepath.Dir(ancestor)
		if parent == ancestor {
			return Report{}, errors.New("output has no existing ancestor")
		}
		ancestor = parent
	}
	canonical, e := filepath.EvalSymlinks(ancestor)
	if e != nil {
		return Report{}, e
	}
	for i := len(suffix) - 1; i >= 0; i-- {
		canonical = filepath.Join(canonical, suffix[i])
	}
	dest = canonical
	if rel, e := filepath.Rel(root, dest); e == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return Report{}, errors.New("review output must be outside the source project")
	}
	report, files, err := Analyze(ctx, target, options)
	if err != nil {
		return report, err
	}
	raw, err := evidencebundle.JSON(report)
	if err != nil {
		return report, err
	}
	files["report.json"] = raw
	html, err := render(report)
	if err != nil {
		return report, err
	}
	files["report.html"] = html
	if err = ctx.Err(); err != nil {
		return report, err
	}
	return report, evidencebundle.Publish(output, files)
}

func Verify(root string) error {
	if err := evidencebundle.Verify(root); err != nil {
		return err
	}
	raw, err := evidencebundle.Read(root, "report.json")
	if err != nil {
		return err
	}
	var report Report
	if err = evidencebundle.Decode(raw, &report); err != nil {
		return err
	}
	if report.Schema != Schema || len(report.Sheets) == 0 {
		return errors.New("invalid review report")
	}
	for _, s := range report.Sheets {
		raw, err := evidencebundle.Read(root, "source/"+s.Path)
		if err != nil || evidencebundle.Digest(raw) != s.SHA256 {
			return errors.New("review source identity differs")
		}
	}
	return nil
}

func purpose(ref string) string {
	if strings.HasPrefix(ref, "#") {
		return "Power or schematic-only marker."
	}
	end := 0
	for i, r := range ref {
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
			end = i + 1
		} else {
			break
		}
	}
	switch strings.ToUpper(ref[:end]) {
	case "R":
		return "Resistor; may limit current, bias a node, divide voltage or set a time constant."
	case "C":
		return "Capacitor; may store charge, filter or couple a signal."
	case "U":
		return "Integrated circuit; inspect its exact datasheet and pin functions."
	case "J", "TP":
		return "External connection or observation point."
	}
	return "Purpose not inferred; inspect the symbol and its connections."
}
