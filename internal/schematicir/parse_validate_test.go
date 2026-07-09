package schematicir

import (
	"strings"
	"testing"
)

func TestDecodeStrictRejectsUnknownFields(t *testing.T) {
	_, issues := DecodeStrict(strings.NewReader(`{
		"schema":"kicadai.schematic.ir.v1",
		"version":1,
		"unexpected":true
	}`))
	if len(issues) == 0 {
		t.Fatal("expected unknown field issue")
	}
}

func TestValidateLEDIndicator(t *testing.T) {
	doc := validLEDDocument()
	if issues := Validate(doc); len(issues) != 0 {
		t.Fatalf("expected no issues, got %#v", issues)
	}
}

func TestValidateRejectsUnknownEndpointPin(t *testing.T) {
	doc := validLEDDocument()
	doc.Circuit.Nets[0].Connect = []EndpointRef{"r_limit.3", "led.1"}
	issues := Validate(doc)
	if len(issues) == 0 {
		t.Fatal("expected endpoint validation issue")
	}
}

func TestValidateRejectsDuplicateComponentID(t *testing.T) {
	doc := validLEDDocument()
	doc.Circuit.Components[1].ID = doc.Circuit.Components[0].ID
	issues := Validate(doc)
	if len(issues) == 0 {
		t.Fatal("expected duplicate component id issue")
	}
}

func TestDecodeStrictRejectsConflictingDuplicateNetBeforeMerge(t *testing.T) {
	_, issues := DecodeStrict(strings.NewReader(`{
		"schema":"kicadai.schematic.ir.v1",
		"version":1,
		"metadata":{"name":"LED1"},
		"circuit":{
			"components":[
				{"id":"a","ref":"J1","role":"connector","symbol":"Connector_Generic:Conn_01x02","pins":[{"number":"1"},{"number":"2"}]},
				{"id":"b","ref":"J2","role":"connector","symbol":"Connector_Generic:Conn_01x02","pins":[{"number":"1"},{"number":"2"}]}
			],
			"nets":[
				{"name":"N1","role":"signal","connect":["a.1","b.1"]},
				{"name":"N1","role":"power","connect":["a.2","b.2"]}
			]
		},
		"layout":{"lanes":{},"rules":{}},
		"policy":{"repair":{"allow_ref_assignment":true}}
	}`))
	if len(issues) == 0 {
		t.Fatal("expected conflicting duplicate net issue")
	}
}

func TestDecodeStrictRejectsGeneratedNoConnectNameCollision(t *testing.T) {
	_, issues := DecodeStrict(strings.NewReader(`{
		"schema":"kicadai.schematic.ir.v1",
		"version":1,
		"metadata":{"name":"LED1"},
		"circuit":{
			"components":[
				{"id":"u1","ref":"U1","role":"ic","symbol":"Device:R","pins":[{"number":"1"},{"number":"2"}]},
				{"id":"j1","ref":"J1","role":"connector","symbol":"Connector_Generic:Conn_01x02","pins":[{"number":"1"},{"number":"2"}]}
			],
			"nets":[
				{"role":"no_connect","connect":["u1.1"]},
				{"name":"NC_u1.1","role":"signal","connect":["u1.2","j1.1"]}
			]
		},
		"layout":{"lanes":{},"rules":{}},
		"policy":{"repair":{"allow_ref_assignment":true}}
	}`))
	if len(issues) == 0 {
		t.Fatal("expected generated no-connect net name collision issue")
	}
}

func TestValidateRejectsEmptyNoConnectNet(t *testing.T) {
	doc := validLEDDocument()
	doc.Circuit.Nets = append(doc.Circuit.Nets, Net{Name: "NC_empty", Role: NetRoleNoConnect})
	issues := Validate(doc)
	if len(issues) == 0 {
		t.Fatal("expected empty no-connect issue")
	}
}

func TestValidateAggregatesSplitNetEndpoints(t *testing.T) {
	doc := validLEDDocument()
	doc.Circuit.Nets = []Net{
		{Name: "SPLIT", Role: NetRoleSignal, Connect: []EndpointRef{"vin.1"}},
		{Name: "SPLIT", Role: NetRoleSignal, Connect: []EndpointRef{"led.1"}},
	}
	if issues := Validate(doc); len(issues) != 0 {
		t.Fatalf("expected split net endpoints to validate, got %#v", issues)
	}
}

func TestValidateReportsSplitNetCardinalityOnce(t *testing.T) {
	doc := validLEDDocument()
	doc.Circuit.Nets = []Net{
		{Name: "SPLIT", Role: NetRoleSignal, Connect: []EndpointRef{"vin.1"}},
		{Name: "SPLIT", Role: NetRoleSignal},
	}
	issues := Validate(doc)
	count := 0
	for _, issue := range issues {
		if strings.Contains(issue.Message, "at least two unique endpoints") {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("cardinality issue count = %d, want 1; issues=%#v", count, issues)
	}
}

func TestValidateRejectsEndpointAssignedToMultipleNets(t *testing.T) {
	doc := validLEDDocument()
	doc.Circuit.Nets = []Net{
		{Name: "N1", Role: NetRoleSignal, Connect: []EndpointRef{"vin.1", "r_limit.1"}},
		{Name: "N2", Role: NetRoleSignal, Connect: []EndpointRef{"vin.1", "led.1"}},
	}
	issues := Validate(doc)
	if len(issues) == 0 {
		t.Fatal("expected endpoint assigned to multiple nets issue")
	}
}

func TestValidateRejectsEndpointAssignedToMultipleUnnamedNets(t *testing.T) {
	doc := validLEDDocument()
	doc.Circuit.Nets = []Net{
		{Role: NetRoleSignal, Connect: []EndpointRef{"vin.1", "r_limit.1"}},
		{Role: NetRoleSignal, Connect: []EndpointRef{"vin.1", "led.1"}},
	}
	issues := Validate(doc)
	if len(issues) == 0 {
		t.Fatal("expected endpoint assigned to multiple unnamed nets issue")
	}
}

func TestValidateRejectsEndpointForExplicitEmptyPins(t *testing.T) {
	doc := validLEDDocument()
	doc.Circuit.Components[1].Pins = []Pin{}
	issues := Validate(doc)
	if len(issues) == 0 {
		t.Fatal("expected unknown pin issue for explicit empty pins")
	}
}

func TestValidateRejectsDuplicatePinNumbers(t *testing.T) {
	doc := validLEDDocument()
	doc.Circuit.Components[1].Pins = []Pin{{Number: "1"}, {Number: "1"}}
	issues := Validate(doc)
	if len(issues) == 0 {
		t.Fatal("expected duplicate pin number issue")
	}
}

func TestValidateRejectsSharedRefFootprintConflictAfterEmptyFirst(t *testing.T) {
	doc := validLEDDocument()
	doc.Circuit.Components = []Component{
		{ID: "u1a", Ref: "U1", Unit: "A", Role: ComponentRoleIC, Symbol: "Device:R", Pins: []Pin{{Number: "1"}, {Number: "2"}}},
		{ID: "u1b", Ref: "U1", Unit: "B", Role: ComponentRoleIC, Symbol: "Device:R", Footprint: "Package_SO:SOIC-8", Pins: []Pin{{Number: "1"}, {Number: "2"}}},
		{ID: "u1c", Ref: "U1", Unit: "C", Role: ComponentRoleIC, Symbol: "Device:R", Footprint: "Package_DIP:DIP-8", Pins: []Pin{{Number: "1"}, {Number: "2"}}},
	}
	doc.Circuit.Nets = []Net{{Name: "N1", Role: NetRoleSignal, Connect: []EndpointRef{"u1a.1", "u1b.1"}}}
	issues := Validate(doc)
	if len(issues) == 0 {
		t.Fatal("expected shared-ref footprint conflict")
	}
}

func TestValidateRejectsSharedRefPartialFootprints(t *testing.T) {
	doc := validLEDDocument()
	doc.Circuit.Components = []Component{
		{ID: "u1a", Ref: "U1", Unit: "A", Role: ComponentRoleIC, Symbol: "Device:R", Footprint: "Package_SO:SOIC-8", Pins: []Pin{{Number: "1"}, {Number: "2"}}},
		{ID: "u1b", Ref: "U1", Unit: "B", Role: ComponentRoleIC, Symbol: "Device:R", Footprint: "Package_SO:SOIC-8", Pins: []Pin{{Number: "1"}, {Number: "2"}}},
		{ID: "u1c", Ref: "U1", Unit: "C", Role: ComponentRoleIC, Symbol: "Device:R", Pins: []Pin{{Number: "1"}, {Number: "2"}}},
	}
	doc.Circuit.Nets = []Net{{Name: "N1", Role: NetRoleSignal, Connect: []EndpointRef{"u1a.1", "u1b.1"}}}
	issues := Validate(doc)
	if len(issues) == 0 {
		t.Fatal("expected shared-ref partial footprint issue")
	}
}

func TestValidateRejectsLongElectricalValue(t *testing.T) {
	doc := validLEDDocument()
	doc.Circuit.Components[1].Value = strings.Repeat("1", maxValueLiteralLength+1) + "k"
	issues := Validate(doc)
	if len(issues) == 0 {
		t.Fatal("expected long value issue")
	}
}

func TestValidateRequiresValueForUnitBearingRole(t *testing.T) {
	doc := validLEDDocument()
	doc.Circuit.Components[1].Value = ""
	issues := Validate(doc)
	if len(issues) == 0 {
		t.Fatal("expected missing unit-bearing value issue")
	}
}

func TestValidateUnnamedNetsDoNotShareConnectivity(t *testing.T) {
	doc := validLEDDocument()
	doc.Circuit.Nets = []Net{
		{Role: NetRoleSignal, Connect: []EndpointRef{"vin.1"}},
		{Role: NetRoleSignal, Connect: []EndpointRef{"led.1"}},
	}
	issues := Validate(doc)
	if len(issues) < 2 {
		t.Fatalf("expected unnamed net validation issues, got %#v", issues)
	}
}

func TestValidateRejectsPortWithEmptyNet(t *testing.T) {
	doc := validLEDDocument()
	doc.Circuit.Ports = []Port{{Name: "P1", Direction: PortDirectionInput, Net: "", Side: SideLeft}}
	issues := Validate(doc)
	if len(issues) == 0 {
		t.Fatal("expected empty port net issue")
	}
}

func TestValidateRejectsDuplicatePortNames(t *testing.T) {
	doc := validLEDDocument()
	doc.Circuit.Ports = []Port{
		{Name: "P1", Direction: PortDirectionInput, Net: "VIN", Side: SideLeft},
		{Name: "P1", Direction: PortDirectionOutput, Net: "LED_A", Side: SideRight},
	}
	issues := Validate(doc)
	if len(issues) == 0 {
		t.Fatal("expected duplicate port name issue")
	}
}

func TestValidateUnnamedNetCardinalityUsesUniqueEndpoints(t *testing.T) {
	doc := validLEDDocument()
	doc.Circuit.Nets = []Net{{Role: NetRoleSignal, Connect: []EndpointRef{"vin.1", "vin.1"}}}
	issues := Validate(doc)
	if len(issues) == 0 {
		t.Fatal("expected unnamed net unique cardinality issue")
	}
}

func TestDecodeStrictNormalizesDefaults(t *testing.T) {
	doc, issues := DecodeStrict(strings.NewReader(`{
		"schema":"kicadai.schematic.ir.v1",
		"version":1,
		"metadata":{"name":"LED1"},
		"circuit":{
			"components":[
				{"id":"vin","ref":"J1","role":"input_connector","symbol":"Connector_Generic:Conn_01x02","pins":[{"number":"1"},{"number":"2"}]},
				{"id":"r_limit","ref":"R1","role":"current_limiter","symbol":"Device:R","value":"1k","pins":[{"number":"1"},{"number":"2"}]},
				{"id":"led","ref":"D1","role":"indicator_led","symbol":"Device:LED","value":"LED","pins":[{"number":"1"},{"number":"2"}]}
			],
			"nets":[
				{"name":"VIN","role":"power","connect":["vin.1","r_limit.1"]},
				{"name":"LED_A","role":"signal","connect":["r_limit.2","led.1"]},
				{"name":"GND","role":"ground","connect":["led.2","vin.2"]}
			]
		},
		"layout":{"lanes":{},"rules":{}},
		"policy":{"repair":{"allow_ref_assignment":true}}
	}`))
	if len(issues) != 0 {
		t.Fatalf("expected no issues, got %#v", issues)
	}
	if doc.Metadata.Paper != DefaultPaper {
		t.Fatalf("paper = %q, want %q", doc.Metadata.Paper, DefaultPaper)
	}
	if doc.Layout.Flow != FlowLeftToRight {
		t.Fatalf("flow = %q, want %q", doc.Layout.Flow, FlowLeftToRight)
	}
	if doc.Layout.Rules.MinGroupSpacingMM == nil || *doc.Layout.Rules.MinGroupSpacingMM != DefaultMinGroupSpacingMM {
		t.Fatalf("group spacing default not applied")
	}
}

func TestDecodeStrictPreservesAllFalseRepairPolicy(t *testing.T) {
	doc, issues := DecodeStrict(strings.NewReader(`{
		"schema":"kicadai.schematic.ir.v1",
		"version":1,
		"metadata":{"name":"LED1"},
		"circuit":{
			"components":[
				{"id":"a","ref":"J1","role":"connector","symbol":"Connector_Generic:Conn_01x02","pins":[{"number":"1"}]},
				{"id":"b","ref":"J2","role":"connector","symbol":"Connector_Generic:Conn_01x02","pins":[{"number":"1"}]}
			],
			"nets":[{"name":"N1","role":"signal","connect":["a.1","b.1"]}]
		},
		"layout":{"lanes":{},"rules":{}},
		"policy":{"repair":{}}
	}`))
	if len(issues) != 0 {
		t.Fatalf("expected no issues, got %#v", issues)
	}
	if doc.Policy.Repair.AllowRefAssignment {
		t.Fatal("allow_ref_assignment should remain false when repair policy is explicit all-false")
	}
}

func validLEDDocument() Document {
	doc := *NewDocument()
	doc.Metadata.Name = "LED1"
	doc.Circuit.Components = []Component{
		{ID: "vin", Ref: "J1", Role: ComponentRoleInputConnector, Symbol: "Connector_Generic:Conn_01x02", Pins: []Pin{{Number: "1"}, {Number: "2"}}},
		{ID: "r_limit", Ref: "R1", Role: ComponentRoleCurrentLimiter, Symbol: "Device:R", Value: "1k", Pins: []Pin{{Number: "1"}, {Number: "2"}}},
		{ID: "led", Ref: "D1", Role: ComponentRoleIndicatorLED, Symbol: "Device:LED", Value: "LED", Pins: []Pin{{Number: "1"}, {Number: "2"}}},
	}
	doc.Circuit.Nets = []Net{
		{Name: "VIN", Role: NetRolePower, Connect: []EndpointRef{"vin.1", "r_limit.1"}},
		{Name: "LED_A", Role: NetRoleSignal, Connect: []EndpointRef{"r_limit.2", "led.1"}},
		{Name: "GND", Role: NetRoleGround, Connect: []EndpointRef{"led.2", "vin.2"}},
	}
	return doc
}
