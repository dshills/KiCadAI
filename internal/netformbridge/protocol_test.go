package netformbridge

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func request(t *testing.T, method string, params any) []byte {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"schema": RequestSchema, "id": "test-1", "method": method, "params": params})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
func TestNetFormProtocolContractAndNegativeControls(t *testing.T) {
	for _, c := range []struct {
		method string
		params any
	}{{"capabilities", map[string]any{}}, {"catalog.search", map[string]any{"family": "resistor", "limit": 2}}, {"catalog.get", map[string]any{"id": "resistor.generic.0603"}}, {"pinmap.get", map[string]any{"symbol": "Device:R", "footprint": "Resistor_SMD:R_0603_1608Metric"}}} {
		response := Handle(context.Background(), request(t, c.method, c.params))
		if !response.OK {
			t.Fatalf("%s %+v", c.method, response)
		}
		if response.Schema != ResponseSchema || response.ID != "test-1" || len(response.Result) == 0 {
			t.Fatal("bad envelope")
		}
	}
	for _, raw := range [][]byte{[]byte("{}"), []byte("{bad"), []byte(strings.Repeat(" ", MaxRequest+1)), request(t, "execute", map[string]any{}), request(t, "catalog.search", map[string]any{"limit": 101}), request(t, "capabilities", map[string]any{"provider": "openai"}), request(t, "catalog.get", map[string]any{"id": "unknown"}), request(t, "evidence.verify", map[string]any{"kind": "arbitrary", "path": "/tmp"})} {
		if response := Handle(context.Background(), raw); response.OK || response.Error == nil || len(response.Result) != 0 {
			t.Fatalf("accepted %s", raw[:min(len(raw), 100)])
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if Handle(ctx, request(t, "capabilities", map[string]any{})).Error.Code != "CANCELLED" {
		t.Fatal("cancel ignored")
	}
}
