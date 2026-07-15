package circuitgraph

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"kicadai/internal/components"
	"kicadai/internal/libraryresolver"
	"kicadai/internal/reports"
)

// Resolver binds graphs against one immutable catalog and library-evidence
// snapshot. Construct it once and reuse it for every graph in that snapshot.
type Resolver struct {
	options     ResolveOptions
	catalogHash string
	libraryHash string
	recordsByID map[string]components.ComponentRecord
}

func NewResolver(options ResolveOptions) *Resolver {
	resolver := &Resolver{options: options, libraryHash: libraryEvidenceHash(options), recordsByID: map[string]components.ComponentRecord{}}
	if options.Catalog != nil {
		candidateHash := strings.TrimSpace(options.CatalogHash)
		if len(candidateHash) == sha256.Size*2 {
			if _, err := hex.DecodeString(candidateHash); err == nil {
				resolver.catalogHash = strings.ToLower(candidateHash)
			}
		}
		if resolver.catalogHash == "" {
			resolver.catalogHash = catalogHash(options.Catalog)
		}
		for _, record := range options.Catalog.Records {
			resolver.recordsByID[record.ID] = record
		}
	}
	return resolver
}

func (resolver *Resolver) Resolve(ctx context.Context, document Document) (ResolvedDocument, []reports.Issue) {
	normalized := Normalize(document)
	if issues := Validate(normalized); len(issues) != 0 {
		return ResolvedDocument{Schema: SchemaID, Version: Version, Source: normalized}, issues
	}
	if resolver == nil {
		return ResolvedDocument{Schema: SchemaID, Version: Version, Source: normalized}, []reports.Issue{graphIssue(CodeComponentUnresolved, "resolver", "component resolver is required")}
	}
	if resolver.options.Catalog == nil {
		return ResolvedDocument{Schema: SchemaID, Version: Version, Source: normalized}, []reports.Issue{graphIssue(CodeComponentUnresolved, "catalog", "component catalog is required")}
	}
	options := resolver.options
	result := ResolvedDocument{
		Schema: SchemaID, Version: Version, Source: normalized,
		CatalogID:   strings.TrimSpace(options.CatalogID),
		CatalogHash: resolver.catalogHash,
		LibraryHash: resolver.libraryHash,
	}
	if result.CatalogID == "" {
		result.CatalogID = "catalog"
	}
	resolvedByID := make(map[string]ResolvedComponent, len(normalized.Components))
	type unresolvedRoot struct {
		issueID    string
		retryScope string
	}
	unresolvedRoots := make(map[string]unresolvedRoot)
	var issues []reports.Issue
	for index, instance := range normalized.Components {
		if err := ctx.Err(); err != nil {
			issues = append(issues, reports.Issue{Code: reports.CodeOperationCanceled, Severity: reports.SeverityError, Path: "components", Message: err.Error()})
			break
		}
		component, componentIssues := resolveComponent(ctx, instance, options, resolver.recordsByID, normalized.Project.Acceptance, index)
		issues = append(issues, componentIssues...)
		if !reports.HasBlockingIssue(componentIssues) {
			result.Components = append(result.Components, component)
			resolvedByID[instance.ID] = component
		} else if root := firstGraphRootIssue(componentIssues); root.IssueID != "" {
			unresolvedRoots[instance.ID] = unresolvedRoot{issueID: root.IssueID, retryScope: root.RetryScope}
		}
	}
	usedBindings := map[physicalBindingID]struct{}{}
	physicalOwners := map[physicalBindingID]string{}
	for netIndex, net := range normalized.Nets {
		resolvedNet := ResolvedNet{Intent: net}
		for endpointIndex, endpoint := range net.Endpoints {
			path := fmt.Sprintf("nets[%d].endpoints[%d]", netIndex, endpointIndex)
			if root := unresolvedRoots[endpoint.Component]; root.issueID != "" {
				issues = append(issues, graphDependentIssue(CodePinUnresolved, path, "endpoint depends on an unresolved component", root.issueID, root.retryScope))
				continue
			}
			resolvedEndpoint, endpointIssues := resolveEndpoint(path, endpoint, resolvedByID)
			issues = append(issues, endpointIssues...)
			if len(endpointIssues) == 0 {
				issues = append(issues, registerPhysicalBindings(path, endpoint.Component, resolvedEndpoint.Bindings, physicalOwners)...)
				resolvedNet.Endpoints = append(resolvedNet.Endpoints, resolvedEndpoint)
				markUsedBindings(endpoint.Component, resolvedEndpoint.Bindings, usedBindings)
			}
		}
		result.Nets = append(result.Nets, resolvedNet)
	}
	issues = append(issues, validateResolvedPowerFlags(result.Source.PowerFlags, result.Nets)...)
	for endpointIndex, endpoint := range normalized.NoConnects {
		path := fmt.Sprintf("no_connects[%d]", endpointIndex)
		if root := unresolvedRoots[endpoint.Component]; root.issueID != "" {
			issues = append(issues, graphDependentIssue(CodePinUnresolved, path, "no-connect depends on an unresolved component", root.issueID, root.retryScope))
			continue
		}
		resolvedEndpoint, endpointIssues := resolveEndpoint(path, endpoint, resolvedByID)
		issues = append(issues, endpointIssues...)
		if len(endpointIssues) == 0 {
			issues = append(issues, registerPhysicalBindings(path, endpoint.Component, resolvedEndpoint.Bindings, physicalOwners)...)
			result.NoConnects = append(result.NoConnects, resolvedEndpoint)
			markUsedBindings(endpoint.Component, resolvedEndpoint.Bindings, usedBindings)
		}
	}
	for componentIndex, component := range result.Components {
		requiredFunctions := map[string]struct{}{}
		for _, requested := range component.Instance.RequiredFunctions {
			if function, ok := canonicalFunction(component.Functions, requested); ok {
				requiredFunctions[function] = struct{}{}
			}
		}
		for _, function := range component.Functions {
			_, explicitlyRequired := requiredFunctions[function.Function]
			if !function.Required && !explicitlyRequired {
				continue
			}
			key := physicalBindingKey(component.Instance.ID, ResolvedBinding{Unit: function.Unit, SymbolPin: function.SymbolPin, Pad: function.Pad})
			if _, exists := usedBindings[key]; !exists {
				path := fmt.Sprintf("components[%d].pins.%s", componentIndex, function.SymbolPin)
				issues = append(issues, graphIssue(CodeRequiredPinOpen, path, "required physical symbol pin is neither connected nor explicitly no-connected"))
			}
		}
	}
	if !reports.HasBlockingIssue(issues) {
		result.ResolutionHash = resolvedHash(result)
	}
	return result, finalizeGraphIssues(issues)
}

func firstGraphRootIssue(issues []reports.Issue) reports.Issue {
	for _, issue := range issues {
		if issue.Blocking() && issue.RootCauseID == "" {
			return issue
		}
	}
	return reports.Issue{}
}

func validateResolvedPowerFlags(flags []PowerFlag, nets []ResolvedNet) []reports.Issue {
	byName := make(map[string]ResolvedNet, len(nets))
	for _, net := range nets {
		byName[net.Intent.Name] = net
	}
	var issues []reports.Issue
	seen := map[string]int{}
	for index, flag := range flags {
		if previous, duplicate := seen[flag.Net]; duplicate {
			path := fmt.Sprintf("power_flags[%d].net", index)
			message := fmt.Sprintf("duplicate power flag for net %s already declared at power_flags[%d]", flag.Net, previous)
			issues = append(issues, graphIssue(CodePowerFlagInvalid, path, message))
			continue
		}
		seen[flag.Net] = index
		net, exists := byName[flag.Net]
		if !exists {
			path := fmt.Sprintf("power_flags[%d].net", index)
			issues = append(issues, graphIssue(CodePowerFlagInvalid, path, "power flag net is missing from resolved connectivity"))
			continue
		}
		foundDriver := false
		for _, endpoint := range net.Endpoints {
			for _, binding := range endpoint.Bindings {
				if !strings.EqualFold(strings.TrimSpace(binding.Electrical), "power_out") {
					continue
				}
				path := fmt.Sprintf("power_flags[%d].net", index)
				message := fmt.Sprintf("net %s already has internal power_out driver %s.%s", flag.Net, endpoint.Intent.Component, binding.SymbolPin)
				issues = append(issues, graphIssue(CodePowerFlagInvalid, path, message))
				foundDriver = true
				break
			}
			if foundDriver {
				break
			}
		}
	}
	return issues
}

func LibraryEvidenceFromIndex(index libraryresolver.LibraryIndex) (map[string]LibrarySymbolEvidence, map[string]LibraryFootprintEvidence) {
	symbols := make(map[string]LibrarySymbolEvidence, len(index.Symbols))
	for id, record := range index.Symbols {
		pins := make(map[string]struct{}, len(record.Pins))
		for _, pin := range record.Pins {
			pins[pin.Number] = struct{}{}
		}
		symbols[id] = LibrarySymbolEvidence{LibraryID: id, Pins: pins, Source: record.Path}
	}
	footprints := make(map[string]LibraryFootprintEvidence, len(index.Footprints))
	for id, record := range index.Footprints {
		pads := make(map[string]struct{}, len(record.Pads))
		for _, pad := range record.Pads {
			pads[pad.Name] = struct{}{}
		}
		footprints[id] = LibraryFootprintEvidence{LibraryID: id, Pads: pads, Source: record.Path}
	}
	return symbols, footprints
}

func resolveComponent(ctx context.Context, instance Component, options ResolveOptions, recordsByID map[string]components.ComponentRecord, acceptance AcceptanceLevel, index int) (ResolvedComponent, []reports.Issue) {
	path := fmt.Sprintf("components[%d]", index)
	var resolved components.ResolvedComponent
	var warnings []reports.Issue
	if instance.ComponentID != "" {
		record, exists := recordsByID[instance.ComponentID]
		variantID, issue := explicitVariantID(record, exists, instance.VariantID)
		if issue != nil {
			issue.Path = path + ".variant_id"
			return ResolvedComponent{}, []reports.Issue{*issue}
		}
		var report reports.Result
		resolved, report = components.ResolveBinding(ctx, options.Catalog, instance.ComponentID, variantID)
		if reports.HasBlockingIssue(report.Issues) {
			message := "component or variant did not resolve: " + summarizeIssues(report.Issues)
			return ResolvedComponent{}, []reports.Issue{graphIssue(CodeComponentUnresolved, path+".component_id", message)}
		}
		validation := components.ValidateResolvedComponent(resolved, componentSelectionRequest(instance, acceptance))
		if reports.HasBlockingIssue(validation.Issues) {
			return ResolvedComponent{}, []reports.Issue{graphIssue(CodeComponentUnresolved, path, summarizeIssues(validation.Issues))}
		}
		warnings = append(warnings, validation.Issues...)
	} else {
		selection, report := components.Select(ctx, options.Catalog, componentSelectionRequest(instance, acceptance))
		if reports.HasBlockingIssue(report.Issues) {
			code := CodeComponentUnresolved
			for _, issue := range report.Issues {
				if issue.Code == components.CodeComponentAmbiguous {
					code = CodeComponentAmbiguous
				}
			}
			return ResolvedComponent{}, []reports.Issue{graphIssue(code, path+".query", summarizeIssues(report.Issues))}
		}
		resolved = components.ResolvedComponent{Component: selection.Component, Variant: selection.Variant}
		if len(selection.Component.Symbols) != 0 {
			resolved.Symbol = selection.Component.Symbols[0]
		}
		warnings = append(warnings, selection.Warnings...)
		warnings = append(warnings, report.Issues...)
	}
	if issue := componentConstraintIssue(path, instance, resolved); issue != nil {
		return ResolvedComponent{}, []reports.Issue{*issue}
	}
	selectedSymbols, resolvedUnits, unitIssues := resolveComponentUnits(path, instance, resolved.Component.Symbols)
	if reports.HasBlockingIssue(unitIssues) {
		return ResolvedComponent{}, unitIssues
	}
	functions, functionIssues := resolveFunctions(path, selectedSymbols, resolved.Variant)
	resolutionIssues := append(append([]reports.Issue(nil), unitIssues...), functionIssues...)
	if reports.HasBlockingIssue(resolutionIssues) {
		return ResolvedComponent{}, resolutionIssues
	}
	if libraryIssues := verifyLibraryEvidence(path, functions, resolved, options); len(libraryIssues) != 0 {
		return ResolvedComponent{}, libraryIssues
	}
	confidence := weakerGraphConfidence(resolved.Component.Verification.Confidence, resolved.Variant.Verification.Confidence)
	symbolID := resolved.Symbol.SymbolID
	if symbolID == "" && len(resolved.Component.Symbols) != 0 {
		symbolID = resolved.Component.Symbols[0].SymbolID
	}
	component := ResolvedComponent{
		Instance: instance, ComponentID: resolved.Component.ID, VariantID: resolved.Variant.ID,
		Family: resolved.Component.Family, Manufacturer: resolved.Component.Manufacturer,
		MPN: firstNonEmpty(resolved.Variant.MPN, resolved.Component.MPN), Confidence: confidence,
		SymbolID: symbolID, FootprintID: resolved.Variant.FootprintID, PinMapID: resolved.Variant.PinMapID,
		Functions: functions, Units: resolvedUnits, CatalogSources: append([]string(nil), resolved.Component.Verification.Sources...),
		Warnings: warnings, Record: resolved.Component, Variant: resolved.Variant,
		Symbols: append([]components.SymbolBinding(nil), selectedSymbols...),
	}
	for _, function := range functions {
		if evidence, exists := options.LibrarySymbols[function.SymbolID]; exists && evidence.Source != "" {
			component.SymbolSources = append(component.SymbolSources, evidence.Source)
		}
	}
	slices.Sort(component.SymbolSources)
	component.SymbolSources = slices.Compact(component.SymbolSources)
	if evidence, exists := options.LibraryFootprints[resolved.Variant.FootprintID]; exists && evidence.Source != "" {
		component.FootprintSources = []string{evidence.Source}
	}
	return component, resolutionIssues
}

func resolveComponentUnits(path string, instance Component, symbols []components.SymbolBinding) ([]components.SymbolBinding, []ResolvedUnit, []reports.Issue) {
	named := map[string]components.SymbolBinding{}
	anonymous := false
	for index, symbol := range symbols {
		unitID := canonicalUnitID(symbol.UnitID)
		if unitID == "" {
			anonymous = true
			continue
		}
		if _, exists := named[unitID]; exists {
			return nil, nil, []reports.Issue{graphIssue(CodeUnitInvalid, fmt.Sprintf("%s.catalog_symbols[%d].unit_id", path, index), "catalog contains duplicate named symbol unit "+unitID)}
		}
		named[unitID] = symbol
	}
	if len(named) == 0 {
		if len(instance.Units) != 0 {
			return nil, nil, []reports.Issue{graphIssue(CodeUnitInvalid, path+".units", "component declares named units but catalog record has only anonymous symbol units")}
		}
		return append([]components.SymbolBinding(nil), symbols...), nil, nil
	}
	if anonymous {
		return nil, nil, []reports.Issue{graphIssue(CodeUnitInvalid, path+".catalog_symbols", "catalog must not mix named and anonymous symbol units")}
	}
	if len(instance.Units) == 0 {
		return nil, nil, []reports.Issue{graphIssue(CodeUnitInvalid, path+".units", "named multi-unit catalog component requires explicit unit declarations")}
	}
	declared := make(map[string]ComponentUnit, len(instance.Units))
	for index, unit := range instance.Units {
		unitID := canonicalUnitID(unit.ID)
		if _, exists := named[unitID]; !exists {
			return nil, nil, []reports.Issue{graphIssue(CodeUnitInvalid, fmt.Sprintf("%s.units[%d].id", path, index), "declared unit is absent from catalog evidence")}
		}
		if _, exists := declared[unitID]; exists {
			return nil, nil, []reports.Issue{graphIssue(CodeUnitInvalid, fmt.Sprintf("%s.units[%d].id", path, index), "duplicate declared component unit "+unitID)}
		}
		declared[unitID] = ComponentUnit{ID: unitID, Role: unit.Role}
	}
	var missingRequired []string
	for unitID, symbol := range named {
		if symbol.RequiredUnit {
			if _, exists := declared[unitID]; !exists {
				missingRequired = append(missingRequired, unitID)
			}
		}
	}
	if len(missingRequired) != 0 {
		slices.Sort(missingRequired)
		return nil, nil, []reports.Issue{graphIssue(CodeUnitInvalid, path+".units", "required catalog units are not declared: "+strings.Join(missingRequired, ", "))}
	}
	selected := make([]components.SymbolBinding, 0, len(declared))
	units := make([]ResolvedUnit, 0, len(declared))
	for unitID, declaration := range declared {
		symbol := named[unitID]
		selected = append(selected, symbol)
		units = append(units, ResolvedUnit{
			ID: unitID, Role: declaration.Role, Type: symbol.UnitType,
			Required: symbol.RequiredUnit, Unit: symbol.Unit, SymbolID: symbol.SymbolID,
		})
	}
	slices.SortStableFunc(selected, func(left, right components.SymbolBinding) int {
		if left.Unit != right.Unit {
			return left.Unit - right.Unit
		}
		return strings.Compare(left.UnitID, right.UnitID)
	})
	slices.SortStableFunc(units, func(left, right ResolvedUnit) int { return strings.Compare(left.ID, right.ID) })
	return selected, units, nil
}

func explicitVariantID(record components.ComponentRecord, exists bool, requested string) (string, *reports.Issue) {
	if !exists {
		issue := graphIssue(CodeComponentUnresolved, "component_id", "component id is not present in the catalog")
		return "", &issue
	}
	if requested != "" {
		for _, variant := range record.Packages {
			if variant.ID == requested {
				return requested, nil
			}
		}
		issue := graphIssue(CodeComponentUnresolved, "variant_id", "requested variant does not belong to component")
		return "", &issue
	}
	if len(record.Packages) == 1 {
		return record.Packages[0].ID, nil
	}
	issue := graphIssue(CodeComponentAmbiguous, "variant_id", "component has multiple variants; variant_id is required")
	return "", &issue
}

func componentSelectionRequest(instance Component, acceptance AcceptanceLevel) components.SelectionRequest {
	query := components.Query{}
	if instance.Query != nil {
		query = components.Query{
			Text: instance.Query.Text, Family: instance.Query.Family, Package: instance.Query.Package,
			ValueKind: instance.Query.ValueKind, Value: instance.Query.Value,
			MinVoltageV: instance.Query.MinVoltageV, MinimumConfidence: instance.Query.MinimumConfidence,
		}
	}
	ratings := make([]components.RequiredRating, len(instance.RequiredRatings))
	for index, rating := range instance.RequiredRatings {
		ratings[index] = components.RequiredRating{Kind: rating.Kind, Value: rating.Value, Unit: rating.Unit}
	}
	return components.SelectionRequest{
		Query: query, Acceptance: componentAcceptance(acceptance), RequiredRatings: ratings,
		RequiredFunctions: append([]string(nil), instance.RequiredFunctions...),
		AllowAlternatives: false, RequireConcrete: false, RequireCompanions: false,
	}
}

func componentAcceptance(acceptance AcceptanceLevel) components.AcceptanceLevel {
	switch acceptance {
	case AcceptanceStructural:
		return components.AcceptanceStructural
	case AcceptanceConnectivity:
		return components.AcceptanceConnectivity
	case AcceptanceERCDRC:
		return components.AcceptanceERCDRC
	case AcceptanceFabricationCandidate:
		return components.AcceptanceFabricationCandidate
	default:
		return components.AcceptanceDraft
	}
}

func componentConstraintIssue(path string, instance Component, resolved components.ResolvedComponent) *reports.Issue {
	if instance.Manufacturer != "" && !strings.EqualFold(instance.Manufacturer, resolved.Component.Manufacturer) {
		issue := graphIssue(CodeComponentUnresolved, path+".manufacturer", "manufacturer constraint does not match catalog evidence")
		return &issue
	}
	mpn := firstNonEmpty(resolved.Variant.MPN, resolved.Component.MPN)
	if instance.MPN != "" && !strings.EqualFold(instance.MPN, mpn) {
		issue := graphIssue(CodeComponentUnresolved, path+".mpn", "MPN constraint does not match catalog evidence")
		return &issue
	}
	if instance.Symbol != nil {
		matched := false
		for _, symbol := range resolved.Component.Symbols {
			matched = matched || symbol.SymbolID == instance.Symbol.LibraryID
		}
		if !matched {
			issue := graphIssue(CodeSymbolMismatch, path+".symbol.library_id", "symbol constraint does not match catalog evidence")
			return &issue
		}
	}
	if instance.Footprint != nil && instance.Footprint.LibraryID != resolved.Variant.FootprintID {
		issue := graphIssue(CodeFootprintMismatch, path+".footprint.library_id", "footprint constraint does not match catalog evidence")
		return &issue
	}
	return nil
}

func resolveFunctions(path string, symbols []components.SymbolBinding, variant components.PackageVariant) ([]ResolvedFunction, []reports.Issue) {
	padByFunction := map[string][]components.PadFunction{}
	padCanonical := map[string]string{}
	var issues []reports.Issue
	for _, pad := range variant.PadFunctions {
		key := normalizedFunctionKey(pad.Function)
		if existing, exists := padCanonical[key]; exists && existing != pad.Function {
			issues = append(issues, graphIssue(CodePinmapConflict, path+".functions."+key, "footprint functions differ only by case"))
		}
		padCanonical[key] = pad.Function
		padByFunction[key] = append(padByFunction[key], pad)
	}
	var resolved []ResolvedFunction
	padOwners := map[string]string{}
	for _, symbol := range symbols {
		pinsByFunction := map[string][]components.FunctionPin{}
		pinCanonical := map[string]string{}
		for _, pin := range symbol.FunctionPins {
			key := normalizedFunctionKey(pin.Function)
			if existing, exists := pinCanonical[key]; exists && existing != pin.Function {
				issues = append(issues, graphIssue(CodePinmapConflict, path+".functions."+key, "symbol functions differ only by case"))
			}
			pinCanonical[key] = pin.Function
			pinsByFunction[key] = append(pinsByFunction[key], pin)
		}
		functions := make([]string, 0, len(pinsByFunction))
		for function := range pinsByFunction {
			functions = append(functions, function)
		}
		slices.Sort(functions)
		for _, functionKey := range functions {
			pins := pinsByFunction[functionKey]
			pads := append([]components.PadFunction(nil), padByFunction[functionKey]...)
			slices.SortStableFunc(pins, func(left, right components.FunctionPin) int { return strings.Compare(left.SymbolPin, right.SymbolPin) })
			slices.SortStableFunc(pads, func(left, right components.PadFunction) int { return strings.Compare(left.Pad, right.Pad) })
			function := pins[0].Function
			if len(pads) == 0 {
				issues = append(issues, graphIssue(CodePadUnresolved, path+".functions."+function, "logical function has no footprint pad mapping"))
				continue
			}
			usedPads := map[int]struct{}{}
			for _, pin := range pins {
				padIndex := -1
				for candidateIndex, pad := range pads {
					if pad.Pad == pin.SymbolPin {
						if _, used := usedPads[candidateIndex]; !used {
							padIndex = candidateIndex
							break
						}
					}
				}
				if padIndex < 0 && len(pins) == 1 && len(pads) == 1 {
					padIndex = 0
				}
				if padIndex < 0 {
					issues = append(issues, graphIssue(CodePinmapConflict, path+".functions."+function, "cannot pair symbol pin with footprint pad"))
					continue
				}
				usedPads[padIndex] = struct{}{}
				pad := pads[padIndex]
				// V1 permits repeated unit views of the same physical pin, but it
				// deliberately rejects distinct symbol pins collapsed onto one pad.
				if owner, exists := padOwners[pad.Pad]; exists && owner != pin.SymbolPin {
					issues = append(issues, graphIssue(CodePinmapConflict, path+".functions."+function, "v1 does not support distinct symbol pins mapped to one footprint pad"))
					continue
				}
				padOwners[pad.Pad] = pin.SymbolPin
				aliases := append([]string(nil), pin.Aliases...)
				aliases = append(aliases, pad.Aliases...)
				slices.Sort(aliases)
				aliases = slices.Compact(aliases)
				resolved = append(resolved, ResolvedFunction{
					Function: function, Aliases: aliases, SymbolID: symbol.SymbolID, Unit: symbol.Unit, UnitID: canonicalUnitID(symbol.UnitID),
					SymbolPin: pin.SymbolPin, Pad: pad.Pad, Electrical: pin.Electrical,
					Polarity: firstNonEmpty(pin.Polarity, pad.Polarity), Required: pin.Required,
				})
			}
		}
	}
	slices.SortStableFunc(resolved, func(left, right ResolvedFunction) int {
		if left.Function != right.Function {
			return strings.Compare(left.Function, right.Function)
		}
		if left.Unit != right.Unit {
			return left.Unit - right.Unit
		}
		return strings.Compare(left.SymbolPin, right.SymbolPin)
	})
	return resolved, issues
}

func normalizedFunctionKey(function string) string {
	return strings.ToUpper(strings.TrimSpace(function))
}

func verifyLibraryEvidence(path string, functions []ResolvedFunction, resolved components.ResolvedComponent, options ResolveOptions) []reports.Issue {
	if !options.RequireLibraryEvidence && options.LibrarySymbols == nil && options.LibraryFootprints == nil {
		return nil
	}
	var issues []reports.Issue
	for _, function := range functions {
		symbol, symbolOK := options.LibrarySymbols[function.SymbolID]
		if !symbolOK {
			if options.RequireLibraryEvidence {
				issues = append(issues, graphIssue(CodeSymbolMismatch, path+".symbol", "symbol is absent from library evidence"))
			}
		} else if _, exists := symbol.Pins[function.SymbolPin]; !exists {
			issues = append(issues, graphIssue(CodePinUnresolved, path+".symbol", "symbol pin is absent from library evidence"))
		}
		footprint, footprintOK := options.LibraryFootprints[resolved.Variant.FootprintID]
		if !footprintOK {
			if options.RequireLibraryEvidence {
				issues = append(issues, graphIssue(CodeFootprintMismatch, path+".footprint", "footprint is absent from library evidence"))
			}
		} else if _, exists := footprint.Pads[function.Pad]; !exists {
			issues = append(issues, graphIssue(CodePadUnresolved, path+".footprint", "footprint pad is absent from library evidence"))
		}
	}
	return dedupeGraphIssues(issues)
}

func resolveEndpoint(path string, endpoint Endpoint, componentsByID map[string]ResolvedComponent) (ResolvedEndpoint, []reports.Issue) {
	component, exists := componentsByID[endpoint.Component]
	if !exists {
		return ResolvedEndpoint{}, []reports.Issue{graphIssue(CodePinUnresolved, path, "component was not resolved")}
	}
	unit, unitSpecified, unitOK := resolvedEndpointUnit(component, endpoint.Unit)
	if !unitOK {
		return ResolvedEndpoint{}, []reports.Issue{graphIssue(CodePinUnresolved, path+".unit", "unit selector is invalid")}
	}
	var matches []ResolvedFunction
	for _, function := range component.Functions {
		if unitSpecified && function.Unit != unit {
			continue
		}
		matched := false
		switch endpoint.SelectorKind {
		case SelectorFunction:
			matched = strings.EqualFold(function.Function, endpoint.Selector)
		case SelectorAlias:
			for _, alias := range function.Aliases {
				matched = matched || strings.EqualFold(alias, endpoint.Selector)
			}
		case SelectorSymbolPin:
			matched = function.SymbolPin == endpoint.Selector
		}
		if matched {
			matches = append(matches, function)
		}
	}
	if len(matches) == 0 {
		return ResolvedEndpoint{}, []reports.Issue{graphIssue(CodePinUnresolved, path+".selector", "selector does not resolve to a verified function and pin")}
	}
	functionName := matches[0].Function
	matchedUnits := map[int]struct{}{matches[0].Unit: {}}
	for _, match := range matches[1:] {
		if match.Function != functionName {
			return ResolvedEndpoint{}, []reports.Issue{graphIssue(CodePinmapConflict, path+".selector", "selector resolves to multiple logical functions")}
		}
		matchedUnits[match.Unit] = struct{}{}
	}
	if !unitSpecified && len(matchedUnits) > 1 {
		return ResolvedEndpoint{}, []reports.Issue{graphIssue(CodePinmapConflict, path+".unit", "unit is required because the selector matches multiple symbol units")}
	}
	bindings := make([]ResolvedBinding, 0, len(matches))
	for _, match := range matches {
		bindings = append(bindings, ResolvedBinding{SymbolID: match.SymbolID, Unit: match.Unit, UnitID: match.UnitID, SymbolPin: match.SymbolPin, Pad: match.Pad, Electrical: match.Electrical, Polarity: match.Polarity})
	}
	return ResolvedEndpoint{Intent: endpoint, Function: functionName, Bindings: bindings}, nil
}

func resolvedEndpointUnit(component ResolvedComponent, value string) (int, bool, bool) {
	if len(component.Units) == 0 {
		return parseUnitSelector(value)
	}
	unitID := canonicalUnitID(value)
	if unitID == "" {
		return 0, false, false
	}
	for _, unit := range component.Units {
		if unit.ID == unitID {
			return unit.Unit, true, true
		}
	}
	return 0, true, false
}

func canonicalFunction(functions []ResolvedFunction, requested string) (string, bool) {
	for _, function := range functions {
		if strings.EqualFold(function.Function, requested) {
			return function.Function, true
		}
		for _, alias := range function.Aliases {
			if strings.EqualFold(alias, requested) {
				return function.Function, true
			}
		}
	}
	return "", false
}

type physicalBindingID struct {
	component string
	pad       string
}

func registerPhysicalBindings(path, component string, bindings []ResolvedBinding, owners map[physicalBindingID]string) []reports.Issue {
	var issues []reports.Issue
	for _, binding := range bindings {
		key := physicalBindingKey(component, binding)
		if owner, exists := owners[key]; exists && owner != path {
			issues = append(issues, graphIssue(CodePinmapConflict, path, "physical symbol pin is already assigned by "+owner))
			continue
		}
		owners[key] = path
	}
	return issues
}

func markUsedBindings(component string, bindings []ResolvedBinding, used map[physicalBindingID]struct{}) {
	for _, binding := range bindings {
		used[physicalBindingKey(component, binding)] = struct{}{}
	}
}

func physicalBindingKey(component string, binding ResolvedBinding) physicalBindingID {
	return physicalBindingID{component: component, pad: binding.Pad}
}

func parseUnitSelector(value string) (int, bool, bool) {
	if value == "" {
		return 0, false, true
	}
	if number, err := strconv.Atoi(value); err == nil && number > 0 {
		return number, true, true
	}
	runes := []rune(strings.ToUpper(value))
	if len(runes) == 1 && unicode.IsLetter(runes[0]) && runes[0] >= 'A' && runes[0] <= 'Z' {
		return int(runes[0]-'A') + 1, true, true
	}
	return 0, true, false
}

func catalogHash(catalog *components.Catalog) string {
	families := append([]components.FamilyDefinition(nil), catalog.Families...)
	records := append([]components.ComponentRecord(nil), catalog.Records...)
	slices.SortStableFunc(families, func(left, right components.FamilyDefinition) int {
		return strings.Compare(left.ID, right.ID)
	})
	slices.SortStableFunc(records, func(left, right components.ComponentRecord) int {
		return strings.Compare(left.ID, right.ID)
	})
	return hashGraphValue(struct {
		Version  string                        `json:"version"`
		Families []components.FamilyDefinition `json:"families"`
		Records  []components.ComponentRecord  `json:"records"`
	}{Version: catalog.Version, Families: families, Records: records})
}

func libraryEvidenceHash(options ResolveOptions) string {
	type entry struct {
		ID, Source string
		Values     []string
	}
	var entries []entry
	for id, evidence := range options.LibrarySymbols {
		values := make([]string, 0, len(evidence.Pins))
		for pin := range evidence.Pins {
			values = append(values, pin)
		}
		slices.Sort(values)
		entries = append(entries, entry{ID: "symbol:" + id, Source: evidence.Source, Values: values})
	}
	for id, evidence := range options.LibraryFootprints {
		values := make([]string, 0, len(evidence.Pads))
		for pad := range evidence.Pads {
			values = append(values, pad)
		}
		slices.Sort(values)
		entries = append(entries, entry{ID: "footprint:" + id, Source: evidence.Source, Values: values})
	}
	slices.SortStableFunc(entries, func(left, right entry) int { return strings.Compare(left.ID, right.ID) })
	if len(entries) == 0 {
		return ""
	}
	return hashGraphValue(entries)
}

func resolvedHash(document ResolvedDocument) string {
	copy := document
	copy.ResolutionHash = ""
	return hashGraphValue(copy)
}

func hashGraphValue(value any) string {
	hash := sha256.New()
	if err := json.NewEncoder(hash).Encode(value); err != nil {
		panic(fmt.Sprintf("circuit graph canonical hash: %v", err))
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func summarizeIssues(issues []reports.Issue) string {
	if len(issues) == 0 {
		return "component resolution failed"
	}
	messages := make([]string, 0, len(issues))
	for _, issue := range issues {
		messages = append(messages, issue.Message)
	}
	slices.Sort(messages)
	messages = slices.Compact(messages)
	return strings.Join(messages, "; ")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func weakerGraphConfidence(left, right components.ConfidenceLevel) components.ConfidenceLevel {
	rank := func(value components.ConfidenceLevel) int {
		switch value {
		case components.ConfidenceVerified:
			return 5
		case components.ConfidenceLibraryDerived:
			return 4
		case components.ConfidenceRuleInferred:
			return 3
		case components.ConfidencePlaceholder:
			return 2
		default:
			return 1
		}
	}
	if rank(left) <= rank(right) {
		return left
	}
	return right
}

func dedupeGraphIssues(issues []reports.Issue) []reports.Issue {
	return finalizeGraphIssues(issues)
}
