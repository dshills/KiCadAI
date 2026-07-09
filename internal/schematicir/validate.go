package schematicir

import (
	"fmt"
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
	ctx.validateNoConnectPins(noConnectEndpoints, usedEndpoints)
	portsByNet := ctx.validatePorts(netNames)
	ctx.validateNetCardinality(netEndpoints, unnamedNetEndpointCounts, portsByNet)
	ctx.validateGroups(componentPins)
	validateLayout(ctx.document, componentPins, ctx.add)
	validatePolicy(ctx.document, ctx.add)
	validateValues(ctx.document, ctx.add)
	return ctx.issues
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
		NetRoleReturn, NetRoleFeedback, NetRoleBias, NetRoleShield, NetRoleNoConnect:
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
