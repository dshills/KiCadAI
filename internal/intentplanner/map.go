package intentplanner

import (
	"context"
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

func PlanContext(ctx context.Context, request Request) (PlanResult, error) {
	if err := ctx.Err(); err != nil {
		return PlanResult{}, err
	}
	plan := Plan(request)
	if err := ctx.Err(); err != nil {
		return PlanResult{}, err
	}
	return plan, nil
}

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
			Version:             designworkflow.RequestVersion,
			Name:                normalized.Name,
			SchematicLayout:     normalized.SchematicLayout,
			AutoSchematicLayout: normalized.AutoSchematicLayout,
			Intent: designworkflow.Intent{
				Summary:  normalized.Summary,
				Category: string(normalized.Kind),
			},
			Board: designworkflow.BoardSpec{
				WidthMM:         normalized.Board.WidthMM,
				HeightMM:        normalized.Board.HeightMM,
				EdgeClearanceMM: normalized.Board.EdgeClearanceMM,
				Layers:          normalized.Board.Layers,
			},
			Validation: designworkflow.ValidationSpec{
				Acceptance: normalized.Acceptance,
			},
		},
		ids:                map[string]int{},
		usedIDs:            map[string]bool{},
		instanceBlockIDs:   map[string]string{},
		instanceParams:     map[string]map[string]any{},
		instanceVoltages:   map[string]string{},
		regulatorSources:   map[string]powerSource{},
		protectedSources:   map[string]string{},
		semantic:           newSemanticIndex(),
		supportTargets:     map[string]semanticSupportIntent{},
		i2cBuses:           map[string]string{},
		i2cMCUBus:          map[string]string{},
		instanceReqIDs:     map[string]string{},
		instanceSupplies:   map[string]string{},
		railAliasVoltage:   map[string]string{},
		requirementIndex:   map[string]int{},
		selectedBlockIndex: map[string]int{},
		workflowBlockIndex: map[string]int{},
	}
	builder.applyBoardDefaults()
	builder.applyPolicyDefaults()
	builder.mapPower()
	builder.mapFunctions()
	builder.mapInterfaces()
	builder.mapProtection()
	builder.connectPowerAndSignals()
	builder.applyCalculatedValueApplications()
	if len(builder.workflow.Blocks) == 0 && !reports.HasBlockingIssue(builder.plan.Issues) {
		builder.addIssue("intent", "intent did not map to any supported circuit blocks", "choose a supported function, interface, power input, or protection requirement")
	}
	workflow := designworkflow.NormalizeRequest(builder.workflow)
	builder.plan.GeneratedRequest = &workflow
	builder.plan.Issues = append(builder.plan.Issues, designworkflow.ValidateRequest(workflow)...)
	builder.finalizeSynthesisTrace()
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
	semantic           *semanticIndex
	supportTargets     map[string]semanticSupportIntent
	i2cBuses           map[string]string
	i2cMCUBus          map[string]string
	instanceReqIDs     map[string]string
	instanceSupplies   map[string]string
	railAliasVoltage   map[string]string
	requirementIndex   map[string]int
	selectedBlockIndex map[string]int
	workflowBlockIndex map[string]int
	i2cDefaultNoted    bool
	i2cMultiBusBlocked bool
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
	classABOutputIDs   []string
	clockIDs           []string
	poweredClockIDs    []string
	programmingIDs     []string
	esdIDs             []string
	reversePolarityIDs []string
}

const (
	ap2112kPlannerOutputVoltage = 3.3
	ap2112kPlannerMaxInputV     = 6.0
	ap2112kPlannerMaxCurrentA   = 0.15
	defaultRegulatorCurrentA    = 0.25
	voltageComparisonEpsilon    = 0.001
	currentComparisonEpsilon    = 1e-9
)

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
	if builder.request.Constraints.AllowBackLayer != nil {
		builder.workflow.Constraints.AllowBackLayer = *builder.request.Constraints.AllowBackLayer
	} else if builder.workflow.Board.Layers > 1 {
		builder.workflow.Constraints.AllowBackLayer = true
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
	if acceptance == designworkflow.AcceptanceDraft || acceptance == designworkflow.AcceptanceStructural {
		return designworkflow.RoutingRetryPolicySpec{}
	}
	policy := designworkflow.RoutingRetryPolicySpec{
		Enabled:              true,
		MaxAttempts:          2,
		MinRoutingScoreDelta: 0.01,
		DRCPolicy:            designworkflow.RetryDRCPolicyOptional,
		StopOnNewBlockers:    true,
	}
	if acceptance == designworkflow.AcceptanceFabricationCandidate {
		policy.PreserveFixed = true
	}
	return policy
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
			id := builder.addBlock(reqID, "usb_power", "usb_c_power", map[string]any{
				"include_bulk_capacitor": false,
				"include_fuse":           false,
				"include_power_led":      false,
				"include_tvs":            false,
				"shield_policy":          "floating",
			}, "USB-C power input satisfies power input requirement")
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
		if rail.Name != "" {
			builder.railAliasVoltage[normalizeToken(rail.Name)] = rail.Voltage
		}
		if rail.Alias != "" {
			builder.railAliasVoltage[normalizeToken(rail.Alias)] = rail.Voltage
		}
		for _, target := range rail.SuppliedTargets {
			builder.plan.Assumptions = append(builder.plan.Assumptions, PlanNote{ID: reqID + ".supplied_targets." + firstNonEmpty(target.ID, target.Role), Path: fmt.Sprintf("power.rails[%d].supplied_targets", index), Message: "rail " + rail.Name + " explicitly supplies target " + firstNonEmpty(target.ID, target.Role)})
		}
		if needsRegulator(builder.request.Power.Inputs, rail) {
			params := map[string]any{"output_voltage": rail.Voltage}
			if rail.CurrentMA > 0 {
				params["output_current"] = formatScaledLiteral(rail.CurrentMA/1000.0) + "A"
			}
			source, sourceOK, ambiguous := builder.powerSourceForRail(rail.Voltage)
			if sourceOK {
				params["input_voltage"] = source.voltage
				for key, value := range regulatorVariantParams(source.voltage, rail.Voltage, rail.CurrentMA) {
					params[key] = value
				}
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

func regulatorVariantParams(inputVoltage string, outputVoltage string, currentMA float64) map[string]any {
	input, inputOK := parseVoltage(inputVoltage)
	output, outputOK := parseVoltage(outputVoltage)
	if !inputOK || !outputOK {
		return nil
	}
	currentA := currentMA / 1000.0
	if currentA <= 0 {
		currentA = defaultRegulatorCurrentA
	}
	if almostEqualVoltage(output, ap2112kPlannerOutputVoltage) &&
		input <= ap2112kPlannerMaxInputV+voltageComparisonEpsilon &&
		currentA <= ap2112kPlannerMaxCurrentA+currentComparisonEpsilon {
		return map[string]any{
			"regulator_symbol":    "Regulator_Linear:AP2112K-3.3",
			"regulator_footprint": "Package_TO_SOT_SMD:SOT-23-5",
			"input_voltage_min":   inputVoltage,
			"input_voltage_max":   inputVoltage,
			"enable_mode":         "tied_input",
		}
	}
	return nil
}

func almostEqualVoltage(a float64, b float64) bool {
	if a > b {
		return a-b < voltageComparisonEpsilon
	}
	return b-a < voltageComparisonEpsilon
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
		if function.Kind == "amplifier" && !supportedAmplifierFamily(function.Family) {
			builder.unsupportedRequirement(reqID, fmt.Sprintf("functions[%d].family", index), "unsupported amplifier family "+function.Family, function.Strength, "use op_amp_gain_stage, opamp_gain_stage, class_a_headphone, class_ab_headphone, or opamp_buffer_headphone")
			continue
		}
		if function.Kind == "amplifier" {
			builder.recordAmplifierFamilyGap(reqID, fmt.Sprintf("functions[%d].family", index), function.Family)
		}
		for count := 0; count < function.Quantity; count++ {
			switch function.Kind {
			case "indicator":
				id := builder.addBlock(reqID, "indicator", "led_indicator", function.Params, "LED indicator implements visual status output")
				builder.ledIDs = appendIfNotEmpty(builder.ledIDs, id)
			case "sensor":
				id := builder.addBlock(reqID, "sensor", "i2c_sensor", function.Params, "I2C sensor block implements requested sensor function")
				builder.sensorIDs = appendIfNotEmpty(builder.sensorIDs, id)
				builder.recordI2CBus(id, function.Bus)
				builder.recordInstanceSupply(id, function.Supply)
			case "mcu":
				blockID := "mcu_minimal"
				evidence := "MCU minimal system implements requested controller"
				if normalizeToken(function.Family) == "esp32" {
					blockID = "esp32_wroom_32e_minimal"
					evidence = "Reviewed ESP32-WROOM-32E minimal system implements requested ESP32 controller"
				}
				id := builder.addBlock(reqID, "mcu", blockID, function.Params, evidence)
				builder.mcuIDs = appendIfNotEmpty(builder.mcuIDs, id)
				builder.recordInstanceSupply(id, function.Supply)
			case "amplifier":
				if normalizeToken(function.Family) == "class_ab_headphone" {
					builder.mapClassABHeadphoneAmplifier(reqID, fmt.Sprintf("functions[%d]", index), function)
					continue
				}
				id := builder.addBlock(reqID, "amplifier", "opamp_gain_stage", function.Params, "op-amp gain stage implements requested amplifier")
				builder.amplifierIDs = appendIfNotEmpty(builder.amplifierIDs, id)
				builder.recordInstanceSupply(id, function.Supply)
				builder.recordAmplifierInstanceGap(reqID, id)
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
				builder.recordSupportTarget(id, reqID, fmt.Sprintf("functions[%d].target", index), function.Target, function.Strength)
				builder.recordInstanceSupply(id, function.Supply)
				if blockID == "canned_oscillator" {
					builder.poweredClockIDs = appendIfNotEmpty(builder.poweredClockIDs, id)
				}
			case "reset_programming":
				id := builder.addBlock(reqID, "programming", "reset_programming_header", function.Params, "reset/programming block implements requested debug interface")
				builder.programmingIDs = appendIfNotEmpty(builder.programmingIDs, id)
				builder.recordSupportTarget(id, reqID, fmt.Sprintf("functions[%d].target", index), function.Target, function.Strength)
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
			builder.unsupportedRequirement(reqID, fmt.Sprintf("interfaces[%d].kind", index), "unsupported interface kind "+iface.Kind, iface.Strength, "use i2c, gpio, analog, connector, or power")
			continue
		}
		for count := 0; count < iface.Quantity; count++ {
			switch iface.Kind {
			case "i2c":
				id := builder.addConnector(reqID, "i2c_connector", []string{"VCC", "GND", "SDA", "SCL"}, iface.Strength)
				builder.connectorIDs = appendIfNotEmpty(builder.connectorIDs, id)
				builder.i2cConnectorIDs = appendIfNotEmpty(builder.i2cConnectorIDs, id)
				if id != "" && iface.Voltage != "" {
					builder.instanceVoltages[id] = iface.Voltage
				}
				builder.recordI2CBus(id, iface.Bus)
			case "analog":
				pins := interfaceSignalConnectorPins(iface)
				id := builder.addInterfaceConnector(reqID, "connector", pins, iface)
				builder.connectorIDs = appendIfNotEmpty(builder.connectorIDs, id)
				builder.signalConnectorIDs = appendIfNotEmpty(builder.signalConnectorIDs, id)
			case "gpio", "connector":
				pins := interfaceSignalConnectorPins(iface)
				if iface.Kind == "gpio" && len(pins) == 2 {
					pins = []string{"SIG", "VCC", "GND"}
				}
				id := builder.addInterfaceConnector(reqID, "connector", pins, iface)
				builder.connectorIDs = appendIfNotEmpty(builder.connectorIDs, id)
				if iface.Kind == "gpio" {
					builder.gpioConnectorIDs = appendIfNotEmpty(builder.gpioConnectorIDs, id)
				}
				builder.signalConnectorIDs = appendIfNotEmpty(builder.signalConnectorIDs, id)
				if iface.Kind == "gpio" && (iface.Target.ID != "" || iface.Target.Role != "") {
					builder.unsupportedRequirement(reqID+".target", fmt.Sprintf("interfaces[%d].target", index), "GPIO target pin assignment is not safely synthesized yet", iface.Strength, "omit target metadata for a generic connector or add a verified GPIO assignment model")
				}
			case "power":
				id := builder.addInterfaceConnector(reqID, "power_connector", []string{"VCC", "GND"}, iface)
				builder.connectorIDs = appendIfNotEmpty(builder.connectorIDs, id)
				builder.powerConnectorIDs = appendIfNotEmpty(builder.powerConnectorIDs, id)
			}
		}
	}
}

func (builder *planBuilder) mapProtection() {
	builder.mapUSBPowerProtection("overcurrent", builder.request.Protection.Overcurrent, "include_fuse", "input overcurrent protection")
	builder.mapUSBPowerProtection("transient", builder.request.Protection.Transient, "include_tvs", "input transient protection")
	builder.mapUSBPowerProtection("bulk_capacitance", builder.request.Protection.BulkCapacitance, "include_bulk_capacitor", "input bulk capacitance")
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

func (builder *planBuilder) mapUSBPowerProtection(name string, strength Strength, param string, description string) {
	if strength == StrengthOptional {
		return
	}
	reqID := "protection." + name
	path := reqID
	enabled := strength == StrengthRequired || strength == StrengthPreferred
	record := RequirementRecord{
		ID:       reqID,
		Path:     path,
		Type:     "protection",
		Strength: strength,
		Value:    name,
		Evidence: []string{fmt.Sprintf("%s=%t", param, enabled)},
	}
	if len(builder.usbPowerIDs) == 0 {
		record.Implementation = "none"
		record.Evidence = []string{"no usb_c_power block selected"}
		builder.addRequirement(record)
		if enabled {
			builder.unsupportedRequirement(reqID, path, description+" requires a USB-C power input", strength, "add a usb_c power input or mark the protection requirement optional")
		}
		return
	}
	implementations := make([]string, 0, len(builder.usbPowerIDs))
	for _, instanceID := range builder.usbPowerIDs {
		implementations = append(implementations, instanceID+"."+param)
	}
	record.Implementation = strings.Join(implementations, ",")
	builder.addRequirement(record)
	for _, instanceID := range builder.usbPowerIDs {
		builder.updateWorkflowBlockParam(instanceID, param, enabled)
		if index, ok := builder.selectedBlockIndex[instanceID]; ok && index >= 0 && index < len(builder.plan.SelectedBlocks) {
			builder.plan.SelectedBlocks[index].RequirementIDs = appendUniqueString(builder.plan.SelectedBlocks[index].RequirementIDs, reqID)
		}
	}
	blockDescription := "usb_c_power block"
	if len(builder.usbPowerIDs) > 1 {
		blockDescription = "all usb_c_power blocks"
	}
	builder.plan.Assumptions = append(builder.plan.Assumptions, PlanNote{
		ID:      reqID + ".usb_c_power",
		Path:    path,
		Message: fmt.Sprintf("%s maps to %s %s=%t", description, blockDescription, param, enabled),
	})
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
		if builder.instanceBlockIDs[target.id] == "led_indicator" && !builder.ledIndicatorActiveHigh(target.id) && paramText(builder.instanceParams[target.id], "supply_voltage") == "" {
			if source, ok := builder.powerSourceForPoweredLED(target.id); ok {
				builder.applyInferredLEDDefaults(target.id, source.voltage)
			}
		}
		supplySource, supplyPort := builder.supplySourceForTarget(target.id)
		if supplySource != "" && target.id == supplySource {
			continue
		}
		if supplySource != "" && target.id != "" {
			netAlias := builder.supplyNetAlias(supplySource)
			builder.addConnection(supplySource+"."+supplyPort, target.id+"."+target.port, netAlias, "supply rail feeds "+target.id)
			builder.appendRequirementEvidenceForInstance(target.id, "supply:"+supplySource+"."+supplyPort)
			builder.appendRequirementEvidenceForInstance(target.id, "net:"+netAlias)
		} else if target.id != "" && builder.targetSupplyVoltage(target.id) != "" {
			builder.addIssue("blocks."+target.id+".supply_voltage", "no compatible supply source found for "+target.id, "add a matching rail, regulator, or power input")
		} else if target.id != "" {
			builder.plan.KnownGaps = append(builder.plan.KnownGaps, PlanNote{ID: "supply_voltage." + normalizeToken(target.id), Path: "blocks." + target.id, Message: "supply voltage for " + target.id + " is not explicit in the current block metadata"})
		}
	}
	groundSource := firstNonEmpty(firstID(builder.usbPowerIDs), firstID(builder.regulatorIDs), firstID(builder.connectorIDs))
	for _, target := range builder.groundTargets() {
		if groundSource != "" && target != "" && target != groundSource {
			builder.addConnection(groundSource+".GND", target+".GND", "GND", "shared ground")
		}
	}
	builder.connectI2CBuses()
	for index, ledID := range builder.ledIDs {
		if gpioConnectorID := builder.signalConnectorAt(index); gpioConnectorID != "" {
			builder.addConnection(gpioConnectorID+".SIG", ledID+".IN", signalNetAlias("LED_SIG", ledID), "connector signal drives LED indicator")
		} else if source, ok := builder.powerSourceForPoweredLED(ledID); ok {
			builder.connectPoweredLED(source, ledID)
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
	builder.connectMCUSupportBlocks()
	for index, esdID := range builder.esdIDs {
		if gpioConnectorID := builder.signalConnectorAt(index); gpioConnectorID != "" {
			builder.addConnection(gpioConnectorID+".SIG", esdID+".SIGNAL", signalNetAlias("PROTECTED_SIG", esdID), "ESD protector shunts exposed connector signal")
		} else {
			builder.signalConnectorGap(esdID)
		}
	}
}

func (builder *planBuilder) mapClassABHeadphoneAmplifier(reqID string, path string, function FunctionIntent) {
	if reason := unsupportedClassABHeadphoneIntent(function); reason != "" {
		builder.addIssue(path+".params", reason, "request a bounded headphone load with a supported load impedance and no speaker, bridge, or power-amplifier output requirement")
		return
	}
	amplifierSupply := function.Supply
	amplifierSupplyVoltage := firstNonEmpty(paramText(function.Params, "supply_voltage"), "9V")
	singleSupply := !classABHeadphoneRequestsBipolarSupply(function.Params)
	inputBufferID := builder.addBlock(reqID, "input_buffer", "amplifier_input_buffer", classABInputBufferParams(function.Params), "AC-coupled input buffer sets input impedance and biases the gain-stage input")
	gainID := builder.addBlock(reqID, "amplifier", "opamp_gain_stage", classABGainStageParams(function.Params, singleSupply), "op-amp gain stage provides requested voltage gain before the Class AB driver")
	decouplingID := builder.addBlock(reqID, "supply_decoupling", "amplifier_supply_decoupling", classABSupplyDecouplingParams(function.Params, amplifierSupplyVoltage, singleSupply), "local rail decoupling evidence supports the active gain and output stages")
	biasID := builder.addBlock(reqID, "bias", "amplifier_bias_network", classABBiasNetworkParams(function.Params), "two-diode Class AB bias string generates complementary output-pair bias nodes")
	outputParams := classABOutputPairParams(function.Params, amplifierSupplyVoltage)
	outputID := builder.addBlock(reqID, "output", "class_ab_output_pair", outputParams, "complementary emitter follower buffers the op-amp for headphone-class load drive")
	protectionID := builder.addBlock(reqID, "output_protection", "headphone_output_protection", headphoneOutputProtectionParams(function.Params), "AC-coupled headphone output protection captures load-safety assumptions")
	headphonesID := builder.addBlock(reqID, "headphones", "headphone_output_connector", map[string]any{"load_kind": "headphone"}, "headphone connector exposes protected output, return, and load reference")
	if inputBufferID == "" || gainID == "" || decouplingID == "" || biasID == "" || outputID == "" || protectionID == "" || headphonesID == "" {
		return
	}
	builder.amplifierIDs = appendIfNotEmpty(builder.amplifierIDs, inputBufferID)
	builder.classABOutputIDs = appendIfNotEmpty(builder.classABOutputIDs, outputID)
	builder.recordInstanceSupply(inputBufferID, amplifierSupply)
	builder.recordInstanceSupply(gainID, amplifierSupply)
	builder.recordInstanceSupply(decouplingID, amplifierSupply)
	builder.recordInstanceSupply(biasID, amplifierSupply)
	builder.recordInstanceSupply(outputID, amplifierSupply)
	builder.addConnection(inputBufferID+".OUT", gainID+".IN", signalNetAlias("AMP_IN_BIASED", gainID), "input buffer feeds the op-amp gain stage")
	builder.addConnection(gainID+".OUT", biasID+".DRIVER_OUT", signalNetAlias("AMP_DRIVER", biasID), "op-amp output drives Class AB bias string")
	builder.addConnection(biasID+".BIAS_P", outputID+".BIAS_P", signalNetAlias("BIAS_P", outputID), "upper bias node drives the NPN output device")
	builder.addConnection(biasID+".BIAS_N", outputID+".BIAS_N", signalNetAlias("BIAS_N", outputID), "lower bias node drives the PNP output device")
	builder.addConnection(biasID+".AMP_OUT", outputID+".AMP_OUT", signalNetAlias("AMP_OUT_DC_BIASED", protectionID), "bias network output anchor aligns with the output-pair emitter node")
	builder.addConnection(outputID+".AMP_OUT", protectionID+".AMP_OUT", signalNetAlias("AMP_OUT_DC_BIASED", protectionID), "Class AB output feeds AC-coupled headphone protection")
	builder.addConnection(protectionID+".HP_OUT", headphonesID+".HP_OUT", signalNetAlias("HP_OUT", headphonesID), "protected AC-coupled output feeds headphone connector")
	builder.addConnection(outputID+".LOAD_REF", protectionID+".LOAD_REF", "GND", "Class AB load reference feeds headphone output protection")
	builder.addConnection(protectionID+".LOAD_RET", headphonesID+".LOAD_RET", signalNetAlias("HP_RET", headphonesID), "headphone connector return is tracked separately from load reference")
	builder.addConnection(protectionID+".LOAD_REF", headphonesID+".LOAD_REF", "GND", "headphone connector carries explicit load reference")
	if groundSource := firstNonEmpty(firstID(builder.usbPowerIDs), firstID(builder.regulatorIDs), firstID(builder.connectorIDs)); groundSource != "" {
		groundPort := builder.groundPortFor(groundSource)
		builder.addConnection(groundSource+"."+groundPort, inputBufferID+".GND", "GND", "input buffer bias divider returns to ground")
		builder.addConnection(groundSource+"."+groundPort, gainID+".GND", "GND", "single-supply op-amp gain stage returns to ground")
		builder.addConnection(groundSource+"."+groundPort, decouplingID+".GND", "GND", "local amplifier decoupling returns to ground")
		if singleSupply {
			builder.addConnection(groundSource+"."+groundPort, biasID+".VEE", "GND", "single-supply Class AB bias lower rail returns to ground")
			builder.addConnection(groundSource+"."+groundPort, outputID+".VEE", "GND", "single-supply Class AB lower rail returns to ground")
		} else {
			builder.addIssue(path+".power.vee", "Class AB headphone mapping currently rejects bipolar supplies before composition", "use a single-supply headphone request until negative-rail mapping is implemented")
		}
		builder.addConnection(groundSource+"."+groundPort, outputID+".LOAD_REF", "GND", "headphone load reference is tied to the single-supply return")
	} else {
		builder.addIssue(path+".power.ground", "Class AB headphone output stage requires a ground source", "declare an external, USB, or regulated power source with ground")
	}
	if supplySource, supplyPort := builder.supplySourceForTarget(gainID); supplySource != "" {
		builder.addConnection(supplySource+"."+supplyPort, inputBufferID+".VCC", builder.supplyNetAlias(supplySource), "input buffer bias divider uses the amplifier rail")
		builder.addConnection(supplySource+"."+supplyPort, gainID+".VCC", builder.supplyNetAlias(supplySource), "op-amp gain stage uses the amplifier rail")
		builder.addConnection(supplySource+"."+supplyPort, decouplingID+".VCC", builder.supplyNetAlias(supplySource), "local decoupling is tied to the amplifier rail")
		builder.addConnection(supplySource+"."+supplyPort, biasID+".VCC", builder.supplyNetAlias(supplySource), "bias network upper rail uses the amplifier rail")
		builder.addConnection(supplySource+"."+supplyPort, outputID+".VCC", builder.supplyNetAlias(supplySource), "Class AB output pair uses the amplifier rail")
	}
	builder.recordAmplifierInstanceGap(reqID, gainID)
	builder.recordClassABHeadphoneChainGap(reqID, outputID, protectionID)
}

func (builder *planBuilder) recordClassABHeadphoneChainGap(reqID string, outputID string, protectionID string) {
	if outputID != "" {
		builder.plan.KnownGaps = append(builder.plan.KnownGaps, PlanNote{
			ID:         reqID + "." + normalizeToken(outputID) + ".class_ab_output_stage_review",
			Path:       "blocks." + outputID,
			Message:    "Class AB output-stage connectivity is supported for headphone-class intent, but thermal, quiescent-current, stability, and PCB-current evidence remain unverified",
			Suggestion: "keep acceptance at connectivity until KiCad ERC/DRC, thermal, and analog stability evidence are available",
		})
	}
	if protectionID != "" {
		builder.plan.KnownGaps = append(builder.plan.KnownGaps, PlanNote{
			ID:         reqID + "." + normalizeToken(protectionID) + ".headphone_output_protection_review",
			Path:       "blocks." + protectionID,
			Message:    "Headphone output protection captures AC coupling and return policy, but active fault protection and fabrication readiness remain unverified",
			Suggestion: "review output-protection diagnostics before promoting beyond connectivity",
		})
	}
}

func unsupportedClassABHeadphoneIntent(function FunctionIntent) string {
	params := function.Params
	loadKind := normalizeToken(paramText(params, "load_kind"))
	if loadKind == "" {
		loadKind = "headphone"
	}
	switch loadKind {
	case "headphone":
	case "speaker", "bridge", "power", "power_amplifier":
		return "Class AB headphone mapping does not support speaker, bridge, or power-amplifier loads"
	default:
		return "Class AB headphone mapping requires an explicit headphone load kind"
	}
	load := firstNonEmpty(paramText(params, "nominal_load_ohms"), paramText(params, "load_impedance"), "32Ω")
	if normalizeToken(load) == "unknown" {
		return "Class AB headphone mapping requires a known headphone load impedance"
	}
	if firstNonEmpty(paramText(params, "output_power_w"), paramText(params, "power_w"), paramText(params, "output_power")) != "" {
		return "Class AB headphone mapping does not support output-power requests yet"
	}
	if classABHeadphoneRequestsBipolarSupply(params) {
		return "Class AB headphone mapping currently supports only single-supply output stages"
	}
	return ""
}

func classABHeadphoneRequestsBipolarSupply(params map[string]any) bool {
	if value, ok := params["single_supply"].(bool); ok && !value {
		return true
	}
	supplyMode := normalizeToken(firstNonEmpty(paramText(params, "supply_mode"), paramText(params, "supply_topology")))
	if supplyMode == "dual" || supplyMode == "bipolar" || supplyMode == "split" || supplyMode == "dual_rail" || supplyMode == "split_rail" {
		return true
	}
	return firstNonEmpty(paramText(params, "negative_supply_voltage"), paramText(params, "vee"), paramText(params, "v-")) != ""
}

func classABInputBufferParams(params map[string]any) map[string]any {
	out := map[string]any{
		"input_impedance":      firstNonEmpty(paramText(params, "input_impedance"), "100kΩ"),
		"coupling_capacitance": firstNonEmpty(paramText(params, "input_coupling_capacitance"), paramText(params, "coupling_capacitance"), "1uF"),
		"input_stopper_value":  firstNonEmpty(paramText(params, "input_stopper_value"), "100Ω"),
	}
	return out
}

func classABGainStageParams(params map[string]any, singleSupply bool) map[string]any {
	out := cloneParams(params)
	out["topology"] = "non_inverting"
	out["gain"] = firstNonEmptyNumber(params["gain"], 2.0)
	out["single_supply"] = singleSupply
	out["input_coupling"] = "dc"
	out["include_output_resistor"] = true
	return out
}

func classABSupplyDecouplingParams(params map[string]any, railVoltage string, singleSupply bool) map[string]any {
	railMode := "single_supply"
	if !singleSupply {
		railMode = "dual_supply"
	}
	return map[string]any{
		"rail_mode":                railMode,
		"rail_voltage":             firstNonEmpty(railVoltage, "9V"),
		"ceramic_capacitance":      firstNonEmpty(paramText(params, "decoupling_capacitance"), "100nF"),
		"bulk_capacitance":         firstNonEmpty(paramText(params, "bulk_capacitance"), "10uF"),
		"include_bulk":             true,
		"capacitor_voltage_rating": firstNonEmpty(paramText(params, "decoupling_voltage_rating"), paramText(params, "capacitor_voltage_rating"), "16V"),
	}
}

func classABBiasNetworkParams(params map[string]any) map[string]any {
	out := map[string]any{
		"topology":                 "diode_string",
		"application":              "headphone",
		"diode_count":              firstNonEmptyNumber(params["diode_count"], 2.0),
		"emitter_resistor_value":   firstNonEmpty(paramText(params, "emitter_resistor_value"), "0.47Ω"),
		"bias_feed_resistor_value": firstNonEmpty(paramText(params, "bias_feed_resistor_value"), "10kΩ"),
		"thermal_coupling_policy":  firstNonEmpty(paramText(params, "thermal_coupling_policy"), "adjacent_to_output_pair"),
	}
	return out
}

func classABOutputPairParams(params map[string]any, supplyVoltage string) map[string]any {
	out := map[string]any{
		"supply_voltage":         firstNonEmpty(supplyVoltage, "9V"),
		"load_impedance":         headphoneLoadParam(params),
		"application":            "headphone",
		"emitter_resistor_value": firstNonEmpty(paramText(params, "emitter_resistor_value"), "0.47Ω"),
	}
	for _, key := range []string{"upper_output_component_id", "lower_output_component_id"} {
		if value, ok := params[key]; ok {
			out[key] = value
		}
	}
	return out
}

func firstNonEmptyNumber(value any, fallback float64) any {
	if value == nil {
		return fallback
	}
	if text := strings.TrimSpace(fmt.Sprint(value)); text != "" {
		return value
	}
	return fallback
}

func headphoneOutputProtectionParams(params map[string]any) map[string]any {
	out := map[string]any{
		"load_kind":               "headphone",
		"nominal_load_ohms":       headphoneLoadParam(params),
		"dc_blocking_capacitance": firstNonEmpty(paramText(params, "dc_blocking_capacitance"), "220uF"),
		"bleed_resistor_ohms":     firstNonEmpty(paramText(params, "bleed_resistor_ohms"), "100kΩ"),
		"connector_return_policy": firstNonEmpty(paramText(params, "connector_return_policy"), "load_ref"),
		"fault_protection_status": firstNonEmpty(paramText(params, "fault_protection_status"), "placeholder_blocked"),
	}
	for _, key := range []string{"coupling", "bleed_required", "series_resistor_ohms"} {
		if value, ok := params[key]; ok {
			out[key] = value
		}
	}
	return out
}

func headphoneLoadParam(params map[string]any) string {
	return firstNonEmpty(paramText(params, "load_impedance"), paramText(params, "nominal_load_ohms"), "32Ω")
}

func paramText(params map[string]any, key string) string {
	if params == nil {
		return ""
	}
	value, ok := params[key]
	if !ok || value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func (builder *planBuilder) signalConnectorGap(instanceID string) {
	builder.plan.KnownGaps = append(builder.plan.KnownGaps, PlanNote{ID: "signal_connector." + normalizeToken(instanceID), Path: "interfaces", Message: "signal consumer " + instanceID + " was not connected because no compatible SIG connector was available"})
}

type amplifierFamilyMetadata struct {
	GapSuffix  string
	Message    string
	Suggestion string
}

var amplifierFamilies = map[string]amplifierFamilyMetadata{
	"":                  {},
	"op_amp_gain_stage": {},
	"opamp_gain_stage":  {},
	"class_a_headphone": {
		GapSuffix:  "class_a_output_stage_unverified",
		Message:    "Class A headphone amplifier request maps to the current op-amp gain-stage foundation; quiescent bias, thermal behavior, and Class A output-stage realization are not yet verified",
		Suggestion: "use checked-in example fixtures for evaluation until Class A circuit blocks and PCB constraints are verified",
	},
	"class_ab_headphone": {
		GapSuffix:  "class_ab_output_stage_unverified",
		Message:    "Class AB headphone amplifier request maps to the current op-amp gain-stage foundation; complementary output-stage bias, stability, and load-drive behavior are not yet verified",
		Suggestion: "use checked-in example fixtures for evaluation until Class AB output-stage blocks and PCB constraints are verified",
	},
	"opamp_buffer_headphone": {
		GapSuffix:  "headphone_buffer_unverified",
		Message:    "op-amp headphone buffer request maps to the current op-amp gain-stage foundation; headphone load drive and output protection are not yet verified",
		Suggestion: "keep acceptance at structural or connectivity until load-drive and PCB evidence are available",
	},
}

func supportedAmplifierFamily(family string) bool {
	_, ok := amplifierFamilies[normalizeToken(family)]
	return ok
}

func (builder *planBuilder) recordAmplifierFamilyGap(reqID string, path string, family string) {
	metadata, ok := amplifierFamilies[normalizeToken(family)]
	if !ok {
		return
	}
	if metadata.GapSuffix != "" {
		builder.plan.KnownGaps = append(builder.plan.KnownGaps, PlanNote{
			ID:         reqID + "." + metadata.GapSuffix,
			Path:       path,
			Message:    metadata.Message,
			Suggestion: metadata.Suggestion,
		})
		if builder.request.Manufacturing.FabricationCandidate || builder.request.Acceptance == designworkflow.AcceptanceFabricationCandidate {
			builder.addIssue(path, "amplifier family "+normalizeToken(family)+" cannot be promoted to fabrication-candidate output yet", "complete verified circuit blocks, thermal constraints, and KiCad ERC/DRC evidence before requesting fabrication-candidate output")
		}
	}
}

func (builder *planBuilder) recordAmplifierInstanceGap(reqID string, instanceID string) {
	if instanceID == "" {
		return
	}
	builder.plan.KnownGaps = append(builder.plan.KnownGaps, PlanNote{
		ID:         reqID + "." + normalizeToken(instanceID) + ".opamp_gain_stage_layout_unverified",
		Path:       "blocks." + instanceID,
		Message:    "op-amp gain-stage schematic intent is supported, but analog layout, feedback stability, and ERC/DRC evidence remain unverified",
		Suggestion: "review generated artifacts and run KiCad-backed validation before treating output as fabrication-ready",
	})
}

func (builder *planBuilder) connectMCUSupportBlocks() {
	for _, clockID := range builder.clockIDs {
		target, ok := builder.resolveMCUSupportTarget(clockID, "mcu.clock.xtal1")
		if !ok {
			continue
		}
		builder.reportClockSupportLimitation(clockID, target)
	}
	for _, programmingID := range builder.programmingIDs {
		target, ok := builder.resolveMCUSupportTarget(programmingID, "mcu.reset")
		if !ok {
			continue
		}
		builder.connectProgrammingSupport(programmingID, target)
	}
}

func (builder *planBuilder) reportClockSupportLimitation(clockID string, target semanticInstance) {
	builder.recordSynthesisDecision(SynthesisDecision{
		ID:        "mcu.clock.topology." + normalizeToken(clockID),
		Type:      "unsupported_gap",
		Path:      builder.supportTargetPath(clockID),
		Selected:  "external_clock_blocked",
		Rationale: "clock target ports are known for " + target.ID + ", but the selected MCU block currently supports only internal clock topology",
	})
	builder.plan.KnownGaps = append(builder.plan.KnownGaps, PlanNote{
		ID:         "mcu.clock.topology_unsupported." + normalizeToken(clockID),
		Path:       builder.supportTargetPath(clockID),
		Message:    "clock target ports are known for " + target.ID + ", but the selected MCU block currently supports only internal clock topology",
		Suggestion: "keep clock_mode internal or add external-clock support to the MCU block before wiring this clock source",
	})
}

func (builder *planBuilder) connectProgrammingSupport(programmingID string, target semanticInstance) {
	support, ok := builder.semantic.instance(programmingID)
	if !ok {
		builder.addIssue(builder.supportTargetPath(programmingID), "programming support block "+programmingID+" is missing semantic metadata", "rebuild the plan with a supported programming block")
		return
	}
	mode := builder.programmingMode(programmingID)
	switch mode {
	case "isp":
		for _, pair := range []struct {
			targetRole  string
			supportRole string
			net         string
		}{
			{targetRole: "mcu.reset", supportRole: "mcu.reset", net: "ISP_RESET"},
			{targetRole: "mcu.spi.mosi", supportRole: "mcu.spi.mosi", net: "ISP_MOSI"},
			{targetRole: "mcu.spi.miso", supportRole: "mcu.spi.miso", net: "ISP_MISO"},
			{targetRole: "mcu.spi.sck", supportRole: "mcu.spi.sck", net: "ISP_SCK"},
			{targetRole: "power.vcc", supportRole: "power.vcc", net: builder.supplyNetAlias(target.ID)},
			{targetRole: "power.gnd", supportRole: "power.gnd", net: "GND"},
		} {
			builder.addSemanticProgrammingConnection(target, support, pair.targetRole, pair.supportRole, builder.programmingNetAlias(target.ID, programmingID, pair.targetRole, pair.net), "ISP programming connects "+programmingID+" to target "+target.ID)
		}
	case "uart":
		for _, pair := range []struct {
			targetRole  string
			supportRole string
			net         string
		}{
			{targetRole: "mcu.uart.tx", supportRole: "mcu.uart.rx", net: "UART_TX"},
			{targetRole: "mcu.uart.rx", supportRole: "mcu.uart.tx", net: "UART_RX"},
			{targetRole: "power.vcc", supportRole: "power.vcc", net: builder.supplyNetAlias(target.ID)},
			{targetRole: "power.gnd", supportRole: "power.gnd", net: "GND"},
		} {
			builder.addSemanticProgrammingConnection(target, support, pair.targetRole, pair.supportRole, builder.programmingNetAlias(target.ID, programmingID, pair.targetRole, pair.net), "UART programming connects "+programmingID+" to target "+target.ID)
		}
	default:
		builder.addIssue(builder.supportTargetPath(programmingID)+".programming_interface", "unsupported programming interface "+mode, "use isp or uart")
	}
}

func (builder *planBuilder) addSemanticProgrammingConnection(target semanticInstance, support semanticInstance, targetRole string, supportRole string, net string, rationale string) {
	targetPort, targetOK := target.portByRole(targetRole)
	supportPort, supportOK := support.portByRole(supportRole)
	if !targetOK {
		builder.addIssue(builder.supportTargetPath(support.ID), "target "+target.ID+" is missing programming role "+targetRole, "select a compatible MCU target")
		return
	}
	if !supportOK {
		builder.addIssue(builder.supportTargetPath(support.ID), "support block "+support.ID+" is missing programming role "+supportRole, "select a compatible programming support block")
		return
	}
	builder.addConnection(target.ID+"."+targetPort.Name, support.ID+"."+supportPort.Name, net, rationale)
}

func (builder *planBuilder) programmingNetAlias(targetID string, programmingID string, targetRole string, net string) string {
	if targetRole == "power.vcc" || targetRole == "power.gnd" {
		return net
	}
	return strings.ToUpper(normalizeToken(targetID) + "_" + normalizeToken(programmingID) + "_" + net)
}

func (builder *planBuilder) programmingMode(programmingID string) string {
	mode := normalizeToken(firstNonEmpty(builder.paramString(programmingID, "programming_interface"), builder.paramString(programmingID, "programming_mode"), builder.paramString(programmingID, "programming_header")))
	if mode == "" {
		return "isp"
	}
	return mode
}

func (builder *planBuilder) connectI2CBuses() {
	if len(builder.sensorIDs) == 0 && len(builder.i2cConnectorIDs) == 0 {
		return
	}
	buses := map[string]struct{}{}
	for _, id := range append(append([]string{}, builder.sensorIDs...), builder.i2cConnectorIDs...) {
		buses[builder.i2cBusFor(id)] = struct{}{}
	}
	for _, bus := range sortedSetKeys(buses) {
		mcuID, hasMCU := builder.i2cMCUTarget(bus)
		sdaNet := builder.i2cSignalNetAlias(bus, "SDA", hasMCU)
		sclNet := builder.i2cSignalNetAlias(bus, "SCL", hasMCU)
		sensorIDs := builder.instancesOnI2CBus(builder.sensorIDs, bus)
		connectorIDs := builder.instancesOnI2CBus(builder.i2cConnectorIDs, bus)
		for _, sensorID := range sensorIDs {
			if hasMCU {
				builder.addConnection(mcuID+".SDA", sensorID+".SDA", sdaNet, "I2C data connects MCU to sensor on "+bus)
				builder.addConnection(mcuID+".SCL", sensorID+".SCL", sclNet, "I2C clock connects MCU to sensor on "+bus)
			}
			for _, connectorID := range connectorIDs {
				builder.addConnection(connectorID+".SDA", sensorID+".SDA", sdaNet, "I2C data connects sensor to breakout connector on "+bus)
				builder.addConnection(connectorID+".SCL", sensorID+".SCL", sclNet, "I2C clock connects sensor to breakout connector on "+bus)
			}
		}
		builder.connectI2CConnectorPower(bus, sensorIDs, connectorIDs)
		if hasMCU {
			for _, connectorID := range connectorIDs {
				if len(sensorIDs) == 0 {
					builder.addConnection(mcuID+".SDA", connectorID+".SDA", sdaNet, "I2C data connects MCU to breakout connector on "+bus)
					builder.addConnection(mcuID+".SCL", connectorID+".SCL", sclNet, "I2C clock connects MCU to breakout connector on "+bus)
				}
			}
		}
	}
}

func (builder *planBuilder) connectI2CConnectorPower(bus string, sensorIDs []string, connectorIDs []string) {
	for _, connectorID := range connectorIDs {
		supplySource := ""
		supplyPort := ""
		conflict := false
		for _, sensorID := range sensorIDs {
			source, port := builder.supplySourceForTarget(sensorID)
			if source == "" || source == connectorID {
				continue
			}
			if supplySource == "" {
				supplySource, supplyPort = source, port
				continue
			}
			if supplySource != source || supplyPort != port {
				builder.addIssue("blocks."+connectorID+".supply", "I2C bus "+bus+" has sensors on conflicting supply sources", "use one voltage domain per exposed I2C connector")
				conflict = true
				break
			}
		}
		if conflict || supplySource == "" {
			continue
		}
		builder.addConnection(supplySource+"."+supplyPort, connectorID+"."+builder.powerPortFor(connectorID), builder.supplyNetAlias(supplySource), "sensor supply rail feeds I2C breakout connector")
	}
}

func (builder *planBuilder) i2cSignalNetAlias(bus string, signal string, hasMCU bool) string {
	if !hasMCU && len(builder.i2cConnectorIDs) == 1 {
		return strings.ToUpper(strings.TrimSpace(signal))
	}
	return builder.busNetAlias(bus, signal)
}

func sortedSetKeys(values map[string]struct{}) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func (builder *planBuilder) recordI2CBus(instanceID string, bus string) {
	instanceID = strings.TrimSpace(instanceID)
	if instanceID == "" {
		return
	}
	builder.i2cBuses[instanceID] = normalizeToken(bus)
}

func (builder *planBuilder) i2cBusFor(instanceID string) string {
	if bus := normalizeToken(builder.i2cBuses[instanceID]); bus != "" {
		return bus
	}
	if !builder.i2cDefaultNoted {
		builder.i2cDefaultNoted = true
		builder.plan.Assumptions = append(builder.plan.Assumptions, PlanNote{ID: "semantic.bus.i2c.default", Path: "interfaces", Message: "defaulted unnamed I2C bus to i2c1"})
	}
	return "i2c1"
}

func (builder *planBuilder) instancesOnI2CBus(ids []string, bus string) []string {
	var out []string
	for _, id := range ids {
		if builder.i2cBusFor(id) == bus {
			out = append(out, id)
		}
	}
	return out
}

func (builder *planBuilder) i2cMCUTarget(bus string) (string, bool) {
	candidates := builder.semantic.withPortRole("mcu.i2c.sda")
	candidates = filterSemanticCandidates(candidates, "mcu", "mcu.i2c.scl")
	if len(candidates) == 0 {
		return "", false
	}
	if len(candidates) > 1 {
		builder.semanticTargetIssue("interfaces."+bus, "multiple compatible MCU I2C targets require explicit target metadata", "set target.id for the I2C interface or split buses by target", StrengthRequired)
		return "", false
	}
	if assigned := builder.i2cMCUBus[candidates[0].ID]; assigned != "" && assigned != bus {
		if !builder.i2cMultiBusBlocked {
			builder.i2cMultiBusBlocked = true
			builder.semanticTargetIssue("interfaces."+bus, "the selected MCU I2C pins are already assigned to "+assigned+" and cannot safely satisfy "+bus, "use one I2C bus or add an MCU template with distinct I2C peripherals", StrengthRequired)
		}
		return "", false
	}
	builder.i2cMCUBus[candidates[0].ID] = bus
	return candidates[0].ID, true
}

func (builder *planBuilder) recordSupportTarget(instanceID string, reqID string, path string, target TargetRef, strength Strength) {
	if strings.TrimSpace(instanceID) == "" {
		return
	}
	builder.supportTargets[instanceID] = semanticSupportIntent{RequirementID: reqID, Path: path, Target: target, Strength: strength}
}

func (builder *planBuilder) supportTargetPath(instanceID string) string {
	if intent, ok := builder.supportTargets[instanceID]; ok && intent.Path != "" {
		return intent.Path
	}
	return "functions"
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

func (builder *planBuilder) powerSourceForPoweredLED(instanceID string) (powerSource, bool) {
	if builder.instanceBlockIDs[instanceID] != "led_indicator" {
		return powerSource{}, false
	}
	if voltage := paramText(builder.instanceParams[instanceID], "supply_voltage"); voltage != "" {
		sourceID, port := builder.supplySourceForTarget(instanceID)
		if sourceID == "" {
			return powerSource{}, false
		}
		return powerSource{id: sourceID, port: port, voltage: voltage}, true
	}
	if len(builder.orderedPowerInputs) == 1 {
		return builder.orderedPowerInputs[0], true
	}
	sources := builder.availablePowerSources()
	if len(sources) == 1 {
		return sources[0], true
	}
	return powerSource{}, false
}

func (builder *planBuilder) connectPoweredLED(source powerSource, ledID string) {
	netAlias := builder.supplyNetAlias(source.id)
	if builder.ledIndicatorActiveHigh(ledID) {
		builder.applyInferredLEDDefaults(ledID, source.voltage)
		builder.addConnection(source.id+"."+source.port, ledID+".IN", netAlias, "power source drives active-high LED indicator")
	} else {
		builder.addConnection(source.id+"."+builder.groundPortFor(source.id), ledID+".IN", "GND", "ground drives active-low LED indicator on")
	}
	builder.appendRequirementEvidenceForInstance(ledID, "supply:"+source.id+"."+source.port)
	builder.appendRequirementEvidenceForInstance(ledID, "net:"+netAlias)
}

func (builder *planBuilder) applyInferredLEDDefaults(ledID string, voltage string) {
	if voltage != "" && paramText(builder.instanceParams[ledID], "supply_voltage") == "" {
		builder.updateWorkflowBlockParam(ledID, "supply_voltage", voltage)
	}
	for _, key := range []string{"led_forward_voltage", "led_current"} {
		if paramText(builder.instanceParams[ledID], key) == "" {
			if value := builder.paramString(ledID, key); value != "" {
				builder.updateWorkflowBlockParam(ledID, key, value)
			}
		}
	}
}

func (builder *planBuilder) supplyNetAlias(sourceID string) string {
	if builder.instanceBlockIDs[sourceID] == "connector_breakout" {
		return builder.powerPortFor(sourceID)
	}
	if voltage := firstNonEmpty(builder.paramString(sourceID, "output_voltage"), builder.inputVoltageForInstance(sourceID)); voltage != "" {
		return "VCC_" + voltageNetToken(voltage)
	}
	if sourceID != "" {
		return "VCC_" + normalizeToken(sourceID)
	}
	return "VCC"
}

func voltageNetToken(voltage string) string {
	voltage = strings.TrimSpace(voltage)
	if strings.Contains(voltage, ".") && strings.HasSuffix(strings.ToUpper(voltage), "V") {
		voltage = strings.TrimSpace(voltage[:len(voltage)-1])
	}
	return normalizeToken(strings.ReplaceAll(voltage, ".", "V"))
}

func (builder *planBuilder) rawPowerSourceForVoltage(voltage string) (string, string) {
	for _, source := range []struct {
		ids  []string
		port string
	}{
		{ids: builder.usbPowerIDs, port: "VBUS_OUT"},
		{ids: builder.inputPowerIDs, port: "VIN"},
		{ids: builder.i2cConnectorIDs, port: "VCC"},
		{ids: builder.powerConnectorIDs, port: "VCC"},
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

func (builder *planBuilder) busNetAlias(bus string, signal string) string {
	bus = normalizeToken(bus)
	signal = strings.ToUpper(strings.TrimSpace(signal))
	if bus == "" {
		return signal
	}
	return strings.ToUpper(strings.ReplaceAll(bus, "-", "_")) + "_" + signal
}

func (builder *planBuilder) targetSupplyVoltage(targetID string) string {
	if supply := builder.instanceSupplies[targetID]; supply != "" {
		if voltage := builder.railAliasVoltage[supply]; voltage != "" {
			return voltage
		}
		if _, ok := parseVoltage(supply); ok {
			return supply
		}
		return ""
	}
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
	appendTarget := func(id string) {
		if port := strings.TrimSpace(builder.powerPortFor(id)); port != "" {
			targets = append(targets, struct{ id, port string }{id: id, port: port})
		}
	}
	for _, id := range builder.sensorIDs {
		appendTarget(id)
	}
	for _, id := range builder.mcuIDs {
		appendTarget(id)
	}
	for _, id := range builder.ledIDs {
		if port := builder.ledIndicatorPowerTargetPort(id); port != "" {
			targets = append(targets, struct{ id, port string }{id: id, port: port})
		}
	}
	for _, id := range builder.amplifierIDs {
		appendTarget(id)
	}
	for _, id := range builder.classABOutputIDs {
		appendTarget(id)
	}
	for _, id := range builder.poweredClockIDs {
		appendTarget(id)
	}
	for _, id := range builder.programmingIDs {
		appendTarget(id)
	}
	for _, id := range builder.powerConnectorIDs {
		appendTarget(id)
	}
	return targets
}

func (builder *planBuilder) ledIndicatorPowerTargetPort(instanceID string) string {
	port := strings.TrimSpace(builder.powerPortFor(instanceID))
	if builder.instanceBlockIDs[instanceID] != "led_indicator" {
		return port
	}
	if builder.ledIndicatorActiveHigh(instanceID) {
		return ""
	}
	return port
}

func (builder *planBuilder) ledIndicatorActiveHigh(instanceID string) bool {
	params := builder.instanceParams[instanceID]
	value, ok := params["active_high"]
	if !ok {
		return true
	}
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		normalized := strings.ToLower(strings.TrimSpace(typed))
		if normalized == "false" || normalized == "0" || normalized == "no" || normalized == "off" || normalized == "disabled" {
			return false
		}
		return true
	case int:
		return typed != 0
	case int8:
		return typed != 0
	case int16:
		return typed != 0
	case int32:
		return typed != 0
	case int64:
		return typed != 0
	case uint:
		return typed != 0
	case uint8:
		return typed != 0
	case uint16:
		return typed != 0
	case uint32:
		return typed != 0
	case uint64:
		return typed != 0
	case float32:
		return math.Abs(float64(typed)) > 1e-9
	case float64:
		return math.Abs(typed) > 1e-9
	default:
		return true
	}
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

func (builder *planBuilder) groundPortFor(instanceID string) string {
	for _, pin := range stringListParam(builder.instanceParams[instanceID]["pin_names"]) {
		if isGroundPortName(pin) {
			return pin
		}
	}
	definition, ok := builder.registry.GetBlock(builder.instanceBlockIDs[instanceID])
	if !ok {
		return "GND"
	}
	for _, preferred := range []string{"GND", "VSS", "0V", "LOAD_REF", "RET"} {
		for _, port := range definition.Ports {
			if strings.EqualFold(port.Name, preferred) {
				return port.Name
			}
		}
	}
	for _, port := range definition.Ports {
		if port.Direction == blocks.PortPower && strings.EqualFold(port.Name, "GND") {
			return port.Name
		}
	}
	return "GND"
}

func isGroundPortName(name string) bool {
	switch strings.ToUpper(strings.TrimSpace(name)) {
	case "GND", "VSS", "0V", "LOAD_REF", "RET":
		return true
	default:
		return false
	}
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

func (builder *planBuilder) addInterfaceConnector(reqID string, prefix string, pins []string, iface InterfaceIntent) string {
	if iface.Strength == StrengthForbidden {
		builder.plan.KnownGaps = append(builder.plan.KnownGaps, PlanNote{ID: reqID + ".forbidden", Path: reqID, Message: "connector requirement was forbidden and omitted"})
		return ""
	}
	params := map[string]any{"pin_names": pins}
	if iface.Kind != "" {
		params["interface_kind"] = iface.Kind
	}
	if iface.Connector != "" {
		params["connector"] = iface.Connector
	}
	if iface.Voltage != "" {
		params["voltage"] = iface.Voltage
	}
	if iface.Bus != "" {
		params["bus"] = iface.Bus
	}
	return builder.addBlock(reqID, prefix, "connector_breakout", params, "connector breakout exposes requested interface")
}

func interfaceSignalConnectorPins(iface InterfaceIntent) []string {
	if iface.Kind == "analog" {
		return []string{"SIG", "GND"}
	}
	switch normalizeToken(iface.Connector) {
	case "audio_input", "headphone_output", "speaker_output":
		return []string{"SIG", "GND"}
	default:
		return []string{"SIG", "VCC", "GND"}
	}
}

func (builder *planBuilder) addBlock(reqID string, prefix string, blockID string, params map[string]any, rationale string) string {
	definition, ok := builder.registry.GetBlock(blockID)
	if !ok {
		builder.addIssue("blocks."+blockID, "unknown block ID "+blockID, "choose a supported built-in block")
		return ""
	}
	id := builder.nextID(prefix)
	clonedParams := cloneParams(params)
	if builder.instanceBlockIDs == nil {
		builder.instanceBlockIDs = map[string]string{}
	}
	if builder.instanceParams == nil {
		builder.instanceParams = map[string]map[string]any{}
	}
	if builder.instanceReqIDs == nil {
		builder.instanceReqIDs = map[string]string{}
	}
	builder.instanceBlockIDs[id] = blockID
	builder.instanceParams[id] = clonedParams
	builder.instanceReqIDs[id] = reqID
	if builder.workflowBlockIndex == nil {
		// Some focused unit tests construct planBuilder directly instead of
		// going through Plan, so keep addBlock resilient.
		builder.workflowBlockIndex = map[string]int{}
	}
	builder.workflowBlockIndex[id] = len(builder.workflow.Blocks)
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
	if builder.selectedBlockIndex == nil {
		builder.selectedBlockIndex = map[string]int{}
	}
	builder.selectedBlockIndex[id] = len(builder.plan.SelectedBlocks)
	builder.plan.SelectedBlocks = append(builder.plan.SelectedBlocks, record)
	builder.semantic.addInstance(id, prefix, blockID, clonedParams, definition)
	return id
}

func (builder *planBuilder) updateSelectedBlockParam(instanceID string, key string, value any) {
	index, ok := builder.selectedBlockIndex[instanceID]
	if !ok || index < 0 || index >= len(builder.plan.SelectedBlocks) {
		return
	}
	if builder.plan.SelectedBlocks[index].Params == nil {
		builder.plan.SelectedBlocks[index].Params = map[string]any{}
	}
	builder.plan.SelectedBlocks[index].Params[key] = value
}

func (builder *planBuilder) updateWorkflowBlockParam(instanceID string, key string, value any) {
	index, ok := builder.workflowBlockIndex[instanceID]
	if !ok || index < 0 || index >= len(builder.workflow.Blocks) {
		return
	}
	params := builder.instanceParams[instanceID]
	if params == nil {
		params = map[string]any{}
		builder.instanceParams[instanceID] = params
		builder.workflow.Blocks[index].Params = params
	}
	params[key] = value
	if selectedIndex, ok := builder.selectedBlockIndex[instanceID]; ok && selectedIndex >= 0 && selectedIndex < len(builder.plan.SelectedBlocks) {
		if builder.plan.SelectedBlocks[selectedIndex].Params == nil {
			builder.plan.SelectedBlocks[selectedIndex].Params = params
		}
		builder.plan.SelectedBlocks[selectedIndex].Params[key] = value
	}
}

func (builder *planBuilder) addRequirement(record RequirementRecord) {
	builder.requirementIndex[record.ID] = len(builder.plan.Requirements)
	builder.plan.Requirements = append(builder.plan.Requirements, record)
}

func (builder *planBuilder) appendRequirementEvidenceForInstance(instanceID string, evidence string) {
	reqID := builder.instanceReqIDs[instanceID]
	if reqID == "" || evidence == "" {
		return
	}
	index, ok := builder.requirementIndex[reqID]
	if !ok || index < 0 || index >= len(builder.plan.Requirements) {
		return
	}
	builder.plan.Requirements[index].Evidence = appendUniqueString(builder.plan.Requirements[index].Evidence, evidence)
}

func (builder *planBuilder) recordInstanceSupply(instanceID string, supply string) {
	if instanceID == "" {
		return
	}
	if supply = normalizeToken(supply); supply != "" {
		builder.instanceSupplies[instanceID] = supply
		builder.validateInstanceSupply(instanceID, supply)
	}
}

func (builder *planBuilder) validateInstanceSupply(instanceID string, supply string) {
	if _, ok := parseVoltage(supply); ok {
		return
	}
	voltage := builder.railAliasVoltage[supply]
	if voltage == "" {
		builder.addIssue("blocks."+instanceID+".supply", "unknown supply alias "+supply+" for "+instanceID, "define a matching power.rails alias/name or use an explicit voltage")
		builder.recordSynthesisGap(SynthesisGap{
			ID:         "supply.unknown." + instanceID,
			Category:   "voltage_domain",
			Path:       "blocks." + instanceID + ".supply",
			Message:    "unknown supply alias " + supply + " for " + instanceID,
			Severity:   reports.SeverityError,
			Suggestion: "define a matching power.rails alias/name or use an explicit voltage",
		})
		return
	}
	explicit := builder.paramString(instanceID, "supply_voltage")
	if explicit != "" && !voltagesEquivalent(explicit, voltage) {
		builder.addIssue("blocks."+instanceID+".supply_voltage", "supply alias "+supply+" resolves to "+voltage+" but "+instanceID+" requested "+explicit, "make the function supply and block supply_voltage agree")
		builder.recordSynthesisGap(SynthesisGap{
			ID:         "supply.conflict." + instanceID,
			Category:   "voltage_domain",
			Path:       "blocks." + instanceID + ".supply_voltage",
			Message:    "supply alias " + supply + " resolves to " + voltage + " but " + instanceID + " requested " + explicit,
			Severity:   reports.SeverityError,
			Suggestion: "make the function supply and block supply_voltage agree",
		})
	}
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
	if len(inputs) == 0 {
		return false
	}
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
	case "i2c", "gpio", "analog", "connector", "power":
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

func appendUniqueString(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
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
