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
		AutoHierarchy: resolved.Source.Schematic.Hierarchy.Mode == "auto",
		Components:    make([]designworkflow.ExplicitComponentSpec, 0, len(resolved.Components)),
		Nets:          make([]designworkflow.ExplicitNetSpec, 0, len(resolved.Nets)),
	}
	if simulation := resolved.Source.Simulation; simulation != nil {
		explicit.Simulation = &designworkflow.ExplicitSimulationSpec{
			ModelID: simulation.ModelID, Component: simulation.Component,
			InputVoltageV: simulation.InputVoltageV, LoadCurrentMA: simulation.LoadCurrentMA, OutputNominalV: simulation.OutputNominalV,
			OutputMinV: simulation.OutputMinV, OutputMaxV: simulation.OutputMaxV,
		}
	}
	for _, flag := range resolved.Source.PowerFlags {
		explicit.SchematicSupport = append(explicit.SchematicSupport, designworkflow.ExplicitSchematicSupportSpec{
			ID: powerFlagComponentID(flag.Net), Kind: designworkflow.ExplicitSchematicSupportPowerFlag, Net: flag.Net,
		})
	}
	for _, region := range resolved.Source.PCB.Regions {
		explicit.Regions = append(explicit.Regions, designworkflow.ExplicitRegionSpec{
			ID: region.ID, Role: region.Role, XMM: region.Bounds.XMM, YMM: region.Bounds.YMM,
			WidthMM: region.Bounds.WidthMM, HeightMM: region.Bounds.HeightMM,
		})
	}
	for _, keepout := range resolved.Source.PCB.Keepouts {
		explicit.Keepouts = append(explicit.Keepouts, designworkflow.ExplicitKeepoutSpec{
			ID: keepout.ID, XMM: keepout.Bounds.XMM, YMM: keepout.Bounds.YMM,
			WidthMM: keepout.Bounds.WidthMM, HeightMM: keepout.Bounds.HeightMM,
			Layers: append([]string(nil), keepout.Layers...),
		})
	}
	for _, zone := range resolved.Source.PCB.Zones {
		explicit.Zones = append(explicit.Zones, designworkflow.ExplicitZoneSpec{
			Net: zone.Net, Layers: append([]string(nil), zone.Layers...), ClearanceMM: zone.ClearanceMM,
		})
	}
	placements := make(map[string]PCBPlacement, len(resolved.Source.PCB.Placements))
	for _, placement := range resolved.Source.PCB.Placements {
		placements[placement.Component] = placement
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
		placement := placements[component.Instance.ID]
		explicit.Components = append(explicit.Components, designworkflow.ExplicitComponentSpec{
			ID: component.Instance.ID, Reference: references[component.Instance.ID], Role: string(component.Instance.Role),
			Value: schematicValue(component, resolved.Source.Policy), FootprintID: component.FootprintID,
			SchematicUnits: resolvedSchematicUnitIDs(component), Pads: pads,
			Placement: designworkflow.ExplicitPlacementSpec{Region: placement.Region, Near: placement.Near, Edge: string(placement.Edge), Priority: placement.Priority, MaxDistanceMM: placement.MaxDistanceMM},
		})
	}
	for _, net := range resolved.Nets {
		explicitNet := designworkflow.ExplicitNetSpec{
			Name: net.Intent.Name, Role: string(net.Intent.Role), NetClass: net.Intent.NetClass,
			Required: net.Intent.Required != nil && *net.Intent.Required, CurrentMA: net.Intent.CurrentMA,
			WidthMM: net.Intent.WidthMM, ClearanceMM: net.Intent.ClearanceMM,
		}
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
		Constraints:     designworkflow.ConstraintSpec{AllowBackLayer: resolved.Source.Project.Board.Layers > 1},
		ExplicitCircuit: &explicit,
		Validation: designworkflow.ValidationSpec{
			Acceptance: designWorkflowAcceptance(resolved.Source.Project.Acceptance),
			RequireERC: resolved.Source.Project.Acceptance == AcceptanceERCDRC || resolved.Source.Project.Acceptance == AcceptanceFabricationCandidate,
			RequireDRC: resolved.Source.Project.Acceptance == AcceptanceERCDRC || resolved.Source.Project.Acceptance == AcceptanceFabricationCandidate,
		},
	}
	designworkflow.EnableGeneratedRoutingRetry(&request, designworkflow.GenericAutonomousCorrectionMaxAttempts)
	request.RoutingRetry.PreserveFixed = true
	request.RoutingRetry.StopOnNewBlockers = true
	request.RoutingRetry.StopOnRepeatedSignature = true
	request.RoutingRetry.StopOnNonImprovement = true
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
