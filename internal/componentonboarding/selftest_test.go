package componentonboarding

import (
	"context"
	"encoding/json"
	"kicadai/internal/circuitgraph"
	"kicadai/internal/evidencebundle"
	"kicadai/internal/libraryresolver"
	"kicadai/internal/modelprovenance"
	"kicadai/internal/simmodel"
	"os"
	"path/filepath"
	"testing"
)

func TestComponentSelfTestExecutesCandidateAndKeepsQuarantine(t *testing.T) {
	base, requirement, documents, extraction, libraries := onboardingFixture(t)
	candidate, err := Onboard(context.Background(), requirement, documents, fixedExtractor{extraction}, base, libraries)
	if err != nil {
		t.Fatal(err)
	}
	// Populate test-library identities from the existing base records. Production
	// self-tests require the caller's actual indexed KiCad libraries instead.
	for _, record := range base.Records {
		for _, symbol := range record.Symbols {
			entry := libraryresolver.SymbolRecord{LibraryID: symbol.SymbolID}
			for _, pin := range symbol.FunctionPins {
				entry.Pins = append(entry.Pins, libraryresolver.SymbolPin{Number: pin.SymbolPin, Name: pin.Function})
			}
			if _, ok := libraries.Symbols[symbol.SymbolID]; !ok {
				libraries.Symbols[symbol.SymbolID] = entry
			}
		}
		for _, variant := range record.Packages {
			entry := libraryresolver.FootprintRecord{FootprintID: variant.FootprintID}
			for _, pad := range variant.PadFunctions {
				entry.Pads = append(entry.Pads, libraryresolver.FootprintPad{Name: pad.Pad, PinFunction: pad.Function})
			}
			if _, ok := libraries.Footprints[variant.FootprintID]; !ok {
				libraries.Footprints[variant.FootprintID] = entry
			}
		}
	}
	raw, err := os.ReadFile(filepath.Join("..", "..", "examples", "ai", "generic_mna_buffered_two_pole", "recorded-response.json"))
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		Intent circuitgraph.Document `json:"intent"`
	}
	if err = json.Unmarshal(raw, &envelope); err != nil {
		t.Fatal(err)
	}
	graph := envelope.Intent
	for i := range graph.Components {
		if graph.Components[i].ComponentID == "opamp.ti.lmv321.sot23_5" {
			graph.Components[i].ComponentID = candidate.SelectedID
			graph.Components[i].Manufacturer = candidate.Proposals[0].Record.Manufacturer
			graph.Components[i].MPN = candidate.Proposals[0].Record.MPN
		}
	}
	ids := map[string]bool{}
	analyses := []simmodel.Analysis{}
	for _, analysis := range graph.Simulation.Analyses {
		if analysis.Kind == simmodel.AnalysisDCOperatingPoint {
			analyses = append(analyses, analysis)
			ids[analysis.ID] = true
		}
	}
	graph.Simulation.Analyses = analyses
	assertions := []simmodel.Assertion{}
	for _, assertion := range graph.Simulation.Assertions {
		if ids[assertion.AnalysisID] {
			assertions = append(assertions, assertion)
		}
	}
	graph.Simulation.Assertions = assertions
	models, diagnostics := modelprovenance.LoadDefault()
	if len(diagnostics) > 0 {
		t.Fatal(diagnostics)
	}
	kit := TestKit{Schema: TestKitSchema, CandidateSHA256: candidate.Hash, Fixtures: []RegressionFixture{{ID: "buffer-dc", Graph: graph}}}
	out := filepath.Join(t.TempDir(), "self-test")
	if requested := os.Getenv("KICADAI_SELF_TEST_EVIDENCE"); requested != "" {
		out = requested
	}
	report, err := SelfTest(context.Background(), candidate, documents, base, models, libraries, kit, out)
	if err != nil || report.Status != "passed" || len(report.CoveredComponents) != 1 || report.CandidateStatus != StatusQuarantined {
		t.Fatalf("self-test %v %+v", err, report)
	}
	if err = evidencebundle.Verify(out); err != nil {
		t.Fatal(err)
	}
	if err = VerifySelfTest(out); err != nil {
		t.Fatal(err)
	}
	planBytes, err := evidencebundle.Read(out, "fixtures/buffer-dc/plan.json")
	if err != nil || evidencebundle.Digest(planBytes) != report.Fixtures[0].PlanSHA256 {
		t.Fatal("plan byte identity differs")
	}
	// Rehash deliberately corrupted candidates to prove semantic revalidation,
	// not just detection of a stale outer hash.
	for _, kind := range []string{"pin", "model", "source"} {
		t.Run(kind, func(t *testing.T) {
			raw, err := json.Marshal(candidate)
			if err != nil {
				t.Fatal(err)
			}
			var changed Candidate
			if err = json.Unmarshal(raw, &changed); err != nil {
				t.Fatal(err)
			}
			docs := append([]DocumentInput(nil), documents...)
			switch kind {
			case "pin":
				changed.Proposals[0].Record.Packages[0].PadFunctions[0].Pad = "999"
			case "model":
				changed.Proposals[0].Model.Provenance.Provenance.SHA256 = hashText("wrong model")
			case "source":
				docs[0].Content = []byte("wrong source")
			}
			changed.Hash, err = hashWithoutField(changed)
			if err != nil {
				t.Fatal(err)
			}
			changedKit := kit
			changedKit.CandidateSHA256 = changed.Hash
			result, err := SelfTest(context.Background(), changed, docs, base, models, libraries, changedKit, filepath.Join(t.TempDir(), "invalid"))
			if err == nil || result.CandidateValidation != "failed" {
				t.Fatalf("semantic corruption passed %v %+v", err, result)
			}
		})
	}
	kit.Fixtures[0].Graph.Simulation.Assertions[0].Min = 1e6
	kit.Fixtures[0].Graph.Simulation.Assertions[0].Max = 1e6 + 1
	out = filepath.Join(t.TempDir(), "failed")
	report, err = SelfTest(context.Background(), candidate, documents, base, models, libraries, kit, out)
	if err == nil || report.Status != "failed" {
		t.Fatal("failing simulation passed")
	}
	if err = evidencebundle.Verify(out); err != nil {
		t.Fatal(err)
	}
	kit.CandidateSHA256 = "stale"
	if _, err = SelfTest(context.Background(), candidate, documents, base, models, libraries, kit, filepath.Join(t.TempDir(), "stale")); err == nil {
		t.Fatal("stale kit accepted")
	}
}
