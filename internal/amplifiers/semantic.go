package amplifiers

import (
	"fmt"
	"strings"

	"kicadai/internal/kicadfiles/schematic"
)

// SchematicLandmarks declares amplifier-specific schematic content that must
// be present for a fixture or generated design to satisfy a topology check.
type SchematicLandmarks struct {
	Labels       []string
	SymbolValues []string
	LibraryIDs   []string
}

// SchematicValidation reports missing amplifier landmarks discovered during a
// semantic schematic check.
type SchematicValidation struct {
	MissingLabels       []string
	MissingSymbolValues []string
	MissingLibraryIDs   []string
}

// OK returns true when every requested amplifier landmark was present.
func (v SchematicValidation) OK() bool {
	return len(v.MissingLabels) == 0 &&
		len(v.MissingSymbolValues) == 0 &&
		len(v.MissingLibraryIDs) == 0
}

// String returns a compact reader-facing validation summary.
func (v SchematicValidation) String() string {
	var parts []string
	if len(v.MissingLabels) > 0 {
		parts = append(parts, fmt.Sprintf("labels=%q", v.MissingLabels))
	}
	if len(v.MissingSymbolValues) > 0 {
		parts = append(parts, fmt.Sprintf("symbol_values=%q", v.MissingSymbolValues))
	}
	if len(v.MissingLibraryIDs) > 0 {
		parts = append(parts, fmt.Sprintf("library_ids=%q", v.MissingLibraryIDs))
	}
	if len(parts) == 0 {
		return "amplifier schematic landmarks satisfied"
	}
	return "missing amplifier schematic landmarks: " + strings.Join(parts, " ")
}

// ValidateSchematicLandmarks checks a schematic file for amplifier topology
// markers such as named nets, power rails, output stages, and decoupling.
func ValidateSchematicLandmarks(file *schematic.SchematicFile, landmarks SchematicLandmarks) SchematicValidation {
	if file == nil {
		return SchematicValidation{
			MissingLabels:       append([]string(nil), landmarks.Labels...),
			MissingSymbolValues: append([]string(nil), landmarks.SymbolValues...),
			MissingLibraryIDs:   append([]string(nil), landmarks.LibraryIDs...),
		}
	}
	labels := make(map[string]bool, len(file.Labels))
	for _, label := range file.Labels {
		labels[label.Text] = true
	}
	symbolValues := make(map[string]bool, len(file.Symbols))
	libraryIDs := make(map[string]bool, len(file.Symbols))
	for _, symbol := range file.Symbols {
		symbolValues[symbol.Value] = true
		libraryIDs[symbol.LibraryID] = true
	}
	return SchematicValidation{
		MissingLabels:       missingStrings(labels, landmarks.Labels),
		MissingSymbolValues: missingStrings(symbolValues, landmarks.SymbolValues),
		MissingLibraryIDs:   missingStrings(libraryIDs, landmarks.LibraryIDs),
	}
}

// ClassABHeadphoneAmpLandmarks returns the required landmarks for the
// checked-in Class AB headphone amplifier example.
func ClassABHeadphoneAmpLandmarks() SchematicLandmarks {
	return SchematicLandmarks{
		Labels: []string{
			"AUDIO_IN",
			"GAIN_FEEDBACK",
			"BIAS_N",
			"BIAS_P",
			"AMP_OUT",
			"HP_OUT",
		},
		SymbolValues: []string{
			"OPAMP",
			"NPN",
			"PNP",
			"32R LOAD",
			"1u",
			"220u",
		},
		LibraryIDs: commonHeadphoneAmpLibraryIDs(),
	}
}

// ClassAHeadphoneAmpLandmarks returns the required landmarks for the checked-in
// Class A headphone amplifier example.
func ClassAHeadphoneAmpLandmarks() SchematicLandmarks {
	return SchematicLandmarks{
		Labels: []string{
			"AUDIO_IN",
			"CLASS_A_FEEDBACK",
			"CLASS_A_BIAS",
			"QUIESCENT_BIAS",
			"CLASS_A_OUT",
			"HP_OUT",
		},
		SymbolValues: []string{
			"CLASS_A_OPAMP",
			"CLASS_A_DRIVER",
			"ACTIVE_LOAD",
			"32R LOAD",
			"1u",
			"220u",
		},
		LibraryIDs: commonHeadphoneAmpLibraryIDs(),
	}
}

// OpAmpBufferHeadphoneAmpLandmarks returns the required landmarks for the
// checked-in op-amp buffer headphone amplifier example.
func OpAmpBufferHeadphoneAmpLandmarks() SchematicLandmarks {
	return SchematicLandmarks{
		Labels: []string{
			"AUDIO_IN",
			"BUFFER_FEEDBACK",
			"BUFFER_BIAS_REF",
			"BUFFER_RETURN",
			"BUFFER_OUT",
			"HP_OUT",
		},
		SymbolValues: []string{
			"BUFFER_OPAMP",
			"OUTPUT_NPN",
			"OUTPUT_PNP",
			"32R LOAD",
			"1u",
			"220u",
		},
		LibraryIDs: commonHeadphoneAmpLibraryIDs(),
	}
}

func commonHeadphoneAmpLibraryIDs() []string {
	return []string{
		"Connector:Conn_01x03_Pin",
		"Simulation_SPICE:OPAMP",
		"Transistor_BJT:Q_NPN_BCE",
		"Transistor_BJT:Q_PNP_BCE",
		"Device:R",
		"Device:D",
		"Device:C",
		"power:GND",
		"power:VCC",
		"power:VEE",
	}
}

func missingStrings(seen map[string]bool, want []string) []string {
	var missing []string
	reported := make(map[string]bool, len(want))
	for _, item := range want {
		if !seen[item] && !reported[item] {
			missing = append(missing, item)
			reported[item] = true
		}
	}
	return missing
}
