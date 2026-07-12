package circuitgraph

import (
	"slices"
	"strings"

	"kicadai/internal/designworkflow"
	"kicadai/internal/reports"
)

func ToDesignRequest(resolved ResolvedDocument) (designworkflow.Request, []reports.Issue) {
	schematic, issues := ToSchematicIR(resolved)
	if reports.HasBlockingIssue(issues) {
		return designworkflow.Request{}, issues
	}
	references, referenceIssues := schematicReferences(resolved)
	issues = append(issues, referenceIssues...)
	if reports.HasBlockingIssue(issues) {
		return designworkflow.Request{}, dedupeGraphIssues(issues)
	}

	padNets := map[physicalBindingID]string{}
	for _, net := range resolved.Nets {
		for _, endpoint := range net.Endpoints {
			for _, binding := range endpoint.Bindings {
				key := physicalBindingKey(endpoint.Intent.Component, binding)
				if existing := padNets[key]; existing != "" && existing != net.Intent.Name {
					issues = append(issues, graphIssue(CodeSchematicLowering, "nets."+net.Intent.Name, "one physical pad resolves to multiple nets"))
					continue
				}
				padNets[key] = net.Intent.Name
			}
		}
	}

	explicit := designworkflow.ExplicitCircuitSpec{
		ResolutionHash: resolved.ResolutionHash, CatalogID: resolved.CatalogID,
		CatalogHash: resolved.CatalogHash, Schematic: schematic,
		Components: make([]designworkflow.ExplicitComponentSpec, 0, len(resolved.Components)),
		Nets:       make([]designworkflow.ExplicitNetSpec, 0, len(resolved.Nets)),
	}
	for _, component := range resolved.Components {
		padsByName := map[string]designworkflow.ExplicitPadSpec{}
		for _, function := range component.Functions {
			pad := designworkflow.ExplicitPadSpec{
				Name: function.Pad, SymbolPin: function.SymbolPin,
				Net: padNets[physicalBindingID{component: component.Instance.ID, pad: function.Pad}],
			}
			if existing, exists := padsByName[pad.Name]; exists && (existing.SymbolPin != pad.SymbolPin || existing.Net != pad.Net) {
				issues = append(issues, graphIssue(CodePinmapConflict, "components."+component.Instance.ID+".pads."+pad.Name, "one footprint pad has conflicting resolved bindings"))
				continue
			}
			padsByName[pad.Name] = pad
		}
		pads := make([]designworkflow.ExplicitPadSpec, 0, len(padsByName))
		for _, pad := range padsByName {
			pads = append(pads, pad)
		}
		slices.SortStableFunc(pads, func(left, right designworkflow.ExplicitPadSpec) int { return strings.Compare(left.Name, right.Name) })
		explicit.Components = append(explicit.Components, designworkflow.ExplicitComponentSpec{
			ID: component.Instance.ID, Reference: references[component.Instance.ID], Role: string(component.Instance.Role),
			Value: schematicValue(component, resolved.Source.Policy), FootprintID: component.FootprintID, Pads: pads,
		})
	}
	for _, net := range resolved.Nets {
		explicitNet := designworkflow.ExplicitNetSpec{Name: net.Intent.Name}
		seen := map[physicalBindingID]struct{}{}
		for _, endpoint := range net.Endpoints {
			for _, binding := range endpoint.Bindings {
				key := physicalBindingKey(endpoint.Intent.Component, binding)
				if _, exists := seen[key]; exists {
					continue
				}
				seen[key] = struct{}{}
				explicitNet.Endpoints = append(explicitNet.Endpoints, designworkflow.ExplicitNetEndpoint{Component: endpoint.Intent.Component, Pad: binding.Pad})
			}
		}
		slices.SortStableFunc(explicitNet.Endpoints, func(left, right designworkflow.ExplicitNetEndpoint) int {
			if left.Component != right.Component {
				return strings.Compare(left.Component, right.Component)
			}
			return strings.Compare(left.Pad, right.Pad)
		})
		explicit.Nets = append(explicit.Nets, explicitNet)
	}

	request := designworkflow.Request{
		Version: designworkflow.RequestVersion, Name: resolved.Source.Project.Name,
		Intent: designworkflow.Intent{Summary: resolved.Source.Project.Description, Category: "explicit_circuit_graph"},
		Board: designworkflow.BoardSpec{
			WidthMM: resolved.Source.Project.Board.WidthMM, HeightMM: resolved.Source.Project.Board.HeightMM,
			Layers: resolved.Source.Project.Board.Layers, EdgeClearanceMM: resolved.Source.Project.Board.EdgeClearanceMM,
		},
		ExplicitCircuit: &explicit,
		Validation: designworkflow.ValidationSpec{
			Acceptance: designWorkflowAcceptance(resolved.Source.Project.Acceptance),
			RequireERC: resolved.Source.Project.Acceptance == AcceptanceERCDRC || resolved.Source.Project.Acceptance == AcceptanceFabricationCandidate,
			RequireDRC: resolved.Source.Project.Acceptance == AcceptanceERCDRC || resolved.Source.Project.Acceptance == AcceptanceFabricationCandidate,
		},
	}
	request = designworkflow.NormalizeRequest(request)
	issues = append(issues, designworkflow.ValidateRequest(request)...)
	if reports.HasBlockingIssue(issues) {
		return designworkflow.Request{}, dedupeGraphIssues(issues)
	}
	return request, dedupeGraphIssues(issues)
}

func designWorkflowAcceptance(acceptance AcceptanceLevel) designworkflow.AcceptanceLevel {
	switch acceptance {
	case AcceptanceStructural:
		return designworkflow.AcceptanceStructural
	case AcceptanceConnectivity:
		return designworkflow.AcceptanceConnectivity
	case AcceptanceERCDRC:
		return designworkflow.AcceptanceERCDRC
	case AcceptanceFabricationCandidate:
		return designworkflow.AcceptanceFabricationCandidate
	default:
		return designworkflow.AcceptanceDraft
	}
}
