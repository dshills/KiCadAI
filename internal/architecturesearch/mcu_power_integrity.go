package architecturesearch

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"

	"kicadai/internal/components"
)

const mcuPowerCapacitorRejectionDetailLimit = 8

type mcuPowerEnvelope struct {
	startupCurrentA         float64
	transientCurrentA       float64
	transientDurationS      float64
	localTransientDurationS float64
	maximumRippleV          float64
	maximumNoiseV           float64
	brownoutThresholdV      float64
	sourceImpedanceOhm      float64
	localPlacementMM        float64
	bulkPlacementMM         float64
	ambientMinimumC         float64
	ambientMaximumC         float64
}

type mcuPowerCapacitorChoice struct {
	part                  catalogPart
	nominalCapacitanceF   float64
	effectiveCapacitanceF float64
	tolerancePct          float64
	esrOhm                float64
	rippleCurrentA        float64
	voltageRatingV        float64
	voltageDeratingFactor float64
	temperatureMinimumC   float64
	temperatureMaximumC   float64
	observedDroopV        float64
	requiredCapacitanceF  float64
}

func (provider *CatalogProvider) expandMCUPowerIntegrity(
	ctx context.Context,
	request ProviderRequest,
	parent catalogPart,
	parts []catalogPart,
	connections []RealizationConnection,
) ([]catalogPart, []RealizationConnection, []CalculationEvidence, error) {
	if parent.record.MCU == nil {
		return nil, nil, nil, &mcuAssignmentError{
			Code: CodeMCUPowerIntegrityEvidence, Text: "selected controller lacks normalized MCU evidence",
		}
	}
	groups := mcuSupplyGroups(parent.record.MCU)
	if len(groups) == 0 {
		return nil, nil, nil, &mcuAssignmentError{
			Code: CodeMCUPowerIntegrityDomain, Text: "selected controller has no normalized supply rail groups",
		}
	}
	evidenceByGroup := make(map[string]components.MCUPowerIntegrity, len(parent.record.MCU.PowerIntegrity))
	for _, evidence := range parent.record.MCU.PowerIntegrity {
		id := strings.TrimSpace(evidence.RailGroup)
		if id == "" {
			continue
		}
		if _, duplicate := evidenceByGroup[id]; duplicate {
			return nil, nil, nil, &mcuAssignmentError{
				Code: CodeMCUPowerIntegrityDomain, Text: "selected controller has duplicate power-integrity evidence for rail group " + id,
			}
		}
		evidenceByGroup[id] = evidence
	}
	railMinimumV, railMaximumV, railOK := mcuProgrammingTargetVoltageRange(request.Ports)
	if !railOK || !finitePositive(railMinimumV) || railMaximumV < railMinimumV {
		return nil, nil, nil, &mcuAssignmentError{
			Code: CodeMCUPowerIntegrityDomain, Text: "MCU power-integrity synthesis requires a bounded positive target supply range",
		}
	}
	ambientMinimumC, ambientMaximumC, err := mcuPowerAmbientRange(request.Constraints)
	if err != nil {
		return nil, nil, nil, err
	}

	var calculations []CalculationEvidence
	for _, group := range groups {
		if railMinimumV < group.MinimumV || railMaximumV > group.MaximumV {
			return nil, nil, nil, &mcuAssignmentError{
				Code: CodeMCUPowerIntegrityDomain,
				Text: fmt.Sprintf("target rail %.9g..%.9g V is outside MCU rail group %s range %.9g..%.9g V", railMinimumV, railMaximumV, group.ID, group.MinimumV, group.MaximumV),
			}
		}
		evidence, exists := evidenceByGroup[group.ID]
		if !exists {
			return nil, nil, nil, &mcuAssignmentError{
				Code: CodeMCUPowerIntegrityEvidence, Text: "selected controller lacks power-integrity evidence for rail group " + group.ID,
			}
		}
		envelope, envelopeErr := mcuResolvedPowerEnvelope(request.Constraints, evidence, ambientMinimumC, ambientMaximumC)
		if envelopeErr != nil {
			return nil, nil, nil, envelopeErr
		}
		sourceDropV := envelope.startupCurrentA * envelope.sourceImpedanceOhm
		brownoutBudgetV := railMinimumV - envelope.brownoutThresholdV - sourceDropV
		if !finitePositive(brownoutBudgetV) {
			return nil, nil, nil, &mcuAssignmentError{
				Code: CodeMCUPowerIntegrityBudget,
				Text: fmt.Sprintf("rail group %s has no positive brownout margin after %.9g V startup source drop", group.ID, sourceDropV),
			}
		}
		localBudgetV := math.Min(envelope.maximumNoiseV, brownoutBudgetV)
		bulkBudgetV := math.Min(envelope.maximumRippleV, brownoutBudgetV)
		if !finitePositive(localBudgetV) || !finitePositive(bulkBudgetV) {
			return nil, nil, nil, &mcuAssignmentError{
				Code: CodeMCUPowerIntegrityBudget, Text: "rail group " + group.ID + " has no positive local or bulk transient budget",
			}
		}

		groupDomains := mcuDomainsForGroup(parent.record.MCU, group.ID)
		if len(groupDomains) == 0 {
			return nil, nil, nil, &mcuAssignmentError{
				Code: CodeMCUPowerIntegrityDomain, Text: "rail group " + group.ID + " has no mapped supply domains",
			}
		}
		for _, domain := range groupDomains {
			powerFunction, groundFunction, functionOK := mcuDomainConnectionFunctions(domain)
			if !functionOK {
				return nil, nil, nil, &mcuAssignmentError{
					Code: CodeMCUPowerIntegrityDomain, Text: "supply domain " + domain.ID + " lacks deterministic power and ground functions",
				}
			}
			choice, selectErr := provider.selectMCUPowerCapacitor(ctx, mcuPowerCapacitorRequest{
				role: "local", transientCurrentA: envelope.transientCurrentA,
				durationS: envelope.localTransientDurationS, budgetV: localBudgetV,
				railMaximumV: railMaximumV, ambientMinimumC: envelope.ambientMinimumC,
				ambientMaximumC: envelope.ambientMaximumC,
			})
			if selectErr != nil {
				return nil, nil, nil, selectErr
			}
			instanceID := mcuPowerIntegrityIdentifier("local", parent.selected.InstanceID, group.ID, domain.ID)
			choice.part.selected.InstanceID = instanceID
			choice.part.usage = "decoupling_capacitor"
			choice.part.near = parent.selected.InstanceID
			choice.part.maxDistanceMM = envelope.localPlacementMM
			choice.part.parameters = mcuPowerPartParameters(choice, envelope.localTransientDurationS, localBudgetV, railMaximumV)
			choice.part.evidenceSources = append(choice.part.evidenceSources,
				"mcu_power_integrity:rail_group:"+group.ID,
				"mcu_power_integrity:supply_domain:"+domain.ID,
				"mcu_power_integrity:power_function:"+powerFunction,
				"mcu_power_integrity:ground_function:"+groundFunction,
			)
			parts = append(parts, choice.part)
			connections = appendMCUSupportConnection(connections, parent, powerFunction, endpoint(choice.part, "A"))
			connections = appendMCUSupportConnection(connections, parent, groundFunction, endpoint(choice.part, "B"))
			calculation, calculationErr := mcuPowerIntegrityCalculation(
				instanceID,
				railMinimumV, railMaximumV, sourceDropV,
				brownoutBudgetV, localBudgetV, envelope.transientCurrentA,
				envelope.localTransientDurationS, envelope, choice,
			)
			if calculationErr != nil {
				return nil, nil, nil, calculationErr
			}
			calculations = append(calculations, calculation)
		}

		powerFunction, groundFunction, functionOK := mcuGroupConnectionFunctions(group)
		if !functionOK {
			return nil, nil, nil, &mcuAssignmentError{
				Code: CodeMCUPowerIntegrityDomain, Text: "rail group " + group.ID + " lacks deterministic power and ground functions",
			}
		}
		choice, selectErr := provider.selectMCUPowerCapacitor(ctx, mcuPowerCapacitorRequest{
			role: "bulk", transientCurrentA: envelope.transientCurrentA,
			durationS: envelope.transientDurationS, budgetV: bulkBudgetV,
			railMaximumV: railMaximumV, ambientMinimumC: envelope.ambientMinimumC,
			ambientMaximumC: envelope.ambientMaximumC,
		})
		if selectErr != nil {
			return nil, nil, nil, selectErr
		}
		instanceID := mcuPowerIntegrityIdentifier("bulk", parent.selected.InstanceID, group.ID, "")
		choice.part.selected.InstanceID = instanceID
		choice.part.usage = "decoupling_capacitor"
		choice.part.near = parent.selected.InstanceID
		choice.part.maxDistanceMM = envelope.bulkPlacementMM
		choice.part.parameters = mcuPowerPartParameters(choice, envelope.transientDurationS, bulkBudgetV, railMaximumV)
		choice.part.evidenceSources = append(choice.part.evidenceSources,
			"mcu_power_integrity:rail_group:"+group.ID,
			"mcu_power_integrity:power_function:"+powerFunction,
			"mcu_power_integrity:ground_function:"+groundFunction,
		)
		parts = append(parts, choice.part)
		connections = appendMCUSupportConnection(connections, parent, powerFunction, endpoint(choice.part, "A"))
		connections = appendMCUSupportConnection(connections, parent, groundFunction, endpoint(choice.part, "B"))
		calculation, calculationErr := mcuPowerIntegrityCalculation(
			instanceID,
			railMinimumV, railMaximumV, sourceDropV,
			brownoutBudgetV, bulkBudgetV, envelope.transientCurrentA,
			envelope.transientDurationS, envelope, choice,
		)
		if calculationErr != nil {
			return nil, nil, nil, calculationErr
		}
		calculations = append(calculations, calculation)
	}
	return parts, connections, calculations, nil
}

func mcuPowerIntegrityIdentifier(kind, parentID, groupID, domainID string) string {
	parts := []string{"mcu_power", strings.TrimSpace(kind), strings.TrimSpace(parentID), strings.TrimSpace(groupID)}
	if domainID = strings.TrimSpace(domainID); domainID != "" {
		parts = append(parts, domainID)
	}
	return boundedMCUIdentifier(strings.Join(parts, "_"))
}

func mcuDomainsForGroup(evidence *components.MCUEvidence, groupID string) []components.MCUSupplyDomain {
	var domains []components.MCUSupplyDomain
	for _, domain := range evidence.SupplyDomains {
		candidate := strings.TrimSpace(domain.RailGroup)
		if candidate == "" {
			candidate = strings.TrimSpace(domain.ID)
		}
		if candidate == groupID {
			domains = append(domains, domain)
		}
	}
	slices.SortStableFunc(domains, func(left, right components.MCUSupplyDomain) int {
		return strings.Compare(left.ID, right.ID)
	})
	return domains
}

func mcuDomainConnectionFunctions(domain components.MCUSupplyDomain) (string, string, bool) {
	power := slices.Clone(domain.PowerFunctions)
	ground := slices.Clone(domain.GroundFunctions)
	slices.Sort(power)
	slices.Sort(ground)
	if len(power) == 0 || len(ground) == 0 {
		return "", "", false
	}
	return power[0], ground[0], true
}

func mcuGroupConnectionFunctions(group mcuSupplyGroup) (string, string, bool) {
	if len(group.PowerFunctions) == 0 || len(group.GroundFunctions) == 0 {
		return "", "", false
	}
	return group.PowerFunctions[0], group.GroundFunctions[0], true
}

func mcuResolvedPowerEnvelope(
	constraints []Constraint,
	evidence components.MCUPowerIntegrity,
	ambientMinimumC, ambientMaximumC float64,
) (mcuPowerEnvelope, error) {
	type quantity struct {
		name      string
		relation  string
		unit      string
		evidence  *components.EvidenceMeasurement
		target    *float64
		policy    string
		allowZero bool
	}
	envelope := mcuPowerEnvelope{ambientMinimumC: ambientMinimumC, ambientMaximumC: ambientMaximumC}
	quantities := []quantity{
		{"mcu_startup_current", "maximum", "A", evidence.StartupCurrent, &envelope.startupCurrentA, "stress", false},
		{"mcu_transient_current_step", "maximum", "A", evidence.TransientCurrentStep, &envelope.transientCurrentA, "stress", false},
		{"mcu_transient_duration", "maximum", "s", evidence.TransientDuration, &envelope.transientDurationS, "stress", false},
		{"mcu_local_transient_duration", "maximum", "s", evidence.LocalTransientDuration, &envelope.localTransientDurationS, "stress", false},
		{"maximum_supply_ripple", "maximum", "V", evidence.MaximumRipple, &envelope.maximumRippleV, "budget", false},
		{"maximum_supply_noise", "maximum", "V", evidence.MaximumNoise, &envelope.maximumNoiseV, "budget", false},
		{"mcu_brownout_voltage", "minimum", "V", evidence.BrownoutThreshold, &envelope.brownoutThresholdV, "minimum", false},
		{"power_source_impedance", "maximum", "Ohm", evidence.MaximumSourceImpedance, &envelope.sourceImpedanceOhm, "stress", true},
		{"", "", "mm", evidence.LocalPlacementMaximumMM, &envelope.localPlacementMM, "catalog", false},
		{"", "", "mm", evidence.BulkPlacementMaximumMM, &envelope.bulkPlacementMM, "catalog", false},
	}
	for _, item := range quantities {
		reviewed, ok := clockMeasurement(item.evidence, item.unit)
		if !ok || !finitePositive(reviewed) {
			return mcuPowerEnvelope{}, &mcuAssignmentError{
				Code: CodeMCUPowerIntegrityEvidence, Text: "selected controller has missing or invalid power-integrity measurement for " + item.name,
			}
		}
		selected := reviewed
		if item.name != "" {
			override, present, err := mcuPowerConstraint(constraints, item.name, item.relation, item.unit, item.allowZero)
			if err != nil {
				return mcuPowerEnvelope{}, err
			}
			if present {
				switch item.policy {
				case "stress", "budget":
					if override > reviewed {
						return mcuPowerEnvelope{}, &mcuAssignmentError{
							Code: CodeMCUPowerIntegrityEvidence,
							Text: fmt.Sprintf("%s %.9g %s exceeds reviewed envelope %.9g %s", item.name, override, item.unit, reviewed, item.unit),
						}
					}
				case "minimum":
					if override < reviewed {
						return mcuPowerEnvelope{}, &mcuAssignmentError{
							Code: CodeMCUPowerIntegrityEvidence,
							Text: fmt.Sprintf("%s %.9g %s is below reviewed minimum %.9g %s", item.name, override, item.unit, reviewed, item.unit),
						}
					}
				}
				selected = override
			}
		}
		*item.target = selected
	}
	return envelope, nil
}

func mcuPowerConstraint(constraints []Constraint, name, relation, unit string, allowZero bool) (float64, bool, error) {
	constraint, exists := namedConstraint(constraints, name)
	if !exists {
		return 0, false, nil
	}
	if constraint.Relation != relation || constraint.Unit != unit {
		return 0, true, &mcuAssignmentError{
			Code: CodeMCUPowerIntegrityEvidence,
			Text: fmt.Sprintf("constraint %s must use relation %s and unit %s", name, relation, unit),
		}
	}
	var value float64
	invalid := json.Unmarshal(constraint.Value, &value) != nil || math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value == 0 && !allowZero
	if invalid {
		requirement := "positive"
		if allowZero {
			requirement = "non-negative"
		}
		return 0, true, &mcuAssignmentError{
			Code: CodeMCUPowerIntegrityEvidence, Text: "constraint " + name + " requires a finite " + requirement + " numeric value",
		}
	}
	return value, true, nil
}

func mcuPowerAmbientRange(constraints []Constraint) (float64, float64, error) {
	minimum, maximum := 25.0, 25.0
	minimumPresent, maximumPresent := false, false
	for _, item := range []struct {
		name     string
		relation string
		target   *float64
		present  *bool
	}{
		{"ambient_temperature_minimum", "minimum", &minimum, &minimumPresent},
		{"ambient_temperature_maximum", "maximum", &maximum, &maximumPresent},
	} {
		constraint, exists := namedConstraint(constraints, item.name)
		if !exists {
			continue
		}
		if constraint.Relation != item.relation || constraint.Unit != "degC" {
			return 0, 0, &mcuAssignmentError{
				Code: CodeMCUPowerIntegrityEvidence,
				Text: fmt.Sprintf("constraint %s must use relation %s and unit degC", item.name, item.relation),
			}
		}
		var value float64
		if json.Unmarshal(constraint.Value, &value) != nil || math.IsNaN(value) || math.IsInf(value, 0) {
			return 0, 0, &mcuAssignmentError{
				Code: CodeMCUPowerIntegrityEvidence, Text: "constraint " + item.name + " requires a finite numeric value",
			}
		}
		*item.target = value
		*item.present = true
	}
	if maximumPresent && !minimumPresent && maximum < minimum {
		minimum = maximum
	}
	if minimumPresent && !maximumPresent && minimum > maximum {
		maximum = minimum
	}
	if maximum < minimum {
		return 0, 0, &mcuAssignmentError{
			Code: CodeMCUPowerIntegrityEvidence, Text: "ambient temperature range must be increasing",
		}
	}
	return minimum, maximum, nil
}

type mcuPowerCapacitorRequest struct {
	role              string
	transientCurrentA float64
	durationS         float64
	budgetV           float64
	railMaximumV      float64
	ambientMinimumC   float64
	ambientMaximumC   float64
}

func (provider *CatalogProvider) selectMCUPowerCapacitor(ctx context.Context, request mcuPowerCapacitorRequest) (mcuPowerCapacitorChoice, error) {
	var choices []mcuPowerCapacitorChoice
	rejectionCounts := map[string]int{}
	for _, record := range provider.familyRecords["capacitor"] {
		if err := ctx.Err(); err != nil {
			return mcuPowerCapacitorChoice{}, err
		}
		reject := func(reason string) {
			rejectionCounts[reason]++
		}
		if record.Generic || strings.TrimSpace(record.MPN) == "" ||
			confidenceRank(EvidenceConfidenceFromComponent(record.Verification.Confidence)) < confidenceRank(EvidenceVerified) {
			reject("concrete_verified_part_required")
			continue
		}
		evidence := record.Capacitor
		if evidence == nil || !evidence.FabricationProof || evidence.FabricationCandidateBlocks ||
			!containsEqualFold([]string{"proven", "not_applicable"}, evidence.DCBiasReview) ||
			!strings.EqualFold(strings.TrimSpace(evidence.EffectiveCapacitanceReview), "proven") ||
			!strings.EqualFold(strings.TrimSpace(evidence.ESRReview), "proven") ||
			!strings.EqualFold(strings.TrimSpace(evidence.VoltageDeratingReview), "proven") ||
			evidence.CapacitanceTolerancePct == nil || evidence.MaximumVoltageUseRatio == nil ||
			evidence.ESR == nil || evidence.RippleCurrent == nil {
			reject("complete_capacitor_evidence_required")
			continue
		}
		nominalCapacitanceF, capacitanceOK := recordValue(record, "capacitance", "F")
		esrOhm, esrOK := clockMeasurement(evidence.ESR, "Ohm")
		rippleCurrentA, rippleOK := mcuPowerRippleCurrent(evidence.RippleCurrent)
		voltageRatingV, voltageOK := recordRatingMaximum(record, "voltage", "V")
		temperatureMinimumC, temperatureMaximumC, temperatureOK := mcuPowerTemperatureRange(record)
		tolerancePct := *evidence.CapacitanceTolerancePct
		voltageDeratingFactor := *evidence.MaximumVoltageUseRatio
		effectiveCapacitanceF := nominalCapacitanceF * (1 - tolerancePct/100)
		if !capacitanceOK || !esrOK || !rippleOK || !voltageOK || !temperatureOK ||
			!finitePositive(effectiveCapacitanceF) || tolerancePct <= 0 || tolerancePct >= 100 ||
			!finitePositive(voltageDeratingFactor) || voltageDeratingFactor > 1 {
			reject("normalized_numeric_evidence_required")
			continue
		}
		if request.railMaximumV > voltageRatingV*voltageDeratingFactor {
			reject("voltage_derating")
			continue
		}
		if request.ambientMinimumC < temperatureMinimumC || request.ambientMaximumC > temperatureMaximumC {
			reject("temperature_range")
			continue
		}
		if request.transientCurrentA > rippleCurrentA {
			reject("ripple_current")
			continue
		}
		esrDroopV := request.transientCurrentA * esrOhm
		remainingBudgetV := request.budgetV - esrDroopV
		if !finitePositive(remainingBudgetV) {
			reject("esr_budget")
			continue
		}
		requiredCapacitanceF := request.transientCurrentA * request.durationS / remainingBudgetV
		observedDroopV := esrDroopV + request.transientCurrentA*request.durationS/effectiveCapacitanceF
		if !finitePositive(requiredCapacitanceF) || effectiveCapacitanceF < requiredCapacitanceF || observedDroopV > request.budgetV {
			reject("capacitance_budget")
			continue
		}
		for _, variant := range record.Packages {
			if variant.ID == "" || variant.DimensionsMM == nil ||
				confidenceRank(EvidenceConfidenceFromComponent(variant.Verification.Confidence)) < confidenceRank(EvidenceVerified) ||
				!mcuPowerVariantFunctions(variant) {
				continue
			}
			contractEvidence := componentEvidence(record, variant.Verification.Confidence)
			part := catalogPart{
				selected: SelectedComponent{CatalogID: record.ID, VariantID: variant.ID, Evidence: contractEvidence.Confidence},
				record:   record, value: engineeringValue(nominalCapacitanceF, "F"), evidence: contractEvidence,
				evidenceSources: slices.Clone(record.Verification.Sources),
			}
			choices = append(choices, mcuPowerCapacitorChoice{
				part: part, nominalCapacitanceF: nominalCapacitanceF, effectiveCapacitanceF: effectiveCapacitanceF,
				tolerancePct: tolerancePct, esrOhm: esrOhm, rippleCurrentA: rippleCurrentA,
				voltageRatingV: voltageRatingV, voltageDeratingFactor: voltageDeratingFactor,
				temperatureMinimumC: temperatureMinimumC,
				temperatureMaximumC: temperatureMaximumC, observedDroopV: observedDroopV,
				requiredCapacitanceF: requiredCapacitanceF,
			})
		}
	}
	slices.SortStableFunc(choices, func(left, right mcuPowerCapacitorChoice) int {
		leftArea := mcuPowerVariantArea(left.part)
		rightArea := mcuPowerVariantArea(right.part)
		if leftArea < rightArea {
			return -1
		}
		if leftArea > rightArea {
			return 1
		}
		if left.effectiveCapacitanceF < right.effectiveCapacitanceF {
			return -1
		}
		if left.effectiveCapacitanceF > right.effectiveCapacitanceF {
			return 1
		}
		if left.esrOhm < right.esrOhm {
			return -1
		}
		if left.esrOhm > right.esrOhm {
			return 1
		}
		if order := strings.Compare(left.part.record.ID, right.part.record.ID); order != 0 {
			return order
		}
		return strings.Compare(left.part.selected.VariantID, right.part.selected.VariantID)
	})
	if len(choices) == 0 {
		keys := make([]string, 0, len(rejectionCounts))
		for key := range rejectionCounts {
			keys = append(keys, key)
		}
		slices.Sort(keys)
		if len(keys) > mcuPowerCapacitorRejectionDetailLimit {
			keys = keys[:mcuPowerCapacitorRejectionDetailLimit]
		}
		details := make([]string, 0, len(keys))
		for _, key := range keys {
			details = append(details, fmt.Sprintf("%s=%d", key, rejectionCounts[key]))
		}
		return mcuPowerCapacitorChoice{}, &mcuAssignmentError{
			Code: CodeMCUDecouplingUnavailable,
			Text: fmt.Sprintf("no concrete capacitor closes the %s power-integrity envelope (%s)", request.role, strings.Join(details, ",")),
		}
	}
	return choices[0], nil
}

func mcuPowerRippleCurrent(evidence *components.EvidenceMeasurement) (float64, bool) {
	if evidence == nil {
		return 0, false
	}
	unit := strings.ToLower(strings.TrimSpace(evidence.Unit))
	if unit != "a" && unit != "a_rms" {
		return 0, false
	}
	return evidence.Value, finitePositive(evidence.Value)
}

func mcuPowerTemperatureRange(record components.ComponentRecord) (float64, float64, bool) {
	if record.Temperature == nil || record.Temperature.Unit != "C" {
		return 0, 0, false
	}
	minimum, minimumErr := strconv.ParseFloat(record.Temperature.Min, 64)
	maximum, maximumErr := strconv.ParseFloat(record.Temperature.Max, 64)
	return minimum, maximum, minimumErr == nil && maximumErr == nil &&
		finiteNumbers(minimum, maximum) && maximum >= minimum
}

func mcuPowerVariantFunctions(variant components.PackageVariant) bool {
	hasA, hasB := false, false
	for _, pad := range variant.PadFunctions {
		switch strings.ToUpper(strings.TrimSpace(pad.Function)) {
		case "A":
			hasA = true
		case "B":
			hasB = true
		}
	}
	return hasA && hasB
}

func mcuPowerVariantArea(part catalogPart) float64 {
	for _, variant := range part.record.Packages {
		if variant.ID == part.selected.VariantID && variant.DimensionsMM != nil {
			return variant.DimensionsMM.Width * variant.DimensionsMM.Height
		}
	}
	return math.Inf(1)
}

func mcuPowerPartParameters(choice mcuPowerCapacitorChoice, durationS, budgetV, railMaximumV float64) []RealizationParameter {
	return []RealizationParameter{
		{Name: "nominal_capacitance", Value: choice.nominalCapacitanceF, Unit: "F"},
		{Name: "effective_capacitance_minimum", Value: choice.effectiveCapacitanceF, Unit: "F"},
		{Name: "capacitance_tolerance", Value: choice.tolerancePct, Unit: "%"},
		{Name: "maximum_esr", Value: choice.esrOhm, Unit: "Ohm"},
		{Name: "ripple_current_rating", Value: choice.rippleCurrentA, Unit: "A"},
		{Name: "voltage_rating", Value: choice.voltageRatingV, Unit: "V"},
		{Name: "voltage_derating_factor", Value: choice.voltageDeratingFactor, Unit: "ratio"},
		{Name: "transient_duration", Value: durationS, Unit: "s"},
		{Name: "transient_droop_budget", Value: budgetV, Unit: "V"},
		{Name: "maximum_applied_voltage", Value: railMaximumV, Unit: "V"},
	}
}

func mcuPowerIntegrityCalculation(
	id string,
	railMinimumV, railMaximumV, sourceDropV, brownoutBudgetV, transientBudgetV,
	transientCurrentA, durationS float64,
	envelope mcuPowerEnvelope,
	choice mcuPowerCapacitorChoice,
) (CalculationEvidence, error) {
	bounds := []CalculationBound{
		maximumBound("startup_source_drop", railMinimumV-envelope.brownoutThresholdV, sourceDropV, "V"),
		maximumBound("transient_droop", transientBudgetV, choice.observedDroopV, "V"),
		minimumBound("effective_capacitance", choice.requiredCapacitanceF, choice.effectiveCapacitanceF, "F"),
		minimumBound("derated_voltage_rating", railMaximumV, choice.voltageRatingV*choice.voltageDeratingFactor, "V"),
		minimumBound("ripple_current_rating", transientCurrentA, choice.rippleCurrentA, "A"),
		maximumBound("minimum_operating_temperature", envelope.ambientMinimumC, choice.temperatureMinimumC, "C"),
		minimumBound("maximum_operating_temperature", envelope.ambientMaximumC, choice.temperatureMaximumC, "C"),
	}
	worstMargin, pass := normalizedBoundsMargin(bounds)
	if !pass {
		return CalculationEvidence{}, &mcuAssignmentError{
			Code: CodeMCUPowerIntegrityBudget, Text: "selected capacitor failed finalized MCU power-integrity bounds",
		}
	}
	calculation := CalculationEvidence{
		ID: id, FormulaID: FormulaRatingMargin, FormulaRevision: FormulaRevision,
		Inputs: []NamedQuantity{
			{Name: "rail_minimum", Value: railMinimumV, Unit: "V"},
			{Name: "rail_maximum", Value: railMaximumV, Unit: "V"},
			{Name: "startup_current", Value: envelope.startupCurrentA, Unit: "A"},
			{Name: "transient_current_step", Value: transientCurrentA, Unit: "A"},
			{Name: "transient_duration", Value: durationS, Unit: "s"},
			{Name: "source_impedance", Value: envelope.sourceImpedanceOhm, Unit: "Ohm"},
			{Name: "maximum_ripple", Value: envelope.maximumRippleV, Unit: "V"},
			{Name: "maximum_noise", Value: envelope.maximumNoiseV, Unit: "V"},
			{Name: "brownout_threshold", Value: envelope.brownoutThresholdV, Unit: "V"},
			{Name: "ambient_minimum", Value: envelope.ambientMinimumC, Unit: "C"},
			{Name: "ambient_maximum", Value: envelope.ambientMaximumC, Unit: "C"},
		},
		SelectedValues: []SelectedValueEvidence{{
			Name: "capacitance", Ideal: choice.requiredCapacitanceF, Selected: choice.nominalCapacitanceF,
			Unit: "F", TolerancePercent: choice.tolerancePct,
			RelativeError: (choice.nominalCapacitanceF - choice.requiredCapacitanceF) / choice.requiredCapacitanceF,
		}},
		NominalOutputs: []NamedQuantity{
			{Name: "source_drop", Value: sourceDropV, Unit: "V"},
			{Name: "brownout_budget", Value: brownoutBudgetV, Unit: "V"},
			{Name: "transient_budget", Value: transientBudgetV, Unit: "V"},
			{Name: "required_capacitance", Value: choice.requiredCapacitanceF, Unit: "F"},
			{Name: "selected_nominal_capacitance", Value: choice.nominalCapacitanceF, Unit: "F"},
			{Name: "selected_effective_capacitance", Value: choice.effectiveCapacitanceF, Unit: "F"},
			{Name: "selected_esr", Value: choice.esrOhm, Unit: "Ohm"},
			{Name: "observed_transient_droop", Value: choice.observedDroopV, Unit: "V"},
		},
		Bounds: bounds, WorstMargin: worstMargin, Pass: true,
	}
	finalized, err := FinalizeCalculation(calculation)
	if err != nil {
		return CalculationEvidence{}, fmt.Errorf("finalize MCU power-integrity calculation: %w", err)
	}
	return finalized, nil
}
