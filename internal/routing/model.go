package routing

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"kicadai/internal/reports"
)

type Status string

const (
	StatusRouted  Status = "routed"
	StatusPartial Status = "partial"
	StatusBlocked Status = "blocked"
)

type RouteMode string

const (
	ModeSingleLayer  RouteMode = "single_layer"
	ModeTwoLayer     RouteMode = "two_layer"
	ModeValidateOnly RouteMode = "validate_only"
)

type LayerKind string

const (
	LayerCopper LayerKind = "copper"
	LayerOther  LayerKind = "other"
)

type PadShape string

const (
	PadCircle      PadShape = "circle"
	PadOval        PadShape = "oval"
	PadRect        PadShape = "rect"
	PadRoundedRect PadShape = "roundrect"
)

type PadType string

const (
	PadSMD         PadType = "smd"
	PadThroughHole PadType = "through_hole"
)

type NetRole string

const (
	NetPower   NetRole = "power"
	NetGround  NetRole = "ground"
	NetSignal  NetRole = "signal"
	NetUnknown NetRole = "unknown"
)

type CopperKind string

const (
	CopperSegment CopperKind = "segment"
	CopperVia     CopperKind = "via"
	CopperZone    CopperKind = "zone"
)

type ObstacleKind string

const (
	ObstacleBoardEdge      ObstacleKind = "board_edge"
	ObstacleKeepout        ObstacleKind = "keepout"
	ObstacleOtherNetPad    ObstacleKind = "other_net_pad"
	ObstacleSameNetPad     ObstacleKind = "same_net_pad"
	ObstacleExistingCopper ObstacleKind = "existing_copper"
	ObstacleViaKeepout     ObstacleKind = "via_keepout"
	ObstacleMechanical     ObstacleKind = "mechanical"
	ObstacleZone           ObstacleKind = "zone"
)

type ZoneRoutingPolicy string

const (
	ZoneIgnore      ZoneRoutingPolicy = "ignore"
	ZoneObstacle    ZoneRoutingPolicy = "obstacle"
	ZoneUnsupported ZoneRoutingPolicy = "unsupported"
)

type Request struct {
	Board      Board            `json:"board"`
	Components []Component      `json:"components,omitempty"`
	Nets       []Net            `json:"nets,omitempty"`
	Obstacles  []Obstacle       `json:"obstacles,omitempty"`
	Existing   []ExistingCopper `json:"existing,omitempty"`
	Rules      Rules            `json:"rules"`
	Strategy   Strategy         `json:"strategy"`
	Seed       string           `json:"seed,omitempty"`
}

type Board struct {
	Origin   Point   `json:"origin"`
	WidthMM  float64 `json:"width_mm"`
	HeightMM float64 `json:"height_mm"`
	Outline  []Shape `json:"outline,omitempty"`
	Layers   []Layer `json:"layers,omitempty"`
	MarginMM float64 `json:"margin_mm,omitempty"`
}

type Layer struct {
	Name     string    `json:"name"`
	Kind     LayerKind `json:"kind"`
	Routable bool      `json:"routable"`
}

type Component struct {
	Ref       string    `json:"ref"`
	Footprint string    `json:"footprint,omitempty"`
	Position  Placement `json:"position"`
	Pads      []Pad     `json:"pads,omitempty"`
	Fixed     bool      `json:"fixed,omitempty"`
}

type Placement struct {
	XMM         float64 `json:"x_mm"`
	YMM         float64 `json:"y_mm"`
	RotationDeg float64 `json:"rotation_deg,omitempty"`
	Layer       string  `json:"layer,omitempty"`
}

type Pad struct {
	Ref       string   `json:"ref,omitempty"`
	Name      string   `json:"name"`
	Net       string   `json:"net,omitempty"`
	Position  Point    `json:"position"`
	Shape     PadShape `json:"shape"`
	Type      PadType  `json:"type"`
	Size      Size     `json:"size"`
	Drill     *Drill   `json:"drill,omitempty"`
	Layers    []string `json:"layers,omitempty"`
	Clearance *float64 `json:"clearance_mm,omitempty"`
}

type Drill struct {
	DiameterMM float64 `json:"diameter_mm"`
}

type Net struct {
	Name      string     `json:"name"`
	Endpoints []Endpoint `json:"endpoints,omitempty"`
	Role      NetRole    `json:"role,omitempty"`
	Class     string     `json:"class,omitempty"`
	Priority  int        `json:"priority,omitempty"`
	Fixed     bool       `json:"fixed,omitempty"`
}

type Endpoint struct {
	Ref string `json:"ref"`
	Pin string `json:"pin"`
}

type ExistingCopper struct {
	Kind     CopperKind `json:"kind"`
	Net      string     `json:"net,omitempty"`
	Layer    string     `json:"layer,omitempty"`
	Geometry Shape      `json:"geometry"`
	Fixed    bool       `json:"fixed,omitempty"`
}

type Obstacle struct {
	Kind      ObstacleKind `json:"kind"`
	Layer     string       `json:"layer,omitempty"`
	Geometry  Shape        `json:"geometry"`
	Clearance float64      `json:"clearance_mm,omitempty"`
	Source    string       `json:"source,omitempty"`
}

type Rules struct {
	GridMM          float64             `json:"grid_mm,omitempty"`
	TraceWidthMM    float64             `json:"trace_width_mm,omitempty"`
	ClearanceMM     float64             `json:"clearance_mm,omitempty"`
	ViaDiameterMM   float64             `json:"via_diameter_mm,omitempty"`
	ViaDrillMM      float64             `json:"via_drill_mm,omitempty"`
	ViaClearanceMM  float64             `json:"via_clearance_mm,omitempty"`
	EdgeClearanceMM float64             `json:"edge_clearance_mm,omitempty"`
	MaxSearchNodes  int                 `json:"max_search_nodes,omitempty"`
	MaxViasPerNet   int                 `json:"max_vias_per_net,omitempty"`
	AllowVias       *bool               `json:"allow_vias,omitempty"`
	AllowBackLayer  *bool               `json:"allow_back_layer,omitempty"`
	PreferLayer     string              `json:"prefer_layer,omitempty"`
	NetClasses      map[string]NetClass `json:"net_classes,omitempty"`
}

type NetClass struct {
	TraceWidthMM  float64 `json:"trace_width_mm,omitempty"`
	ClearanceMM   float64 `json:"clearance_mm,omitempty"`
	ViaDiameterMM float64 `json:"via_diameter_mm,omitempty"`
	ViaDrillMM    float64 `json:"via_drill_mm,omitempty"`
}

type Strategy struct {
	Mode             RouteMode         `json:"mode,omitempty"`
	NetOrder         string            `json:"net_order,omitempty"`
	RipupRetryLimit  int               `json:"ripup_retry_limit,omitempty"`
	AllowPartial     bool              `json:"allow_partial,omitempty"`
	PreserveExisting bool              `json:"preserve_existing,omitempty"`
	TreatZonesAs     ZoneRoutingPolicy `json:"treat_zones_as,omitempty"`
}

type Result struct {
	Status     Status          `json:"status"`
	Routes     []Route         `json:"routes,omitempty"`
	Operations []Operation     `json:"operations,omitempty"`
	Issues     []reports.Issue `json:"issues,omitempty"`
	Metrics    Metrics         `json:"metrics"`
}

type Operation struct {
	Op    string          `json:"op"`
	Raw   json.RawMessage `json:"-"`
	Index int             `json:"-"`
}

func (operation *Operation) UnmarshalJSON(data []byte) error {
	var head struct {
		Op string `json:"op"`
	}
	if err := json.Unmarshal(data, &head); err != nil {
		return err
	}
	operation.Op = head.Op
	operation.Raw = append([]byte(nil), data...)
	return nil
}

func (operation Operation) MarshalJSON() ([]byte, error) {
	if len(operation.Raw) > 0 {
		return operation.Raw, nil
	}
	type alias Operation
	return json.Marshal(alias(operation))
}

type OperationPoint struct {
	XMM float64 `json:"x_mm"`
	YMM float64 `json:"y_mm"`
}

type RouteOperation struct {
	Op      string              `json:"op"`
	NetName string              `json:"net_name"`
	Layer   string              `json:"layer,omitempty"`
	WidthMM float64             `json:"width_mm,omitempty"`
	Points  []OperationPoint    `json:"points"`
	Vias    []RouteViaOperation `json:"vias,omitempty"`
}

type RouteViaOperation struct {
	At         OperationPoint `json:"at"`
	DiameterMM float64        `json:"diameter_mm"`
	DrillMM    float64        `json:"drill_mm"`
	Layers     []string       `json:"layers,omitempty"`
}

type Route struct {
	Net      string          `json:"net"`
	Segments []Segment       `json:"segments,omitempty"`
	Vias     []Via           `json:"vias,omitempty"`
	Status   RouteStatus     `json:"status"`
	Issues   []reports.Issue `json:"issues,omitempty"`
}

type RouteStatus string

const (
	RouteStatusRouted  RouteStatus = "routed"
	RouteStatusSkipped RouteStatus = "skipped"
	RouteStatusFailed  RouteStatus = "failed"
)

type Segment struct {
	Net     string  `json:"net"`
	Layer   string  `json:"layer"`
	Start   Point   `json:"start"`
	End     Point   `json:"end"`
	WidthMM float64 `json:"width_mm"`
}

type Via struct {
	Net        string   `json:"net"`
	At         Point    `json:"at"`
	DiameterMM float64  `json:"diameter_mm"`
	DrillMM    float64  `json:"drill_mm"`
	Layers     []string `json:"layers,omitempty"`
}

type Metrics struct {
	NetCount          int     `json:"net_count"`
	RoutedNetCount    int     `json:"routed_net_count"`
	FailedNetCount    int     `json:"failed_net_count"`
	SegmentCount      int     `json:"segment_count"`
	ViaCount          int     `json:"via_count"`
	TotalLengthMM     float64 `json:"total_length_mm"`
	SearchNodes       int     `json:"search_nodes"`
	MaxSearchNodesHit bool    `json:"max_search_nodes_hit"`
}

func DefaultRules() Rules {
	enabled := true
	return Rules{
		GridMM:          0.25,
		TraceWidthMM:    0.25,
		ClearanceMM:     0.20,
		ViaDiameterMM:   0.60,
		ViaDrillMM:      0.30,
		ViaClearanceMM:  0.20,
		EdgeClearanceMM: 0.25,
		MaxSearchNodes:  250000,
		MaxViasPerNet:   4,
		AllowVias:       &enabled,
		AllowBackLayer:  &enabled,
		PreferLayer:     "F.Cu",
	}
}

func NormalizeRequest(request *Request) {
	if request == nil {
		return
	}
	defaults := DefaultRules()
	if request.Rules.GridMM == 0 {
		request.Rules.GridMM = defaults.GridMM
	}
	if request.Rules.TraceWidthMM == 0 {
		request.Rules.TraceWidthMM = defaults.TraceWidthMM
	}
	if request.Rules.ClearanceMM == 0 {
		request.Rules.ClearanceMM = defaults.ClearanceMM
	}
	if request.Rules.ViaDiameterMM == 0 {
		request.Rules.ViaDiameterMM = defaults.ViaDiameterMM
	}
	if request.Rules.ViaDrillMM == 0 {
		request.Rules.ViaDrillMM = defaults.ViaDrillMM
	}
	if request.Rules.ViaClearanceMM == 0 {
		request.Rules.ViaClearanceMM = defaults.ViaClearanceMM
	}
	if request.Rules.EdgeClearanceMM == 0 {
		request.Rules.EdgeClearanceMM = defaults.EdgeClearanceMM
	}
	if request.Rules.MaxSearchNodes == 0 {
		request.Rules.MaxSearchNodes = defaults.MaxSearchNodes
	}
	if request.Rules.MaxViasPerNet == 0 {
		request.Rules.MaxViasPerNet = defaults.MaxViasPerNet
	}
	if request.Rules.AllowVias == nil {
		request.Rules.AllowVias = boolPtr(true)
	}
	if request.Rules.AllowBackLayer == nil {
		request.Rules.AllowBackLayer = boolPtr(true)
	}
	if request.Rules.PreferLayer == "" {
		request.Rules.PreferLayer = defaults.PreferLayer
	}
	if request.Strategy.Mode == "" {
		request.Strategy.Mode = ModeTwoLayer
	}
	if request.Strategy.TreatZonesAs == "" {
		request.Strategy.TreatZonesAs = ZoneObstacle
	}
	if len(request.Board.Layers) == 0 {
		request.Board.Layers = []Layer{
			{Name: "F.Cu", Kind: LayerCopper, Routable: true},
			{Name: "B.Cu", Kind: LayerCopper, Routable: true},
		}
	}
	if len(request.Rules.NetClasses) > 0 {
		normalizedClasses := make(map[string]NetClass, len(request.Rules.NetClasses))
		for name, netClass := range request.Rules.NetClasses {
			normalizedClasses[strings.TrimSpace(name)] = netClass
		}
		request.Rules.NetClasses = normalizedClasses
	}
	if request.Strategy.Mode == ModeSingleLayer {
		request.Rules.AllowVias = boolPtr(false)
		request.Rules.AllowBackLayer = boolPtr(false)
	}
}

func Validate(request *Request) []reports.Issue {
	if request == nil {
		return []reports.Issue{issue(reports.CodeInvalidArgument, reports.SeverityBlocked, "request", "request is required")}
	}
	issues := []reports.Issue{}
	validateFinitePositive := func(path string, value float64, message string) {
		if math.IsNaN(value) || math.IsInf(value, 0) || value <= 0 {
			issues = append(issues, issue(reports.CodeInvalidArgument, reports.SeverityBlocked, path, message))
		}
	}
	validateFiniteNonNegative := func(path string, value float64, message string) {
		if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
			issues = append(issues, issue(reports.CodeInvalidArgument, reports.SeverityBlocked, path, message))
		}
	}
	validatePoint := func(path string, point Point) {
		if math.IsNaN(point.XMM) || math.IsInf(point.XMM, 0) || math.IsNaN(point.YMM) || math.IsInf(point.YMM, 0) {
			issues = append(issues, issue(reports.CodeInvalidArgument, reports.SeverityBlocked, path, "point coordinates must be finite"))
		}
	}
	validateFinitePositive("board.width_mm", request.Board.WidthMM, "board width must be positive")
	validateFinitePositive("board.height_mm", request.Board.HeightMM, "board height must be positive")
	validateFiniteNonNegative("board.margin_mm", request.Board.MarginMM, "board margin cannot be negative")
	validatePoint("board.origin", request.Board.Origin)
	validateFinitePositive("rules.grid_mm", request.Rules.GridMM, "routing grid must be positive")
	validateFinitePositive("rules.trace_width_mm", request.Rules.TraceWidthMM, "trace width must be positive")
	validateFiniteNonNegative("rules.clearance_mm", request.Rules.ClearanceMM, "clearance cannot be negative")
	validateFinitePositive("rules.via_diameter_mm", request.Rules.ViaDiameterMM, "via diameter must be positive")
	validateFinitePositive("rules.via_drill_mm", request.Rules.ViaDrillMM, "via drill must be positive")
	if request.Rules.ViaDrillMM >= request.Rules.ViaDiameterMM {
		issues = append(issues, issue(reports.CodeInvalidArgument, reports.SeverityBlocked, "rules.via_drill_mm", "via drill must be smaller than via diameter"))
	}
	if request.Rules.MaxSearchNodes <= 0 {
		issues = append(issues, issue(reports.CodeInvalidArgument, reports.SeverityBlocked, "rules.max_search_nodes", "max search nodes must be positive"))
	}
	if !supportedMode(request.Strategy.Mode) {
		issues = append(issues, issue(reports.CodeUnsupportedOperation, reports.SeverityBlocked, "strategy.mode", fmt.Sprintf("unsupported routing mode %q", request.Strategy.Mode)))
	}
	for name, netClass := range request.Rules.NetClasses {
		prefix := fmt.Sprintf("rules.net_classes[%s]", name)
		if netClass.TraceWidthMM != 0 {
			validateFinitePositive(prefix+".trace_width_mm", netClass.TraceWidthMM, "net class trace width must be positive")
		}
		if netClass.ClearanceMM != 0 {
			validateFiniteNonNegative(prefix+".clearance_mm", netClass.ClearanceMM, "net class clearance cannot be negative")
		}
		if netClass.ViaDiameterMM != 0 {
			validateFinitePositive(prefix+".via_diameter_mm", netClass.ViaDiameterMM, "net class via diameter must be positive")
		}
		if netClass.ViaDrillMM != 0 {
			validateFinitePositive(prefix+".via_drill_mm", netClass.ViaDrillMM, "net class via drill must be positive")
		}
		if netClass.ViaDiameterMM > 0 && netClass.ViaDrillMM > 0 && netClass.ViaDrillMM >= netClass.ViaDiameterMM {
			issues = append(issues, issue(reports.CodeInvalidArgument, reports.SeverityBlocked, prefix+".via_drill_mm", "net class via drill must be smaller than via diameter"))
		}
	}
	knownLayers := map[string]Layer{}
	routableLayers := map[string]Layer{}
	for index, layer := range request.Board.Layers {
		name := strings.TrimSpace(layer.Name)
		if name == "" {
			issues = append(issues, issue(reports.CodeInvalidArgument, reports.SeverityBlocked, fmt.Sprintf("board.layers[%d].name", index), "layer name is required"))
			continue
		}
		if _, ok := knownLayers[name]; ok {
			issues = append(issues, issue(reports.CodeInvalidArgument, reports.SeverityBlocked, fmt.Sprintf("board.layers[%d].name", index), "duplicate layer name"))
		}
		knownLayers[name] = layer
		if layer.Routable {
			routableLayers[name] = layer
		}
	}
	if len(routableLayers) == 0 {
		issues = append(issues, issue(reports.CodeInvalidArgument, reports.SeverityBlocked, "board.layers", "at least one routable copper layer is required"))
	}
	refs := map[string]struct{}{}
	pads := map[endpointID]Pad{}
	for componentIndex, component := range request.Components {
		ref := normalizeKey(component.Ref)
		path := fmt.Sprintf("components[%d]", componentIndex)
		if ref == "" {
			issues = append(issues, issue(reports.CodeInvalidArgument, reports.SeverityBlocked, path+".ref", "component reference is required"))
			continue
		}
		if component.Position.Layer != "" {
			if _, ok := knownLayers[strings.TrimSpace(component.Position.Layer)]; !ok {
				issues = append(issues, issue(reports.CodeInvalidArgument, reports.SeverityBlocked, path+".position.layer", "component placement layer is not in board layer table"))
			}
		}
		validatePoint(path+".position", Point{XMM: component.Position.XMM, YMM: component.Position.YMM})
		if _, ok := refs[ref]; ok {
			issues = append(issues, issue(reports.CodeDuplicateReference, reports.SeverityBlocked, path+".ref", "duplicate component reference"))
		}
		refs[ref] = struct{}{}
		componentPadNames := map[string]struct{}{}
		for padIndex, pad := range component.Pads {
			pin := normalizeKey(pad.Name)
			padPath := fmt.Sprintf("%s.pads[%d]", path, padIndex)
			if pin == "" {
				issues = append(issues, issue(reports.CodeInvalidArgument, reports.SeverityBlocked, padPath+".name", "pad name is required"))
				continue
			}
			if _, ok := componentPadNames[pin]; ok {
				issues = append(issues, issue(reports.CodeInvalidArgument, reports.SeverityBlocked, padPath+".name", "duplicate pad name on component"))
			}
			componentPadNames[pin] = struct{}{}
			if len(pad.Layers) == 0 {
				issues = append(issues, issue(reports.CodeInvalidArgument, reports.SeverityBlocked, padPath+".layers", "pad must have at least one layer"))
			}
			validatePoint(padPath+".position", pad.Position)
			for layerIndex, layer := range pad.Layers {
				layer = strings.TrimSpace(layer)
				if _, ok := knownLayers[layer]; !ok && layer != "*.Cu" && layer != "*.Mask" {
					issues = append(issues, issue(reports.CodeInvalidArgument, reports.SeverityBlocked, fmt.Sprintf("%s.layers[%d]", padPath, layerIndex), "pad layer is not in board layer table"))
				}
			}
			if pad.Size.WidthMM <= 0 || pad.Size.HeightMM <= 0 || math.IsNaN(pad.Size.WidthMM) || math.IsNaN(pad.Size.HeightMM) || math.IsInf(pad.Size.WidthMM, 0) || math.IsInf(pad.Size.HeightMM, 0) {
				issues = append(issues, issue(reports.CodeInvalidArgument, reports.SeverityBlocked, padPath+".size", "pad size must be positive and finite"))
			}
			if pad.Type == PadThroughHole {
				if pad.Drill == nil {
					issues = append(issues, issue(reports.CodeInvalidArgument, reports.SeverityBlocked, padPath+".drill", "through-hole pad requires a drill"))
				} else if pad.Drill.DiameterMM <= 0 {
					issues = append(issues, issue(reports.CodeInvalidArgument, reports.SeverityBlocked, padPath+".drill.diameter_mm", "through-hole pad drill diameter must be positive"))
				} else if pad.Drill.DiameterMM >= min(pad.Size.WidthMM, pad.Size.HeightMM) {
					issues = append(issues, issue(reports.CodeInvalidArgument, reports.SeverityBlocked, padPath+".drill.diameter_mm", "through-hole pad drill must be smaller than pad size"))
				}
			}
			pads[endpointKey(ref, pin)] = pad
		}
	}
	validateLayeredShapes := func(prefix string, layer string, shape Shape) {
		if layer != "" {
			if _, ok := knownLayers[strings.TrimSpace(layer)]; !ok {
				issues = append(issues, issue(reports.CodeInvalidArgument, reports.SeverityBlocked, prefix+".layer", "layer is not in board layer table"))
			}
		}
		if shape.Rect == nil && len(shape.Polygon) == 0 {
			issues = append(issues, issue(reports.CodeInvalidArgument, reports.SeverityBlocked, prefix+".geometry", "shape rectangle or polygon is required"))
			return
		}
		if shape.Rect != nil {
			if shape.Rect.Min.XMM > shape.Rect.Max.XMM || shape.Rect.Min.YMM > shape.Rect.Max.YMM {
				issues = append(issues, issue(reports.CodeInvalidArgument, reports.SeverityBlocked, prefix+".geometry.rect", "rectangle min must be less than or equal to max"))
			}
			validatePoint(prefix+".geometry.rect.min", shape.Rect.Min)
			validatePoint(prefix+".geometry.rect.max", shape.Rect.Max)
		}
		for pointIndex, point := range shape.Polygon {
			validatePoint(fmt.Sprintf("%s.geometry.polygon[%d]", prefix, pointIndex), point)
		}
	}
	for index, obstacle := range request.Obstacles {
		validateLayeredShapes(fmt.Sprintf("obstacles[%d]", index), obstacle.Layer, obstacle.Geometry)
	}
	for index, copper := range request.Existing {
		validateLayeredShapes(fmt.Sprintf("existing[%d]", index), copper.Layer, copper.Geometry)
	}
	netNames := map[string]struct{}{}
	for netIndex, net := range request.Nets {
		netName := normalizeKey(net.Name)
		if netName == "" {
			issues = append(issues, issue(reports.CodeInvalidArgument, reports.SeverityBlocked, fmt.Sprintf("nets[%d].name", netIndex), "net name is required"))
		} else if _, ok := netNames[netName]; ok {
			issues = append(issues, issue(reports.CodeInvalidArgument, reports.SeverityBlocked, fmt.Sprintf("nets[%d].name", netIndex), "duplicate net name"))
		}
		netNames[netName] = struct{}{}
		netClass := strings.TrimSpace(net.Class)
		if netClass != "" {
			if _, ok := request.Rules.NetClasses[netClass]; !ok {
				issues = append(issues, issue(reports.CodeInvalidArgument, reports.SeverityBlocked, fmt.Sprintf("nets[%d].class", netIndex), "net references an unknown net class"))
			}
		}
		for endpointIndex, endpoint := range net.Endpoints {
			ref := normalizeKey(endpoint.Ref)
			pin := normalizeKey(endpoint.Pin)
			path := fmt.Sprintf("nets[%d].endpoints[%d]", netIndex, endpointIndex)
			if ref == "" || pin == "" {
				issues = append(issues, issue(reports.CodeInvalidArgument, reports.SeverityBlocked, path, "endpoint ref and pin are required"))
				continue
			}
			if _, ok := pads[endpointKey(ref, pin)]; !ok {
				issues = append(issues, issue(reports.CodeInvalidArgument, reports.SeverityBlocked, path, "endpoint references an unknown pad"))
			}
		}
	}
	return issues
}

func supportedMode(mode RouteMode) bool {
	return mode == ModeSingleLayer || mode == ModeTwoLayer || mode == ModeValidateOnly
}

func boolPtr(value bool) *bool {
	return &value
}

func issue(code reports.Code, severity reports.Severity, path string, message string) reports.Issue {
	return reports.Issue{Code: code, Severity: severity, Path: path, Message: message}
}

func normalizeKey(value string) string {
	return strings.ToUpper(strings.TrimSpace(value))
}

type endpointID struct {
	Ref string
	Pin string
}

func endpointKey(ref string, pin string) endpointID {
	return endpointID{Ref: normalizeKey(ref), Pin: normalizeKey(pin)}
}
