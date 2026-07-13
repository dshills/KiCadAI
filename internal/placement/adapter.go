package placement

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"kicadai/internal/blocks"
	"kicadai/internal/libraryresolver"
	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

type AdapterOptions struct {
	Board          BoardPlacementArea
	Rules          Rules
	DefaultBounds  Bounds
	LibraryIndex   *libraryresolver.LibraryIndex
	PreservePlaced bool
}

var netNameRoleTokenPattern = regexp.MustCompile(`[a-z0-9]+`)

func RequestFromBlockOutput(output blocks.BlockOutput, opts AdapterOptions) (Request, []reports.Issue) {
	request, issues := RequestFromOperations(output.Operations, opts)
	request.Seed = output.Instance.InstanceID
	return request, combineIssues(output.Issues, issues)
}

func RequestFromBlockPCBRealization(realization blocks.BlockPCBRealizationResult, opts AdapterOptions) (Request, []reports.Issue) {
	request := Request{
		Board:    opts.Board,
		Rules:    opts.Rules,
		Existing: ExistingPlacementPolicy{PreserveFixed: opts.PreservePlaced},
		Seed:     realization.Instance.InstanceID,
	}
	for _, component := range realization.Components {
		placement := Placement{
			XMM:         component.Placement.XMM,
			YMM:         component.Placement.YMM,
			RotationDeg: component.Placement.RotationDeg,
			Layer:       firstNonEmpty(component.Placement.Layer, "F.Cu"),
		}
		fixed := component.Placement.Fixed || opts.PreservePlaced
		request.Components = append(request.Components, Component{
			Ref:         component.Ref,
			Value:       component.Value,
			FootprintID: component.FootprintID,
			Role:        component.ComponentRole,
			Fixed:       fixed,
			Position:    &placement,
			Side:        sideFromLayer(placement.Layer),
			Rotation:    fixedRotation(component.Placement.RotationDeg),
		})
	}
	for _, route := range realization.LocalRoutes {
		request.Nets = append(request.Nets, Net{
			Name: route.NetName,
			Endpoints: []Endpoint{
				{Ref: route.From.Ref, Pin: route.From.Pin},
				{Ref: route.To.Ref, Pin: route.To.Pin},
			},
			Role:   netRoleFromName(route.NetName),
			Weight: 10,
		})
	}
	request = NormalizeRequest(request)
	return request, combineIssues(realization.Issues, Validate(request))
}

func RequestFromCompositionOutput(output blocks.CompositionOutput, opts AdapterOptions) (Request, []reports.Issue) {
	request, issues := RequestFromOperations(output.Operations, opts)
	request.Seed = output.ProjectName
	return request, combineIssues(output.Issues, issues)
}

func RequestFromOperations(operations []transactions.Operation, opts AdapterOptions) (Request, []reports.Issue) {
	builder := operationRequestBuilder{
		components: map[string]*Component{},
		netPins:    map[string]map[string]Endpoint{},
		opts:       opts,
	}
	for index, operation := range operations {
		builder.ingest(index, operation)
	}
	return Request{
		Board:      opts.Board,
		Components: builder.sortedComponents(),
		Nets:       builder.sortedNets(),
		Rules:      opts.Rules,
		Existing:   ExistingPlacementPolicy{PreserveFixed: opts.PreservePlaced},
	}, builder.issues
}

type operationRequestBuilder struct {
	components map[string]*Component
	netPins    map[string]map[string]Endpoint
	issues     []reports.Issue
	opts       AdapterOptions
}

func (builder *operationRequestBuilder) ingest(index int, operation transactions.Operation) {
	switch operation.Op {
	case transactions.OpAddSymbol:
		var payload transactions.AddSymbolOperation
		if !builder.decode(index, operation, &payload) {
			return
		}
		component := builder.component(payload.Ref)
		component.Ref = strings.TrimSpace(payload.Ref)
		component.Value = payload.Value
	case transactions.OpAssignFootprint:
		var payload transactions.AssignFootprintOperation
		if !builder.decode(index, operation, &payload) {
			return
		}
		component := builder.component(payload.Ref)
		component.Ref = strings.TrimSpace(payload.Ref)
		component.FootprintID = strings.TrimSpace(payload.FootprintID)
		builder.hydrateFootprint(index, component)
	case transactions.OpPlaceFootprint:
		var payload transactions.PlaceFootprintOperation
		if !builder.decode(index, operation, &payload) {
			return
		}
		component := builder.component(payload.Ref)
		component.Ref = strings.TrimSpace(payload.Ref)
		component.Value = firstNonEmpty(component.Value, payload.Value)
		component.FootprintID = firstNonEmpty(component.FootprintID, strings.TrimSpace(payload.FootprintID))
		if len(payload.Pads) > 0 {
			component.Pads = padSummaries(payload.Pads)
			if component.Bounds.WidthMM <= 0 || component.Bounds.HeightMM <= 0 {
				component.Bounds = boundsFromPadSpecs(payload.Pads)
			}
		}
		builder.hydrateFootprint(index, component)
		if builder.opts.PreservePlaced {
			component.Fixed = true
			placement := Placement{XMM: payload.At.XMM, YMM: payload.At.YMM, RotationDeg: payload.Rotation, Layer: payload.Layer}
			component.Position = &placement
		}
	case transactions.OpConnect:
		var payload transactions.ConnectOperation
		if !builder.decode(index, operation, &payload) {
			return
		}
		builder.addNetEndpoint(payload.NetName, payload.From)
		builder.addNetEndpoint(payload.NetName, payload.To)
	}
}

func (builder *operationRequestBuilder) decode(index int, operation transactions.Operation, payload any) bool {
	if len(operation.Raw) == 0 {
		builder.issues = append(builder.issues, adapterIssue(index, "operation payload missing raw JSON"))
		return false
	}
	if err := json.Unmarshal(operation.Raw, payload); err != nil {
		builder.issues = append(builder.issues, adapterIssue(index, err.Error()))
		return false
	}
	return true
}

func (builder *operationRequestBuilder) component(ref string) *Component {
	key := strings.ToUpper(strings.TrimSpace(ref))
	if component, ok := builder.components[key]; ok {
		return component
	}
	component := &Component{Ref: strings.TrimSpace(ref)}
	builder.components[key] = component
	return component
}

func (builder *operationRequestBuilder) hydrateFootprint(index int, component *Component) {
	if component.FootprintID != "" && builder.opts.LibraryIndex != nil {
		if record, ok := libraryresolver.ResolveFootprint(*builder.opts.LibraryIndex, component.FootprintID); ok {
			bounds, pads, issues := BoundsFromFootprint(record)
			component.FootprintID = strings.TrimSpace(record.FootprintID)
			if bounds.WidthMM > 0 && bounds.HeightMM > 0 {
				component.Bounds = bounds
			}
			if len(pads) > 0 {
				component.Pads = pads
			}
			builder.issues = append(builder.issues, issues...)
			return
		}
	}
	if component.Bounds.WidthMM > 0 && component.Bounds.HeightMM > 0 {
		return
	}
	if builder.opts.DefaultBounds.WidthMM > 0 && builder.opts.DefaultBounds.HeightMM > 0 {
		component.Bounds = builder.opts.DefaultBounds
		component.Bounds.Source = BoundsEstimated
		builder.issues = append(builder.issues, reports.Issue{
			Code:     reports.CodeValidationFailed,
			Severity: reports.SeverityWarning,
			Path:     fmt.Sprintf("operations[%d].footprint_id", index),
			Message:  "using estimated placement bounds for " + component.Ref,
		})
	}
}

func (builder *operationRequestBuilder) addNetEndpoint(netName string, endpoint transactions.Endpoint) {
	netName = strings.TrimSpace(netName)
	if netName == "" {
		return
	}
	if _, ok := builder.netPins[netName]; !ok {
		builder.netPins[netName] = map[string]Endpoint{}
	}
	normalized := strings.ToUpper(strings.TrimSpace(endpoint.Ref)) + "." + strings.ToUpper(strings.TrimSpace(endpoint.Pin))
	builder.netPins[netName][normalized] = Endpoint{Ref: strings.TrimSpace(endpoint.Ref), Pin: strings.TrimSpace(endpoint.Pin)}
}

func (builder *operationRequestBuilder) sortedComponents() []Component {
	components := make([]Component, 0, len(builder.components))
	for _, component := range builder.components {
		components = append(components, *component)
	}
	components = slicesForPlacement(components)
	return components
}

func (builder *operationRequestBuilder) sortedNets() []Net {
	names := make([]string, 0, len(builder.netPins))
	for name := range builder.netPins {
		names = append(names, name)
	}
	sort.Strings(names)
	nets := make([]Net, 0, len(names))
	for _, name := range names {
		endpoints := make([]Endpoint, 0, len(builder.netPins[name]))
		for _, endpoint := range builder.netPins[name] {
			if _, ok := builder.components[strings.ToUpper(strings.TrimSpace(endpoint.Ref))]; !ok {
				continue
			}
			endpoints = append(endpoints, endpoint)
		}
		if len(endpoints) == 0 {
			continue
		}
		sortEndpoints(endpoints)
		nets = append(nets, Net{Name: name, Endpoints: endpoints, Role: NetUnknown})
	}
	return nets
}

func padSummaries(pads []transactions.PadSpec) []PadSummary {
	summaries := make([]PadSummary, 0, len(pads))
	for _, pad := range pads {
		net := ""
		if pad.Net != nil {
			net = *pad.Net
		}
		summaries = append(summaries, PadSummary{
			Name:        strings.TrimSpace(pad.Name),
			Net:         strings.TrimSpace(net),
			XMM:         pad.XMM,
			YMM:         pad.YMM,
			RotationDeg: pad.RotationDeg,
			WidthMM:     pad.WidthMM,
			HeightMM:    pad.HeightMM,
		})
	}
	return summaries
}

func fixedRotation(rotation float64) RotationConstraint {
	value := rotation
	return RotationConstraint{FixedDeg: &value}
}

func sideFromLayer(layer string) SideConstraint {
	if strings.EqualFold(strings.TrimSpace(layer), "B.Cu") {
		return SideBottom
	}
	return SideTop
}

func netRoleFromName(name string) NetRole {
	normalized := strings.ToLower(strings.TrimSpace(name))
	for _, token := range netNameRoleTokenPattern.FindAllString(normalized, -1) {
		switch token {
		case "gnd", "ground":
			return NetGround
		case "vcc", "vdd", "vbus", "vin", "vout":
			return NetPower
		case "scl", "clk", "clock":
			return NetClock
		}
	}
	return NetSignal
}

func sortEndpoints(endpoints []Endpoint) {
	sort.SliceStable(endpoints, func(i int, j int) bool {
		leftRef := strings.ToUpper(endpoints[i].Ref)
		rightRef := strings.ToUpper(endpoints[j].Ref)
		if leftRef != rightRef {
			return leftRef < rightRef
		}
		return strings.ToUpper(endpoints[i].Pin) < strings.ToUpper(endpoints[j].Pin)
	})
}

func combineIssues(first []reports.Issue, second []reports.Issue) []reports.Issue {
	combined := make([]reports.Issue, 0, len(first)+len(second))
	combined = append(combined, first...)
	combined = append(combined, second...)
	return combined
}

func boundsFromPadSpecs(pads []transactions.PadSpec) Bounds {
	first := true
	var minX, minY, maxX, maxY float64
	for _, pad := range pads {
		if pad.WidthMM <= 0 || pad.HeightMM <= 0 {
			continue
		}
		left := pad.XMM - pad.WidthMM/2
		right := pad.XMM + pad.WidthMM/2
		top := pad.YMM - pad.HeightMM/2
		bottom := pad.YMM + pad.HeightMM/2
		if first {
			minX, maxX = left, right
			minY, maxY = top, bottom
			first = false
			continue
		}
		minX = min(minX, left)
		maxX = max(maxX, right)
		minY = min(minY, top)
		maxY = max(maxY, bottom)
	}
	if first || maxX <= minX || maxY <= minY {
		return Bounds{}
	}
	return Bounds{
		WidthMM:      maxX - minX,
		HeightMM:     maxY - minY,
		AnchorOffset: Point{XMM: -minX, YMM: -minY},
		Source:       BoundsGeneratedPads,
	}
}

func adapterIssue(index int, message string) reports.Issue {
	return reports.Issue{
		Code:     reports.CodeValidationFailed,
		Severity: reports.SeverityError,
		Path:     fmt.Sprintf("operations[%d]", index),
		Message:  message,
	}
}
