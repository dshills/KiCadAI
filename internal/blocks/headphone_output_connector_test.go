package blocks

import (
	"context"
	"slices"
	"testing"

	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

func TestHeadphoneOutputConnectorInstantiatesBoardEdgeConnector(t *testing.T) {
	output, issues := NewBuiltinRegistry().Instantiate(context.Background(), BlockRequest{
		BlockID:    "headphone_output_connector",
		InstanceID: "jack",
	})
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("issues = %#v", issues)
	}
	for _, net := range []string{"jack_hp_out", "jack_load_ret", "jack_load_ref"} {
		if !slices.Contains(output.Instance.Nets, net) {
			t.Fatalf("nets = %#v, want %s", output.Instance.Nets, net)
		}
	}
	validation := transactions.Validate(transactions.Transaction{Operations: output.Operations})
	if len(validation.Issues) != 0 {
		t.Fatalf("transaction validation issues = %#v", validation.Issues)
	}
}

func TestHeadphoneOutputConnectorBlocksSpeaker(t *testing.T) {
	_, issues := NewBuiltinRegistry().Instantiate(context.Background(), BlockRequest{
		BlockID:    "headphone_output_connector",
		InstanceID: "speaker",
		Params:     map[string]any{"load_kind": "speaker"},
	})
	if !reports.HasBlockingIssue(issues) {
		t.Fatalf("issues = %#v, want speaker connector blocker", issues)
	}
}
