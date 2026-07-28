package components

import (
	"context"
	"slices"
	"strconv"
	"strings"
	"testing"
)

func TestCheckedInCatalogTransistorTesterVerifiedParts(t *testing.T) {
	catalog, err := LoadCatalog(context.Background(), LoadOptions{CatalogDir: checkedInCatalogDir(t)})
	if err != nil {
		t.Fatalf("load checked-in catalog: %v", err)
	}

	relay := requireCatalogRecord(t, catalog, "relay.omron.g5v_1.dc5")
	if relay.MPN != "G5V-1 DC5" || relay.Verification.Confidence != ConfidenceVerified {
		t.Fatalf("relay identity = %#v", relay)
	}
	requireSymbolFunctions(t, relay, "Relay:G5V-1", []string{"COIL_A", "COIL_B", "COMMON", "NORMALLY_OPEN", "NORMALLY_CLOSED"})
	requirePackagePads(t, relay, "g5v_1_tht", []string{"COIL_A", "COIL_B", "COMMON", "NORMALLY_OPEN", "NORMALLY_CLOSED"})
	requireFunctionPins(t, relay, "Relay:G5V-1", map[string][]string{
		"COIL_A":          {"2"},
		"COIL_B":          {"9"},
		"COMMON":          {"5", "6"},
		"NORMALLY_OPEN":   {"10"},
		"NORMALLY_CLOSED": {"1"},
	})
	requireValueTyp(t, relay, "coil_voltage", "5", "V")

	driver := requireCatalogRecord(t, catalog, "driver.st.uln2803a.dip18")
	if driver.MPN != "ULN2803A" || driver.Verification.Confidence != ConfidenceVerified {
		t.Fatalf("driver identity = %#v", driver)
	}
	if !recordHasElectricalRole(driver, "relay_driver") {
		t.Fatalf("driver lacks relay-driver role: %#v", driver.ElectricalRoles)
	}
	requireSymbolFunctions(t, driver, "Transistor_Array:ULN2803A", []string{"GND", "COM"})
	requirePackagePads(t, driver, "dip18", []string{"GND", "COM"})
	for channel := 1; channel <= 8; channel++ {
		input := "IN_" + strconv.Itoa(channel)
		output := "OUT_" + strconv.Itoa(channel)
		requireFunctionPins(t, driver, "Transistor_Array:ULN2803A", map[string][]string{
			input:  {strconv.Itoa(channel)},
			output: {strconv.Itoa(19 - channel)},
		})
	}
	requireFunctionPins(t, driver, "Transistor_Array:ULN2803A", map[string][]string{
		"GND": {"9"},
		"COM": {"10"},
	})

	adc := requireCatalogRecord(t, catalog, "adc.ti.ads1115idgsr.vssop10")
	if adc.Verification.Confidence != ConfidenceVerified || adc.ADC == nil || adc.ADC.ProofStatus != "proven" {
		t.Fatalf("ADS1115 evidence = %#v", adc.ADC)
	}
	requireSymbolFunctions(t, adc, "Analog_ADC:ADS1115IDGS", []string{"VDD", "GND", "SDA", "SCL", "ADDR", "ALERT_RDY", "AIN0", "AIN1", "AIN2", "AIN3"})
	requirePackagePads(t, adc, "vssop10", []string{"VDD", "GND", "SDA", "SCL", "ADDR", "ALERT_RDY", "AIN0", "AIN1", "AIN2", "AIN3"})
	requireFunctionsOptional(t, adc, "Analog_ADC:ADS1115IDGS", []string{"AIN0", "AIN1", "AIN2", "AIN3"})

	dac := requireCatalogRecord(t, catalog, "dac.microchip.mcp4725a0t_e_ch.sot23_6")
	if dac.Verification.Confidence != ConfidenceVerified || dac.Interface == nil || dac.Interface.ProofStatus != "proven" {
		t.Fatalf("MCP4725 evidence = %#v", dac.Interface)
	}
	requireSymbolFunctions(t, dac, "Analog_DAC:MCP4725xxx-xCH", []string{"VDD", "GND", "SDA", "SCL", "VOUT", "A0"})
	requirePackagePads(t, dac, "sot23_6", []string{"VDD", "GND", "SDA", "SCL", "VOUT", "A0"})

	bjt := requireCatalogRecord(t, catalog, "bjt.st.bd139_16.to126")
	if bjt.MPN != "BD139-16" || bjt.PowerSemiconductor == nil || bjt.PowerSemiconductor.FabricationProof {
		t.Fatalf("BD139 bounded evidence = %#v", bjt.PowerSemiconductor)
	}
	requireFunctionPins(t, bjt, "Transistor_BJT:BD139", map[string][]string{
		"EMITTER":   {"1"},
		"COLLECTOR": {"2"},
		"BASE":      {"3"},
	})
	requirePackagePads(t, bjt, "to126", []string{"EMITTER", "COLLECTOR", "BASE"})

	mux := requireCatalogRecord(t, catalog, "switch.ti.cd74hc4053e.dip16")
	if mux.MPN != "CD74HC4053E" || mux.Verification.Confidence != ConfidenceVerified {
		t.Fatalf("CD74HC4053 identity = %#v", mux)
	}
	requireSymbolFunctions(t, mux, "4xxx:4053", []string{
		"X", "X0", "X1", "Y", "Y0", "Y1", "Z", "Z0", "Z1",
		"A", "B", "C", "INHIBIT", "VCC", "VEE", "GND",
	})
	requirePackagePads(t, mux, "dip16", []string{
		"X", "X0", "X1", "Y", "Y0", "Y1", "Z", "Z0", "Z1",
		"A", "B", "C", "INHIBIT", "VCC", "VEE", "GND",
	})
}

func TestCheckedInCatalogTransistorTesterModulesFailClosed(t *testing.T) {
	catalog, err := LoadCatalog(context.Background(), LoadOptions{CatalogDir: checkedInCatalogDir(t)})
	if err != nil {
		t.Fatalf("load checked-in catalog: %v", err)
	}

	for _, id := range []string{
		"mcu.module.esp32_devkitc_compatible.38pin.provisional",
		"adc.module.ads1115.i2c.provisional",
		"dac.module.mcp4725.i2c.provisional",
		"display.module.ssd1306.i2c.128x64.provisional",
	} {
		record := requireCatalogRecord(t, catalog, id)
		if record.Verification.Confidence != ConfidencePlaceholder {
			t.Fatalf("%s confidence = %q, want placeholder", id, record.Verification.Confidence)
		}
		if !AcceptanceAllows(AcceptanceDraft, record.Verification.Confidence) {
			t.Fatalf("%s should remain available for draft schematics", id)
		}
		if AcceptanceAllows(AcceptanceStructural, record.Verification.Confidence) ||
			AcceptanceAllows(AcceptanceConnectivity, record.Verification.Confidence) {
			t.Fatalf("%s provisional module escaped draft-only acceptance", id)
		}
		if len(record.Packages) != 1 ||
			!strings.Contains(strings.ToLower(strings.Join(record.Packages[0].Verification.Notes, " ")), "not") {
			t.Fatalf("%s package lacks an explicit surrogate warning: %#v", id, record.Packages)
		}
	}

	esp32 := requireCatalogRecord(t, catalog, "mcu.module.esp32_devkitc_compatible.38pin.provisional")
	requireSymbolFunctions(t, esp32, "Connector_Generic:Conn_02x19_Odd_Even", []string{
		"3V3", "5V", "GND", "EN", "GPIO0", "TX0", "RX0", "SENSOR_VP", "SENSOR_VN",
	})
	requireFunctionsOptional(t, esp32, "Connector_Generic:Conn_02x19_Odd_Even", []string{"3V3", "5V"})
	for _, reserved := range []string{"GPIO6", "GPIO7", "GPIO8", "GPIO9", "GPIO10", "GPIO11"} {
		if !symbolHasFunction(esp32.Symbols[0], reserved) {
			t.Fatalf("ESP32 provisional header missing flash-reserved function %s", reserved)
		}
	}

	adc := requireCatalogRecord(t, catalog, "adc.module.ads1115.i2c.provisional")
	requireFunctionsOptional(t, adc, "Connector_Generic:Conn_01x10", []string{"AIN0", "AIN1", "AIN2", "AIN3"})

	dac := requireCatalogRecord(t, catalog, "dac.module.mcp4725.i2c.provisional")
	requireFunctionsOptional(t, dac, "Connector_Generic:Conn_01x06", []string{"A0"})

	oled := requireCatalogRecord(t, catalog, "display.module.ssd1306.i2c.128x64.provisional")
	if !strings.Contains(oled.Description, "GND-VCC") || !strings.Contains(oled.Description, "VCC-GND") {
		t.Fatalf("OLED provisional record lacks explicit reversed-power warning: %q", oled.Description)
	}
}

func TestCheckedInCatalogTransistorTestSocketRemainsNeutral(t *testing.T) {
	catalog, err := LoadCatalog(context.Background(), LoadOptions{CatalogDir: checkedInCatalogDir(t)})
	if err != nil {
		t.Fatalf("load checked-in catalog: %v", err)
	}

	socket := requireCatalogRecord(t, catalog, "connector.transistor_test_socket.generic.1x03")
	if socket.Verification.Confidence != ConfidenceLibraryDerived {
		t.Fatalf("socket confidence = %q, want library-derived", socket.Verification.Confidence)
	}
	if !AcceptanceAllows(AcceptanceStructural, socket.Verification.Confidence) ||
		AcceptanceAllows(AcceptanceConnectivity, socket.Verification.Confidence) {
		t.Fatal("generic socket must be structural-only")
	}
	requireSymbolFunctions(t, socket, "Connector_Generic:Conn_01x03", []string{"TERMINAL_1", "TERMINAL_2", "TERMINAL_3"})
	requirePackagePads(t, socket, "generic_1x03", []string{"TERMINAL_1", "TERMINAL_2", "TERMINAL_3"})
	for _, forbidden := range []string{"BASE", "COLLECTOR", "EMITTER"} {
		if symbolHasFunction(socket.Symbols[0], forbidden) || packageHasPadFunction(socket.Packages[0], forbidden) {
			t.Fatalf("socket assigned forbidden transistor role %s", forbidden)
		}
	}
}

func requireFunctionPins(t *testing.T, record *ComponentRecord, symbolID string, want map[string][]string) {
	t.Helper()
	for _, symbol := range record.Symbols {
		if symbol.SymbolID != symbolID {
			continue
		}
		got := map[string][]string{}
		for _, pin := range symbol.FunctionPins {
			if _, needed := want[pin.Function]; needed {
				got[pin.Function] = append(got[pin.Function], pin.SymbolPin)
			}
		}
		for function, pins := range want {
			gotPins := slices.Clone(got[function])
			wantPins := slices.Clone(pins)
			slices.Sort(gotPins)
			slices.Sort(wantPins)
			if !slices.Equal(gotPins, wantPins) {
				t.Fatalf("%s symbol %s function %s pins = %v, want %v", record.ID, symbolID, function, gotPins, wantPins)
			}
		}
		return
	}
	t.Fatalf("%s missing symbol %s", record.ID, symbolID)
}

func requireFunctionsOptional(t *testing.T, record *ComponentRecord, symbolID string, functions []string) {
	t.Helper()
	for _, symbol := range record.Symbols {
		if symbol.SymbolID != symbolID {
			continue
		}
		for _, pin := range symbol.FunctionPins {
			if slices.Contains(functions, pin.Function) && pin.Required {
				t.Fatalf("%s symbol %s function %s must remain optional", record.ID, symbolID, pin.Function)
			}
		}
		return
	}
	t.Fatalf("%s missing symbol %s", record.ID, symbolID)
}

func recordHasElectricalRole(record *ComponentRecord, role string) bool {
	for _, candidate := range record.ElectricalRoles {
		if candidate.Role == role {
			return true
		}
	}
	return false
}
