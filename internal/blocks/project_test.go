package blocks

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

func TestProjectTransactionForLEDMaterializesExportedPorts(t *testing.T) {
	registry := NewBuiltinRegistry()
	output, issues := registry.Instantiate(context.Background(), BlockRequest{BlockID: "led_indicator", InstanceID: "status"})
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("instantiate issues: %#v", issues)
	}

	tx, err := ProjectTransactionForBlockOutput("status_led", output, false)
	if err != nil {
		t.Fatal(err)
	}

	if tx.Name != "status_led" || tx.Project != "status_led" {
		t.Fatalf("unexpected transaction identity: %#v", tx)
	}
	if len(tx.Operations) != 11 {
		t.Fatalf("expected create + 6 component ops + 2 exported-port labels + 1 internal connect + write, got %d", len(tx.Operations))
	}
	if tx.Operations[0].Op != transactions.OpCreateProject || tx.Operations[len(tx.Operations)-1].Op != transactions.OpWriteProject {
		t.Fatalf("transaction must be bracketed by create/write: %#v", tx.Operations)
	}
	labels := 0
	connects := 0
	for _, operation := range tx.Operations {
		if operation.Op == transactions.OpAddLabel {
			var payload transactions.AddLabelOperation
			if err := decodeBlockOperation(operation, &payload); err != nil {
				t.Fatal(err)
			}
			labels++
			if payload.Text != "IN" && payload.Text != "GND" {
				t.Fatalf("unexpected generated label: %#v", payload)
			}
		}
		if operation.Op != transactions.OpConnect {
			continue
		}
		connects++
		var payload transactions.ConnectOperation
		if err := decodeBlockOperation(operation, &payload); err != nil {
			t.Fatal(err)
		}
		if payload.From.Ref == output.Instance.InstanceID || payload.To.Ref == output.Instance.InstanceID {
			t.Fatalf("project transaction leaked pseudo-port connect: %#v", payload)
		}
	}
	if labels != 2 {
		t.Fatalf("expected two generated labels, got %d", labels)
	}
	if connects != 1 {
		t.Fatalf("expected one internal connect, got %d", connects)
	}
	if validation := transactions.Validate(tx); len(validation.Issues) != 0 {
		t.Fatalf("unexpected validation issues: %#v", validation.Issues)
	}
}

func TestProjectTransactionForConnectorFiltersPseudoPortConnections(t *testing.T) {
	registry := NewBuiltinRegistry()
	output, issues := registry.Instantiate(context.Background(), BlockRequest{
		BlockID:    "connector_breakout",
		InstanceID: "header",
		Params:     map[string]any{"pin_names": []string{"VCC", "GND", "SCL", "SDA"}},
	})
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("instantiate issues: %#v", issues)
	}

	tx, err := ProjectTransactionForBlockOutput("header", output, false)
	if err != nil {
		t.Fatal(err)
	}

	for _, operation := range tx.Operations {
		if operation.Op == transactions.OpConnect {
			t.Fatalf("connector project transaction should not include pseudo-port connects: %#v", operation)
		}
	}
	if validation := transactions.Validate(tx); len(validation.Issues) != 0 {
		t.Fatalf("unexpected validation issues: %#v", validation.Issues)
	}
}

func TestProjectTransactionForI2CBreakoutMaterializesInternalNets(t *testing.T) {
	output := ComposeBlocks(context.Background(), NewBuiltinRegistry(), CompositionRequest{
		ProjectName: "i2c_sensor_breakout",
		Instances: []CompositionInstance{
			{ID: "sensor", BlockID: "i2c_sensor", Params: map[string]any{"i2c_address": "0x48"}},
			{ID: "i2c_connector", BlockID: "connector_breakout", Params: map[string]any{"pin_names": []string{"VCC", "GND", "SDA", "SCL"}}},
		},
		Connections: []CompositionConnection{
			{From: PortRef{InstanceID: "i2c_connector", Port: "VCC"}, To: PortRef{InstanceID: "sensor", Port: "VCC"}, NetAlias: "VCC"},
			{From: PortRef{InstanceID: "i2c_connector", Port: "GND"}, To: PortRef{InstanceID: "sensor", Port: "GND"}, NetAlias: "GND"},
			{From: PortRef{InstanceID: "i2c_connector", Port: "SDA"}, To: PortRef{InstanceID: "sensor", Port: "SDA"}, NetAlias: "SDA"},
			{From: PortRef{InstanceID: "i2c_connector", Port: "SCL"}, To: PortRef{InstanceID: "sensor", Port: "SCL"}, NetAlias: "SCL"},
		},
	})
	if reports.HasBlockingIssue(output.Issues) {
		t.Fatalf("compose issues: %#v", output.Issues)
	}
	tx, err := ProjectTransactionForCompositionOutput("i2c_sensor_breakout", output, false)
	if err != nil {
		t.Fatal(err)
	}
	connectsByNet := map[string]int{}
	for _, operation := range tx.Operations {
		if operation.Op == transactions.OpConnect {
			connectsByNet[operation.Net]++
		}
	}
	for _, net := range []string{"VCC", "GND", "SDA", "SCL"} {
		if connectsByNet[net] == 0 {
			t.Fatalf("connects by net = %#v, want materialized connect for %s", connectsByNet, net)
		}
	}
}

func TestProjectTransactionLabelsExportedMultiEndpointNet(t *testing.T) {
	resistor := BlockComponent{
		Role:      "series_resistor",
		RefPrefix: "R",
		Value:     "1k",
		SymbolID:  "Device:R",
		Pins: []transactions.PinSpec{
			{Number: "1", XMM: -3.81, YMM: 0},
			{Number: "2", XMM: 3.81, YMM: 0},
		},
	}
	led := BlockComponent{
		Role:      "led",
		RefPrefix: "D",
		Value:     "LED",
		SymbolID:  "Device:LED",
		Pins: []transactions.PinSpec{
			{Number: "1", XMM: -3.81, YMM: 0},
			{Number: "2", XMM: 3.81, YMM: 0},
		},
	}
	var operations []transactions.Operation
	for _, item := range []struct {
		component BlockComponent
		ref       string
		at        transactions.Point
	}{
		{component: resistor, ref: "R1", at: transactions.Point{XMM: 10, YMM: 20}},
		{component: led, ref: "D1", at: transactions.Point{XMM: 25, YMM: 20}},
	} {
		componentOps, issues := ComponentOperations(item.component, item.ref, item.at)
		if len(issues) != 0 {
			t.Fatalf("component issues: %#v", issues)
		}
		operations = append(operations, componentOps...)
	}
	for _, pair := range []struct {
		from transactions.Endpoint
		to   transactions.Endpoint
	}{
		{from: transactions.Endpoint{Ref: "status", Pin: "IN"}, to: transactions.Endpoint{Ref: "R1", Pin: "1"}},
		{from: transactions.Endpoint{Ref: "R1", Pin: "1"}, to: transactions.Endpoint{Ref: "D1", Pin: "1"}},
	} {
		connect, issues := ConnectOperation(pair.from.Ref, pair.from.Pin, pair.to.Ref, pair.to.Pin, "STATUS_IN")
		if len(issues) != 0 {
			t.Fatalf("connect issues: %#v", issues)
		}
		operations = append(operations, connect)
	}

	tx, err := ProjectTransactionForBlockOutput("status", BlockOutput{
		Instance:   BlockInstance{InstanceID: "status", Refs: []string{"R1", "D1"}},
		Operations: operations,
	}, false)
	if err != nil {
		t.Fatal(err)
	}

	labels := 0
	connects := 0
	for _, operation := range tx.Operations {
		switch operation.Op {
		case transactions.OpAddLabel:
			var payload transactions.AddLabelOperation
			if err := decodeBlockOperation(operation, &payload); err != nil {
				t.Fatal(err)
			}
			labels++
			if payload.Text != "IN" || math.Abs(payload.At.XMM-6.35) > 0.000001 || math.Abs(payload.At.YMM-20.32) > 0.000001 {
				t.Fatalf("unexpected exported label: %#v", payload)
			}
		case transactions.OpConnect:
			var payload transactions.ConnectOperation
			if err := decodeBlockOperation(operation, &payload); err != nil {
				t.Fatal(err)
			}
			connects++
			if payload.From.Ref == "status" || payload.To.Ref == "status" {
				t.Fatalf("pseudo-port connect leaked: %#v", payload)
			}
		}
	}
	if connects != 1 {
		t.Fatalf("connects = %d, want 1", connects)
	}
	if labels != 1 {
		t.Fatalf("labels = %d, want 1", labels)
	}
}

func TestProjectTransactionLabelsOneOfMultiplePseudoPorts(t *testing.T) {
	componentOps, issues := ComponentOperations(BlockComponent{
		Role:      "test_node",
		RefPrefix: "J",
		Value:     "Test",
		SymbolID:  "Connector:Conn_01x01_Pin",
		Pins:      []transactions.PinSpec{{Number: "1", XMM: 0, YMM: 0}},
	}, "J1", transactions.Point{XMM: 10, YMM: 20})
	if len(issues) != 0 {
		t.Fatalf("component issues: %#v", issues)
	}
	operations := append([]transactions.Operation{}, componentOps...)
	for _, pair := range []struct {
		from transactions.Endpoint
		to   transactions.Endpoint
	}{
		{from: transactions.Endpoint{Ref: "status", Pin: "IN"}, to: transactions.Endpoint{Ref: "J1", Pin: "1"}},
		{from: transactions.Endpoint{Ref: "status", Pin: "OUT"}, to: transactions.Endpoint{Ref: "J1", Pin: "1"}},
	} {
		connect, issues := ConnectOperation(pair.from.Ref, pair.from.Pin, pair.to.Ref, pair.to.Pin, "STATUS")
		if len(issues) != 0 {
			t.Fatalf("connect issues: %#v", issues)
		}
		operations = append(operations, connect)
	}

	tx, err := ProjectTransactionForBlockOutput("status", BlockOutput{
		Instance:   BlockInstance{InstanceID: "status", Refs: []string{"J1"}},
		Operations: operations,
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	labels := 0
	for _, operation := range tx.Operations {
		if operation.Op != transactions.OpAddLabel {
			continue
		}
		var payload transactions.AddLabelOperation
		if err := decodeBlockOperation(operation, &payload); err != nil {
			t.Fatal(err)
		}
		labels++
		if payload.Text != "STATUS" {
			t.Fatalf("label = %#v, want stable first pseudo-port label", payload)
		}
	}
	if labels != 1 {
		t.Fatalf("labels = %d, want 1", labels)
	}
}

func TestProjectTransactionPreservesForcedLabelsAcrossPseudoPortGroup(t *testing.T) {
	var operations []transactions.Operation
	for index, ref := range []string{"U1", "C1", "R1"} {
		componentOps, issues := ComponentOperations(BlockComponent{
			Role:      "test_node",
			RefPrefix: string(ref[0]),
			Value:     "Test",
			SymbolID:  "Connector:Conn_01x01_Pin",
			Pins:      []transactions.PinSpec{{Number: "1"}},
		}, ref, transactions.Point{XMM: float64(index * 10), YMM: 20})
		if len(issues) != 0 {
			t.Fatalf("component issues: %#v", issues)
		}
		operations = append(operations, componentOps...)
	}
	useLabels := true
	for _, pair := range []struct{ from, to transactions.Endpoint }{
		{from: transactions.Endpoint{Ref: "sensor", Pin: "VCC"}, to: transactions.Endpoint{Ref: "U1", Pin: "1"}},
		{from: transactions.Endpoint{Ref: "U1", Pin: "1"}, to: transactions.Endpoint{Ref: "C1", Pin: "1"}},
		{from: transactions.Endpoint{Ref: "R1", Pin: "1"}, to: transactions.Endpoint{Ref: "sensor", Pin: "VCC"}},
	} {
		operation, err := wrapOperation(transactions.OpConnect, transactions.ConnectOperation{
			Op:        transactions.OpConnect,
			From:      pair.from,
			To:        pair.to,
			NetName:   "VCC",
			UseLabels: &useLabels,
		})
		if err != nil {
			t.Fatal(err)
		}
		operations = append(operations, operation)
	}

	tx, err := ProjectTransactionForBlockOutput("sensor", BlockOutput{
		Instance:   BlockInstance{InstanceID: "sensor", Refs: []string{"U1", "C1", "R1"}},
		Operations: operations,
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	connects := 0
	for _, operation := range tx.Operations {
		if operation.Op != transactions.OpConnect {
			continue
		}
		var payload transactions.ConnectOperation
		if err := decodeBlockOperation(operation, &payload); err != nil {
			t.Fatal(err)
		}
		connects++
		if payload.NetName != "VCC" {
			t.Fatalf("materialized connect lost pseudo-port net identity: %#v", payload)
		}
		if payload.UseLabels == nil || !*payload.UseLabels {
			t.Fatalf("materialized connect lost forced-label policy: %#v", payload)
		}
		if connects == 1 {
			if payload.From != payload.To || payload.SkipFromLabel || !payload.SkipToLabel {
				t.Fatalf("first connect must explicitly materialize one block-port label: %#v", payload)
			}
		} else if !payload.SkipFromLabel || payload.SkipToLabel {
			t.Fatalf("later materialized connect must preserve only its new endpoint label: %#v", payload)
		}
	}
	if connects != 3 {
		t.Fatalf("connects = %d, want one port materialization and two endpoint joins", connects)
	}
}

func TestProjectTransactionFallsBackToCapacitorLabelWhenNoOtherAnchorExists(t *testing.T) {
	componentOps, issues := ComponentOperations(BlockComponent{
		Role:      "dc_blocking_capacitor",
		RefPrefix: "C",
		Value:     "220uF",
		SymbolID:  "Device:C",
		Pins:      twoTerminalHorizontalPins(),
	}, "C1", transactions.Point{XMM: 10, YMM: 20})
	if len(issues) != 0 {
		t.Fatalf("component issues: %#v", issues)
	}
	operations := append([]transactions.Operation{}, componentOps...)
	for _, pair := range []struct {
		from transactions.Endpoint
		to   transactions.Endpoint
		net  string
	}{
		{from: transactions.Endpoint{Ref: "protect", Pin: "AMP_OUT"}, to: transactions.Endpoint{Ref: "C1", Pin: "1"}, net: "AMP_OUT_DC_BIASED"},
		{from: transactions.Endpoint{Ref: "source", Pin: "AMP_OUT"}, to: transactions.Endpoint{Ref: "C1", Pin: "1"}, net: "AMP_OUT_DC_BIASED"},
		{from: transactions.Endpoint{Ref: "C1", Pin: "2"}, to: transactions.Endpoint{Ref: "protect", Pin: "HP_OUT"}, net: "HP_OUT"},
		{from: transactions.Endpoint{Ref: "C1", Pin: "2"}, to: transactions.Endpoint{Ref: "sink", Pin: "HP_OUT"}, net: "HP_OUT"},
	} {
		connect, issues := ConnectOperation(pair.from.Ref, pair.from.Pin, pair.to.Ref, pair.to.Pin, pair.net)
		if len(issues) != 0 {
			t.Fatalf("connect issues: %#v", issues)
		}
		operations = append(operations, connect)
	}
	tx, err := projectTransaction("protect", operations, map[string]struct{}{"C1": {}}, map[string]struct{}{"protect": {}, "source": {}, "sink": {}}, false)
	if err != nil {
		t.Fatal(err)
	}
	labels := map[string]bool{}
	for _, operation := range tx.Operations {
		if operation.Op == transactions.OpAddLabel {
			var payload transactions.AddLabelOperation
			if err := decodeBlockOperation(operation, &payload); err != nil {
				t.Fatal(err)
			}
			labels[payload.Text] = true
		}
	}
	for _, label := range []string{"AMP_OUT_DC_BIASED", "HP_OUT"} {
		if !labels[label] {
			t.Fatalf("missing fallback label %s in %#v", label, labels)
		}
	}
}

func TestProjectTransactionPrefersNonCapacitorMaterializedLabelAnchor(t *testing.T) {
	capOps, issues := ComponentOperations(BlockComponent{
		Role:      "dc_blocking_capacitor",
		RefPrefix: "C",
		Value:     "220uF",
		SymbolID:  "Device:C",
		Pins:      twoTerminalHorizontalPins(),
	}, "C1", transactions.Point{XMM: 10, YMM: 20})
	if len(issues) != 0 {
		t.Fatalf("capacitor issues: %#v", issues)
	}
	resistorOps, issues := ComponentOperations(BlockComponent{
		Role:      "load_resistor",
		RefPrefix: "R",
		Value:     "33",
		SymbolID:  "Device:R",
		Pins:      twoTerminalHorizontalPins(),
	}, "R1", transactions.Point{XMM: 30, YMM: 20})
	if len(issues) != 0 {
		t.Fatalf("resistor issues: %#v", issues)
	}
	operations := append([]transactions.Operation{}, capOps...)
	operations = append(operations, resistorOps...)
	for _, pair := range []struct {
		from transactions.Endpoint
		to   transactions.Endpoint
		net  string
	}{
		{from: transactions.Endpoint{Ref: "protect", Pin: "AMP_OUT"}, to: transactions.Endpoint{Ref: "C1", Pin: "1"}, net: "AMP_OUT_DC_BIASED"},
		{from: transactions.Endpoint{Ref: "source", Pin: "AMP_OUT"}, to: transactions.Endpoint{Ref: "R1", Pin: "1"}, net: "AMP_OUT_DC_BIASED"},
		{from: transactions.Endpoint{Ref: "C1", Pin: "1"}, to: transactions.Endpoint{Ref: "R1", Pin: "1"}, net: "AMP_OUT_DC_BIASED"},
	} {
		connect, issues := ConnectOperation(pair.from.Ref, pair.from.Pin, pair.to.Ref, pair.to.Pin, pair.net)
		if len(issues) != 0 {
			t.Fatalf("connect issues: %#v", issues)
		}
		operations = append(operations, connect)
	}
	tx, err := projectTransaction("protect", operations, map[string]struct{}{"C1": {}, "R1": {}}, map[string]struct{}{"protect": {}, "source": {}}, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, operation := range tx.Operations {
		if operation.Op == transactions.OpAddLabel {
			t.Fatalf("multi-endpoint protection net should not emit redundant materialized label: %#v", operation)
		}
	}
}

func TestProjectEndpointAnchorsApplySymbolRotation(t *testing.T) {
	operation, err := wrapOperation(transactions.OpAddSymbol, transactions.AddSymbolOperation{
		Op:        transactions.OpAddSymbol,
		Ref:       "R1",
		Role:      "series_resistor",
		Value:     "1k",
		LibraryID: "Device:R",
		At:        transactions.Point{XMM: 10, YMM: 20},
		Rotation:  90,
		Pins: []transactions.PinSpec{
			{Number: "1", XMM: -3.81, YMM: 0},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	anchors, err := projectEndpointAnchors([]transactions.Operation{operation})
	if err != nil {
		t.Fatal(err)
	}
	anchor := anchors[projectEndpointKey{ref: "R1", pin: "1"}]
	if math.Abs(anchor.xMM-10) > 0.000001 || math.Abs(anchor.yMM-16.19) > 0.000001 {
		t.Fatalf("anchor = %#v, want rotated pin offset applied", anchor)
	}
}

func TestProjectTransactionMaterializesGeneratedToExternalConnection(t *testing.T) {
	connect, issues := ConnectOperation("R1", "1", "J1", "1", "SIG")
	if len(issues) != 0 {
		t.Fatalf("connect issues: %#v", issues)
	}
	tx, err := ProjectTransactionForBlockOutput("mixed", BlockOutput{
		Instance: BlockInstance{Refs: []string{"R1"}},
		Operations: []transactions.Operation{
			connect,
		},
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	var connects []transactions.ConnectOperation
	for _, operation := range tx.Operations {
		if operation.Op != transactions.OpConnect {
			continue
		}
		var payload transactions.ConnectOperation
		if err := decodeBlockOperation(operation, &payload); err != nil {
			t.Fatal(err)
		}
		connects = append(connects, payload)
	}
	if len(connects) != 1 {
		t.Fatalf("connects = %#v, want one generated-to-external connection", connects)
	}
	got := connects[0]
	if got.NetName != "SIG" || got.From.Ref != "J1" || got.From.Pin != "1" || got.To.Ref != "R1" || got.To.Pin != "1" {
		t.Fatalf("materialized connect = %#v, want external endpoint preserved", got)
	}
}

func TestProjectTransactionReportsMalformedConnect(t *testing.T) {
	_, err := ProjectTransactionForBlockOutput("bad", BlockOutput{
		Instance: BlockInstance{Refs: []string{"R1", "D1"}},
		Operations: []transactions.Operation{
			{Op: transactions.OpConnect},
		},
	}, false)
	if err == nil {
		t.Fatal("expected malformed connect error")
	}
}

func TestProjectTransactionRejectsDuplicateBlockRefs(t *testing.T) {
	_, err := ProjectTransactionForBlockOutput("bad", BlockOutput{
		Instance: BlockInstance{InstanceID: "status", Refs: []string{"R1", "R1"}},
	}, false)
	if err == nil || !strings.Contains(err.Error(), "duplicate generated reference R1") {
		t.Fatalf("expected duplicate ref error, got %v", err)
	}
}

func TestProjectTransactionRejectsDuplicateCompositionRefs(t *testing.T) {
	_, err := ProjectTransactionForCompositionOutput("bad", CompositionOutput{
		Instances: []BlockInstance{
			{InstanceID: "left", Refs: []string{"R1"}},
			{InstanceID: "right", Refs: []string{"R1"}},
		},
	}, false)
	if err == nil || !strings.Contains(err.Error(), "left and right") {
		t.Fatalf("expected duplicate composition ref error, got %v", err)
	}
}

func TestProjectTransactionAppliesBlockOutput(t *testing.T) {
	registry := NewBuiltinRegistry()
	output, issues := registry.Instantiate(context.Background(), BlockRequest{BlockID: "led_indicator", InstanceID: "status"})
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("instantiate issues: %#v", issues)
	}
	tx, err := ProjectTransactionForBlockOutput("status_led", output, false)
	if err != nil {
		t.Fatal(err)
	}
	outputDir := filepath.Join(t.TempDir(), "status_led")

	result := transactions.Apply(tx, transactions.ApplyOptions{OutputDir: outputDir})
	if len(result.Issues) != 0 {
		t.Fatalf("unexpected apply issues: %#v", result.Issues)
	}
	for _, name := range []string{"status_led.kicad_pro", "status_led.kicad_sch", "status_led.kicad_pcb"} {
		if _, err := os.Stat(filepath.Join(outputDir, name)); err != nil {
			t.Fatalf("expected %s: %v", name, err)
		}
	}
}

func TestProjectTransactionAppliesCompositionOutput(t *testing.T) {
	registry := NewBuiltinRegistry()
	output := ComposeBlocks(context.Background(), registry, CompositionRequest{
		ProjectName: "composed",
		Instances: []CompositionInstance{
			{ID: "status", BlockID: "led_indicator"},
			{ID: "header", BlockID: "connector_breakout", Params: map[string]any{"pin_names": []string{"SIG", "GND"}}},
		},
		Connections: []CompositionConnection{
			{From: PortRef{InstanceID: "header", Port: "SIG"}, To: PortRef{InstanceID: "status", Port: "IN"}, NetAlias: "LED_EN"},
		},
	})
	issues := output.Issues
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("compose issues: %#v", issues)
	}
	tx, err := ProjectTransactionForCompositionOutput("composed", output, false)
	if err != nil {
		t.Fatal(err)
	}
	outputDir := filepath.Join(t.TempDir(), "composed")

	result := transactions.Apply(tx, transactions.ApplyOptions{OutputDir: outputDir})
	if len(result.Issues) != 0 {
		t.Fatalf("unexpected apply issues: %#v", result.Issues)
	}
	for _, name := range []string{"composed.kicad_pro", "composed.kicad_sch", "composed.kicad_pcb"} {
		if _, err := os.Stat(filepath.Join(outputDir, name)); err != nil {
			t.Fatalf("expected %s: %v", name, err)
		}
	}
}
