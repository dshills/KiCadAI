package transactions

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"kicadai/internal/reports"
)

type Plan struct {
	Target     string             `json:"target"`
	Operations []PlannedOperation `json:"operations"`
	Issues     []reports.Issue    `json:"issues"`
}

type PlannedOperation struct {
	Index      int                `json:"index"`
	Op         OperationKind      `json:"op"`
	Refs       []string           `json:"refs,omitempty"`
	Nets       []string           `json:"nets,omitempty"`
	Artifacts  []reports.Artifact `json:"artifacts,omitempty"`
	Supported  bool               `json:"supported"`
	WillWrite  bool               `json:"will_write,omitempty"`
	Capability string             `json:"capability,omitempty"`
}

func PlanTransaction(target string, tx Transaction) Plan {
	if strings.TrimSpace(target) == "" {
		target = "."
	}
	plan := Plan{Target: filepath.ToSlash(target), Operations: []PlannedOperation{}, Issues: []reports.Issue{}}
	validation := Validate(tx)
	plan.Issues = append(plan.Issues, validation.Issues...)
	projectName := projectNameFromTransaction(tx)
	if projectName == "" {
		projectName = "generated_design"
	}
	if existingProjectTarget(target) {
		plan.Issues = append(plan.Issues, reports.Issue{
			Code:       reports.CodeUnsupportedOperation,
			Severity:   reports.SeverityBlocked,
			Path:       "transaction.target",
			Message:    "planning mutations against existing projects is blocked until readers are implemented",
			Suggestion: "use an empty output directory for generated-project planning",
		})
	}
	for i, op := range tx.Operations {
		op.Index = i
		planned := PlannedOperation{Index: i, Op: op.Op, Supported: supportedPlanOperation(op.Op)}
		plan.Issues = append(plan.Issues, populatePlanFields(&planned, op, target, projectName)...)
		if !planned.Supported {
			plan.Issues = append(plan.Issues, reports.Issue{
				Code:     reports.CodeUnsupportedOperation,
				Severity: reports.SeverityBlocked,
				Path:     "operations[" + strconv.Itoa(i) + "].op",
				Message:  "operation " + string(op.Op) + " is not supported by generated-project planning",
			})
		}
		plan.Operations = append(plan.Operations, planned)
	}
	return plan
}

func supportedPlanOperation(kind OperationKind) bool {
	switch kind {
	case OpCreateProject, OpSetBoardOutline, OpAddSymbol, OpConnect, OpAssignFootprint, OpPlaceFootprint, OpRoute, OpAddZone, OpWriteProject:
		return true
	default:
		return false
	}
}

func populatePlanFields(planned *PlannedOperation, op Operation, target string, projectName string) []reports.Issue {
	decodeIssue := func(err error) []reports.Issue {
		if err == nil {
			return nil
		}
		return []reports.Issue{{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityError,
			Path:     "operations[" + strconv.Itoa(op.Index) + "]",
			Message:  err.Error(),
		}}
	}
	switch op.Op {
	case OpCreateProject:
		var payload CreateProjectOperation
		if err := decodeRaw(op, &payload); err != nil {
			return decodeIssue(err)
		} else if strings.TrimSpace(payload.Name) != "" {
			planned.Capability = "create generated project " + payload.Name
		}
	case OpAddSymbol:
		var payload AddSymbolOperation
		if err := decodeRaw(op, &payload); err != nil {
			return decodeIssue(err)
		} else {
			addRef(planned, payload.Ref)
		}
	case OpConnect:
		var payload ConnectOperation
		if err := decodeRaw(op, &payload); err != nil {
			return decodeIssue(err)
		} else {
			addRef(planned, payload.From.Ref)
			addRef(planned, payload.To.Ref)
			addNet(planned, payload.NetName)
		}
	case OpAssignFootprint:
		var payload AssignFootprintOperation
		if err := decodeRaw(op, &payload); err != nil {
			return decodeIssue(err)
		} else {
			addRef(planned, payload.Ref)
		}
	case OpPlaceFootprint:
		var payload PlaceFootprintOperation
		if err := decodeRaw(op, &payload); err != nil {
			return decodeIssue(err)
		} else {
			addRef(planned, payload.Ref)
			for _, pad := range payload.Pads {
				if pad.Net != nil {
					addNet(planned, *pad.Net)
				}
			}
		}
	case OpRoute:
		var payload RouteOperation
		if err := decodeRaw(op, &payload); err != nil {
			return decodeIssue(err)
		} else {
			addNet(planned, payload.NetName)
		}
	case OpAddZone:
		var payload AddZoneOperation
		if err := decodeRaw(op, &payload); err != nil {
			return decodeIssue(err)
		} else if payload.NetName != nil {
			addNet(planned, *payload.NetName)
		}
	case OpWriteProject:
		planned.WillWrite = true
		planned.Artifacts = plannedWriteArtifacts(target, projectName)
	}
	return nil
}

func plannedWriteArtifacts(target string, projectName string) []reports.Artifact {
	if strings.TrimSpace(target) == "" {
		target = "."
	}
	return []reports.Artifact{
		{Kind: reports.ArtifactKiCadProject, Path: filepath.ToSlash(filepath.Join(target, projectName+".kicad_pro")), Description: "planned project file"},
		{Kind: reports.ArtifactSchematic, Path: filepath.ToSlash(filepath.Join(target, projectName+".kicad_sch")), Description: "planned schematic file"},
		{Kind: reports.ArtifactPCB, Path: filepath.ToSlash(filepath.Join(target, projectName+".kicad_pcb")), Description: "planned PCB file"},
	}
}

func projectNameFromTransaction(tx Transaction) string {
	for _, op := range tx.Operations {
		if op.Op != OpCreateProject {
			continue
		}
		var payload CreateProjectOperation
		if decodeRaw(op, &payload) == nil && strings.TrimSpace(payload.Name) != "" {
			return strings.TrimSpace(payload.Name)
		}
	}
	if strings.TrimSpace(tx.Name) != "" {
		return strings.TrimSpace(tx.Name)
	}
	return ""
}

func existingProjectTarget(target string) bool {
	if strings.TrimSpace(target) == "" {
		return false
	}
	entries, err := os.ReadDir(target)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".kicad_pro") {
			return true
		}
	}
	return false
}

func decodeRaw(op Operation, target any) error {
	return json.Unmarshal(op.Raw, target)
}

func addRef(planned *PlannedOperation, ref string) {
	ref = strings.TrimSpace(ref)
	if ref == "" || contains(planned.Refs, ref) {
		return
	}
	planned.Refs = append(planned.Refs, ref)
}

func addNet(planned *PlannedOperation, net string) {
	net = strings.TrimSpace(net)
	if net == "" || contains(planned.Nets, net) {
		return
	}
	planned.Nets = append(planned.Nets, net)
}

func contains(values []string, value string) bool {
	for _, existing := range values {
		if existing == value {
			return true
		}
	}
	return false
}
