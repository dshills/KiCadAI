package components

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	"kicadai/internal/reports"
)

func TestLoadCatalogEmptyDirectory(t *testing.T) {
	dir := t.TempDir()
	catalog, err := LoadCatalog(context.Background(), LoadOptions{CatalogDir: dir})
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}
	if len(catalog.Diagnostics) != 1 || catalog.Diagnostics[0].Code != CodeCatalogEmpty {
		t.Fatalf("expected empty catalog diagnostic, got %+v", catalog.Diagnostics)
	}
}

func TestLoadCatalogRejectsRelativeParentTraversal(t *testing.T) {
	_, err := LoadCatalog(context.Background(), LoadOptions{CatalogDir: "../components"})
	if err == nil {
		t.Fatal("expected parent traversal catalog dir to fail")
	}
}

func TestLoadCatalogMergesDeterministically(t *testing.T) {
	dir := t.TempDir()
	writeCatalogFile(t, dir, "b.json", `{
  "families": [{"id": "resistor", "name": "Resistor"}],
  "records": [`+validRecordJSON("resistor.generic.0805", "resistor", "0805")+`]
}`)
	writeCatalogFile(t, dir, "a.json", `{
  "families": [{"id": "capacitor", "name": "Capacitor"}],
  "records": [`+validRecordJSON("capacitor.generic.0805", "capacitor", "0805")+`]
}`)

	catalog, err := LoadCatalog(context.Background(), LoadOptions{CatalogDir: dir})
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}
	if len(catalog.Records) != 2 {
		t.Fatalf("expected two records, got %d", len(catalog.Records))
	}
	if catalog.Families[0].ID != "capacitor" || catalog.Records[0].ID != "capacitor.generic.0805" {
		t.Fatalf("catalog merge order is not deterministic: %+v", catalog)
	}
	if reports.HasBlockingIssue(catalog.Diagnostics) {
		t.Fatalf("unexpected diagnostics: %+v", catalog.Diagnostics)
	}
}

func TestLoadCatalogUsesEmbeddedDefaultOutsideRepository(t *testing.T) {
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(originalDir); err != nil {
			t.Errorf("restore working directory: %v", err)
		}
	})
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatalf("change working directory: %v", err)
	}

	embedded, err := LoadCatalog(context.Background(), LoadOptions{})
	if err != nil {
		t.Fatalf("load embedded catalog: %v", err)
	}
	checkedIn, err := LoadCatalog(context.Background(), LoadOptions{CatalogDir: checkedInCatalogDir(t)})
	if err != nil {
		t.Fatalf("load checked-in catalog: %v", err)
	}
	if reports.HasBlockingIssue(embedded.Diagnostics) {
		t.Fatalf("embedded catalog diagnostics: %+v", embedded.Diagnostics)
	}
	if !slices.Equal(catalogRecordIDs(embedded), catalogRecordIDs(checkedIn)) {
		t.Fatalf("embedded record IDs = %v, want %v", catalogRecordIDs(embedded), catalogRecordIDs(checkedIn))
	}
}

func catalogRecordIDs(catalog *Catalog) []string {
	ids := make([]string, 0, len(catalog.Records))
	for _, record := range catalog.Records {
		ids = append(ids, record.ID)
	}
	return ids
}

func TestCheckedInCatalogLoadsAndValidates(t *testing.T) {
	catalog, err := LoadCatalog(context.Background(), LoadOptions{CatalogDir: checkedInCatalogDir(t)})
	if err != nil {
		t.Fatalf("load checked-in catalog: %v", err)
	}
	if len(catalog.Records) == 0 {
		t.Fatal("checked-in catalog has no records")
	}
	result := ValidateCatalog(catalog)
	if !result.OK {
		t.Fatalf("checked-in catalog validation failed: %+v", result.Issues)
	}
	coveredFamilies := catalogFamilyCoverage(catalog)
	for _, family := range catalog.Families {
		if !coveredFamilies[family.ID] {
			t.Fatalf("checked-in catalog missing family record for %s", family.ID)
		}
	}
}

func TestCheckedInCatalogLM358MultiUnitEvidence(t *testing.T) {
	catalog, err := LoadCatalog(context.Background(), LoadOptions{CatalogDir: checkedInCatalogDir(t)})
	if err != nil {
		t.Fatalf("load checked-in catalog: %v", err)
	}
	record := requireCatalogRecord(t, catalog, "opamp.ti.lm358.soic8")
	if record.MPN != "LM358DR" || record.Verification.Confidence != ConfidenceVerified {
		t.Fatalf("LM358 identity = MPN:%q confidence:%q", record.MPN, record.Verification.Confidence)
	}
	wantUnits := map[string]struct {
		unit     int
		unitType SymbolUnitType
		required bool
		pins     []string
	}{
		"A": {unit: 1, unitType: SymbolUnitFunctional, pins: []string{"1", "2", "3"}},
		"B": {unit: 2, unitType: SymbolUnitFunctional, pins: []string{"5", "6", "7"}},
		"P": {unit: 3, unitType: SymbolUnitPower, required: true, pins: []string{"4", "8"}},
	}
	if len(record.Symbols) != len(wantUnits) {
		t.Fatalf("LM358 symbol units = %d, want %d", len(record.Symbols), len(wantUnits))
	}
	for _, symbol := range record.Symbols {
		want, exists := wantUnits[symbol.UnitID]
		if !exists {
			t.Fatalf("unexpected LM358 unit %#v", symbol)
		}
		if symbol.SymbolID != "Amplifier_Operational:LM358" || symbol.Unit != want.unit || symbol.UnitType != want.unitType || symbol.RequiredUnit != want.required {
			t.Fatalf("LM358 unit %s = %#v", symbol.UnitID, symbol)
		}
		pins := make([]string, 0, len(symbol.FunctionPins))
		for _, pin := range symbol.FunctionPins {
			pins = append(pins, pin.SymbolPin)
		}
		slices.Sort(pins)
		if !slices.Equal(pins, want.pins) {
			t.Fatalf("LM358 unit %s pins = %v, want %v", symbol.UnitID, pins, want.pins)
		}
	}
	if len(record.Packages) != 1 || record.Packages[0].FootprintID != "Package_SO:SOIC-8_3.9x4.9mm_P1.27mm" || len(record.Packages[0].PadFunctions) != 8 {
		t.Fatalf("LM358 package evidence = %#v", record.Packages)
	}
	if record.OpAmp == nil || record.OpAmp.StabilityStatus != "proven" || record.OpAmp.GainBandwidthStatus != "proven" || record.OpAmp.InputCommonModeStatus != "proven" || record.OpAmp.OutputDriveStatus != "proven" || record.OpAmp.LoadCompatibilityStatus != "proven" || record.OpAmp.FabricationCandidateBlocks || record.OpAmp.NoiseStatus != "review_required" || record.OpAmp.DistortionStatus != "review_required" {
		t.Fatalf("LM358 analog review evidence = %#v", record.OpAmp)
	}
	wantAliases := map[string]string{
		"1": "CHANNEL_1_OUT", "2": "CHANNEL_1_IN_MINUS", "3": "CHANNEL_1_IN_PLUS",
		"5": "CHANNEL_2_IN_PLUS", "6": "CHANNEL_2_IN_MINUS", "7": "CHANNEL_2_OUT",
	}
	for _, symbol := range record.Symbols {
		for _, pin := range symbol.FunctionPins {
			if want := wantAliases[pin.SymbolPin]; want != "" && !slices.Contains(pin.Aliases, want) {
				t.Fatalf("LM358 pin %s aliases = %v, want %s", pin.SymbolPin, pin.Aliases, want)
			}
		}
	}
}

func TestCheckedInCatalogBJTLibraryIdentityIsConsistent(t *testing.T) {
	catalog, err := LoadCatalog(context.Background(), LoadOptions{CatalogDir: checkedInCatalogDir(t)})
	if err != nil {
		t.Fatalf("load checked-in catalog: %v", err)
	}
	for _, test := range []struct {
		recordID string
		symbolID string
	}{
		{recordID: "bjt.onsemi.mmbt3904.sot23", symbolID: "Transistor_BJT:Q_NPN_BEC"},
		{recordID: "bjt.onsemi.mmbt3906.sot23", symbolID: "Transistor_BJT:Q_PNP_BEC"},
	} {
		record := requireCatalogRecord(t, catalog, test.recordID)
		wantEvidence := "builtin_pinmap:" + test.symbolID
		if record.AmplifierOutput == nil || record.AmplifierOutput.SymbolID != test.symbolID || record.AmplifierOutput.PinmapEvidence != wantEvidence {
			t.Fatalf("%s amplifier output identity is inconsistent: %+v", test.recordID, record.AmplifierOutput)
		}
		if len(record.Symbols) != 1 || record.Symbols[0].SymbolID != test.symbolID || !slices.Contains(record.Symbols[0].Verification.Sources, wantEvidence) {
			t.Fatalf("%s symbol identity is inconsistent: %+v", test.recordID, record.Symbols)
		}
		if len(record.Packages) != 1 || record.Packages[0].PinMapID != test.symbolID+"|Package_TO_SOT_SMD:SOT-23" || !slices.Contains(record.Packages[0].Verification.Sources, wantEvidence) {
			t.Fatalf("%s package pinmap identity is inconsistent: %+v", test.recordID, record.Packages)
		}
		if !slices.Contains(record.Verification.Sources, wantEvidence) {
			t.Fatalf("%s record evidence does not include %q: %+v", test.recordID, wantEvidence, record.Verification.Sources)
		}
	}
}

func TestCheckedInCatalogAudioPowerSemiconductorEvidence(t *testing.T) {
	catalog, err := LoadCatalog(context.Background(), LoadOptions{CatalogDir: checkedInCatalogDir(t)})
	if err != nil {
		t.Fatalf("load checked-in catalog: %v", err)
	}
	tests := []struct {
		id               string
		family           string
		deviceClass      string
		polarity         string
		fabricationProof bool
		linearStatus     string
	}{
		{id: "bjt.onsemi.d44h11g.to220", family: "bjt", deviceClass: "bjt", polarity: "npn", fabricationProof: true, linearStatus: "proven"},
		{id: "bjt.onsemi.d45h11g.to220", family: "bjt", deviceClass: "bjt", polarity: "pnp", fabricationProof: true, linearStatus: "proven"},
		{id: "bjt.onsemi.njw0281g.to3p", family: "bjt", deviceClass: "bjt", polarity: "npn", fabricationProof: true, linearStatus: "proven"},
		{id: "bjt.onsemi.njw0302g.to3p", family: "bjt", deviceClass: "bjt", polarity: "pnp", fabricationProof: true, linearStatus: "proven"},
		{id: "mosfet.vishay.irfp240.to247", family: "mosfet", deviceClass: "mosfet", polarity: "n_channel", linearStatus: "review_required"},
		{id: "mosfet.vishay.irfp9240.to247", family: "mosfet", deviceClass: "mosfet", polarity: "p_channel", linearStatus: "review_required"},
	}
	for _, test := range tests {
		record := requireCatalogRecord(t, catalog, test.id)
		if record.Family != test.family || record.PowerSemiconductor == nil {
			t.Fatalf("%s power semiconductor evidence = %#v", test.id, record.PowerSemiconductor)
		}
		evidence := record.PowerSemiconductor
		if evidence.DeviceClass != test.deviceClass || evidence.Polarity != test.polarity || evidence.FabricationProof != test.fabricationProof || evidence.LinearModeStatus != test.linearStatus {
			t.Fatalf("%s power semiconductor evidence = %#v", test.id, evidence)
		}
		if evidence.MaxVoltage == nil || evidence.ContinuousCurrent == nil || evidence.PeakCurrent == nil || evidence.PowerDissipation == nil || evidence.MaxJunctionTemperatureC == nil || evidence.JunctionToCaseCPerW == nil {
			t.Fatalf("%s missing quantitative ratings: %#v", test.id, evidence)
		}
		if test.fabricationProof && (len(evidence.SOA) < 2 || evidence.MountingAssumptions == "") {
			t.Fatalf("%s missing fabrication SOA/thermal evidence: %#v", test.id, evidence)
		}
		if record.AmplifierOutput == nil || record.AmplifierOutput.DeviceClass != test.deviceClass || record.AmplifierOutput.Polarity != test.polarity || record.AmplifierOutput.ComplementaryGroup != evidence.ComplementaryGroup {
			t.Fatalf("%s amplifier output evidence mismatch: %#v", test.id, record.AmplifierOutput)
		}
		wantModel := "mna_bjt_npn_ebers_moll_v1"
		if test.polarity == "pnp" {
			wantModel = "mna_bjt_pnp_ebers_moll_v1"
		}
		if test.family == "bjt" && (len(record.SimulationModels) != 1 || record.SimulationModels[0].ModelID != wantModel) {
			t.Fatalf("%s trusted compact model = %#v, want %s", test.id, record.SimulationModels, wantModel)
		}
	}
}

func TestCheckedInCatalogMediumPowerDriverThermalEvidence(t *testing.T) {
	catalog, err := LoadCatalog(context.Background(), LoadOptions{CatalogDir: checkedInCatalogDir(t)})
	if err != nil {
		t.Fatalf("load checked-in catalog: %v", err)
	}
	for _, id := range []string{"bjt.onsemi.pzt3904t1g.sot223", "bjt.onsemi.pzt3906t1g.sot223"} {
		record := requireCatalogRecord(t, catalog, id)
		if record.Thermal == nil || record.Thermal.MaxJunctionTemperatureC == nil || record.Thermal.JunctionToAmbientCPerW == nil || *record.Thermal.JunctionToAmbientCPerW != 83.3 {
			t.Fatalf("%s thermal evidence = %#v", id, record.Thermal)
		}
		if len(record.SimulationModels) != 1 || len(record.Symbols) != 1 || len(record.Packages) != 1 || record.Verification.Confidence != ConfidenceVerified {
			t.Fatalf("%s fabrication identity is incomplete: model=%#v symbols=%#v packages=%#v verification=%#v", id, record.SimulationModels, record.Symbols, record.Packages, record.Verification)
		}
	}
}

func TestCheckedInCatalogSpeakerAmplifierComponentEvidence(t *testing.T) {
	catalog, err := LoadCatalog(context.Background(), LoadOptions{CatalogDir: checkedInCatalogDir(t)})
	if err != nil {
		t.Fatalf("load checked-in catalog: %v", err)
	}

	opamp := requireCatalogRecord(t, catalog, "opamp.ti.opa134ua.soic8")
	if opamp.OpAmp == nil || !opamp.OpAmp.FabricationProof {
		t.Fatalf("OPA134 fabrication evidence = %#v", opamp.OpAmp)
	}
	for name, status := range map[string]string{
		"output drive": opamp.OpAmp.OutputDriveStatus,
		"load":         opamp.OpAmp.LoadCompatibilityStatus,
		"bandwidth":    opamp.OpAmp.GainBandwidthStatus,
		"stability":    opamp.OpAmp.StabilityStatus,
		"common mode":  opamp.OpAmp.InputCommonModeStatus,
		"swing":        opamp.OpAmp.OutputSwingStatus,
		"noise":        opamp.OpAmp.NoiseStatus,
		"distortion":   opamp.OpAmp.DistortionStatus,
	} {
		if status != "proven" {
			t.Fatalf("OPA134 %s status = %q", name, status)
		}
	}
	requireSymbolFunctions(t, opamp, "Amplifier_Operational:OPA134", []string{"IN_MINUS", "IN_PLUS", "OUT", "V_MINUS", "V_PLUS"})
	requirePackagePads(t, opamp, "soic8", []string{"IN_MINUS", "IN_PLUS", "OUT", "V_MINUS", "V_PLUS"})
	for _, variant := range opamp.Packages {
		if variant.ID != "soic8" {
			continue
		}
		pads := map[string]string{}
		for _, mapping := range variant.PadFunctions {
			pads[mapping.Function] = mapping.Pad
		}
		if pads["OFFSET_TRIM_1"] != "1" || pads["NC"] != "5" || pads["OFFSET_TRIM_2"] != "8" {
			t.Fatalf("OPA134 auxiliary pad mapping = %#v, want legacy symbol pins 1/8 and hidden NC pad 5", pads)
		}
	}

	for _, test := range []struct {
		id       string
		polarity string
		symbol   string
	}{
		{id: "bjt.onsemi.mje243g.to225", polarity: "npn", symbol: "Transistor_BJT:Q_NPN_ECB"},
		{id: "bjt.onsemi.mje253g.to225", polarity: "pnp", symbol: "Transistor_BJT:Q_PNP_ECB"},
	} {
		record := requireCatalogRecord(t, catalog, test.id)
		evidence := record.PowerSemiconductor
		if evidence == nil || !evidence.FabricationProof || evidence.Polarity != test.polarity || evidence.ComplementaryGroup != "onsemi_mje243_mje253" || len(evidence.SOA) < 2 || evidence.BJT == nil {
			t.Fatalf("%s driver evidence = %#v", test.id, evidence)
		}
		requireSymbolFunctions(t, record, test.symbol, []string{"BASE", "COLLECTOR", "EMITTER"})
		requirePackagePads(t, record, "to225", []string{"BASE", "COLLECTOR", "EMITTER"})
		wantModel := "mna_bjt_npn_ebers_moll_v1"
		if test.polarity == "pnp" {
			wantModel = "mna_bjt_pnp_ebers_moll_v1"
		}
		if len(record.SimulationModels) != 1 || record.SimulationModels[0].ModelID != wantModel {
			t.Fatalf("%s trusted compact model = %#v, want %s", test.id, record.SimulationModels, wantModel)
		}
	}

	for _, test := range []struct {
		id    string
		value string
	}{
		{id: "resistor.vishay.ac03.0r22.axial", value: "0.22"},
		{id: "resistor.vishay.ac03.10r.axial", value: "10"},
	} {
		record := requireCatalogRecord(t, catalog, test.id)
		if record.Generic || record.MPN == "" || record.Verification.Confidence != ConfidenceVerified {
			t.Fatalf("%s fabrication identity = %#v", test.id, record)
		}
		if record.Resistor == nil || !record.Resistor.FabricationProof || record.Resistor.RatedPower == nil || record.Resistor.DeratedPower == nil {
			t.Fatalf("%s resistor derating evidence = %#v", test.id, record.Resistor)
		}
		requireValueTyp(t, record, "resistance", test.value, "ohm")
		requireRatingMax(t, record, "power", "3", "W")
		requirePackagePads(t, record, "axial_ac03", []string{"A", "B"})
	}
	baseStopper := requireCatalogRecord(t, catalog, "resistor.yageo.rc0805fr_0747rl.0805")
	requireValueTyp(t, baseStopper, "resistance", "47", "ohm")
	requirePackagePads(t, baseStopper, "0805", []string{"A", "B"})
	filmBypass := requireCatalogRecord(t, catalog, "capacitor.wima.mks2c031001a00kssd.tht")
	if filmBypass.Capacitor == nil || filmBypass.Capacitor.FabricationCandidateBlocks || filmBypass.Capacitor.DCBiasReview != "not_applicable" || filmBypass.Capacitor.EffectiveCapacitanceReview != "proven" || filmBypass.Capacitor.ESRReview != "proven" {
		t.Fatalf("film bypass evidence = %#v", filmBypass.Capacitor)
	}
	requireValueTyp(t, filmBypass, "capacitance", "100n", "F")
	requireRatingMax(t, filmBypass, "voltage", "63", "V")
	requirePackagePads(t, filmBypass, "mks2_pcm5", []string{"A", "B"})
	bulkCap := requireCatalogRecord(t, catalog, "capacitor.panasonic.eeufr1v221.radial")
	if bulkCap.Capacitor == nil || !bulkCap.Capacitor.FabricationProof || bulkCap.Capacitor.ESR == nil || bulkCap.Capacitor.RippleCurrent == nil || bulkCap.Capacitor.EnduranceHours == nil {
		t.Fatalf("low-ESR bulk evidence = %#v", bulkCap.Capacitor)
	}
	requireValueTyp(t, bulkCap, "capacitance", "220u", "F")
	requireRatingMax(t, bulkCap, "voltage", "35", "V")
	requirePackagePads(t, bulkCap, "radial_d8_p3_5", []string{"A", "B"})
	comparator := requireCatalogRecord(t, catalog, "comparator.ti.tlv1701aidbvr.sot23_5")
	if comparator.Generic || comparator.Lifecycle != "active" || comparator.Verification.Confidence != ConfidenceVerified {
		t.Fatalf("speaker protection comparator evidence = %#v", comparator)
	}
	requireSymbolFunctions(t, comparator, "Comparator:LMV331", []string{"IN_PLUS", "IN_MINUS", "OUT", "V_PLUS", "V_MINUS"})
	requirePackagePads(t, comparator, "sot23_5", []string{"IN_PLUS", "IN_MINUS", "OUT", "V_PLUS", "V_MINUS"})

	relay := requireCatalogRecord(t, catalog, "relay.omron.g5q_1a.dc12")
	if relay.Family != "relay" || relay.Generic || relay.MPN == "" || relay.Verification.Confidence != ConfidenceVerified {
		t.Fatalf("speaker relay evidence = %#v", relay)
	}
	requireRatingMax(t, relay, "contact_current_dc", "5", "A")
	requireSymbolFunctions(t, relay, "Relay:G5Q-1A", []string{"COIL_A", "COIL_B", "CONTACT_IN", "CONTACT_OUT"})
	requirePackagePads(t, relay, "g5q_1a", []string{"COIL_A", "COIL_B", "CONTACT_IN", "CONTACT_OUT"})
}

func TestCheckedInCatalogRegulatorSliceEvidence(t *testing.T) {
	catalog, err := LoadCatalog(context.Background(), LoadOptions{CatalogDir: checkedInCatalogDir(t)})
	if err != nil {
		t.Fatalf("load checked-in catalog: %v", err)
	}

	regulator := requireCatalogRecord(t, catalog, "regulator.linear.ams1117_3v3.sot223")
	if regulator.Verification.Confidence != ConfidenceVerified {
		t.Fatalf("regulator confidence = %q", regulator.Verification.Confidence)
	}
	requireRatingMax(t, regulator, "input_voltage", "12", "V")
	requireRatingMax(t, regulator, "output_current", "800", "mA")
	requireValueTyp(t, regulator, "output_voltage", "3.3", "V")
	for _, role := range []string{"input_capacitor", "output_capacitor"} {
		requireCompanionRole(t, regulator, role)
	}
	requireSymbolFunctions(t, regulator, "Regulator_Linear:AMS1117-3.3", []string{"GND", "VOUT", "VIN"})
	requirePackagePads(t, regulator, "sot223", []string{"GND", "VOUT", "VIN"})

	ap2112 := requireCatalogRecord(t, catalog, "regulator.linear.ap2112k_3v3.sot23_5")
	if ap2112.Verification.Confidence != ConfidenceVerified {
		t.Fatalf("AP2112K confidence = %q", ap2112.Verification.Confidence)
	}
	requireRatingMinMax(t, ap2112, "input_voltage", "3.8", "6", "V")
	requireRatingMax(t, ap2112, "output_current", "600", "mA")
	requireRatingMax(t, ap2112, "enable_voltage", "6", "V")
	requireRatingMax(t, ap2112, "enable_voltage_abs_max", "6.5", "V")
	requireRatingMax(t, ap2112, "power_dissipation_max", "250", "mW")
	requireValueTyp(t, ap2112, "output_voltage", "3.3", "V")
	requireValueMax(t, ap2112, "dropout_voltage", "400", "mV")
	requireValueTyp(t, ap2112, "headroom_margin", "100", "mV")
	for _, role := range []string{"input_capacitor", "output_capacitor"} {
		requireCompanionRole(t, ap2112, role)
	}
	requireSymbolFunctions(t, ap2112, "Regulator_Linear:AP2112K-3.3", []string{"VIN", "GND", "EN", "NC", "VOUT"})
	requirePackagePads(t, ap2112, "sot23_5", []string{"VIN", "GND", "EN", "NC", "VOUT"})
	requireDeratingRule(t, ap2112, "thermal")
	requireDeratingRule(t, ap2112, "enable_voltage")
	requireDeratingRule(t, ap2112, "capacitor_stability")
	requireRegulatorStability(t, regulator, "esr_window_required", true)
	requireRegulatorStability(t, ap2112, "ceramic_stable", true)

	ap2127 := requireCatalogRecord(t, catalog, "regulator.linear.ap2127k_adj.sot23_5")
	if ap2127.Verification.Confidence != ConfidenceVerified {
		t.Fatalf("AP2127K-ADJ confidence = %q", ap2127.Verification.Confidence)
	}
	requireRatingMinMax(t, ap2127, "input_voltage", "2.5", "6", "V")
	requireRatingMax(t, ap2127, "output_current", "300", "mA")
	requireSymbolFunctions(t, ap2127, "Regulator_Linear:AP2127K-ADJ", []string{"VIN", "GND", "EN", "ADJ", "VOUT"})
	requirePackagePads(t, ap2127, "sot23_5", []string{"VIN", "GND", "EN", "ADJ", "VOUT"})
	requireRegulatorStability(t, ap2127, "ceramic_stable", true)
	outputRangeFound := false
	for _, value := range ap2127.Values {
		if value.Kind == "output_voltage" && value.Min == "0.8" && value.Max == "5.5" && value.Unit == "V" {
			outputRangeFound = true
			break
		}
	}
	if !outputRangeFound {
		t.Fatalf("AP2127K-ADJ output range = %+v", ap2127.Values)
	}
	var feedback *CompanionValueFormula
	for _, companion := range ap2127.Companions {
		for _, recipe := range companion.Recipes {
			if recipe.ID == "feedback_upper" {
				feedback = recipe.ValueFormula
			}
		}
	}
	if feedback == nil || feedback.Kind != "divider_upper_from_output_v1" || feedback.Parameter != "output_voltage_v" || feedback.ReferenceVoltageV != 0.8 || feedback.LowerResistanceOhm != 10_000 || feedback.PreferredSeries != "E96" {
		t.Fatalf("AP2127K-ADJ feedback formula = %+v", feedback)
	}

	capacitor := requireCatalogRecord(t, catalog, "capacitor.ceramic.0805")
	if capacitor.Verification.Confidence != ConfidenceRuleInferred {
		t.Fatalf("capacitor confidence = %q", capacitor.Verification.Confidence)
	}
	requireValueMax(t, capacitor, "capacitance", "100u", "F")
	requireRatingMax(t, capacitor, "voltage", "25", "V")
	requireToleranceMax(t, capacitor, "capacitance", "20", "%")
	requireSymbolFunctions(t, capacitor, "Device:C", []string{"A", "B"})
	requirePackagePads(t, capacitor, "0805", []string{"A", "B"})
	requireCapacitorEvidence(t, capacitor, "unknown", true)

	requireCapacitorEvidence(t, requireCatalogRecord(t, catalog, "capacitor.murata.grm21br71h104ka01l.0805"), "X7R", true)
	requireCapacitorEvidence(t, requireCatalogRecord(t, catalog, "capacitor.murata.grm188r71h104ka93d.0603"), "X7R", true)
	requireCapacitorEvidence(t, requireCatalogRecord(t, catalog, "capacitor.murata.grm21br61a106ke19l.0805"), "X5R", true)

	npn := requireCatalogRecord(t, catalog, "bjt.onsemi.mmbt3904.sot23")
	requireAmplifierOutputEvidence(t, npn, "npn", false)
	requireRatingMax(t, npn, "collector_current", "200", "mA")
	requireRatingMax(t, npn, "collector_emitter_voltage", "40", "V")
	requireRatingMax(t, npn, "power_dissipation_max", "225", "mW")
	if npn.PowerSemiconductor == nil || !npn.PowerSemiconductor.FabricationProof || npn.PowerSemiconductor.JunctionToAmbientCPerW == nil {
		t.Fatalf("MMBT3904 typed power evidence = %#v", npn.PowerSemiconductor)
	}
	requireCompanionRole(t, npn, "emitter_resistor")

	pnp := requireCatalogRecord(t, catalog, "bjt.onsemi.mmbt3906.sot23")
	requireAmplifierOutputEvidence(t, pnp, "pnp", false)
	if pnp.PowerSemiconductor == nil || !pnp.PowerSemiconductor.FabricationProof || pnp.PowerSemiconductor.JunctionToAmbientCPerW == nil {
		t.Fatalf("MMBT3906 typed power evidence = %#v", pnp.PowerSemiconductor)
	}
	requireCompanionRole(t, pnp, "emitter_resistor")

	placeholder := requireCatalogRecord(t, catalog, "bjt.placeholder.npn_power_output.to220")
	requireAmplifierOutputEvidence(t, placeholder, "npn", true)
	if placeholder.Verification.Confidence != ConfidenceBlocked {
		t.Fatalf("power output placeholder confidence = %q, want blocked", placeholder.Verification.Confidence)
	}
}

func TestCheckedInCatalogSensorFamilyEvidence(t *testing.T) {
	catalog, err := LoadCatalog(context.Background(), LoadOptions{CatalogDir: checkedInCatalogDir(t)})
	if err != nil {
		t.Fatalf("load checked-in catalog: %v", err)
	}
	tests := []struct {
		id        string
		symbol    string
		pkg       string
		addresses []string
	}{
		{id: "sensor.bosch.bme280.lga8", symbol: "Sensor:BME280", pkg: "lga8", addresses: []string{"0x76", "0x77"}},
		{id: "sensor.bosch.bmp280.lga8", symbol: "Sensor_Pressure:BMP280", pkg: "lga8", addresses: []string{"0x76", "0x77"}},
		{id: "sensor.sensirion.sht31_dis.dfn8", symbol: "Sensor_Humidity:SHT31-DIS", pkg: "dfn8_ep", addresses: []string{"0x44", "0x45"}},
	}
	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			record := requireCatalogRecord(t, catalog, tt.id)
			if record.Verification.Confidence != ConfidenceVerified || record.Sensor == nil {
				t.Fatalf("sensor evidence = %#v", record.Sensor)
			}
			requireSymbolFunctions(t, record, tt.symbol, []string{"SDA", "SCL"})
			if len(record.Packages) != 1 || record.Packages[0].ID != tt.pkg || !record.Packages[0].Verification.PinMapChecked {
				t.Fatalf("package evidence = %#v", record.Packages)
			}
			got := make([]string, len(record.Sensor.I2CAddresses))
			for i, option := range record.Sensor.I2CAddresses {
				got[i] = option.Address
			}
			if !slices.Equal(got, tt.addresses) {
				t.Fatalf("addresses = %#v, want %#v", got, tt.addresses)
			}
		})
	}
}

func TestValidateCatalogSensorEvidenceRejectsMalformedMetadata(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*SensorEvidence)
		path   string
	}{
		{name: "reserved address", mutate: func(e *SensorEvidence) { e.I2CAddresses[0].Address = "0x02" }, path: "records[0].sensor_evidence.i2c_addresses[0].address"},
		{name: "missing default", mutate: func(e *SensorEvidence) { e.I2CAddresses[0].Default = false }, path: "records[0].sensor_evidence.i2c_addresses"},
		{name: "unknown select function", mutate: func(e *SensorEvidence) { e.I2CAddresses[0].SelectFunction = "MAGIC" }, path: "records[0].sensor_evidence.i2c_addresses[0].function"},
		{name: "unknown parent function", mutate: func(e *SensorEvidence) { e.I2CAddresses[0].ParentFunction = "MAGIC" }, path: "records[0].sensor_evidence.i2c_addresses[0].parent_function"},
		{name: "invalid pin level", mutate: func(e *SensorEvidence) { e.I2CModeConnections[0].Level = "floating" }, path: "records[0].sensor_evidence.i2c_mode_connections[0].level"},
		{name: "unknown interrupt", mutate: func(e *SensorEvidence) { e.OptionalInterruptFunction = "IRQ" }, path: "records[0].sensor_evidence.optional_interrupt_function"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			catalog := validCatalog()
			catalog.Families[0] = FamilyDefinition{ID: "sensor", Name: "Sensor"}
			record := &catalog.Records[0]
			record.ID = "sensor.example.i2c"
			record.Family = "sensor"
			record.Symbols[0].FunctionPins = []FunctionPin{
				{Function: "SDA", SymbolPin: "1"},
				{Function: "SCL", SymbolPin: "2"},
				{Function: "ADDR", SymbolPin: "3"},
			}
			record.Sensor = &SensorEvidence{
				Interfaces:         []string{"i2c"},
				I2CAddresses:       []SensorI2CAddress{{Address: "0x44", SelectFunction: "ADDR", Level: "low", Default: true}},
				I2CModeConnections: []SensorPinConnection{{Function: "ADDR", Level: "low"}},
			}
			tt.mutate(record.Sensor)
			result := ValidateCatalog(&catalog)
			if result.OK {
				t.Fatal("expected invalid sensor evidence to fail")
			}
			assertIssuePath(t, result.Issues, tt.path)
		})
	}
}

func checkedInCatalogDir(t *testing.T) string {
	t.Helper()
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate test source file")
	}
	return filepath.Join(filepath.Dir(sourceFile), "..", "..", "data", "components")
}

func requireCatalogRecord(t *testing.T, catalog *Catalog, id string) *ComponentRecord {
	t.Helper()
	for index := range catalog.Records {
		if catalog.Records[index].ID == id {
			return &catalog.Records[index]
		}
	}
	t.Fatalf("catalog missing record %s", id)
	return nil
}

func requireRatingMax(t *testing.T, record *ComponentRecord, kind, max, unit string) {
	t.Helper()
	for _, rating := range record.Ratings {
		if rating.Kind == kind && rating.Unit == unit && rating.Max == max {
			return
		}
	}
	t.Fatalf("%s missing max rating %s=%s%s: %+v", record.ID, kind, max, unit, record.Ratings)
}

func requireRatingMinMax(t *testing.T, record *ComponentRecord, kind, min, max, unit string) {
	t.Helper()
	for _, rating := range record.Ratings {
		if rating.Kind == kind && rating.Unit == unit && rating.Min == min && rating.Max == max {
			return
		}
	}
	t.Fatalf("%s missing min/max rating %s=%s..%s%s: %+v", record.ID, kind, min, max, unit, record.Ratings)
}

func requireValueTyp(t *testing.T, record *ComponentRecord, kind, typ, unit string) {
	t.Helper()
	for _, value := range record.Values {
		if value.Kind == kind && value.Unit == unit && value.Typ == typ {
			return
		}
	}
	t.Fatalf("%s missing typ value %s=%s%s: %+v", record.ID, kind, typ, unit, record.Values)
}

func requireValueMax(t *testing.T, record *ComponentRecord, kind, max, unit string) {
	t.Helper()
	for _, value := range record.Values {
		if value.Kind == kind && value.Unit == unit && value.Max == max {
			return
		}
	}
	t.Fatalf("%s missing max value %s=%s%s: %+v", record.ID, kind, max, unit, record.Values)
}

func requireToleranceMax(t *testing.T, record *ComponentRecord, kind, max, unit string) {
	t.Helper()
	for _, tolerance := range record.Tolerances {
		if tolerance.Kind == kind && tolerance.Unit == unit && tolerance.Max == max {
			return
		}
	}
	t.Fatalf("%s missing max tolerance %s=%s%s: %+v", record.ID, kind, max, unit, record.Tolerances)
}

func requireCompanionRole(t *testing.T, record *ComponentRecord, role string) {
	t.Helper()
	for _, companion := range record.Companions {
		if companion.Role == role && companion.Required {
			return
		}
	}
	t.Fatalf("%s missing required companion role %s: %+v", record.ID, role, record.Companions)
}

func requireDeratingRule(t *testing.T, record *ComponentRecord, kind string) {
	t.Helper()
	for _, rule := range record.DeratingRules {
		if rule.Kind == kind {
			return
		}
	}
	t.Fatalf("%s missing derating rule %s: %+v", record.ID, kind, record.DeratingRules)
}

func requireRegulatorStability(t *testing.T, record *ComponentRecord, kind string, blocksFabrication bool) {
	t.Helper()
	if record.Regulator == nil || record.Regulator.OutputCapacitor == nil {
		t.Fatalf("%s missing regulator output-capacitor evidence", record.ID)
	}
	stability := record.Regulator.OutputCapacitor
	if stability.Kind != kind {
		t.Fatalf("%s stability kind = %q, want %q", record.ID, stability.Kind, kind)
	}
	if stability.FabricationCandidateBlocks != blocksFabrication {
		t.Fatalf("%s fabrication block = %t, want %t", record.ID, stability.FabricationCandidateBlocks, blocksFabrication)
	}
}

func requireCapacitorEvidence(t *testing.T, record *ComponentRecord, dielectric string, blocksFabrication bool) {
	t.Helper()
	if record.Capacitor == nil {
		t.Fatalf("%s missing capacitor evidence", record.ID)
	}
	if record.Capacitor.Dielectric != dielectric {
		t.Fatalf("%s dielectric = %q, want %q", record.ID, record.Capacitor.Dielectric, dielectric)
	}
	if record.Capacitor.FabricationCandidateBlocks != blocksFabrication {
		t.Fatalf("%s fabrication block = %t, want %t", record.ID, record.Capacitor.FabricationCandidateBlocks, blocksFabrication)
	}
}

func requireAmplifierOutputEvidence(t *testing.T, record *ComponentRecord, polarity string, blocksFabrication bool) {
	t.Helper()
	if record.AmplifierOutput == nil {
		t.Fatalf("%s missing amplifier output evidence", record.ID)
	}
	evidence := record.AmplifierOutput
	if evidence.DeviceClass != "bjt" {
		t.Fatalf("%s device class = %q, want bjt", record.ID, evidence.DeviceClass)
	}
	if evidence.Polarity != polarity {
		t.Fatalf("%s polarity = %q, want %q", record.ID, evidence.Polarity, polarity)
	}
	if evidence.Package == "" || evidence.SymbolID == "" || evidence.FootprintID == "" || evidence.PinmapEvidence == "" {
		t.Fatalf("%s missing package/symbol/footprint/pinmap evidence: %+v", record.ID, evidence)
	}
	if evidence.ComplementaryGroup == "" {
		t.Fatalf("%s missing complementary group: %+v", record.ID, evidence)
	}
	if evidence.ControlTerminal == "" || evidence.UpperOrLowerTerminal == "" || evidence.OutputTerminal == "" {
		t.Fatalf("%s missing terminal role mapping: %+v", record.ID, evidence)
	}
	if evidence.FabricationCandidateBlocks != blocksFabrication {
		t.Fatalf("%s fabrication block = %t, want %t", record.ID, evidence.FabricationCandidateBlocks, blocksFabrication)
	}
}

func requireSymbolFunctions(t *testing.T, record *ComponentRecord, symbolID string, functions []string) {
	t.Helper()
	for _, symbol := range record.Symbols {
		if symbol.SymbolID != symbolID {
			continue
		}
		for _, function := range functions {
			if !symbolHasFunction(symbol, function) {
				t.Fatalf("%s symbol %s missing function %s: %+v", record.ID, symbolID, function, symbol.FunctionPins)
			}
		}
		return
	}
	t.Fatalf("%s missing symbol %s", record.ID, symbolID)
}

func requirePackagePads(t *testing.T, record *ComponentRecord, packageID string, functions []string) {
	t.Helper()
	for _, pkg := range record.Packages {
		if pkg.ID != packageID {
			continue
		}
		for _, function := range functions {
			if !packageHasPadFunction(pkg, function) {
				t.Fatalf("%s package %s missing pad function %s: %+v", record.ID, packageID, function, pkg.PadFunctions)
			}
		}
		return
	}
	t.Fatalf("%s missing package %s", record.ID, packageID)
}

func symbolHasFunction(symbol SymbolBinding, function string) bool {
	for _, pin := range symbol.FunctionPins {
		if pin.Function == function && pin.SymbolPin != "" {
			return true
		}
	}
	return false
}

func packageHasPadFunction(pkg PackageVariant, function string) bool {
	for _, pad := range pkg.PadFunctions {
		if pad.Function == function && pad.Pad != "" {
			return true
		}
	}
	return false
}

func TestValidateCatalogDuplicateID(t *testing.T) {
	catalog := validCatalog()
	catalog.Records = append(catalog.Records, catalog.Records[0])
	result := ValidateCatalog(&catalog)
	if result.OK {
		t.Fatal("expected duplicate id to fail")
	}
	assertIssueCode(t, result.Issues, CodeDuplicateComponentID)
}

func catalogFamilyCoverage(catalog *Catalog) map[string]bool {
	covered := map[string]bool{}
	for _, record := range catalog.Records {
		covered[record.Family] = true
	}
	return covered
}

func TestValidateCatalogUnknownFamily(t *testing.T) {
	catalog := validCatalog()
	catalog.Records[0].Family = "unknown"
	result := ValidateCatalog(&catalog)
	if result.OK {
		t.Fatal("expected unknown family to fail")
	}
	assertIssueCode(t, result.Issues, CodeUnknownFamily)
}

func TestValidateCatalogDuplicateFamily(t *testing.T) {
	catalog := validCatalog()
	catalog.Families = append(catalog.Families, catalog.Families[0])
	result := ValidateCatalog(&catalog)
	if result.OK {
		t.Fatal("expected duplicate family to fail")
	}
	assertIssueCode(t, result.Issues, CodeInvalidComponentFamily)
}

func TestValidateCatalogMissingFootprint(t *testing.T) {
	catalog := validCatalog()
	catalog.Records[0].Packages[0].FootprintID = ""
	result := ValidateCatalog(&catalog)
	if result.OK {
		t.Fatal("expected missing footprint to fail")
	}
	assertIssueCode(t, result.Issues, CodeMissingFootprint)
}

func TestValidateCatalogInvalidConfidence(t *testing.T) {
	catalog := validCatalog()
	catalog.Records[0].Verification.Confidence = "maybe"
	result := ValidateCatalog(&catalog)
	if result.OK {
		t.Fatal("expected invalid confidence to fail")
	}
	assertIssueCode(t, result.Issues, CodeInvalidConfidence)
}

func TestValidateCatalogExtendedMetadata(t *testing.T) {
	catalog := validCatalog()
	record := &catalog.Records[0]
	record.Lifecycle = "active"
	record.Tolerances = []ToleranceConstraint{{Kind: "resistance", Max: "1", Unit: "%"}}
	record.Temperature = &TemperatureRange{Min: "-40", Max: "85", Unit: "C"}
	record.Companions = []CompanionRequirement{{
		ID:       "cap.input",
		Family:   "capacitor",
		Role:     "input_capacitor",
		Required: true,
	}}
	record.DeratingRules = []DeratingRule{{Kind: "voltage", Expression: "rated_voltage >= 2 * operating_voltage"}}
	record.PlacementHints = []PlacementHint{{Kind: "near", Target: "power_pin", Value: "2", Unit: "mm"}}
	record.RoutingHints = []RoutingHint{{Kind: "net_class", NetRole: "power", Value: "0.25", Unit: "mm"}}
	record.Properties = []SchematicProperty{{Name: "MPN", Value: "GENERIC-0805"}}
	record.Packages[0].MPN = "GENERIC-0805-PKG"
	record.Packages[0].Lifecycle = "preferred"
	record.Packages[0].HeightMM = 0.55

	result := ValidateCatalog(&catalog)
	if !result.OK {
		t.Fatalf("expected extended metadata to validate: %+v", result.Issues)
	}
}

func TestValidateCatalogInvalidExtendedMetadata(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Catalog)
		code   reports.Code
	}{
		{
			name: "lifecycle",
			mutate: func(c *Catalog) {
				c.Records[0].Lifecycle = "shipping"
			},
			code: CodeInvalidLifecycle,
		},
		{
			name: "tolerance",
			mutate: func(c *Catalog) {
				c.Records[0].Tolerances = []ToleranceConstraint{{Kind: "resistance", Max: "one", Unit: "%"}}
			},
			code: CodeInvalidConstraint,
		},
		{
			name: "duplicate companion",
			mutate: func(c *Catalog) {
				c.Records[0].Companions = []CompanionRequirement{
					{ID: "c1", Role: "decoupling", Required: true},
					{ID: "c1", Role: "decoupling", Required: true},
				}
			},
			code: CodeInvalidMetadata,
		},
		{
			name: "placement unit",
			mutate: func(c *Catalog) {
				c.Records[0].PlacementHints = []PlacementHint{{Kind: "near", Value: "2"}}
			},
			code: CodeInvalidMetadata,
		},
		{
			name: "negative height",
			mutate: func(c *Catalog) {
				c.Records[0].Packages[0].HeightMM = -1
			},
			code: CodeInvalidMetadata,
		},
		{
			name: "duplicate property",
			mutate: func(c *Catalog) {
				c.Records[0].Properties = []SchematicProperty{{Name: "MPN", Value: "a"}, {Name: "MPN", Value: "b"}}
			},
			code: CodeInvalidMetadata,
		},
		{
			name: "untrimmed lifecycle",
			mutate: func(c *Catalog) {
				c.Records[0].Lifecycle = "active "
			},
			code: CodeInvalidLifecycle,
		},
		{
			name: "untrimmed property",
			mutate: func(c *Catalog) {
				c.Records[0].Properties = []SchematicProperty{{Name: " MPN", Value: "a"}}
			},
			code: CodeInvalidMetadata,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			catalog := validCatalog()
			tt.mutate(&catalog)
			result := ValidateCatalog(&catalog)
			if result.OK {
				t.Fatal("expected validation to fail")
			}
			assertIssueCode(t, result.Issues, tt.code)
		})
	}
}

func TestValidateCatalogEquivalenceMetadata(t *testing.T) {
	catalog := validCatalog()
	catalog.Records = append(catalog.Records, catalog.Records[0])
	catalog.Records[0].ID = "resistor.yageo.10k.0805"
	catalog.Records[0].Generic = false
	catalog.Records[0].Equivalence = &EquivalenceMetadata{Group: "resistor.10k.0805", Role: EquivalencePreferred}
	catalog.Records[1].ID = "resistor.generic.10k.0805"
	catalog.Records[1].Equivalence = &EquivalenceMetadata{Group: "resistor.10k.0805", Role: EquivalenceFallback}

	result := ValidateCatalog(&catalog)
	if !result.OK {
		t.Fatalf("expected equivalence metadata to validate: %+v", result.Issues)
	}
}

func TestValidateCatalogInvalidEquivalenceMetadata(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Catalog)
	}{
		{
			name: "missing group",
			mutate: func(c *Catalog) {
				c.Records[0].Equivalence = &EquivalenceMetadata{Role: EquivalencePreferred}
			},
		},
		{
			name: "invalid role",
			mutate: func(c *Catalog) {
				c.Records[0].Equivalence = &EquivalenceMetadata{Group: "resistor.10k.0805", Role: "maybe"}
			},
		},
		{
			name: "multiple preferred",
			mutate: func(c *Catalog) {
				c.Records = append(c.Records, c.Records[0])
				c.Records[0].ID = "resistor.a.0805"
				c.Records[1].ID = "resistor.b.0805"
				c.Records[0].Equivalence = &EquivalenceMetadata{Group: "resistor.10k.0805", Role: EquivalencePreferred}
				c.Records[1].Equivalence = &EquivalenceMetadata{Group: "resistor.10k.0805", Role: EquivalencePreferred}
			},
		},
		{
			name: "missing preferred",
			mutate: func(c *Catalog) {
				c.Records[0].Equivalence = &EquivalenceMetadata{Group: "resistor.10k.0805", Role: EquivalenceAlternate}
			},
		},
		{
			name: "incompatible values",
			mutate: func(c *Catalog) {
				c.Records = append(c.Records, c.Records[0])
				c.Records[0].ID = "resistor.10k.0805"
				c.Records[1].ID = "resistor.1k.0805"
				c.Records[1].Values = []ValueConstraint{{Kind: "resistance", Typ: "1k", Unit: "ohm"}}
				c.Records[0].Equivalence = &EquivalenceMetadata{Group: "resistor.10k.0805", Role: EquivalencePreferred}
				c.Records[1].Equivalence = &EquivalenceMetadata{Group: "resistor.10k.0805", Role: EquivalenceAlternate}
			},
		},
		{
			name: "incompatible package",
			mutate: func(c *Catalog) {
				c.Records = append(c.Records, c.Records[0])
				c.Records[0].ID = "resistor.0805"
				c.Records[1].ID = "resistor.0603"
				c.Records[1].Packages = []PackageVariant{{
					ID:          "0603",
					Name:        "0603",
					FootprintID: "Resistor_SMD:R_0603_1608Metric",
					PadFunctions: []PadFunction{
						{Function: "A", Pad: "1"},
						{Function: "B", Pad: "2"},
					},
					Verification: VerificationRecord{Confidence: ConfidenceRuleInferred},
				}}
				c.Records[1].Equivalence = &EquivalenceMetadata{Group: "resistor.10k.0805", Role: EquivalenceAlternate}
				c.Records[0].Equivalence = &EquivalenceMetadata{Group: "resistor.10k.0805", Role: EquivalencePreferred}
			},
		},
		{
			name: "incompatible pad map",
			mutate: func(c *Catalog) {
				c.Records = append(c.Records, c.Records[0])
				c.Records[0].ID = "resistor.a.0805"
				c.Records[1].ID = "resistor.b.0805"
				c.Records[1].Packages = []PackageVariant{c.Records[1].Packages[0]}
				c.Records[1].Packages[0].PadFunctions = []PadFunction{
					{Function: "A", Pad: "2"},
					{Function: "B", Pad: "1"},
				}
				c.Records[0].Equivalence = &EquivalenceMetadata{Group: "resistor.10k.0805", Role: EquivalencePreferred}
				c.Records[1].Equivalence = &EquivalenceMetadata{Group: "resistor.10k.0805", Role: EquivalenceAlternate}
			},
		},
		{
			name: "incompatible rating",
			mutate: func(c *Catalog) {
				c.Records = append(c.Records, c.Records[0])
				c.Records[0].ID = "resistor.a.0805"
				c.Records[1].ID = "resistor.b.0805"
				c.Records[0].Ratings = []RatingConstraint{{Kind: "power", Max: "125", Unit: "mW"}}
				c.Records[1].Ratings = []RatingConstraint{{Kind: "power", Max: "63", Unit: "mW"}}
				c.Records[0].Equivalence = &EquivalenceMetadata{Group: "resistor.10k.0805", Role: EquivalencePreferred}
				c.Records[1].Equivalence = &EquivalenceMetadata{Group: "resistor.10k.0805", Role: EquivalenceAlternate}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			catalog := validCatalog()
			tt.mutate(&catalog)
			result := ValidateCatalog(&catalog)
			if result.OK {
				t.Fatal("expected validation to fail")
			}
			assertIssueCode(t, result.Issues, CodeInvalidMetadata)
		})
	}
}

func TestValidateCatalogTemperaturePath(t *testing.T) {
	catalog := validCatalog()
	catalog.Records[0].Temperature = &TemperatureRange{Min: "cold", Max: "85", Unit: "C"}
	result := ValidateCatalog(&catalog)
	if result.OK {
		t.Fatal("expected invalid temperature to fail")
	}
	for _, issue := range result.Issues {
		if issue.Path == "records[0].temperature.min" {
			return
		}
	}
	t.Fatalf("expected temperature min path in %+v", result.Issues)
}

func TestValidateCatalogRegulatorEvidenceRejectsMalformedStability(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(record *ComponentRecord)
		path   string
	}{
		{
			name: "invalid stability kind",
			mutate: func(record *ComponentRecord) {
				record.Regulator = &RegulatorEvidence{OutputCapacitor: &RegulatorCapacitorStability{
					Kind:            "magic",
					MinCapacitance:  "10u",
					CapacitanceUnit: "F",
				}}
			},
			path: "records[0].regulator_evidence.output_capacitor.kind",
		},
		{
			name: "missing required capacitance",
			mutate: func(record *ComponentRecord) {
				record.Regulator = &RegulatorEvidence{OutputCapacitor: &RegulatorCapacitorStability{
					Kind: "ceramic_stable",
				}}
			},
			path: "records[0].regulator_evidence.output_capacitor.min_capacitance",
		},
		{
			name: "ESR minimum greater than maximum",
			mutate: func(record *ComponentRecord) {
				record.Regulator = &RegulatorEvidence{OutputCapacitor: &RegulatorCapacitorStability{
					Kind:            "esr_window_required",
					MinCapacitance:  "10u",
					CapacitanceUnit: "F",
					ESRMin:          "2",
					ESRMax:          "0.5",
					ESRUnit:         "ohm",
				}}
			},
			path: "records[0].regulator_evidence.output_capacitor.esr_min",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			catalog := validCatalog()
			tt.mutate(&catalog.Records[0])
			result := ValidateCatalog(&catalog)
			if result.OK {
				t.Fatal("expected validation to fail")
			}
			assertIssueCode(t, result.Issues, CodeInvalidMetadata)
			assertIssuePath(t, result.Issues, tt.path)
		})
	}
}

func TestValidateCatalogCapacitorEvidenceRejectsMalformedMetadata(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(record *ComponentRecord)
		path   string
	}{
		{
			name: "invalid nominal capacitance",
			mutate: func(record *ComponentRecord) {
				record.Capacitor = &CapacitorEvidence{
					NominalCapacitance: "many",
					CapacitanceUnit:    "F",
				}
			},
			path: "records[0].capacitor_evidence.nominal_capacitance",
		},
		{
			name: "invalid voltage rating",
			mutate: func(record *ComponentRecord) {
				record.Capacitor = &CapacitorEvidence{
					VoltageRating: "high",
					VoltageUnit:   "V",
				}
			},
			path: "records[0].capacitor_evidence.voltage_rating",
		},
		{
			name: "generic fabrication proof",
			mutate: func(record *ComponentRecord) {
				record.Capacitor = &CapacitorEvidence{
					NominalCapacitance: "100n",
					CapacitanceUnit:    "F",
					FabricationProof:   true,
				}
			},
			path: "records[0].capacitor_evidence.fabrication_proof",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			catalog := validCatalog()
			catalog.Records[0].Family = "capacitor"
			catalog.Families = append(catalog.Families, FamilyDefinition{ID: "capacitor", Name: "Capacitor"})
			tt.mutate(&catalog.Records[0])
			result := ValidateCatalog(&catalog)
			if result.OK {
				t.Fatal("expected validation to fail")
			}
			assertIssueCode(t, result.Issues, CodeInvalidMetadata)
			assertIssuePath(t, result.Issues, tt.path)
		})
	}
}

func TestValidateCatalogAmplifierOutputEvidenceRejectsMalformedMetadata(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(record *ComponentRecord)
		path   string
	}{
		{
			name: "missing symbol",
			mutate: func(record *ComponentRecord) {
				record.AmplifierOutput = validAmplifierOutputEvidence()
				record.AmplifierOutput.SymbolID = ""
			},
			path: "records[0].amplifier_output_evidence.symbol_id",
		},
		{
			name: "invalid polarity",
			mutate: func(record *ComponentRecord) {
				record.AmplifierOutput = validAmplifierOutputEvidence()
				record.AmplifierOutput.Polarity = "sideways"
			},
			path: "records[0].amplifier_output_evidence.polarity",
		},
		{
			name: "missing intended role",
			mutate: func(record *ComponentRecord) {
				record.AmplifierOutput = validAmplifierOutputEvidence()
				record.AmplifierOutput.IntendedRoles = nil
			},
			path: "records[0].amplifier_output_evidence.intended_roles",
		},
		{
			name: "invalid thermal status",
			mutate: func(record *ComponentRecord) {
				record.AmplifierOutput = validAmplifierOutputEvidence()
				record.AmplifierOutput.ThermalReview = "maybe"
			},
			path: "records[0].amplifier_output_evidence.thermal_review",
		},
		{
			name: "symbol does not match binding",
			mutate: func(record *ComponentRecord) {
				record.AmplifierOutput = validAmplifierOutputEvidence()
				record.AmplifierOutput.SymbolID = "Device:Q_PNP_BEC"
			},
			path: "records[0].amplifier_output_evidence.symbol_id",
		},
		{
			name: "footprint does not match package",
			mutate: func(record *ComponentRecord) {
				record.AmplifierOutput = validAmplifierOutputEvidence()
				record.AmplifierOutput.FootprintID = "Package_TO_SOT_THT:TO-220-3_Vertical"
			},
			path: "records[0].amplifier_output_evidence.footprint_id",
		},
		{
			name: "pinmap evidence does not match sources",
			mutate: func(record *ComponentRecord) {
				record.AmplifierOutput = validAmplifierOutputEvidence()
				record.AmplifierOutput.PinmapEvidence = "builtin_pinmap:missing"
			},
			path: "records[0].amplifier_output_evidence.pinmap_evidence",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			catalog := validCatalog()
			catalog.Records[0].Family = "bjt"
			catalog.Families = append(catalog.Families, FamilyDefinition{ID: "bjt", Name: "BJT"})
			catalog.Records[0].Symbols = []SymbolBinding{{
				SymbolID: "Device:Q_NPN_BEC",
				FunctionPins: []FunctionPin{
					{Function: "BASE", SymbolPin: "1", Required: true},
					{Function: "EMITTER", SymbolPin: "2", Required: true},
					{Function: "COLLECTOR", SymbolPin: "3", Required: true},
				},
				Verification: VerificationRecord{Confidence: ConfidenceVerified, Sources: []string{"builtin_pinmap:Device:Q_NPN_BEC"}, PinMapChecked: true},
			}}
			catalog.Records[0].Packages = []PackageVariant{{
				ID:          "sot23",
				Name:        "SOT-23",
				FootprintID: "Package_TO_SOT_SMD:SOT-23",
				PadFunctions: []PadFunction{
					{Function: "BASE", Pad: "1"},
					{Function: "EMITTER", Pad: "2"},
					{Function: "COLLECTOR", Pad: "3"},
				},
				Verification: VerificationRecord{Confidence: ConfidenceVerified, Sources: []string{"builtin_pinmap:Device:Q_NPN_BEC"}, PinMapChecked: true},
			}}
			tt.mutate(&catalog.Records[0])
			result := ValidateCatalog(&catalog)
			if result.OK {
				t.Fatal("expected validation to fail")
			}
			assertIssueCode(t, result.Issues, CodeInvalidMetadata)
			assertIssuePath(t, result.Issues, tt.path)
		})
	}
}

func TestValidateCatalogOpAmpEvidenceRejectsMalformedMetadata(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(record *ComponentRecord)
		path   string
	}{
		{
			name: "missing intended role",
			mutate: func(record *ComponentRecord) {
				record.OpAmp = validOpAmpEvidence()
				record.OpAmp.IntendedRoles = nil
			},
			path: "records[0].opamp_evidence.intended_roles",
		},
		{
			name: "missing supply mode",
			mutate: func(record *ComponentRecord) {
				record.OpAmp = validOpAmpEvidence()
				record.OpAmp.SupplyMode = ""
			},
			path: "records[0].opamp_evidence.supply_mode",
		},
		{
			name: "invalid supply mode",
			mutate: func(record *ComponentRecord) {
				record.OpAmp = validOpAmpEvidence()
				record.OpAmp.SupplyMode = "battery_magic"
			},
			path: "records[0].opamp_evidence.supply_mode",
		},
		{
			name: "invalid status",
			mutate: func(record *ComponentRecord) {
				record.OpAmp = validOpAmpEvidence()
				record.OpAmp.StabilityStatus = "probably"
			},
			path: "records[0].opamp_evidence.stability_status",
		},
		{
			name: "missing output drive status",
			mutate: func(record *ComponentRecord) {
				record.OpAmp = validOpAmpEvidence()
				record.OpAmp.OutputDriveStatus = ""
			},
			path: "records[0].opamp_evidence.output_drive_status",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			catalog := validCatalog()
			catalog.Records[0].Family = "opamp"
			catalog.Families = append(catalog.Families, FamilyDefinition{ID: "opamp", Name: "Op-Amp"})
			tt.mutate(&catalog.Records[0])
			result := ValidateCatalog(&catalog)
			if result.OK {
				t.Fatal("expected validation to fail")
			}
			assertIssueCode(t, result.Issues, CodeInvalidMetadata)
			assertIssuePath(t, result.Issues, tt.path)
		})
	}
}

func TestValidateCatalogRejectsInvalidNamedSymbolUnits(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*ComponentRecord)
		path   string
	}{
		{name: "duplicate id after normalization", mutate: func(record *ComponentRecord) { record.Symbols[1].UnitID = "A" }, path: "records[0].symbols[1].unit_id"},
		{name: "duplicate KiCad unit", mutate: func(record *ComponentRecord) { record.Symbols[1].Unit = 1 }, path: "records[0].symbols[1].unit"},
		{name: "mixed named and anonymous", mutate: func(record *ComponentRecord) { record.Symbols[1].UnitID = ""; record.Symbols[1].UnitType = "" }, path: "records[0].symbols"},
		{name: "missing power unit", mutate: func(record *ComponentRecord) { record.Symbols = record.Symbols[:2] }, path: "records[0].symbols"},
		{name: "power unit is optional", mutate: func(record *ComponentRecord) { record.Symbols[2].RequiredUnit = false }, path: "records[0].symbols[2].required_unit"},
		{name: "invalid unit type", mutate: func(record *ComponentRecord) { record.Symbols[0].UnitType = "magic" }, path: "records[0].symbols[0].unit_type"},
		{name: "zero named unit", mutate: func(record *ComponentRecord) { record.Symbols[0].Unit = 0 }, path: "records[0].symbols[0].unit"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			catalog := validCatalog()
			catalog.Records[0].Family = "opamp"
			catalog.Families = append(catalog.Families, FamilyDefinition{ID: "opamp", Name: "Op-Amp"})
			catalog.Records[0].Symbols = validNamedOpAmpSymbols()
			tt.mutate(&catalog.Records[0])
			result := ValidateCatalog(&catalog)
			if result.OK {
				t.Fatal("expected named unit validation failure")
			}
			assertIssueCode(t, result.Issues, CodeInvalidSymbolUnit)
			assertIssuePath(t, result.Issues, tt.path)
		})
	}
}

func validNamedOpAmpSymbols() []SymbolBinding {
	verification := VerificationRecord{Confidence: ConfidenceVerified, Sources: []string{"unit-test"}, PinMapChecked: true}
	return []SymbolBinding{
		{SymbolID: "Amplifier_Operational:LM358", Unit: 1, UnitID: "A", UnitType: SymbolUnitFunctional, FunctionPins: []FunctionPin{{Function: "OUT", SymbolPin: "1", Required: true}}, Verification: verification},
		{SymbolID: "Amplifier_Operational:LM358", Unit: 2, UnitID: "B", UnitType: SymbolUnitFunctional, FunctionPins: []FunctionPin{{Function: "OUT", SymbolPin: "7", Required: true}}, Verification: verification},
		{SymbolID: "Amplifier_Operational:LM358", Unit: 3, UnitID: "P", UnitType: SymbolUnitPower, RequiredUnit: true, FunctionPins: []FunctionPin{{Function: "V_PLUS", SymbolPin: "8", Required: true}}, Verification: verification},
	}
}

func validAmplifierOutputEvidence() *AmplifierOutputEvidence {
	return &AmplifierOutputEvidence{
		DeviceClass:                "bjt",
		Polarity:                   "npn",
		IntendedRoles:              []string{"headphone_output"},
		Package:                    "SOT-23",
		SymbolID:                   "Device:Q_NPN_BEC",
		FootprintID:                "Package_TO_SOT_SMD:SOT-23",
		PinmapEvidence:             "builtin_pinmap:Device:Q_NPN_BEC",
		ComplementaryGroup:         "mmbt390x_sot23",
		ControlTerminal:            "BASE",
		UpperOrLowerTerminal:       "COLLECTOR",
		OutputTerminal:             "EMITTER",
		VoltageRatingStatus:        "proven",
		CurrentRatingStatus:        "proven",
		PowerDissipationStatus:     "review_required",
		ThermalReview:              "review_required",
		SafeOperatingAreaStatus:    "review_required",
		FabricationCandidateBlocks: true,
	}
}

func validOpAmpEvidence() *OpAmpEvidence {
	return &OpAmpEvidence{
		IntendedRoles:              []string{"gain_stage"},
		SupplyMode:                 "rail_to_rail_single_supply",
		OutputDriveStatus:          "review_required",
		LoadCompatibilityStatus:    "review_required",
		GainBandwidthStatus:        "review_required",
		StabilityStatus:            "review_required",
		InputCommonModeStatus:      "proven",
		OutputSwingStatus:          "review_required",
		NoiseStatus:                "review_required",
		DistortionStatus:           "review_required",
		FabricationCandidateBlocks: true,
	}
}

func validCatalog() Catalog {
	return Catalog{
		Version: CatalogVersion,
		Families: []FamilyDefinition{{
			ID:   "resistor",
			Name: "Resistor",
		}},
		Records: []ComponentRecord{{
			ID:      "resistor.generic.0805",
			Family:  "resistor",
			Name:    "Generic 0805 resistor",
			Generic: true,
			Values: []ValueConstraint{{
				Kind: "resistance",
				Typ:  "10k",
				Unit: "ohm",
			}},
			Symbols: []SymbolBinding{{
				SymbolID: "Device:R",
				FunctionPins: []FunctionPin{
					{Function: "A", SymbolPin: "1", Required: true},
					{Function: "B", SymbolPin: "2", Required: true},
				},
				Verification: VerificationRecord{Confidence: ConfidenceRuleInferred, Sources: []string{"builtin_pinmap:Device:R"}},
			}},
			Packages: []PackageVariant{{
				ID:          "0805",
				Name:        "0805",
				FootprintID: "Resistor_SMD:R_0805_2012Metric",
				PadFunctions: []PadFunction{
					{Function: "A", Pad: "1"},
					{Function: "B", Pad: "2"},
				},
				Verification: VerificationRecord{Confidence: ConfidenceRuleInferred, Sources: []string{"builtin_pinmap:Device:R"}},
			}},
			Verification: VerificationRecord{Confidence: ConfidenceRuleInferred, Sources: []string{"builtin_pinmap:Device:R"}},
		}},
	}
}

func validRecordJSON(id string, family string, variant string) string {
	return `{
  "id": "` + id + `",
  "family": "` + family + `",
  "name": "` + id + `",
  "generic": true,
  "values": [{"kind": "resistance", "typ": "10k", "unit": "ohm"}],
  "symbols": [{
    "symbol_id": "Device:R",
    "function_pins": [
      {"function": "A", "symbol_pin": "1", "required": true},
      {"function": "B", "symbol_pin": "2", "required": true}
    ],
    "verification": {"confidence": "rule_inferred"}
  }],
  "packages": [{
    "id": "` + variant + `",
    "name": "` + variant + `",
    "footprint_id": "Resistor_SMD:R_0805_2012Metric",
    "pad_functions": [
      {"function": "A", "pad": "1"},
      {"function": "B", "pad": "2"}
    ],
    "verification": {"confidence": "rule_inferred"}
  }],
  "verification": {"confidence": "rule_inferred"}
}`
}

func writeCatalogFile(t *testing.T, dir string, name string, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatalf("write catalog file: %v", err)
	}
}

func assertIssueCode(t *testing.T, issues []reports.Issue, code reports.Code) {
	t.Helper()
	for _, issue := range issues {
		if issue.Code == code {
			return
		}
	}
	t.Fatalf("expected issue code %s in %+v", code, issues)
}

func assertIssuePath(t *testing.T, issues []reports.Issue, path string) {
	t.Helper()
	for _, issue := range issues {
		if issue.Path == path {
			return
		}
	}
	t.Fatalf("expected issue path %s in %+v", path, issues)
}
