package schematicassistant

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"kicadai/internal/atomicdir"
	"kicadai/internal/evaluate"
	"kicadai/internal/kicadfiles/checks"
	"kicadai/internal/reports"
	"kicadai/internal/schematicir"
	"kicadai/internal/transactions"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Check struct {
	Name     string `json:"name"`
	Status   string `json:"status"`
	Detail   string `json:"detail"`
	Evidence string `json:"evidence,omitempty"`
}
type Part struct {
	Ref     string `json:"ref"`
	Value   string `json:"value"`
	Purpose string `json:"purpose"`
}
type Report struct {
	Schema             string            `json:"schema"`
	Spec               Spec              `json:"spec"`
	SpecSHA256         string            `json:"spec_sha256"`
	Status             string            `json:"status"`
	Checks             []Check           `json:"checks"`
	Measurements       []Measurement     `json:"measurements"`
	Parts              []Part            `json:"parts"`
	Limitations        []string          `json:"limitations"`
	Diagram            string            `json:"diagram"`
	DiagramDescription string            `json:"diagram_description"`
	Artifacts          map[string]string `json:"artifacts"`
	PreviewData        string            `json:"-"`
}
type Options struct {
	KiCadCLI   string
	RequireERC bool
	Runner     checks.Runner
}

func Generate(ctx context.Context, session Session, output string, options Options) (report Report, err error) {
	stageName := "confirmation"
	defer func() {
		if err != nil {
			err = &StageError{Stage: stageName, Cause: err}
		}
	}()
	if err := session.ValidateConfirmation(); err != nil {
		return report, err
	}
	stageName = "output_preflight"
	if strings.TrimSpace(output) == "" {
		return report, errors.New("output directory is required")
	}
	if err := ctx.Err(); err != nil {
		return report, err
	}
	absolute, err := filepath.Abs(output)
	if err != nil {
		return report, err
	}
	if _, err = os.Lstat(absolute); err == nil {
		return report, errors.New("output already exists; choose a new directory")
	} else if !os.IsNotExist(err) {
		return report, err
	}
	parent := filepath.Dir(absolute)
	if err = os.MkdirAll(parent, 0755); err != nil {
		return report, err
	}
	stagingRoot, err := os.MkdirTemp(parent, ".schematic-build-")
	if err != nil {
		return report, err
	}
	defer func() { _ = os.RemoveAll(stagingRoot) }()
	stage := filepath.Join(stagingRoot, "project")
	stageName = "schematic_construction"
	document, err := Circuit(session.Spec)
	if err != nil {
		return report, err
	}
	tx, issues := schematicir.ToProjectTransaction(document)
	if reports.HasBlockingIssue(issues) {
		return report, fmt.Errorf("schematic construction blocked: %s", issues[0].Message)
	}
	stageName = "schematic_write"
	applied := transactions.Apply(tx, transactions.ApplyOptions{OutputDir: stage})
	if reports.HasBlockingIssue(applied.Issues) {
		return report, fmt.Errorf("schematic write blocked: %s", firstBlocking(applied.Issues))
	}
	schematicPath := filepath.Join(stage, session.Spec.Name+".kicad_sch")
	stageName = "internal_evaluation"
	evaluated, err := evaluate.Schematic(schematicPath)
	if err != nil {
		return report, err
	}
	report = Report{Schema: Schema + ".report", Spec: session.Spec, SpecSHA256: session.ConfirmationSHA256, Status: "candidate", Diagram: "diagram.svg", DiagramDescription: "Functional illustration of the confirmed specification, not evidence of native layout. Open the KiCad project for exact geometry.", Artifacts: map[string]string{},
		Limitations: []string{"Ideal zero-impedance source and ideal passive components; computed values are not measurements or simulator results.", "No tolerance sweep, parasitic, source-loading, capacitor ESR/leakage, temperature or manufacturer model is qualified.", "The 0.125 W dissipation budget is a workflow bound, not a verified component rating. Select appropriately derated real components.", "This is a schematic-only candidate, not a PCB, manufacturing package, physical validation or fabrication approval."}}
	report.Measurements, err = Analyze(session.Spec)
	if err != nil {
		return report, err
	}
	report.Checks = append(report.Checks, Check{"Confirmed specification", "checked", "The exact canonical specification matches its confirmation hash.", "session.json"}, Check{"Analytical bounds", "checked", "Finite SI values, supported family, explicit load, and resistor power budget checked.", "report.json"})
	for _, check := range evaluated.Checks {
		status := "checked"
		if check.Status != evaluate.CheckPassed {
			status = string(check.Status)
		}
		report.Checks = append(report.Checks, Check{check.Name, status, "Existing KiCadAI schematic evaluator; see full issues in internal-checks.json.", "internal-checks.json"})
		if check.Required && check.Status != evaluate.CheckPassed {
			return report, fmt.Errorf("internal schematic check %s: %s", check.Name, check.Status)
		}
	}
	if reports.HasBlockingIssue(evaluated.Issues) {
		return report, fmt.Errorf("schematic evaluation blocked: %s", firstBlocking(evaluated.Issues))
	}
	stageName = "report_artifacts"
	if err = writeJSON(stage, "internal-checks.json", evaluated); err != nil {
		return report, err
	}
	if err = writeJSON(stage, "session.json", session); err != nil {
		return report, err
	}
	if err = writeJSON(stage, "source-ir.json", document); err != nil {
		return report, err
	}
	report.Parts = []Part{{"TP1", fmt.Sprintf("%.9g V maximum", *session.Spec.InputV), "External input connection; the voltage source is not generated."}, {"R1", fmt.Sprintf("%.9g ohm", *session.Spec.R1), "Upper divider resistor or RC series resistor."}, {"TP2", "VOUT", "Output observation point; load is included only when explicitly specified."}}
	if session.Spec.R2 != nil {
		report.Parts = append(report.Parts, Part{"R2", fmt.Sprintf("%.9g ohm", *session.Spec.R2), "Lower divider resistor."})
	}
	if session.Spec.C != nil {
		report.Parts = append(report.Parts, Part{"C1", fmt.Sprintf("%.9g F", *session.Spec.C), "Shunts AC to ground; with R1 sets the first-order time constant."})
	}
	if *session.Spec.Load > 0 {
		report.Parts = append(report.Parts, Part{"R3", fmt.Sprintf("%.9g ohm", *session.Spec.Load), "Explicit output load; included in the calculated response."})
	}
	if err = os.WriteFile(filepath.Join(stage, "diagram.svg"), []byte(diagramSVG(session.Spec)), 0644); err != nil {
		return report, err
	}
	stageName = "native_kicad"
	if err = runNative(ctx, stage, schematicPath, options, &report); err != nil {
		return report, err
	}
	stageName = "report_artifacts"
	report.Checks = append(report.Checks, Check{"Hardware behavior", "not_checked", "No assembled hardware or measured results exist in this workflow.", ""}, Check{"PCB / manufacturing", "not_applicable", "No PCB or manufacturing approval is produced.", ""})
	report.Artifacts, err = inventory(stage)
	if err != nil {
		return report, err
	}
	if err = writeJSON(stage, "report.json", report); err != nil {
		return report, err
	}
	if err = writeHTML(stage, report); err != nil {
		return report, err
	}
	manifest, err := inventory(stage)
	if err != nil {
		return report, err
	}
	if err = writeJSON(stage, "manifest.json", manifest); err != nil {
		return report, err
	}
	if err = ctx.Err(); err != nil {
		return report, err
	}
	stageName = "publication"
	err = atomicdir.Publish(absolute, func(destination string) error {
		entries, readErr := os.ReadDir(stage)
		if readErr != nil {
			return readErr
		}
		for _, entry := range entries {
			if moveErr := os.Rename(filepath.Join(stage, entry.Name()), filepath.Join(destination, entry.Name())); moveErr != nil {
				return moveErr
			}
		}
		return ctx.Err()
	})
	if err != nil {
		return report, err
	}
	return report, nil
}
func firstBlocking(issues []reports.Issue) string {
	for _, issue := range issues {
		if issue.Blocking() {
			return issue.Message
		}
	}
	return "unknown failure"
}
func writeJSON(root, name string, value any) error {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(root, name), append(raw, '\n'), 0644)
}
func inventory(root string) (map[string]string, error) {
	result := map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if len(result) >= 128 {
			return errors.New("bundle exceeds 128 files")
		}
		if !entry.Type().IsRegular() {
			return errors.New("bundle contains non-regular file")
		}
		file, e := os.Open(path)
		if e != nil {
			return e
		}
		raw, e := io.ReadAll(io.LimitReader(file, (16<<20)+1))
		closeErr := file.Close()
		if e == nil {
			e = closeErr
		}
		if e != nil {
			return e
		}
		if len(raw) > 16<<20 {
			return errors.New("bundle artifact exceeds 16 MiB")
		}
		relative, e := filepath.Rel(root, path)
		if e != nil {
			return e
		}
		sum := sha256.Sum256(raw)
		result[filepath.ToSlash(relative)] = hex.EncodeToString(sum[:])
		return nil
	})
	return result, err
}

// Verify checks an exact portable file inventory; hashes establish integrity, not independent attestation.
func Verify(root string) error {
	file, err := os.Open(filepath.Join(root, "manifest.json"))
	if err != nil {
		return err
	}
	raw, err := io.ReadAll(io.LimitReader(file, MaxInputBytes+1))
	closeErr := file.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if len(raw) > MaxInputBytes {
		return errors.New("manifest too large")
	}
	var expected map[string]string
	if err = json.Unmarshal(raw, &expected); err != nil {
		return err
	}
	actual, err := inventory(root)
	if err != nil {
		return err
	}
	delete(actual, "manifest.json")
	if len(actual) != len(expected) {
		return errors.New("bundle inventory differs")
	}
	for name, hash := range expected {
		if actual[name] != hash {
			return fmt.Errorf("artifact integrity mismatch: %s", name)
		}
	}
	for _, required := range []string{"session.json", "source-ir.json", "report.json", "report.html", "internal-checks.json", "diagram.svg"} {
		if expected[required] == "" {
			return fmt.Errorf("missing required artifact: %s", required)
		}
	}
	file, err = os.Open(filepath.Join(root, "session.json"))
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	session, err := Decode(file)
	if err != nil {
		return err
	}
	if err = session.ValidateConfirmation(); err != nil {
		return err
	}
	if expected[session.Spec.Name+".kicad_sch"] == "" || expected[session.Spec.Name+".kicad_pro"] == "" {
		return errors.New("native schematic/project missing")
	}
	return nil
}
func runNative(ctx context.Context, stage, path string, opts Options, report *Report) error {
	if opts.KiCadCLI == "" && !opts.RequireERC {
		report.Checks = append(report.Checks, Check{"KiCad ERC", "not_checked", "Not requested. Re-run into a new directory with --require-erc to require installed-KiCad validation.", ""})
		return nil
	}
	cli, err := checks.DiscoverCLI(opts.KiCadCLI)
	if err != nil {
		if opts.RequireERC || opts.KiCadCLI != "" {
			return fmt.Errorf("KiCad unavailable: %w", err)
		}
		report.Checks = append(report.Checks, Check{"KiCad ERC", "not_checked", "KiCad CLI unavailable; install KiCad or pass --kicad-cli. This is not a native-validated result.", ""})
		return nil
	}
	runner := opts.Runner
	if runner == nil {
		runner = SafeRunner{}
	}
	nativeCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	result, err := checks.RunERCWithRunner(nativeCtx, runner, cli, path, checks.Options{KeepArtifacts: true, ArtifactDir: filepath.Join(stage, "native-evidence"), Timeout: 20 * time.Second})
	evidenceRaw, e := json.MarshalIndent(result, "", "  ")
	if e != nil {
		return e
	}
	if e = os.WriteFile(filepath.Join(stage, "erc.json"), evidenceRaw, 0644); e != nil {
		return e
	}
	if err != nil || result.Status != checks.CheckStatusPass || result.KiCadVersion == "" {
		return fmt.Errorf("KiCad ERC did not pass (status %s, tool error %s): %v", result.Status, result.ToolErrorKind, err)
	}
	report.Checks = append(report.Checks, Check{"KiCad ERC", "checked", "Native ERC passed with " + result.KiCadVersion + "; all severities included.", "erc.json"})
	report.Status = "erc_checked_candidate"
	preview := runner.Run(nativeCtx, stage, cli.Path, "sch", "export", "svg", "--output", stage+string(os.PathSeparator), path)
	if e = writeJSON(stage, "preview-command.json", preview); e != nil {
		return e
	}
	previewPath := filepath.Join(stage, report.Spec.Name+".svg")
	if preview.Err == nil {
		if info, statErr := os.Stat(previewPath); statErr == nil && info.Size() > 0 && info.Size() <= 16<<20 {
			report.Diagram = report.Spec.Name + ".svg"
			report.DiagramDescription = "Native KiCad SVG export of the generated schematic."
			report.Checks = append(report.Checks, Check{"KiCad preview", "checked", "Native schematic preview generated without manual artifact changes.", "preview-command.json"})
			return nil
		}
	}
	report.Checks = append(report.Checks, Check{"KiCad preview", "not_checked", "SVG export unavailable or invalid; functional diagram is shown instead.", "preview-command.json"})
	return nil
}
