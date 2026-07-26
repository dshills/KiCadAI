package architecturesearch

import (
	"encoding/json"
	"math"
	"strings"
)

type railSequenceDelay struct {
	Before  string  `json:"before"`
	After   string  `json:"after"`
	Seconds float64 `json:"seconds"`
}

type railStartupInrush struct {
	Signal   string  `json:"signal"`
	CurrentA float64 `json:"current_a"`
}

func validatePowerSequenceConstraint(requirement Requirement, selections []FragmentSelection, constraint Constraint, path string) (GlobalCheck, *candidateValidation) {
	reject := func(message string) (GlobalCheck, *candidateValidation) {
		return GlobalCheck{}, &candidateValidation{Code: CodePowerSequenceUnproven, Path: path, Message: message}
	}
	switch constraint.Name {
	case "startup_order", "shutdown_order":
		beforeAnchor, afterAnchor, ok := orderedRailAnchors(requirement, constraint)
		if constraint.Relation != "required" || !ok {
			return reject("rail order requires a canonical '<rail>_before_<rail>' pair")
		}
		proof, proofOK := supervisedRailSequenceProof(selections)
		if !proofOK {
			return reject("selected rail sequencer lacks topology-bound startup and shutdown evidence")
		}
		expectedBefore, expectedAfter := proof.railAAnchor, proof.railBAnchor
		orderOutput := "startup_order_role_a_before_role_b"
		delay := proof.startupDelayS
		if constraint.Name == "shutdown_order" {
			expectedBefore, expectedAfter = proof.railBAnchor, proof.railAAnchor
			orderOutput = "shutdown_order_role_b_before_role_a"
			delay = proof.shutdownDelayS
		}
		if beforeAnchor != expectedBefore || afterAnchor != expectedAfter || proof.outputs[orderOutput] < 1 || !finitePositive(delay) {
			return reject("selected rail-sequencer direction does not prove the requested rail order")
		}
		return GlobalCheck{
			Code: CodePowerSequenceUnproven, Path: path,
			Message:  "selected topology and bounded timing evidence prove the requested rail order",
			Required: float64Pointer(0), Observed: float64Pointer(delay), Margin: float64Pointer(delay),
		}, nil
	case "rail_sequence_before":
		var signals []string
		if (constraint.Relation != "required" && constraint.Relation != "before") || json.Unmarshal(constraint.Value, &signals) != nil || len(signals) != 2 {
			return reject("rail sequence requires an ordered pair of rail signal identifiers")
		}
		before, beforeOK := railStartupTime(requirement, selections, signals[0])
		after, afterOK := railStartupTime(requirement, selections, signals[1])
		if !beforeOK || !afterOK || before > after {
			return reject("selected rail producers do not prove the requested startup order")
		}
		margin := after - before
		return GlobalCheck{Code: CodePowerSequenceUnproven, Path: path, Message: "selected producer startup evidence proves the requested rail order", Required: float64Pointer(0), Observed: float64Pointer(margin), Margin: float64Pointer(margin)}, nil
	case "rail_sequence_delay":
		var delay railSequenceDelay
		if json.Unmarshal(constraint.Value, &delay) != nil || canonicalUnit(constraint.Unit) != "s" || !finitePositive(delay.Seconds) || (constraint.Relation != "minimum" && constraint.Relation != "maximum") {
			return reject("rail sequence delay requires before/after signals, positive seconds, and a minimum or maximum relation")
		}
		before, beforeOK := railStartupTime(requirement, selections, delay.Before)
		after, afterOK := railStartupTime(requirement, selections, delay.After)
		observed := after - before
		if !beforeOK || !afterOK || observed < 0 {
			return reject("selected rail producers lack ordered startup timing evidence")
		}
		margin := observed - delay.Seconds
		if constraint.Relation == "maximum" {
			margin = delay.Seconds - observed
		}
		if margin < 0 {
			return reject("selected producer startup timing violates the requested rail delay")
		}
		return GlobalCheck{Code: CodePowerSequenceUnproven, Path: path, Message: "selected producer startup timing satisfies the requested rail delay", Required: float64Pointer(delay.Seconds), Observed: float64Pointer(observed), Margin: float64Pointer(margin)}, nil
	case "startup_monotonic":
		var signal string
		observed, ok := railCalculationOutput(requirement, selections, signalFromConstraint(constraint, &signal), "startup_monotonic", "ratio")
		if constraint.Relation != "required" || !ok || observed < 1 {
			return reject("selected producer evidence does not prove monotonic startup")
		}
		return GlobalCheck{Code: CodePowerSequenceUnproven, Path: path, Message: "selected producer evidence proves monotonic startup", Required: float64Pointer(1), Observed: float64Pointer(observed), Margin: float64Pointer(observed - 1)}, nil
	case "startup_inrush_current":
		var bound railStartupInrush
		if json.Unmarshal(constraint.Value, &bound) != nil || constraint.Relation != "maximum" || canonicalUnit(constraint.Unit) != "A" || canonicalIdentifier(bound.Signal) == "" || !finitePositive(bound.CurrentA) {
			return reject("startup inrush requires a named rail, a positive current_a, maximum relation, and ampere unit")
		}
		observed, ok := railCalculationOutput(requirement, selections, bound.Signal, "startup_inrush_current", "A")
		if !ok || observed > bound.CurrentA {
			return reject("selected producer evidence does not bound startup inrush current")
		}
		margin := bound.CurrentA - observed
		return GlobalCheck{Code: CodePowerSequenceUnproven, Path: path, Message: "selected producer evidence satisfies the startup inrush limit", Required: float64Pointer(bound.CurrentA), Observed: float64Pointer(observed), Margin: float64Pointer(margin)}, nil
	default:
		return reject("unsupported power-sequence constraint")
	}
}

type railSequenceProof struct {
	railAAnchor    string
	railBAnchor    string
	startupDelayS  float64
	shutdownDelayS float64
	outputs        map[string]float64
}

func orderedRailAnchors(requirement Requirement, constraint Constraint) (string, string, bool) {
	var order string
	if json.Unmarshal(constraint.Value, &order) != nil {
		return "", "", false
	}
	parts := strings.Split(order, "_before_")
	if len(parts) != 2 || canonicalIdentifier(parts[0]) == "" || canonicalIdentifier(parts[1]) == "" {
		return "", "", false
	}
	before, beforeOK := railOrderAnchor(requirement, parts[0])
	after, afterOK := railOrderAnchor(requirement, parts[1])
	return before, after, beforeOK && afterOK && before != after
}

func railOrderAnchor(requirement Requirement, token string) (string, bool) {
	token = canonicalIdentifier(token)
	for _, signal := range requirement.Requirements.Signals {
		if canonicalIdentifier(signal.ID) == token {
			return signalAnchor(signal.ID), true
		}
	}
	for _, domain := range requirement.Requirements.Domains {
		if canonicalIdentifier(domain.ID) != token {
			continue
		}
		for _, signal := range requirement.Requirements.Signals {
			if canonicalIdentifier(signal.ID) == canonicalIdentifier(domain.Source) {
				return signalAnchor(signal.ID), true
			}
		}
		return domainAnchor(domain.ID), true
	}
	for _, port := range requirement.Requirements.Ports {
		if canonicalIdentifier(port.ID) == token {
			return externalAnchor(port.ID), true
		}
	}
	return "", false
}

func supervisedRailSequenceProof(selections []FragmentSelection) (railSequenceProof, bool) {
	for _, selection := range selections {
		if canonicalIdentifier(selection.Capability) != "rail_sequencing" {
			continue
		}
		realization, err := DecodeFragmentRealization(selection.Payload)
		if err != nil || canonicalIdentifier(realization.Capability) != "rail_sequencing" {
			continue
		}
		roles := map[string]RealizationPortBinding{}
		for _, binding := range realization.PortBindings {
			roles[canonicalIdentifier(binding.Role)] = binding
		}
		if roles["rail_a"].Instance == "" || roles["rail_b"].Instance == "" ||
			roles["enable"].Instance == "" || roles["state"].Instance == "" ||
			roles["reference"].Instance == "" ||
			(roles["rail_a"].Instance == roles["rail_b"].Instance && roles["rail_a"].Function == roles["rail_b"].Function) {
			continue
		}
		anchors := map[string]string{}
		for _, port := range selection.Ports {
			anchors[canonicalIdentifier(port.Role)] = port.Anchor
		}
		if anchors["rail_a"] == "" || anchors["rail_b"] == "" || anchors["rail_a"] == anchors["rail_b"] {
			continue
		}
		outputs := map[string]float64{}
		for _, calculation := range selection.Calculations {
			if calculation.FormulaID != FormulaRCSequenceDelay || !calculation.Pass {
				continue
			}
			for _, output := range calculation.NominalOutputs {
				if finiteNumbers(output.Value) {
					outputs[output.Name] = output.Value
				}
			}
		}
		startupDelayS := outputs["startup_sequence_delay"]
		shutdownDelayS := outputs["shutdown_sequence_delay"]
		if outputs["startup_order_role_a_before_role_b"] < 1 ||
			outputs["shutdown_order_role_b_before_role_a"] < 1 ||
			!finitePositive(startupDelayS) || !finitePositive(shutdownDelayS) {
			continue
		}
		return railSequenceProof{
			railAAnchor: anchors["rail_a"], railBAnchor: anchors["rail_b"],
			startupDelayS: startupDelayS, shutdownDelayS: shutdownDelayS,
			outputs: outputs,
		}, true
	}
	return railSequenceProof{}, false
}

func railStartupTime(requirement Requirement, selections []FragmentSelection, signal string) (float64, bool) {
	return railCalculationOutput(requirement, selections, signal, "startup_time", "s")
}

func signalFromConstraint(constraint Constraint, signal *string) string {
	if json.Unmarshal(constraint.Value, signal) != nil {
		return ""
	}
	return *signal
}

func railCalculationOutput(requirement Requirement, selections []FragmentSelection, signal, outputName, unit string) (float64, bool) {
	producer, ok := powerSignalProducer(requirement, canonicalIdentifier(signal))
	if !ok {
		return 0, false
	}
	path := "objective:" + producer.ID
	for _, selection := range selections {
		if selection.ObligationPath != path {
			continue
		}
		for _, calculation := range selection.Calculations {
			for _, output := range calculation.NominalOutputs {
				if output.Name == outputName && canonicalUnit(output.Unit) == canonicalUnit(unit) && output.Value >= 0 && !math.IsNaN(output.Value) && !math.IsInf(output.Value, 0) {
					return output.Value, true
				}
			}
		}
	}
	return 0, false
}
