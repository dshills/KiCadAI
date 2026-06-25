package intentplanner

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"kicadai/internal/blocks"
	"kicadai/internal/components"
	"kicadai/internal/designworkflow"
	"kicadai/internal/reports"
)

const voltageCompareEpsilon = 0.01

var builtinIntentRegistry = blocks.NewBuiltinRegistry()

func Plan(request Request) PlanResult {
	normalized := NormalizeRequest(request)
	plan := NewPlan(normalized)
	if reports.HasBlockingIssue(plan.Issues) {
		return NormalizePlan(plan)
	}
	builder := planBuilder{
		request:  normalized,
		registry: builtinIntentRegistry,
		plan:     plan,
		workflow: designworkflow.Request{
			Version: designworkflow.RequestVersion,
			Name:    normalized.Name,
			Intent: designworkflow.Intent{
				Summary:  normalized.Summary,
				Category: string(normalized.Kind),
			},
			Board: designworkflow.BoardSpec{
				WidthMM:  normalized.Board.WidthMM,
				HeightMM: normalized.Board.HeightMM,
				Layers:   normalized.Board.Layers,
			},
			Validation: designworkflow.ValidationSpec{
				Acceptance: normalized.Acceptance,
			},
		},
		ids:              map[string]int{},
		usedIDs:          map[string]bool{},
		instanceBlockIDs: map[string]string{},
		instanceParams:   map[string]map[string]any{},
		instanceVoltages: map[string]string{},
		regulatorSources: map[string]powerSource{},
		protectedSources: map[string]string{},
	}
	builder.applyBoardDefaults()
	builder.applyPolicyDefaults()
	builder.mapPower()
	builder.mapFunctions()
	builder.mapInterfaces()
	builder.mapProtection()
	builder.connectPowerAndSignals()
	if len(builder.workflow.Blocks) == 0 && !reports.HasBlockingIssue(builder.plan.Issues) {
		builder.addIssue("intent", "intent did not map to any supported circuit blocks", "choose a supported function, interface, power input, or protection requirement")
	}
	workflow := designworkflow.NormalizeRequest(builder.workflow)
	builder.plan.GeneratedRequest = &workflow
	builder.plan.Issues = append(builder.plan.Issues, designworkflow.ValidateRequest(workflow)...)
	return NormalizePlan(builder.plan)
}

type planBuilder struct {
	request            Request
	registry           blocks.Registry
	plan               PlanResult
	workflow           designworkflow.Request
	ids                map[string]int
	usedIDs            map[string]bool
	instanceBlockIDs   map[string]string
	instanceParams     map[string]map[string]any
	instanceVoltages   map[string]string
	regulatorSources   map[string]powerSource
	protectedSources   map[string]string
	usbPowerIDs        []string
	regulatorIDs       []string
	sensorIDs          []string
	mcuIDs             []string
	connectorIDs       []string
	i2cConnectorIDs    []string
	gpioConnectorIDs   []string
	signalConnectorIDs []string
	powerConnectorIDs  []string
	inputPowerIDs      []string
	orderedPowerInputs []powerSource
	ledIDs             []string
	amplifierIDs       []string
	clockIDs           []string
	poweredClockIDs    []string
	programmingIDs     []string
	esdIDs             []string
	reversePolarityIDs []string
}

type powerSource struct {
	id      string
	port    string
	voltage string
}

func (builder *planBuilder) applyBoardDefaults() {
	if builder.workflow.Board.WidthMM == 0 {
		builder.workflow.Board.WidthMM = 50
		builder.plan.Assumptions = append(builder.plan.Assumptions, PlanNote{ID: "board.width.default", Path: "board.width_mm", Message: "defaulted board width to 50 mm"})
	}
	if builder.workflow.Board.HeightMM == 0 {
		builder.workflow.Board.HeightMM = 30
		builder.plan.Assumptions = append(builder.plan.Assumptions, PlanNote{ID: "board.height.default", Path: "board.height_mm", Message: "defaulted board height to 30 mm"})
	}
}

func (builder *planBuilder) applyPolicyDefaults() {
	acceptance := builder.request.Acceptance
	if builder.request.Manufacturing.FabricationCandidate {
		acceptance = designworkflow.AcceptanceFabricationCandidate
	}
	builder.workflow.Components = designworkflow.ComponentPolicySpec{
		MinimumConfidence:  componentConfidenceForIntent(acceptance, builder.request.Constraints.AllowPlaceholders),
		Acceptance:         componentAcceptanceForIntent(acceptance),
		PackagePreferences: cloneStringMap(builder.request.Constraints.PackagePreferences),
	}
	if builder.request.Constraints.RouteWidthMM > 0 {
		builder.workflow.Constraints.RouteWidthMM = builder.request.Constraints.RouteWidthMM
	}
	if builder.request.Constraints.ClearanceMM > 0 {
		builder.workflow.Constraints.ClearanceMM = builder.request.Constraints.ClearanceMM
	}
	builder.workflow.Validation = validationForIntent(acceptance, builder.request.Constraints.SkipRouting)
	builder.workflow.RoutingRetry = routingRetryForIntent(acceptance)
	builder.recordComponentPolicy(acceptance)
}

func componentConfidenceForIntent(acceptance designworkflow.AcceptanceLevel, allowPlaceholders bool) components.ConfidenceLevel {
	if allowPlaceholders && acceptance == designworkflow.AcceptanceDraft {
		return components.ConfidencePlaceholder
	}
	switch acceptance {
	case designworkflow.AcceptanceDraft:
		return components.ConfidenceRuleInferred
	case designworkflow.AcceptanceStructural:
		return components.ConfidenceRuleInferred
	case designworkflow.AcceptanceConnectivity:
		return components.ConfidenceLibraryDerived
	case designworkflow.AcceptanceERCDRC:
		return components.ConfidenceLibraryDerived
	case designworkflow.AcceptanceFabricationCandidate:
		return components.ConfidenceVerified
	default:
		return components.ConfidenceRuleInferred
	}
}

func componentAcceptanceForIntent(acceptance designworkflow.AcceptanceLevel) components.AcceptanceLevel {
	switch acceptance {
	case designworkflow.AcceptanceDraft:
		return components.AcceptanceDraft
	case designworkflow.AcceptanceStructural:
		return components.AcceptanceStructural
	case designworkflow.AcceptanceConnectivity:
		return components.AcceptanceConnectivity
	case designworkflow.AcceptanceERCDRC:
		return components.AcceptanceERCDRC
	case designworkflow.AcceptanceFabricationCandidate:
		return components.AcceptanceFabricationCandidate
	default:
		return components.AcceptanceStructural
	}
}

func validationForIntent(acceptance designworkflow.AcceptanceLevel, skipRouting bool) designworkflow.ValidationSpec {
	validation := designworkflow.ValidationSpec{Acceptance: acceptance, SkipRouting: skipRouting}
	switch acceptance {
	case designworkflow.AcceptanceConnectivity:
		validation.StrictUnrouted = !skipRouting
	case designworkflow.AcceptanceERCDRC, designworkflow.AcceptanceFabricationCandidate:
		validation.StrictUnrouted = !skipRouting
		validation.StrictZones = true
		validation.RequireERC = true
		validation.RequireDRC = true
	}
	return validation
}

func routingRetryForIntent(acceptance designworkflow.AcceptanceLevel) designworkflow.RoutingRetryPolicySpec {
	if acceptance != designworkflow.AcceptanceFabricationCandidate {
		return designworkflow.RoutingRetryPolicySpec{}
	}
	return designworkflow.RoutingRetryPolicySpec{
		Enabled:              true,
		MaxAttempts:          2,
		MinRoutingScoreDelta: 0.01,
		DRCPolicy:            designworkflow.RetryDRCPolicyOptional,
		PreserveFixed:        true,
		StopOnNewBlockers:    true,
	}
}

func (builder *planBuilder) recordComponentPolicy(acceptance designworkflow.AcceptanceLevel) {
	var preferences []string
	for _, key := range sortedStringKeys(builder.workflow.Components.PackagePreferences) {
		preferences = append(preferences, key+":"+builder.workflow.Components.PackagePreferences[key])
	}
	message := "component policy derived from intent: confidence=" + string(builder.workflow.Components.MinimumConfidence) + ", acceptance=" + string(builder.workflow.Components.Acceptance)
	if len(preferences) > 0 {
		message += ", packages=" + strings.Join(preferences, ",")
	}
	if ratings := builder.requiredRatingStrings(); len(ratings) > 0 {
		message += ", ratings=" + strings.Join(ratings, ",")
	}
	builder.plan.Assumptions = append(builder.plan.Assumptions, PlanNote{ID: "constraints.component_policy", Path: "constraints", Message: message})
	if builder.request.Manufacturing.Profile != "" {
		builder.plan.KnownGaps = append(builder.plan.KnownGaps, PlanNote{ID: "manufacturing.profile", Path: "manufacturing.profile", Message: "manufacturing profile " + builder.request.Manufacturing.Profile + " is captured in the plan; the current design workflow has no dedicated manufacturing profile field"})
	}
	if acceptance == designworkflow.AcceptanceFabricationCandidate {
		builder.plan.Assumptions = append(builder.plan.Assumptions, PlanNote{ID: "manufacturing.fabrication_candidate", Path: "manufacturing.fabrication_candidate", Message: "fabrication-candidate intent requires verified component confidence and ERC/DRC evidence"})
	}
}

func (builder *planBuilder) requiredRatingStrings() []string {
	var ratings []string
	for _, input := range builder.request.Power.Inputs {
		if input.Voltage != "" {
			ratings = append(ratings, "input_voltage:"+input.Voltage)
		}
		if input.CurrentMA > 0 {
			ratings = append(ratings, fmt.Sprintf("input_current:%gmA", input.CurrentMA))
		}
	}
	for _, rail := range builder.request.Power.Rails {
		if rail.Voltage != "" {
			ratings = append(ratings, "rail_voltage:"+rail.Voltage)
		}
		if rail.CurrentMA > 0 {
			ratings = append(ratings, fmt.Sprintf("rail_current:%gmA", rail.CurrentMA))
		}
	}
	return ratings
}

func (builder *planBuilder) mapPower() {
	for index, input := range builder.request.Power.Inputs {
		reqID := fmt.Sprintf("power.input.%d", index+1)
		builder.addRequirement(RequirementRecord{ID: reqID, Path: fmt.Sprintf("power.inputs[%d]", index), Type: "power_input", Strength: input.Strength, Value: input.Kind, Implementation: input.Kind})
		switch input.Kind {
		case "usb_c":
			id := builder.addBlock(reqID, "usb_power", "usb_c_power", map[string]any{}, "USB-C power input satisfies power input requirement")
			builder.usbPowerIDs = appendIfNotEmpty(builder.usbPowerIDs, id)
			if id != "" {
				builder.instanceVoltages[id] = input.Voltage
				builder.orderedPowerInputs = append(builder.orderedPowerInputs, powerSource{id: id, port: "VBUS_OUT", voltage: input.Voltage})
			}
		case "header", "external":
			pins := []string{"VIN", "GND"}
			id := builder.addBlock(reqID, "power_header", "connector_breakout", map[string]any{"pin_names": pins}, "power header exposes external power input")
			builder.connectorIDs = appendIfNotEmpty(builder.connectorIDs, id)
			builder.inputPowerIDs = appendIfNotEmpty(builder.inputPowerIDs, id)
			if id != "" {
				builder.instanceVoltages[id] = input.Voltage
				builder.orderedPowerInputs = append(builder.orderedPowerInputs, powerSource{id: id, port: "VIN", voltage: input.Voltage})
			}
		default:
			builder.unsupportedRequirement(reqID, fmt.Sprintf("power.inputs[%d].kind", index), "unsupported power input kind "+input.Kind, input.Strength, "use usb_c, header, or external")
		}
	}
	for index, rail := range builder.request.Power.Rails {
		reqID := fmt.Sprintf("power.rail.%d", index+1)
		builder.addRequirement(RequirementRecord{ID: reqID, Path: fmt.Sprintf("power.rails[%d]", index), Type: "power_rail", Strength: rail.Strength, Value: rail.Name + ":" + rail.Voltage})
		if rail.Voltage == "" {
			continue
		}
		if needsRegulator(builder.request.Power.Inputs, rail) {
			params := map[string]any{"output_voltage": rail.Voltage}
			source, sourceOK, ambiguous := builder.powerSourceForRail(rail.Voltage)
			if sourceOK {
				params["input_voltage"] = source.voltage
				if ambiguous {
					builder.plan.Assumptions = append(builder.plan.Assumptions, PlanNote{ID: reqID + ".input_voltage.selected", Path: "power.inputs", Message: "selected the nearest compatible declared power input voltage for regulator input"})
				}
			} else {
				builder.addIssue(fmt.Sprintf("power.rails[%d].voltage", index), "no compatible input voltage can feed requested rail "+rail.Voltage, "add a suitable source voltage or explicit power topology")
				continue
			}
			id := builder.addBlock(reqID, "regulator", "voltage_regulator", params, "regulator creates requested rail "+rail.Name)
			builder.regulatorIDs = appendIfNotEmpty(builder.regulatorIDs, id)
			if id != "" {
				builder.regulatorSources[id] = source
			}
		}
	}
}

func (builder *planBuilder) mapFunctions() {
	for index, function := range builder.request.Functions {
		reqID := fmt.Sprintf("function.%d", index+1)
		builder.addRequirement(RequirementRecord{ID: reqID, Path: fmt.Sprintf("functions[%d]", index), Type: "function", Strength: function.Strength, Value: firstNonEmpty(function.Family, function.Kind)})
		if !supportedFunctionKind(function.Kind) {
			builder.unsupportedRequirement(reqID, fmt.Sprintf("functions[%d].kind", index), "unsupported function kind "+function.Kind, function.Strength, "choose a supported function family")
			continue
		}
		if function.Kind == "sensor" && function.Family != "" && function.Family != "i2c_sensor" {
			builder.unsupportedRequirement(reqID, fmt.Sprintf("functions[%d].family", index), "unsupported sensor family "+function.Family, function.Strength, "use i2c_sensor")
			continue
		}
		if function.Kind == "clock" && function.Family != "" && function.Family != "crystal_oscillator" && function.Family != "canned_oscillator" {
			builder.unsupportedRequirement(reqID, fmt.Sprintf("functions[%d].family", index), "unsupported clock family "+function.Family, function.Strength, "use crystal_oscillator or canned_oscillator")
			continue
		}
		for count := 0; count < function.Quantity; count++ {
			switch function.Kind {
			case "indicator":
				id := builder.addBlock(reqID, "indicator", "led_indicator", function.Params, "LED indicator implements visual status output")
				builder.ledIDs = appendIfNotEmpty(builder.ledIDs, id)
			case "sensor":
				id := builder.addBlock(reqID, "sensor", "i2c_sensor", function.Params, "I2C sensor block implements requested sensor function")
				builder.sensorIDs = appendIfNotEmpty(builder.sensorIDs, id)
			case "mcu":
				id := builder.addBlock(reqID, "mcu", "mcu_minimal", function.Params, "MCU minimal system implements requested controller")
				builder.mcuIDs = appendIfNotEmpty(builder.mcuIDs, id)
			case "amplifier":
				id := builder.addBlock(reqID, "amplifier", "opamp_gain_stage", function.Params, "op-amp gain stage implements requested amplifier")
				builder.amplifierIDs = appendIfNotEmpty(builder.amplifierIDs, id)
			case "regulator", "power":
				id := builder.addBlock(reqID, "regulator", "voltage_regulator", function.Params, "regulator block implements requested power conversion")
				builder.regulatorIDs = appendIfNotEmpty(builder.regulatorIDs, id)
			case "clock":
				blockID := "crystal_oscillator"
				if function.Family == "canned_oscillator" {
					blockID = "canned_oscillator"
				}
				id := builder.addBlock(reqID, "clock", blockID, function.Params, "clock block implements requested timing source")
				builder.clockIDs = appendIfNotEmpty(builder.clockIDs, id)
				if blockID == "canned_oscillator" {
					builder.poweredClockIDs = appendIfNotEmpty(builder.poweredClockIDs, id)
				}
			case "reset_programming":
				id := builder.addBlock(reqID, "programming", "reset_programming_header", function.Params, "reset/programming block implements requested debug interface")
				builder.programmingIDs = appendIfNotEmpty(builder.programmingIDs, id)
			case "connector":
				id := builder.addConnector(reqID, "connector", []string{"SIG", "VCC", "GND"}, function.Strength)
				builder.connectorIDs = appendIfNotEmpty(builder.connectorIDs, id)
				builder.gpioConnectorIDs = appendIfNotEmpty(builder.gpioConnectorIDs, id)
				builder.signalConnectorIDs = appendIfNotEmpty(builder.signalConnectorIDs, id)
			}
		}
	}
}

func (builder *planBuilder) mapInterfaces() {
	for index, iface := range builder.request.Interfaces {
		reqID := fmt.Sprintf("interface.%d", index+1)
		builder.addRequirement(RequirementRecord{ID: reqID, Path: fmt.Sprintf("interfaces[%d]", index), Type: "interface", Strength: iface.Strength, Value: iface.Kind})
		if !supportedInterfaceKind(iface.Kind) {
			builder.unsupportedRequirement(reqID, fmt.Sprintf("interfaces[%d].kind", index), "unsupported interface kind "+iface.Kind, iface.Strength, "use i2c, gpio, connector, or power")
			continue
		}
		for count := 0; count < iface.Quantity; count++ {
			switch iface.Kind {
			case "i2c":
				id := builder.addConnector(reqID, "i2c_connector", []string{"SDA", "SCL", "VCC", "GND"}, iface.Strength)
				builder.connectorIDs = appendIfNotEmpty(builder.connectorIDs, id)
				builder.i2cConnectorIDs = appendIfNotEmpty(builder.i2cConnectorIDs, id)
			case "gpio", "connector":
				pins := []string{"SIG", "VCC", "GND"}
				id := builder.addConnector(reqID, "connector", pins, iface.Strength)
				builder.connectorIDs = appendIfNotEmpty(builder.connectorIDs, id)
				builder.gpioConnectorIDs = appendIfNotEmpty(builder.gpioConnectorIDs, id)
				builder.signalConnectorIDs = appendIfNotEmpty(builder.signalConnectorIDs, id)
			case "power":
				id := builder.addConnector(reqID, "power_connector", []string{"VCC", "GND"}, iface.Strength)
				builder.connectorIDs = appendIfNotEmpty(builder.connectorIDs, id)
				builder.powerConnectorIDs = appendIfNotEmpty(builder.powerConnectorIDs, id)
			}
		}
	}
}

func (builder *planBuilder) mapProtection() {
	if builder.request.Protection.ESD == StrengthPreferred || builder.request.Protection.ESD == StrengthRequired {
		reqID := "protection.esd"
		builder.addRequirement(RequirementRecord{ID: reqID, Path: "protection.esd", Type: "protection", Strength: builder.request.Protection.ESD, Value: "esd"})
		id := builder.addBlock(reqID, "esd", "esd_protection", map[string]any{}, "ESD protection requested for exposed interface")
		builder.esdIDs = appendIfNotEmpty(builder.esdIDs, id)
	}
	if builder.request.Protection.ReversePolarity == StrengthPreferred || builder.request.Protection.ReversePolarity == StrengthRequired {
		reqID := "protection.reverse_polarity"
		builder.addRequirement(RequirementRecord{ID: reqID, Path: "protection.reverse_polarity", Type: "protection", Strength: builder.request.Protection.ReversePolarity, Value: "reverse_polarity"})
		if len(builder.orderedPowerInputs) == 0 {
			id := builder.addBlock(reqID, "reverse_polarity", "reverse_polarity_protection", map[string]any{}, "reverse-polarity protection requested for power input")
			if id != "" {
				builder.reversePolarityIDs = append(builder.reversePolarityIDs, id)
			}
			return
		}
		for index, source := range builder.orderedPowerInputs {
			id := builder.addBlock(reqID, "reverse_polarity", "reverse_polarity_protection", map[string]any{"input_voltage": source.voltage}, "reverse-polarity protection requested for power input")
			if id != "" {
				builder.reversePolarityIDs = append(builder.reversePolarityIDs, id)
				builder.instanceVoltages[id] = source.voltage
				builder.protectedSources[source.id] = id
				builder.plan.Assumptions = append(builder.plan.Assumptions, PlanNote{ID: fmt.Sprintf("%s.input.%d", reqID, index+1), Path: fmt.Sprintf("power.inputs[%d]", index), Message: "applied reverse-polarity protection to declared power input"})
			}
		}
	}
}

func (builder *planBuilder) connectPowerAndSignals() {
	for _, source := range builder.availablePowerSources() {
		reverseID := builder.protectedSources[source.id]
		if reverseID != "" {
			builder.addConnection(source.id+"."+source.port, reverseID+".VIN_RAW", builder.rawPowerNetAlias(reverseID), "raw input feeds reverse-polarity protection")
			builder.addConnection(source.id+".GND", reverseID+".GND", "GND", "reverse-polarity protection shares input ground")
		}
	}
	for _, regulatorID := range builder.regulatorIDs {
		regulatorSource, regulatorSourcePort := builder.regulatorSourceFor(regulatorID)
		if regulatorSource != "" {
			builder.addConnection(regulatorSource+"."+regulatorSourcePort, regulatorID+".VIN", rawSourceNetAlias(regulatorSource), "input source feeds the regulator input")
			builder.addConnection(regulatorSource+".GND", regulatorID+".GND", "GND", "input ground ties to regulator ground")
		}
	}
	for _, target := range builder.powerTargets() {
		supplySource, supplyPort := builder.supplySourceForTarget(target.id)
		if supplySource != "" && target.id != "" && target.id != supplySource {
			builder.addConnection(supplySource+"."+supplyPort, target.id+"."+target.port, builder.supplyNetAlias(supplySource), "supply rail feeds "+target.id)
		} else if target.id != "" {
			builder.addIssue("blocks."+target.id+".supply_voltage", "no compatible supply source found for "+target.id, "add a matching rail, regulator, or power input")
		}
	}
	groundSource := firstNonEmpty(firstID(builder.usbPowerIDs), firstID(builder.regulatorIDs), firstID(builder.connectorIDs))
	for _, target := range builder.groundTargets() {
		if groundSource != "" && target != "" && target != groundSource {
			builder.addConnection(groundSource+".GND", target+".GND", "GND", "shared ground")
		}
	}
	for index, sensorID := range builder.sensorIDs {
		i2cConnectorID := builder.i2cConnectorAt(index)
		if i2cConnectorID != "" {
			builder.addConnection(i2cConnectorID+".SDA", sensorID+".SDA", builder.busNetAlias(i2cConnectorID, "SDA"), "I2C data connects sensor to breakout connector")
			builder.addConnection(i2cConnectorID+".SCL", sensorID+".SCL", builder.busNetAlias(i2cConnectorID, "SCL"), "I2C clock connects sensor to breakout connector")
		}
	}
	if len(builder.mcuIDs) > 0 && len(builder.i2cConnectorIDs) > 0 {
		builder.plan.KnownGaps = append(builder.plan.KnownGaps, PlanNote{ID: "mcu.i2c.pin_assignment", Path: "functions", Message: "MCU I2C bus wiring is deferred until the MCU block exposes distinct SDA/SCL-capable ports"})
	}
	for index, ledID := range builder.ledIDs {
		if gpioConnectorID := builder.signalConnectorAt(index); gpioConnectorID != "" {
			builder.addConnection(gpioConnectorID+".SIG", ledID+".IN", signalNetAlias("LED_SIG", ledID), "connector signal drives LED indicator")
		} else {
			builder.signalConnectorGap(ledID)
		}
	}
	for index, amplifierID := range builder.amplifierIDs {
		if gpioConnectorID := builder.signalConnectorAt(index); gpioConnectorID != "" {
			builder.addConnection(gpioConnectorID+".SIG", amplifierID+".IN", signalNetAlias("AMP_IN", amplifierID), "connector signal feeds amplifier input")
		} else {
			builder.signalConnectorGap(amplifierID)
		}
	}
	if builder.supportTargetingIsAmbiguous() {
		builder.addIssue("functions", "multiple MCUs require explicit clock/programming target mapping before support blocks can be wired", "split the design into one MCU per request or add explicit target metadata")
	} else {
		builder.connectMCUSupportBlocks()
	}
	for index, esdID := range builder.esdIDs {
		if gpioConnectorID := builder.signalConnectorAt(index); gpioConnectorID != "" {
			builder.addConnection(gpioConnectorID+".SIG", esdID+".SIGNAL", signalNetAlias("PROTECTED_SIG", esdID), "ESD protector shunts exposed connector signal")
		} else {
			builder.signalConnectorGap(esdID)
		}
	}
}

func (builder *planBuilder) signalConnectorGap(instanceID string) {
	builder.plan.KnownGaps = append(builder.plan.KnownGaps, PlanNote{ID: "signal_connector." + normalizeToken(instanceID), Path: "interfaces", Message: "signal consumer " + instanceID + " was not connected because no compatible SIG connector was available"})
}

func (builder *planBuilder) supportTargetingIsAmbiguous() bool {
	return len(builder.mcuIDs) > 1 && (len(builder.clockIDs) > 0 || len(builder.programmingIDs) > 0)
}

func (builder *planBuilder) connectMCUSupportBlocks() {
	if len(builder.clockIDs) > 0 && len(builder.mcuIDs) > 0 {
		builder.plan.KnownGaps = append(builder.plan.KnownGaps, PlanNote{ID: "mcu.clock.pin_assignment", Path: "functions", Message: "MCU clock wiring is deferred until MCU block metadata exposes clock-capable target ports"})
	}
	if len(builder.programmingIDs) > 0 && len(builder.mcuIDs) > 0 {
		builder.plan.KnownGaps = append(builder.plan.KnownGaps, PlanNote{ID: "mcu.programming.pin_assignment", Path: "functions", Message: "MCU programming wiring is deferred until MCU block metadata exposes debug/programming target ports"})
	}
}

func (builder *planBuilder) powerSourceForRail(railVoltage string) (powerSource, bool, bool) {
	target, targetOK := parseVoltage(railVoltage)
	if !targetOK {
		return powerSource{}, false, len(builder.availablePowerSources()) > 1
	}
	var compatible []struct {
		source powerSource
		value  float64
	}
	for _, source := range builder.orderedPowerInputs {
		value, ok := parseVoltage(source.voltage)
		if ok && value+voltageCompareEpsilon >= target {
			compatible = append(compatible, struct {
				source powerSource
				value  float64
			}{source: source, value: value})
		}
	}
	if len(compatible) == 0 {
		return powerSource{}, false, len(builder.orderedPowerInputs) > 1
	}
	best := compatible[0]
	for _, candidate := range compatible[1:] {
		if candidate.value < best.value {
			best = candidate
		}
	}
	return best.source, true, len(compatible) > 1
}

func (builder *planBuilder) availablePowerSources() []powerSource {
	sources := append([]powerSource(nil), builder.orderedPowerInputs...)
	for _, regulatorID := range builder.regulatorIDs {
		if voltage := builder.paramString(regulatorID, "output_voltage"); voltage != "" {
			sources = append(sources, powerSource{id: regulatorID, port: "VOUT", voltage: voltage})
		}
	}
	return sources
}

func (builder *planBuilder) protectedSourceFor(rawSourceID string) string {
	return strings.TrimSpace(builder.protectedSources[rawSourceID])
}

func (builder *planBuilder) rawPowerNetAlias(reverseID string) string {
	if len(builder.reversePolarityIDs) <= 1 {
		return "VIN_RAW"
	}
	return "VIN_RAW_" + normalizeToken(reverseID)
}

func (builder *planBuilder) regulatorSourceFor(regulatorID string) (string, string) {
	source, ok := builder.regulatorSources[regulatorID]
	if !ok {
		return "", ""
	}
	if protectedID := builder.protectedSourceFor(source.id); protectedID != "" {
		return protectedID, "VIN_PROTECTED"
	}
	return source.id, source.port
}

func (builder *planBuilder) supplySourceForTarget(targetID string) (string, string) {
	if voltage := builder.targetSupplyVoltage(targetID); voltage != "" {
		for _, source := range builder.regulatorIDs {
			if voltagesEquivalent(builder.paramString(source, "output_voltage"), voltage) {
				return source, "VOUT"
			}
		}
		for _, source := range builder.reversePolarityIDs {
			if voltagesEquivalent(builder.inputVoltageForInstance(source), voltage) {
				return source, "VIN_PROTECTED"
			}
		}
		if source, port := builder.rawPowerSourceForVoltage(voltage); source != "" {
			return source, port
		}
		return "", ""
	}
	return "", ""
}

func (builder *planBuilder) supplyNetAlias(sourceID string) string {
	if voltage := firstNonEmpty(builder.paramString(sourceID, "output_voltage"), builder.inputVoltageForInstance(sourceID)); voltage != "" {
		return "VCC_" + normalizeToken(strings.ReplaceAll(voltage, ".", "V"))
	}
	if sourceID != "" {
		return "VCC_" + normalizeToken(sourceID)
	}
	return "VCC"
}

func (builder *planBuilder) rawPowerSourceForVoltage(voltage string) (string, string) {
	for _, source := range []struct {
		ids  []string
		port string
	}{
		{ids: builder.usbPowerIDs, port: "VBUS_OUT"},
		{ids: builder.inputPowerIDs, port: "VIN"},
	} {
		for _, id := range source.ids {
			if voltagesEquivalent(builder.inputVoltageForInstance(id), voltage) {
				return id, source.port
			}
		}
	}
	return "", ""
}

func (builder *planBuilder) inputVoltageForInstance(instanceID string) string {
	return strings.TrimSpace(builder.instanceVoltages[instanceID])
}

func (builder *planBuilder) signalConnectorAt(index int) string {
	if index >= 0 && index < len(builder.signalConnectorIDs) {
		return strings.TrimSpace(builder.signalConnectorIDs[index])
	}
	if index == 0 {
		return firstID(builder.signalConnectorIDs)
	}
	return ""
}

func (builder *planBuilder) i2cConnectorAt(index int) string {
	if len(builder.i2cConnectorIDs) == 0 {
		return ""
	}
	if len(builder.i2cConnectorIDs) == 1 {
		return firstID(builder.i2cConnectorIDs)
	}
	if index < 0 {
		return ""
	}
	return strings.TrimSpace(builder.i2cConnectorIDs[index%len(builder.i2cConnectorIDs)])
}

func (builder *planBuilder) busNetAlias(connectorID string, signal string) string {
	if len(builder.i2cConnectorIDs) <= 1 {
		return signal
	}
	return normalizeToken(connectorID) + "_" + signal
}

func (builder *planBuilder) targetSupplyVoltage(targetID string) string {
	return builder.paramString(targetID, "supply_voltage")
}

func (builder *planBuilder) paramString(instanceID string, key string) string {
	params := builder.instanceParams[instanceID]
	if params != nil {
		if value, ok := params[key]; ok && value != nil {
			return strings.TrimSpace(fmt.Sprint(value))
		}
	}
	definition, ok := builder.registry.GetBlock(builder.instanceBlockIDs[instanceID])
	if !ok {
		return ""
	}
	for _, parameter := range definition.Parameters {
		if parameter.Name == key && parameter.Default != nil {
			return strings.TrimSpace(fmt.Sprint(parameter.Default))
		}
	}
	return ""
}

func (builder *planBuilder) powerTargets() []struct{ id, port string } {
	var targets []struct{ id, port string }
	for _, id := range builder.sensorIDs {
		targets = append(targets, struct{ id, port string }{id: id, port: builder.powerPortFor(id)})
	}
	for _, id := range builder.mcuIDs {
		targets = append(targets, struct{ id, port string }{id: id, port: builder.powerPortFor(id)})
	}
	for _, id := range builder.ledIDs {
		targets = append(targets, struct{ id, port string }{id: id, port: builder.powerPortFor(id)})
	}
	for _, id := range builder.amplifierIDs {
		targets = append(targets, struct{ id, port string }{id: id, port: builder.powerPortFor(id)})
	}
	for _, id := range builder.poweredClockIDs {
		targets = append(targets, struct{ id, port string }{id: id, port: builder.powerPortFor(id)})
	}
	for _, id := range builder.programmingIDs {
		targets = append(targets, struct{ id, port string }{id: id, port: builder.powerPortFor(id)})
	}
	for _, id := range builder.powerConnectorIDs {
		targets = append(targets, struct{ id, port string }{id: id, port: builder.powerPortFor(id)})
	}
	return targets
}

func (builder *planBuilder) powerPortFor(instanceID string) string {
	for _, pin := range stringListParam(builder.instanceParams[instanceID]["pin_names"]) {
		if strings.EqualFold(pin, "VCC") || strings.EqualFold(pin, "VDD") || strings.EqualFold(pin, "VIN") {
			return pin
		}
	}
	definition, ok := builder.registry.GetBlock(builder.instanceBlockIDs[instanceID])
	if !ok {
		return "VCC"
	}
	for _, preferred := range []string{"VCC", "VDD", "VIN"} {
		for _, port := range definition.Ports {
			if strings.EqualFold(port.Name, preferred) {
				return port.Name
			}
		}
	}
	for _, port := range definition.Ports {
		if port.Direction == blocks.PortPower && !strings.EqualFold(port.Name, "GND") {
			return port.Name
		}
	}
	return "VCC"
}

func (builder *planBuilder) groundTargets() []string {
	var targets []string
	targets = append(targets, builder.usbPowerIDs...)
	targets = append(targets, builder.connectorIDs...)
	targets = append(targets, builder.sensorIDs...)
	targets = append(targets, builder.mcuIDs...)
	targets = append(targets, builder.ledIDs...)
	targets = append(targets, builder.regulatorIDs...)
	targets = append(targets, builder.amplifierIDs...)
	targets = append(targets, builder.clockIDs...)
	targets = append(targets, builder.programmingIDs...)
	targets = append(targets, builder.esdIDs...)
	targets = append(targets, builder.reversePolarityIDs...)
	return targets
}

func (builder *planBuilder) addConnector(reqID string, prefix string, pins []string, strength Strength) string {
	if strength == StrengthForbidden {
		builder.plan.KnownGaps = append(builder.plan.KnownGaps, PlanNote{ID: reqID + ".forbidden", Path: reqID, Message: "connector requirement was forbidden and omitted"})
		return ""
	}
	return builder.addBlock(reqID, prefix, "connector_breakout", map[string]any{"pin_names": pins}, "connector breakout exposes requested interface")
}

func (builder *planBuilder) addBlock(reqID string, prefix string, blockID string, params map[string]any, rationale string) string {
	definition, ok := builder.registry.GetBlock(blockID)
	if !ok {
		builder.addIssue("blocks."+blockID, "unknown block ID "+blockID, "choose a supported built-in block")
		return ""
	}
	id := builder.nextID(prefix)
	clonedParams := cloneParams(params)
	builder.instanceBlockIDs[id] = blockID
	builder.instanceParams[id] = clonedParams
	builder.workflow.Blocks = append(builder.workflow.Blocks, designworkflow.BlockInstanceSpec{ID: id, BlockID: blockID, Params: clonedParams})
	record := SelectedBlockRecord{
		RequirementIDs: []string{reqID},
		InstanceID:     id,
		BlockID:        blockID,
		Params:         cloneParams(clonedParams),
		Rationale:      rationale,
	}
	record.Readiness = definition.Category
	record.Verification = string(definition.Verification.Level)
	if definition.PCBRealization != nil {
		record.RequiredRoutes = append(record.RequiredRoutes, definition.PCBRealization.Validation.RequiredRoutes...)
	}
	for _, rule := range definition.ValidationRules {
		if rule.Severity == blocks.BlockValidationSeverityBlocked {
			record.KnownGaps = append(record.KnownGaps, rule.ID)
		}
	}
	builder.plan.SelectedBlocks = append(builder.plan.SelectedBlocks, record)
	return id
}

func (builder *planBuilder) addRequirement(record RequirementRecord) {
	builder.plan.Requirements = append(builder.plan.Requirements, record)
}

func (builder *planBuilder) addConnection(from string, to string, net string, rationale string) {
	builder.workflow.Connections = append(builder.workflow.Connections, designworkflow.ConnectionSpec{From: from, To: to, NetAlias: net})
	builder.plan.Connections = append(builder.plan.Connections, ConnectionRecord{From: from, To: to, NetAlias: net, Rationale: rationale})
}

func (builder *planBuilder) unsupportedRequirement(id string, path string, message string, strength Strength, suggestion string) {
	note := PlanNote{ID: id + ".unsupported", Path: path, Message: message, Severity: reports.SeverityWarning, Suggestion: suggestion}
	if strength == StrengthRequired {
		builder.plan.Clarifications = append(builder.plan.Clarifications, note)
		builder.addIssue(path, message, suggestion)
		return
	}
	builder.plan.KnownGaps = append(builder.plan.KnownGaps, note)
}

func (builder *planBuilder) addIssue(path string, message string, suggestion string) {
	builder.plan.Issues = append(builder.plan.Issues, reports.Issue{Code: reports.CodeUnsupportedOperation, Severity: reports.SeverityError, Path: path, Message: message, Suggestion: suggestion})
}

func (builder *planBuilder) nextID(prefix string) string {
	prefix = normalizeToken(prefix)
	if prefix == "" {
		prefix = "block"
	}
	for {
		builder.ids[prefix]++
		id := prefix
		if builder.ids[prefix] > 1 {
			id = fmt.Sprintf("%s_%d", prefix, builder.ids[prefix])
		}
		if !builder.usedIDs[id] {
			builder.usedIDs[id] = true
			return id
		}
	}
}

func needsRegulator(inputs []PowerInputIntent, rail PowerRailIntent) bool {
	railVoltage, railOK := parseVoltage(rail.Voltage)
	if !railOK {
		return true
	}
	for _, input := range inputs {
		sourceVoltage, sourceOK := parseVoltage(input.Voltage)
		if sourceOK && math.Abs(sourceVoltage-railVoltage) <= voltageCompareEpsilon {
			return false
		}
	}
	return true
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func supportedFunctionKind(kind string) bool {
	switch kind {
	case "indicator", "sensor", "mcu", "amplifier", "regulator", "power", "clock", "reset_programming", "connector":
		return true
	default:
		return false
	}
}

func supportedInterfaceKind(kind string) bool {
	switch kind {
	case "i2c", "gpio", "connector", "power":
		return true
	default:
		return false
	}
}

func appendIfNotEmpty(values []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return values
	}
	return append(values, value)
}

func firstID(values []string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func stringListParam(value any) []string {
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		var values []string
		for _, item := range typed {
			if text := strings.TrimSpace(fmt.Sprint(item)); text != "" {
				values = append(values, text)
			}
		}
		return values
	default:
		return nil
	}
}

func cloneStringMap(values map[string]string) map[string]string {
	out := make(map[string]string, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

func sortedStringKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func voltagesEquivalent(left string, right string) bool {
	leftVoltage, leftOK := parseVoltage(left)
	rightVoltage, rightOK := parseVoltage(right)
	if !leftOK || !rightOK {
		return strings.EqualFold(strings.TrimSpace(left), strings.TrimSpace(right))
	}
	return math.Abs(leftVoltage-rightVoltage) <= voltageCompareEpsilon
}

func signalNetAlias(prefix string, instanceID string) string {
	instanceID = normalizeToken(instanceID)
	if instanceID == "" {
		return prefix
	}
	return prefix + "_" + instanceID
}

func rawSourceNetAlias(sourceID string) string {
	sourceID = normalizeToken(sourceID)
	if sourceID == "" {
		return "VIN_RAW"
	}
	return "VIN_RAW_" + sourceID
}
