package blocks

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"kicadai/internal/kicadfiles/schematic"
	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

func TestUSBCPowerInstantiatesDefaultSink(t *testing.T) {
	registry := NewBuiltinRegistry()
	output, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    "usb_c_power",
		InstanceID: "usb",
	})
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("issues = %#v", issues)
	}
	if warningCount(issues) != 1 {
		t.Fatalf("expected no-connect warning, got %#v", issues)
	}
	if got := output.Instance.Refs; len(got) != 6 || !strings.HasPrefix(got[0], "J") || !strings.HasPrefix(got[1], "R") {
		t.Fatalf("refs = %#v", got)
	}
	if got := output.Instance.Nets; len(got) < 6 || got[0] != "usb_vbus_out" {
		t.Fatalf("nets = %#v", got)
	}
	validation := transactions.Validate(transactions.Transaction{Operations: output.Operations})
	if len(validation.Issues) != 0 {
		t.Fatalf("transaction validation issues = %#v", validation.Issues)
	}
}

func TestUSBCPowerSymbolPinsFollowPowerOnlyRoleMap(t *testing.T) {
	pins := usbCSymbolPins(usbCPowerPins)
	got := map[string]bool{}
	for _, pin := range pins {
		got[pin.Number] = true
	}
	for _, want := range []string{"A5", "A9", "A12", "B5", "B9", "B12", "SH"} {
		if !got[want] {
			t.Fatalf("missing power-only USB-C pin %s in %#v", want, pins)
		}
	}
	for _, forbidden := range []string{"A1", "A4", "A6", "A7", "A8", "B1", "B4", "B6", "B7", "B8"} {
		if got[forbidden] {
			t.Fatalf("unexpected 16-pin USB-C pin %s in %#v", forbidden, pins)
		}
	}
}

func TestUSBCPowerSymbolPinsMatchKiCadERCConnectionGeometry(t *testing.T) {
	pins := usbCSymbolPins(usbCPowerPins)
	got := map[string]transactions.Point{}
	for _, pin := range pins {
		got[pin.Number] = transactions.Point{XMM: pin.XMM, YMM: pin.YMM}
	}
	want := map[string]transactions.Point{
		"A5":  {XMM: 15.24, YMM: 5.08},
		"A9":  {XMM: 15.24, YMM: -7.62},
		"A12": {XMM: 0, YMM: 17.78},
		"B5":  {XMM: 15.24, YMM: 7.62},
		"B9":  {XMM: 15.24, YMM: -7.62},
		"B12": {XMM: 0, YMM: 17.78},
		"SH":  {XMM: -7.62, YMM: 17.78},
	}
	for pin, position := range want {
		if got[pin] != position {
			t.Fatalf("pin %s = %#v, want %#v", pin, got[pin], position)
		}
	}
}

func TestUSBCPowerCCPullDownsArePresent(t *testing.T) {
	registry := NewBuiltinRegistry()
	output, issues := registry.Instantiate(context.Background(), BlockRequest{BlockID: "usb_c_power", InstanceID: "usb"})
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("issues = %#v", issues)
	}
	data, err := json.Marshal(output.Operations)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{`"value":"5.1k"`, `"net_name":"usb_cc1"`, `"net_name":"usb_cc2"`} {
		if !strings.Contains(text, want) {
			t.Fatalf("operations missing %q: %s", want, text)
		}
	}
}

func TestUSBCPowerCC2PullDownUsesTopLayerEscape(t *testing.T) {
	realization := usbCPowerPCBRealization()
	routes := map[string]PCBLocalRoute{}
	for _, candidate := range realization.LocalRoutes {
		routes[candidate.ID] = candidate
	}
	route, ok := routes["cc2_pull_down"]
	if !ok {
		t.Fatalf("cc2_pull_down route missing")
	}
	if route.Layer != "F.Cu" {
		t.Fatalf("cc2_pull_down layer = %q, want F.Cu to avoid a B.Cu via-in-pad copper sliver", route.Layer)
	}
	want := []RelativePoint{{XMM: 0.5, YMM: -0.6}, {XMM: 5.0, YMM: -0.6}}
	if len(route.Waypoints) != len(want) {
		t.Fatalf("cc2_pull_down waypoint count = %d, want %d", len(route.Waypoints), len(want))
	}
	for i := range want {
		if !relativePointNear(route.Waypoints[i], want[i]) {
			t.Fatalf("cc2_pull_down waypoint %d = %#v, want %#v", i, route.Waypoints[i], want[i])
		}
	}
}

func TestUSBCPowerProtectedTVSUsesShortWideBulkGroundReturn(t *testing.T) {
	realization := usbCPowerPCBRealization()
	routes := map[string]PCBLocalRoute{}
	for _, candidate := range realization.LocalRoutes {
		routes[candidate.ID] = candidate
	}
	route := routes["tvs_ground"]
	if route.ID == "" {
		t.Fatalf("tvs_ground route missing")
	}
	if route.To.ComponentRole != "bulk_capacitor" || route.To.Pin != "2" {
		t.Fatalf("tvs_ground route endpoint = %#v, want bulk_capacitor pin 2", route.To)
	}
	if len(route.Waypoints) != 0 {
		t.Fatalf("tvs_ground waypoints = %#v, want direct short return", route.Waypoints)
	}
	if route.Layer != "F.Cu" {
		t.Fatalf("tvs_ground layer = %q, want F.Cu short return", route.Layer)
	}
	if route.WidthMM < 0.8 {
		t.Fatalf("tvs_ground width = %g, want >= 0.8mm", route.WidthMM)
	}
	if route.When.Params["include_tvs"] != true || route.When.Params["include_bulk_capacitor"] != true {
		t.Fatalf("tvs_ground conditions = %#v, want protected TVS+bulk path", route.When.Params)
	}
	bulk := routes["bulk_ground"]
	if bulk.ID == "" || bulk.WidthMM < 0.8 {
		t.Fatalf("bulk_ground route = %#v, want >= 0.8mm continuation", bulk)
	}
	if bulk.Layer != "F.Cu" {
		t.Fatalf("bulk_ground layer = %q, want F.Cu return", bulk.Layer)
	}
	if bulk.To.ComponentRole != "cc2_rd" || bulk.To.Pin != "2" {
		t.Fatalf("bulk endpoint = %#v, want local CC2 ground hub", bulk.To)
	}
	if want := []RelativePoint{{XMM: 18, YMM: -5}, {XMM: 7.1, YMM: -5}}; !reflect.DeepEqual(bulk.Waypoints, want) {
		t.Fatalf("bulk_ground waypoints = %#v, want %#v", bulk.Waypoints, want)
	}
	fallback := routes["tvs_ground_fallback"]
	if fallback.ID == "" {
		t.Fatalf("tvs_ground_fallback route missing")
	}
	if fallback.To.ComponentRole != "cc2_rd" || fallback.To.Pin != "2" {
		t.Fatalf("fallback endpoint = %#v, want local CC2 ground hub", fallback.To)
	}
	if want := []RelativePoint{{XMM: 20, YMM: 4}, {XMM: 20, YMM: -5}, {XMM: 7.1, YMM: -5}}; !reflect.DeepEqual(fallback.Waypoints, want) {
		t.Fatalf("tvs_ground_fallback waypoints = %#v, want %#v", fallback.Waypoints, want)
	}
	if fallback.WidthMM < 0.8 {
		t.Fatalf("fallback width = %g, want >= 0.8mm", fallback.WidthMM)
	}
	if fallback.Layer != "F.Cu" {
		t.Fatalf("fallback layer = %q, want F.Cu return", fallback.Layer)
	}
	if fallback.When.Params["include_tvs"] != true || fallback.When.Params["include_bulk_capacitor"] != false {
		t.Fatalf("fallback conditions = %#v, want TVS-only path", fallback.When.Params)
	}
}

func TestUSBCPowerProtectedPowerPathWidths(t *testing.T) {
	realization := usbCPowerPCBRealization()
	routes := map[string]PCBLocalRoute{}
	for _, candidate := range realization.LocalRoutes {
		routes[candidate.ID] = candidate
	}

	for _, routeID := range []string{"vbus_entry_a", "vbus_entry_b"} {
		route := routes[routeID]
		if route.ID == "" {
			t.Errorf("%s route missing", routeID)
			continue
		}
		if route.WidthMM != 0.75 {
			t.Errorf("%s width = %g, want 0.75mm connector-entry width", routeID, route.WidthMM)
		}
	}
	for _, routeID := range []string{"vbus_tvs", "vbus_bulk", "tvs_ground", "tvs_ground_fallback", "bulk_ground"} {
		route := routes[routeID]
		if route.ID == "" {
			t.Errorf("%s route missing", routeID)
			continue
		}
		if route.WidthMM != 0.8 {
			t.Errorf("%s width = %g, want 0.8mm", routeID, route.WidthMM)
		}
	}
}

func TestUSBCPowerTVSOnlyRealizationKeepsGroundRoute(t *testing.T) {
	registry := NewBuiltinRegistry()
	definition, ok := registry.GetBlock("usb_c_power")
	if !ok {
		t.Fatal("missing usb_c_power")
	}
	output, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    "usb_c_power",
		InstanceID: "usb",
		Params: map[string]any{
			"include_bulk_capacitor": false,
			"include_tvs":            true,
		},
	})
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("instantiate issues = %#v", issues)
	}
	rolesByRef := addSymbolRolesByRef(t, output.Operations)
	realized := RealizeBlockPCB(definition, output, PCBRealizationOptions{})
	if reports.HasBlockingIssue(realized.Issues) {
		t.Fatalf("realize issues = %#v", realized.Issues)
	}
	routeIDs := map[string]bool{}
	for _, route := range realized.LocalRoutes {
		routeIDs[route.ID] = true
		if route.ID == "tvs_ground_fallback" && (rolesByRef[route.To.Ref] != "cc2_rd" || route.To.Pin != "2") {
			t.Fatalf("fallback route = %#v, want local CC2 ground endpoint", route)
		}
	}
	if !routeIDs["tvs_ground_fallback"] {
		t.Fatalf("routes = %#v, want tvs_ground_fallback", realized.LocalRoutes)
	}
	if routeIDs["tvs_ground"] {
		t.Fatalf("routes = %#v, want bulk TVS route omitted without bulk capacitor", realized.LocalRoutes)
	}
	if routeIDs["bulk_ground"] {
		t.Fatalf("routes = %#v, want bulk ground route omitted without bulk capacitor", realized.LocalRoutes)
	}
}

func TestUSBCPowerDefaultRealizationUsesBulkTVSGroundRoute(t *testing.T) {
	registry := NewBuiltinRegistry()
	definition, ok := registry.GetBlock("usb_c_power")
	if !ok {
		t.Fatal("missing usb_c_power")
	}
	output, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    "usb_c_power",
		InstanceID: "usb",
	})
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("instantiate issues = %#v", issues)
	}
	realized := RealizeBlockPCB(definition, output, PCBRealizationOptions{})
	if reports.HasBlockingIssue(realized.Issues) {
		t.Fatalf("realize issues = %#v", realized.Issues)
	}
	routeIDs := map[string]bool{}
	for _, route := range realized.LocalRoutes {
		routeIDs[route.ID] = true
	}
	if !routeIDs["tvs_ground"] || !routeIDs["bulk_ground"] {
		t.Fatalf("default routes = %#v, want tvs_ground and bulk_ground active", realized.LocalRoutes)
	}
	if routeIDs["tvs_ground_fallback"] {
		t.Fatalf("default routes = %#v, want fallback omitted when bulk capacitor defaults active", realized.LocalRoutes)
	}
}

func TestUSBCPowerMinimalRealizationOwnsCCGroundReturns(t *testing.T) {
	registry := NewBuiltinRegistry()
	definition, ok := registry.GetBlock("usb_c_power")
	if !ok {
		t.Fatal("missing usb_c_power")
	}
	realize := func(includePowerLED bool) map[string]RealizedPCBLocalRoute {
		output, issues := registry.Instantiate(context.Background(), BlockRequest{
			BlockID: "usb_c_power", InstanceID: "usb",
			Params: map[string]any{
				"include_fuse":           false,
				"include_tvs":            false,
				"include_bulk_capacitor": false,
				"include_power_led":      includePowerLED,
			},
		})
		if reports.HasBlockingIssue(issues) {
			t.Fatalf("instantiate issues = %#v", issues)
		}
		result := RealizeBlockPCB(definition, output, PCBRealizationOptions{})
		if reports.HasBlockingIssue(result.Issues) {
			t.Fatalf("realize issues = %#v", result.Issues)
		}
		routes := map[string]RealizedPCBLocalRoute{}
		for _, route := range result.LocalRoutes {
			routes[route.ID] = route
		}
		return routes
	}

	minimal := realize(false)
	for _, routeID := range []string{"minimal_cc_ground_pair", "minimal_cc_ground_return"} {
		if _, exists := minimal[routeID]; !exists {
			t.Fatalf("minimal routes = %#v, missing %s", minimal, routeID)
		}
	}
	cc2 := minimal["cc2_pull_down"]
	if cc2.Layer != "B.Cu" || !cc2.FromEndpointDogbone || !cc2.ToEndpointDogbone || len(cc2.Points) != 4 {
		t.Fatalf("minimal CC2 route = %#v, want B.Cu route with endpoint dogbones", cc2)
	}
	withLED := realize(true)
	for _, routeID := range []string{"minimal_cc_ground_pair", "minimal_cc_ground_return"} {
		if _, exists := withLED[routeID]; exists {
			t.Fatalf("LED routes = %#v, want %s omitted", withLED, routeID)
		}
	}
}

func TestUSBCPowerPowerLEDIsForwardBiased(t *testing.T) {
	registry := NewBuiltinRegistry()
	output, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    "usb_c_power",
		InstanceID: "usb",
		Params: map[string]any{
			"include_power_led": true,
		},
	})
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("issues = %#v", issues)
	}
	rolesByRef := addSymbolRolesByRef(t, output.Operations)
	var resistorRef, ledRef string
	for ref, role := range rolesByRef {
		switch role {
		case "power_led_resistor":
			resistorRef = ref
		case "power_led":
			ledRef = ref
		}
	}
	if resistorRef == "" || ledRef == "" {
		t.Fatalf("roles by ref = %#v", rolesByRef)
	}
	if !hasPinOnNet(output.Operations, resistorRef, "1", "usb_vbus_out") {
		t.Fatalf("expected VBUS_OUT net to feed power LED resistor pin 1")
	}
	if !hasConnect(output.Operations, resistorRef, "2", ledRef, "2", "usb_power_led_series") {
		t.Fatalf("expected power LED resistor pin 2 to feed LED anode pin 2")
	}
	if !hasPinOnNet(output.Operations, ledRef, "1", "usb_gnd") {
		t.Fatalf("expected power LED cathode pin 1 to return to GND")
	}
}

func TestUSBCPowerFuseCanBeDisabled(t *testing.T) {
	registry := NewBuiltinRegistry()
	output, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    "usb_c_power",
		InstanceID: "usb",
		Params: map[string]any{
			"include_fuse": false,
		},
	})
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("issues = %#v", issues)
	}
	for _, ref := range output.Instance.Refs {
		if strings.HasPrefix(ref, "F") {
			t.Fatalf("fuse ref should be absent: %#v", output.Instance.Refs)
		}
	}
}

func TestUSBCPowerFloatingShieldIsNotExported(t *testing.T) {
	registry := NewBuiltinRegistry()
	output, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    "usb_c_power",
		InstanceID: "usb",
		Params: map[string]any{
			"shield_policy": "floating",
		},
	})
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("issues = %#v", issues)
	}
	if warningCount(issues) != 1 {
		t.Fatalf("expected only data-mode no-connect warning, got %#v", issues)
	}
	data, err := json.Marshal(output.Operations)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, `"op":"add_no_connect"`) || !strings.Contains(text, `"pin":"SH"`) {
		t.Fatalf("floating shield should emit SH no-connect: %s", text)
	}
	if strings.Contains(text, `"SHIELD"`) {
		t.Fatalf("floating shield should not export SHIELD net: %s", text)
	}
}

func TestUSBCPowerExportsVBUSOut(t *testing.T) {
	registry := NewBuiltinRegistry()
	output, issues := registry.Instantiate(context.Background(), BlockRequest{BlockID: "usb_c_power", InstanceID: "usb"})
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("issues = %#v", issues)
	}
	found := false
	for _, port := range output.Instance.Ports {
		found = found || port.Name == "VBUS_OUT"
	}
	if !found {
		t.Fatalf("ports = %#v", output.Instance.Ports)
	}
}

func TestUSBCPowerRejectsUnsupportedDataMode(t *testing.T) {
	registry := NewBuiltinRegistry()
	_, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    "usb_c_power",
		InstanceID: "usb",
		Params: map[string]any{
			"data_mode": "usb2",
		},
	})
	if len(issues) != 1 || !issues[0].Blocking() || issues[0].Path != "params.data_mode" {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestUSBCPowerProjectTransactionApplies(t *testing.T) {
	registry := NewBuiltinRegistry()
	output, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    "usb_c_power",
		InstanceID: "usb",
	})
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("issues = %#v", issues)
	}
	tx, err := ProjectTransactionForBlockOutput("usb_power", output, false)
	if err != nil {
		t.Fatal(err)
	}
	outputDir := filepath.Join(t.TempDir(), "usb_power")
	result := transactions.Apply(tx, transactions.ApplyOptions{OutputDir: outputDir})
	if len(result.Issues) != 0 {
		t.Fatalf("apply issues = %#v", result.Issues)
	}
	for _, name := range []string{"usb_power.kicad_pro", "usb_power.kicad_sch", "usb_power.kicad_pcb"} {
		if _, err := os.Stat(filepath.Join(outputDir, name)); err != nil {
			t.Fatalf("expected %s: %v", name, err)
		}
	}
	file, err := schematic.ReadFile(filepath.Join(outputDir, "usb_power.kicad_sch"))
	if err != nil {
		t.Fatal(err)
	}
	var fusePin1Found bool
	for _, symbol := range file.Symbols {
		if !strings.HasPrefix(symbol.Reference, "F") {
			continue
		}
		pinIndex := -1
		for index, pin := range symbol.Pins {
			if pin.Number == "1" {
				pinIndex = index
				break
			}
		}
		if pinIndex < 0 || pinIndex >= len(symbol.PinAnchors) {
			continue
		}
		fusePin1Found = true
	}
	vbusLabels := 0
	for _, label := range file.Labels {
		if label.Text == "usb_vbus_connector" {
			vbusLabels++
		}
	}
	if !fusePin1Found || vbusLabels < 2 {
		t.Fatalf("stacked VBUS materialization must label both sides of the connection: found=%t labels=%#v", fusePin1Found, file.Labels)
	}
}

func warningCount(issues []reports.Issue) int {
	count := 0
	for _, issue := range issues {
		if issue.Severity == reports.SeverityWarning {
			count++
		}
	}
	return count
}

func relativePointNear(got RelativePoint, want RelativePoint) bool {
	const toleranceMM = 1e-9
	return math.Abs(got.XMM-want.XMM) <= toleranceMM && math.Abs(got.YMM-want.YMM) <= toleranceMM
}
