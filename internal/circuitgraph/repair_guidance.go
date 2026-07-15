package circuitgraph

import (
	"sort"
	"strconv"
	"strings"

	"kicadai/internal/components"
	"kicadai/internal/reports"
)

// RepairOption is advisory, bounded input for circuit patch. It never applies
// a change and may omit a candidate whenever the graph evidence is ambiguous.
type RepairOption struct {
	DiagnosticID      string              `json:"diagnostic_id"`
	Code              reports.Code        `json:"code"`
	Path              string              `json:"path"`
	OperationTemplate PatchOperation      `json:"operation_template"`
	RequiredValues    []string            `json:"required_values"`
	AllowedValues     map[string][]string `json:"allowed_values,omitempty"`
	Rationale         string              `json:"rationale"`
	Stage             string              `json:"stage"`
	RetryScope        string              `json:"retry_scope"`
	Disposition       string              `json:"disposition"`
}

// RepairOptions returns only deterministic graph/catalog-derived candidates.
func RepairOptions(document Document, catalog *components.Catalog, issues []reports.Issue) []RepairOption {
	result := make([]RepairOption, 0)
	for _, issue := range issues {
		option, ok := repairOption(document, catalog, issue)
		if ok {
			result = append(result, option)
		}
	}
	sort.SliceStable(result, func(i, j int) bool { return result[i].DiagnosticID < result[j].DiagnosticID })
	return result
}

func repairOption(document Document, catalog *components.Catalog, issue reports.Issue) (RepairOption, bool) {
	base := RepairOption{DiagnosticID: issue.IssueID, Code: issue.Code, Path: issue.Path, Stage: issue.Stage, RetryScope: issue.RetryScope, Disposition: "agent_selectable"}
	if base.DiagnosticID == "" || base.Path == "" || base.Stage == "" || base.RetryScope == "" {
		return RepairOption{}, false
	}
	if index, ok := componentIndex(issue.Path); ok && index < len(document.Components) && issue.Code == CodeComponentUnresolved {
		component := document.Components[index]
		ids := catalogIDs(catalog)
		if len(ids) == 0 {
			return RepairOption{}, false
		}
		base.OperationTemplate = PatchOperation{Op: "replace_component", Component: component.ID}
		base.RequiredValues = []string{"component_patch.component_id"}
		base.AllowedValues = map[string][]string{"component_patch.component_id": ids}
		base.Rationale = "component selection did not resolve through the trusted catalog"
		return base, true
	}
	if net, endpoint, ok := endpointAt(document, issue.Path); ok && (issue.Code == CodeUnitInvalid || issue.Code == CodePinUnresolved) {
		if endpoint.Component == "" || componentUnits(document, endpoint.Component) == nil {
			return RepairOption{}, false
		}
		replacement := endpoint
		base.OperationTemplate = PatchOperation{Op: "replace_endpoint", Net: net.Name, Endpoint: &endpoint, Replacement: &replacement}
		if issue.Code == CodeUnitInvalid {
			units := availableFunctionUnits(document, catalog, endpoint.Component, endpoint.Selector, net.Name)
			if len(units) == 0 {
				return RepairOption{}, false
			}
			if len(units) == 1 {
				base.OperationTemplate.Replacement.Unit = units[0]
				base.Rationale = "one verified functional unit remains unused by this component's other graph connections"
			} else {
				base.RequiredValues = []string{"replacement.unit"}
				base.AllowedValues = map[string][]string{"replacement.unit": units}
				base.Rationale = "endpoint unit is not declared; choose a verified unit that supports the endpoint function"
			}
		} else {
			pins := availableSymbolPins(document, catalog, endpoint.Component, net.Name)
			if len(pins) == 1 {
				base.OperationTemplate.Replacement.Selector = pins[0]
				base.Rationale = "one verified symbol pin remains unused by this component's other graph connections"
			} else {
				base.RequiredValues = []string{"replacement.selector"}
				base.AllowedValues = map[string][]string{"replacement.selector": pins}
				base.Rationale = "endpoint selector did not resolve; choose a verified selector from capability output"
			}
		}
		return base, true
	}
	if index, ok := regionIndex(issue.Path); ok && index < len(document.PCB.Regions) && issue.Code == CodePCBConstraintInvalid {
		region := document.PCB.Regions[index]
		base.OperationTemplate = PatchOperation{Op: "replace_pcb_region", Region: region.ID}
		if bounds, ok := clampedRegionBounds(document.Project.Board, region.Bounds); ok {
			base.OperationTemplate.Bounds = &bounds
			base.Rationale = "region origin is valid and its overflowing bounds have one board-limited correction"
		} else {
			base.RequiredValues = []string{"bounds.x_mm", "bounds.y_mm", "bounds.width_mm", "bounds.height_mm"}
			base.AllowedValues = map[string][]string{"bounds.max_width_mm": {strconv.FormatFloat(document.Project.Board.WidthMM, 'f', -1, 64)}, "bounds.max_height_mm": {strconv.FormatFloat(document.Project.Board.HeightMM, 'f', -1, 64)}}
			base.Rationale = "PCB region bounds must be positive and remain inside the declared board"
		}
		return base, true
	}
	return RepairOption{}, false
}

func availableFunctionUnits(document Document, catalog *components.Catalog, componentID, function, netName string) []string {
	if catalog == nil || function == "" {
		return componentUnits(document, componentID)
	}
	var component Component
	for _, candidate := range document.Components {
		if candidate.ID == componentID {
			component = candidate
			break
		}
	}
	if component.ComponentID == "" {
		return nil
	}
	known := map[string]struct{}{}
	for _, record := range catalog.Records {
		if record.ID != component.ComponentID {
			continue
		}
		for _, symbol := range record.Symbols {
			for _, pin := range symbol.FunctionPins {
				if pin.Function == function && symbol.UnitID != "" {
					known[symbol.UnitID] = struct{}{}
				}
			}
		}
	}
	used := map[string]struct{}{}
	for _, net := range document.Nets {
		if net.Name == netName {
			continue
		}
		for _, endpoint := range net.Endpoints {
			if endpoint.Component == componentID && endpoint.SelectorKind == SelectorFunction && endpoint.Selector == function && endpoint.Unit != "" {
				used[endpoint.Unit] = struct{}{}
			}
		}
	}
	result := []string{}
	for _, unit := range componentUnits(document, componentID) {
		if _, supported := known[unit]; !supported {
			continue
		}
		if _, occupied := used[unit]; !occupied {
			result = append(result, unit)
		}
	}
	return result
}

func clampedRegionBounds(board Board, bounds Bounds) (Bounds, bool) {
	if board.WidthMM <= 0 || board.HeightMM <= 0 || bounds.XMM < 0 || bounds.YMM < 0 || bounds.XMM >= board.WidthMM || bounds.YMM >= board.HeightMM || bounds.WidthMM <= 0 || bounds.HeightMM <= 0 {
		return Bounds{}, false
	}
	maximumWidth, maximumHeight := board.WidthMM-bounds.XMM, board.HeightMM-bounds.YMM
	if bounds.WidthMM <= maximumWidth && bounds.HeightMM <= maximumHeight {
		return Bounds{}, false
	}
	if maximumWidth <= 0 || maximumHeight <= 0 {
		return Bounds{}, false
	}
	corrected := bounds
	if corrected.WidthMM > maximumWidth {
		corrected.WidthMM = maximumWidth
	}
	if corrected.HeightMM > maximumHeight {
		corrected.HeightMM = maximumHeight
	}
	return corrected, true
}

func availableSymbolPins(document Document, catalog *components.Catalog, componentID, netName string) []string {
	var component Component
	for _, candidate := range document.Components {
		if candidate.ID == componentID {
			component = candidate
			break
		}
	}
	if component.ComponentID == "" || catalog == nil {
		return nil
	}
	known := map[string]struct{}{}
	for _, record := range catalog.Records {
		if record.ID != component.ComponentID {
			continue
		}
		for _, symbol := range record.Symbols {
			for _, pin := range symbol.FunctionPins {
				if pin.SymbolPin != "" {
					known[pin.SymbolPin] = struct{}{}
				}
			}
		}
	}
	used := map[string]struct{}{}
	for _, net := range document.Nets {
		if net.Name == netName {
			continue
		}
		for _, endpoint := range net.Endpoints {
			if endpoint.Component == componentID && endpoint.SelectorKind == SelectorSymbolPin {
				used[endpoint.Selector] = struct{}{}
			}
		}
	}
	result := []string{}
	for pin := range known {
		if _, exists := used[pin]; !exists {
			result = append(result, pin)
		}
	}
	sort.Strings(result)
	return result
}

func componentIndex(path string) (int, bool) { return indexedPath(path, "components[") }
func regionIndex(path string) (int, bool)    { return indexedPath(path, "pcb.regions[") }

func indexedPath(path, prefix string) (int, bool) {
	if !strings.HasPrefix(path, prefix) {
		return 0, false
	}
	rest := strings.TrimPrefix(path, prefix)
	end := strings.IndexByte(rest, ']')
	if end < 1 {
		return 0, false
	}
	index, err := strconv.Atoi(rest[:end])
	return index, err == nil && index >= 0
}

func endpointAt(document Document, path string) (Net, Endpoint, bool) {
	if !strings.HasPrefix(path, "nets[") {
		return Net{}, Endpoint{}, false
	}
	parts := strings.Split(path, ".endpoints[")
	if len(parts) != 2 {
		return Net{}, Endpoint{}, false
	}
	netIndex, ok := indexedPath(parts[0], "nets[")
	if !ok || netIndex >= len(document.Nets) {
		return Net{}, Endpoint{}, false
	}
	endpointIndex, ok := indexedPath("components["+parts[1], "components[")
	if !ok || endpointIndex >= len(document.Nets[netIndex].Endpoints) {
		return Net{}, Endpoint{}, false
	}
	return document.Nets[netIndex], document.Nets[netIndex].Endpoints[endpointIndex], true
}

func componentUnits(document Document, id string) []string {
	for _, component := range document.Components {
		if component.ID == id {
			units := make([]string, 0, len(component.Units))
			for _, unit := range component.Units {
				units = append(units, unit.ID)
			}
			sort.Strings(units)
			return units
		}
	}
	return nil
}

func catalogIDs(catalog *components.Catalog) []string {
	if catalog == nil {
		return nil
	}
	ids := make([]string, 0, len(catalog.Records))
	for _, record := range catalog.Records {
		ids = append(ids, record.ID)
	}
	sort.Strings(ids)
	return ids
}
