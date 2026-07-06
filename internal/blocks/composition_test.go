package blocks

import (
	"context"
	"encoding/json"
	"testing"

	"kicadai/internal/transactions"
)

func TestComposeBlocksLEDPlusConnector(t *testing.T) {
	registry := NewBuiltinRegistry()
	output := ComposeBlocks(context.Background(), registry, CompositionRequest{
		ProjectName: "status_board",
		Instances: []CompositionInstance{
			{ID: "status", BlockID: "led_indicator"},
			{ID: "header", BlockID: "connector_breakout", Params: map[string]any{"pin_names": []string{"SIG", "GND"}}},
		},
		Connections: []CompositionConnection{
			{From: PortRef{InstanceID: "header", Port: "SIG"}, To: PortRef{InstanceID: "status", Port: "IN"}, NetAlias: "STATUS"},
			{From: PortRef{InstanceID: "header", Port: "GND"}, To: PortRef{InstanceID: "status", Port: "GND"}, NetAlias: "GND"},
		},
	})
	if len(output.Issues) != 0 {
		t.Fatalf("issues = %#v", output.Issues)
	}
	if len(output.Instances) != 2 {
		t.Fatalf("instances = %#v", output.Instances)
	}
	if len(output.Operations) == 0 || output.Operations[len(output.Operations)-1].Op != transactions.OpConnect {
		t.Fatalf("operations = %#v", output.Operations)
	}
	validation := transactions.Validate(transactions.Transaction{Operations: output.Operations})
	if len(validation.Issues) != 0 {
		t.Fatalf("validation issues = %#v", validation.Issues)
	}
}

func TestComposeBlocksRejectsDuplicateInstanceID(t *testing.T) {
	registry := NewBuiltinRegistry()
	output := ComposeBlocks(context.Background(), registry, CompositionRequest{Instances: []CompositionInstance{
		{ID: "dup", BlockID: "led_indicator"},
		{ID: "dup", BlockID: "connector_breakout", Params: map[string]any{"pin_names": []string{"A"}}},
	}})
	if len(output.Issues) != 1 || output.Issues[0].Path != "instances.dup" {
		t.Fatalf("issues = %#v", output.Issues)
	}
}

func TestComposeBlocksRejectsUnknownPort(t *testing.T) {
	registry := NewBuiltinRegistry()
	output := ComposeBlocks(context.Background(), registry, CompositionRequest{
		Instances: []CompositionInstance{
			{ID: "status", BlockID: "led_indicator"},
		},
		Connections: []CompositionConnection{
			{From: PortRef{InstanceID: "status", Port: "MISSING"}, To: PortRef{InstanceID: "status", Port: "GND"}},
		},
	})
	if len(output.Issues) != 1 || output.Issues[0].Path != "connections[0].from" {
		t.Fatalf("issues = %#v", output.Issues)
	}
}

func TestComposeBlocksRejectsConflictingVoltageDomains(t *testing.T) {
	registry := NewRegistry([]BlockDefinition{
		testPowerBlock("a", "3.3V"),
		testPowerBlock("b", "5V"),
	})
	output := ComposeBlocks(context.Background(), registry, CompositionRequest{
		Instances: []CompositionInstance{
			{ID: "a1", BlockID: "a"},
			{ID: "b1", BlockID: "b"},
		},
		Connections: []CompositionConnection{
			{From: PortRef{InstanceID: "a1", Port: "VOUT"}, To: PortRef{InstanceID: "b1", Port: "VOUT"}},
		},
	})
	if len(output.Issues) != 1 || output.Issues[0].Message != "conflicting voltage domains" {
		t.Fatalf("issues = %#v", output.Issues)
	}
}

func TestComposeBlocksRejectsTransitiveVoltageConflict(t *testing.T) {
	registry := NewRegistry([]BlockDefinition{
		testPowerBlock("a", "3.3V"),
		testSignalBlock("b"),
		testPowerBlock("c", "5V"),
	})
	output := ComposeBlocks(context.Background(), registry, CompositionRequest{
		Instances: []CompositionInstance{
			{ID: "a1", BlockID: "a"},
			{ID: "b1", BlockID: "b"},
			{ID: "c1", BlockID: "c"},
		},
		Connections: []CompositionConnection{
			{From: PortRef{InstanceID: "a1", Port: "VOUT"}, To: PortRef{InstanceID: "b1", Port: "SIG"}},
			{From: PortRef{InstanceID: "b1", Port: "SIG"}, To: PortRef{InstanceID: "c1", Port: "VOUT"}},
		},
	})
	if len(output.Issues) != 1 || output.Issues[0].Message != "conflicting voltage domains" {
		t.Fatalf("issues = %#v", output.Issues)
	}
}

func TestCompatiblePortDirectionsAllowsSharedSignalsAndRails(t *testing.T) {
	if !compatiblePortDirections(PortInput, PortInput) {
		t.Fatalf("input to input should be allowed")
	}
	if !compatiblePortDirections(PortPower, PortInput) {
		t.Fatalf("power to input should be allowed")
	}
}

func TestComposeBlocksMergesUnnamedConnectionNets(t *testing.T) {
	registry := NewRegistry([]BlockDefinition{
		testSignalBlock("a"),
		testSignalBlock("b"),
		testSignalBlock("c"),
	})
	output := ComposeBlocks(context.Background(), registry, CompositionRequest{
		Instances: []CompositionInstance{
			{ID: "a1", BlockID: "a"},
			{ID: "b1", BlockID: "b"},
			{ID: "c1", BlockID: "c"},
		},
		Connections: []CompositionConnection{
			{From: PortRef{InstanceID: "a1", Port: "SIG"}, To: PortRef{InstanceID: "b1", Port: "SIG"}},
			{From: PortRef{InstanceID: "b1", Port: "SIG"}, To: PortRef{InstanceID: "c1", Port: "SIG"}},
		},
	})
	if len(output.Issues) != 0 {
		t.Fatalf("issues = %#v", output.Issues)
	}
	var first transactions.ConnectOperation
	var second transactions.ConnectOperation
	if err := json.Unmarshal(output.Operations[0].Raw, &first); err != nil {
		t.Fatalf("decode first: %v", err)
	}
	if err := json.Unmarshal(output.Operations[1].Raw, &second); err != nil {
		t.Fatalf("decode second: %v", err)
	}
	if first.NetName == "" || first.NetName != second.NetName {
		t.Fatalf("net names = %q, %q", first.NetName, second.NetName)
	}
}

func TestComposeBlocksAppliesAliasToConnectedGroup(t *testing.T) {
	registry := NewRegistry([]BlockDefinition{
		testSignalBlock("a"),
		testSignalBlock("b"),
		testSignalBlock("c"),
	})
	output := ComposeBlocks(context.Background(), registry, CompositionRequest{
		Instances: []CompositionInstance{
			{ID: "a1", BlockID: "a"},
			{ID: "b1", BlockID: "b"},
			{ID: "c1", BlockID: "c"},
		},
		Connections: []CompositionConnection{
			{From: PortRef{InstanceID: "a1", Port: "SIG"}, To: PortRef{InstanceID: "b1", Port: "SIG"}, NetAlias: "BUS"},
			{From: PortRef{InstanceID: "b1", Port: "SIG"}, To: PortRef{InstanceID: "c1", Port: "SIG"}},
		},
		NetAliases: map[string]string{"BUS": "SENSOR_BUS"},
	})
	if len(output.Issues) != 0 {
		t.Fatalf("issues = %#v", output.Issues)
	}
	for _, operation := range output.Operations {
		var connect transactions.ConnectOperation
		if err := json.Unmarshal(operation.Raw, &connect); err != nil {
			t.Fatalf("decode connect: %v", err)
		}
		if connect.NetName != "SENSOR_BUS" {
			t.Fatalf("connect = %#v", connect)
		}
	}
}

func TestComposeBlocksRewritesLocalLabelsForConnectionAliases(t *testing.T) {
	output := ComposeBlocks(context.Background(), NewBuiltinRegistry(), CompositionRequest{
		Instances: []CompositionInstance{
			{ID: "headphones", BlockID: "headphone_output_connector"},
			{ID: "header", BlockID: "connector_breakout", Params: map[string]any{"pin_names": []string{"SIG", "RET", "LOAD_REF"}}},
		},
		Connections: []CompositionConnection{
			{From: PortRef{InstanceID: "headphones", Port: "HP_OUT"}, To: PortRef{InstanceID: "header", Port: "SIG"}, NetAlias: "HP_OUT"},
			{From: PortRef{InstanceID: "headphones", Port: "LOAD_RET"}, To: PortRef{InstanceID: "header", Port: "RET"}, NetAlias: "HP_RET"},
			{From: PortRef{InstanceID: "headphones", Port: "LOAD_REF"}, To: PortRef{InstanceID: "header", Port: "LOAD_REF"}, NetAlias: "LOAD_REF"},
		},
	})
	if len(output.Issues) != 0 {
		t.Fatalf("issues = %#v", output.Issues)
	}
	labels := compositionLabels(t, output.Operations)
	for _, old := range []string{"headphones_hp_out", "headphones_load_ret", "headphones_load_ref"} {
		if labels[old] {
			t.Fatalf("local label %s was not canonicalized: %#v", old, labels)
		}
	}
	for _, canonical := range []string{"HP_OUT", "HP_RET", "LOAD_REF"} {
		if !labels[canonical] {
			t.Fatalf("missing canonical label %s in %#v", canonical, labels)
		}
	}
}

func TestComposeBlocksSanitizesAliases(t *testing.T) {
	registry := NewRegistry([]BlockDefinition{
		testSignalBlock("a"),
		testSignalBlock("b"),
	})
	output := ComposeBlocks(context.Background(), registry, CompositionRequest{
		Instances: []CompositionInstance{
			{ID: "a1", BlockID: "a"},
			{ID: "b1", BlockID: "b"},
		},
		Connections: []CompositionConnection{
			{From: PortRef{InstanceID: "a1", Port: "SIG"}, To: PortRef{InstanceID: "b1", Port: "SIG"}, NetAlias: "A BUS"},
		},
	})
	if len(output.Issues) != 0 {
		t.Fatalf("issues = %#v", output.Issues)
	}
	var connect transactions.ConnectOperation
	if err := json.Unmarshal(output.Operations[0].Raw, &connect); err != nil {
		t.Fatalf("decode connect: %v", err)
	}
	if connect.NetName != "A_BUS" {
		t.Fatalf("connect = %#v", connect)
	}
}

func compositionLabels(t *testing.T, operations []transactions.Operation) map[string]bool {
	t.Helper()
	labels := map[string]bool{}
	for _, operation := range operations {
		if operation.Op != transactions.OpAddLabel {
			continue
		}
		var payload transactions.AddLabelOperation
		if err := json.Unmarshal(operation.Raw, &payload); err != nil {
			t.Fatalf("decode add_label: %v", err)
		}
		labels[payload.Text] = true
	}
	return labels
}

func TestComposeBlocksReportsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	output := ComposeBlocks(ctx, NewBuiltinRegistry(), CompositionRequest{Instances: []CompositionInstance{{ID: "status", BlockID: "led_indicator"}}})
	if len(output.Issues) != 1 || output.Issues[0].Path != "composition" {
		t.Fatalf("issues = %#v", output.Issues)
	}
}

func TestComposeBlocksRejectsConflictingAliasesInGroup(t *testing.T) {
	registry := NewRegistry([]BlockDefinition{
		testSignalBlock("a"),
		testSignalBlock("b"),
		testSignalBlock("c"),
	})
	output := ComposeBlocks(context.Background(), registry, CompositionRequest{
		Instances: []CompositionInstance{
			{ID: "a1", BlockID: "a"},
			{ID: "b1", BlockID: "b"},
			{ID: "c1", BlockID: "c"},
		},
		Connections: []CompositionConnection{
			{From: PortRef{InstanceID: "a1", Port: "SIG"}, To: PortRef{InstanceID: "b1", Port: "SIG"}, NetAlias: "A"},
			{From: PortRef{InstanceID: "b1", Port: "SIG"}, To: PortRef{InstanceID: "c1", Port: "SIG"}, NetAlias: "B"},
		},
	})
	if len(output.Issues) != 1 || output.Issues[0].Path != "connections[1].net_alias" {
		t.Fatalf("issues = %#v", output.Issues)
	}
}

func TestPortMapRejectsDuplicatePorts(t *testing.T) {
	_, issues := portMap("dup", []BlockPort{{Name: "A"}, {Name: "A"}})
	if len(issues) != 1 || issues[0].Path != "instances.dup.ports.A" {
		t.Fatalf("issues = %#v", issues)
	}
}

func testSignalBlock(id string) BlockDefinition {
	return BlockDefinition{
		ID:      id,
		Name:    id,
		Version: "0.1.0",
		Parameters: []BlockParameter{
			{Name: "enabled", Type: ParameterBool, Default: true},
		},
		Ports:        []BlockPort{{Name: "SIG", Direction: PortInput}},
		Verification: VerificationRecord{Level: VerificationExperimental},
	}
}

func testPowerBlock(id string, voltage string) BlockDefinition {
	return BlockDefinition{
		ID:      id,
		Name:    id,
		Version: "0.1.0",
		Parameters: []BlockParameter{
			{Name: "enabled", Type: ParameterBool, Default: true},
		},
		Ports:        []BlockPort{{Name: "VOUT", Direction: PortPower, Voltage: voltage}},
		Verification: VerificationRecord{Level: VerificationExperimental},
	}
}
