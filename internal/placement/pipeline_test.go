package placement

import (
	"context"
	"math"
	"path/filepath"
	"testing"
	"time"

	"kicadai/internal/blocks"
	"kicadai/internal/kicadfiles"
	kicaddesign "kicadai/internal/kicadfiles/design"
	"kicadai/internal/libraryresolver"
	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

func TestBlockToPCBPlacementPipelineWritesPlacedFootprints(t *testing.T) {
	registry := blocks.NewBuiltinRegistry()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	output, instantiateIssues := registry.Instantiate(ctx, blocks.BlockRequest{
		BlockID:    "led_indicator",
		InstanceID: "status",
	})
	if reports.HasBlockingIssue(instantiateIssues) {
		t.Fatalf("instantiate issues: %#v", instantiateIssues)
	}
	index := placementPipelineLibraryIndex()
	request, adapterIssues := RequestFromBlockOutput(output, AdapterOptions{
		Board: BoardPlacementArea{WidthMM: 40, HeightMM: 25, MarginMM: 1},
		Rules: Rules{
			MaxCandidatesPerPart: 1000,
		},
		LibraryIndex: &index,
	})
	if reports.HasBlockingIssue(adapterIssues) {
		t.Fatalf("adapter issues: %#v", adapterIssues)
	}

	placementResult := Place(request)
	if placementResult.Status != StatusPlaced {
		t.Fatalf("placement status = %s, issues=%#v", placementResult.Status, placementResult.Issues)
	}
	quality := BuildQualityReport(request, placementResult)
	if !quality.Ready || quality.Metrics.PlacedCount != len(output.Instance.Refs) || quality.Metrics.EstimatedBoundsCount != 0 {
		t.Fatalf("quality = %#v, want ready resolver-backed placement for all block refs", quality)
	}
	placementOps, placementIssues := PlacementOperations(request, placementResult.Placements)
	if len(placementIssues) != 0 {
		t.Fatalf("placement operation issues: %#v", placementIssues)
	}
	projectName := output.Instance.InstanceID
	tx, err := blocks.ProjectTransactionForBlockOutput(projectName, output, false)
	if err != nil {
		t.Fatal(err)
	}
	tx = insertOperationsBeforeWriteProject(tx, placementOps)

	outputDir := filepath.Join(t.TempDir(), projectName)
	applyResult := transactions.Apply(tx, transactions.ApplyOptions{
		OutputDir:    outputDir,
		LibraryIndex: &index,
		Seed:         "placement-pipeline",
	})
	if len(applyResult.Issues) != 0 {
		t.Fatalf("apply issues: %#v", applyResult.Issues)
	}
	design, err := kicaddesign.ReadProjectDirectory(outputDir)
	if err != nil {
		t.Fatal(err)
	}
	if design.Schematic == nil || len(design.Schematic.Symbols) != len(output.Instance.Refs) {
		t.Fatalf("schematic symbols = %#v, want block refs %#v", design.Schematic, output.Instance.Refs)
	}
	if design.PCB == nil {
		t.Fatal("generated project did not include PCB")
	}
	if len(design.PCB.Footprints) != quality.Metrics.PlacedCount {
		t.Fatalf("PCB footprints = %d, want placed count %d", len(design.PCB.Footprints), quality.Metrics.PlacedCount)
	}
	placedByRef := placementResultsByRef(placementResult.Placements)
	for _, footprint := range design.PCB.Footprints {
		placement, ok := placedByRef[normalizeRef(footprint.Reference)]
		if !ok {
			t.Fatalf("unexpected PCB footprint reference %s", footprint.Reference)
		}
		if !sameMM(footprint.Position.X, placement.Position.XMM) || !sameMM(footprint.Position.Y, placement.Position.YMM) {
			t.Fatalf("footprint %s position = %#v, want %.2f,%.2f", footprint.Reference, footprint.Position, placement.Position.XMM, placement.Position.YMM)
		}
		if len(footprint.Pads) != 2 {
			t.Fatalf("footprint %s pads = %#v, want resolver-backed two-pad footprint", footprint.Reference, footprint.Pads)
		}
	}
}

func sameMM(got kicadfiles.IU, wantMM float64) bool {
	return math.Abs(float64(got-kicadfiles.MM(wantMM))) <= 10
}

func insertOperationsBeforeWriteProject(tx transactions.Transaction, operations []transactions.Operation) transactions.Transaction {
	if len(operations) == 0 {
		return tx
	}
	insertAt := len(tx.Operations)
	for i, operation := range tx.Operations {
		if operation.Op == transactions.OpWriteProject {
			insertAt = i
			break
		}
	}
	next := make([]transactions.Operation, 0, len(tx.Operations)+len(operations))
	next = append(next, tx.Operations[:insertAt]...)
	next = append(next, operations...)
	next = append(next, tx.Operations[insertAt:]...)
	tx.Operations = next
	return tx
}

func placementPipelineLibraryIndex() libraryresolver.LibraryIndex {
	return libraryresolver.LibraryIndex{
		Footprints: map[string]libraryresolver.FootprintRecord{
			"Resistor_SMD:R_0805_2012Metric": twoPadSMDRecord("Resistor_SMD:R_0805_2012Metric", "Resistor_SMD", "R_0805_2012Metric"),
			"LED_SMD:LED_0805_2012Metric":    twoPadSMDRecord("LED_SMD:LED_0805_2012Metric", "LED_SMD", "LED_0805_2012Metric"),
		},
	}
}

func twoPadSMDRecord(id string, nickname string, name string) libraryresolver.FootprintRecord {
	return libraryresolver.FootprintRecord{
		FootprintID:     id,
		LibraryNickname: nickname,
		Name:            name,
		Attributes:      []string{"smd"},
		GraphicsSummary: libraryresolver.GraphicsSummary{HasCourtyard: true},
		BoundingBox: libraryresolver.BoundingBox{
			Min: kicadfiles.Point{X: kicadfiles.MM(-1.0), Y: kicadfiles.MM(-0.6)},
			Max: kicadfiles.Point{X: kicadfiles.MM(1.0), Y: kicadfiles.MM(0.6)},
		},
		Pads: []libraryresolver.FootprintPad{
			{
				Name:     "1",
				Type:     "smd",
				Shape:    "roundrect",
				Position: kicadfiles.Point{X: kicadfiles.MM(-0.8)},
				Size:     kicadfiles.Point{X: kicadfiles.MM(0.8), Y: kicadfiles.MM(0.9)},
				Layers:   []kicadfiles.BoardLayer{kicadfiles.LayerFCu, kicadfiles.LayerFMask},
			},
			{
				Name:     "2",
				Type:     "smd",
				Shape:    "roundrect",
				Position: kicadfiles.Point{X: kicadfiles.MM(0.8)},
				Size:     kicadfiles.Point{X: kicadfiles.MM(0.8), Y: kicadfiles.MM(0.9)},
				Layers:   []kicadfiles.BoardLayer{kicadfiles.LayerFCu, kicadfiles.LayerFMask},
			},
		},
	}
}
