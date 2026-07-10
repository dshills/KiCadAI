package roundtrip

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"kicadai/internal/kicadfiles/checks"
	"kicadai/internal/libraryresolver"
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

func TestKiCadRoundTripSchematicIRLEDIndicator(t *testing.T) {
	cli := requireKiCadCLI(t)
	fixturePath := repoPath(t, "examples", "schematic-ir", "led_indicator.json")
	fixture, err := os.Open(fixturePath)
	if err != nil {
		t.Fatalf("open LED IR: %v", err)
	}
	document, issues := schematicir.DecodeStrict(fixture)
	_ = fixture.Close()
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("decode LED IR: %#v", issues)
	}
	tx, issues := schematicir.ToProjectTransaction(document)
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("adapt LED IR: %#v", issues)
	}
	output := filepath.Join(t.TempDir(), "led_indicator")
	apply := transactions.Apply(tx, transactions.ApplyOptions{OutputDir: output, Overwrite: true})
	if reports.HasBlockingIssue(apply.Issues) {
		t.Fatalf("write LED schematic: %#v", apply.Issues)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	erc, err := checks.RunERC(ctx, checks.KiCadCLI{Path: cli.Path}, filepath.Join(output, "led_indicator.kicad_sch"), checks.Options{KeepArtifacts: true, ArtifactDir: filepath.Join(t.TempDir(), "erc")})
	if err != nil {
		t.Fatalf("RunERC returned error: %v\nresult=%#v", err, erc)
	}
	if erc.Status != checks.CheckStatusPass {
		t.Fatalf("LED ERC status = %s, findings=%#v parser=%#v", erc.Status, erc.Findings, erc.ParserIssues)
	}
	roundTrip, err := RoundTripSchematic(ctx, cli, filepath.Join(output, "led_indicator.kicad_sch"), Options{KeepArtifacts: true, ArtifactDir: filepath.Join(t.TempDir(), "roundtrip")})
	if err != nil {
		t.Fatalf("RoundTripSchematic returned error: %v\nresult=%#v", err, roundTrip)
	}
	if !roundTrip.Equal {
		t.Fatalf("LED round trip changed generated schematic: %s", firstResultDifference(roundTrip))
	}
}

func TestKiCadRoundTripSchematicIRI2CSensorRegulator(t *testing.T) {
	cli := requireKiCadCLI(t)
	fixturePath := repoPath(t, "examples", "schematic-ir", "i2c_sensor_3v3_regulator.json")
	fixture, err := os.Open(fixturePath)
	if err != nil {
		t.Fatalf("open I2C IR: %v", err)
	}
	defer fixture.Close()
	document, issues := schematicir.DecodeStrict(fixture)
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("decode I2C IR: %#v", issues)
	}
	tx, issues := schematicir.ToProjectTransaction(document)
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("adapt I2C IR: %#v", issues)
	}
	output := filepath.Join(t.TempDir(), "i2c_sensor_3v3_regulator")
	apply := transactions.Apply(tx, transactions.ApplyOptions{OutputDir: output, Overwrite: true})
	if reports.HasBlockingIssue(apply.Issues) {
		t.Fatalf("write I2C schematic: %#v", apply.Issues)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	erc, err := checks.RunERC(ctx, checks.KiCadCLI{Path: cli.Path}, output, checks.Options{KeepArtifacts: true, ArtifactDir: filepath.Join(t.TempDir(), "erc")})
	if err != nil {
		t.Fatalf("RunERC returned error: %v\nresult=%#v", err, erc)
	}
	if erc.Status != checks.CheckStatusPass {
		t.Fatalf("I2C ERC status = %s, findings=%#v parser=%#v", erc.Status, erc.Findings, erc.ParserIssues)
	}
	schematicPath := filepath.Join(output, "i2c_sensor_3v3_regulator.kicad_sch")
	roundTrip, err := RoundTripSchematic(ctx, cli, schematicPath, Options{KeepArtifacts: true, ArtifactDir: filepath.Join(t.TempDir(), "roundtrip")})
	if err != nil {
		t.Fatalf("RoundTripSchematic returned error: %v\nresult=%#v", err, roundTrip)
	}
	if !roundTrip.Equal {
		t.Fatalf("I2C round trip changed generated schematic: %s", firstResultDifference(roundTrip))
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

func TestKiCadRoundTripSchematicIRResolverExternal(t *testing.T) {
	cli := requireKiCadCLI(t)
	symbolsRoot := strings.TrimSpace(os.Getenv(libraryresolver.EnvSymbolsRoot))
	if symbolsRoot == "" {
		t.Skip("set KICADAI_SYMBOLS_ROOT to a symbol library matching the KiCad CLI for resolver promotion")
	}
	symbolPath := filepath.Join(symbolsRoot, "Connector_Generic.kicad_sym")
	symbolData, err := os.ReadFile(symbolPath)
	if err != nil {
		t.Skipf("matching Connector_Generic library is unavailable at %s: %v", symbolPath, err)
	}
	resolverRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(resolverRoot, "Connector_Generic.kicad_sym"), symbolData, 0o600); err != nil {
		t.Fatalf("stage resolver symbol library: %v", err)
	}
	index, issues := libraryresolver.Load(context.Background(), libraryresolver.LibraryRoots{SymbolsRoot: resolverRoot}, libraryresolver.LoadOptions{})
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("load resolver symbol root: %#v", issues)
	}
	fixturePath := repoPath(t, "examples", "schematic-ir", "external_connector_indicator.json")
	fixture, err := os.Open(fixturePath)
	if err != nil {
		t.Fatalf("open resolver external IR: %v", err)
	}
	defer fixture.Close()
	document, decodeIssues := schematicir.DecodeStrict(fixture)
	if reports.HasBlockingIssue(decodeIssues) {
		t.Fatalf("decode resolver external IR: %#v", decodeIssues)
	}
	tx, adapterIssues := schematicir.ToProjectTransactionWithLibraryIndex(document, &index)
	if reports.HasBlockingIssue(adapterIssues) {
		t.Fatalf("adapt resolver external IR: %#v", adapterIssues)
	}
	output := filepath.Join(t.TempDir(), "external_connector_indicator")
	apply := transactions.Apply(tx, transactions.ApplyOptions{OutputDir: output, Overwrite: true, LibraryIndex: &index})
	if reports.HasBlockingIssue(apply.Issues) {
		t.Fatalf("write resolver external schematic: %#v", apply.Issues)
	}
	if _, err := os.Stat(filepath.Join(output, "sym-lib-table")); err != nil {
		t.Fatalf("resolver external project missing project-local symbol table: %v artifacts=%#v", err, apply.Artifacts)
	}
	if _, err := os.Stat(filepath.Join(output, "lib", "kicadai_resolved_Connector_Generic.kicad_sym")); err != nil {
		t.Fatalf("resolver external project missing materialized symbol library: %v artifacts=%#v", err, apply.Artifacts)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	schematicPath := filepath.Join(output, "external_connector_indicator.kicad_sch")
	erc, err := checks.RunERC(ctx, checks.KiCadCLI{Path: cli.Path}, schematicPath, checks.Options{KeepArtifacts: true, ArtifactDir: filepath.Join(t.TempDir(), "erc")})
	if err != nil {
		t.Fatalf("resolver external RunERC returned error: %v\nresult=%#v", err, erc)
	}
	if erc.Status != checks.CheckStatusPass || len(erc.Findings) != 0 {
		t.Fatalf("resolver external ERC status = %s, findings=%#v parser=%#v", erc.Status, erc.Findings, erc.ParserIssues)
	}
	roundTrip, err := RoundTripSchematic(ctx, cli, schematicPath, Options{KeepArtifacts: true, ArtifactDir: filepath.Join(t.TempDir(), "roundtrip")})
	if err != nil {
		t.Fatalf("resolver external round trip returned error: %v\nresult=%#v", err, roundTrip)
	}
	if !roundTrip.Equal {
		t.Fatalf("resolver external round trip changed generated schematic: %s", firstResultDifference(roundTrip))
	}
}
