package roundtrip

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"kicadai/internal/kicadfiles/checks"
	kicaddesign "kicadai/internal/kicadfiles/design"
	"kicadai/internal/kicadfiles/schematic"
	"kicadai/internal/libraryresolver"
	"kicadai/internal/reports"
	"kicadai/internal/schematicir"
	"kicadai/internal/schematiclayout"
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

func TestKiCadRoundTripSchematicIRMixedSupplyOpAmp(t *testing.T) {
	cli := requireKiCadCLI(t)
	fixture, err := os.Open(repoPath(t, "examples", "schematic-ir", "mixed_supply_opamp.json"))
	if err != nil {
		t.Fatalf("open mixed-supply op-amp IR: %v", err)
	}
	defer fixture.Close()
	document, issues := schematicir.DecodeStrict(fixture)
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("decode mixed-supply op-amp IR: %#v", issues)
	}
	tx, issues := schematicir.ToProjectTransaction(document)
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("adapt mixed-supply op-amp IR: %#v", issues)
	}
	output := filepath.Join(t.TempDir(), "mixed_supply_opamp")
	apply := transactions.Apply(tx, transactions.ApplyOptions{OutputDir: output, Overwrite: true})
	if reports.HasBlockingIssue(apply.Issues) {
		t.Fatalf("write mixed-supply op-amp schematic: %#v", apply.Issues)
	}
	schematicPath := filepath.Join(output, "mixed_supply_opamp.kicad_sch")
	generated, err := schematic.ReadFile(schematicPath)
	if err != nil {
		t.Fatalf("read mixed-supply op-amp schematic: %v", err)
	}
	request, layout := schematiclayout.AdaptSchematic(&generated)
	layout = schematiclayout.Validate(layout, request)
	readability := schematiclayout.BuildReport(layout, schematiclayout.ProfileStrict)
	if !readability.Passed {
		t.Fatalf("mixed-supply op-amp readability failed: %#v diagnostics=%#v", readability, layout.Diagnostics)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	erc, err := checks.RunERC(ctx, checks.KiCadCLI{Path: cli.Path}, schematicPath, checks.Options{KeepArtifacts: true, ArtifactDir: filepath.Join(t.TempDir(), "erc")})
	if err != nil {
		t.Fatalf("mixed-supply op-amp ERC returned error: %v\nresult=%#v", err, erc)
	}
	if erc.Status != checks.CheckStatusPass || len(erc.Findings) != 0 {
		t.Fatalf("mixed-supply op-amp ERC status = %s, findings=%#v parser=%#v", erc.Status, erc.Findings, erc.ParserIssues)
	}
	roundTrip, err := RoundTripSchematic(ctx, cli, schematicPath, Options{KeepArtifacts: true, ArtifactDir: filepath.Join(t.TempDir(), "roundtrip")})
	if err != nil {
		t.Fatalf("mixed-supply op-amp round trip returned error: %v\nresult=%#v", err, roundTrip)
	}
	if !roundTrip.Equal {
		t.Fatalf("mixed-supply op-amp round trip changed generated schematic: %s", firstResultDifference(roundTrip))
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
	symbolPath := resolverFixtureSymbolPath(symbolsRoot, "Connector_Generic", "Conn_02x02_Odd_Even")
	if symbolPath == "" {
		t.Skipf("matching Connector_Generic:Conn_02x02_Odd_Even symbol is unavailable under %s", symbolsRoot)
	}
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

func TestKiCadRoundTripSchematicIRMultiUnit(t *testing.T) {
	cli := requireKiCadCLI(t)
	symbolsRoot := strings.TrimSpace(os.Getenv(libraryresolver.EnvSymbolsRoot))
	if symbolsRoot == "" {
		t.Skip("set KICADAI_SYMBOLS_ROOT to a symbol library matching the KiCad CLI for multi-unit promotion")
	}
	connectorPath := resolverFixtureSymbolPath(symbolsRoot, "Connector_Generic", "Conn_02x02_Odd_Even")
	if connectorPath == "" {
		t.Skipf("matching Connector_Generic:Conn_02x02_Odd_Even symbol is unavailable under %s", symbolsRoot)
	}
	connectorData, err := os.ReadFile(connectorPath)
	if err != nil {
		t.Fatalf("read resolver connector library: %v", err)
	}
	multiUnitData, err := os.ReadFile(repoPath(t, "internal", "schematicir", "testdata", "symbols", "MultiUnit.kicad_sym"))
	if err != nil {
		t.Fatalf("read multi-unit resolver library: %v", err)
	}
	resolverRoot := t.TempDir()
	for name, data := range map[string][]byte{
		"MultiUnit.kicad_sym":         multiUnitData,
		"Connector_Generic.kicad_sym": connectorData,
	} {
		if err := os.WriteFile(filepath.Join(resolverRoot, name), data, 0o600); err != nil {
			t.Fatalf("stage resolver library %s: %v", name, err)
		}
	}
	index, issues := libraryresolver.Load(context.Background(), libraryresolver.LibraryRoots{SymbolsRoot: resolverRoot}, libraryresolver.LoadOptions{})
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("load multi-unit resolver root: %#v", issues)
	}

	fixture, err := os.Open(repoPath(t, "examples", "schematic-ir", "multi_unit.json"))
	if err != nil {
		t.Fatalf("open multi-unit IR: %v", err)
	}
	defer fixture.Close()
	document, decodeIssues := schematicir.DecodeStrict(fixture)
	if len(decodeIssues) != 0 {
		t.Fatalf("decode multi-unit IR: %#v", decodeIssues)
	}
	tx, adapterIssues := schematicir.ToProjectTransactionWithLibraryIndex(document, &index)
	if reports.HasBlockingIssue(adapterIssues) {
		t.Fatalf("adapt multi-unit IR: %#v", adapterIssues)
	}
	output := filepath.Join(t.TempDir(), "multi_unit")
	apply := transactions.Apply(tx, transactions.ApplyOptions{OutputDir: output, Overwrite: true, LibraryIndex: &index})
	if reports.HasBlockingIssue(apply.Issues) {
		t.Fatalf("write multi-unit project: %#v", apply.Issues)
	}
	read, err := kicaddesign.ReadProjectDirectory(output)
	if err != nil {
		t.Fatalf("read multi-unit project: %v", err)
	}
	if read.Schematic == nil {
		t.Fatal("multi-unit project has no root schematic")
	}
	units := make([]int, 0, 2)
	for _, symbol := range read.Schematic.Symbols {
		if symbol.Reference == "U1" {
			units = append(units, symbol.Unit)
		}
	}
	sort.Ints(units)
	if !reflect.DeepEqual(units, []int{1, 2}) {
		t.Fatalf("multi-unit U1 units = %#v, want [1 2]", units)
	}
	request, result := schematiclayout.AdaptSchematic(read.Schematic)
	result = schematiclayout.Validate(result, request)
	readability := schematiclayout.BuildReport(result, schematiclayout.ProfileStrict)
	if !readability.Passed {
		t.Fatalf("multi-unit readability failed: %#v diagnostics=%#v", readability, result.Diagnostics)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	erc, err := checks.RunERC(ctx, checks.KiCadCLI{Path: cli.Path}, output, checks.Options{KeepArtifacts: true, ArtifactDir: filepath.Join(t.TempDir(), "erc")})
	if err != nil {
		t.Fatalf("multi-unit ERC returned error: %v\nresult=%#v", err, erc)
	}
	if erc.Status != checks.CheckStatusPass || len(erc.Findings) != 0 {
		t.Fatalf("multi-unit ERC status = %s, findings=%#v parser=%#v", erc.Status, erc.Findings, erc.ParserIssues)
	}
	schematicPath := filepath.Join(output, read.Schematic.Filename)
	roundTrip, err := RoundTripSchematic(ctx, cli, schematicPath, Options{KeepArtifacts: true, ArtifactDir: filepath.Join(t.TempDir(), "roundtrip")})
	if err != nil {
		t.Fatalf("multi-unit round trip returned error: %v\nresult=%#v", err, roundTrip)
	}
	if !roundTrip.Equal {
		t.Fatalf("multi-unit round trip changed generated schematic: %s", firstResultDifference(roundTrip))
	}
}

func resolverFixtureSymbolPath(symbolsRoot, libraryName, symbolName string) string {
	candidates := []string{
		filepath.Join(symbolsRoot, libraryName+".kicad_sym"),
		filepath.Join(symbolsRoot, libraryName+".kicad_symdir", symbolName+".kicad_sym"),
	}
	for _, candidate := range candidates {
		if resolverFixtureContainsSymbol(candidate, libraryName, symbolName) {
			return candidate
		}
	}
	return ""
}

func resolverFixtureContainsSymbol(path, libraryName, symbolName string) bool {
	if info, err := os.Stat(path); err != nil || info.IsDir() {
		return false
	}
	inventory := libraryresolver.LibraryInventory{SymbolFiles: []libraryresolver.LibraryFile{{
		Kind:            libraryresolver.LibraryFileSymbol,
		Path:            filepath.ToSlash(path),
		LibraryNickname: libraryName,
		Name:            filepath.Base(path),
		IDPrefix:        libraryName + ":",
	}}}
	records, issues := libraryresolver.IndexSymbols(inventory)
	if reports.HasBlockingIssue(issues) {
		return false
	}
	_, ok := records[libraryName+":"+symbolName]
	return ok
}

func TestKiCadRoundTripSchematicIRResolverInheritedGeometry(t *testing.T) {
	cli := requireKiCadCLI(t)
	root := repoPath(t, "internal", "schematicir", "testdata", "symbols")
	index, issues := libraryresolver.Load(context.Background(), libraryresolver.LibraryRoots{SymbolsRoot: root}, libraryresolver.LoadOptions{})
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("load resolver geometry fixture: %#v", issues)
	}
	fixturePath := repoPath(t, "examples", "schematic-ir", "resolver_geometry_stress.json")
	fixture, err := os.Open(fixturePath)
	if err != nil {
		t.Fatalf("open resolver geometry IR: %v", err)
	}
	document, decodeIssues := schematicir.DecodeStrict(fixture)
	_ = fixture.Close()
	if reports.HasBlockingIssue(decodeIssues) {
		t.Fatalf("decode resolver geometry IR: %#v", decodeIssues)
	}
	tx, adapterIssues := schematicir.ToProjectTransactionWithLibraryIndex(document, &index)
	if reports.HasBlockingIssue(adapterIssues) {
		t.Fatalf("adapt resolver geometry IR: %#v", adapterIssues)
	}
	output := filepath.Join(t.TempDir(), "resolver_geometry_stress")
	apply := transactions.Apply(tx, transactions.ApplyOptions{OutputDir: output, Overwrite: true, LibraryIndex: &index})
	if reports.HasBlockingIssue(apply.Issues) {
		t.Fatalf("write resolver geometry project: %#v", apply.Issues)
	}
	for _, asset := range []string{
		filepath.Join(output, "lib", "kicadai_resolved_Amplifier.kicad_sym"),
		filepath.Join(output, "lib", "kicadai_resolved_Connector_Generic.kicad_sym"),
	} {
		if _, err := os.Stat(asset); err != nil {
			t.Fatalf("missing resolver materialization %s: %v", asset, err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	schematicPath := filepath.Join(output, "resolver_geometry_stress.kicad_sch")
	erc, err := checks.RunERC(ctx, checks.KiCadCLI{Path: cli.Path}, output, checks.Options{KeepArtifacts: true, ArtifactDir: filepath.Join(t.TempDir(), "erc")})
	if err != nil {
		t.Fatalf("resolver geometry ERC returned error: %v\nresult=%#v", err, erc)
	}
	if erc.Status != checks.CheckStatusPass || len(erc.Findings) != 0 {
		t.Fatalf("resolver geometry ERC status = %s, findings=%#v parser=%#v", erc.Status, erc.Findings, erc.ParserIssues)
	}
	artifactDir := filepath.Join(t.TempDir(), "roundtrip")
	if configured := strings.TrimSpace(os.Getenv(envArtifactDir)); configured != "" {
		artifactDir = configured
	}
	roundTrip, err := RoundTripSchematic(ctx, cli, schematicPath, Options{KeepArtifacts: true, ArtifactDir: artifactDir})
	if err != nil {
		t.Fatalf("resolver geometry round trip returned error: %v\nresult=%#v", err, roundTrip)
	}
	if !roundTrip.Equal {
		t.Fatalf("resolver geometry round trip changed generated schematic: %s", firstResultDifference(roundTrip))
	}
}

func TestKiCadRoundTripSchematicIROversizedVectorBusHierarchy(t *testing.T) {
	cli := requireKiCadCLI(t)
	fixture, err := os.Open(repoPath(t, "examples", "schematic-ir", "vector_bus.json"))
	if err != nil {
		t.Fatalf("open vector bus IR: %v", err)
	}
	document, issues := schematicir.DecodeStrict(fixture)
	_ = fixture.Close()
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("decode vector bus IR: %#v", issues)
	}
	document.Policy.Acceptance = schematicir.AcceptanceReadable
	for index := 0; index < 80; index++ {
		document.Circuit.Components = append(document.Circuit.Components, schematicir.Component{
			ID: fmt.Sprintf("extra_%d", index), Ref: fmt.Sprintf("R%d", index+10),
			Role: schematicir.ComponentRoleResistor, Symbol: "Device:R", Value: "10k",
			Pins: []schematicir.Pin{{Number: "1", Role: schematicir.PinRoleOutput, NoConnect: true}, {Number: "2", Role: schematicir.PinRoleInput, NoConnect: true}},
		})
	}
	tx, adapterIssues := schematicir.ToProjectTransaction(document)
	if reports.HasBlockingIssue(adapterIssues) {
		t.Fatalf("adapt oversized vector bus IR: %#v", adapterIssues)
	}
	output := filepath.Join(t.TempDir(), "oversized_vector_bus")
	apply := transactions.Apply(tx, transactions.ApplyOptions{OutputDir: output, Overwrite: true})
	if reports.HasBlockingIssue(apply.Issues) {
		t.Fatalf("write oversized vector bus project: %#v", apply.Issues)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	erc, err := checks.RunERC(ctx, checks.KiCadCLI{Path: cli.Path}, output, checks.Options{KeepArtifacts: true, ArtifactDir: filepath.Join(t.TempDir(), "erc")})
	if err != nil {
		t.Fatalf("oversized vector bus ERC returned error: %v\nresult=%#v", err, erc)
	}
	if erc.Status != checks.CheckStatusPass || len(erc.Findings) != 0 {
		t.Fatalf("oversized vector bus ERC status = %s, findings=%#v parser=%#v", erc.Status, erc.Findings, erc.ParserIssues)
	}
}

func TestKiCadRoundTripSchematicIRArbitraryTopology(t *testing.T) {
	cli := requireKiCadCLI(t)
	fixture, err := os.Open(repoPath(t, "examples", "schematic-ir", "arbitrary_topology.json"))
	if err != nil {
		t.Fatalf("open arbitrary topology IR: %v", err)
	}
	defer fixture.Close()
	document, issues := schematicir.DecodeStrict(fixture)
	if len(issues) != 0 {
		t.Fatalf("decode arbitrary topology IR: %#v", issues)
	}
	tx, adapterIssues := schematicir.ToProjectTransaction(document)
	if reports.HasBlockingIssue(adapterIssues) {
		t.Fatalf("adapt arbitrary topology IR: %#v", adapterIssues)
	}
	baseDir := t.TempDir()
	output := filepath.Join(baseDir, "arbitrary_topology")
	apply := transactions.Apply(tx, transactions.ApplyOptions{OutputDir: output, Overwrite: true})
	if reports.HasBlockingIssue(apply.Issues) {
		t.Fatalf("write arbitrary topology project: %#v", apply.Issues)
	}

	read, err := kicaddesign.ReadProjectDirectory(output)
	if err != nil {
		t.Fatalf("read arbitrary topology project: %v", err)
	}
	if read.Schematic == nil {
		t.Fatalf("arbitrary topology project has no root schematic")
	}
	schematicPath := filepath.Join(output, read.Schematic.Filename)
	paths := make([]string, 0, len(read.SheetFiles)+1)
	seenPaths := map[string]struct{}{}
	appendPath := func(path string) {
		path = filepath.Clean(path)
		if _, seen := seenPaths[path]; seen {
			return
		}
		seenPaths[path] = struct{}{}
		paths = append(paths, path)
	}
	appendPath(schematicPath)
	for _, child := range read.SheetFiles {
		appendPath(filepath.Join(output, child.Filename))
	}
	for _, path := range paths {
		file, readErr := schematic.ReadFile(path)
		if readErr != nil {
			t.Fatalf("read generated schematic %s: %v", path, readErr)
		}
		request, result := schematiclayout.AdaptSchematic(&file)
		result = schematiclayout.Validate(result, request)
		readability := schematiclayout.BuildReport(result, schematiclayout.ProfileStrict)
		if !readability.Passed {
			t.Fatalf("arbitrary topology readability for %s: %#v diagnostics=%#v", path, readability, result.Diagnostics)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	erc, err := checks.RunERC(ctx, checks.KiCadCLI{Path: cli.Path}, output, checks.Options{KeepArtifacts: true, ArtifactDir: filepath.Join(baseDir, "erc")})
	if err != nil {
		t.Fatalf("arbitrary topology ERC returned error: %v\nresult=%#v", err, erc)
	}
	if erc.Status != checks.CheckStatusPass || len(erc.Findings) != 0 {
		t.Fatalf("arbitrary topology ERC status = %s, findings=%#v parser=%#v", erc.Status, erc.Findings, erc.ParserIssues)
	}
	roundTrip, err := RoundTripSchematic(ctx, cli, schematicPath, Options{KeepArtifacts: true, ArtifactDir: filepath.Join(baseDir, "roundtrip")})
	if err != nil {
		t.Fatalf("arbitrary topology round trip returned error: %v\nresult=%#v", err, roundTrip)
	}
	if !roundTrip.Equal {
		t.Fatalf("arbitrary topology round trip changed generated schematic: %s", firstResultDifference(roundTrip))
	}
}
