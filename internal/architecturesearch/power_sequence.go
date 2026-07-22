package architecturesearch

import (
	"encoding/json"
	"math"
)

type railSequenceDelay struct {
	Before  string  `json:"before"`
	After   string  `json:"after"`
	Seconds float64 `json:"seconds"`
}

func validatePowerSequenceConstraint(requirement Requirement, selections []FragmentSelection, constraint Constraint, path string) (GlobalCheck, *candidateValidation) {
	reject := func(message string) (GlobalCheck, *candidateValidation) {
		return GlobalCheck{}, &candidateValidation{Code: CodePowerSequenceUnproven, Path: path, Message: message}
	}
	switch constraint.Name {
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
		return reject("selected producer evidence does not prove monotonic startup")
	case "startup_inrush_current":
		return reject("selected producer evidence does not bound startup inrush current")
	default:
		return reject("unsupported power-sequence constraint")
	}
}

func railStartupTime(requirement Requirement, selections []FragmentSelection, signal string) (float64, bool) {
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
				if output.Name == "startup_time" && canonicalUnit(output.Unit) == "s" && output.Value >= 0 && !math.IsNaN(output.Value) && !math.IsInf(output.Value, 0) {
					return output.Value, true
				}
			}
		}
	}
	return 0, false
}
