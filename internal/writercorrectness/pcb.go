package writercorrectness

import (
	"cmp"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	kpcb "kicadai/internal/kicadfiles/pcb"
	"kicadai/internal/reports"
)

type PCBSnapshot struct {
	Path           string              `json:"path"`
	NetCount       int                 `json:"net_count"`
	FootprintCount int                 `json:"footprint_count"`
	PadCount       int                 `json:"pad_count"`
	Nets           []PCBNetSnapshot    `json:"nets,omitempty"`
	Footprints     []FootprintSnapshot `json:"footprints,omitempty"`
}

type PCBNetSnapshot struct {
	Code int    `json:"code"`
	Name string `json:"name"`
}

type FootprintSnapshot struct {
	Reference   string        `json:"reference"`
	LibraryID   string        `json:"library_id,omitempty"`
	PadCount    int           `json:"pad_count"`
	PositionXIU int64         `json:"position_x_iu"`
	PositionYIU int64         `json:"position_y_iu"`
	Pads        []PadSnapshot `json:"pads,omitempty"`
}

type PadSnapshot struct {
	Name     string `json:"name"`
	Type     string `json:"type,omitempty"`
	Shape    string `json:"shape,omitempty"`
	NetCode  int    `json:"net_code"`
	NetName  string `json:"net_name,omitempty"`
	WidthIU  int64  `json:"width_iu"`
	HeightIU int64  `json:"height_iu"`
	DrillIU  int64  `json:"drill_iu,omitempty"`
}

func CheckPCBFootprintPads(target Target) (PCBSnapshot, []CheckResult) {
	if target.PCBPath == "" {
		return PCBSnapshot{}, []CheckResult{{
			Name:     CheckPCBParse,
			Status:   CheckSkipped,
			Required: false,
			Summary:  "no PCB resolved",
		}, {
			Name:     CheckPCBNetTable,
			Status:   CheckSkipped,
			Required: false,
			Summary:  "no PCB resolved",
		}, {
			Name:     CheckFootprintPadNets,
			Status:   CheckSkipped,
			Required: false,
			Summary:  "no PCB resolved",
		}}
	}

	board, err := kpcb.ReadFile(filepath.FromSlash(target.PCBPath))
	if err != nil {
		issue := BlockingIssue(reports.CodeValidationFailed, target.PCBPath, err.Error())
		return PCBSnapshot{Path: target.PCBPath}, []CheckResult{{
			Name:     CheckPCBParse,
			Required: true,
			Issues:   []reports.Issue{issue},
			Summary:  "PCB parse failed",
		}}
	}

	snapshot := snapshotPCB(target.PCBPath, board)
	parseIssues := issuesFromPCBValidate(target.PCBPath, kpcb.Validate(board))
	netIssues := validatePCBNetTable(board, target.PCBPath)
	padIssues := validateFootprintPads(board, target.PCBPath)
	return snapshot, []CheckResult{{
		Name:     CheckPCBParse,
		Required: true,
		Issues:   parseIssues,
		Summary:  "PCB parsed",
	}, {
		Name:     CheckPCBNetTable,
		Required: true,
		Issues:   netIssues,
		Summary:  fmt.Sprintf("%d net(s)", len(board.Nets)),
	}, {
		Name:     CheckFootprintPadNets,
		Required: true,
		Issues:   padIssues,
		Summary:  fmt.Sprintf("%d footprint(s), %d pad(s)", len(board.Footprints), snapshot.PadCount),
	}}
}

func snapshotPCB(path string, board kpcb.PCBFile) PCBSnapshot {
	snapshot := PCBSnapshot{Path: slashPath(path), NetCount: len(board.Nets), FootprintCount: len(board.Footprints)}
	snapshot.Nets = make([]PCBNetSnapshot, 0, len(board.Nets))
	snapshot.Footprints = make([]FootprintSnapshot, 0, len(board.Footprints))
	for _, net := range board.Nets {
		snapshot.Nets = append(snapshot.Nets, PCBNetSnapshot{Code: net.Code, Name: net.Name})
	}
	for _, footprint := range board.Footprints {
		fp := FootprintSnapshot{
			Reference:   strings.TrimSpace(footprint.Reference),
			LibraryID:   strings.TrimSpace(footprint.LibraryID),
			PadCount:    len(footprint.Pads),
			PositionXIU: int64(footprint.Position.X),
			PositionYIU: int64(footprint.Position.Y),
		}
		fp.Pads = make([]PadSnapshot, 0, len(footprint.Pads))
		for _, pad := range footprint.Pads {
			fp.Pads = append(fp.Pads, PadSnapshot{
				Name:     strings.TrimSpace(pad.Name),
				Type:     strings.TrimSpace(pad.Type),
				Shape:    strings.TrimSpace(pad.Shape),
				NetCode:  pad.NetCode,
				NetName:  strings.TrimSpace(pad.NetName),
				WidthIU:  int64(pad.Size.X),
				HeightIU: int64(pad.Size.Y),
				DrillIU:  int64(pad.Drill),
			})
			snapshot.PadCount++
		}
		slices.SortStableFunc(fp.Pads, func(a, b PadSnapshot) int {
			return cmp.Or(
				strings.Compare(a.Name, b.Name),
				a.NetCode-b.NetCode,
				strings.Compare(a.Shape, b.Shape),
				cmp.Compare(a.WidthIU, b.WidthIU),
				cmp.Compare(a.HeightIU, b.HeightIU),
			)
		})
		snapshot.Footprints = append(snapshot.Footprints, fp)
	}
	slices.SortFunc(snapshot.Nets, func(a, b PCBNetSnapshot) int {
		return a.Code - b.Code
	})
	slices.SortFunc(snapshot.Footprints, func(a, b FootprintSnapshot) int {
		return strings.Compare(a.Reference, b.Reference)
	})
	return snapshot
}

func validatePCBNetTable(board kpcb.PCBFile, path string) []reports.Issue {
	var issues []reports.Issue
	seenCodes := map[int]struct{}{}
	seenNames := map[string]struct{}{}
	for _, net := range board.Nets {
		if net.Code < 0 {
			issues = append(issues, reports.Issue{Code: reports.CodeInvalidNetAssignment, Severity: reports.SeverityError, Path: slashPath(path), Message: "PCB net code must be non-negative"})
		}
		if _, ok := seenCodes[net.Code]; ok {
			issues = append(issues, reports.Issue{Code: reports.CodeInvalidNetAssignment, Severity: reports.SeverityError, Path: slashPath(path), Message: "duplicate PCB net code"})
		}
		seenCodes[net.Code] = struct{}{}
		name := strings.TrimSpace(net.Name)
		if net.Code > 0 && name == "" {
			issues = append(issues, reports.Issue{Code: reports.CodeInvalidNetAssignment, Severity: reports.SeverityError, Path: slashPath(path), Message: "named PCB net is required for non-zero net code"})
		}
		if name != "" {
			if _, ok := seenNames[name]; ok {
				issues = append(issues, reports.Issue{Code: reports.CodeInvalidNetAssignment, Severity: reports.SeverityError, Path: slashPath(path), Message: "duplicate PCB net name", Nets: []string{name}})
			}
			seenNames[name] = struct{}{}
		}
	}
	return issues
}

func validateFootprintPads(board kpcb.PCBFile, path string) []reports.Issue {
	validNetCodes := map[int]string{}
	for _, net := range board.Nets {
		validNetCodes[net.Code] = net.Name
	}
	seenRefs := map[string]struct{}{}
	var issues []reports.Issue
	for _, footprint := range board.Footprints {
		ref := strings.TrimSpace(footprint.Reference)
		if ref == "" {
			issues = append(issues, reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Path: slashPath(path), Message: "PCB footprint is missing a reference"})
		} else if _, ok := seenRefs[ref]; ok {
			issues = append(issues, reports.Issue{Code: reports.CodeDuplicateReference, Severity: reports.SeverityError, Path: slashPath(path), Message: "duplicate PCB footprint reference", Refs: []string{ref}})
		}
		seenRefs[ref] = struct{}{}
		if strings.TrimSpace(footprint.LibraryID) == "" {
			issues = append(issues, reports.Issue{Code: reports.CodeUnknownFootprintLibrary, Severity: reports.SeverityWarning, Path: slashPath(path) + ".footprints." + ref, Message: "PCB footprint has no library ID", Refs: []string{ref}})
		}
		for _, pad := range footprint.Pads {
			padPath := slashPath(path) + ".footprints." + ref + ".pads." + strings.TrimSpace(pad.Name)
			if strings.TrimSpace(pad.Name) == "" {
				issues = append(issues, reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Path: padPath, Message: "PCB pad is missing a name", Refs: []string{ref}})
			}
			if pad.Size.X <= 0 || pad.Size.Y <= 0 {
				issues = append(issues, reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Path: padPath, Message: "PCB pad size must be positive", Refs: []string{ref}})
			}
			if _, ok := validNetCodes[pad.NetCode]; !ok {
				issues = append(issues, reports.Issue{Code: reports.CodeInvalidNetAssignment, Severity: reports.SeverityError, Path: padPath, Message: "PCB pad references missing net code", Refs: []string{ref}, Nets: []string{pad.NetName}})
				continue
			}
			if want := validNetCodes[pad.NetCode]; strings.TrimSpace(pad.NetName) != strings.TrimSpace(want) {
				issues = append(issues, reports.Issue{Code: reports.CodeInvalidNetAssignment, Severity: reports.SeverityError, Path: padPath, Message: "PCB pad net name does not match net table", Refs: []string{ref}, Nets: []string{pad.NetName, want}})
			}
		}
	}
	return issues
}

func issuesFromPCBValidate(path string, err error) []reports.Issue {
	if err == nil {
		return nil
	}
	return []reports.Issue{{
		Code:     reports.CodeValidationFailed,
		Severity: reports.SeverityError,
		Path:     slashPath(path),
		Message:  err.Error(),
	}}
}
