package amplifiers

import (
	"strings"
	"testing"

	"kicadai/internal/kicadfiles/schematic"
)

func TestValidateSchematicLandmarksAcceptsClassABFixtureShape(t *testing.T) {
	file := schematicWithLandmarks(
		[]string{"AUDIO_IN", "GAIN_FEEDBACK", "BIAS_N", "BIAS_P", "AMP_OUT", "HP_OUT"},
		[]schematic.SchematicSymbol{
			{LibraryID: "Connector:Conn_01x03_Pin", Value: "AUDIO_IN"},
			{LibraryID: "Simulation_SPICE:OPAMP", Value: "OPAMP"},
			{LibraryID: "Transistor_BJT:Q_NPN_BCE", Value: "NPN"},
			{LibraryID: "Transistor_BJT:Q_PNP_BCE", Value: "PNP"},
			{LibraryID: "Device:R", Value: "32R LOAD"},
			{LibraryID: "Device:C", Value: "1u"},
			{LibraryID: "Device:C", Value: "220u"},
			{LibraryID: "power:GND", Value: "GND"},
			{LibraryID: "power:VCC", Value: "VCC"},
			{LibraryID: "power:VEE", Value: "VEE"},
		},
	)

	if validation := ValidateSchematicLandmarks(file, ClassABHeadphoneAmpLandmarks()); !validation.OK() {
		t.Fatalf("%s", validation)
	}
}

func TestValidateSchematicLandmarksReportsMissingAmplifierContent(t *testing.T) {
	tests := []struct {
		name     string
		file     *schematic.SchematicFile
		contains []string
	}{
		{
			name: "feedback",
			file: schematicWithLandmarks(
				[]string{"AUDIO_IN", "BIAS_N", "BIAS_P", "AMP_OUT", "HP_OUT"},
				classABSymbols(),
			),
			contains: []string{"GAIN_FEEDBACK"},
		},
		{
			name: "output connector",
			file: schematicWithLandmarks(
				[]string{"AUDIO_IN", "GAIN_FEEDBACK", "BIAS_N", "BIAS_P", "AMP_OUT"},
				classABSymbols(),
			),
			contains: []string{"HP_OUT"},
		},
		{
			name: "bias reference",
			file: schematicWithLandmarks(
				[]string{"AUDIO_IN", "GAIN_FEEDBACK", "AMP_OUT", "HP_OUT"},
				classABSymbols(),
			),
			contains: []string{"BIAS_N", "BIAS_P"},
		},
		{
			name: "decoupling",
			file: schematicWithLandmarks(
				[]string{"AUDIO_IN", "GAIN_FEEDBACK", "BIAS_N", "BIAS_P", "AMP_OUT", "HP_OUT"},
				[]schematic.SchematicSymbol{
					{LibraryID: "Connector:Conn_01x03_Pin", Value: "AUDIO_IN"},
					{LibraryID: "Simulation_SPICE:OPAMP", Value: "OPAMP"},
					{LibraryID: "Transistor_BJT:Q_NPN_BCE", Value: "NPN"},
					{LibraryID: "Transistor_BJT:Q_PNP_BCE", Value: "PNP"},
					{LibraryID: "Device:R", Value: "32R LOAD"},
					{LibraryID: "power:GND", Value: "GND"},
					{LibraryID: "power:VCC", Value: "VCC"},
					{LibraryID: "power:VEE", Value: "VEE"},
				},
			),
			contains: []string{"1u", "220u", "Device:C"},
		},
		{
			name:     "nil schematic",
			file:     nil,
			contains: []string{"AUDIO_IN", "OPAMP", "power:GND"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validation := ValidateSchematicLandmarks(tt.file, ClassABHeadphoneAmpLandmarks())
			if validation.OK() {
				t.Fatalf("expected missing amplifier landmarks")
			}
			message := validation.String()
			for _, fragment := range tt.contains {
				if !strings.Contains(message, fragment) {
					t.Fatalf("expected validation message %q to contain %q", message, fragment)
				}
			}
		})
	}
}

func schematicWithLandmarks(labels []string, symbols []schematic.SchematicSymbol) *schematic.SchematicFile {
	file := &schematic.SchematicFile{Symbols: symbols}
	for _, label := range labels {
		file.Labels = append(file.Labels, schematic.Label{Text: label})
	}
	return file
}

func classABSymbols() []schematic.SchematicSymbol {
	return []schematic.SchematicSymbol{
		{LibraryID: "Connector:Conn_01x03_Pin", Value: "AUDIO_IN"},
		{LibraryID: "Simulation_SPICE:OPAMP", Value: "OPAMP"},
		{LibraryID: "Transistor_BJT:Q_NPN_BCE", Value: "NPN"},
		{LibraryID: "Transistor_BJT:Q_PNP_BCE", Value: "PNP"},
		{LibraryID: "Device:R", Value: "32R LOAD"},
		{LibraryID: "Device:C", Value: "1u"},
		{LibraryID: "Device:C", Value: "220u"},
		{LibraryID: "power:GND", Value: "GND"},
		{LibraryID: "power:VCC", Value: "VCC"},
		{LibraryID: "power:VEE", Value: "VEE"},
	}
}
