package architecturesearch

import (
	"context"
	"fmt"
	"math"
	"slices"
	"strings"

	"kicadai/internal/components"
)

type mcuCrystalChoice struct {
	part                  catalogPart
	frequencyHz           float64
	loadCapacitanceF      float64
	driveLevelW           float64
	frequencyStabilityPct float64
}

// This is a nominal catalog-value matching resolution, not a claim about
// physical capacitor tolerance or frequency pulling.
const mcuCrystalNominalLoadMatchResolutionF = 0.1e-12

const (
	mcuProgrammingVoltageRangeResolutionV = 1e-3
	mcuClockMaximumDistanceMM             = 5
	mcuCrystalRejectionDetailLimit        = 8
	// Nominal catalog frequency matching allows one ppm of representation
	// variance; reviewed crystal stability is enforced separately.
	mcuCrystalFrequencyMatchRelativeError = 1e-6
)

func isExternalCrystalClock(option components.MCUClockOption) bool {
	switch strings.ToLower(strings.TrimSpace(option.Kind)) {
	case "external_crystal", "external_crystal_or_clock":
		return true
	default:
		return false
	}
}

func (provider *CatalogProvider) expandMCUExternalCrystal(
	ctx context.Context,
	request ProviderRequest,
	parent catalogPart,
	assignment mcuAssignment,
	parts []catalogPart,
	connections []RealizationConnection,
) ([]catalogPart, []RealizationConnection, CalculationEvidence, error) {
	option := assignment.ClockOption
	if !isExternalCrystalClock(option) {
		return parts, connections, CalculationEvidence{}, nil
	}
	if len(option.Pins) != 2 || option.MaximumCrystalDriveW == nil || option.MaximumStartupS == nil {
		return nil, nil, CalculationEvidence{}, &mcuAssignmentError{
			Code: CodeMCUClockUnavailable, Text: "selected external-crystal option lacks two-pin drive and startup evidence",
		}
	}
	frequencyHz, tolerancePct, frequencyOK := firstNumericConstraint(request.Constraints, "clock_frequency", "cpu_clock_frequency")
	if !frequencyOK || !finitePositive(frequencyHz) || tolerancePct <= 0 {
		return nil, nil, CalculationEvidence{}, &mcuAssignmentError{
			Code: CodeMCUClockFrequency, Text: "external-crystal synthesis requires a positive clock-frequency target and tolerance",
		}
	}
	frequencyMinimumHz := frequencyHz * (1 - tolerancePct/100)
	frequencyMaximumHz := frequencyHz * (1 + tolerancePct/100)
	if frequencyMinimumHz < option.MinimumHz || frequencyMaximumHz > option.MaximumHz {
		return nil, nil, CalculationEvidence{}, &mcuAssignmentError{
			Code: CodeMCUClockFrequency, Text: "requested crystal frequency envelope is outside the selected MCU clock-option range",
		}
	}
	strayCapacitanceF, _, strayOK := firstNumericConstraint(request.Constraints, "crystal_stray_capacitance")
	if !strayOK || !finitePositive(strayCapacitanceF) {
		return nil, nil, CalculationEvidence{}, &mcuAssignmentError{
			Code: CodeMCUClockUnavailable, Text: "external-crystal synthesis requires bounded positive board-stray capacitance",
		}
	}
	startupLimitS, _, startupConstrained := firstNumericConstraint(request.Constraints, "maximum_clock_startup_time", "clock_startup_time")
	if startupConstrained && (!finitePositive(startupLimitS) || *option.MaximumStartupS > startupLimitS) {
		return nil, nil, CalculationEvidence{}, &mcuAssignmentError{
			Code: CodeMCUClockUnavailable, Text: "selected external-crystal option does not satisfy the requested startup bound",
		}
	}

	choice, err := provider.selectMCUCrystal(ctx, frequencyHz, tolerancePct, *option.MaximumCrystalDriveW)
	if err != nil {
		return nil, nil, CalculationEvidence{}, err
	}
	loadCapacitorF := 2 * (choice.loadCapacitanceF - strayCapacitanceF)
	if !finitePositive(loadCapacitorF) {
		return nil, nil, CalculationEvidence{}, &mcuAssignmentError{
			Code: CodeMCUClockUnavailable, Text: "crystal load capacitance does not exceed the bounded board-stray capacitance",
		}
	}
	supplyMaximumV := maximumPortVoltage(request.Ports)
	loadCapacitor, err := provider.selectPassiveComponentWithRatings(ctx, "capacitor", "capacitance", engineeringValue(loadCapacitorF, "F"), []components.RequiredRating{
		{Kind: "voltage", Value: numericString(supplyMaximumV), Unit: "V"},
	})
	if err != nil {
		return nil, nil, CalculationEvidence{}, fmt.Errorf("select external-crystal load capacitor: %w", err)
	}
	selectedLoadCapacitorF, selectedLoadOK := components.ParseEngineeringValue(loadCapacitor.value)
	if !selectedLoadOK || !finitePositive(selectedLoadCapacitorF) {
		return nil, nil, CalculationEvidence{}, &mcuAssignmentError{
			Code: CodeMCUClockUnavailable, Text: "selected external-crystal load capacitor lacks normalized nominal capacitance evidence",
		}
	}

	choice.part.selected.InstanceID, choice.part.usage = "mcu_clock_crystal", "external_crystal_resonator"
	choice.part.value = engineeringValue(choice.frequencyHz, "Hz")
	choice.part.near, choice.part.maxDistanceMM = parent.selected.InstanceID, mcuClockMaximumDistanceMM
	loadIn := instantiateCatalogPart(loadCapacitor, "mcu_clock_load_in", "load_capacitor", engineeringValue(selectedLoadCapacitorF, "F"), parent.selected.InstanceID, mcuClockMaximumDistanceMM)
	loadOut := instantiateCatalogPart(loadCapacitor, "mcu_clock_load_out", "load_capacitor", engineeringValue(selectedLoadCapacitorF, "F"), parent.selected.InstanceID, mcuClockMaximumDistanceMM)
	parts = append(parts, choice.part, loadIn, loadOut)

	connections = appendMCUSupportConnection(connections, parent, option.Pins[0], endpoint(choice.part, "XTAL_1"))
	connections = appendMCUSupportConnection(connections, parent, option.Pins[0], endpoint(loadIn, "A"))
	connections = appendMCUSupportConnection(connections, parent, option.Pins[1], endpoint(choice.part, "XTAL_2"))
	connections = appendMCUSupportConnection(connections, parent, option.Pins[1], endpoint(loadOut, "A"))
	groups := mcuSupplyGroups(parent.record.MCU)
	if len(groups) != 1 || len(groups[0].GroundFunctions) == 0 {
		return nil, nil, CalculationEvidence{}, &mcuAssignmentError{
			Code: CodeMCUClockUnavailable, Text: "external-crystal synthesis requires one qualified MCU reference rail group",
		}
	}
	// Ground functions in one rail group are catalog-qualified as the same
	// electrical reference; sorted supply-group normalization makes this choice
	// deterministic. Multiple independent reference groups fail closed above.
	connections = appendMCUSupportConnection(connections, parent, groups[0].GroundFunctions[0], endpoint(loadIn, "B"))
	connections = appendMCUSupportConnection(connections, parent, groups[0].GroundFunctions[0], endpoint(loadOut, "B"))

	effectiveLoadF := selectedLoadCapacitorF/2 + strayCapacitanceF
	bounds := []CalculationBound{
		maximumBound("frequency_stability", tolerancePct, choice.frequencyStabilityPct, "%"),
		minimumBound("crystal_drive_rating", *option.MaximumCrystalDriveW, choice.driveLevelW, "W"),
		maximumBound("nominal_load_capacitance_error", mcuCrystalNominalLoadMatchResolutionF, math.Abs(effectiveLoadF-choice.loadCapacitanceF), "F"),
	}
	if startupConstrained {
		bounds = append(bounds, maximumBound("startup_time", startupLimitS, *option.MaximumStartupS, "s"))
	}
	worstMargin, pass := normalizedBoundsMargin(bounds)
	if !pass {
		return nil, nil, CalculationEvidence{}, &mcuAssignmentError{
			Code: CodeMCUClockUnavailable, Text: "external-crystal load, drive, accuracy, or startup calculation did not close",
		}
	}
	calculation := CalculationEvidence{
		ID: "mcu_external_crystal_worst_case", FormulaID: FormulaRatingMargin, FormulaRevision: FormulaRevision,
		Inputs: []NamedQuantity{
			{Name: "target_frequency", Value: frequencyHz, Unit: "Hz"},
			{Name: "target_frequency_tolerance", Value: tolerancePct, Unit: "%"},
			{Name: "crystal_load_capacitance", Value: choice.loadCapacitanceF, Unit: "F"},
			{Name: "board_stray_capacitance", Value: strayCapacitanceF, Unit: "F"},
			{Name: "mcu_maximum_crystal_drive", Value: *option.MaximumCrystalDriveW, Unit: "W"},
			{Name: "calculated_load_capacitor_each", Value: loadCapacitorF, Unit: "F"},
		},
		NominalOutputs: []NamedQuantity{
			{Name: "selected_crystal_frequency", Value: choice.frequencyHz, Unit: "Hz"},
			{Name: "selected_load_capacitor_each", Value: selectedLoadCapacitorF, Unit: "F"},
			{Name: "effective_load_capacitance", Value: effectiveLoadF, Unit: "F"},
			{Name: "maximum_startup_time", Value: *option.MaximumStartupS, Unit: "s"},
		},
		Bounds: bounds, WorstMargin: worstMargin, Pass: true,
	}
	calculation, err = FinalizeCalculation(calculation)
	if err != nil {
		return nil, nil, CalculationEvidence{}, fmt.Errorf("finalize MCU external-crystal calculation: %w", err)
	}
	return parts, connections, calculation, nil
}

func (provider *CatalogProvider) selectMCUCrystal(
	ctx context.Context,
	frequencyHz float64,
	tolerancePct float64,
	requiredDriveW float64,
) (mcuCrystalChoice, error) {
	var choices []mcuCrystalChoice
	var rejections []string
	rejectionCount := 0
	appendRejection := func(detail string) {
		rejectionCount++
		if len(rejections) < mcuCrystalRejectionDetailLimit {
			rejections = append(rejections, detail)
		}
	}
	for _, record := range provider.familyRecords["crystal"] {
		if err := ctx.Err(); err != nil {
			return mcuCrystalChoice{}, err
		}
		if record.Generic || record.MPN == "" ||
			confidenceRank(EvidenceConfidenceFromComponent(record.Verification.Confidence)) < confidenceRank(EvidenceVerified) {
			appendRejection(record.ID + ":concrete_verified_part_required")
			continue
		}
		frequency, frequencyOK := recordValue(record, "frequency", "Hz")
		loadCapacitance, loadOK := recordValue(record, "load_capacitance", "F")
		driveLevel, driveOK := recordRatingMaximum(record, "drive_level", "W")
		stabilityPPM, stabilityOK := recordRatingMaximum(record, "frequency_stability", "ppm")
		if !frequencyOK || !loadOK || !driveOK || !stabilityOK ||
			math.Abs(frequency-frequencyHz) > math.Max(frequencyHz*mcuCrystalFrequencyMatchRelativeError, 1e-6) ||
			driveLevel < requiredDriveW || stabilityPPM/10_000 > tolerancePct {
			appendRejection(fmt.Sprintf(
				"%s:frequency=%t,load=%t,drive=%t,stability=%t",
				record.ID, frequencyOK, loadOK, driveOK, stabilityOK,
			))
			continue
		}
		for _, variant := range record.Packages {
			if variant.ID == "" || !catalogRecordSupportsFunctions(record, []string{"xtal_1", "xtal_2"}) {
				continue
			}
			evidence := componentEvidence(record, variant.Verification.Confidence)
			choices = append(choices, mcuCrystalChoice{
				part: catalogPart{
					selected: SelectedComponent{CatalogID: record.ID, VariantID: variant.ID, Evidence: evidence.Confidence},
					record:   record, evidence: evidence,
				},
				frequencyHz: frequency, loadCapacitanceF: loadCapacitance,
				driveLevelW: driveLevel, frequencyStabilityPct: stabilityPPM / 10_000,
			})
		}
	}
	if err := ctx.Err(); err != nil {
		return mcuCrystalChoice{}, err
	}
	slices.SortStableFunc(choices, func(left, right mcuCrystalChoice) int {
		if left.frequencyStabilityPct < right.frequencyStabilityPct {
			return -1
		}
		if left.frequencyStabilityPct > right.frequencyStabilityPct {
			return 1
		}
		if order := strings.Compare(left.part.record.ID, right.part.record.ID); order != 0 {
			return order
		}
		return strings.Compare(left.part.selected.VariantID, right.part.selected.VariantID)
	})
	if len(choices) == 0 {
		slices.Sort(rejections)
		if omitted := rejectionCount - len(rejections); omitted > 0 {
			rejections = append(rejections, fmt.Sprintf("%d_additional_rejections_omitted", omitted))
		}
		detail := strings.Join(rejections, ";")
		if detail != "" {
			detail = " (" + detail + ")"
		}
		return mcuCrystalChoice{}, &mcuAssignmentError{
			Code: CodeMCUClockUnavailable, Text: "no verified crystal satisfies the requested frequency, stability, drive, and package-function envelope" + detail,
		}
	}
	return choices[0], nil
}

// instantiateCatalogPart creates independent per-instance mutable state while
// retaining the catalog record as immutable shared evidence.
func instantiateCatalogPart(template catalogPart, instanceID, usage, value, near string, maxDistanceMM float64) catalogPart {
	return catalogPart{
		selected: SelectedComponent{
			InstanceID: instanceID,
			CatalogID:  template.selected.CatalogID,
			VariantID:  template.selected.VariantID,
			Evidence:   template.selected.Evidence,
		},
		record:               template.record,
		usage:                usage,
		value:                value,
		evidence:             ContractEvidence{Confidence: template.evidence.Confidence, Sources: slices.Clone(template.evidence.Sources)},
		toleranceKind:        template.toleranceKind,
		maximumTolerance:     template.maximumTolerance,
		toleranceUnit:        template.toleranceUnit,
		maximumTempcoPPMPerC: template.maximumTempcoPPMPerC,
		near:                 near,
		maxDistanceMM:        maxDistanceMM,
		parameters:           slices.Clone(template.parameters),
		evidenceSources:      slices.Clone(template.evidenceSources),
	}
}

func mcuProgrammingCalculation(request ProviderRequest, assignment mcuAssignment) (CalculationEvidence, error) {
	electrical := assignment.ProgrammingInterface.Electrical
	if electrical == nil || electrical.MaximumConnectedCapacitance == nil ||
		electrical.SeriesIsolationResistance == nil || electrical.MaximumFrequency == nil {
		return CalculationEvidence{}, &mcuAssignmentError{
			Code: CodeMCUProgrammingLoad, Text: "selected programming interface lacks complete loading, isolation, and frequency evidence",
		}
	}
	maximumCapacitanceF, capacitanceOK := clockMeasurement(electrical.MaximumConnectedCapacitance, "F")
	maximumFrequencyHz, frequencyOK := clockMeasurement(electrical.MaximumFrequency, "Hz")
	minimumResistance, maximumResistance, resistanceOK := clockEvidenceRange(electrical.SeriesIsolationResistance, "Ohm")
	if !capacitanceOK || !frequencyOK || !resistanceOK || !finitePositive(maximumCapacitanceF) ||
		!finitePositive(maximumFrequencyHz) || !finitePositive(minimumResistance) || maximumResistance < minimumResistance {
		return CalculationEvidence{}, &mcuAssignmentError{
			Code: CodeMCUProgrammingLoad, Text: "selected programming interface has invalid normalized loading evidence",
		}
	}
	requestedCapacitanceF := maximumCapacitanceF
	if value, _, ok := firstNumericConstraint(request.Constraints, "programming_load_capacitance", "debug_load_capacitance"); ok {
		requestedCapacitanceF = value
	}
	requestedFrequencyHz := maximumFrequencyHz
	if value, _, ok := firstNumericConstraint(request.Constraints, "programming_frequency", "debug_frequency"); ok {
		requestedFrequencyHz = value
	}
	if !finitePositive(requestedCapacitanceF) || !finitePositive(requestedFrequencyHz) {
		return CalculationEvidence{}, &mcuAssignmentError{
			Code: CodeMCUProgrammingLoad, Text: "programming-interface loading and frequency requirements must be positive",
		}
	}
	targetVoltageMinimum, targetVoltageMaximum, voltageRangeOK := mcuProgrammingTargetVoltageRange(request.Ports)
	if !voltageRangeOK || !finitePositive(targetVoltageMinimum) || targetVoltageMaximum < targetVoltageMinimum {
		return CalculationEvidence{}, &mcuAssignmentError{
			Code: CodeMCUProgrammingLoad, Text: "programming-interface voltage compatibility requires a bounded positive target supply range",
		}
	}
	programmerVoltage := targetVoltageMaximum
	if value, _, ok := firstNumericConstraint(request.Constraints, "programmer_voltage", "debugger_voltage"); ok {
		programmerVoltage = value
	}
	resetMapped := 0.0
	if programmingResetEntryMapped(assignment.ProgrammingInterface) {
		resetMapped = 1
	}
	isolationPopulated := requiresMCUProgrammingPhysicalIsolation(request)
	selectedResistance := 0.0
	modeledEdgeS := 0.0
	maximumEdgeS := 0.25 / requestedFrequencyHz
	bounds := []CalculationBound{
		maximumBound("connected_capacitance", maximumCapacitanceF, requestedCapacitanceF, "F"),
		maximumBound("programming_frequency", maximumFrequencyHz, requestedFrequencyHz, "Hz"),
		minimumBound("programmer_voltage_minimum", targetVoltageMinimum, programmerVoltage, "V"),
		maximumBound("programmer_voltage_maximum", targetVoltageMaximum, programmerVoltage, "V"),
		minimumBound("reset_entry_mapping", 1, resetMapped, "count"),
	}
	if isolationPopulated {
		selectedResistance = minimumResistance
		modeledEdgeS = 2.2 * selectedResistance * requestedCapacitanceF
		bounds = append(bounds,
			minimumBound("series_isolation_minimum", minimumResistance, selectedResistance, "Ohm"),
			maximumBound("series_isolation_maximum", maximumResistance, selectedResistance, "Ohm"),
			maximumBound("isolation_rc_edge", maximumEdgeS, modeledEdgeS, "s"),
		)
	}
	worstMargin, pass := normalizedBoundsMargin(bounds)
	if !pass {
		return CalculationEvidence{}, &mcuAssignmentError{
			Code: CodeMCUProgrammingLoad, Text: "programming loading, frequency, voltage, reset, or isolation calculation did not close",
		}
	}
	calculation := CalculationEvidence{
		ID: "mcu_programming_interface_worst_case", FormulaID: FormulaRatingMargin, FormulaRevision: FormulaRevision,
		Inputs: []NamedQuantity{
			{Name: "requested_connected_capacitance", Value: requestedCapacitanceF, Unit: "F"},
			{Name: "requested_programming_frequency", Value: requestedFrequencyHz, Unit: "Hz"},
			{Name: "programmer_voltage", Value: programmerVoltage, Unit: "V"},
			{Name: "target_voltage_minimum", Value: targetVoltageMinimum, Unit: "V"},
			{Name: "target_voltage_maximum", Value: targetVoltageMaximum, Unit: "V"},
			{Name: "series_isolation_populated", Value: boolQuantity(isolationPopulated), Unit: "bool"},
		},
		NominalOutputs: []NamedQuantity{
			{Name: "maximum_connected_capacitance", Value: maximumCapacitanceF, Unit: "F"},
			{Name: "maximum_programming_frequency", Value: maximumFrequencyHz, Unit: "Hz"},
			{Name: "series_isolation_resistance", Value: selectedResistance, Unit: "Ohm"},
			{Name: "modeled_isolation_edge", Value: modeledEdgeS, Unit: "s"},
			{Name: "reset_entry_mapped", Value: resetMapped, Unit: "count"},
		},
		Bounds: bounds, WorstMargin: worstMargin, Pass: true,
	}
	finalized, err := FinalizeCalculation(calculation)
	if err != nil {
		return CalculationEvidence{}, fmt.Errorf("finalize MCU programming-interface calculation: %w", err)
	}
	return finalized, nil
}

func mcuProgrammingTargetVoltageRange(ports []RoleContract) (float64, float64, bool) {
	if minimum, maximum, ok := roleVoltageRange(ports, "power"); ok {
		return minimum, maximum, true
	}
	if minimum, maximum, ok := roleVoltageRange(ports, "supply"); ok {
		return minimum, maximum, true
	}
	var minimum, maximum float64
	found := false
	for _, port := range ports {
		if port.Contract.Kind == "reference" || port.Contract.Voltage.Minimum == nil || port.Contract.Voltage.Maximum == nil {
			continue
		}
		candidateMinimum, candidateMaximum := *port.Contract.Voltage.Minimum, *port.Contract.Voltage.Maximum
		if !found {
			minimum, maximum, found = candidateMinimum, candidateMaximum, true
			continue
		}
		minimum = math.Max(minimum, candidateMinimum)
		maximum = math.Min(maximum, candidateMaximum)
	}
	if found && maximum < minimum {
		if minimum-maximum > mcuProgrammingVoltageRangeResolutionV {
			return 0, 0, false
		}
		normalized := (minimum + maximum) / 2
		minimum, maximum = normalized, normalized
	}
	return minimum, maximum, found
}

func programmingResetEntryMapped(programming components.MCUProgrammingInterface) bool {
	hasEnable, hasBoot := false, false
	for _, signal := range programming.Signals {
		switch canonicalIdentifier(signal.Signal) {
		case "reset":
			return true
		case "enable":
			hasEnable = true
		case "boot":
			hasBoot = true
		}
	}
	return hasEnable && hasBoot
}

func boolQuantity(value bool) float64 {
	if value {
		return 1
	}
	return 0
}
