package schematicir

import (
	"fmt"
	"math"
	"regexp"
	"strings"

	"kicadai/internal/reports"
)

var electricalValuePattern = regexp.MustCompile(`^[+-]?([0-9]*\.[0-9]+|[0-9]+(\.[0-9]*)?)([eE][+-]?[0-9]+)?\s*(p|n|u|U|µ|μ|m|k|K|Meg|M|G|T)?\s*(Ohm|ohm|Ω|R|F|H|h|V|v|A|a|W|w|Hz|hz)?\s*$|^[+-]?([0-9]+(R|p|n|u|U|µ|μ|m|k|K|Meg|M|G|T)[0-9]*|[0-9]*(R|p|n|u|U|µ|μ|m|k|K|Meg|M|G|T)[0-9]+)\s*(Ohm|ohm|Ω|F|H|h|V|v|A|a|W|w|Hz|hz)?\s*$`)
var bareNumericPattern = regexp.MustCompile(`^[+-]?([0-9]+(\.[0-9]*)?|\.[0-9]+)$`)
var componentIDPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,62}$`)

const maxValueLiteralLength = 64

func ValidComponentID(id string) bool {
	return componentIDPattern.MatchString(id)
}

func Validate(document Document) []reports.Issue {
	return validateDefaulted(applyDefaults(document, true))
}

func validateDefaulted(document Document) []reports.Issue {
	ctx := validationContext{document: document}
	ctx.validateHeader()

	componentPins, componentRefs := ctx.validateComponents()
	ctx.validateSharedRefs(componentRefs)
	netNames, netEndpoints, unnamedNetEndpointCounts, noConnectEndpoints, usedEndpoints := ctx.validateNets(componentPins)
	ctx.validateBuses(netNames)
	ctx.validateNoConnectPins(noConnectEndpoints, usedEndpoints)
	portsByNet := ctx.validatePorts(netNames)
	ctx.validateNetCardinality(netEndpoints, unnamedNetEndpointCounts, portsByNet)
	ctx.validateGroups(componentPins)
	validateLayout(ctx.document, componentPins, ctx.add)
	validatePolicy(ctx.document, ctx.add)
	validateValues(ctx.document, ctx.add)
	return ctx.issues
}

func (ctx *validationContext) validateBuses(netNames map[string]Net) {
	busNames := map[string]int{}
	memberOwners := map[string]string{}
	busMembers := map[string]map[string]struct{}{}
	for index, bus := range ctx.document.Circuit.Buses {
		path := fmt.Sprintf("circuit.buses[%d]", index)
		if !ValidComponentID(bus.ID) {
			ctx.add(path+".id", "bus id must be a safe identifier")
		}
		if previous, exists := busNames[bus.ID]; exists {
			ctx.add(path+".id", fmt.Sprintf("duplicate bus id already defined at circuit.buses[%d]", previous))
		}
		busNames[bus.ID] = index
		if strings.TrimSpace(bus.Name) == "" {
			ctx.add(path+".name", "bus name is required")
		}
		if len(bus.Members) == 0 {
			ctx.add(path+".members", "bus must declare at least one member")
		}
		members := map[string]struct{}{}
		for memberIndex, member := range bus.Members {
			memberPath := fmt.Sprintf("%s.members[%d]", path, memberIndex)
			netName := strings.TrimSpace(member.Net)
			if netName == "" {
				ctx.add(memberPath+".net", "bus member net is required")
			} else if _, exists := members[netName]; exists {
				ctx.add(memberPath+".net", "bus member net is duplicated")
			} else {
				members[netName] = struct{}{}
			}
			if _, exists := netNames[netName]; !exists {
				ctx.add(memberPath+".net", "bus member references unknown net "+netName)
			} else if netNames[netName].Role == NetRoleNoConnect {
				ctx.add(memberPath+".net", "no_connect net cannot be a bus member")
			}
			if strings.TrimSpace(member.Label) == "" {
				ctx.add(memberPath+".label", "bus member label is required")
			}
			if previous, exists := memberOwners[netName]; exists && netName != "" {
				ctx.add(memberPath+".net", "net is already assigned to bus "+previous)
			}
			if netName != "" {
				memberOwners[netName] = bus.ID
			}
		}
		busMembers[bus.ID] = members
	}
	layoutNames := map[string]int{}
	for index, layout := range ctx.document.Layout.Buses {
		path := fmt.Sprintf("layout.buses[%d]", index)
		if previous, exists := layoutNames[layout.Bus]; exists {
			ctx.add(path+".bus", fmt.Sprintf("duplicate bus layout already defined at layout.buses[%d]", previous))
		}
		layoutNames[layout.Bus] = index
		members, known := busMembers[layout.Bus]
		if !known {
			ctx.add(path+".bus", "bus layout references unknown bus "+layout.Bus)
		}
		validateBusPoints(path+".points", layout.Points, ctx.add)
		entryMembers := map[string]struct{}{}
		entryEndpoints := map[string]map[EndpointRef]struct{}{}
		for entryIndex, entry := range layout.Entries {
			entryPath := fmt.Sprintf("%s.entries[%d]", path, entryIndex)
			memberNet := ""
			if _, exists := members[entry.Member]; !exists {
				ctx.add(entryPath+".member", "bus entry references unknown bus member "+entry.Member)
			} else {
				for _, bus := range ctx.document.Circuit.Buses {
					if bus.ID != layout.Bus {
						continue
					}
					for _, member := range bus.Members {
						if member.Net == entry.Member {
							memberNet = member.Net
						}
					}
				}
			}
			entryMembers[entry.Member] = struct{}{}
			if strings.TrimSpace(string(entry.Endpoint)) == "" {
				ctx.add(entryPath+".endpoint", "bus entry endpoint is required")
			} else if memberNet != "" {
				if _, exists := entryEndpoints[memberNet]; !exists {
					entryEndpoints[memberNet] = map[EndpointRef]struct{}{}
				}
				if _, duplicate := entryEndpoints[memberNet][entry.Endpoint]; duplicate {
					ctx.add(entryPath+".endpoint", "bus entry endpoint is duplicated for this member")
				}
				entryEndpoints[memberNet][entry.Endpoint] = struct{}{}
				if !busNetContainsEndpoint(ctx.document.Circuit.Nets, memberNet, entry.Endpoint) {
					ctx.add(entryPath+".endpoint", "bus entry endpoint is not connected to member net "+memberNet)
				}
			}
			if !finiteFloats(entry.At.XMM, entry.At.YMM, entry.Size.XMM, entry.Size.YMM) {
				ctx.add(entryPath, "bus entry geometry must be finite")
			}
			if entry.Size.XMM == 0 || entry.Size.YMM == 0 {
				ctx.add(entryPath+".size", "bus entry size must be non-zero on both axes")
			}
			if !layoutPointOnSegments(entry.At, layout.Points) {
				ctx.add(entryPath+".at", "bus entry must lie on a bus spine segment")
			}
		}
		if known {
			for member := range members {
				if _, exists := entryMembers[member]; !exists {
					ctx.add(path+".entries", "bus member "+member+" has no declared entry")
				}
				for _, bus := range ctx.document.Circuit.Buses {
					if bus.ID != layout.Bus {
						continue
					}
					for _, busMember := range bus.Members {
						if busMember.Net != member {
							continue
						}
						if len(entryEndpoints[member]) < countNetEndpoints(ctx.document.Circuit.Nets, member) {
							ctx.add(path+".entries", "bus member "+member+" does not have an entry for every scalar endpoint")
						}
					}
				}
			}
		}
	}
	for busID := range busNames {
		if _, exists := layoutNames[busID]; !exists {
			ctx.add("layout.buses", "bus "+busID+" requires explicit layout geometry")
		}
	}
}

func busNetContainsEndpoint(nets []Net, name string, endpoint EndpointRef) bool {
	for _, net := range nets {
		if net.Name != name {
			continue
		}
		for _, candidate := range net.Connect {
			if candidate == endpoint {
				return true
			}
		}
	}
	return false
}

func countNetEndpoints(nets []Net, name string) int {
	seen := map[EndpointRef]struct{}{}
	for _, net := range nets {
		if net.Name != name {
			continue
		}
		for _, endpoint := range net.Connect {
			seen[endpoint] = struct{}{}
		}
	}
	return len(seen)
}

func validateBusPoints(path string, points []LayoutPoint, add func(string, string)) {
	if len(points) < 2 {
		add(path, "bus requires at least two spine points")
		return
	}
	for index, point := range points {
		if !finiteFloats(point.XMM, point.YMM) {
			add(fmt.Sprintf("%s[%d]", path, index), "bus point must be finite")
		}
		if index == 0 {
			continue
		}
		previous := points[index-1]
		if point.XMM == previous.XMM && point.YMM == previous.YMM {
			add(fmt.Sprintf("%s[%d]", path, index), "bus spine segment must have non-zero length")
		} else if point.XMM != previous.XMM && point.YMM != previous.YMM {
			add(fmt.Sprintf("%s[%d]", path, index), "bus spine segments must be orthogonal")
		}
	}
}

func layoutPointOnSegments(point LayoutPoint, segments []LayoutPoint) bool {
	const epsilon = 0.000001
	for index := 1; index < len(segments); index++ {
		a, b := segments[index-1], segments[index]
		if a.XMM == b.XMM && math.Abs(point.XMM-a.XMM) <= epsilon && betweenFloat(point.YMM, a.YMM, b.YMM, epsilon) {
			return true
		}
		if a.YMM == b.YMM && math.Abs(point.YMM-a.YMM) <= epsilon && betweenFloat(point.XMM, a.XMM, b.XMM, epsilon) {
			return true
		}
	}
	return false
}

func betweenFloat(value, a, b, epsilon float64) bool {
	if a > b {
		a, b = b, a
	}
	return value >= a-epsilon && value <= b+epsilon
}

type validationContext struct {
	document Document
	issues   []reports.Issue
}

func (ctx *validationContext) add(path, message string) {
	ctx.issues = append(ctx.issues, issue(path, message))
}

func (ctx *validationContext) validateHeader() {
	if ctx.document.Schema != SchemaID {
		ctx.add("schema", "unsupported schema "+ctx.document.Schema)
	}
	if ctx.document.Version != Version {
		ctx.add("version", fmt.Sprintf("unsupported version %d", ctx.document.Version))
	}
	if !ValidComponentID(ctx.document.Metadata.Name) {
		ctx.add("metadata.name", "metadata name must match "+componentIDPattern.String())
	}
}

func (ctx *validationContext) validateComponents() (map[string]map[string]struct{}, map[string][]Component) {
	componentPins := map[string]map[string]struct{}{}
	componentRefs := map[string][]Component{}
	for index, component := range ctx.document.Circuit.Components {
		path := fmt.Sprintf("circuit.components[%d]", index)
		if !ValidComponentID(component.ID) {
			ctx.add(path+".id", "component id must match ^[a-zA-Z0-9][a-zA-Z0-9_-]{0,62}$ and cannot contain dots")
		}
		if _, exists := componentPins[component.ID]; exists {
			ctx.add(path+".id", "duplicate component id "+component.ID)
		}
		if component.Role == "" || !validComponentRole(component.Role) {
			ctx.add(path+".role", "component role is required and must be supported")
		}
		if !validLibraryID(component.Symbol) {
			ctx.add(path+".symbol", "symbol must be in Library:Name form")
		}
		if component.Footprint != "" && !validLibraryID(component.Footprint) {
			ctx.add(path+".footprint", "footprint must be in Library:Name form")
		}
		if component.Body != nil {
			if !finiteFloats(component.Body.MinXMM, component.Body.MinYMM, component.Body.MaxXMM, component.Body.MaxYMM) {
				ctx.add(path+".body", "body bounds must be finite")
			} else if component.Body.MaxXMM <= component.Body.MinXMM || component.Body.MaxYMM <= component.Body.MinYMM {
				ctx.add(path+".body", "body bounds must satisfy max > min on both axes")
			}
		}
		if !ctx.document.Policy.Repair.AllowRefAssignment && strings.TrimSpace(component.Ref) == "" {
			ctx.add(path+".ref", "ref is required when allow_ref_assignment is false")
		}
		if component.Ref != "" {
			componentRefs[component.Ref] = append(componentRefs[component.Ref], component)
		}
		pins := map[string]struct{}{}
		for pinIndex, pin := range component.Pins {
			pinPath := fmt.Sprintf("%s.pins[%d]", path, pinIndex)
			if strings.TrimSpace(pin.Number) == "" {
				ctx.add(pinPath+".number", "pin number is required")
			}
			if _, exists := pins[pin.Number]; exists {
				ctx.add(pinPath+".number", "duplicate pin number "+pin.Number)
			}
			if pin.Role != "" && !validPinRole(pin.Role) {
				ctx.add(pinPath+".role", "pin role is not supported")
			}
			pins[pin.Number] = struct{}{}
		}
		componentPins[component.ID] = pins
	}
	return componentPins, componentRefs
}

func finiteFloats(values ...float64) bool {
	for _, value := range values {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return false
		}
	}
	return true
}

func (ctx *validationContext) validateSharedRefs(componentRefs map[string][]Component) {
	for ref, components := range componentRefs {
		if len(components) < 2 {
			continue
		}
		units := map[string]struct{}{}
		symbol := components[0].Symbol
		footprint := ""
		footprintSources := 0
		for _, component := range components {
			if component.Unit == "" {
				ctx.add("circuit.components", "shared ref "+ref+" requires unique non-empty unit values")
			}
			if _, exists := units[component.Unit]; exists {
				ctx.add("circuit.components", "shared ref "+ref+" has duplicate unit "+component.Unit)
			}
			units[component.Unit] = struct{}{}
			if component.Symbol != symbol {
				ctx.add("circuit.components", "shared ref "+ref+" components must use identical symbols")
			}
			if component.Footprint != "" {
				footprintSources++
				if footprint == "" {
					footprint = component.Footprint
				} else if component.Footprint != footprint {
					ctx.add("circuit.components", "shared ref "+ref+" components must use identical footprints")
				}
			}
		}
		if footprintSources > 0 && footprintSources < len(components) {
			ctx.add("circuit.components", "shared ref "+ref+" footprint must be specified identically on every unit")
		}
	}
}

func (ctx *validationContext) validateNets(componentPins map[string]map[string]struct{}) (map[string]Net, map[string]map[EndpointRef]struct{}, map[int]int, map[EndpointRef]string, map[EndpointRef]struct{}) {
	netNames := map[string]Net{}
	netEndpoints := map[string]map[EndpointRef]struct{}{}
	unnamedNetEndpointCounts := map[int]int{}
	noConnectEndpoints := map[EndpointRef]string{}
	usedEndpoints := map[EndpointRef]struct{}{}
	pinToNet := map[EndpointRef]string{}
	for index, net := range ctx.document.Circuit.Nets {
		path := fmt.Sprintf("circuit.nets[%d]", index)
		netKey := net.Name
		if netKey == "" {
			netKey = path
		}
		if net.Role == "" || !validNetRole(net.Role) {
			ctx.add(path+".role", "net role is required and must be supported")
		}
		if net.Name == "" && net.Role != NetRoleNoConnect {
			ctx.add(path+".name", "net name is required")
		}
		if previous, exists := netNames[net.Name]; exists && net.Name != "" {
			if previous.Role != net.Role || previous.Label != net.Label || !sameBool(previous.UseLabel, net.UseLabel) {
				ctx.add(path+".name", "duplicate net names must have identical role, label, and use_label values")
			}
		}
		netNames[net.Name] = net
		uniqueEndpoints := map[EndpointRef]struct{}{}
		for endpointIndex, endpoint := range net.Connect {
			usedEndpoints[endpoint] = struct{}{}
			componentID, pinSelector, ok := endpoint.Split()
			if !ok {
				ctx.add(fmt.Sprintf("%s.connect[%d]", path, endpointIndex), "endpoint must use component_id.pin_selector form")
				continue
			}
			pins, exists := componentPins[componentID]
			if !exists {
				ctx.add(fmt.Sprintf("%s.connect[%d]", path, endpointIndex), "endpoint references unknown component "+componentID)
				continue
			}
			if _, exists := pins[pinSelector]; !exists {
				ctx.add(fmt.Sprintf("%s.connect[%d]", path, endpointIndex), "endpoint references unknown pin "+pinSelector)
			}
			if existingNet, exists := pinToNet[endpoint]; exists && existingNet != netKey {
				ctx.add(fmt.Sprintf("%s.connect[%d]", path, endpointIndex), fmt.Sprintf("endpoint %s is assigned to multiple nets: %s and %s", endpoint, existingNet, netKey))
			}
			pinToNet[endpoint] = netKey
			uniqueEndpoints[endpoint] = struct{}{}
		}
		if net.Name != "" {
			if _, exists := netEndpoints[net.Name]; !exists {
				netEndpoints[net.Name] = map[EndpointRef]struct{}{}
			}
			for endpoint := range uniqueEndpoints {
				netEndpoints[net.Name][endpoint] = struct{}{}
			}
		} else {
			unnamedNetEndpointCounts[index] = len(uniqueEndpoints)
		}
		if net.Role == NetRoleNoConnect {
			if len(uniqueEndpoints) != 1 {
				ctx.add(path+".connect", "no_connect nets must contain exactly one endpoint")
			}
			for endpoint := range uniqueEndpoints {
				noConnectEndpoints[endpoint] = path
			}
		}
	}
	return netNames, netEndpoints, unnamedNetEndpointCounts, noConnectEndpoints, usedEndpoints
}

func (ctx *validationContext) validateNoConnectPins(noConnectEndpoints map[EndpointRef]string, usedEndpoints map[EndpointRef]struct{}) {
	for index, component := range ctx.document.Circuit.Components {
		for pinIndex, pin := range component.Pins {
			if !pin.NoConnect {
				continue
			}
			ref := EndpointRef(component.ID + "." + pin.Number)
			if source, exists := noConnectEndpoints[ref]; exists {
				ctx.add(fmt.Sprintf("circuit.components[%d].pins[%d].no_connect", index, pinIndex), "pin no_connect duplicates "+source)
			}
			if _, exists := usedEndpoints[ref]; exists {
				ctx.add(fmt.Sprintf("circuit.components[%d].pins[%d].no_connect", index, pinIndex), "no_connect pin must not appear in any net endpoint")
			}
		}
	}
}

func (ctx *validationContext) validatePorts(netNames map[string]Net) map[string]int {
	portsByNet := map[string]int{}
	seenNames := map[string]struct{}{}
	for index, port := range ctx.document.Circuit.Ports {
		path := fmt.Sprintf("circuit.ports[%d]", index)
		if port.Name == "" {
			ctx.add(path+".name", "port name is required")
		} else if _, exists := seenNames[port.Name]; exists {
			ctx.add(path+".name", "duplicate port name "+port.Name)
		}
		seenNames[port.Name] = struct{}{}
		if !validPortDirection(port.Direction) {
			ctx.add(path+".direction", "port direction is not supported")
		}
		if port.ElectricalType != "" && !validElectricalType(port.ElectricalType) {
			ctx.add(path+".electrical_type", "port electrical_type is not supported")
		}
		if port.ElectricalType != "" && !validPortElectricalPair(port.Direction, port.ElectricalType) {
			ctx.add(path+".electrical_type", "port direction conflicts with electrical_type")
		}
		if port.Net == "" {
			ctx.add(path+".net", "port net is required")
		}
		if _, exists := netNames[port.Net]; !exists {
			ctx.add(path+".net", "port references unknown net "+port.Net)
		}
		if !validSide(port.Side) {
			ctx.add(path+".side", "port side is not supported")
		}
		if port.Net != "" {
			portsByNet[port.Net]++
		}
	}
	return portsByNet
}

func (ctx *validationContext) validateNetCardinality(netEndpoints map[string]map[EndpointRef]struct{}, unnamedNetEndpointCounts map[int]int, portsByNet map[string]int) {
	checkedNames := map[string]struct{}{}
	for index, net := range ctx.document.Circuit.Nets {
		if net.Role == NetRoleNoConnect {
			continue
		}
		if net.Name == "" {
			if unnamedNetEndpointCounts[index]+portsByNet[net.Name] < 2 {
				ctx.add(fmt.Sprintf("circuit.nets[%d]", index), "net must connect at least two unique endpoints or ports")
			}
			continue
		}
		if _, checked := checkedNames[net.Name]; checked {
			continue
		}
		checkedNames[net.Name] = struct{}{}
		if len(netEndpoints[net.Name])+portsByNet[net.Name] < 2 {
			ctx.add(fmt.Sprintf("circuit.nets[%d]", index), "net must connect at least two unique endpoints or ports")
		}
	}
}

func (ctx *validationContext) validateGroups(componentPins map[string]map[string]struct{}) {
	componentGroups := map[string]string{}
	for index, group := range ctx.document.Layout.Groups {
		path := fmt.Sprintf("layout.groups[%d]", index)
		if !ValidComponentID(group.ID) {
			ctx.add(path+".id", "group id must be a safe identifier")
		}
		for _, member := range group.Members {
			if _, exists := componentPins[member]; !exists {
				ctx.add(path+".members", "group member references unknown component "+member)
			}
			if existing, exists := componentGroups[member]; exists {
				ctx.add(path+".members", "component "+member+" appears in multiple groups: "+existing+" and "+group.ID)
			}
			componentGroups[member] = group.ID
		}
		if group.Side != "" && !validSide(group.Side) {
			ctx.add(path+".side", "group side is not supported")
		}
	}
}

func validateLayout(document Document, componentPins map[string]map[string]struct{}, add func(string, string)) {
	if document.Layout.Flow != FlowLeftToRight {
		add("layout.flow", "only left_to_right flow is supported in v1")
	}
	if document.Layout.Origin != OriginCentered && document.Layout.Origin != OriginPageUpperLeft {
		add("layout.origin", "layout origin is not supported")
	}
	if document.Layout.Lanes.Power != LanePositionTop {
		add("layout.lanes.power", "power lane must be top in v1")
	}
	if document.Layout.Lanes.Ground != LanePositionBottom {
		add("layout.lanes.ground", "ground lane must be bottom in v1")
	}
	if document.Layout.Lanes.Signals != LanePositionMiddle {
		add("layout.lanes.signals", "signals lane must be middle in v1")
	}
	for _, net := range document.Circuit.Nets {
		if net.Role == NetRolePowerNeg && document.Layout.Lanes.PowerNegative == LanePositionNone {
			add("layout.lanes.power_negative", "power_neg nets require explicit power_negative lane")
		}
	}
	if document.Layout.Origin == OriginCentered && document.Layout.Rules.CenterOnPage != nil && !*document.Layout.Rules.CenterOnPage {
		add("layout.rules.center_on_page", "centered origin conflicts with center_on_page=false")
	}
	if document.Layout.Origin == OriginPageUpperLeft && document.Layout.Rules.CenterOnPage != nil && *document.Layout.Rules.CenterOnPage {
		add("layout.rules.center_on_page", "page_upper_left origin conflicts with center_on_page=true")
	}
	if document.Layout.Rules.MinGroupSpacingMM != nil && *document.Layout.Rules.MinGroupSpacingMM < 0 {
		add("layout.rules.min_group_spacing_mm", "group spacing must be non-negative")
	}
	if document.Layout.Rules.MinComponentSpacingMM != nil && *document.Layout.Rules.MinComponentSpacingMM < 0 {
		add("layout.rules.min_component_spacing_mm", "component spacing must be non-negative")
	}
	if document.Layout.MaxComponentsPerSheet < 0 {
		add("layout.max_components_per_sheet", "maximum components per sheet must be non-negative")
	}
	groups := map[string]struct{}{}
	for _, group := range document.Layout.Groups {
		groups[group.ID] = struct{}{}
	}
	placementTargets := map[string]int{}
	for index, placement := range document.Layout.Placements {
		path := fmt.Sprintf("layout.placements[%d]", index)
		if placement.Target == "" {
			add(path+".target", "placement target is required")
		} else if _, exists := componentPins[placement.Target]; !exists {
			add(path+".target", "placement references unknown component "+placement.Target)
		} else if firstIndex, exists := placementTargets[placement.Target]; exists {
			add(path+".target", fmt.Sprintf("placement target %s is already defined at layout.placements[%d]", placement.Target, firstIndex))
		} else {
			placementTargets[placement.Target] = index
		}
		if placement.Group != "" {
			if _, exists := groups[placement.Group]; !exists {
				add(path+".group", "placement references unknown group "+placement.Group)
			}
		}
		if !supportedOrientation(placement.Orientation) {
			add(path+".orientation", "placement orientation is not supported")
		}
		if !supportedMirror(placement.Mirror) {
			add(path+".mirror", "placement mirror is not supported")
		}
		relations := []struct {
			field   string
			targets []string
		}{{"near", placement.Near}, {"above", placement.Above}, {"right_of", placement.RightOf}}
		for _, relation := range relations {
			seenTargets := map[string]struct{}{}
			for targetIndex, target := range relation.targets {
				targetPath := fmt.Sprintf("%s.%s[%d]", path, relation.field, targetIndex)
				if target == placement.Target {
					add(targetPath, "placement relation cannot reference its own target")
				} else if _, exists := componentPins[target]; !exists {
					add(targetPath, "placement relation references unknown component "+target)
				} else if _, duplicate := seenTargets[target]; duplicate {
					add(targetPath, "placement relation contains duplicate target "+target)
				}
				seenTargets[target] = struct{}{}
			}
		}
	}
	for _, relation := range []string{"above", "right_of"} {
		if cycle := PlacementRelationCycle(document.Layout.Placements, relation); len(cycle) != 0 {
			add("layout.placements", relation+" relation contains a cycle: "+FormatPlacementRelationCycle(cycle))
		}
	}
}

func supportedOrientation(orientation Orientation) bool {
	switch orientation {
	case "", OrientationNormal, OrientationRotated, OrientationRotated90, OrientationRotated180, OrientationRotated270:
		return true
	default:
		return false
	}
}

func supportedMirror(mirror Mirror) bool {
	switch mirror {
	case MirrorNone, MirrorX, MirrorY:
		return true
	default:
		return false
	}
}

func validatePolicy(document Document, add func(string, string)) {
	for key, action := range document.Policy.Validation {
		if action != IssueActionError && action != IssueActionWarning && action != IssueActionIgnore {
			add("policy.validation."+key, "validation action is not supported")
		}
	}
	if document.Policy.Repair.AllowSymbolSubstitution {
		add("policy.repair.allow_symbol_substitution", "symbol substitution is not supported in v1")
	}
	if document.Policy.Repair.AllowPinGuessing {
		add("policy.repair.allow_pin_guessing", "pin guessing is not supported in v1")
	}
	if document.Policy.Acceptance != AcceptanceStructural &&
		document.Policy.Acceptance != AcceptanceERCClean &&
		document.Policy.Acceptance != AcceptanceReadable {
		add("policy.acceptance", "acceptance level is not supported")
	}
}

func validateValues(document Document, add func(string, string)) {
	for index, component := range document.Circuit.Components {
		if !roleRequiresUnitValue(component.Role) {
			continue
		}
		value := strings.TrimSpace(component.Value)
		if value == "" {
			add(fmt.Sprintf("circuit.components[%d].value", index), "component value is required for this role")
			continue
		}
		if len(value) > maxValueLiteralLength {
			add(fmt.Sprintf("circuit.components[%d].value", index), "component value exceeds maximum literal length")
			continue
		}
		if bareNumeric(value) || !electricalValuePattern.MatchString(value) {
			add(fmt.Sprintf("circuit.components[%d].value", index), "component value requires an explicit electrical unit or suffix")
		}
	}
}

func validLibraryID(value string) bool {
	return strings.Count(value, ":") == 1 && !strings.HasPrefix(value, ":") && !strings.HasSuffix(value, ":")
}

func sameBool(a, b *bool) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func bareNumeric(value string) bool {
	return bareNumericPattern.MatchString(value)
}

func roleRequiresUnitValue(role ComponentRole) bool {
	switch role {
	case ComponentRoleResistor, ComponentRoleCurrentLimiter, ComponentRolePullup,
		ComponentRoleCapacitor, ComponentRoleDecouplingCapacitor,
		ComponentRoleBulkCapacitor, ComponentRoleInductor:
		return true
	default:
		return false
	}
}

func validComponentRole(role ComponentRole) bool {
	switch role {
	case ComponentRoleConnector, ComponentRoleInputConnector, ComponentRoleOutputConnector,
		ComponentRoleResistor, ComponentRoleCurrentLimiter, ComponentRolePullup,
		ComponentRoleCapacitor, ComponentRoleDecouplingCapacitor, ComponentRoleBulkCapacitor,
		ComponentRoleInductor, ComponentRoleDiode, ComponentRoleIndicatorLED,
		ComponentRoleIC, ComponentRoleSensor, ComponentRoleRegulator,
		ComponentRoleTransistor, ComponentRoleBJT, ComponentRoleMOSFET,
		ComponentRoleSwitch, ComponentRoleCrystal, ComponentRoleOscillator,
		ComponentRoleProtection, ComponentRoleFuse, ComponentRoleTVS,
		ComponentRolePowerSymbol, ComponentRoleGroundSymbol,
		ComponentRoleTestpoint, ComponentRoleGeneric:
		return true
	default:
		return false
	}
}

func validPinRole(role PinRole) bool {
	switch role {
	case PinRoleInput, PinRoleOutput, PinRolePower, PinRoleGround, PinRolePassive, PinRoleBidirectional:
		return true
	default:
		return false
	}
}

func validNetRole(role NetRole) bool {
	switch role {
	case NetRoleSignal, NetRolePower, NetRolePowerPos, NetRolePowerNeg, NetRoleGround,
		NetRoleReturn, NetRoleFeedback, NetRoleBias, NetRoleShield, NetRoleBus, NetRoleNoConnect:
		return true
	default:
		return false
	}
}

func validPortDirection(direction PortDirection) bool {
	switch direction {
	case PortDirectionInput, PortDirectionOutput, PortDirectionBidirectional,
		PortDirectionPassive, PortDirectionTriState, PortDirectionUnspecified:
		return true
	default:
		return false
	}
}

func validElectricalType(electricalType ElectricalType) bool {
	switch electricalType {
	case ElectricalTypeInput, ElectricalTypeOutput, ElectricalTypeBidirectional,
		ElectricalTypeTriState, ElectricalTypePassive, ElectricalTypeUnspecified,
		ElectricalTypePowerInput, ElectricalTypePowerOutput,
		ElectricalTypeOpenCollector, ElectricalTypeOpenEmitter,
		ElectricalTypeNoConnect:
		return true
	default:
		return false
	}
}

func validPortElectricalPair(direction PortDirection, electricalType ElectricalType) bool {
	switch direction {
	case PortDirectionInput:
		return electricalType == ElectricalTypeInput || electricalType == ElectricalTypePassive ||
			electricalType == ElectricalTypePowerInput || electricalType == ElectricalTypeUnspecified
	case PortDirectionOutput:
		return electricalType == ElectricalTypeOutput || electricalType == ElectricalTypePassive ||
			electricalType == ElectricalTypePowerOutput || electricalType == ElectricalTypeOpenCollector ||
			electricalType == ElectricalTypeOpenEmitter || electricalType == ElectricalTypeUnspecified
	case PortDirectionBidirectional:
		return electricalType == ElectricalTypeBidirectional || electricalType == ElectricalTypePassive ||
			electricalType == ElectricalTypeUnspecified
	case PortDirectionTriState:
		return electricalType == ElectricalTypeTriState || electricalType == ElectricalTypeUnspecified
	case PortDirectionPassive:
		return electricalType == ElectricalTypePassive || electricalType == ElectricalTypeUnspecified
	case PortDirectionUnspecified:
		return true
	default:
		return false
	}
}

func validSide(side Side) bool {
	switch side {
	case SideLeft, SideRight, SideTop, SideBottom:
		return true
	default:
		return false
	}
}
