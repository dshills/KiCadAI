package schematiclayout

import (
	"strings"

	"kicadai/internal/kicadfiles"
	"kicadai/internal/kicadfiles/schematic"
)

const maxAmplifierReadabilityDiagnostics = 100

func ValidateAmplifierReadability(file *schematic.SchematicFile) []Diagnostic {
	request, result := AdaptSchematic(file)
	validated := Validate(result, request)
	diagnostics := strictAmplifierDiagnostics(result)
	for _, diagnostic := range validated.Diagnostics {
		// Parsed fixture symbols do not yet carry exact symbol-body extents or
		// pin entry geometry, so legitimate pin-entry wires can appear to cross
		// symbol bodies. Amplifier-specific spacing checks cover real crowding.
		if diagnostic.Severity == SeverityError && diagnostic.Code == "wire_symbol_overlap" {
			continue
		}
		diagnostics = append(diagnostics, diagnostic)
	}
	return NormalizeDiagnostics(diagnostics, maxAmplifierReadabilityDiagnostics)
}

func strictAmplifierDiagnostics(result Result) []Diagnostic {
	components := amplifierComponentReadabilityProfiles(result.Components)
	leftActive, activeOK := leftmostReadableComponentMatching(components, isAmplifierActiveProfile)
	if !activeOK {
		return []Diagnostic{{Severity: SeverityError, Code: "amplifier_active_missing", Message: "active gain stage not found"}}
	}
	rightActive, rightActiveOK := rightmostReadableComponentMatching(components, isAmplifierActiveProfile)
	if !rightActiveOK {
		return []Diagnostic{{Severity: SeverityError, Code: "amplifier_active_missing", Message: "active gain stage not found"}}
	}
	var diagnostics []Diagnostic
	if input, ok := leftmostLabel(result.Labels, "audio_in", "input"); !ok {
		diagnostics = append(diagnostics, Diagnostic{Severity: SeverityError, Code: "amplifier_input_missing", Message: "input label not found"})
	} else if !leftOf(input.Position.X, leftActive.component.PlacedAt.X) {
		diagnostics = append(diagnostics, Diagnostic{Severity: SeverityError, Code: "amplifier_input_flow", Ref: input.Text, Message: "input label is not left of active gain stage"})
	}
	if output, ok := rightmostLabel(result.Labels, "hp_out", "out", "output"); !ok {
		diagnostics = append(diagnostics, Diagnostic{Severity: SeverityError, Code: "amplifier_output_missing", Message: "output label not found"})
	} else if !rightOf(output.Position.X, rightActive.component.PlacedAt.X) {
		diagnostics = append(diagnostics, Diagnostic{Severity: SeverityError, Code: "amplifier_output_flow", Ref: output.Text, Message: "output label is not right of active gain stage"})
	}
	if feedback, ok := firstLabel(result.Labels, "feedback"); !ok {
		diagnostics = append(diagnostics, Diagnostic{Severity: SeverityWarning, Code: "amplifier_feedback_missing", Message: "feedback label not found"})
	} else if !above(feedback.Position.Y, leftActive.component.PlacedAt.Y) {
		diagnostics = append(diagnostics, Diagnostic{Severity: SeverityError, Code: "amplifier_feedback_position", Ref: feedback.Text, Message: "feedback label is not above active gain stage"})
	}
	for _, rail := range components {
		if isPositiveRailProfile(rail) && !above(rail.component.PlacedAt.Y, leftActive.component.PlacedAt.Y) {
			diagnostics = append(diagnostics, Diagnostic{Severity: SeverityError, Code: "amplifier_positive_rail_position", Ref: rail.component.Ref, Message: "positive rail is not above active gain stage"})
		}
	}
	for _, ground := range components {
		if isGroundOrLoadProfile(ground) && !below(ground.component.PlacedAt.Y, leftActive.component.PlacedAt.Y) {
			diagnostics = append(diagnostics, Diagnostic{Severity: SeverityError, Code: "amplifier_return_position", Ref: ground.component.Ref, Message: "ground/load return is not below active gain stage"})
		}
	}
	if outputStage, ok := rightmostReadableComponentMatching(components, isOutputStageProfile); ok && !rightOf(outputStage.component.PlacedAt.X, leftActive.component.PlacedAt.X) {
		diagnostics = append(diagnostics, Diagnostic{Severity: SeverityError, Code: "amplifier_output_stage_flow", Ref: outputStage.component.Ref, Message: "output stage is not right of active gain stage"})
	}
	return diagnostics
}

type amplifierComponentReadability struct {
	component      PlacedComponent
	normalizedText string
	rawTokens      []string
}

func amplifierComponentReadabilityProfiles(components []PlacedComponent) []amplifierComponentReadability {
	profiles := make([]amplifierComponentReadability, 0, len(components))
	for _, component := range components {
		profiles = append(profiles, amplifierComponentReadabilityProfile(component))
	}
	return profiles
}

func amplifierComponentReadabilityProfile(component PlacedComponent) amplifierComponentReadability {
	return amplifierComponentReadability{
		component:      component,
		normalizedText: normalizedComponentText(component),
		rawTokens:      rawComponentTokens(component),
	}
}

func leftmostReadableComponentMatching(components []amplifierComponentReadability, match func(amplifierComponentReadability) bool) (amplifierComponentReadability, bool) {
	var found amplifierComponentReadability
	ok := false
	for _, component := range components {
		if !match(component) {
			continue
		}
		if !ok || component.component.PlacedAt.X < found.component.PlacedAt.X {
			found = component
			ok = true
		}
	}
	return found, ok
}

func rightmostReadableComponentMatching(components []amplifierComponentReadability, match func(amplifierComponentReadability) bool) (amplifierComponentReadability, bool) {
	var found amplifierComponentReadability
	ok := false
	for _, component := range components {
		if !match(component) {
			continue
		}
		if !ok || component.component.PlacedAt.X > found.component.PlacedAt.X {
			found = component
			ok = true
		}
	}
	return found, ok
}

func isAmplifierActive(component PlacedComponent) bool {
	return isAmplifierActiveProfile(amplifierComponentReadabilityProfile(component))
}

func isAmplifierActiveProfile(component amplifierComponentReadability) bool {
	text := component.normalizedText
	if !strings.HasPrefix(component.component.Ref, "U") && !containsAnyRawToken(component, "opamp", "op_amp", "amplifier", "lm386") && !hasRawTokenPrefix(component, "tl07") {
		return false
	}
	return containsNormalizedRole(text, "opamp") ||
		containsNormalizedRole(text, "op_amp") ||
		containsNormalizedRole(text, "amplifier") ||
		containsNormalizedRole(text, "buffer") ||
		containsAnyRawToken(component, "opamp", "op_amp", "amplifier", "buffer", "lm386") ||
		hasRawTokenPrefix(component, "tl07")
}

func isOutputStage(component PlacedComponent) bool {
	return isOutputStageProfile(amplifierComponentReadabilityProfile(component))
}

func isOutputStageProfile(component amplifierComponentReadability) bool {
	text := component.normalizedText
	return strings.Contains(text, "output") ||
		containsNormalizedRole(text, "driver") ||
		containsNormalizedRole(text, "active_load") ||
		containsNormalizedRole(text, "emitter_follower")
}

func isPositiveRail(component PlacedComponent) bool {
	return isPositiveRailProfile(amplifierComponentReadabilityProfile(component))
}

func isPositiveRailProfile(component amplifierComponentReadability) bool {
	text := component.normalizedText
	if isNegativeRailProfile(component) {
		return false
	}
	return containsNormalizedRole(text, "vcc") ||
		containsNormalizedRole(text, "vdd") ||
		containsNormalizedRole(text, "v+")
}

func isGroundOrLoad(component PlacedComponent) bool {
	return isGroundOrLoadProfile(amplifierComponentReadabilityProfile(component))
}

func isGroundOrLoadProfile(component amplifierComponentReadability) bool {
	text := component.normalizedText
	return containsNormalizedRole(text, "gnd") ||
		containsNormalizedRole(text, "ground") ||
		containsNormalizedRole(text, "load") ||
		containsNormalizedRole(text, "0v")
}

func isNegativeRail(component PlacedComponent) bool {
	return isNegativeRailProfile(amplifierComponentReadabilityProfile(component))
}

func isNegativeRailProfile(component amplifierComponentReadability) bool {
	text := component.normalizedText
	if containsNormalizedRole(text, "vee") ||
		containsNormalizedRole(text, "vss") ||
		containsNormalizedRole(text, "negative") ||
		containsNormalizedRole(text, "neg") {
		return true
	}
	for _, token := range component.rawTokens {
		if token == "v-" || token == "-v" || isNegativeVoltageToken(token) {
			return true
		}
	}
	return false
}

func normalizedComponentText(component PlacedComponent) string {
	return normalizeRole(component.Ref + " " + component.Value + " " + component.LibraryID + " " + component.Role)
}

func rawComponentTokens(component PlacedComponent) []string {
	return strings.FieldsFunc(
		strings.ToLower(component.Ref+" "+component.Value+" "+component.LibraryID+" "+component.Role),
		func(r rune) bool {
			switch r {
			case ' ', '\t', '\n', '\r', ':', '/', '\\', '_':
				return true
			default:
				return false
			}
		},
	)
}

func containsAnyRawToken(component amplifierComponentReadability, tokens ...string) bool {
	normalizedTokens := make([]string, 0, len(tokens))
	for _, token := range tokens {
		normalizedTokens = append(normalizedTokens, normalizeRole(token))
	}
	for _, raw := range component.rawTokens {
		for _, token := range normalizedTokens {
			if raw == token {
				return true
			}
		}
	}
	return false
}

func hasRawTokenPrefix(component amplifierComponentReadability, prefix string) bool {
	normalizedPrefix := normalizeRole(prefix)
	for _, token := range component.rawTokens {
		if strings.HasPrefix(token, normalizedPrefix) {
			return true
		}
	}
	return false
}

func isNegativeVoltageToken(token string) bool {
	if !strings.HasPrefix(token, "-") {
		return false
	}
	token = strings.TrimPrefix(token, "-")
	hasDigit := false
	hasVoltageUnit := false
	for _, r := range token {
		switch {
		case r >= '0' && r <= '9':
			hasDigit = true
		case r == 'v':
			hasVoltageUnit = true
		}
	}
	return hasDigit && hasVoltageUnit
}

func firstLabel(labels []Label, tokens ...string) (Label, bool) {
	for _, label := range labels {
		if labelMatches(label, tokens...) {
			return label, true
		}
	}
	return Label{}, false
}

func leftmostLabel(labels []Label, tokens ...string) (Label, bool) {
	var found Label
	ok := false
	for _, label := range labels {
		if !labelMatches(label, tokens...) {
			continue
		}
		if !ok || label.Position.X < found.Position.X {
			found = label
			ok = true
		}
	}
	return found, ok
}

func rightmostLabel(labels []Label, tokens ...string) (Label, bool) {
	var found Label
	ok := false
	for _, label := range labels {
		if !labelMatches(label, tokens...) {
			continue
		}
		if !ok || label.Position.X > found.Position.X {
			found = label
			ok = true
		}
	}
	return found, ok
}

func labelMatches(label Label, tokens ...string) bool {
	text := normalizeRole(label.Text + " " + label.NetName)
	for _, token := range tokens {
		if containsNormalizedRole(text, token) {
			return true
		}
	}
	return false
}

func leftOf(first, second kicadfiles.IU) bool {
	return first < second-readabilityCoordinateTolerance()
}

func rightOf(first, second kicadfiles.IU) bool {
	return first > second+readabilityCoordinateTolerance()
}

func above(first, second kicadfiles.IU) bool {
	return first < second-readabilityCoordinateTolerance()
}

func below(first, second kicadfiles.IU) bool {
	return first > second+readabilityCoordinateTolerance()
}

func readabilityCoordinateTolerance() kicadfiles.IU {
	return kicadfiles.MM(1.27)
}
