package designworkflow

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"kicadai/internal/blocks"
	"kicadai/internal/componentprops"
	"kicadai/internal/components"
	"kicadai/internal/kicadfiles"
	"kicadai/internal/kicadfiles/schematic"
	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

const componentSelectionIUPerMM = 1_000_000

// Skip KiCad's derived Reference, Value, Footprint, and Datasheet rows when
// assigning fallback positions for visible custom properties.
const componentSelectionDerivedVisiblePropertySlots = 4

type ComponentSelectionOptions struct {
	CatalogDir string
	SourceDir  string
	Catalog    *components.Catalog
	Sources    *components.SourceCollection
}

type ComponentSelectionResult struct {
	CatalogDir string                    `json:"catalog_dir,omitempty"`
	SourceDir  string                    `json:"source_dir,omitempty"`
	Selections []ComponentSelectionEntry `json:"selections,omitempty"`
	Stage      StageResult               `json:"stage"`
}

type ComponentSelectionEntry struct {
	InstanceID      string                              `json:"instance_id"`
	BlockID         string                              `json:"block_id"`
	Role            string                              `json:"role"`
	ComponentID     string                              `json:"component_id"`
	VariantID       string                              `json:"variant_id"`
	Manufacturer    string                              `json:"manufacturer,omitempty"`
	MPN             string                              `json:"mpn,omitempty"`
	ComponentClass  string                              `json:"component_class,omitempty"`
	SymbolID        string                              `json:"symbol_id,omitempty"`
	FunctionPins    []components.FunctionPin            `json:"function_pins,omitempty"`
	Value           string                              `json:"value,omitempty"`
	FootprintID     string                              `json:"footprint_id,omitempty"`
	PinMapID        string                              `json:"pinmap_id,omitempty"`
	Confidence      components.ConfidenceLevel          `json:"confidence"`
	ResolverChecked bool                                `json:"resolver_checked,omitempty"`
	PinMapChecked   bool                                `json:"pinmap_checked,omitempty"`
	Companions      []components.CompanionRequirement   `json:"companions,omitempty"`
	Regulator       *components.RegulatorEvidence       `json:"regulator_evidence,omitempty"`
	Capacitor       *components.CapacitorEvidence       `json:"capacitor_evidence,omitempty"`
	OpAmp           *components.OpAmpEvidence           `json:"opamp_evidence,omitempty"`
	AmplifierOutput *components.AmplifierOutputEvidence `json:"amplifier_output_evidence,omitempty"`
	PlacementHints  []components.PlacementHint          `json:"placement_hints,omitempty"`
	RoutingHints    []components.RoutingHint            `json:"routing_hints,omitempty"`
	Procurement     *components.ProcurementEvidence     `json:"procurement,omitempty"`
	Rejected        []components.CandidateRejection     `json:"rejected,omitempty"`
	Warnings        []reports.Issue                     `json:"warnings,omitempty"`
}

func SelectWorkflowComponents(ctx context.Context, registry blocks.Registry, plan BlockPlanResult, opts ComponentSelectionOptions) ComponentSelectionResult {
	catalogDir := componentCatalogDir(plan.Request.Components, opts)
	sourceDir := componentSourceDir(plan.Request.Components, opts)
	catalog := opts.Catalog
	if catalog == nil {
		if _, err := os.Stat(catalogDir); err != nil {
			issue := reports.Issue{Code: components.CodeCatalogReadFailed, Severity: reports.SeverityBlocked, Path: "component_policy.catalog_dir", Message: err.Error()}
			return ComponentSelectionResult{CatalogDir: catalogDir, SourceDir: sourceDir, Stage: NewStageResult(StageComponentSelection, []reports.Issue{issue})}
		}
		loaded, err := components.LoadCatalog(ctx, components.LoadOptions{CatalogDir: catalogDir})
		if err != nil {
			issue := reports.Issue{Code: components.CodeCatalogReadFailed, Severity: reports.SeverityBlocked, Path: "component_policy.catalog_dir", Message: err.Error()}
			return ComponentSelectionResult{CatalogDir: catalogDir, SourceDir: sourceDir, Stage: NewStageResult(StageComponentSelection, []reports.Issue{issue})}
		}
		catalog = loaded
	}
	issues := append([]reports.Issue(nil), catalog.Diagnostics...)
	sources := opts.Sources
	if sources != nil {
		issues = append(issues, sources.Diagnostics...)
	} else if sourceDir != "" {
		loaded, err := components.LoadSources(ctx, components.SourceLoadOptions{SourceDir: sourceDir})
		if err != nil {
			issues = append(issues, reports.Issue{Code: components.CodeSourceReadFailed, Severity: reports.SeverityBlocked, Path: "component_policy.source_dir", Message: err.Error()})
		} else if loaded == nil {
			issues = append(issues, reports.Issue{Code: components.CodeSourceReadFailed, Severity: reports.SeverityBlocked, Path: "component_policy.source_dir", Message: "component source loader returned nil collection"})
		} else {
			sources = loaded
			issues = append(issues, loaded.Diagnostics...)
		}
	}
	if reports.HasBlockingIssue(issues) {
		stage := NewStageResult(StageComponentSelection, issues)
		stage.Summary = componentSelectionSummary(catalogDir, sourceDir, nil)
		return ComponentSelectionResult{CatalogDir: catalogDir, SourceDir: sourceDir, Stage: stage}
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
			request.Sources = sources
			request.Procurement = plan.Request.Components.Procurement
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
					ComponentClass:  selection.Candidate.Family,
					SymbolID:        firstSelectedSymbolID(selection),
					FunctionPins:    selectedFunctionPins(selection),
					FootprintID:     selection.Candidate.FootprintID,
					PinMapID:        selectedPinMapID(selection),
					Confidence:      selection.Candidate.Confidence,
					ResolverChecked: selectedResolverChecked(selection),
					PinMapChecked:   selectedPinMapChecked(selection),
					Companions:      append([]components.CompanionRequirement(nil), selection.Component.Companions...),
					Regulator:       cloneRegulatorEvidence(selection.Component.Regulator),
					Capacitor:       cloneCapacitorEvidence(selection.Component.Capacitor),
					OpAmp:           cloneOpAmpEvidence(selection.Component.OpAmp),
					AmplifierOutput: cloneAmplifierOutputEvidence(selection.Component.AmplifierOutput),
					PlacementHints:  append([]components.PlacementHint(nil), selection.Component.PlacementHints...),
					RoutingHints:    append([]components.RoutingHint(nil), selection.Component.RoutingHints...),
					Procurement:     cloneProcurementEvidence(selection.Procurement),
					Rejected:        append([]components.CandidateRejection(nil), selection.Rejected...),
					Warnings:        append([]reports.Issue(nil), selection.Warnings...),
				})
			}
		}
	}
	stage := NewStageResult(StageComponentSelection, issues)
	stage.Summary = componentSelectionSummary(catalogDir, sourceDir, selections)
	return ComponentSelectionResult{CatalogDir: catalogDir, SourceDir: sourceDir, Selections: selections, Stage: stage}
}

func ApplyComponentSelectionsToPlan(plan *BlockPlanResult, registry blocks.Registry, selections []ComponentSelectionEntry) []reports.Issue {
	if plan == nil || registry == nil || len(selections) == 0 {
		return nil
	}
	selectionsBySchematicRef, issues := componentSelectionsBySchematicRef(registry, *plan, selections)
	if reports.HasBlockingIssue(issues) {
		return issues
	}
	operations := append([]transactions.Operation(nil), plan.Output.Operations...)
	for index, operation := range operations {
		selection, ok := selectionsBySchematicRef[operation.Ref]
		if !ok {
			continue
		}
		updated, changed, opIssues := operationWithComponentSelection(operation, selection)
		if len(opIssues) > 0 {
			issues = append(issues, opIssues...)
		}
		if reports.HasBlockingIssue(opIssues) {
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

func componentSelectionsBySchematicRef(registry blocks.Registry, plan BlockPlanResult, selections []ComponentSelectionEntry) (map[string]ComponentSelectionEntry, []reports.Issue) {
	byKey := map[string]ComponentSelectionEntry{}
	for _, selection := range selections {
		byKey[selection.InstanceID+"."+selection.Role] = selection
	}
	facts, issues := componentOperationFacts(plan.Output.Operations)
	instances := map[string]blocks.BlockInstance{}
	for _, instance := range plan.Output.Instances {
		instances[instance.InstanceID] = instance
	}
	// Resolve instance/role selections to generated schematic references.
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

func componentSourceDir(policy ComponentPolicySpec, opts ComponentSelectionOptions) string {
	if strings.TrimSpace(policy.SourceDir) != "" {
		return strings.TrimSpace(policy.SourceDir)
	}
	if strings.TrimSpace(opts.SourceDir) != "" {
		return strings.TrimSpace(opts.SourceDir)
	}
	return ""
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

func operationWithComponentSelection(operation transactions.Operation, selection ComponentSelectionEntry) (transactions.Operation, bool, []reports.Issue) {
	switch operation.Op {
	case transactions.OpAddSymbol:
		var payload transactions.AddSymbolOperation
		if err := json.Unmarshal(operation.Raw, &payload); err != nil {
			return operation, false, []reports.Issue{componentSelectionRewriteIssue(operation.Ref, "decode add_symbol: "+err.Error())}
		}
		if selection.SymbolID != "" {
			payload.LibraryID = selection.SymbolID
		}
		if selection.Value != "" {
			payload.Value = selection.Value
		}
		merged, mergeIssues := componentSelectionSymbolProperties(payload, selection)
		payload.Properties = merged
		updated, err := wrapWorkflowOperation(transactions.OpAddSymbol, payload)
		if err != nil {
			return operation, false, []reports.Issue{componentSelectionRewriteIssue(operation.Ref, "encode add_symbol: "+err.Error())}
		}
		return updated, true, mergeIssues
	case transactions.OpAssignFootprint:
		if selection.FootprintID == "" {
			return operation, false, nil
		}
		var payload transactions.AssignFootprintOperation
		if err := json.Unmarshal(operation.Raw, &payload); err != nil {
			return operation, false, []reports.Issue{componentSelectionRewriteIssue(operation.Ref, "decode assign_footprint: "+err.Error())}
		}
		payload.FootprintID = selection.FootprintID
		updated, err := wrapWorkflowOperation(transactions.OpAssignFootprint, payload)
		if err != nil {
			return operation, false, []reports.Issue{componentSelectionRewriteIssue(operation.Ref, "encode assign_footprint: "+err.Error())}
		}
		return updated, true, nil
	case transactions.OpPlaceFootprint:
		if selection.FootprintID == "" {
			return operation, false, nil
		}
		var payload transactions.PlaceFootprintOperation
		if err := json.Unmarshal(operation.Raw, &payload); err != nil {
			return operation, false, []reports.Issue{componentSelectionRewriteIssue(operation.Ref, "decode place_footprint: "+err.Error())}
		}
		payload.FootprintID = selection.FootprintID
		if selection.Value != "" {
			payload.Value = selection.Value
		}
		updated, err := wrapWorkflowOperation(transactions.OpPlaceFootprint, payload)
		if err != nil {
			return operation, false, []reports.Issue{componentSelectionRewriteIssue(operation.Ref, "encode place_footprint: "+err.Error())}
		}
		return updated, true, nil
	default:
		return operation, false, nil
	}
}

func componentSelectionSymbolProperties(payload transactions.AddSymbolOperation, selection ComponentSelectionEntry) ([]transactions.SymbolProperty, []reports.Issue) {
	position := pointFromTransactionMM(payload.At)
	properties, issues := componentprops.MergeIdentityProperties(
		schematicPropertiesFromTransaction(payload.Properties, position, kicadfiles.Angle(payload.Rotation)),
		componentSelectionEvidence(selection),
		componentprops.MergeOptions{
			Policy:   componentprops.PolicyGeneratedReplace,
			Ref:      payload.Ref,
			Position: position,
			Rotation: kicadfiles.Angle(payload.Rotation),
			Path:     "component_selection." + payload.Ref,
		},
	)
	return transactionPropertiesFromSchematic(properties), issues
}

func pointFromTransactionMM(point transactions.Point) kicadfiles.Point {
	return kicadfiles.Point{
		X: kicadfiles.IU(math.Round(point.XMM * componentSelectionIUPerMM)),
		Y: kicadfiles.IU(math.Round(point.YMM * componentSelectionIUPerMM)),
	}
}

func componentSelectionVisiblePropertyPosition(symbolPosition kicadfiles.Point, index int, rotation kicadfiles.Angle) kicadfiles.Point {
	slot := componentSelectionDerivedVisiblePropertySlots + index
	offset := 2.54 * float64(slot) * componentSelectionIUPerMM
	radians := float64(rotation) * math.Pi / 180
	sin, cos := math.Sincos(radians)
	return kicadfiles.Point{
		X: symbolPosition.X + kicadfiles.IU(math.Round(-offset*sin)),
		Y: symbolPosition.Y + kicadfiles.IU(math.Round(offset*cos)),
	}
}

func componentSelectionEvidence(selection ComponentSelectionEntry) componentprops.Evidence {
	evidence := componentprops.Evidence{
		ComponentID:         selection.ComponentID,
		VariantID:           selection.VariantID,
		ComponentRole:       selection.Role,
		BlockID:             strings.TrimSpace(selection.BlockID),
		Manufacturer:        selection.Manufacturer,
		MPN:                 selection.MPN,
		ComponentClass:      selection.ComponentClass,
		ComponentConfidence: string(selection.Confidence),
		ComponentSource:     componentSelectionSource(selection),
		PinmapID:            selection.PinMapID,
	}
	if selection.Procurement != nil {
		evidence.LifecycleStatus = string(selection.Procurement.LifecycleStatus)
		evidence.AvailabilityStatus = string(selection.Procurement.AvailabilityStatus)
	}
	return evidence
}

func componentSelectionSource(selection ComponentSelectionEntry) string {
	if selection.Procurement != nil {
		return componentprops.SourceCatalogSnapshot
	}
	if selection.Manufacturer != "" || selection.MPN != "" || selection.VariantID != "" {
		return componentprops.SourceCatalog
	}
	switch selection.Confidence {
	case components.ConfidenceRuleInferred:
		return componentprops.SourcePolicyAllowed
	default:
		return componentprops.SourceGeneric
	}
}

func schematicPropertiesFromTransaction(properties []transactions.SymbolProperty, position kicadfiles.Point, rotation kicadfiles.Angle) []schematic.Property {
	out := make([]schematic.Property, 0, len(properties))
	visibleIndex := 0
	for _, property := range properties {
		propertyPosition := position
		if property.At != nil {
			propertyPosition = pointFromTransactionMM(*property.At)
		} else if !property.Hidden {
			propertyPosition = componentSelectionVisiblePropertyPosition(position, visibleIndex, rotation)
			visibleIndex++
		}
		propertyRotation := kicadfiles.Angle(0)
		if property.Rotation != nil {
			propertyRotation = kicadfiles.Angle(*property.Rotation)
		}
		out = append(out, schematic.Property{
			Name:           strings.TrimSpace(property.Name),
			Value:          property.Value,
			Private:        property.Private,
			Hidden:         property.Hidden,
			ShowName:       schematic.CloneBool(property.ShowName),
			DoNotAutoplace: schematic.CloneBool(property.DoNotAutoplace),
			Position:       propertyPosition,
			Rotation:       propertyRotation,
		})
	}
	return out
}

func transactionPropertiesFromSchematic(properties []schematic.Property) []transactions.SymbolProperty {
	out := make([]transactions.SymbolProperty, 0, len(properties))
	for _, property := range properties {
		rotation := float64(property.Rotation)
		out = append(out, transactions.SymbolProperty{
			Name:           property.Name,
			Value:          property.Value,
			Private:        property.Private,
			Hidden:         property.Hidden,
			ShowName:       schematic.CloneBool(property.ShowName),
			DoNotAutoplace: schematic.CloneBool(property.DoNotAutoplace),
			At:             &transactions.Point{XMM: iuToMM(property.Position.X), YMM: iuToMM(property.Position.Y)},
			Rotation:       &rotation,
		})
	}
	return out
}

func iuToMM(value kicadfiles.IU) float64 {
	return float64(value) / componentSelectionIUPerMM
}

func wrapWorkflowOperation(kind transactions.OperationKind, payload any) (transactions.Operation, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return transactions.Operation{}, err
	}
	return transactions.NewOperationWithMetadata(kind, data, operationReference(payload), ""), nil
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

func componentSelectionRewriteIssue(ref string, message string) reports.Issue {
	return reports.Issue{
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

func selectedFunctionPins(selection components.Selection) []components.FunctionPin {
	symbolID := firstSelectedSymbolID(selection)
	for _, binding := range selection.Component.Symbols {
		if binding.SymbolID == symbolID {
			return append([]components.FunctionPin(nil), binding.FunctionPins...)
		}
	}
	return nil
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

func selectedPinMapID(selection components.Selection) string {
	if selection.Variant.PinMapID != "" {
		return selection.Variant.PinMapID
	}
	symbolID := firstSelectedSymbolID(selection)
	for _, binding := range selection.Component.Symbols {
		if binding.SymbolID == symbolID && binding.PinMapID != "" {
			return binding.PinMapID
		}
	}
	return ""
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

func componentSelectionSummary(catalogDir string, sourceDir string, selections []ComponentSelectionEntry) map[string]any {
	hints := NormalizeComponentHints(selections)
	return map[string]any{
		"catalog_dir":         catalogDir,
		"source_dir":          sourceDir,
		"selection_count":     len(selections),
		"selected_components": selectedComponentSummary(selections),
		"procurement":         procurementSelectionSummary(selections),
		"component_hints":     hints,
		"hint_summary":        SummarizeComponentHints(hints),
	}
}

func selectedComponentSummary(selections []ComponentSelectionEntry) []map[string]any {
	out := make([]map[string]any, 0, len(selections))
	for _, selection := range selections {
		item := map[string]any{
			"instance_id":          selection.InstanceID,
			"role":                 selection.Role,
			"component_id":         selection.ComponentID,
			"variant_id":           selection.VariantID,
			"manufacturer":         selection.Manufacturer,
			"mpn":                  selection.MPN,
			"symbol_id":            selection.SymbolID,
			"footprint_id":         selection.FootprintID,
			"confidence":           selection.Confidence,
			"resolver_checked":     selection.ResolverChecked,
			"pinmap_checked":       selection.PinMapChecked,
			"companion_count":      len(selection.Companions),
			"placement_hint_count": len(selection.PlacementHints),
			"routing_hint_count":   len(selection.RoutingHints),
			"rejected_count":       len(selection.Rejected),
			"warning_count":        len(selection.Warnings),
		}
		if selection.Regulator != nil {
			item["regulator_evidence"] = regulatorEvidenceSummary(selection.Regulator)
		}
		if selection.Capacitor != nil {
			item["capacitor_evidence"] = capacitorEvidenceSummary(selection.Capacitor)
		}
		if selection.OpAmp != nil {
			item["opamp_evidence"] = opAmpEvidenceSummary(selection.OpAmp)
		}
		if selection.AmplifierOutput != nil {
			item["amplifier_output_evidence"] = amplifierOutputEvidenceSummary(selection.AmplifierOutput)
		}
		if selection.Procurement != nil {
			item["procurement"] = selection.Procurement
		}
		out = append(out, item)
	}
	return out
}

func procurementSelectionSummary(selections []ComponentSelectionEntry) map[string]any {
	selectedWithEvidence := 0
	lifecycleEvidence := 0
	availabilityEvidence := 0
	warningCount := 0
	blockedRejections := 0
	for _, selection := range selections {
		if selection.Procurement != nil {
			selectedWithEvidence++
			if selection.Procurement.LifecycleStatus != "" {
				lifecycleEvidence++
			}
			if selection.Procurement.AvailabilityStatus != "" {
				availabilityEvidence++
			}
		}
		warningCount += len(selection.Warnings)
		for _, rejection := range selection.Rejected {
			if reports.HasBlockingIssue(rejection.Issues) {
				blockedRejections++
			}
		}
	}
	return map[string]any{
		"selected_with_evidence":      selectedWithEvidence,
		"lifecycle_evidence_count":    lifecycleEvidence,
		"availability_evidence_count": availabilityEvidence,
		"warning_count":               warningCount,
		"blocked_rejection_count":     blockedRejections,
	}
}

func cloneProcurementEvidence(evidence *components.ProcurementEvidence) *components.ProcurementEvidence {
	if evidence == nil {
		return nil
	}
	clone := *evidence
	if evidence.LifecycleFresh != nil {
		value := *evidence.LifecycleFresh
		clone.LifecycleFresh = &value
	}
	if evidence.AvailabilityFresh != nil {
		value := *evidence.AvailabilityFresh
		clone.AvailabilityFresh = &value
	}
	return &clone
}

func cloneRegulatorEvidence(evidence *components.RegulatorEvidence) *components.RegulatorEvidence {
	if evidence == nil {
		return nil
	}
	clone := *evidence
	if evidence.OutputCapacitor != nil {
		output := *evidence.OutputCapacitor
		output.AcceptedDielectrics = append([]string(nil), evidence.OutputCapacitor.AcceptedDielectrics...)
		clone.OutputCapacitor = &output
	}
	clone.Notes = append([]string(nil), evidence.Notes...)
	return &clone
}

func cloneCapacitorEvidence(evidence *components.CapacitorEvidence) *components.CapacitorEvidence {
	if evidence == nil {
		return nil
	}
	clone := *evidence
	return &clone
}

func cloneOpAmpEvidence(evidence *components.OpAmpEvidence) *components.OpAmpEvidence {
	if evidence == nil {
		return nil
	}
	clone := *evidence
	clone.IntendedRoles = append([]string(nil), evidence.IntendedRoles...)
	return &clone
}

func cloneAmplifierOutputEvidence(evidence *components.AmplifierOutputEvidence) *components.AmplifierOutputEvidence {
	if evidence == nil {
		return nil
	}
	clone := *evidence
	return &clone
}

func regulatorEvidenceSummary(evidence *components.RegulatorEvidence) map[string]any {
	out := map[string]any{
		"thermal_review": evidence.ThermalReview,
	}
	if evidence.OutputCapacitor != nil {
		out["output_capacitor"] = map[string]any{
			"kind":                         evidence.OutputCapacitor.Kind,
			"proof_status":                 evidence.OutputCapacitor.ProofStatus,
			"fabrication_candidate_blocks": evidence.OutputCapacitor.FabricationCandidateBlocks,
			"review_note":                  evidence.OutputCapacitor.ReviewNote,
		}
	}
	return out
}

func capacitorEvidenceSummary(evidence *components.CapacitorEvidence) map[string]any {
	return map[string]any{
		"dielectric":                   evidence.Dielectric,
		"dc_bias_review":               evidence.DCBiasReview,
		"effective_capacitance_review": evidence.EffectiveCapacitanceReview,
		"esr_review":                   evidence.ESRReview,
		"fabrication_candidate_blocks": evidence.FabricationCandidateBlocks,
		"fabrication_proof":            evidence.FabricationProof,
		"review_note":                  evidence.ReviewNote,
	}
}

func opAmpEvidenceSummary(evidence *components.OpAmpEvidence) map[string]any {
	return map[string]any{
		"intended_roles":               append([]string(nil), evidence.IntendedRoles...),
		"supply_mode":                  evidence.SupplyMode,
		"output_drive_status":          evidence.OutputDriveStatus,
		"load_compatibility_status":    evidence.LoadCompatibilityStatus,
		"gain_bandwidth_status":        evidence.GainBandwidthStatus,
		"stability_status":             evidence.StabilityStatus,
		"input_common_mode_status":     evidence.InputCommonModeStatus,
		"fabrication_candidate_blocks": evidence.FabricationCandidateBlocks,
		"review_note":                  evidence.ReviewNote,
	}
}

func amplifierOutputEvidenceSummary(evidence *components.AmplifierOutputEvidence) map[string]any {
	return map[string]any{
		"device_class":                 evidence.DeviceClass,
		"polarity":                     evidence.Polarity,
		"package":                      evidence.Package,
		"complementary_group":          evidence.ComplementaryGroup,
		"voltage_rating_status":        evidence.VoltageRatingStatus,
		"current_rating_status":        evidence.CurrentRatingStatus,
		"power_dissipation_status":     evidence.PowerDissipationStatus,
		"thermal_review":               evidence.ThermalReview,
		"safe_operating_area_status":   evidence.SafeOperatingAreaStatus,
		"fabrication_candidate_blocks": evidence.FabricationCandidateBlocks,
		"review_note":                  evidence.ReviewNote,
	}
}
