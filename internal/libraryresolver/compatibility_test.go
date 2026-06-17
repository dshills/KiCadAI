package libraryresolver

import (
	"context"
	"path/filepath"
	"testing"

	"kicadai/internal/reports"
)

func TestValidateAssignmentMissingRecords(t *testing.T) {
	result := ValidateAssignment(LibraryIndex{}, "Device:R", "Resistor:R_0805")
	if result.Status != CompatibilityUnknown || len(result.Issues) != 1 || result.Issues[0].Code != reports.CodeMissingFile {
		t.Fatalf("result = %#v", result)
	}
}

func TestValidateAssignmentResistorCompatible(t *testing.T) {
	index := compatibilityFixture(t)
	result := ValidateAssignment(index, "Device:R", "Resistor_SMD:R_0805_2012Metric")
	if result.Status != CompatibilityCompatible {
		t.Fatalf("result = %#v", result)
	}
}

func TestValidateAssignmentPadCountMismatchIncompatible(t *testing.T) {
	index := compatibilityFixture(t)
	result := ValidateAssignment(index, "Device:R", "Bad:MissingPin")
	if result.Status != CompatibilityIncompatible || len(result.Issues) == 0 {
		t.Fatalf("result = %#v", result)
	}
}

func TestValidateAssignmentAllowsExtraFootprintPads(t *testing.T) {
	index := compatibilityFixture(t)
	result := ValidateAssignment(index, "Device:R", "Package_SO:SOIC-8")
	if result.Status == CompatibilityIncompatible {
		t.Fatalf("result = %#v", result)
	}
}

func TestValidateAssignmentIncludesHiddenElectricalPins(t *testing.T) {
	index := compatibilityFixture(t)
	result := ValidateAssignment(index, "Device:U_HIDDEN", "Resistor_SMD:R_0805_2012Metric")
	if result.Status != CompatibilityIncompatible {
		t.Fatalf("result = %#v", result)
	}
}

func TestValidateAssignmentTransistorNeedsVerification(t *testing.T) {
	index := compatibilityFixture(t)
	result := ValidateAssignment(index, "Device:Q_NPN", "Package_TO_SOT_THT:TO-92")
	if result.Status != CompatibilityNeedsVerification && result.Status != CompatibilityCandidate {
		t.Fatalf("result = %#v", result)
	}
	if len(result.Issues) != 1 || result.Issues[0].Code != reports.CodePinmapUnverified {
		t.Fatalf("issues = %#v", result.Issues)
	}
}

func TestCompatibleFootprintsDeterministicRanking(t *testing.T) {
	index := compatibilityFixture(t)
	results := CompatibleFootprints(index, "Device:R", MatchOptions{})
	if len(results) == 0 || results[0].FootprintID != "Resistor_SMD:R_0805_2012Metric" {
		t.Fatalf("results = %#v", results)
	}
}

func compatibilityFixture(t *testing.T) LibraryIndex {
	t.Helper()
	root := t.TempDir()
	symbols := filepath.Join(root, "symbols")
	footprints := filepath.Join(root, "footprints")
	mustWrite(t, filepath.Join(symbols, "Device.kicad_sym"), compatibilitySymbolLibrary())
	mustWrite(t, filepath.Join(symbols, "Connector.kicad_sym"), connectorSymbolLibrary())
	mustWrite(t, filepath.Join(footprints, "Resistor_SMD.pretty", "R_0805_2012Metric.kicad_mod"), resistor0805Footprint())
	mustWrite(t, filepath.Join(footprints, "Package_SO.pretty", "SOIC-8.kicad_mod"), soic8Footprint())
	mustWrite(t, filepath.Join(footprints, "Package_TO_SOT_THT.pretty", "TO-92.kicad_mod"), to92Footprint())
	mustWrite(t, filepath.Join(footprints, "Bad.pretty", "MissingPin.kicad_mod"), missingPinFootprint())
	mustWrite(t, filepath.Join(footprints, "Connector_PinHeader_2.54mm.pretty", "PinHeader_1x02_P2.54mm_Vertical.kicad_mod"), pinHeader1x02Footprint())
	mustWrite(t, filepath.Join(footprints, "Duplicate.pretty", "DupPads.kicad_mod"), duplicatePadsFootprint())
	mustWrite(t, filepath.Join(footprints, "Function.pretty", "FuncPads.kicad_mod"), functionPadsFootprint())
	index, issues := Load(context.Background(), LibraryRoots{SymbolsRoot: symbols, FootprintsRoot: footprints}, LoadOptions{})
	for _, issue := range issues {
		if issue.Blocking() {
			t.Fatalf("issues = %#v", issues)
		}
	}
	return index
}

func compatibilitySymbolLibrary() string {
	return `
(kicad_symbol_lib
  (version 20220914)
  (generator "kicadai-test")
  (symbol "R"
    (property "Reference" "R" (at 0 0 0))
    (property "Value" "R" (at 0 -2.54 0))
    (ki_keywords "resistor R")
    (ki_description "Resistor")
    (ki_fp_filters "R_*" "Resistor_*")
    (symbol "R_1_1"
      (pin passive line (at -5.08 0 0) (length 2.54) (name "~") (number "1"))
      (pin passive line (at 5.08 0 180) (length 2.54) (name "~") (number "2"))
    )
  )
  (symbol "Q_NPN"
    (property "Reference" "Q" (at 0 0 0))
    (property "Value" "Q_NPN" (at 0 -2.54 0))
    (ki_keywords "transistor npn")
    (ki_description "NPN transistor")
    (ki_fp_filters "TO?92*")
    (symbol "Q_NPN_1_1"
      (pin input line (at -5.08 0 0) (length 2.54) (name "B") (number "1"))
      (pin passive line (at 5.08 2.54 180) (length 2.54) (name "C") (number "2"))
      (pin passive line (at 5.08 -2.54 180) (length 2.54) (name "E") (number "3"))
    )
  )
  (symbol "U_HIDDEN"
    (property "Reference" "U" (at 0 0 0))
    (property "Value" "U_HIDDEN" (at 0 -2.54 0))
    (ki_keywords "logic hidden power")
    (ki_description "Logic device with hidden power pin")
    (symbol "U_HIDDEN_1_1"
      (pin input line (at -5.08 0 0) (length 2.54) (name "A") (number "1"))
      (pin output line (at 5.08 0 180) (length 2.54) (name "Y") (number "2"))
      (pin passive line (at 0 5.08 270) (length 2.54) hide (name "VCC") (number "8"))
    )
  )
  (symbol "STACKED"
    (property "Reference" "U" (at 0 0 0))
    (property "Value" "STACKED" (at 0 -2.54 0))
    (ki_keywords "stacked pins")
    (ki_description "Symbol with stacked duplicate pin numbers")
    (symbol "STACKED_1_1"
      (pin passive line (at -5.08 0 0) (length 2.54) (name "A") (number "1"))
      (pin passive line (at -5.08 2.54 0) (length 2.54) (name "A_ALT") (number "1"))
      (pin passive line (at 5.08 0 180) (length 2.54) (name "B") (number "2"))
    )
  )
  (symbol "FUNC"
    (property "Reference" "U" (at 0 0 0))
    (property "Value" "FUNC" (at 0 -2.54 0))
    (ki_keywords "function pin hints")
    (ki_description "Symbol with nonnumeric pin designators")
    (symbol "FUNC_1_1"
      (pin input line (at -5.08 0 0) (length 2.54) (name "IN") (number "A"))
      (pin output line (at 5.08 0 180) (length 2.54) (name "OUT") (number "Y"))
    )
  )
)`
}

func connectorSymbolLibrary() string {
	return `
(kicad_symbol_lib
  (version 20220914)
  (generator "kicadai-test")
  (symbol "Conn_01x02"
    (property "Reference" "J" (at 0 0 0))
    (property "Value" "Conn_01x02" (at 0 -2.54 0))
    (ki_keywords "connector")
    (ki_description "Generic connector, single row, 01x02")
    (ki_fp_filters "PinHeader_1x02*")
    (symbol "Conn_01x02_1_1"
      (pin passive line (at -5.08 2.54 0) (length 2.54) (name "Pin_1") (number "1"))
      (pin passive line (at -5.08 0 0) (length 2.54) (name "Pin_2") (number "2"))
    )
  )
)`
}

func soic8Footprint() string {
	return `
(footprint "SOIC-8"
  (pad "1" smd rect (at 0 0) (size 1 1) (layers "F.Cu"))
  (pad "2" smd rect (at 1 0) (size 1 1) (layers "F.Cu"))
  (pad "3" smd rect (at 2 0) (size 1 1) (layers "F.Cu"))
  (pad "4" smd rect (at 3 0) (size 1 1) (layers "F.Cu"))
  (pad "5" smd rect (at 4 0) (size 1 1) (layers "F.Cu"))
  (pad "6" smd rect (at 5 0) (size 1 1) (layers "F.Cu"))
  (pad "7" smd rect (at 6 0) (size 1 1) (layers "F.Cu"))
  (pad "8" smd rect (at 7 0) (size 1 1) (layers "F.Cu"))
)`
}

func to92Footprint() string {
	return `
(footprint "TO-92"
  (descr "transistor TO-92")
  (tags "transistor to-92")
  (pad "1" thru_hole circle (at 0 0) (size 1 1) (drill 0.5) (layers "*.Cu" "*.Mask"))
  (pad "2" thru_hole circle (at 1.27 0) (size 1 1) (drill 0.5) (layers "*.Cu" "*.Mask"))
  (pad "3" thru_hole circle (at 2.54 0) (size 1 1) (drill 0.5) (layers "*.Cu" "*.Mask"))
)`
}

func missingPinFootprint() string {
	return `
(footprint "MissingPin"
  (pad "1" smd rect (at 0 0) (size 1 1) (layers "F.Cu"))
  (pad "3" smd rect (at 1 0) (size 1 1) (layers "F.Cu"))
)`
}

func pinHeader1x02Footprint() string {
	return `
(footprint "PinHeader_1x02_P2.54mm_Vertical"
  (descr "Through hole straight pin header, 1x02")
  (tags "connector pin header")
  (pad "1" thru_hole rect (at 0 0) (size 1.7 1.7) (drill 1) (layers "*.Cu" "*.Mask"))
  (pad "2" thru_hole oval (at 0 2.54) (size 1.7 1.7) (drill 1) (layers "*.Cu" "*.Mask"))
)`
}

func duplicatePadsFootprint() string {
	return `
(footprint "DupPads"
  (pad "1" smd rect (at 0 0) (size 1 1) (layers "F.Cu"))
  (pad "1" smd rect (at 0 1) (size 1 1) (layers "F.Cu"))
  (pad "2" smd rect (at 1 0) (size 1 1) (layers "F.Cu"))
)`
}

func functionPadsFootprint() string {
	return `
(footprint "FuncPads"
  (pad "P1" smd rect (at 0 0) (size 1 1) (layers "F.Cu") (pinfunction "IN"))
  (pad "P2" smd rect (at 1 0) (size 1 1) (layers "F.Cu") (pinfunction "OUT"))
)`
}
