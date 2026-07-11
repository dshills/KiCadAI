package writercorrectness

import (
	"encoding/json"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	kicaddesign "kicadai/internal/kicadfiles/design"
	"kicadai/internal/kicadfiles/schematic"
	"kicadai/internal/reports"
	"kicadai/internal/schematicpcb"
	"kicadai/internal/transactions"
)

type TransferSnapshot struct {
	SymbolCount       int                 `json:"symbol_count"`
	AssignedCount     int                 `json:"assigned_count"`
	PlacedCount       int                 `json:"placed_count"`
	NetHintCount      int                 `json:"net_hint_count"`
	RequiresLibraries bool                `json:"requires_libraries"`
	Placements        []TransferPlacement `json:"placements,omitempty"`
	Confidence        TransferConfidence  `json:"confidence"`
}

type TransferPlacement struct {
	Reference   string            `json:"reference"`
	FootprintID string            `json:"footprint_id"`
	PadNets     map[string]string `json:"pad_nets,omitempty"`
}

type TransferConfidence string

const (
	TransferConfidenceVerified  TransferConfidence = "verified"
	TransferConfidenceInferred  TransferConfidence = "inferred"
	TransferConfidenceSynthetic TransferConfidence = "synthetic"
	TransferConfidenceUnknown   TransferConfidence = "unknown"
)

func CheckSchematicToPCBTransfer(target Target) (TransferSnapshot, CheckResult) {
	return CheckSchematicToPCBTransferWithOptions(target, Options{})
}

func CheckSchematicToPCBTransferWithOptions(target Target, opts Options) (TransferSnapshot, CheckResult) {
	if target.SchematicPath == "" {
		return TransferSnapshot{Confidence: TransferConfidenceUnknown}, CheckResult{
			Name:     CheckSchematicPCBTransfer,
			Status:   CheckSkipped,
			Required: false,
			Summary:  "no schematic resolved",
		}
	}
	design, issues := transferDesign(target)
	if len(issues) > 0 {
		return TransferSnapshot{Confidence: TransferConfidenceUnknown}, CheckResult{
			Name:     CheckSchematicPCBTransfer,
			Required: true,
			Issues:   issues,
			Summary:  "failed to load schematic design for transfer",
		}
	}
	transferOpts := schematicpcb.Options{}
	if opts.HasLibraryIndex {
		transferOpts.LibraryIndex = &opts.LibraryIndex
	}
	result := schematicpcb.FromDesign(design, transferOpts)
	snapshot := TransferSnapshot{
		SymbolCount:       result.SymbolCount,
		AssignedCount:     result.AssignedCount,
		PlacedCount:       result.PlacedCount,
		NetHintCount:      result.NetHintCount,
		RequiresLibraries: result.RequiresLibraries,
		Confidence:        transferConfidence(result),
	}
	placements, placementIssues := transferPlacements(result.Transaction.Operations)
	snapshot.Placements = placements
	issues = append(issues, promoteTransferIssues(result.Issues)...)
	issues = append(issues, placementIssues...)
	issues = append(issues, validateTransferResult(snapshot)...)
	return snapshot, CheckResult{
		Name:     CheckSchematicPCBTransfer,
		Status:   StatusForIssues(issues),
		Required: true,
		Issues:   issues,
		Summary:  string(snapshot.Confidence) + " transfer for " + strconv.Itoa(snapshot.PlacedCount) + " footprint(s)",
	}
}

func transferDesign(target Target) (kicaddesign.Design, []reports.Issue) {
	if target.ProjectDir != "" && target.ProjectPath != "" {
		design, err := kicaddesign.ReadProjectDirectory(filepath.FromSlash(target.ProjectDir))
		if err != nil {
			return kicaddesign.Design{}, []reports.Issue{BlockingIssue(reports.CodeValidationFailed, target.ProjectDir, err.Error())}
		}
		return design, nil
	}
	root, err := schematic.ReadFile(filepath.FromSlash(target.SchematicPath))
	if err != nil {
		return kicaddesign.Design{}, []reports.Issue{BlockingIssue(reports.CodeValidationFailed, target.SchematicPath, err.Error())}
	}
	root.Filename = filepath.Base(filepath.FromSlash(target.SchematicPath))
	return kicaddesign.Design{
		Name:      strings.TrimSuffix(root.Filename, filepath.Ext(root.Filename)),
		Schematic: &root,
	}, nil
}

func transferPlacements(ops []transactions.Operation) ([]TransferPlacement, []reports.Issue) {
	var placements []TransferPlacement
	var issues []reports.Issue
	for index, op := range ops {
		if op.Op != transactions.OpPlaceFootprint {
			continue
		}
		var payload transactions.PlaceFootprintOperation
		if err := json.Unmarshal(op.Raw, &payload); err != nil {
			issues = append(issues, reports.Issue{
				Code:     reports.CodeValidationFailed,
				Severity: reports.SeverityError,
				Path:     "schematic_to_pcb.transaction." + strconv.Itoa(index),
				Message:  "failed to decode footprint placement operation: " + err.Error(),
			})
			continue
		}
		placement := TransferPlacement{
			Reference:   strings.TrimSpace(payload.Ref),
			FootprintID: strings.TrimSpace(payload.FootprintID),
		}
		for _, pad := range payload.Pads {
			if pad.Net == nil || strings.TrimSpace(pad.Name) == "" || strings.TrimSpace(*pad.Net) == "" {
				continue
			}
			if placement.PadNets == nil {
				placement.PadNets = map[string]string{}
			}
			placement.PadNets[pad.Name] = *pad.Net
		}
		placements = append(placements, placement)
	}
	slices.SortFunc(placements, func(a, b TransferPlacement) int {
		return strings.Compare(a.Reference, b.Reference)
	})
	return placements, issues
}

func transferConfidence(result schematicpcb.Result) TransferConfidence {
	switch {
	case result.AssignedCount == 0:
		return TransferConfidenceUnknown
	case result.NetHintCount > 0 && !result.RequiresLibraries:
		return TransferConfidenceInferred
	case result.NetHintCount > 0:
		return TransferConfidenceSynthetic
	default:
		return TransferConfidenceUnknown
	}
}

func validateTransferResult(snapshot TransferSnapshot) []reports.Issue {
	var issues []reports.Issue
	if snapshot.AssignedCount != snapshot.PlacedCount {
		issues = append(issues, reports.Issue{
			Code:     reports.CodeValidationFailed,
			Severity: reports.SeverityError,
			Path:     "schematic_to_pcb",
			Message:  "not every assigned schematic symbol produced a footprint placement",
		})
	}
	if snapshot.Confidence == TransferConfidenceUnknown && snapshot.AssignedCount > 0 {
		issues = append(issues, reports.Issue{
			Code:       reports.CodePinmapUnverified,
			Severity:   reports.SeverityWarning,
			Path:       "schematic_to_pcb",
			Message:    "schematic-to-PCB transfer has no pad net hints; pinmap confidence is unknown",
			Suggestion: "load resolver-backed symbol and footprint libraries before PCB realization",
		})
	}
	return issues
}

func promoteTransferIssues(issues []reports.Issue) []reports.Issue {
	promoted := append([]reports.Issue(nil), issues...)
	for i := range promoted {
		if promoted[i].Code == reports.CodeMissingFootprint {
			promoted[i].Severity = reports.SeverityError
		}
	}
	return promoted
}
