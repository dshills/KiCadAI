package schematiclayout

import "strings"

func Classify(request Request) Request {
	request = NormalizeRequest(request)
	for index := range request.Components {
		component := &request.Components[index]
		component.Role = InferComponentRole(*component)
		if component.Stage == StageUnknown {
			component.Stage = StageForRole(component.Role)
		}
		if component.Lane == LaneUnknown {
			component.Lane = LaneForRole(component.Role)
		}
	}
	for index := range request.Nets {
		net := &request.Nets[index]
		net.Role = InferNetRole(*net)
	}
	for index := range request.Groups {
		group := &request.Groups[index]
		if group.Stage == StageUnknown {
			group.Stage = StageForRole(group.Role)
		}
	}
	return NormalizeRequest(request)
}

func InferComponentRole(component Component) string {
	role := normalizeRole(component.Role)
	if role != "" {
		return role
	}
	libraryID := normalizeRole(component.LibraryID)
	value := normalizeRole(component.Value)
	ref := strings.ToUpper(strings.TrimSpace(component.Ref))
	switch {
	case strings.HasPrefix(ref, "J"), containsNormalizedRole(libraryID, "connector"):
		if containsNormalizedRole(value, "gnd", "ground", "return") {
			return "ground_connector"
		}
		if containsNormalizedRole(value, "power", "vcc", "vin") {
			return "power_connector"
		}
		if containsNormalizedRole(value, "out", "output", "load", "headphone") {
			return "output_connector"
		}
		return "input_connector"
	case containsNormalizedRole(value, "gnd", "ground"):
		return "ground"
	case containsNormalizedRole(value, "vcc", "vin", "vdd", "positive"):
		return "positive_rail"
	case containsNormalizedRole(value, "vee", "vss", "negative"):
		return "negative_rail"
	case strings.HasPrefix(ref, "U") && containsNormalizedRole(libraryID, "opamp", "amplifier"):
		return "opamp"
	case strings.HasPrefix(ref, "U") && containsNormalizedRole(value, "mcu", "controller", "atmega"):
		return "mcu"
	case strings.HasPrefix(ref, "Q") && containsNormalizedRole(value, "output", "driver", "buffer"):
		return "output_stage"
	case strings.HasPrefix(ref, "Q"):
		return "transistor"
	case strings.HasPrefix(ref, "R") && containsNormalizedRole(value, "feedback", "fb"):
		return "feedback"
	case strings.HasPrefix(ref, "R") && containsNormalizedRole(value, "bias", "ref"):
		return "bias"
	case strings.HasPrefix(ref, "R"):
		return "resistor"
	case strings.HasPrefix(ref, "C") && containsNormalizedRole(value, "decoupling", "bypass", "100n"):
		return "decoupling"
	case strings.HasPrefix(ref, "C"):
		return "capacitor"
	case strings.HasPrefix(ref, "D") && containsNormalizedRole(value, "tvs", "zener", "esd", "protection"):
		return "protection"
	case strings.HasPrefix(ref, "D") && containsNormalizedRole(value, "led"):
		return "indicator"
	case strings.HasPrefix(ref, "D"):
		return "diode"
	default:
		return "component"
	}
}

func InferNetRole(net Net) string {
	role := normalizeRole(net.Role)
	if role != "" {
		return role
	}
	name := normalizeRole(net.Name)
	switch {
	case isGroundNetName(name):
		return "ground"
	case containsNormalizedRole(name, "vcc", "vdd", "vin", "power", "3v3", "5v"):
		return "power"
	case containsNormalizedRole(name, "vee", "vss", "negative"):
		return "negative_rail"
	case containsNormalizedRole(name, "out", "output", "hp_out", "load"):
		return "output_signal"
	case containsNormalizedRole(name, "in", "input", "audio_in"):
		return "input_signal"
	case containsNormalizedRole(name, "scl", "sda", "i2c", "bus"):
		return "bus"
	case containsNormalizedRole(name, "feedback", "fb"):
		return "feedback"
	default:
		return "signal"
	}
}

func isGroundNetName(name string) bool {
	return name == "gnd" ||
		strings.HasSuffix(name, "gnd") ||
		strings.HasPrefix(name, "gnd_") ||
		strings.Contains(name, "_gnd_") ||
		containsNormalizedRole(name, "ground", "return")
}

func StageForRole(role string) Stage {
	role = normalizeRole(role)
	switch {
	case containsNormalizedRole(role, "input_connector", "power_connector"):
		return StageBoundaryInput
	case containsNormalizedRole(role, "protection", "conditioning"):
		return StageConditioning
	case containsNormalizedRole(role, "output_connector", "load"):
		return StageBoundaryOutput
	case containsNormalizedRole(role, "driver", "output_stage", "output_resistor"):
		return StageDriverOutput
	default:
		return StageProcessing
	}
}

func LaneForRole(role string) Lane {
	role = normalizeRole(role)
	switch {
	case containsNormalizedRole(role, "positive_rail", "power_connector", "vcc", "vdd", "vin", "decoupling"):
		return LanePositiveRail
	case containsNormalizedRole(role, "negative_rail", "vee", "vss"):
		return LaneNegativeRail
	case containsNormalizedRole(role, "ground", "return", "load"):
		return LaneGround
	case containsNormalizedRole(role, "bias", "reference"):
		return LaneReference
	default:
		return LaneSignal
	}
}
