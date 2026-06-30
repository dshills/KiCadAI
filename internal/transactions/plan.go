package transactions

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	kicaddesign "kicadai/internal/kicadfiles/design"
	"kicadai/internal/libraryresolver"
	"kicadai/internal/preservation"
	"kicadai/internal/reports"
)

type Plan struct {
	Target       string               `json:"target"`
	Operations   []PlannedOperation   `json:"operations"`
	Preservation *preservation.Report `json:"preservation,omitempty"`
	Issues       []reports.Issue      `json:"issues"`
}

type PlannedOperation struct {
	ID         string             `json:"id"`
	Index      int                `json:"index"`
	Op         OperationKind      `json:"op"`
	Refs       []string           `json:"refs,omitempty"`
	Nets       []string           `json:"nets,omitempty"`
	Artifacts  []reports.Artifact `json:"artifacts,omitempty"`
	Supported  bool               `json:"supported"`
	WillWrite  bool               `json:"will_write,omitempty"`
	Capability string             `json:"capability,omitempty"`
}

type PlanOptions struct {
	LibraryIndex             *libraryresolver.LibraryIndex `json:"-"`
	LibraryIssues            []reports.Issue               `json:"-"`
	RequireLibraryValidation bool                          `json:"-"`
}

func PlanTransaction(target string, tx Transaction) Plan {
	return PlanTransactionWithOptions(target, tx, PlanOptions{})
}

func PlanTransactionWithOptions(target string, tx Transaction, opts PlanOptions) Plan {
	if strings.TrimSpace(target) == "" {
		target = "."
	}
	plan := Plan{Target: filepath.ToSlash(target), Operations: []PlannedOperation{}, Issues: []reports.Issue{}}
	validation := Validate(tx)
	plan.Issues = append(plan.Issues, validation.Issues...)
	plan.Issues = append(plan.Issues, opts.LibraryIssues...)
	if opts.RequireLibraryValidation && opts.LibraryIndex == nil {
		plan.Issues = append(plan.Issues, reports.Issue{
			Code:       reports.CodeUnknownSymbolLibrary,
			Severity:   reports.SeverityWarning,
			Path:       "transaction.library",
			Message:    "library resolver roots are not configured; transaction library validation was skipped",
			Suggestion: "pass --symbols-root and --footprints-root to validate library IDs",
		})
	}
	projectName := projectNameFromTransaction(tx)
	if projectName == "" {
		projectName = "generated_design"
	}
	existingProject := existingProjectTarget(target)
	var existing *kicaddesign.Design
	if existingProject {
		loaded, err := kicaddesign.ReadProjectDirectory(target)
		if err != nil {
			plan.Issues = append(plan.Issues, reports.Issue{
				Code:     reports.CodeValidationFailed,
				Severity: reports.SeverityBlocked,
				Path:     "transaction.target",
				Message:  err.Error(),
			})
		} else {
			existing = &loaded
		}
	}
	addedRefs := map[string]struct{}{}
	operationIDs := map[string]struct{}{}
	operationIDCounts := map[string]int{}
	for i, op := range tx.Operations {
		op.Index = i
		planned := PlannedOperation{Index: i, Op: op.Op, Supported: supportedPlanOperation(op.Op)}
		if existingProject {
			planned.Supported = supportedExistingPlanOperation(op.Op)
		}
		plan.Issues = append(plan.Issues, populatePlanFields(&planned, op, target, projectName)...)
		planned.ID = uniquePlannedOperationID(plannedOperationID(planned, op), operationIDs, operationIDCounts)
		if existing != nil {
			plan.Issues = append(plan.Issues, existingProjectIssues(*existing, op, addedRefs)...)
		}
		if opts.LibraryIndex != nil {
			plan.Issues = append(plan.Issues, transactionLibraryIssues(*opts.LibraryIndex, op)...)
			enrichPlannedOperationWithLibrary(*opts.LibraryIndex, &planned, op)
		}
		if !planned.Supported {
			plan.Issues = append(plan.Issues, reports.Issue{
				Code:     reports.CodeUnsupportedOperation,
				Severity: reports.SeverityBlocked,
				Path:     "operations[" + strconv.Itoa(i) + "].op",
				Message:  "operation " + string(op.Op) + " is not supported by this planning mode",
			})
		}
		plan.Operations = append(plan.Operations, planned)
		if existingProject && op.Op == OpAddSymbol {
			var payload AddSymbolOperation
			if decodeRaw(op, &payload) == nil && strings.TrimSpace(payload.Ref) != "" {
				addedRefs[strings.TrimSpace(payload.Ref)] = struct{}{}
			}
		}
	}
	annotatePlanIssueOperationIDs(&plan)
	if existingProject {
		plan.Preservation = preservationReportForPlan(&plan)
	}
	return plan
}

func transactionLibraryIssues(index libraryresolver.LibraryIndex, op Operation) []reports.Issue {
	switch op.Op {
	case OpAddSymbol:
		var payload AddSymbolOperation
		if decodeRaw(op, &payload) != nil {
			return nil
		}
		if _, ok := libraryresolver.ResolveSymbol(index, payload.LibraryID); !ok {
			return []reports.Issue{missingTransactionLibraryIssue(op.Index, "library_id", "symbol library record not found: "+payload.LibraryID)}
		}
	case OpAssignFootprint:
		var payload AssignFootprintOperation
		if decodeRaw(op, &payload) != nil {
			return nil
		}
		if _, ok := libraryresolver.ResolveFootprint(index, payload.FootprintID); !ok {
			return []reports.Issue{missingTransactionLibraryIssue(op.Index, "footprint_id", "footprint library record not found: "+payload.FootprintID)}
		}
	case OpPlaceFootprint:
		var payload PlaceFootprintOperation
		if decodeRaw(op, &payload) != nil || strings.TrimSpace(payload.FootprintID) == "" {
			return nil
		}
		if _, ok := libraryresolver.ResolveFootprint(index, payload.FootprintID); !ok {
			return []reports.Issue{missingTransactionLibraryIssue(op.Index, "footprint_id", "footprint library record not found: "+payload.FootprintID)}
		}
	case OpConnect:
		var payload ConnectOperation
		if decodeRaw(op, &payload) != nil {
			return nil
		}
		return []reports.Issue{{
			Code:       reports.CodePinmapUnverified,
			Severity:   reports.SeverityWarning,
			Path:       "operations[" + strconv.Itoa(op.Index) + "]",
			Message:    "connect operations require verified pinmaps before fabrication export",
			Refs:       []string{payload.From.Ref, payload.To.Ref},
			Suggestion: "run pinmap validate with resolver roots after assigning footprints",
		}}
	}
	return nil
}

func missingTransactionLibraryIssue(index int, field string, message string) reports.Issue {
	return reports.Issue{
		Code:       reports.CodeMissingFile,
		Severity:   reports.SeverityError,
		Path:       "operations[" + strconv.Itoa(index) + "]." + field,
		Message:    message,
		Suggestion: "run library search commands or configure KiCad library roots",
	}
}

func enrichPlannedOperationWithLibrary(index libraryresolver.LibraryIndex, planned *PlannedOperation, op Operation) {
	switch op.Op {
	case OpAddSymbol:
		var payload AddSymbolOperation
		if decodeRaw(op, &payload) != nil {
			return
		}
		candidates := libraryresolver.CompatibleFootprints(index, payload.LibraryID, libraryresolver.MatchOptions{Limit: 1})
		if len(candidates) > 0 {
			planned.Capability = "compatible footprint candidate: " + candidates[0].FootprintID
		}
	case OpPlaceFootprint:
		var payload PlaceFootprintOperation
		if decodeRaw(op, &payload) != nil || strings.TrimSpace(payload.FootprintID) == "" {
			return
		}
		if footprint, ok := libraryresolver.ResolveFootprint(index, payload.FootprintID); ok {
			planned.Capability = "resolver footprint geometry available: " + footprint.FootprintID
		}
	}
}

func supportedExistingPlanOperation(kind OperationKind) bool {
	switch kind {
	case OpAddSymbol, OpAssignFootprint, OpPlaceFootprint, OpRoute, OpAddZone, OpAddNoConnect, OpWriteProject:
		return true
	default:
		return false
	}
}

func supportedPlanOperation(kind OperationKind) bool {
	switch kind {
	case OpCreateProject, OpSetBoardOutline, OpAddSymbol, OpConnect, OpAssignFootprint, OpPlaceFootprint, OpRoute, OpAddZone, OpAddNoConnect, OpWriteProject:
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

func existingProjectIssues(design kicaddesign.Design, op Operation, addedRefs map[string]struct{}) []reports.Issue {
	var issues []reports.Issue
	if touchesDesign(op.Op) && hasUnsupportedImportedContent(design) {
		issues = append(issues, reports.Issue{
			Code:     reports.CodePreservationConflict,
			Severity: reports.SeverityBlocked,
			Path:     "operations[" + strconv.Itoa(op.Index) + "]",
			Message:  "existing project contains preserved unsupported content; mutation planning is blocked until preservation-aware apply is implemented",
		})
	}
	switch op.Op {
	case OpRemoveSymbol:
		issues = append(issues, reports.Issue{
			Code:     reports.CodeUnsafeRemove,
			Severity: reports.SeverityBlocked,
			Path:     "operations[" + strconv.Itoa(op.Index) + "]",
			Message:  "removing symbols from imported projects is unsafe until dependency analysis is implemented",
		})
	case OpAssignFootprint:
		var payload AssignFootprintOperation
		if decodeRaw(op, &payload) == nil {
			issues = append(issues, refIssues(design, op.Index, payload.Ref, addedRefs)...)
		}
	case OpPlaceFootprint:
		var payload PlaceFootprintOperation
		if decodeRaw(op, &payload) == nil {
			issues = append(issues, refIssues(design, op.Index, payload.Ref, addedRefs)...)
		}
	case OpConnect:
		var payload ConnectOperation
		if decodeRaw(op, &payload) == nil {
			issues = append(issues, refIssues(design, op.Index, payload.From.Ref, addedRefs)...)
			issues = append(issues, refIssues(design, op.Index, payload.To.Ref, addedRefs)...)
			issues = append(issues, reports.Issue{
				Code:     reports.CodePinmapUnverified,
				Severity: reports.SeverityBlocked,
				Path:     "operations[" + strconv.Itoa(op.Index) + "]",
				Message:  "connecting imported symbols requires verified pin maps",
				Refs:     []string{payload.From.Ref, payload.To.Ref},
			})
		}
	}
	return issues
}

func touchesDesign(kind OperationKind) bool {
	switch kind {
	case OpAddSymbol, OpAssignFootprint, OpPlaceFootprint, OpConnect, OpRoute, OpAddZone, OpRemoveSymbol:
		return true
	default:
		return false
	}
}

func hasUnsupportedImportedContent(design kicaddesign.Design) bool {
	if design.Schematic != nil && len(design.Schematic.RawItems) > 0 {
		return true
	}
	if design.PCB != nil && len(design.PCB.Preserved) > 0 {
		return true
	}
	return false
}

func preservationReportForPlan(plan *Plan) *preservation.Report {
	report := preservation.New(preservation.ScopeImported)
	report.Files = append(report.Files, preservation.File{
		Path:       filepath.ToSlash(plan.Target),
		Kind:       "project",
		Ownership:  preservation.OwnershipImportedUser,
		Mutability: preservation.MutabilityPlanOnly,
	})
	issuesByOperationID := map[string][]reports.Issue{}
	for _, issue := range plan.Issues {
		operationID := strings.TrimSpace(issue.OperationID)
		if operationID == "" {
			continue
		}
		issuesByOperationID[operationID] = append(issuesByOperationID[operationID], issue)
	}
	for _, operation := range plan.Operations {
		operationID := strings.TrimSpace(operation.ID)
		issues := issuesByOperationID[operationID]
		mutability := importedPlanMutability(operation, issues)
		reason := importedPlanMutabilityReason(operation, mutability, issues)
		report.OperationReviews = append(report.OperationReviews, preservation.OperationReviewFor(
			operation.Index,
			string(operation.Op),
			operationID,
			mutability,
			reason,
			issues,
		))
	}
	report.Normalize()
	return &report
}

func importedPlanMutability(operation PlannedOperation, issues []reports.Issue) preservation.Mutability {
	if !operation.Supported || reports.HasBlockingIssue(issues) {
		return preservation.MutabilityUnsafe
	}
	switch operation.Op {
	case OpAddSymbol, OpAddNoConnect:
		return preservation.MutabilitySafeAdd
	default:
		return preservation.MutabilityPlanOnly
	}
}

func importedPlanMutabilityReason(operation PlannedOperation, mutability preservation.Mutability, issues []reports.Issue) string {
	if !operation.Supported {
		return "operation is not supported for imported project planning"
	}
	if reports.HasBlockingIssue(issues) {
		return "operation is blocked by imported-project safety checks"
	}
	switch mutability {
	case preservation.MutabilitySafeAdd:
		return "operation adds isolated generated content without modifying existing KiCad objects"
	case preservation.MutabilityPlanOnly:
		return "operation requires preservation-aware apply review before writing an imported project"
	default:
		return "operation mutability is unsafe for imported projects"
	}
}

func refIssues(design kicaddesign.Design, index int, ref string, addedRefs map[string]struct{}) []reports.Issue {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil
	}
	if _, ok := addedRefs[ref]; ok {
		return nil
	}
	if design.Schematic == nil {
		return []reports.Issue{{
			Code:     reports.CodeMissingFile,
			Severity: reports.SeverityBlocked,
			Path:     "operations[" + strconv.Itoa(index) + "].ref",
			Message:  "cannot resolve reference " + ref + " because the imported project has no root schematic",
			Refs:     []string{ref},
		}}
	}
	count := 0
	for _, symbol := range design.Schematic.Symbols {
		if symbol.Reference == ref {
			count++
		}
	}
	if count == 1 {
		return nil
	}
	code := reports.CodeInvalidArgument
	message := "reference " + ref + " does not exist in imported schematic"
	if count > 1 {
		code = reports.CodeAmbiguousReference
		message = "reference " + ref + " is ambiguous in imported schematic"
	}
	return []reports.Issue{{
		Code:     code,
		Severity: reports.SeverityBlocked,
		Path:     "operations[" + strconv.Itoa(index) + "].ref",
		Message:  message,
		Refs:     []string{ref},
	}}
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
