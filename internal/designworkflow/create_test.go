package designworkflow

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"kicadai/internal/blocks"
	"kicadai/internal/componentprops"
	"kicadai/internal/components"
	"kicadai/internal/fabrication"
	"kicadai/internal/inspect"
	"kicadai/internal/kicadfiles"
	pcbfiles "kicadai/internal/kicadfiles/pcb"
	"kicadai/internal/kicadfiles/schematic"
	"kicadai/internal/libraryresolver"
	"kicadai/internal/reports"
	"kicadai/internal/schematicrules"
	"kicadai/internal/transactions"
)

func TestPlacementOptionsForCreatePropagatesAuthoritativeLibraryIndex(t *testing.T) {
	topLevel := &libraryresolver.LibraryIndex{Footprints: map[string]libraryresolver.FootprintRecord{"Test:Top": {FootprintID: "Test:Top"}}}
	placementLevel := &libraryresolver.LibraryIndex{Footprints: map[string]libraryresolver.FootprintRecord{"Test:Placement": {FootprintID: "Test:Placement"}}}
	selection := ComponentSelectionEntry{InstanceID: "amp", Role: "output"}

	got := placementOptionsForCreate(CreateOptions{LibraryIndex: topLevel}, []ComponentSelectionEntry{selection})
	if got.LibraryIndex != topLevel || len(got.ComponentSelections) != 1 || got.ComponentSelections[0].Role != "output" {
		t.Fatalf("propagated placement options = %#v", got)
	}
	got = placementOptionsForCreate(CreateOptions{LibraryIndex: topLevel, Placement: PlacementOptions{LibraryIndex: placementLevel}}, nil)
	if got.LibraryIndex != placementLevel {
		t.Fatalf("explicit placement library index was replaced: %#v", got.LibraryIndex)
	}
}

func TestCreateValidationOptionsRunsRequiredDRCOnlyInCanonicalKiCadStage(t *testing.T) {
	request := Request{Validation: ValidationSpec{RequireDRC: true}}
	validation, checks := createValidationOptions(request, CreateOptions{
		Validation:  ValidationOptions{RequireDRC: true, KiCadCLI: "/test/kicad-cli"},
		KiCadChecks: KiCadCheckOptions{EnforceRequirements: true},
	})
	if validation.RequireDRC || validation.KiCadCLI != "" {
		t.Fatalf("structural validation retained duplicate DRC options: %#v", validation)
	}
	if !checks.RequireDRC || checks.KiCadCLI != "/test/kicad-cli" {
		t.Fatalf("canonical KiCad options = %#v", checks)
	}
}

func TestCreateWritesWorkflowResult(t *testing.T) {
	request := Request{
		Version:    RequestVersion,
		Name:       "status_board",
		Board:      BoardSpec{WidthMM: 40, HeightMM: 25, Layers: 2},
		Blocks:     []BlockInstanceSpec{{ID: "status", BlockID: "led_indicator"}},
		Validation: ValidationSpec{Acceptance: AcceptanceStructural, SkipRouting: true},
	}
	output := filepath.Join(t.TempDir(), "status_board")

	result := Create(context.Background(), request, CreateOptions{OutputDir: output})
	if result.Project.OutputDir != output {
		t.Fatalf("project = %#v", result.Project)
	}
	if len(result.Stages) == 0 {
		t.Fatalf("stages missing")
	}
	if result.Acceptance.Achieved == "" {
		t.Fatalf("acceptance = %#v feedback = %#v", result.Acceptance, result.Feedback)
	}
	if !hasStage(result, StageWriterCorrect) {
		t.Fatalf("writer correctness stage missing: %#v", result.Stages)
	}
	if !hasStage(result, StageSchematicElectrical) {
		t.Fatalf("schematic electrical stage missing: %#v", result.Stages)
	}
	componentStage, ok := stageByName(result, StageComponentSelection)
	if !ok {
		t.Fatalf("component selection stage missing: %#v", result.Stages)
	}
	if got := componentStage.Summary["selection_count"]; got != 2 {
		t.Fatalf("component selection count = %#v, want 2", got)
	}
	selected := selectedComponentsFromSummary(t, componentStage.Summary["selected_components"])
	if len(selected) == 0 {
		t.Fatal("expected at least one selected component")
	}
	componentID, _ := selected[0]["component_id"].(string)
	if componentID == "" {
		t.Fatalf("selected component missing component_id: %#v", selected[0])
	}
	schematicFile, err := schematic.ReadFile(filepath.Join(output, "status_board.kicad_sch"))
	if err != nil {
		t.Fatal(err)
	}
	if got := countSymbolsWithProperty(schematicFile.Symbols, componentprops.PropertyComponentID); got < 2 {
		t.Fatalf("component identity properties not propagated to schematic: %#v", schematicFile.Symbols)
	}
	if !hasSymbolWithProperties(schematicFile.Symbols, map[string]string{componentprops.PropertyComponentID: componentID}) {
		t.Fatalf("selected component identity not propagated to schematic: want component_id %q symbols %#v", componentID, schematicFile.Symbols)
	}
}

func TestSchematicElectricalStageBlocksInvalidGeneratedSchematic(t *testing.T) {
	operation := transactions.NewOperation(transactions.OpAddSymbol, []byte(`{"op":"add_symbol","ref":"R1","value":"10k","library_id":"Device:R","at":{"x_mm":10,"y_mm":10},"pins":[{"number":"1"},{"number":"2"}]}`))
	plan := BlockPlanResult{
		Request:     Request{Name: "bad_schematic"},
		Composition: blocks.CompositionRequest{ProjectName: "bad_schematic"},
		Output: blocks.CompositionOutput{
			Instances: []blocks.BlockInstance{
				{InstanceID: "a", Refs: []string{"R1"}},
				{InstanceID: "b", Refs: []string{"R1"}},
			},
			Operations: []transactions.Operation{operation},
		},
		Stage: NewStageResult(StageBlockPlanning, nil),
	}

	stage := SchematicElectricalStage(plan)

	if stage.Status != StageStatusBlocked {
		t.Fatalf("stage status = %q, want blocked: %#v", stage.Status, stage)
	}
	if stage.Summary["status"] != "blocked" {
		t.Fatalf("summary = %#v", stage.Summary)
	}
	if !hasIssueCode(stage.Issues, reports.CodeValidationFailed) {
		t.Fatalf("expected validation issue: %#v", stage.Issues)
	}
}

func TestSchematicElectricalStageBlocksUnresolvedEndpoint(t *testing.T) {
	addSymbol := transactions.NewOperation(transactions.OpAddSymbol, []byte(`{"op":"add_symbol","ref":"R1","value":"10k","library_id":"Device:R","at":{"x_mm":10,"y_mm":10},"pins":[{"number":"1"},{"number":"2"}]}`))
	connect := transactions.NewOperation(transactions.OpConnect, []byte(`{"op":"connect","from":{"ref":"R1","pin":"1"},"to":{"ref":"R2","pin":"1"},"net_name":"SIG"}`))
	plan := BlockPlanResult{
		Request:     Request{Name: "bad_endpoint"},
		Composition: blocks.CompositionRequest{ProjectName: "bad_endpoint"},
		Output: blocks.CompositionOutput{
			Instances:  []blocks.BlockInstance{{InstanceID: "a", Refs: []string{"R1", "R2"}}},
			Operations: []transactions.Operation{addSymbol, connect},
		},
		Stage: NewStageResult(StageBlockPlanning, nil),
	}

	stage := SchematicElectricalStage(plan)

	if stage.Status != StageStatusBlocked {
		t.Fatalf("stage status = %q, want blocked: %#v", stage.Status, stage)
	}
	if len(stage.Issues) != 1 || stage.Issues[0].Path != "operations[2].to" {
		t.Fatalf("unexpected endpoint issues: %#v", stage.Issues)
	}
}

func TestSchematicElectricalStageAllowsProtectedUSBCPower(t *testing.T) {
	registry := blocks.NewBuiltinRegistry()
	output, issues := registry.Instantiate(context.Background(), blocks.BlockRequest{
		BlockID:    "usb_c_power",
		InstanceID: "usb",
		Params: map[string]any{
			"include_bulk_capacitor": true,
			"include_fuse":           true,
			"include_power_led":      false,
			"include_tvs":            true,
		},
	})
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("instantiate issues = %#v", issues)
	}
	tx, err := blocks.ProjectTransactionForBlockOutput("usb_power", output, false)
	if err != nil {
		t.Fatal(err)
	}
	file, opts, inputIssues := schematicElectricalInputsFromTransaction(tx)
	if len(inputIssues) != 0 {
		t.Fatalf("schematic electrical input issues = %#v", inputIssues)
	}
	report := schematicrules.Inspect(file, opts)
	if report.Status != schematicrules.StatusClean {
		t.Fatalf("protected USB-C schematic electrical status = %s findings = %#v labels = %#v wires = %#v", report.Status, report.Findings, file.Labels, file.Wires)
	}
}

func TestSchematicElectricalWireAvoidsUnrelatedNoConnectAnchor(t *testing.T) {
	candidate := schematicElectricalWireCandidate{
		NetName: "VCC",
		From:    kicadfiles.Point{X: kicadfiles.MM(0), Y: kicadfiles.MM(10)},
		To:      kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(-10)},
	}
	noConnect := kicadfiles.Point{X: kicadfiles.MM(10), Y: 0}
	wire := schematicElectricalWireForCandidate(candidate, nil, []kicadfiles.Point{candidate.From, candidate.To, noConnect})
	for index := 1; index < len(wire.Points); index++ {
		if schematicElectricalPointOnSegment(noConnect, wire.Points[index-1], wire.Points[index]) {
			t.Fatalf("wire %#v crosses unrelated no-connect anchor %#v", wire.Points, noConnect)
		}
	}
}

func TestSchematicElectricalSafeWiresDoNotInventCrossNetShort(t *testing.T) {
	candidates := []schematicElectricalWireCandidate{
		{NetName: "NET_A", From: kicadfiles.Point{X: kicadfiles.MM(0), Y: kicadfiles.MM(10)}, To: kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(10)}},
		{NetName: "NET_B", From: kicadfiles.Point{X: kicadfiles.MM(10), Y: kicadfiles.MM(0)}, To: kicadfiles.Point{X: kicadfiles.MM(10), Y: kicadfiles.MM(20)}},
	}
	obstacles := []kicadfiles.Point{candidates[0].From, candidates[0].To, candidates[1].From, candidates[1].To}
	wires := schematicElectricalSafeWires(candidates, nil, obstacles)
	if len(wires) != 1 {
		t.Fatalf("safe wires = %#v, want only the first collision-free representative", wires)
	}
	if wires[0].Points[0] != candidates[0].From || wires[0].Points[len(wires[0].Points)-1] != candidates[0].To {
		t.Fatalf("retained wire = %#v, want NET_A endpoints", wires[0].Points)
	}
}

func TestSchematicElectricalInputsRespectLabelOnlyConnections(t *testing.T) {
	addA := transactions.NewOperation(transactions.OpAddSymbol, []byte(`{"op":"add_symbol","ref":"A1","value":"A","library_id":"Connector:Test","at":{"x_mm":10,"y_mm":10},"pins":[{"number":"1"}]}`))
	addB := transactions.NewOperation(transactions.OpAddSymbol, []byte(`{"op":"add_symbol","ref":"B1","value":"B","library_id":"Connector:Test","at":{"x_mm":20,"y_mm":20},"pins":[{"number":"1"}]}`))
	connect := transactions.NewOperation(transactions.OpConnect, []byte(`{"op":"connect","from":{"ref":"A1","pin":"1"},"to":{"ref":"B1","pin":"1"},"net_name":"SIG","use_labels":true,"skip_from_label":true}`))

	file, _, issues := schematicElectricalInputsFromTransaction(transactions.Transaction{Operations: []transactions.Operation{addA, addB, connect}})
	if len(issues) != 0 {
		t.Fatalf("input issues = %#v", issues)
	}
	if len(file.Wires) != 0 {
		t.Fatalf("label-only connection emitted synthetic wires: %#v", file.Wires)
	}
	expectedPosition := kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(20)}
	if len(file.Labels) != 1 || file.Labels[0].Text != "SIG" || file.Labels[0].Position != expectedPosition {
		t.Fatalf("label-only connection labels = %#v", file.Labels)
	}
}

func TestSchematicElectricalInputsResolveSharedReferenceByUnit(t *testing.T) {
	addA := transactions.NewOperation(transactions.OpAddSymbol, []byte(`{"op":"add_symbol","ref":"U1","unit":1,"value":"LM358","library_id":"Amplifier_Operational:LM358","at":{"x_mm":10,"y_mm":10},"pins":[{"number":"1","x_mm":2.54}]}`))
	addB := transactions.NewOperation(transactions.OpAddSymbol, []byte(`{"op":"add_symbol","ref":"U1","unit":2,"value":"LM358","library_id":"Amplifier_Operational:LM358","at":{"x_mm":20,"y_mm":10},"pins":[{"number":"7","x_mm":2.54}]}`))
	connect := transactions.NewOperation(transactions.OpConnect, []byte(`{"op":"connect","from":{"ref":"U1","pin":"1","unit":1},"to":{"ref":"U1","pin":"7","unit":2},"net_name":"BUFFERED"}`))

	file, _, issues := schematicElectricalInputsFromTransaction(transactions.Transaction{Operations: []transactions.Operation{addA, addB, connect}})
	if len(issues) != 0 {
		t.Fatalf("multi-unit input issues = %#v", issues)
	}
	if len(file.Symbols) != 2 || file.Symbols[0].Unit != 1 || file.Symbols[1].Unit != 2 {
		t.Fatalf("multi-unit symbols = %#v", file.Symbols)
	}
	if len(file.Wires) != 1 || len(file.Wires[0].Points) < 2 || file.Wires[0].Points[0] == file.Wires[0].Points[len(file.Wires[0].Points)-1] {
		t.Fatalf("multi-unit wire = %#v", file.Wires)
	}

	wrongUnit := transactions.NewOperation(transactions.OpConnect, []byte(`{"op":"connect","from":{"ref":"U1","pin":"1","unit":2},"to":{"ref":"U1","pin":"7","unit":2},"net_name":"WRONG"}`))
	_, _, issues = schematicElectricalInputsFromTransaction(transactions.Transaction{Operations: []transactions.Operation{addA, addB, wrongUnit}})
	if len(issues) != 1 || issues[0].Path != "operations[2].from" {
		t.Fatalf("wrong-unit issues = %#v", issues)
	}
}

func TestSchematicElectricalInputsPreserveBusLabelAnchor(t *testing.T) {
	addBus := transactions.NewOperation(transactions.OpAddBus, []byte(`{"op":"add_bus","points":[{"x_mm":10,"y_mm":10},{"x_mm":30,"y_mm":10}]}`))
	addLabel := transactions.NewOperation(transactions.OpAddLabel, []byte(`{"op":"add_label","text":"I2C","at":{"x_mm":10,"y_mm":10},"kind":"local"}`))

	file, opts, issues := schematicElectricalInputsFromTransaction(transactions.Transaction{Operations: []transactions.Operation{addBus, addLabel}})
	if len(issues) != 0 {
		t.Fatalf("input issues = %#v", issues)
	}
	if len(file.Buses) != 1 {
		t.Fatalf("buses = %#v, want one preserved bus", file.Buses)
	}
	report := schematicrules.Inspect(file, opts)
	for _, finding := range report.Findings {
		if finding.RuleID == schematicrules.RuleLabelFloating {
			t.Fatalf("bus label should be anchored: %#v", report.Findings)
		}
	}
}

func TestCreateStructuralRequestSkipsFabricationReadiness(t *testing.T) {
	request := Request{
		Version:    RequestVersion,
		Name:       "structural_board",
		Board:      BoardSpec{WidthMM: 40, HeightMM: 25, Layers: 2},
		Blocks:     []BlockInstanceSpec{{ID: "status", BlockID: "led_indicator"}},
		Validation: ValidationSpec{Acceptance: AcceptanceStructural, SkipRouting: true},
	}
	result := Create(context.Background(), request, CreateOptions{OutputDir: filepath.Join(t.TempDir(), "structural_board")})
	if hasStage(result, StageFabricationReady) {
		t.Fatalf("fabrication readiness stage should not run for structural request: %#v", result.Stages)
	}
}

func TestFabricationReadinessStageBlocksMissingPackageEvidence(t *testing.T) {
	request := Request{Validation: ValidationSpec{Acceptance: AcceptanceFabricationCandidate}}
	written := ProjectWriteResult{Inspection: inspect.ProjectSummary{Root: t.TempDir()}}
	stage := FabricationReadinessStage(context.Background(), &request, &written)
	if stage.Name != StageFabricationReady {
		t.Fatalf("stage name = %q", stage.Name)
	}
	if stage.Status != StageStatusBlocked {
		t.Fatalf("stage = %#v, want blocked", stage)
	}
	if !hasIssueCode(stage.Issues, reports.CodeValidationFailed) {
		t.Fatalf("expected readiness issue in %#v", stage.Issues)
	}
	if stage.Summary["dry_run"] != true {
		t.Fatalf("summary = %#v, want dry run", stage.Summary)
	}
}

func TestFabricationReadinessStagePackagePathRetainsPreflightFailure(t *testing.T) {
	request := Request{Validation: ValidationSpec{Acceptance: AcceptanceFabricationCandidate}}
	written := ProjectWriteResult{Inspection: inspect.ProjectSummary{Root: t.TempDir()}}
	stage := fabricationReadinessStageWithOptions(context.Background(), &request, &written, &fabrication.Options{})
	if stage.Status != StageStatusBlocked || stage.Summary["dry_run"] != true {
		t.Fatalf("stage = %#v, want package path to retain its blocked dry-run preflight", stage)
	}
}

func TestFabricationReadinessStageSummarizesPhysicalRules(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "demo.kicad_pro"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeWorkflowTestPCB(t, filepath.Join(root, "demo.kicad_pcb"))
	request := Request{Validation: ValidationSpec{Acceptance: AcceptanceFabricationCandidate}}
	written := ProjectWriteResult{Inspection: inspect.ProjectSummary{Root: root}}

	stage := FabricationReadinessStage(context.Background(), &request, &written)

	physical, ok := stage.Summary["physical_rules"].(map[string]any)
	if !ok {
		t.Fatalf("physical_rules summary missing or wrong type: %#v", stage.Summary)
	}
	if physical["status"] == "" {
		t.Fatalf("physical_rules status missing: %#v", physical)
	}
	if physical["report_path"] != "fabrication/physical-rules.json" {
		t.Fatalf("physical_rules report_path = %#v", physical["report_path"])
	}
	if _, ok := physical["blocker_count"].(int); !ok {
		t.Fatalf("physical_rules blocker_count missing: %#v", physical)
	}
}

func TestFabricationBlockReadinessReportRequiresVerifiedEvidence(t *testing.T) {
	verified := StageResult{Summary: map[string]any{"block_evidence": []BlockEvidenceSummary{
		{InstanceID: "gain", BlockID: "speaker_opamp_driver", Status: "verified"},
		{InstanceID: "output", BlockID: "class_ab_speaker_power_stage", Status: "verified"},
	}}}
	data, err := fabricationBlockReadinessReport(verified)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Fatal("verified block evidence did not produce readiness report")
	}
	var report struct {
		Status string `json:"status"`
		Gates  []struct {
			Status string `json:"status"`
		} `json:"gates"`
	}
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatal(err)
	}
	if report.Status != "pass" || len(report.Gates) != 2 || report.Gates[0].Status != "pass" || report.Gates[1].Status != "pass" {
		t.Fatalf("readiness report = %#v", report)
	}

	missing := verified
	missing.Summary = map[string]any{"block_evidence": []BlockEvidenceSummary{{BlockID: "speaker_opamp_driver", Status: "missing"}}}
	if data, err := fabricationBlockReadinessReport(missing); err != nil || len(data) != 0 {
		t.Fatalf("missing evidence produced readiness report: %s", data)
	}
}

func TestWorkflowIssueAndArtifactCollectors(t *testing.T) {
	result := WorkflowResult{Stages: []StageResult{{
		Issues:    []reports.Issue{{Code: reports.CodeValidationFailed, Severity: reports.SeverityWarning, Message: "warn"}},
		Artifacts: []reports.Artifact{{Kind: reports.ArtifactKiCadProject, Path: "out/demo.kicad_pro"}},
	}}}
	if len(WorkflowIssues(result)) != 1 || len(WorkflowArtifacts(result)) != 1 {
		t.Fatalf("collectors failed")
	}
}

func writeWorkflowTestPCB(t *testing.T, path string) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	board := pcbfiles.PCBFile{
		Version:          kicadfiles.KiCadPCBFormatV20260206,
		Generator:        "kicadai-test",
		GeneratorVersion: "phase7",
		General:          pcbfiles.DefaultGeneral(),
		Paper:            kicadfiles.Paper{Name: "A4"},
		Layers:           pcbfiles.DefaultTwoLayerStack(),
		Setup:            pcbfiles.DefaultSetup(),
		Drawings: []pcbfiles.Drawing{{
			UUID:  kicadfiles.UUID("11111111-1111-4111-8111-111111111111"),
			Layer: kicadfiles.LayerEdge,
			Rect: &pcbfiles.RectDrawing{
				Start: kicadfiles.Point{X: kicadfiles.MM(0), Y: kicadfiles.MM(0)},
				End:   kicadfiles.Point{X: kicadfiles.MM(20), Y: kicadfiles.MM(15)},
				Width: kicadfiles.MM(0.1),
			},
		}},
	}
	if err := pcbfiles.Write(file, board); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

func hasStage(result WorkflowResult, name StageName) bool {
	for _, stage := range result.Stages {
		if stage.Name == name {
			return true
		}
	}
	return false
}

func countSymbolsWithProperty(symbols []schematic.SchematicSymbol, name string) int {
	count := 0
	for _, symbol := range symbols {
		for _, property := range symbol.Properties {
			if property.Name == name && property.Value != "" {
				count++
				break
			}
		}
	}
	return count
}

func hasSymbolWithProperties(symbols []schematic.SchematicSymbol, want map[string]string) bool {
	for _, symbol := range symbols {
		values := map[string]string{}
		for _, property := range symbol.Properties {
			values[property.Name] = property.Value
		}
		matches := true
		for name, value := range want {
			if values[name] != value {
				matches = false
				break
			}
		}
		if matches {
			return true
		}
	}
	return false
}

func selectedComponentsFromSummary(t *testing.T, value any) []map[string]any {
	t.Helper()
	if selected, ok := value.([]map[string]any); ok {
		return selected
	}
	items, ok := value.([]any)
	if !ok {
		t.Fatalf("selected component summary type = %T", value)
	}
	selected := make([]map[string]any, 0, len(items))
	for _, item := range items {
		entry, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("selected component entry type = %T", item)
		}
		selected = append(selected, entry)
	}
	return selected
}

func stageByName(result WorkflowResult, name StageName) (StageResult, bool) {
	for _, stage := range result.Stages {
		if stage.Name == name {
			return stage, true
		}
	}
	return StageResult{}, false
}

func requireSelectedProcurement(t *testing.T, selected []map[string]any, role string, manufacturer string, mpn string) {
	t.Helper()
	for _, item := range selected {
		if item["role"] != role {
			continue
		}
		if item["manufacturer"] != manufacturer || item["mpn"] != mpn {
			t.Fatalf("selected %s identity = %#v", role, item)
		}
		procurement, ok := item["procurement"].(*components.ProcurementEvidence)
		if !ok {
			t.Fatalf("selected %s procurement evidence missing: %#v", role, item)
		}
		if procurement.LifecycleStatus != components.LifecycleActive || procurement.AvailabilityStatus != components.AvailabilityNotChecked {
			t.Fatalf("selected %s procurement = %#v", role, procurement)
		}
		return
	}
	t.Fatalf("selected role %s missing from %#v", role, selected)
}

func hasIssueCode(issues []reports.Issue, code reports.Code) bool {
	for _, issue := range issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}

func hasSelectedComponentID(selected []map[string]any, id string) bool {
	for _, item := range selected {
		componentID, ok := item["component_id"].(string)
		if ok && componentID == id {
			return true
		}
	}
	return false
}

func TestCreateShortCircuitsAfterPlanFailure(t *testing.T) {
	request := Request{
		Version: RequestVersion,
		Name:    "bad",
		Board:   BoardSpec{WidthMM: 40, HeightMM: 25, Layers: 2},
		Blocks:  []BlockInstanceSpec{{ID: "missing", BlockID: "does_not_exist"}},
	}
	result := Create(context.Background(), request, CreateOptions{OutputDir: filepath.Join(t.TempDir(), "bad")})
	if result.Stages[0].Status != StageStatusBlocked {
		t.Fatalf("plan stage = %#v", result.Stages[0])
	}
	for _, stage := range result.Stages[2:] {
		if stage.Status != StageStatusSkipped {
			t.Fatalf("stage %s = %#v, want skipped", stage.Name, stage)
		}
	}
}

func TestCreateComponentSelectionFailureBlocksBeforeWrite(t *testing.T) {
	output := filepath.Join(t.TempDir(), "blocked")
	request := Request{
		Version:    RequestVersion,
		Name:       "blocked",
		Board:      BoardSpec{WidthMM: 40, HeightMM: 25, Layers: 2},
		Components: ComponentPolicySpec{CatalogDir: t.TempDir()},
		Blocks:     []BlockInstanceSpec{{ID: "status", BlockID: "led_indicator"}},
		Validation: ValidationSpec{Acceptance: AcceptanceStructural, SkipRouting: true},
	}
	result := Create(context.Background(), request, CreateOptions{OutputDir: output})
	stage, ok := stageByName(result, StageComponentSelection)
	if !ok {
		t.Fatalf("component selection stage missing: %#v", result.Stages)
	}
	if stage.Status != StageStatusBlocked {
		t.Fatalf("component stage = %#v, want blocked", stage)
	}
	projectWrite, ok := stageByName(result, StageProjectWrite)
	if !ok || projectWrite.Status != StageStatusSkipped {
		t.Fatalf("project write stage = %#v ok=%v, want skipped", projectWrite, ok)
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("output dir stat err = %v, want not exist", err)
	}
}

func TestCreateDraftComponentPolicySelectsVerifiedOpAmp(t *testing.T) {
	request := Request{
		Version: RequestVersion,
		Name:    "draft_opamp",
		Board:   BoardSpec{WidthMM: 60, HeightMM: 35, Layers: 2},
		Blocks:  []BlockInstanceSpec{{ID: "gain", BlockID: "opamp_gain_stage"}},
		Components: ComponentPolicySpec{
			Acceptance: components.AcceptanceDraft,
		},
		Validation: ValidationSpec{Acceptance: AcceptanceDraft, SkipRouting: true},
	}
	result := Create(context.Background(), request, CreateOptions{OutputDir: filepath.Join(t.TempDir(), "draft")})
	stage, ok := stageByName(result, StageComponentSelection)
	if !ok {
		t.Fatalf("component selection stage missing: %#v", result.Stages)
	}
	if stage.Status == StageStatusBlocked {
		t.Fatalf("draft component stage blocked: %#v", stage)
	}
	selected := selectedComponentsFromSummary(t, stage.Summary["selected_components"])
	if !hasSelectedComponentID(selected, "opamp.ti.lmv321.sot23_5") {
		t.Fatalf("expected verified LMV321 selection in %#v", selected)
	}
}

func TestCreateConnectivityAllowsVerifiedOpAmpSelection(t *testing.T) {
	output := filepath.Join(t.TempDir(), "connectivity")
	request := Request{
		Version:    RequestVersion,
		Name:       "connectivity_opamp",
		Board:      BoardSpec{WidthMM: 60, HeightMM: 35, Layers: 2},
		Blocks:     []BlockInstanceSpec{{ID: "gain", BlockID: "opamp_gain_stage"}},
		Validation: ValidationSpec{Acceptance: AcceptanceStructural, SkipRouting: true},
	}
	result := Create(context.Background(), request, CreateOptions{OutputDir: output})
	stage, ok := stageByName(result, StageComponentSelection)
	if !ok {
		t.Fatalf("component selection stage missing: %#v", result.Stages)
	}
	if stage.Status == StageStatusBlocked {
		t.Fatalf("component stage blocked unexpectedly: %#v", stage)
	}
	selected := selectedComponentsFromSummary(t, stage.Summary["selected_components"])
	if !hasSelectedComponentID(selected, "opamp.ti.lmv321.sot23_5") {
		t.Fatalf("expected verified LMV321 selection in %#v", selected)
	}
}

func TestCreateConnectivityRejectsPlaceholderActiveComponent(t *testing.T) {
	output := filepath.Join(t.TempDir(), "connectivity_placeholder")
	catalogDir := writeUnsafeOpAmpCatalog(t)
	request := Request{
		Version:    RequestVersion,
		Name:       "connectivity_placeholder",
		Board:      BoardSpec{WidthMM: 60, HeightMM: 35, Layers: 2},
		Components: ComponentPolicySpec{CatalogDir: catalogDir},
		Blocks:     []BlockInstanceSpec{{ID: "gain", BlockID: "opamp_gain_stage"}},
		Validation: ValidationSpec{Acceptance: AcceptanceStructural, SkipRouting: true},
	}
	result := Create(context.Background(), request, CreateOptions{OutputDir: output})
	stage, ok := stageByName(result, StageComponentSelection)
	if !ok {
		t.Fatalf("component selection stage missing: %#v", result.Stages)
	}
	if stage.Status != StageStatusBlocked {
		t.Fatalf("component stage = %#v, want blocked", stage)
	}
	if !hasIssueCode(stage.Issues, components.CodeComponentUnsafe) {
		t.Fatalf("expected unsafe component issue in %#v", stage.Issues)
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("output dir stat err = %v, want not exist", err)
	}
}

func writeUnsafeOpAmpCatalog(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	data, err := json.Marshal(map[string]any{
		"version":  "0.1.0",
		"families": []map[string]any{{"id": "opamp", "name": "Operational Amplifier"}},
		"records": []map[string]any{{
			"id":      "opamp.ti.lmv321.sot23_5",
			"family":  "opamp",
			"name":    "Placeholder LMV321",
			"generic": true,
			"symbols": []map[string]any{{
				"symbol_id":    "Amplifier_Operational:LMV321",
				"verification": map[string]any{"confidence": "placeholder"},
			}},
			"packages": []map[string]any{{
				"id":           "sot23_5",
				"footprint_id": "Package_TO_SOT_SMD:SOT-23-5",
				"verification": map[string]any{"confidence": "placeholder"},
			}},
			"verification": map[string]any{"confidence": "placeholder"},
		}},
	})
	if err != nil {
		t.Fatalf("marshal unsafe opamp catalog: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "catalog.json"), data, 0o644); err != nil {
		t.Fatalf("write unsafe opamp catalog: %v", err)
	}
	return dir
}

func TestCreateComponentSelectionSummaryCarriesMetadata(t *testing.T) {
	request := Request{
		Version:    RequestVersion,
		Name:       "metadata",
		Board:      BoardSpec{WidthMM: 40, HeightMM: 25, Layers: 2},
		Blocks:     []BlockInstanceSpec{{ID: "status", BlockID: "led_indicator"}},
		Validation: ValidationSpec{Acceptance: AcceptanceStructural, SkipRouting: true},
	}
	result := Create(context.Background(), request, CreateOptions{OutputDir: filepath.Join(t.TempDir(), "metadata")})
	stage, ok := stageByName(result, StageComponentSelection)
	if !ok {
		t.Fatalf("component selection stage missing: %#v", result.Stages)
	}
	selected, ok := stage.Summary["selected_components"].([]map[string]any)
	if !ok {
		t.Fatalf("selected component summary type = %T", stage.Summary["selected_components"])
	}
	if len(selected) != 2 {
		t.Fatalf("selected components = %#v", selected)
	}
	if selected[0]["component_id"] == "" || selected[0]["footprint_id"] == "" {
		t.Fatalf("selected component metadata incomplete: %#v", selected)
	}
	if _, ok := selected[0]["pinmap_checked"].(bool); !ok {
		t.Fatalf("selected component evidence missing pinmap flag: %#v", selected[0])
	}
	if _, ok := selected[0]["rejected_count"].(int); !ok {
		t.Fatalf("selected component evidence missing rejected count: %#v", selected[0])
	}
}

func TestCreateComponentSelectionCarriesProcurementEvidence(t *testing.T) {
	request := Request{
		Version: RequestVersion,
		Name:    "sourced_regulator",
		Board:   BoardSpec{WidthMM: 45, HeightMM: 30, Layers: 2},
		Blocks: []BlockInstanceSpec{{
			ID:      "rail",
			BlockID: "voltage_regulator",
			Params: map[string]any{
				"regulator_symbol":    "Regulator_Linear:AP2112K-3.3",
				"regulator_footprint": "Package_TO_SOT_SMD:SOT-23-5",
				"enable_mode":         "tied_input",
			},
		}},
		Validation: ValidationSpec{Acceptance: AcceptanceStructural, SkipRouting: true},
	}
	outputDir := filepath.Join(t.TempDir(), "sourced_regulator")
	result := Create(context.Background(), request, CreateOptions{
		OutputDir: outputDir,
		Components: ComponentSelectionOptions{
			SourceDir: componentSourceFixtureDir(t, "valid"),
		},
	})
	stage, ok := stageByName(result, StageComponentSelection)
	if !ok {
		t.Fatalf("component selection stage missing: %#v", result.Stages)
	}
	if stage.Status == StageStatusBlocked {
		t.Fatalf("component selection blocked: %#v", stage)
	}
	procurement, ok := stage.Summary["procurement"].(map[string]any)
	if !ok {
		t.Fatalf("procurement summary missing: %#v", stage.Summary)
	}
	if procurement["lifecycle_evidence_count"] != 3 {
		t.Fatalf("procurement summary = %#v, want three lifecycle evidence rows", procurement)
	}
	selected, ok := stage.Summary["selected_components"].([]map[string]any)
	if !ok {
		t.Fatalf("selected components type = %T", stage.Summary["selected_components"])
	}
	found := false
	foundOutputCapEvidence := false
	for _, item := range selected {
		if item["component_id"] != "regulator.linear.ap2112k_3v3.sot23_5" {
			if item["component_id"] == "capacitor.murata.grm21br61a106ke19l.0805" {
				evidence, ok := item["capacitor_evidence"].(map[string]any)
				if !ok {
					t.Fatalf("selected output capacitor evidence missing: %#v", item)
				}
				if evidence["dielectric"] != "X5R" || evidence["effective_capacitance_review"] != "review_required" {
					t.Fatalf("output capacitor evidence = %#v", evidence)
				}
				foundOutputCapEvidence = true
			}
			continue
		}
		procurement, ok := item["procurement"].(*components.ProcurementEvidence)
		if !ok {
			t.Fatalf("selected AP2112 procurement evidence missing: %#v", item)
		}
		if procurement.LifecycleStatus != components.LifecycleActive || procurement.AvailabilityStatus != components.AvailabilityNotChecked {
			t.Fatalf("procurement = %#v", procurement)
		}
		evidence, ok := item["regulator_evidence"].(map[string]any)
		if !ok {
			t.Fatalf("selected AP2112 regulator evidence missing: %#v", item)
		}
		outputCap, ok := evidence["output_capacitor"].(map[string]any)
		if !ok {
			t.Fatalf("selected AP2112 output-cap evidence missing: %#v", evidence)
		}
		if outputCap["kind"] != "ceramic_stable" || outputCap["proof_status"] != "review_required" {
			t.Fatalf("AP2112 output-cap evidence = %#v", outputCap)
		}
		if item["placement_hint_count"] != 2 || item["routing_hint_count"] != 3 {
			t.Fatalf("AP2112 hint counts missing: %#v", item)
		}
		found = true
	}
	if !found {
		t.Fatalf("AP2112 selection missing from %#v", selected)
	}
	if !foundOutputCapEvidence {
		t.Fatalf("10uF capacitor evidence missing from %#v", selected)
	}
}

func TestCreateComponentSelectionUsesConcreteAlternatives(t *testing.T) {
	request := Request{
		Version: RequestVersion,
		Name:    "sourced_reset",
		Board:   BoardSpec{WidthMM: 40, HeightMM: 25, Layers: 2},
		Blocks: []BlockInstanceSpec{{
			ID:      "prog",
			BlockID: "reset_programming_header",
		}},
		Validation: ValidationSpec{Acceptance: AcceptanceConnectivity, SkipRouting: true},
	}
	result := Create(context.Background(), request, CreateOptions{
		OutputDir: filepath.Join(t.TempDir(), "sourced_reset"),
		Components: ComponentSelectionOptions{
			SourceDir: componentSourceFixtureDir(t, "valid"),
		},
	})
	stage, ok := stageByName(result, StageComponentSelection)
	if !ok {
		t.Fatalf("component selection stage missing: %#v", result.Stages)
	}
	if stage.Status == StageStatusBlocked {
		t.Fatalf("component selection blocked: %#v", stage)
	}
	selected, ok := stage.Summary["selected_components"].([]map[string]any)
	if !ok {
		t.Fatalf("selected components type = %T", stage.Summary["selected_components"])
	}
	requireSelectedProcurement(t, selected, "reset_pullup", "Yageo", "RC0805FR-0710KL")
}

func TestCreateComponentSelectionStaleLifecycleWarnsForConnectivity(t *testing.T) {
	request := sourcedAP2112WorkflowRequest(t, AcceptanceConnectivity, "stale")
	result := Create(context.Background(), request, CreateOptions{
		OutputDir: filepath.Join(t.TempDir(), "stale_connectivity"),
		Components: ComponentSelectionOptions{
			SourceDir: componentSourceFixtureDir(t, "stale"),
		},
	})
	stage, ok := stageByName(result, StageComponentSelection)
	if !ok {
		t.Fatalf("component selection stage missing: %#v", result.Stages)
	}
	if stage.Status == StageStatusBlocked {
		t.Fatalf("connectivity stale lifecycle should warn, not block: %#v", stage)
	}
	if !hasIssueCode(stage.Issues, components.CodeComponentLifecycleStale) {
		t.Fatalf("expected stale lifecycle warning in %#v", stage.Issues)
	}
}

func TestCreateComponentSelectionStaleLifecycleBlocksFabricationCandidate(t *testing.T) {
	request := sourcedAP2112WorkflowRequest(t, AcceptanceFabricationCandidate, "stale")
	result := Create(context.Background(), request, CreateOptions{
		OutputDir: filepath.Join(t.TempDir(), "stale_fab"),
		Components: ComponentSelectionOptions{
			SourceDir: componentSourceFixtureDir(t, "stale"),
		},
	})
	stage, ok := stageByName(result, StageComponentSelection)
	if !ok {
		t.Fatalf("component selection stage missing: %#v", result.Stages)
	}
	if stage.Status != StageStatusBlocked {
		t.Fatalf("fabrication stale lifecycle stage = %#v, want blocked", stage)
	}
	if !hasIssueCode(stage.Issues, components.CodeComponentLifecycleStale) {
		t.Fatalf("expected stale lifecycle blocker in %#v", stage.Issues)
	}
}

func sourcedAP2112WorkflowRequest(t *testing.T, acceptance AcceptanceLevel, sourceFixture string) Request {
	t.Helper()
	_ = sourceFixture
	return Request{
		Version: RequestVersion,
		Name:    "sourced_ap2112",
		Board:   BoardSpec{WidthMM: 45, HeightMM: 30, Layers: 2},
		Blocks: []BlockInstanceSpec{{
			ID:      "rail",
			BlockID: "voltage_regulator",
			Params: map[string]any{
				"regulator_symbol":    "Regulator_Linear:AP2112K-3.3",
				"regulator_footprint": "Package_TO_SOT_SMD:SOT-23-5",
				"enable_mode":         "tied_input",
			},
		}},
		Validation: ValidationSpec{Acceptance: acceptance, SkipRouting: true},
	}
}

func componentSourceFixtureDir(t *testing.T, name string) string {
	t.Helper()
	dir := filepath.Join("..", "components", "testdata", "sources", name)
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		t.Fatalf("component source fixture not found: %s", dir)
	}
	return dir
}
