package amplifiers

import (
	"sort"
	"strings"
	"unicode"

	"kicadai/internal/blocks"
	"kicadai/internal/routing"
)

var opAmpConstraintEvidenceCategories = map[string][]string{
	"opamp_feedback_proximity":          {"feedback"},
	"opamp_supply_decoupling_proximity": {"power_supply"},
	"opamp_input_output_separation":     {"analog_input", "high_current_output"},
	"opamp_output_resistor_pairing":     {"high_current_output"},
	"opamp_output_min_width":            {"high_current_output"},
	"opamp_thermal_edge_preference":     nil,
}

type netEvidenceFlags uint32

const (
	netEvidenceAnalogInput netEvidenceFlags = 1 << iota
	netEvidenceFeedback
	netEvidenceHighCurrentOutput
	netEvidencePowerSupply
)

// PCBConstraintEvidence summarizes amplifier-specific layout and routing
// evidence without claiming KiCad DRC or analog performance proof.
type PCBConstraintEvidence struct {
	MissingConstraints []string
	MissingNetEvidence []string
	IncompleteRoutes   []string
	Blockers           []string
}

// OK returns true when all required amplifier PCB evidence is present and no
// incomplete amplifier routes were found.
func (e PCBConstraintEvidence) OK() bool {
	return len(e.MissingConstraints) == 0 &&
		len(e.MissingNetEvidence) == 0 &&
		len(e.IncompleteRoutes) == 0 &&
		len(e.Blockers) == 0
}

// OpAmpPCBConstraintIDs lists the op-amp layout-intent constraints that must
// be carried by the built-in op-amp gain-stage PCB realization.
func OpAmpPCBConstraintIDs() []string {
	ids := make([]string, 0, len(opAmpConstraintEvidenceCategories))
	for id := range opAmpConstraintEvidenceCategories {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// ValidatePCBConstraintEvidence checks that amplifier PCB constraint metadata
// and routing quality reports preserve the intent needed for later KiCad-backed
// validation. Missing or incomplete evidence is reported as a blocker because a
// parseable board can still be electrically wrong.
func ValidatePCBConstraintEvidence(realization *blocks.PCBRealization, quality routing.QualityReport) PCBConstraintEvidence {
	required := requiredNetEvidenceFlags(realization)
	found, incomplete := scanAmplifierRoutingEvidence(quality)
	evidence := PCBConstraintEvidence{
		MissingConstraints: missingConstraintIDs(realization, OpAmpPCBConstraintIDs()),
		MissingNetEvidence: missingAmplifierNetEvidence(required, found),
		IncompleteRoutes:   incomplete,
	}
	for _, id := range evidence.MissingConstraints {
		evidence.Blockers = append(evidence.Blockers, "missing amplifier PCB constraint "+id)
	}
	for _, id := range evidence.MissingNetEvidence {
		evidence.Blockers = append(evidence.Blockers, "missing amplifier routing net evidence "+id)
	}
	for _, net := range evidence.IncompleteRoutes {
		evidence.Blockers = append(evidence.Blockers, "amplifier route is incomplete for net "+net)
	}
	sort.Strings(evidence.Blockers)
	return evidence
}

func missingConstraintIDs(realization *blocks.PCBRealization, required []string) []string {
	seen := map[string]bool{}
	if realization != nil {
		for _, constraint := range realization.Constraints {
			seen[normalizeEvidenceText(constraint.ID)] = true
		}
	}
	var missing []string
	for _, id := range required {
		if !seen[id] {
			missing = append(missing, id)
		}
	}
	return missing
}

func missingAmplifierNetEvidence(required netEvidenceFlags, found netEvidenceFlags) []string {
	ordered := []struct {
		name string
		flag netEvidenceFlags
	}{
		{name: "analog_input", flag: netEvidenceAnalogInput},
		{name: "feedback", flag: netEvidenceFeedback},
		{name: "high_current_output", flag: netEvidenceHighCurrentOutput},
		{name: "power_supply", flag: netEvidencePowerSupply},
	}
	var missing []string
	for _, item := range ordered {
		if required.has(item.flag) && !found.has(item.flag) {
			missing = append(missing, item.name)
		}
	}
	return missing
}

func requiredNetEvidenceFlags(realization *blocks.PCBRealization) netEvidenceFlags {
	var required netEvidenceFlags
	if realization == nil {
		return 0
	}
	for _, constraint := range realization.Constraints {
		id := normalizeEvidenceText(constraint.ID)
		for _, category := range opAmpConstraintEvidenceCategories[id] {
			required |= netEvidenceFlag(category)
		}
	}
	return required
}

func scanAmplifierRoutingEvidence(quality routing.QualityReport) (netEvidenceFlags, []string) {
	var found netEvidenceFlags
	var nets []string
	for _, net := range quality.NetReports {
		flags := classifyAmplifierNet(net)
		found |= flags
		if flags != 0 && (net.Status != routing.RouteStatusRouted || net.RoutedEndpoints < net.EndpointCount) {
			nets = append(nets, net.NetName)
		}
	}
	sort.Strings(nets)
	return found, nets
}

func classifyAmplifierNet(net routing.NetQualityReport) netEvidenceFlags {
	name := normalizeEvidenceText(net.NetName)
	class := normalizeEvidenceText(net.Class)
	var flags netEvidenceFlags
	if hasEvidencePhrase(class, "analog_input") || net.Role == routing.NetAnalog && hasAnyEvidenceToken(name, "in", "input", "vin") {
		flags |= netEvidenceAnalogInput
	}
	if hasAnyEvidenceToken(name, "feedback", "fb") || hasAnyEvidenceToken(class, "feedback", "fb") {
		flags |= netEvidenceFeedback
	}
	if net.Role == routing.NetHighCurrent || hasEvidencePhrase(class, "high_current") || hasEvidencePhrase(class, "headphone_output") {
		flags |= netEvidenceHighCurrentOutput
	}
	if net.Role == routing.NetPower || net.Role == routing.NetGround || hasAnyEvidenceToken(name, "vcc", "vdd", "vee", "vss", "vpp", "gnd", "5v", "3v3", "3_3v") || hasEvidenceToken(class, "supply") {
		flags |= netEvidencePowerSupply
	}
	return flags
}

func netEvidenceFlag(category string) netEvidenceFlags {
	switch category {
	case "analog_input":
		return netEvidenceAnalogInput
	case "feedback":
		return netEvidenceFeedback
	case "high_current_output":
		return netEvidenceHighCurrentOutput
	case "power_supply":
		return netEvidencePowerSupply
	default:
		return 0
	}
}

func (flags netEvidenceFlags) has(flag netEvidenceFlags) bool {
	return flags&flag != 0
}

func normalizeEvidenceText(value string) string {
	var builder strings.Builder
	builder.Grow(len(value))
	lastDelimiter := false
	for _, char := range value {
		char = unicode.ToLower(char)
		if (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') {
			builder.WriteRune(char)
			lastDelimiter = false
			continue
		}
		if !lastDelimiter {
			builder.WriteByte('_')
			lastDelimiter = true
		}
	}
	return strings.Trim(builder.String(), "_")
}

func hasEvidenceToken(value string, token string) bool {
	return hasEvidencePhrase(value, token)
}

func hasAnyEvidenceToken(value string, tokens ...string) bool {
	for _, token := range tokens {
		if hasEvidenceToken(value, token) {
			return true
		}
	}
	return false
}

func hasEvidencePhrase(value string, phrase string) bool {
	if phrase == "" {
		return false
	}
	if value == phrase {
		return true
	}
	index := 0
	for {
		found := strings.Index(value[index:], phrase)
		if found < 0 {
			return false
		}
		start := index + found
		end := start + len(phrase)
		leftOK := start == 0 || evidenceBoundary(value[start-1], value[start])
		rightOK := end == len(value) || evidenceBoundary(value[end-1], value[end])
		if leftOK && rightOK {
			return true
		}
		index = end
		if index >= len(value) {
			return false
		}
	}
}

func evidenceBoundary(left byte, right byte) bool {
	if left == '_' || right == '_' {
		return true
	}
	return asciiDigit(left) != asciiDigit(right)
}

func asciiDigit(value byte) bool {
	return value >= '0' && value <= '9'
}
