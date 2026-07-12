package circuitgraph

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"kicadai/internal/reports"
	"kicadai/internal/schematicir"
)

// ToSchematicIR lowers only trusted resolved bindings. Unresolved provider
// selectors never reach schematic transactions through this adapter.
func ToSchematicIR(resolved ResolvedDocument) (schematicir.Document, []reports.Issue) {
	if resolved.ResolutionHash == "" {
		return schematicir.Document{}, []reports.Issue{graphIssue(CodeSchematicLowering, "resolution_hash", "resolved graph provenance is required")}
	}

	unitIDs, unitsByComponent, issues := schematicUnitIDs(resolved)
	references, referenceIssues := schematicReferences(resolved)
	issues = append(issues, referenceIssues...)
	noConnects := schematicNoConnects(resolved)

	unitCount := 0
	for _, units := range unitsByComponent {
		unitCount += len(units)
	}
	document := schematicir.Document{
		Schema: schematicir.SchemaID, Version: schematicir.Version,
		Metadata: schematicir.Metadata{
			Name: resolved.Source.Project.Name, Title: resolved.Source.Project.Title,
			Description: resolved.Source.Project.Description, Seed: resolved.ResolutionHash, Paper: schematicir.DefaultPaper,
		},
		Circuit: schematicir.Circuit{
			Components: make([]schematicir.Component, 0, unitCount),
			Nets:       make([]schematicir.Net, 0, len(resolved.Nets)),
			Buses:      make([]schematicir.Bus, 0, len(resolved.Source.Buses)),
		},
		Layout: schematicir.Layout{
			Flow: schematicir.FlowLeftToRight, Origin: schematicir.OriginCentered,
			Lanes: schematicir.Lanes{Power: schematicir.LanePositionTop, Signals: schematicir.LanePositionMiddle, Ground: schematicir.LanePositionBottom},
		},
		Policy: schematicPolicy(resolved.Source),
	}

	for _, component := range resolved.Components {
		units := unitsByComponent[component.Instance.ID]
		for _, unit := range units {
			irComponent, componentIssues := schematicComponent(component, unit, unitIDs, references, noConnects, resolved)
			issues = append(issues, componentIssues...)
			if !reports.HasBlockingIssue(componentIssues) {
				document.Circuit.Components = append(document.Circuit.Components, irComponent)
			}
		}
	}
	for netIndex, net := range resolved.Nets {
		irNet, netIssues := schematicNet(net, netIndex, unitIDs)
		issues = append(issues, netIssues...)
		if !reports.HasBlockingIssue(netIssues) {
			document.Circuit.Nets = append(document.Circuit.Nets, irNet)
		}
	}
	for _, bus := range resolved.Source.Buses {
		irBus := schematicir.Bus{ID: bus.ID, Name: bus.Name, Members: make([]schematicir.BusMember, 0, len(bus.Members))}
		for _, member := range bus.Members {
			irBus.Members = append(irBus.Members, schematicir.BusMember{Net: member.Net, Label: member.Label})
		}
		document.Circuit.Buses = append(document.Circuit.Buses, irBus)
	}
	document.Layout = schematicLayoutIntent(resolved.Source, unitIDs, unitsByComponent)
	document.Layout.Buses = inferredSchematicBusLayouts(document.Circuit)
	document = schematicir.NormalizeLayoutIntent(document)
	issues = append(issues, schematicir.Validate(document)...)
	if reports.HasBlockingIssue(issues) {
		return schematicir.Document{}, dedupeGraphIssues(issues)
	}
	return document, dedupeGraphIssues(issues)
}

type schematicUnitKey struct {
	component string
	unit      int
}

func schematicUnitIDs(resolved ResolvedDocument) (map[schematicUnitKey]string, map[string][]int, []reports.Issue) {
	ids := map[schematicUnitKey]string{}
	unitsByComponent := map[string][]int{}
	idOwners := map[string]string{}
	var issues []reports.Issue
	for _, component := range resolved.Components {
		unitSet := map[int]struct{}{}
		for _, function := range component.Functions {
			unitSet[function.Unit] = struct{}{}
		}
		units := make([]int, 0, len(unitSet))
		for unit := range unitSet {
			units = append(units, unit)
		}
		slices.Sort(units)
		if len(units) == 0 {
			issues = append(issues, graphIssue(CodeSchematicLowering, "components."+component.Instance.ID, "component has no resolved symbol units"))
			continue
		}
		unitsByComponent[component.Instance.ID] = units
		for _, unit := range units {
			id := component.Instance.ID
			if len(units) > 1 {
				id += "_u" + strconv.Itoa(unit)
			}
			if owner, exists := idOwners[id]; exists && owner != component.Instance.ID {
				issues = append(issues, graphIssue(CodeSchematicLowering, "components."+component.Instance.ID+".id", "lowered schematic component id collides with "+owner))
				continue
			}
			idOwners[id] = component.Instance.ID
			ids[schematicUnitKey{component: component.Instance.ID, unit: unit}] = id
		}
	}
	return ids, unitsByComponent, issues
}

func schematicReferences(resolved ResolvedDocument) (map[string]string, []reports.Issue) {
	result := map[string]string{}
	used := map[string]struct{}{}
	counters := map[string]int{}
	for _, component := range resolved.Components {
		if component.Instance.Reference != "" {
			result[component.Instance.ID] = component.Instance.Reference
			used[strings.ToUpper(component.Instance.Reference)] = struct{}{}
		}
	}
	var issues []reports.Issue
	for _, component := range resolved.Components {
		if result[component.Instance.ID] != "" {
			continue
		}
		if !policyEnabled(resolved.Source.Policy.AllowReferenceAssignment) {
			issues = append(issues, graphIssue(CodeSchematicLowering, "components."+component.Instance.ID+".reference", "reference is missing and assignment is forbidden"))
			continue
		}
		prefix := schematicReferencePrefix(component.Instance.Role)
		assigned := false
		for attempt := 0; attempt <= MaxComponents; attempt++ {
			counters[prefix]++
			candidate := prefix + strconv.Itoa(counters[prefix])
			if _, exists := used[candidate]; exists {
				continue
			}
			used[candidate] = struct{}{}
			result[component.Instance.ID] = candidate
			assigned = true
			break
		}
		if !assigned {
			issues = append(issues, graphIssue(CodeSchematicLowering, "components."+component.Instance.ID+".reference", "no unique reference is available within the component limit"))
		}
	}
	return result, issues
}

func schematicComponent(component ResolvedComponent, unit int, unitIDs map[schematicUnitKey]string, references map[string]string, noConnects map[schematicUnitKey]map[string]struct{}, resolved ResolvedDocument) (schematicir.Component, []reports.Issue) {
	key := schematicUnitKey{component: component.Instance.ID, unit: unit}
	id := unitIDs[key]
	symbolID := ""
	pinsByNumber := map[string]schematicir.Pin{}
	for _, function := range component.Functions {
		if function.Unit != unit {
			continue
		}
		if function.SymbolID == "" {
			return schematicir.Component{}, []reports.Issue{graphIssue(CodeSchematicLowering, "components."+component.Instance.ID, "resolved function has no symbol library id")}
		}
		if symbolID == "" {
			symbolID = function.SymbolID
		} else if symbolID != function.SymbolID {
			return schematicir.Component{}, []reports.Issue{graphIssue(CodeSchematicLowering, "components."+component.Instance.ID, "one symbol unit resolves to multiple library symbols")}
		}
		pin := schematicir.Pin{Number: function.SymbolPin, Name: function.Function, Role: schematicPinRole(function)}
		if _, exists := noConnects[key][function.SymbolPin]; exists {
			pin.NoConnect = true
		}
		if existing, exists := pinsByNumber[pin.Number]; exists && (existing.Name != pin.Name || existing.Role != pin.Role || existing.NoConnect != pin.NoConnect) {
			return schematicir.Component{}, []reports.Issue{graphIssue(CodeSchematicLowering, "components."+component.Instance.ID+".pins."+pin.Number, "one symbol pin resolves to conflicting logical functions")}
		}
		pinsByNumber[pin.Number] = pin
	}
	if id == "" || symbolID == "" || len(pinsByNumber) == 0 {
		return schematicir.Component{}, []reports.Issue{graphIssue(CodeSchematicLowering, "components."+component.Instance.ID, "symbol unit is incomplete")}
	}
	pins := make([]schematicir.Pin, 0, len(pinsByNumber))
	for _, pin := range pinsByNumber {
		pins = append(pins, pin)
	}
	slices.SortStableFunc(pins, func(left, right schematicir.Pin) int { return compareSchematicPinNumbers(left.Number, right.Number) })
	properties := map[string]string{}
	for _, property := range component.Instance.Properties {
		properties[property.Name] = property.Value
	}
	if component.Manufacturer != "" {
		properties["Manufacturer"] = component.Manufacturer
	}
	if component.MPN != "" {
		properties["MPN"] = component.MPN
	}
	properties["KiCadAI Component ID"] = component.ComponentID
	properties["KiCadAI Variant ID"] = component.VariantID
	properties["KiCadAI Catalog"] = resolved.CatalogID
	properties["KiCadAI Catalog Hash"] = resolved.CatalogHash
	properties["KiCadAI Resolution Hash"] = resolved.ResolutionHash
	unitValue := ""
	if unit > 0 {
		unitValue = strconv.Itoa(unit)
	}
	return schematicir.Component{
		ID: id, Ref: references[component.Instance.ID], Unit: unitValue,
		Role: schematicComponentRole(component.Instance.Role), Symbol: symbolID,
		Value: schematicValue(component, resolved.Source.Policy), Footprint: component.FootprintID,
		Pins: pins, Properties: properties,
	}, nil
}

func schematicNet(net ResolvedNet, index int, unitIDs map[schematicUnitKey]string) (schematicir.Net, []reports.Issue) {
	result := schematicir.Net{Name: net.Intent.Name, Role: schematicNetRole(net.Intent.Role)}
	seen := map[schematicir.EndpointRef]struct{}{}
	var issues []reports.Issue
	for endpointIndex, endpoint := range net.Endpoints {
		for _, binding := range endpoint.Bindings {
			id := unitIDs[schematicUnitKey{component: endpoint.Intent.Component, unit: binding.Unit}]
			if id == "" || binding.SymbolPin == "" {
				path := fmt.Sprintf("nets[%d].endpoints[%d]", index, endpointIndex)
				issues = append(issues, graphIssue(CodeSchematicLowering, path, "resolved endpoint has no schematic unit or pin"))
				continue
			}
			ref := schematicir.EndpointRef(id + "." + binding.SymbolPin)
			if _, exists := seen[ref]; exists {
				continue
			}
			seen[ref] = struct{}{}
			result.Connect = append(result.Connect, ref)
		}
	}
	slices.Sort(result.Connect)
	return result, issues
}

func schematicNoConnects(resolved ResolvedDocument) map[schematicUnitKey]map[string]struct{} {
	result := map[schematicUnitKey]map[string]struct{}{}
	for _, endpoint := range resolved.NoConnects {
		for _, binding := range endpoint.Bindings {
			key := schematicUnitKey{component: endpoint.Intent.Component, unit: binding.Unit}
			if result[key] == nil {
				result[key] = map[string]struct{}{}
			}
			result[key][binding.SymbolPin] = struct{}{}
		}
	}
	return result
}

func inferredSchematicBusLayouts(circuit schematicir.Circuit) []schematicir.BusLayout {
	const (
		gridMM       = 2.54
		busOriginXMM = 76.2
		busOriginYMM = 76.2
		busRowMM     = 12.7
		entryStepMM  = 5.08
		minBusMM     = 25.4
	)
	netsByName := make(map[string]schematicir.Net, len(circuit.Nets))
	for _, net := range circuit.Nets {
		netsByName[net.Name] = net
	}
	layouts := make([]schematicir.BusLayout, 0, len(circuit.Buses))
	for busIndex, bus := range circuit.Buses {
		y := busOriginYMM + float64(busIndex)*busRowMM
		length := minBusMM
		endpointCount := 0
		for _, member := range bus.Members {
			endpointCount += len(netsByName[member.Net].Connect)
		}
		if endpointLength := float64(endpointCount+2) * entryStepMM; endpointLength > length {
			length = endpointLength
		}
		layout := schematicir.BusLayout{
			Bus:    bus.ID,
			Points: []schematicir.LayoutPoint{{XMM: busOriginXMM, YMM: y}, {XMM: busOriginXMM + length, YMM: y}},
		}
		entryIndex := 0
		for _, member := range bus.Members {
			for _, endpoint := range netsByName[member.Net].Connect {
				x := busOriginXMM + 2*gridMM + float64(entryIndex)*entryStepMM
				direction := gridMM
				if entryIndex%2 != 0 {
					direction = -gridMM
				}
				layout.Entries = append(layout.Entries, schematicir.BusEntryLayout{
					Member: member.Net, Endpoint: endpoint,
					At: schematicir.LayoutPoint{XMM: x, YMM: y}, Size: schematicir.LayoutPoint{XMM: gridMM, YMM: direction},
				})
				entryIndex++
			}
		}
		layouts = append(layouts, layout)
	}
	return layouts
}

func schematicLayoutIntent(source Document, unitIDs map[schematicUnitKey]string, unitsByComponent map[string][]int) schematicir.Layout {
	layout := schematicir.Layout{
		Flow: schematicir.FlowLeftToRight, Origin: schematicir.OriginCentered,
		Lanes: schematicir.Lanes{Power: schematicir.LanePositionTop, Signals: schematicir.LanePositionMiddle, Ground: schematicir.LanePositionBottom},
		Rules: schematicir.LayoutRules{
			PositivePowerTop: source.Schematic.Rules.PositivePowerTop, GroundBottom: source.Schematic.Rules.GroundBottom,
			CenterOnPage: source.Schematic.Rules.CenterOnPage, PreferLabelsForLongNets: source.Schematic.Rules.PreferLabelsForLongNets,
			AvoidWireCrossings: source.Schematic.Rules.AvoidWireCrossings,
			MinGroupSpacingMM:  floatPointer(source.Schematic.Rules.MinGroupSpacingMM), MinComponentSpacingMM: floatPointer(source.Schematic.Rules.MinComponentSpacingMM),
		},
	}
	for _, group := range source.Schematic.Groups {
		irGroup := schematicir.Group{ID: group.ID, Label: group.Label, Role: schematicir.GroupRole(group.Role), Rank: group.Rank, Side: schematicir.Side(group.Side)}
		for _, member := range group.Members {
			for _, unit := range unitsByComponent[member] {
				irGroup.Members = append(irGroup.Members, unitIDs[schematicUnitKey{component: member, unit: unit}])
			}
		}
		layout.Groups = append(layout.Groups, irGroup)
	}
	for _, placement := range source.Schematic.Placements {
		units := unitsByComponent[placement.Component]
		for unitIndex, unit := range units {
			target := unitIDs[schematicUnitKey{component: placement.Component, unit: unit}]
			irPlacement := schematicir.Placement{Target: target, Group: placement.Group, Orientation: schematicir.Orientation(placement.Orientation), Mirror: schematicMirror(placement.Mirror)}
			if unitIndex == 0 {
				irPlacement.Near = optionalSchematicRelation(placement.Near, unitIDs, unitsByComponent)
				irPlacement.Above = optionalSchematicRelation(placement.Above, unitIDs, unitsByComponent)
				irPlacement.RightOf = optionalSchematicRelation(placement.RightOf, unitIDs, unitsByComponent)
			} else {
				irPlacement.Near = []string{unitIDs[schematicUnitKey{component: placement.Component, unit: units[0]}]}
			}
			layout.Placements = append(layout.Placements, irPlacement)
		}
	}
	return layout
}

func optionalSchematicRelation(component string, unitIDs map[schematicUnitKey]string, unitsByComponent map[string][]int) []string {
	id := primarySchematicID(component, unitIDs, unitsByComponent)
	if id == "" {
		return nil
	}
	return []string{id}
}

func primarySchematicID(component string, unitIDs map[schematicUnitKey]string, unitsByComponent map[string][]int) string {
	units := unitsByComponent[component]
	if len(units) == 0 {
		return ""
	}
	return unitIDs[schematicUnitKey{component: component, unit: units[0]}]
}

func schematicPolicy(source Document) schematicir.Policy {
	acceptance := schematicir.AcceptanceStructural
	if source.Project.Acceptance == AcceptanceConnectivity || source.Project.Acceptance == AcceptanceERCDRC {
		acceptance = schematicir.AcceptanceERCClean
	}
	if source.Project.Acceptance == AcceptanceFabricationCandidate {
		acceptance = schematicir.AcceptanceReadable
	}
	return schematicir.Policy{
		Acceptance: acceptance,
		Repair: schematicir.RepairPolicy{
			AllowRefAssignment:          policyEnabled(source.Policy.AllowReferenceAssignment),
			AllowLabelInsertion:         policyEnabled(source.Policy.AllowLabelInsertion),
			AllowGroupSpacingAdjustment: policyEnabled(source.Policy.AllowSpacingAdjustment),
			AllowSymbolSubstitution:     false, AllowPinGuessing: false,
		},
	}
}

func schematicComponentRole(role ComponentRole) schematicir.ComponentRole {
	return schematicir.ComponentRole(role)
}
func schematicNetRole(role NetRole) schematicir.NetRole { return schematicir.NetRole(role) }

func schematicPinRole(function ResolvedFunction) schematicir.PinRole {
	electrical := strings.ToLower(function.Electrical)
	name := strings.ToUpper(function.Function)
	switch {
	case schematicGroundFunction(name):
		return schematicir.PinRoleGround
	case strings.Contains(electrical, "power"):
		return schematicir.PinRolePower
	case electrical == "input":
		return schematicir.PinRoleInput
	case electrical == "output":
		return schematicir.PinRoleOutput
	case electrical == "bidirectional":
		return schematicir.PinRoleBidirectional
	default:
		return schematicir.PinRolePassive
	}
}

func schematicGroundFunction(name string) bool {
	switch name {
	case "GND", "GROUND", "VSS", "AGND", "DGND", "PGND":
		return true
	default:
		return strings.HasSuffix(name, "_GND") || strings.HasSuffix(name, "_GROUND")
	}
}

func compareSchematicPinNumbers(left, right string) int {
	leftNumber, leftErr := strconv.Atoi(left)
	rightNumber, rightErr := strconv.Atoi(right)
	if leftErr == nil && rightErr == nil {
		if leftNumber < rightNumber {
			return -1
		}
		if leftNumber > rightNumber {
			return 1
		}
		return strings.Compare(left, right)
	}
	return strings.Compare(left, right)
}

func schematicMirror(value string) schematicir.Mirror {
	if value == "none" {
		return schematicir.MirrorNone
	}
	return schematicir.Mirror(value)
}

func schematicReferencePrefix(role ComponentRole) string {
	switch role {
	case RoleResistor, RoleCurrentLimiter, RolePullup:
		return "R"
	case RoleCapacitor, RoleDecouplingCapacitor, RoleBulkCapacitor:
		return "C"
	case RoleDiode, RoleIndicatorLED, RoleProtection, RoleTVS:
		return "D"
	case RoleConnector, RoleInputConnector, RoleOutputConnector:
		return "J"
	case RoleFuse:
		return "F"
	case RoleInductor:
		return "L"
	case RoleTransistor, RoleBJT, RoleMOSFET:
		return "Q"
	case RoleSwitch:
		return "SW"
	case RoleCrystal, RoleOscillator:
		return "Y"
	default:
		return "U"
	}
}

func schematicValue(component ResolvedComponent, policy Policy) string {
	value := firstNonEmpty(component.Instance.Value, component.MPN, component.Record.Name)
	if !policyEnabled(policy.AllowValueNormalization) {
		return value
	}
	if component.Instance.Role == RoleResistor || component.Instance.Role == RoleCurrentLimiter || component.Instance.Role == RolePullup {
		if _, err := strconv.ParseFloat(value, 64); err == nil {
			return value + "ohm"
		}
	}
	return value
}

func policyEnabled(value *bool) bool { return value != nil && *value }

func floatPointer(value float64) *float64 { return &value }
