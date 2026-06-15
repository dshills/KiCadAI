package placement

import (
	"context"
	"encoding/json"
	"testing"

	"kicadai/internal/blocks"
	"kicadai/internal/kicadfiles"
	"kicadai/internal/libraryresolver"
	"kicadai/internal/transactions"
)

func TestRequestFromOperationsBuildsPlacementRequest(t *testing.T) {
	net := "SIG"
	operations := []transactions.Operation{
		mustOperation(t, transactions.OpAddSymbol, transactions.AddSymbolOperation{
			Op:        transactions.OpAddSymbol,
			Ref:       "R1",
			Value:     "10k",
			LibraryID: "Device:R",
		}),
		mustOperation(t, transactions.OpPlaceFootprint, transactions.PlaceFootprintOperation{
			Op:          transactions.OpPlaceFootprint,
			Ref:         "R1",
			FootprintID: "Resistor_SMD:R_0805_2012Metric",
			At:          transactions.Point{XMM: 5, YMM: 6},
			Pads: []transactions.PadSpec{
				{Name: "1", XMM: -0.8, WidthMM: 0.6, HeightMM: 1, Net: &net},
				{Name: "2", XMM: 0.8, WidthMM: 0.6, HeightMM: 1},
			},
		}),
		mustOperation(t, transactions.OpConnect, transactions.ConnectOperation{
			Op:      transactions.OpConnect,
			From:    transactions.Endpoint{Ref: "R1", Pin: "1"},
			To:      transactions.Endpoint{Ref: "R1", Pin: "2"},
			NetName: "LOOP",
		}),
	}

	request, issues := RequestFromOperations(operations, AdapterOptions{
		Board:          BoardPlacementArea{WidthMM: 20, HeightMM: 15, MarginMM: 1},
		PreservePlaced: true,
	})
	if len(issues) != 0 {
		t.Fatalf("RequestFromOperations returned issues: %#v", issues)
	}
	if !request.Existing.PreserveFixed {
		t.Fatal("request Existing.PreserveFixed = false, want true")
	}
	if len(request.Components) != 1 {
		t.Fatalf("components = %#v, want one", request.Components)
	}
	component := request.Components[0]
	if component.Ref != "R1" || component.Value != "10k" || component.FootprintID != "Resistor_SMD:R_0805_2012Metric" {
		t.Fatalf("component identity = %#v", component)
	}
	if component.Position == nil || !component.Fixed || component.Position.XMM != 5 || component.Position.YMM != 6 {
		t.Fatalf("component position = %#v fixed=%v", component.Position, component.Fixed)
	}
	if component.Bounds.Source != BoundsGeneratedPads || component.Bounds.WidthMM <= 0 || len(component.Pads) != 2 {
		t.Fatalf("component footprint geometry = %#v pads=%#v", component.Bounds, component.Pads)
	}
	if len(request.Nets) != 1 || request.Nets[0].Name != "LOOP" || len(request.Nets[0].Endpoints) != 2 {
		t.Fatalf("nets = %#v", request.Nets)
	}
}

func TestRequestFromOperationsHydratesLibraryFootprint(t *testing.T) {
	index := libraryresolver.LibraryIndex{Footprints: map[string]libraryresolver.FootprintRecord{
		"Test:R": {
			FootprintID: "Test:R",
			BoundingBox: libraryresolver.BoundingBox{
				Min: kicadfiles.Point{X: kicadfiles.MM(-1), Y: kicadfiles.MM(-0.5)},
				Max: kicadfiles.Point{X: kicadfiles.MM(1), Y: kicadfiles.MM(0.5)},
			},
			Pads: []libraryresolver.FootprintPad{{Name: "1"}},
		},
	}}
	operations := []transactions.Operation{mustOperation(t, transactions.OpAssignFootprint, transactions.AssignFootprintOperation{
		Op:          transactions.OpAssignFootprint,
		Ref:         "R1",
		FootprintID: "Test:R",
	})}

	request, issues := RequestFromOperations(operations, AdapterOptions{LibraryIndex: &index})
	if len(issues) != 0 {
		t.Fatalf("RequestFromOperations returned issues: %#v", issues)
	}
	if len(request.Components) != 1 || request.Components[0].Bounds.WidthMM != 2 || len(request.Components[0].Pads) != 1 {
		t.Fatalf("components = %#v", request.Components)
	}
}

func TestRequestFromOperationsHydrationPreservesPlacementState(t *testing.T) {
	index := libraryresolver.LibraryIndex{Footprints: map[string]libraryresolver.FootprintRecord{
		"Test:R": {
			FootprintID: "Test:R",
			BoundingBox: libraryresolver.BoundingBox{
				Min: kicadfiles.Point{X: kicadfiles.MM(-2), Y: kicadfiles.MM(-1)},
				Max: kicadfiles.Point{X: kicadfiles.MM(2), Y: kicadfiles.MM(1)},
			},
			Pads: []libraryresolver.FootprintPad{{Name: "A"}},
		},
	}}
	operations := []transactions.Operation{mustOperation(t, transactions.OpPlaceFootprint, transactions.PlaceFootprintOperation{
		Op:          transactions.OpPlaceFootprint,
		Ref:         "R1",
		FootprintID: "Test:R",
		At:          transactions.Point{XMM: 7, YMM: 8},
		Pads: []transactions.PadSpec{
			{Name: "1", XMM: 0, WidthMM: 0.5, HeightMM: 0.5},
		},
	})}

	request, issues := RequestFromOperations(operations, AdapterOptions{LibraryIndex: &index, PreservePlaced: true})
	if len(issues) != 0 {
		t.Fatalf("RequestFromOperations returned issues: %#v", issues)
	}
	component := request.Components[0]
	if !component.Fixed || component.Position == nil || component.Position.XMM != 7 || component.Position.YMM != 8 {
		t.Fatalf("hydration lost placement state: %#v", component)
	}
	if component.Bounds.WidthMM != 4 || len(component.Pads) != 1 || component.Pads[0].Name != "A" {
		t.Fatalf("library geometry did not replace pad-derived geometry: bounds=%#v pads=%#v", component.Bounds, component.Pads)
	}
}

func TestRequestFromBlockOutputCreatesPlacementComponents(t *testing.T) {
	registry := blocks.NewBuiltinRegistry()
	output, instantiateIssues := registry.Instantiate(context.Background(), blocks.BlockRequest{BlockID: "led_indicator", InstanceID: "status"})
	if len(instantiateIssues) != 0 {
		t.Fatalf("Instantiate returned issues: %#v", instantiateIssues)
	}

	request, issues := RequestFromBlockOutput(output, AdapterOptions{
		Board:         BoardPlacementArea{WidthMM: 50, HeightMM: 25, MarginMM: 1},
		DefaultBounds: Bounds{WidthMM: 2, HeightMM: 1.25, Source: BoundsEstimated},
	})
	if len(issues) == 0 {
		t.Fatal("expected estimated bounds warnings for block footprints without resolver geometry")
	}
	if len(request.Components) == 0 {
		t.Fatal("expected block output to produce placement components")
	}
	result := Place(request)
	if result.Metrics.PlacedCount == 0 {
		t.Fatalf("expected placed block components, result=%#v", result)
	}
}

func mustOperation(t *testing.T, kind transactions.OperationKind, payload any) transactions.Operation {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal operation: %v", err)
	}
	return transactions.NewOperation(kind, raw)
}
