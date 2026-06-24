package repair

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"kicadai/internal/kicadfiles/checks"
	"kicadai/internal/reports"
)

const (
	postValidatorZoneRefill                 = "zone_refill"
	postValidatorZoneRefillBeforeValidation = "zone_refill_before_validation"
	postValidatorZoneRefillAfterRepair      = "zone_refill_after_repair_before_validation"
)

type ZoneRefillPolicy string

const (
	ZoneRefillNever                 ZoneRefillPolicy = "never"
	ZoneRefillBeforeValidation      ZoneRefillPolicy = "before_validation"
	ZoneRefillAfterRepairValidation ZoneRefillPolicy = "after_repair_before_validation"
)

type ZoneRefillOptions struct {
	Policy        ZoneRefillPolicy `json:"policy,omitempty"`
	KiCadCLI      string           `json:"kicad_cli,omitempty"`
	KeepArtifacts bool             `json:"keep_artifacts,omitempty"`
	ArtifactDir   string           `json:"artifact_dir,omitempty"`
}

type ZoneRefillResult struct {
	Policy    ZoneRefillPolicy   `json:"policy"`
	Ran       bool               `json:"ran"`
	Skipped   bool               `json:"skipped,omitempty"`
	Target    string             `json:"target,omitempty"`
	Command   []string           `json:"command,omitempty"`
	Artifacts []reports.Artifact `json:"artifacts,omitempty"`
	Issues    []reports.Issue    `json:"issues,omitempty"`
}

type ZoneRefillRunResult struct {
	Command   []string
	Artifacts []reports.Artifact
	Issues    []reports.Issue
}

type ZoneRefillRunner interface {
	RefillZones(context.Context, checks.KiCadCLI, string, ZoneRefillOptions) (ZoneRefillRunResult, error)
}

type KiCadZoneRefillRunner struct {
	Runner checks.Runner
}

func (runner KiCadZoneRefillRunner) RefillZones(ctx context.Context, cli checks.KiCadCLI, target string, opts ZoneRefillOptions) (ZoneRefillRunResult, error) {
	pcbPath, err := discoverZoneRefillPCB(target)
	if err != nil {
		return ZoneRefillRunResult{}, err
	}
	workspace, cleanup, err := checks.NewArtifactWorkspace("zone-refill", checks.Options{
		KeepArtifacts: opts.KeepArtifacts,
		ArtifactDir:   opts.ArtifactDir,
	})
	if err != nil {
		return ZoneRefillRunResult{}, err
	}
	defer cleanup()
	reportPath, err := workspace.Path("zone-refill-drc.json")
	if err != nil {
		return ZoneRefillRunResult{}, err
	}
	args := []string{
		"pcb", "drc",
		"--format", "json",
		"--severity-all",
		"--refill-zones",
		"--save-board",
		"--output", reportPath,
		pcbPath,
	}
	execRunner := runner.Runner
	if execRunner == nil {
		execRunner = checks.ExecRunner{}
	}
	command := append([]string{cli.Path}, args...)
	commandResult := execRunner.Run(ctx, filepath.Dir(pcbPath), cli.Path, args...)
	run := ZoneRefillRunResult{
		Command: command,
	}
	if opts.KeepArtifacts {
		run.Artifacts = []reports.Artifact{{
			Kind:        reports.ArtifactDRCReport,
			Path:        filepath.ToSlash(reportPath),
			Description: "KiCad DRC report produced while refilling zones",
		}}
	}
	if commandResult.Err != nil && commandResult.ExitCode != 1 {
		return run, fmt.Errorf("zone refill command failed with exit code %d: %s", commandResult.ExitCode, strings.TrimSpace(commandResult.Stderr))
	}
	return run, nil
}

func zoneRefillOptionsFromPostValidation(opts PostValidationOptions) ZoneRefillOptions {
	return ZoneRefillOptions{
		Policy:        ParseZoneRefillPolicy(opts.ZoneRefill),
		KiCadCLI:      opts.KiCadCLI,
		KeepArtifacts: opts.KeepArtifacts,
		ArtifactDir:   opts.ArtifactDir,
	}
}

func ParseZoneRefillPolicy(value string) ZoneRefillPolicy {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch ZoneRefillPolicy(normalized) {
	case "", ZoneRefillNever:
		return ZoneRefillNever
	case ZoneRefillBeforeValidation:
		return ZoneRefillBeforeValidation
	case ZoneRefillAfterRepairValidation:
		return ZoneRefillAfterRepairValidation
	default:
		return ZoneRefillPolicy(normalized)
	}
}

func RunZoneRefill(ctx context.Context, target Target, outputDir string, opts ZoneRefillOptions, runner ZoneRefillRunner) ZoneRefillResult {
	opts.Policy = normalizeZoneRefillPolicy(opts.Policy)
	result := ZoneRefillResult{Policy: opts.Policy, Target: filepath.ToSlash(strings.TrimSpace(outputDir))}
	if opts.Policy == ZoneRefillNever {
		result.Skipped = true
		return result
	}
	if !validZoneRefillPolicy(opts.Policy) {
		result.Issues = append(result.Issues, zoneRefillIssue(reports.CodeInvalidArgument, "zone_refill.policy", "unsupported zone refill policy "+string(opts.Policy)))
		return result
	}
	if err := ctx.Err(); err != nil {
		result.Issues = append(result.Issues, contextIssue(err))
		return result
	}
	if result.Target == "" {
		result.Issues = append(result.Issues, zoneRefillIssue(reports.CodeInvalidArgument, "zone_refill.target", "zone refill target is required"))
		return result
	}
	if strings.TrimSpace(opts.ArtifactDir) == "" {
		opts.ArtifactDir = defaultZoneRefillArtifactDir(result.Target)
	}
	if !target.Generated || target.Transaction == nil {
		result.Issues = append(result.Issues, reports.Issue{
			Code:     reports.CodePreservationConflict,
			Severity: reports.SeverityBlocked,
			Path:     "zone_refill.target",
			Message:  "zone refill requires generated KiCadAI project provenance",
		})
		return result
	}
	cli, err := checks.DiscoverCLI(opts.KiCadCLI)
	if err != nil {
		result.Issues = append(result.Issues, reports.Issue{
			Code:     reports.CodeSkippedExternalTool,
			Severity: reports.SeverityBlocked,
			Path:     "zone_refill.kicad_cli",
			Message:  err.Error(),
		})
		return result
	}
	if runner == nil {
		runner = KiCadZoneRefillRunner{}
	}
	run, err := runner.RefillZones(ctx, cli, result.Target, opts)
	result.Ran = true
	result.Command = append([]string(nil), run.Command...)
	result.Artifacts = append(result.Artifacts, run.Artifacts...)
	result.Issues = appendIssues(result.Issues, run.Issues)
	if ctxErr := ctx.Err(); ctxErr != nil {
		result.Issues = append(result.Issues, contextIssue(ctxErr))
	}
	if err != nil {
		result.Issues = append(result.Issues, reports.Issue{
			Code:     reports.CodeKiCadCLIFailed,
			Severity: reports.SeverityBlocked,
			Path:     "zone_refill",
			Message:  err.Error(),
		})
	}
	if opts.KeepArtifacts {
		if artifact, err := writeZoneRefillEvidence(opts, result); err == nil {
			result.Artifacts = append(result.Artifacts, artifact)
		} else {
			result.Issues = append(result.Issues, zoneRefillIssue(reports.CodeValidationFailed, "zone_refill.artifact", err.Error()))
		}
	}
	return result
}

func normalizeZoneRefillPolicy(policy ZoneRefillPolicy) ZoneRefillPolicy {
	switch policy {
	case "", ZoneRefillNever:
		return ZoneRefillNever
	case ZoneRefillBeforeValidation, ZoneRefillAfterRepairValidation:
		return policy
	default:
		return policy
	}
}

func validZoneRefillPolicy(policy ZoneRefillPolicy) bool {
	switch policy {
	case ZoneRefillNever, ZoneRefillBeforeValidation, ZoneRefillAfterRepairValidation:
		return true
	default:
		return false
	}
}

func (result ZoneRefillResult) PostApplyValidation() PostApplyValidation {
	return PostApplyValidation{
		Name:      zoneRefillValidationName(result.Policy),
		Issues:    result.Issues,
		Artifacts: result.Artifacts,
		Skipped:   result.Skipped,
	}
}

func zoneRefillValidationName(policy ZoneRefillPolicy) string {
	switch policy {
	case ZoneRefillBeforeValidation:
		return postValidatorZoneRefillBeforeValidation
	case ZoneRefillAfterRepairValidation:
		return postValidatorZoneRefillAfterRepair
	default:
		return postValidatorZoneRefill
	}
}

func writeZoneRefillEvidence(opts ZoneRefillOptions, result ZoneRefillResult) (reports.Artifact, error) {
	base := strings.TrimSpace(opts.ArtifactDir)
	if base == "" {
		base = filepath.Join(os.TempDir(), "kicadai-zone-refill")
	}
	if err := os.MkdirAll(base, 0o755); err != nil {
		return reports.Artifact{}, err
	}
	dir, err := os.MkdirTemp(base, "zone-refill-evidence-")
	if err != nil {
		return reports.Artifact{}, err
	}
	payload, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return reports.Artifact{}, err
	}
	path := filepath.Join(dir, "zone-refill.json")
	if err := os.WriteFile(path, []byte(string(payload)+"\n"), 0o644); err != nil {
		return reports.Artifact{}, err
	}
	return reports.Artifact{
		Kind:        reports.ArtifactValidationReport,
		Path:        filepath.ToSlash(path),
		Description: "KiCad zone refill evidence",
	}, nil
}

func zoneRefillIssue(code reports.Code, path string, message string) reports.Issue {
	return reports.Issue{Code: code, Severity: reports.SeverityBlocked, Path: path, Message: message}
}

func defaultZoneRefillArtifactDir(target string) string {
	native := filepath.FromSlash(strings.TrimSpace(target))
	if native == "" {
		return ""
	}
	if info, err := os.Stat(native); err == nil && !info.IsDir() {
		native = filepath.Dir(native)
	}
	return filepath.Join(native, ".kicadai", "artifacts")
}

func discoverZoneRefillPCB(target string) (string, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return "", fmt.Errorf("zone refill target is required")
	}
	info, err := os.Stat(target)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		if strings.EqualFold(filepath.Ext(target), ".kicad_pcb") {
			return target, nil
		}
		return "", fmt.Errorf("zone refill target is not a PCB file: %s", target)
	}
	var matches []string
	err = filepath.WalkDir(target, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch strings.ToLower(d.Name()) {
			case ".git", ".kicadai", "reports", "fabrication", "gerbers", "plot", "plots", "backup", "backups":
				if path != target {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if strings.EqualFold(filepath.Ext(path), ".kicad_pcb") && !strings.HasPrefix(filepath.Base(path), "_autosave-") {
			matches = append(matches, path)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("no .kicad_pcb file found in %s", target)
	}
	sort.Strings(matches)
	base := filepath.Base(filepath.Clean(target))
	for _, match := range matches {
		if strings.EqualFold(strings.TrimSuffix(filepath.Base(match), filepath.Ext(match)), base) {
			return match, nil
		}
	}
	if len(matches) > 1 {
		return "", fmt.Errorf("multiple .kicad_pcb files found in %s; specify a PCB file target", target)
	}
	return matches[0], nil
}
