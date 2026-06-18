package repair

import (
	"os"
	"path/filepath"
	"strings"

	"kicadai/internal/inspect"
	"kicadai/internal/reports"
)

type TargetKind string

const (
	TargetProjectDir TargetKind = "project_dir"
	TargetProject    TargetKind = "project"
	TargetSchematic  TargetKind = "schematic"
	TargetPCB        TargetKind = "pcb"
	TargetUnknown    TargetKind = "unknown"
)

type Target struct {
	Path       string                  `json:"path"`
	Root       string                  `json:"root,omitempty"`
	Kind       TargetKind              `json:"kind"`
	Generated  bool                    `json:"generated"`
	Mutable    bool                    `json:"mutable"`
	Bundle     *Bundle                 `json:"bundle,omitempty"`
	Inspection *inspect.ProjectSummary `json:"inspection,omitempty"`
	Issues     []reports.Issue         `json:"issues,omitempty"`
}

type HydrateOptions struct {
	Bundle         *Bundle
	InspectProject func(path string) (inspect.ProjectSummary, error)
}

func HydrateTarget(path string, opts HydrateOptions) Target {
	path = strings.TrimSpace(path)
	target := Target{Path: filepath.ToSlash(path), Kind: TargetUnknown}
	if target.Path == "" {
		target.Issues = append(target.Issues, targetIssue(reports.CodeInvalidArgument, "target", "repair target is required"))
		return target
	}
	info, err := os.Stat(path)
	if err != nil {
		target.Issues = append(target.Issues, targetIssue(reports.CodeMissingFile, "target", err.Error()))
		return target
	}
	root := path
	if info.IsDir() {
		target.Kind = TargetProjectDir
	} else {
		target.Kind = targetKindForPath(path)
		root = filepath.Dir(path)
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		target.Issues = append(target.Issues, targetIssue(reports.CodeInvalidArgument, "target", err.Error()))
		return target
	}
	target.Root = filepath.ToSlash(absRoot)
	target.Bundle = opts.Bundle
	target.Generated = opts.Bundle != nil && opts.Bundle.Generated
	inspector := opts.InspectProject
	if inspector == nil {
		inspector = inspect.Project
	}
	if summary, err := inspector(absRoot); err != nil {
		target.Issues = append(target.Issues, targetIssue(reports.CodeValidationFailed, "target.inspect", err.Error()))
	} else {
		target.Inspection = &summary
		target.Issues = append(target.Issues, summary.Issues...)
		if hasUnsupportedContent(summary) {
			target.Issues = append(target.Issues, reports.Issue{
				Code:     reports.CodePreservationConflict,
				Severity: reports.SeverityBlocked,
				Path:     "target",
				Message:  "target contains preserved unsupported content; repair apply is blocked",
			})
		}
	}
	if !target.Generated {
		target.Issues = append(target.Issues, reports.Issue{
			Code:     reports.CodePreservationConflict,
			Severity: reports.SeverityBlocked,
			Path:     "target.generated",
			Message:  "repair apply requires generated KiCadAI provenance",
		})
	}
	if opts.Bundle == nil || opts.Bundle.Transaction == nil {
		target.Issues = append(target.Issues, reports.Issue{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityBlocked,
			Path:     "target.transaction",
			Message:  "repair apply requires generated transaction provenance for safe persistence",
		})
	}
	target.Mutable = target.Generated && opts.Bundle != nil && opts.Bundle.Transaction != nil && !reports.HasBlockingIssue(target.Issues)
	return target
}

func targetKindForPath(path string) TargetKind {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".kicad_pro":
		return TargetProject
	case ".kicad_sch":
		return TargetSchematic
	case ".kicad_pcb":
		return TargetPCB
	default:
		return TargetUnknown
	}
}

func hasUnsupportedContent(summary inspect.ProjectSummary) bool {
	return len(summary.Unsupported) > 0 ||
		len(summary.PreservationOnly) > 0 ||
		(summary.Schematic != nil && (len(summary.Schematic.Unsupported) > 0 || len(summary.Schematic.PreservationOnly) > 0)) ||
		(summary.PCB != nil && (len(summary.PCB.Unsupported) > 0 || len(summary.PCB.PreservationOnly) > 0))
}

func targetIssue(code reports.Code, path string, message string) reports.Issue {
	severity := reports.SeverityBlocked
	if code == reports.CodeMissingFile {
		severity = reports.SeverityError
	}
	return reports.Issue{Code: code, Severity: severity, Path: path, Message: message}
}
