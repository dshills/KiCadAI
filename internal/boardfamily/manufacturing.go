package boardfamily

import (
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"

	"kicadai/internal/fabrication"
	"kicadai/internal/kicadfiles"
	"kicadai/internal/kicadfiles/sexpr"
)

const manufacturingLayers = "F.Cu,B.Cu,F.Paste,B.Paste,F.SilkS,B.SilkS,F.Mask,B.Mask,Edge.Cuts"

var gerberFunctions = map[string]string{
	"board-F_Cu.gbr": "Copper,L1,Top", "board-B_Cu.gbr": "Copper,L2,Bot",
	"board-F_Paste.gbr": "SolderPaste,Top", "board-B_Paste.gbr": "SolderPaste,Bot",
	"board-F_Silkscreen.gbr": "Legend,Top", "board-B_Silkscreen.gbr": "Legend,Bot",
	"board-F_Mask.gbr": "SolderMask,Top", "board-B_Mask.gbr": "SolderMask,Bot",
	"board-Edge_Cuts.gbr": "Profile",
}

// exportManufacturing is called only after native validation. It creates new
// artifacts, never reroutes/refills/saves the source board or overwrites a bundle.
func exportManufacturing(ctx context.Context, dir, cli string) error {
	out := filepath.Join(dir, "manufacturing")
	if err := os.Mkdir(out, 0755); err != nil {
		return fmt.Errorf("manufacturing output must be new: %w", err)
	}
	board := filepath.Join(dir, "board.kicad_pcb")
	jobs := []struct {
		name string
		args []string
	}{
		{"gerbers", []string{"pcb", "export", "gerbers", "--layers", manufacturingLayers, "--precision", "6", "--no-protel-ext", "--subtract-soldermask", "--output", filepath.Join(out, "gerbers") + string(os.PathSeparator), board}},
		{"drill", []string{"pcb", "export", "drill", "--format", "excellon", "--drill-origin", "absolute", "--excellon-units", "mm", "--excellon-zeros-format", "decimal", "--excellon-separate-th", "--generate-map", "--map-format", "svg", "--generate-report", "--report-path", filepath.Join(out, "drill-report.txt"), "--output", filepath.Join(out, "drill") + string(os.PathSeparator), board}},
		{"placement", []string{"pcb", "export", "pos", "--format", "csv", "--units", "mm", "--side", "both", "--output", filepath.Join(out, "placement.csv"), board}},
	}
	for _, job := range jobs {
		if err := command(ctx, cli, out, job.name, job.args...); err != nil {
			return err
		}
	}
	base := fabrication.ValidateFabricationArtifacts(ctx, fabrication.PlotRequest{PCBPath: board, GerberDir: filepath.Join(out, "gerbers"), DrillDir: filepath.Join(out, "drill")})
	if base.Gerber != fabrication.EvidencePass || base.Drill != fabrication.EvidencePass {
		return fmt.Errorf("fabrication artifact presence checks failed: %+v", base.Issues)
	}
	if err := verifyManufacturing(dir); err != nil {
		return err
	}
	native, err := os.ReadFile(board)
	if err != nil {
		return err
	}
	hashes := map[string]string{}
	if err := filepath.WalkDir(out, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(out, path)
		if err != nil {
			return err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		hashes[filepath.ToSlash(rel)] = fmt.Sprintf("%x", sha256.Sum256(b))
		return nil
	}); err != nil {
		return err
	}
	return writeJSON(filepath.Join(out, "manifest.json"), map[string]any{
		"status": "software-validated-exports-not-fabrication-approval", "source_pcb_sha256": fmt.Sprintf("%x", sha256.Sum256(native)),
		"kicad_version": "10.0.3", "files_sha256": hashes, "units": "mm", "origin": "absolute board origin; placement and drill Y is negative native board Y",
		"checks":      []string{"required native layer/drill outputs", "Gerber X2 layer identity, units and terminators", "Gerber job two-layer 1.6 mm stack", "placement reference/value/package/position/rotation against every native footprint", "plated and non-plated circular drill hits against native pads and vias, 0.00051 mm serialization tolerance"},
		"limitations": []string{"No fabrication order or assembly authorization. Fabricator must confirm stack, finish, tolerances and accepted file conventions.", "Position CSV includes all parts, including through-hole headers; it is not a machine-specific assembly program. Verify component orientation/pin 1 with the assembler.", "KiCad job size includes the 0.1 mm outline stroke (120.1x80.1 mm); nominal board dimensions follow the 120x80 mm Edge.Cuts centerline.", "Raw exports contain native timestamps; preserve these bytes and normalize timestamps only for explicitly labeled replay comparisons."},
	})
}

func verifyManufacturing(dir string) error {
	out := filepath.Join(dir, "manufacturing")
	for name, function := range gerberFunctions {
		b, err := os.ReadFile(filepath.Join(out, "gerbers", name))
		if err != nil {
			return err
		}
		if err := verifyGerberFraming(string(b), function); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}
	b, err := os.ReadFile(filepath.Join(out, "gerbers", "board-job.gbrjob"))
	if err != nil {
		return err
	}
	var job struct {
		GeneralSpecs struct {
			LayerNumber    int
			BoardThickness float64
			Size           struct{ X, Y float64 }
		}
		FilesAttributes []struct{ Path, FileFunction string }
	}
	if err := json.Unmarshal(b, &job); err != nil {
		return err
	}
	if job.GeneralSpecs.LayerNumber != 2 || job.GeneralSpecs.BoardThickness != 1.6 || math.Abs(job.GeneralSpecs.Size.X-120.1) > 1e-6 || math.Abs(job.GeneralSpecs.Size.Y-80.1) > 1e-6 {
		return errors.New("Gerber job geometry/stack differs from fixed family")
	}
	listed := map[string]string{}
	for _, file := range job.FilesAttributes {
		if _, exists := listed[file.Path]; exists {
			return errors.New("duplicate Gerber job file")
		}
		listed[file.Path] = file.FileFunction
	}
	if !reflect.DeepEqual(listed, gerberFunctions) {
		return errors.New("Gerber job layer set differs from required output")
	}
	b, err = os.ReadFile(filepath.Join(dir, "board.kicad_pcb"))
	if err != nil {
		return err
	}
	board, err := sexpr.Parse(b)
	if err != nil {
		return err
	}
	if err := verifyPlacement(filepath.Join(out, "placement.csv"), board); err != nil {
		return err
	}
	return verifyDrills(filepath.Join(out, "drill"), board)
}

func number(n nativeNode, index int) (float64, error) {
	v, err := strconv.ParseFloat(n.ListValue(index), 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, errors.New("invalid native/export coordinate")
	}
	return v, nil
}

func nativePosition(n nativeNode) (x, y, rotation float64, err error) {
	at, ok := n.Child("at")
	if !ok {
		return 0, 0, 0, errors.New("missing native position")
	}
	x, err = number(at, 1)
	if err != nil {
		return
	}
	y, err = number(at, 2)
	if err != nil {
		return
	}
	if at.ListValue(3) != "" {
		rotation, err = number(at, 3)
	}
	return
}

func verifyPlacement(path string, board nativeNode) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	rows, err := csv.NewReader(strings.NewReader(string(b))).ReadAll()
	if err != nil {
		return err
	}
	if len(rows) == 0 || !reflect.DeepEqual(rows[0], []string{"Ref", "Val", "Package", "PosX", "PosY", "Rot", "Side"}) {
		return errors.New("unsupported placement header")
	}
	parts := map[string]nativeNode{}
	for _, fp := range board.ChildrenByHead("footprint") {
		ref := property(fp, "Reference")
		if _, exists := parts[ref]; ref == "" || exists {
			return errors.New("missing/duplicate native reference")
		}
		parts[ref] = fp
	}
	seen := map[string]bool{}
	for _, row := range rows[1:] {
		if len(row) != 7 {
			return errors.New("invalid placement row")
		}
		fp, ok := parts[row[0]]
		if !ok || seen[row[0]] {
			return errors.New("unknown/duplicate placement reference")
		}
		seen[row[0]] = true
		_, pkg, _ := strings.Cut(fp.ListValue(1), ":")
		if row[1] != property(fp, "Value") || row[2] != pkg || row[6] != "top" || field(fp, "layer") != "F.Cu" {
			return fmt.Errorf("placement identity/side mismatch: %s", row[0])
		}
		x, y, angle, err := nativePosition(fp)
		if err != nil {
			return err
		}
		for i, want := range []float64{x, -y, angle} {
			got, err := strconv.ParseFloat(row[3+i], 64)
			if err != nil || math.IsNaN(got) || math.IsInf(got, 0) || math.Abs(got-want) > 1e-6 {
				return fmt.Errorf("placement geometry mismatch: %s", row[0])
			}
		}
	}
	if len(seen) != len(parts) {
		return errors.New("placement omitted a populated component")
	}
	return nil
}

type drillHit struct{ x, y, diameter float64 }

func compareDrillHits(expected, got []drillHit) (missing, unexpected []string) {
	used := make([]bool, len(got))
	for _, want := range expected {
		matched := false
		for i, actual := range got {
			// KiCad's metric decimal Excellon rounds to 0.001 mm. Comparing
			// separately rounded strings fails at exact half-micron boundaries.
			if !used[i] && math.Abs(want.x-actual.x) <= 0.00051 && math.Abs(want.y-actual.y) <= 0.00051 && math.Abs(want.diameter-actual.diameter) <= 0.00051 {
				used[i], matched = true, true
				break
			}
		}
		if !matched {
			missing = append(missing, fmt.Sprintf("%.6f,%.6f,%.6f", want.x, want.y, want.diameter))
		}
	}
	for i, hit := range got {
		if !used[i] {
			unexpected = append(unexpected, fmt.Sprintf("%.6f,%.6f,%.6f", hit.x, hit.y, hit.diameter))
		}
	}
	sortStrings(missing)
	sortStrings(unexpected)
	return
}

func verifyDrills(dir string, board nativeNode) error {
	want := map[string][]drillHit{"PTH": {}, "NPTH": {}}
	for _, via := range board.ChildrenByHead("via") {
		x, y, _, err := nativePosition(via)
		if err != nil {
			return err
		}
		drill, _ := via.Child("drill")
		diameter, err := number(drill, 1)
		if err != nil {
			return err
		}
		want["PTH"] = append(want["PTH"], drillHit{x, -y, diameter})
	}
	for _, fp := range board.ChildrenByHead("footprint") {
		x, y, angle, err := nativePosition(fp)
		if err != nil {
			return err
		}
		for _, pad := range fp.ChildrenByHead("pad") {
			drill, ok := pad.Child("drill")
			if !ok {
				continue
			}
			if len(drill.Children) != 2 {
				return errors.New("non-circular or offset drill unsupported by this family export check")
			}
			diameter, err := number(drill, 1)
			if err != nil {
				return err
			}
			px, py, _, err := nativePosition(pad)
			if err != nil {
				return err
			}
			px, py = kicadfiles.RotateBoardLocalXY(px, py, angle)
			kind := "PTH"
			if pad.ListValue(2) == "np_thru_hole" {
				kind = "NPTH"
			}
			want[kind] = append(want[kind], drillHit{x + px, -(y + py), diameter})
		}
	}
	for kind, expected := range want {
		b, err := os.ReadFile(filepath.Join(dir, "board-"+kind+".drl"))
		if err != nil {
			return err
		}
		got, err := parseKiCadDrills(string(b), kind)
		if err != nil {
			return fmt.Errorf("%s: %w", kind, err)
		}
		if missing, unexpected := compareDrillHits(expected, got); len(missing) != 0 || len(unexpected) != 0 {
			return fmt.Errorf("%s drill positions/diameters differ from native pads/vias: missing=%v unexpected=%v", kind, missing, unexpected)
		}
	}
	return nil
}
