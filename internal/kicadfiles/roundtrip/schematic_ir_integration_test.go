package roundtrip

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"kicadai/internal/kicadfiles/checks"
	"kicadai/internal/reports"
	"kicadai/internal/schematicir"
	"kicadai/internal/transactions"
)

func TestKiCadRoundTripSchematicIRVectorBus(t *testing.T) {
	cli := requireKiCadCLI(t)
	fixturePath := repoPath(t, "examples", "schematic-ir", "vector_bus.json")
	fixture, err := os.Open(fixturePath)
	if err != nil {
		t.Fatalf("open vector bus IR: %v", err)
	}
	document, issues := schematicir.DecodeStrict(fixture)
	_ = fixture.Close()
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("decode vector bus IR: %#v", issues)
	}
	tx, issues := schematicir.ToProjectTransaction(document)
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("adapt vector bus IR: %#v", issues)
	}
	output := filepath.Join(t.TempDir(), "vector_bus")
	apply := transactions.Apply(tx, transactions.ApplyOptions{OutputDir: output, Overwrite: true})
	if reports.HasBlockingIssue(apply.Issues) {
		t.Fatalf("write vector bus schematic: %#v", apply.Issues)
	}
	schematicPath := filepath.Join(output, "vector_bus.kicad_sch")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	erc, err := checks.RunERC(ctx, checks.KiCadCLI{Path: cli.Path}, schematicPath, checks.Options{KeepArtifacts: true, ArtifactDir: filepath.Join(t.TempDir(), "erc")})
	if err != nil {
		t.Fatalf("RunERC returned error: %v\nresult=%#v", err, erc)
	}
	if erc.Status != checks.CheckStatusPass {
		t.Fatalf("vector bus ERC status = %s, findings=%#v parser=%#v", erc.Status, erc.Findings, erc.ParserIssues)
	}
	roundTrip, err := RoundTripSchematic(ctx, cli, schematicPath, Options{KeepArtifacts: true, ArtifactDir: filepath.Join(t.TempDir(), "roundtrip")})
	if err != nil {
		t.Fatalf("RoundTripSchematic returned error: %v\nresult=%#v", err, roundTrip)
	}
	if !roundTrip.Equal {
		t.Fatalf("vector bus round trip changed generated schematic: %s", firstResultDifference(roundTrip))
	}
}

func TestKiCadRoundTripSchematicIRUSBCLocalSymbol(t *testing.T) {
	cli := requireKiCadCLI(t)
	fixturePath := repoPath(t, "examples", "schematic-ir", "usb_c_led_indicator.json")
	fixture, err := os.Open(fixturePath)
	if err != nil {
		t.Fatalf("open USB-C LED IR: %v", err)
	}
	defer fixture.Close()
	document, issues := schematicir.DecodeStrict(fixture)
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("decode USB-C LED IR: %#v", issues)
	}
	tx, issues := schematicir.ToProjectTransaction(document)
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("adapt USB-C LED IR: %#v", issues)
	}
	output := filepath.Join(t.TempDir(), "usb_c_led_indicator")
	apply := transactions.Apply(tx, transactions.ApplyOptions{OutputDir: output, Overwrite: true})
	if reports.HasBlockingIssue(apply.Issues) {
		t.Fatalf("write USB-C LED schematic: %#v", apply.Issues)
	}
	if _, err := os.Stat(filepath.Join(output, "sym-lib-table")); err != nil {
		t.Fatalf("generated USB-C LED project missing sym-lib-table: %v artifacts=%#v", err, apply.Artifacts)
	}
	schematicPath := filepath.Join(output, "usb_c_led_indicator.kicad_sch")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	erc, err := checks.RunERC(ctx, checks.KiCadCLI{Path: cli.Path}, output, checks.Options{KeepArtifacts: true, ArtifactDir: filepath.Join(t.TempDir(), "erc")})
	if err != nil {
		t.Fatalf("RunERC returned error: %v\nresult=%#v", err, erc)
	}
	if erc.Status != checks.CheckStatusPass {
		t.Fatalf("USB-C LED ERC status = %s, findings=%#v parser=%#v", erc.Status, erc.Findings, erc.ParserIssues)
	}
	roundTrip, err := RoundTripSchematic(ctx, cli, schematicPath, Options{KeepArtifacts: true, ArtifactDir: filepath.Join(t.TempDir(), "roundtrip")})
	if err != nil {
		t.Fatalf("RoundTripSchematic returned error: %v\nresult=%#v", err, roundTrip)
	}
	if !roundTrip.Equal {
		t.Fatalf("USB-C LED round trip changed generated schematic: %s", firstResultDifference(roundTrip))
	}
}
