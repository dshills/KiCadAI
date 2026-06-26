package designworkflow

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"kicadai/internal/blocks"
	"kicadai/internal/components"
	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

type ComponentSelectionOptions struct {
	CatalogDir string
	Catalog    *components.Catalog
}

type ComponentSelectionResult struct {
	CatalogDir string                    `json:"catalog_dir,omitempty"`
	Selections []ComponentSelectionEntry `json:"selections,omitempty"`
	Stage      StageResult               `json:"stage"`
}

type ComponentSelectionEntry struct {
	InstanceID      string                            `json:"instance_id"`
	BlockID         string                            `json:"block_id"`
	Role            string                            `json:"role"`
	ComponentID     string                            `json:"component_id"`
	VariantID       string                            `json:"variant_id"`
	Manufacturer    string                            `json:"manufacturer,omitempty"`
	MPN             string                            `json:"mpn,omitempty"`
	SymbolID        string                            `json:"symbol_id,omitempty"`
	Value           string                            `json:"value,omitempty"`
	FootprintID     string                            `json:"footprint_id,omitempty"`
	Confidence      components.ConfidenceLevel        `json:"confidence"`
	ResolverChecked bool                              `json:"resolver_checked,omitempty"`
	PinMapChecked   bool                              `json:"pinmap_checked,omitempty"`
	Companions      []components.CompanionRequirement `json:"companions,omitempty"`
	Rejected        []components.CandidateRejection   `json:"rejected,omitempty"`
	Warnings        []reports.Issue                   `json:"warnings,omitempty"`
}

func SelectWorkflowComponents(ctx context.Context, registry blocks.Registry, plan BlockPlanResult, opts ComponentSelectionOptions) ComponentSelectionResult {
	catalogDir := componentCatalogDir(plan.Request.Components, opts)
	catalog := opts.Catalog
	if catalog == nil {
		if _, err := os.Stat(catalogDir); err != nil {
			issue := reports.Issue{Code: components.CodeCatalogReadFailed, Severity: reports.SeverityBlocked, Path: "component_policy.catalog_dir", Message: err.Error()}
			return ComponentSelectionResult{CatalogDir: catalogDir, Stage: NewStageResult(StageComponentSelection, []reports.Issue{issue})}
		}
		loaded, err := components.LoadCatalog(ctx, components.LoadOptions{CatalogDir: catalogDir})
		if err != nil {
			issue := reports.Issue{Code: components.CodeCatalogReadFailed, Severity: reports.SeverityBlocked, Path: "component_policy.catalog_dir", Message: err.Error()}
			return ComponentSelectionResult{CatalogDir: catalogDir, Stage: NewStageResult(StageComponentSelection, []reports.Issue{issue})}
		}
		catalog = loaded
	}
	issues := append([]reports.Issue(nil), catalog.Diagnostics...)
	if reports.HasBlockingIssue(issues) {
		stage := NewStageResult(StageComponentSelection, issues)
		stage.Summary = componentSelectionSummary(catalogDir, nil)
		return ComponentSelectionResult{CatalogDir: catalogDir, Stage: stage}
	}
	acceptance := componentAcceptanceForRequest(plan.Request)
	var selections []ComponentSelectionEntry
	for index, instance := range plan.Request.Blocks {
		if err := ctx.Err(); err != nil {
			issues = append(issues, reports.Issue{Code: reports.CodeOperationCanceled, Severity: reports.SeverityBlocked, Path: "component_selection", Message: err.Error()})
			break
		}
		definition, ok := registry.GetBlock(instance.BlockID)
		if !ok {
			issues = append(issues, reports.Issue{Code: reports.CodeMissingFile, Severity: reports.SeverityBlocked, Path: "blocks[" + strconv.Itoa(index) + "].block_id", Message: "block not found: " + instance.BlockID})
			continue
		}
		params := blocks.ApplyParameterDefaults(definition, instance.Params)
		for _, blockComponent := range definition.Components {
			if !blocks.ComponentActiveForParams(blockComponent, params) {
				continue
			}
			request, ok := blocks.SelectionRequestForComponentWithParams(blockComponent, acceptance, params)
			if !ok {
				issues = append(issues, reports.Issue{
					Code:     reports.CodeValidationFailed,
					Severity: reports.SeverityWarning,
					Path:     componentSelectionPath(instance.ID, blockComponent.Role),
					Message:  "block component has no component_id or component_query",
				})
				continue
			}
			issues = append(issues, applyComponentPolicy(&request, plan.Request.Components, definition.ID, instance.ID, blockComponent.Role)...)
			selection, result := components.Select(ctx, catalog, request)
			issues = append(issues, result.Issues...)
			if selection.Candidate.ComponentID == "" {
				issues = append(issues, reports.Issue{
					Code:     components.CodeComponentNotFound,
					Severity: reports.SeverityBlocked,
					Path:     componentSelectionPath(instance.ID, blockComponent.Role),
					Message:  "no component selected for " + instance.ID + "." + blockComponent.Role,
				})
				continue
			}
			if result.OK {
				selections = append(selections, ComponentSelectionEntry{
					InstanceID:      instance.ID,
					BlockID:         definition.ID,
					Role:            blockComponent.Role,
					ComponentID:     selection.Candidate.ComponentID,
					VariantID:       selection.Candidate.VariantID,
					Manufacturer:    selection.Component.Manufacturer,
					MPN:             selectedMPN(selection),
					SymbolID:        firstSelectedSymbolID(selection),
					FootprintID:     selection.Candidate.FootprintID,
					Confidence:      selection.Candidate.Confidence,
					ResolverChecked: selectedResolverChecked(selection),
					PinMapChecked:   selectedPinMapChecked(selection),
					Companions:      append([]components.CompanionRequirement(nil), selection.Component.Companions...),
					Rejected:        append([]components.CandidateRejection(nil), selection.Rejected...),
					Warnings:        append([]reports.Issue(nil), selection.Warnings...),
				})
			}
		}
	}
	stage := NewStageResult(StageComponentSelection, issues)
	stage.Summary = componentSelectionSummary(catalogDir, selections)
	return ComponentSelectionResult{CatalogDir: catalogDir, Selections: selections, Stage: stage}
}

func ApplyComponentSelectionsToPlan(plan *BlockPlanResult, registry blocks.Registry, selections []ComponentSelectionEntry) []reports.Issue {
	if plan == nil || registry == nil || len(selections) == 0 {
		return nil
	}
	byRef, issues := componentSelectionsByRef(registry, *plan, selections)
	if reports.HasBlockingIssue(issues) {
		return issues
	}
	operations := append([]transactions.Operation(nil), plan.Output.Operations...)
	for index, operation := range operations {
		selection, ok := byRef[operation.Ref]
		if !ok {
			continue
		}
		updated, changed, issue := operationWithComponentSelection(operation, selection)
		if issue != nil {
			issues = append(issues, *issue)
			continue
		}
		if changed {
			operations[index] = updated
		}
	}
	if !reports.HasBlockingIssue(issues) {
		plan.Output.Operations = operations
	}
	return issues
}

func componentSelectionsByRef(registry blocks.Registry, plan BlockPlanResult, selections []ComponentSelectionEntry) (map[string]ComponentSelectionEntry, []reports.Issue) {
	byKey := map[string]ComponentSelectionEntry{}
	for _, selection := range selections {
		byKey[selection.InstanceID+"."+selection.Role] = selection
	}
	facts, issues := componentOperationFacts(plan.Output.Operations)
	instances := map[string]blocks.BlockInstance{}
	for _, instance := range plan.Output.Instances {
		instances[instance.InstanceID] = instance
	}
	byRef := map[string]ComponentSelectionEntry{}
	for _, spec := range plan.Request.Blocks {
		definition, ok := registry.GetBlock(spec.BlockID)
		if !ok {
			continue
		}
		instance, ok := instances[spec.ID]
		if !ok {
			continue
		}
		used := map[string]struct{}{}
		for _, component := range definition.Components {
			selection, ok := byKey[spec.ID+"."+component.Role]
			if !ok {
				continue
			}
			ref := matchingWorkflowRefForComponent(component, instance.Refs, facts, used)
			if ref == "" {
				issues = append(issues, reports.Issue{
					Code:     reports.CodeValidationFailed,
					Severity: reports.SeverityBlocked,
					Path:     componentSelectionPath(spec.ID, component.Role),
					Message:  "selected component role has no matching generated reference",
				})
				continue
			}
			used[ref] = struct{}{}
			byRef[ref] = selection
		}
	}
	return byRef, issues
}

type workflowComponentFact struct {
	Role        string
	SymbolID    string
	FootprintID string
}

func componentOperationFacts(operations []transactions.Operation) (map[string]workflowComponentFact, []reports.Issue) {
	facts := map[string]workflowComponentFact{}
	var issues []reports.Issue
	for index, operation := range operations {
		switch operation.Op {
		case transactions.OpAddSymbol:
			var payload transactions.AddSymbolOperation
			if err := json.Unmarshal(operation.Raw, &payload); err != nil {
				issues = append(issues, componentSelectionOperationIssue(index, "decode add_symbol: "+err.Error()))
				continue
			}
			fact := facts[payload.Ref]
			fact.Role = payload.Role
			fact.SymbolID = payload.LibraryID
			facts[payload.Ref] = fact
		case transactions.OpAssignFootprint, transactions.OpPlaceFootprint:
			var payload struct {
				Ref         string `json:"ref"`
				Role        string `json:"role,omitempty"`
				FootprintID string `json:"footprint_id"`
			}
			if err := json.Unmarshal(operation.Raw, &payload); err != nil {
				issues = append(issues, componentSelectionOperationIssue(index, "decode footprint operation: "+err.Error()))
				continue
			}
			if payload.FootprintID != "" {
				fact := facts[payload.Ref]
				if payload.Role != "" {
					fact.Role = payload.Role
				}
				fact.FootprintID = payload.FootprintID
				facts[payload.Ref] = fact
			}
		}
	}
	return facts, issues
}

func matchingWorkflowRefForComponent(component blocks.BlockComponent, refs []string, facts map[string]workflowComponentFact, used map[string]struct{}) string {
	for _, ref := range refs {
		if _, ok := used[ref]; ok {
			continue
		}
		fact := facts[ref]
		if component.Role != "" && fact.Role == component.Role {
			return ref
		}
		if component.SymbolID != "" && fact.SymbolID != component.SymbolID {
			continue
		}
		if component.FootprintID != "" && fact.FootprintID != component.FootprintID {
			continue
		}
		return ref
	}
	return ""
}

func componentSelectionOperationIssue(index int, message string) reports.Issue {
	return reports.Issue{
		Code:     reports.CodeValidationFailed,
		Severity: reports.SeverityBlocked,
		Path:     "component_selection.operations." + strconv.Itoa(index),
		Message:  message,
	}
}

func componentCatalogDir(policy ComponentPolicySpec, opts ComponentSelectionOptions) string {
	if strings.TrimSpace(policy.CatalogDir) != "" {
		return policy.CatalogDir
	}
	if strings.TrimSpace(opts.CatalogDir) != "" && opts.CatalogDir != components.DefaultCatalogDir {
		return opts.CatalogDir
	}
	return discoverDefaultComponentCatalogDir()
}

func discoverDefaultComponentCatalogDir() string {
	if _, err := os.Stat(components.DefaultCatalogDir); err == nil {
		return components.DefaultCatalogDir
	}
	wd, err := os.Getwd()
	if err != nil {
		return components.DefaultCatalogDir
	}
	for dir := wd; ; dir = filepath.Dir(dir) {
		candidate := filepath.Join(dir, components.DefaultCatalogDir)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return components.DefaultCatalogDir
		}
	}
}

func componentAcceptanceForRequest(request Request) components.AcceptanceLevel {
	if request.Components.Acceptance != "" {
		return request.Components.Acceptance
	}
	switch request.Validation.Acceptance {
	case AcceptanceDraft:
		return components.AcceptanceDraft
	case AcceptanceStructural:
		return components.AcceptanceStructural
	case AcceptanceConnectivity:
		return components.AcceptanceConnectivity
	case AcceptanceERCDRC:
		return components.AcceptanceERCDRC
	case AcceptanceFabricationCandidate:
		return components.AcceptanceFabricationCandidate
	default:
		return components.AcceptanceStructural
	}
}

func operationWithComponentSelection(operation transactions.Operation, selection ComponentSelectionEntry) (transactions.Operation, bool, *reports.Issue) {
	switch operation.Op {
	case transactions.OpAddSymbol:
		var payload transactions.AddSymbolOperation
		if err := json.Unmarshal(operation.Raw, &payload); err != nil {
			return operation, false, componentSelectionRewriteIssue(operation.Ref, "decode add_symbol: "+err.Error())
		}
		if selection.SymbolID != "" {
			payload.LibraryID = selection.SymbolID
		}
		if selection.Value != "" {
			payload.Value = selection.Value
		}
		updated, err := wrapWorkflowOperation(transactions.OpAddSymbol, payload)
		if err != nil {
			return operation, false, componentSelectionRewriteIssue(operation.Ref, "encode add_symbol: "+err.Error())
		}
		return updated, true, nil
	case transactions.OpAssignFootprint:
		if selection.FootprintID == "" {
			return operation, false, nil
		}
		var payload transactions.AssignFootprintOperation
		if err := json.Unmarshal(operation.Raw, &payload); err != nil {
			return operation, false, componentSelectionRewriteIssue(operation.Ref, "decode assign_footprint: "+err.Error())
		}
		payload.FootprintID = selection.FootprintID
		updated, err := wrapWorkflowOperation(transactions.OpAssignFootprint, payload)
		if err != nil {
			return operation, false, componentSelectionRewriteIssue(operation.Ref, "encode assign_footprint: "+err.Error())
		}
		return updated, true, nil
	case transactions.OpPlaceFootprint:
		if selection.FootprintID == "" {
			return operation, false, nil
		}
		var payload transactions.PlaceFootprintOperation
		if err := json.Unmarshal(operation.Raw, &payload); err != nil {
			return operation, false, componentSelectionRewriteIssue(operation.Ref, "decode place_footprint: "+err.Error())
		}
		payload.FootprintID = selection.FootprintID
		if selection.Value != "" {
			payload.Value = selection.Value
		}
		updated, err := wrapWorkflowOperation(transactions.OpPlaceFootprint, payload)
		if err != nil {
			return operation, false, componentSelectionRewriteIssue(operation.Ref, "encode place_footprint: "+err.Error())
		}
		return updated, true, nil
	default:
		return operation, false, nil
	}
}

func wrapWorkflowOperation(kind transactions.OperationKind, payload any) (transactions.Operation, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return transactions.Operation{}, err
	}
	return transactions.NewOperationWithRef(kind, data, operationReference(payload)), nil
}

func operationReference(payload any) string {
	switch typed := payload.(type) {
	case transactions.AddSymbolOperation:
		return typed.Ref
	case transactions.AssignFootprintOperation:
		return typed.Ref
	case transactions.PlaceFootprintOperation:
		return typed.Ref
	default:
		return ""
	}
}

func componentSelectionRewriteIssue(ref string, message string) *reports.Issue {
	return &reports.Issue{
		Code:     reports.CodeValidationFailed,
		Severity: reports.SeverityBlocked,
		Path:     "component_selection." + ref,
		Message:  message,
	}
}

func firstSelectedSymbolID(selection components.Selection) string {
	if len(selection.Component.Symbols) == 0 {
		return ""
	}
	return selection.Component.Symbols[0].SymbolID
}

func selectedMPN(selection components.Selection) string {
	if selection.Variant.ID != "" && selection.Variant.MPN != "" {
		return selection.Variant.MPN
	}
	return selection.Component.MPN
}

func selectedResolverChecked(selection components.Selection) bool {
	if selection.Component.Verification.ResolverChecked {
		return true
	}
	return selection.Variant.ID != "" && selection.Variant.Verification.ResolverChecked
}

func selectedPinMapChecked(selection components.Selection) bool {
	if selection.Component.Verification.PinMapChecked {
		return true
	}
	return selection.Variant.ID != "" && selection.Variant.Verification.PinMapChecked
}

func applyComponentPolicy(request *components.SelectionRequest, policy ComponentPolicySpec, blockID string, instanceID string, role string) []reports.Issue {
	var issues []reports.Issue
	if request.Query.MinimumConfidence == "" {
		request.Query.MinimumConfidence = policy.MinimumConfidence
	}
	if request.Query.Package == "" {
		request.Query.Package = componentPackagePreference(policy, blockID, instanceID, role)
	}
	if override, ok := componentOverride(policy, blockID, instanceID, role); ok {
		if override.ComponentID != "" {
			// ComponentID is an explicit override and intentionally replaces the block's search filters.
			request.Query.Text = override.ComponentID
			request.Query.Family = ""
			request.Query.ValueKind = ""
			request.Query.Value = ""
		}
		if override.VariantID != "" && override.Package != "" && override.VariantID != override.Package {
			issues = append(issues, reports.Issue{
				Code:     reports.CodeInvalidArgument,
				Severity: reports.SeverityError,
				Path:     componentSelectionPath(instanceID, role),
				Message:  "component override cannot specify both variant_id and package with different values",
			})
		} else if override.Package != "" {
			request.Query.Package = override.Package
		} else if override.VariantID != "" {
			request.Query.Package = override.VariantID
		}
		if override.MinimumConfidence != "" {
			request.Query.MinimumConfidence = override.MinimumConfidence
		}
		if override.Acceptance != "" {
			request.Acceptance = override.Acceptance
		}
		request.AllowAlternatives = request.AllowAlternatives || override.AllowAlternatives
		request.RequiredRatings = append(request.RequiredRatings, override.RequiredRatings...)
	}
	return issues
}

func componentPackagePreference(policy ComponentPolicySpec, blockID string, instanceID string, role string) string {
	for _, key := range componentPolicyKeys(blockID, instanceID, role) {
		if value := policy.PackagePreferences[key]; value != "" {
			return value
		}
	}
	return ""
}

func componentOverride(policy ComponentPolicySpec, blockID string, instanceID string, role string) (ComponentOverrideSpec, bool) {
	for _, key := range componentPolicyKeys(blockID, instanceID, role) {
		if override, ok := policy.Overrides[key]; ok {
			return override, true
		}
	}
	return ComponentOverrideSpec{}, false
}

func componentPolicyKeys(blockID string, instanceID string, role string) []string {
	return []string{
		instanceID + "." + role,
		blockID + "." + role,
		role,
	}
}

func componentSelectionPath(instanceID string, role string) string {
	return "component_selection." + instanceID + "." + role
}

func componentSelectionSummary(catalogDir string, selections []ComponentSelectionEntry) map[string]any {
	return map[string]any{
		"catalog_dir":         catalogDir,
		"selection_count":     len(selections),
		"selected_components": selectedComponentSummary(selections),
	}
}

func selectedComponentSummary(selections []ComponentSelectionEntry) []map[string]any {
	out := make([]map[string]any, 0, len(selections))
	for _, selection := range selections {
		out = append(out, map[string]any{
			"instance_id":      selection.InstanceID,
			"role":             selection.Role,
			"component_id":     selection.ComponentID,
			"variant_id":       selection.VariantID,
			"manufacturer":     selection.Manufacturer,
			"mpn":              selection.MPN,
			"symbol_id":        selection.SymbolID,
			"footprint_id":     selection.FootprintID,
			"confidence":       selection.Confidence,
			"resolver_checked": selection.ResolverChecked,
			"pinmap_checked":   selection.PinMapChecked,
			"companion_count":  len(selection.Companions),
			"rejected_count":   len(selection.Rejected),
		})
	}
	return out
}
