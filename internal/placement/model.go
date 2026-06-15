package placement

import (
	"fmt"
	"math"
	"slices"
	"strings"

	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

const maxCandidatesPerPartLimit = 100000

type Status string

const (
	StatusPlaced  Status = "placed"
	StatusPartial Status = "partial"
	StatusBlocked Status = "blocked"
)

type BoundsSource string

const (
	BoundsLibraryCourtyard BoundsSource = "library_courtyard"
	BoundsLibraryPads      BoundsSource = "library_pads"
	BoundsGeneratedPads    BoundsSource = "generated_pads"
	BoundsExplicit         BoundsSource = "explicit"
	BoundsEstimated        BoundsSource = "estimated"
)

type SideConstraint string

const (
	SideAny    SideConstraint = "any"
	SideTop    SideConstraint = "top"
	SideBottom SideConstraint = "bottom"
)

type EdgeConstraint string

const (
	EdgeNone   EdgeConstraint = ""
	EdgeAny    EdgeConstraint = "any"
	EdgeLeft   EdgeConstraint = "left"
	EdgeRight  EdgeConstraint = "right"
	EdgeTop    EdgeConstraint = "top"
	EdgeBottom EdgeConstraint = "bottom"
)

type NetRole string

const (
	NetPower        NetRole = "power"
	NetGround       NetRole = "ground"
	NetSignal       NetRole = "signal"
	NetClock        NetRole = "clock"
	NetAnalog       NetRole = "analog"
	NetDifferential NetRole = "differential"
	NetUnknown      NetRole = "unknown"
)

type Request struct {
	Board      BoardPlacementArea
	Components []Component
	Nets       []Net
	Groups     []Group
	Keepouts   []Keepout
	Rules      Rules
	Existing   ExistingPlacementPolicy
	Seed       string
}

type BoardPlacementArea struct {
	WidthMM  float64
	HeightMM float64
	Origin   Point
	MarginMM float64
}

type Component struct {
	Ref         string
	Value       string
	FootprintID string
	Role        string
	Bounds      Bounds
	Pads        []PadSummary
	Fixed       bool
	Position    *Placement
	Side        SideConstraint
	Rotation    RotationConstraint
	Edge        EdgeConstraint
	GroupID     string
	Priority    int
	Hints       []Hint
}

type Bounds struct {
	WidthMM      float64
	HeightMM     float64
	CourtyardMM  float64
	AnchorOffset Point
	Source       BoundsSource
}

type PadSummary struct {
	Name     string
	Net      string
	XMM      float64
	YMM      float64
	WidthMM  float64
	HeightMM float64
}

type Net struct {
	Name       string
	Endpoints  []Endpoint
	Role       NetRole
	Weight     int
	WidthClass string
}

type Endpoint struct {
	Ref string
	Pin string
}

type Group struct {
	ID           string
	Role         string
	Components   []string
	Anchor       GroupAnchor
	KeepTogether bool
	MaxSpreadMM  float64
	Priority     int
}

type GroupAnchor struct {
	Ref string
	At  *Point
}

type Keepout struct {
	ID     string
	Bounds Rect
	Layers []string
	Reason string
}

type Rules struct {
	GridMM                   float64
	ComponentSpacingMM       float64
	BoardEdgeClearanceMM     float64
	GroupSpacingMM           float64
	PreferTopLayer           bool
	AllowBackLayer           bool
	ConnectorEdgeClearanceMM float64
	MaxCandidatesPerPart     int
}

type ExistingPlacementPolicy struct {
	PreserveFixed bool
}

type RotationConstraint struct {
	FixedDeg   *float64
	AllowedDeg []float64
}

type Hint struct {
	Kind  string
	Value string
}

type Point struct {
	XMM float64 `json:"x_mm"`
	YMM float64 `json:"y_mm"`
}

type Rect struct {
	Min Point `json:"min"`
	Max Point `json:"max"`
}

type Placement struct {
	XMM         float64
	YMM         float64
	RotationDeg float64
	Layer       string
}

type Result struct {
	Status     Status                   `json:"status"`
	Placements []PlacementResult        `json:"placements"`
	Issues     []reports.Issue          `json:"issues"`
	Metrics    Metrics                  `json:"metrics"`
	Operations []transactions.Operation `json:"operations,omitempty"`
	Quality    *QualityReport           `json:"quality,omitempty"`
}

type PlacementResult struct {
	Ref         string
	FootprintID string
	Position    Placement
	Bounds      Rect
	Fixed       bool
	GroupID     string
	Reason      string
}

type Metrics struct {
	ComponentCount       int
	PlacedCount          int
	FixedCount           int
	UnplacedCount        int
	CollisionCount       int
	OutsideOutlineCount  int
	EstimatedBoundsCount int
	HPWLMM               float64
}

func DefaultRules() Rules {
	return Rules{
		GridMM:                   0.5,
		ComponentSpacingMM:       0.5,
		BoardEdgeClearanceMM:     1.0,
		GroupSpacingMM:           1.0,
		PreferTopLayer:           true,
		AllowBackLayer:           false,
		ConnectorEdgeClearanceMM: 1.0,
		MaxCandidatesPerPart:     5000,
	}
}

func NormalizeRequest(request Request) Request {
	request.Components = slices.Clone(request.Components)
	request.Nets = slices.Clone(request.Nets)
	request.Groups = slices.Clone(request.Groups)
	request.Keepouts = slices.Clone(request.Keepouts)
	request.Rules = normalizeRules(request.Rules)
	for i := range request.Components {
		request.Components[i].Pads = slices.Clone(request.Components[i].Pads)
		request.Components[i].Hints = slices.Clone(request.Components[i].Hints)
		request.Components[i].Rotation.AllowedDeg = slices.Clone(request.Components[i].Rotation.AllowedDeg)
		for j := range request.Components[i].Pads {
			request.Components[i].Pads[j].Name = strings.TrimSpace(request.Components[i].Pads[j].Name)
			request.Components[i].Pads[j].Net = strings.TrimSpace(request.Components[i].Pads[j].Net)
		}
		if request.Components[i].Rotation.FixedDeg != nil {
			rotation := *request.Components[i].Rotation.FixedDeg
			request.Components[i].Rotation.FixedDeg = &rotation
		}
		if request.Components[i].Position != nil {
			position := *request.Components[i].Position
			request.Components[i].Position = &position
		}
		if request.Existing.PreserveFixed && request.Components[i].Position != nil {
			request.Components[i].Fixed = true
		}
		request.Components[i].Ref = strings.TrimSpace(request.Components[i].Ref)
		request.Components[i].FootprintID = strings.TrimSpace(request.Components[i].FootprintID)
		request.Components[i].GroupID = strings.TrimSpace(request.Components[i].GroupID)
		if request.Components[i].Side == "" {
			request.Components[i].Side = SideTop
		}
		if request.Components[i].Side == SideAny && !request.Rules.AllowBackLayer {
			request.Components[i].Side = SideTop
		}
		if len(request.Components[i].Rotation.AllowedDeg) == 0 && request.Components[i].Rotation.FixedDeg == nil {
			request.Components[i].Rotation.AllowedDeg = []float64{0, 90, 180, 270}
		}
	}
	for i := range request.Nets {
		request.Nets[i].Endpoints = slices.Clone(request.Nets[i].Endpoints)
		request.Nets[i].Name = strings.TrimSpace(request.Nets[i].Name)
		for j := range request.Nets[i].Endpoints {
			request.Nets[i].Endpoints[j].Ref = strings.TrimSpace(request.Nets[i].Endpoints[j].Ref)
			request.Nets[i].Endpoints[j].Pin = strings.TrimSpace(request.Nets[i].Endpoints[j].Pin)
		}
		if request.Nets[i].Role == "" {
			request.Nets[i].Role = NetUnknown
		}
	}
	for i := range request.Groups {
		request.Groups[i].Components = slices.Clone(request.Groups[i].Components)
		if request.Groups[i].Anchor.At != nil {
			at := *request.Groups[i].Anchor.At
			request.Groups[i].Anchor.At = &at
		}
		request.Groups[i].ID = strings.TrimSpace(request.Groups[i].ID)
		request.Groups[i].Anchor.Ref = strings.TrimSpace(request.Groups[i].Anchor.Ref)
		for j := range request.Groups[i].Components {
			request.Groups[i].Components[j] = strings.TrimSpace(request.Groups[i].Components[j])
		}
		slices.Sort(request.Groups[i].Components)
	}
	for i := range request.Keepouts {
		request.Keepouts[i].Layers = slices.Clone(request.Keepouts[i].Layers)
		request.Keepouts[i].ID = strings.TrimSpace(request.Keepouts[i].ID)
	}
	return request
}

func normalizeRules(rules Rules) Rules {
	defaults := DefaultRules()
	if rules.GridMM <= 0 {
		rules.GridMM = defaults.GridMM
	}
	if rules.ComponentSpacingMM <= 0 {
		rules.ComponentSpacingMM = defaults.ComponentSpacingMM
	}
	if rules.BoardEdgeClearanceMM <= 0 {
		rules.BoardEdgeClearanceMM = defaults.BoardEdgeClearanceMM
	}
	if rules.GroupSpacingMM <= 0 {
		rules.GroupSpacingMM = defaults.GroupSpacingMM
	}
	if rules.ConnectorEdgeClearanceMM <= 0 {
		rules.ConnectorEdgeClearanceMM = defaults.ConnectorEdgeClearanceMM
	}
	if rules.MaxCandidatesPerPart <= 0 {
		rules.MaxCandidatesPerPart = defaults.MaxCandidatesPerPart
	} else if rules.MaxCandidatesPerPart > maxCandidatesPerPartLimit {
		rules.MaxCandidatesPerPart = maxCandidatesPerPartLimit
	}
	return rules
}

func Validate(request Request) []reports.Issue {
	request.Rules = normalizeRules(request.Rules)
	var issues []reports.Issue
	if request.Board.WidthMM <= 0 {
		issues = append(issues, issue("board.width_mm", "board width must be positive"))
	}
	if request.Board.HeightMM <= 0 {
		issues = append(issues, issue("board.height_mm", "board height must be positive"))
	}
	if request.Board.MarginMM < 0 {
		issues = append(issues, issue("board.margin_mm", "board margin must be non-negative"))
	}
	if request.Board.WidthMM > 0 && request.Board.HeightMM > 0 && request.Board.MarginMM*2 >= min(request.Board.WidthMM, request.Board.HeightMM) {
		issues = append(issues, issue("board.margin_mm", "board margin leaves no usable placement area"))
	}
	refs := map[string]Component{}
	for i, component := range request.Components {
		path := fmt.Sprintf("components[%d]", i)
		ref := strings.TrimSpace(component.Ref)
		if ref == "" {
			issues = append(issues, issue(path+".ref", "component reference required"))
			continue
		}
		key := strings.ToUpper(ref)
		if existing, ok := refs[key]; ok {
			issues = append(issues, issue(path+".ref", "duplicate component reference "+ref+" collides with "+strings.TrimSpace(existing.Ref)))
		}
		refs[key] = component
		hasFootprint := strings.TrimSpace(component.FootprintID) != ""
		hasBounds := component.Bounds.WidthMM > 0 && component.Bounds.HeightMM > 0
		hasPartialBounds := component.Bounds.WidthMM > 0 || component.Bounds.HeightMM > 0
		if !hasFootprint && !hasBounds {
			issues = append(issues, issue(path+".footprint_id", "footprint id or explicit bounds required"))
		}
		if hasPartialBounds && !hasBounds {
			issues = append(issues, issue(path+".bounds", "component bounds must be positive when provided"))
		}
		if component.Fixed && component.Position == nil {
			issues = append(issues, issue(path+".position", "fixed component requires position"))
		}
		if component.Side == SideBottom && !request.Rules.AllowBackLayer {
			issues = append(issues, issue(path+".side", "bottom placement requires AllowBackLayer"))
		}
		if !validSide(component.Side) {
			issues = append(issues, issue(path+".side", "invalid side constraint "+string(component.Side)))
		}
		if !validEdge(component.Edge) {
			issues = append(issues, issue(path+".edge", "invalid edge constraint "+string(component.Edge)))
		}
		if !validBoundsSource(component.Bounds.Source) {
			issues = append(issues, issue(path+".bounds.source", "invalid bounds source "+string(component.Bounds.Source)))
		}
		if err := validateRotation(component.Rotation); err != nil {
			issues = append(issues, issue(path+".rotation", err.Error()))
		}
	}
	groupIDs := map[string]int{}
	for i, group := range request.Groups {
		path := fmt.Sprintf("groups[%d]", i)
		id := strings.TrimSpace(group.ID)
		if id == "" {
			issues = append(issues, issue(path+".id", "group id required"))
		} else {
			key := strings.ToUpper(id)
			if previous, ok := groupIDs[key]; ok {
				issues = append(issues, issue(path+".id", fmt.Sprintf("duplicate group ID %s already defined at index %d", id, previous)))
			}
			groupIDs[key] = i
		}
		for _, ref := range group.Components {
			trimmedRef := strings.TrimSpace(ref)
			component, ok := refs[strings.ToUpper(trimmedRef)]
			if !ok {
				issues = append(issues, issue(path+".components", "group references unknown component "+trimmedRef))
				continue
			}
			componentGroup := strings.TrimSpace(component.GroupID)
			if componentGroup != "" && !strings.EqualFold(componentGroup, id) {
				issues = append(issues, issue(path+".components", fmt.Sprintf("component %s has group ID %s but is listed in group %s", trimmedRef, componentGroup, id)))
			}
		}
		if group.Anchor.Ref != "" {
			trimmedRef := strings.TrimSpace(group.Anchor.Ref)
			if _, ok := refs[strings.ToUpper(trimmedRef)]; !ok {
				issues = append(issues, issue(path+".anchor.ref", "group anchor references unknown component "+trimmedRef))
			}
		}
	}
	netNames := map[string]int{}
	for i, net := range request.Nets {
		path := fmt.Sprintf("nets[%d]", i)
		name := strings.TrimSpace(net.Name)
		if name == "" {
			issues = append(issues, issue(path+".name", "net name required"))
		} else {
			key := strings.ToUpper(name)
			if previous, ok := netNames[key]; ok {
				issues = append(issues, issue(path+".name", fmt.Sprintf("duplicate net name %s already defined at index %d", name, previous)))
			}
			netNames[key] = i
		}
		if !validNetRole(net.Role) {
			issues = append(issues, issue(path+".role", "invalid net role "+string(net.Role)))
		}
		for endpointIndex, endpoint := range net.Endpoints {
			endpointRef := strings.TrimSpace(endpoint.Ref)
			component, ok := refs[strings.ToUpper(endpointRef)]
			if !ok {
				issues = append(issues, issue(path+".endpoints", "net endpoint references unknown component "+endpointRef))
			}
			pin := strings.TrimSpace(endpoint.Pin)
			if pin == "" {
				issues = append(issues, issue(path+".endpoints", "net endpoint pin required"))
			} else if ok && len(component.Pads) > 0 && !componentHasPad(component, pin) {
				issues = append(issues, issue(fmt.Sprintf("%s.endpoints[%d].pin", path, endpointIndex), "pin "+pin+" not found in component "+endpointRef))
			}
		}
	}
	for i, keepout := range request.Keepouts {
		path := fmt.Sprintf("keepouts[%d].bounds", i)
		if keepout.Bounds.Min.XMM > keepout.Bounds.Max.XMM || keepout.Bounds.Min.YMM > keepout.Bounds.Max.YMM {
			issues = append(issues, issue(path, "keepout bounds min must not exceed max"))
		}
	}
	return issues
}

func validSide(side SideConstraint) bool {
	switch side {
	case "", SideAny, SideTop, SideBottom:
		return true
	default:
		return false
	}
}

func validEdge(edge EdgeConstraint) bool {
	switch edge {
	case EdgeNone, EdgeAny, EdgeLeft, EdgeRight, EdgeTop, EdgeBottom:
		return true
	default:
		return false
	}
}

func validNetRole(role NetRole) bool {
	switch role {
	case "", NetPower, NetGround, NetSignal, NetClock, NetAnalog, NetDifferential, NetUnknown:
		return true
	default:
		return false
	}
}

func validBoundsSource(source BoundsSource) bool {
	switch source {
	case "", BoundsLibraryCourtyard, BoundsLibraryPads, BoundsGeneratedPads, BoundsExplicit, BoundsEstimated:
		return true
	default:
		return false
	}
}

func componentHasPad(component Component, pin string) bool {
	for _, pad := range component.Pads {
		if strings.EqualFold(strings.TrimSpace(pad.Name), pin) {
			return true
		}
	}
	return false
}

func validateRotation(rotation RotationConstraint) error {
	if rotation.FixedDeg != nil {
		if !validRotation(*rotation.FixedDeg) {
			return fmt.Errorf("fixed rotation must be one of 0, 90, 180, 270")
		}
	}
	for _, value := range rotation.AllowedDeg {
		if !validRotation(value) {
			return fmt.Errorf("allowed rotation must be one of 0, 90, 180, 270")
		}
	}
	return nil
}

func validRotation(value float64) bool {
	normalized := math.Mod(value, 360)
	if normalized < 0 {
		normalized += 360
	}
	const epsilon = 1e-9
	return math.Abs(normalized-0) < epsilon ||
		math.Abs(normalized-90) < epsilon ||
		math.Abs(normalized-180) < epsilon ||
		math.Abs(normalized-270) < epsilon ||
		math.Abs(normalized-360) < epsilon
}

func issue(path string, message string) reports.Issue {
	return reports.Issue{
		Code:     reports.CodeValidationFailed,
		Severity: reports.SeverityError,
		Path:     path,
		Message:  message,
	}
}
