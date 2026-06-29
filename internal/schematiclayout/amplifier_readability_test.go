package schematiclayout

import (
	"testing"

	"kicadai/internal/kicadfiles"
	"kicadai/internal/kicadfiles/schematic"
)

func TestValidateAmplifierReadabilityAcceptsReadableSynthetic(t *testing.T) {
	file := syntheticAmplifierSchematic(false)
	if diagnostics := ValidateAmplifierReadability(file); len(diagnostics) > 0 {
		t.Fatalf("diagnostics = %#v, want pass", diagnostics)
	}
}

func TestValidateAmplifierReadabilityRejectsDiagonalSynthetic(t *testing.T) {
	file := syntheticAmplifierSchematic(true)
	if !hasDiagnostic(ValidateAmplifierReadability(file), "diagonal_wire", SeverityError) {
		t.Fatalf("expected diagonal wire diagnostic")
	}
}

func TestValidateAmplifierReadabilityRejectsBadSignalFlow(t *testing.T) {
	file := syntheticAmplifierSchematic(false)
	file.Labels[0].Position = pointMM(90, 45)
	diagnostics := ValidateAmplifierReadability(file)
	if !hasDiagnostic(diagnostics, "amplifier_input_flow", SeverityError) {
		t.Fatalf("expected input flow diagnostic")
	}
	if !hasDiagnosticRepair(diagnostics, "amplifier_input_flow") {
		t.Fatalf("expected input flow repair guidance: %#v", diagnostics)
	}
}

func TestValidateAmplifierReadabilityRejectsBadRailPlacement(t *testing.T) {
	file := syntheticAmplifierSchematic(false)
	file.Symbols[3].Position = pointMM(70, 80)
	if !hasDiagnostic(ValidateAmplifierReadability(file), "amplifier_positive_rail_position", SeverityError) {
		t.Fatalf("expected positive rail placement diagnostic")
	}
}

func TestValidateAmplifierReadabilityWarnsForMissingFeedbackLabel(t *testing.T) {
	file := syntheticAmplifierSchematic(false)
	file.Labels = []schematic.Label{file.Labels[0], file.Labels[2]}
	if !hasDiagnostic(ValidateAmplifierReadability(file), "amplifier_feedback_missing", SeverityWarning) {
		t.Fatalf("expected missing feedback warning")
	}
}

func TestAmplifierReadabilityHeuristicsAvoidAmbiguousRailsAndTransistors(t *testing.T) {
	if !isPositiveRail(PlacedComponent{Component: Component{Value: "VCC", LibraryID: "power:VCC"}}) {
		t.Fatalf("expected VCC to match positive rail")
	}
	if isPositiveRail(PlacedComponent{Component: Component{Value: "VEE", LibraryID: "power:VEE"}}) {
		t.Fatalf("expected VEE not to match positive rail")
	}
	if isPositiveRail(PlacedComponent{Component: Component{Value: "-15V", LibraryID: "power:-15V"}}) {
		t.Fatalf("expected negative voltage rail not to match positive rail")
	}
	if isPositiveRail(PlacedComponent{Component: Component{Value: "-40dB", LibraryID: "Device:R"}}) {
		t.Fatalf("expected negative gain value not to match negative rail")
	}
	if !isPositiveRail(PlacedComponent{Component: Component{Value: "V+", LibraryID: "power:V+"}}) {
		t.Fatalf("expected V+ to match positive rail")
	}
	if isAmplifierActive(PlacedComponent{Component: Component{Ref: "C1", Value: "buffer bypass", LibraryID: "Device:C"}}) {
		t.Fatalf("expected passive buffer bypass capacitor not to match active stage")
	}
	if !isAmplifierActive(PlacedComponent{Component: Component{Ref: "U1", Value: "OPAMP", LibraryID: "Simulation_SPICE:OPAMP"}}) {
		t.Fatalf("expected op-amp IC to match active stage")
	}
	if isOutputStage(PlacedComponent{Component: Component{Value: "NPN", LibraryID: "Transistor_BJT:Q_NPN_BCE"}}) {
		t.Fatalf("expected generic NPN transistor not to match output stage")
	}
	if !isOutputStage(PlacedComponent{Component: Component{Value: "OUTPUT_NPN", LibraryID: "Transistor_BJT:Q_NPN_BCE"}}) {
		t.Fatalf("expected explicit output transistor to match output stage")
	}
}

func TestAmplifierReadabilityMatchesCompoundLabels(t *testing.T) {
	if !labelMatches(Label{Text: "AUDIO_IN"}, "audio_in", "input") {
		t.Fatalf("expected AUDIO_IN label to match input tokens")
	}
	if !labelMatches(Label{Text: "HP_OUT"}, "hp_out", "out", "output") {
		t.Fatalf("expected HP_OUT label to match output tokens")
	}
}

func TestAmplifierExamplesPassStrictReadability(t *testing.T) {
	examples := []string{
		"06_class_ab_headphone_amp",
		"09_class_a_headphone_amp",
		"10_opamp_buffer_headphone_amp",
	}
	for _, example := range examples {
		t.Run(example, func(t *testing.T) {
			file := readExampleSchematic(t, example)
			if diagnostics := ValidateAmplifierReadability(file); hasSeverity(diagnostics, SeverityError) {
				t.Fatalf("strict amplifier readability diagnostics for %s = %#v", example, diagnostics)
			}
		})
	}
}

func hasSeverity(diagnostics []Diagnostic, severity Severity) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Severity == severity {
			return true
		}
	}
	return false
}

func hasDiagnosticRepair(diagnostics []Diagnostic, code string) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == code && diagnostic.Repair != "" {
			return true
		}
	}
	return false
}

func syntheticAmplifierSchematic(diagonal bool) *schematic.SchematicFile {
	wireEnd := pointMM(80, 40)
	if diagonal {
		wireEnd = pointMM(80, 45)
	}
	return &schematic.SchematicFile{
		Symbols: []schematic.SchematicSymbol{
			{Reference: "J1", Value: "AUDIO_IN", LibraryID: "Connector:Conn_01x03_Pin", Position: pointMM(20, 50)},
			{Reference: "U1", Value: "OPAMP", LibraryID: "Simulation_SPICE:OPAMP", Position: pointMM(70, 50)},
			{Reference: "Q1", Value: "OUTPUT_NPN", LibraryID: "Transistor_BJT:Q_NPN_BCE", Position: pointMM(110, 45)},
			{Reference: "#PWR01", Value: "VCC", LibraryID: "power:VCC", Position: pointMM(70, 25)},
			{Reference: "#PWR02", Value: "GND", LibraryID: "power:GND", Position: pointMM(70, 90)},
		},
		Labels: []schematic.Label{
			{Text: "AUDIO_IN", Position: pointMM(25, 45)},
			{Text: "GAIN_FEEDBACK", Position: pointMM(45, 30)},
			{Text: "HP_OUT", Position: pointMM(130, 45)},
		},
		Wires: []schematic.Wire{{Points: []kicadfiles.Point{
			pointMM(70, 40),
			wireEnd,
		}}},
	}
}

func pointMM(x, y float64) kicadfiles.Point {
	return kicadfiles.Point{X: kicadfiles.MM(x), Y: kicadfiles.MM(y)}
}
