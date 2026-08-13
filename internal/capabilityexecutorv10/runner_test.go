package capabilityexecutorv10

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"kicadai/internal/capabilityevaluation"
	"kicadai/internal/capabilityfeedback"
	"kicadai/internal/components"
	"kicadai/internal/corpuspublication"
	"kicadai/internal/libraryresolver"
	"kicadai/internal/modelprovenance"
	"kicadai/internal/opentopologysynthesis"
)

func TestRunBuildsReproducibleEvidenceAcrossDistinctRoots(t *testing.T) {
	executor := testExecutor(capabilityfeedback.OutcomeUnsupported)
	request := testRequest(t, filepath.Join(t.TempDir(), "first"))
	first, err := executor.Run(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	request.OutputRoot = filepath.Join(t.TempDir(), "second")
	second, err := executor.Run(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if first.Hash != second.Hash || first.CaseCount != 24 || len(first.Cases) != 24 {
		t.Fatalf("reports differ or have wrong size: %s %s %d", first.Hash, second.Hash, len(first.Cases))
	}
	for _, current := range first.Cases {
		if current.Case.Outcome != "unsupported" || current.ReplaySHA256[0] != current.ReplaySHA256[1] ||
			current.ReplayRootSHA256[0] == current.ReplayRootSHA256[1] || len(current.Promotions) != 0 ||
			len(current.Case.Frontier) != 1 {
			t.Fatalf("invalid nonpass evidence for %s: %#v", current.Case.ID, current)
		}
	}
	for replay := 1; replay <= 2; replay++ {
		marker := filepath.Join(request.OutputRoot, "v10_case_001", fmt.Sprintf("replay-%d", replay), "CLEAN_ROOT.json")
		info, err := os.Stat(marker)
		if err != nil || info.Mode().Perm() != 0o444 {
			t.Fatalf("clean-root marker %d = %#v, %v", replay, info, err)
		}
	}
}

func TestRunPromotesEveryPassInBothOuterRoots(t *testing.T) {
	executor := testExecutor(capabilityfeedback.OutcomePass)
	var promotions atomic.Int64
	executor.promote = func(_ context.Context, _ opentopologysynthesis.SynthesisRun, _ opentopologysynthesis.SimulationEnvironment, _ opentopologysynthesis.PhysicalPromotionOptions) opentopologysynthesis.PhysicalPromotionResult {
		promotions.Add(1)
		return opentopologysynthesis.PhysicalPromotionResult{
			Status: opentopologysynthesis.PhysicalPromotionPassed, Hash: testDigest("promotion"),
			ProjectHash: testDigest("project"), ReplayIdentical: true,
			Runs: []opentopologysynthesis.PhysicalPromotionRun{
				{Number: 1, ProjectHash: testDigest("project")}, {Number: 2, ProjectHash: testDigest("project")},
			},
		}
	}
	report, err := executor.Run(context.Background(), testRequest(t, filepath.Join(t.TempDir(), "baseline")))
	if err != nil {
		t.Fatal(err)
	}
	if promotions.Load() != 48 {
		t.Fatalf("outer promotions = %d, want 48", promotions.Load())
	}
	for _, current := range report.Cases {
		if current.Case.Outcome != "pass" || len(current.Promotions) != 2 || current.Promotions[0].RunSHA256 != current.Promotions[1].RunSHA256 {
			t.Fatalf("invalid pass evidence for %s", current.Case.ID)
		}
	}
}

func TestRunExecutesFixedParallelCaseBatches(t *testing.T) {
	executor := testExecutor(capabilityfeedback.OutcomeUnsupported)
	started := make(chan struct{}, ParallelCaseLimit)
	release := make(chan struct{})
	var calls atomic.Int64
	var active atomic.Int64
	var maximum atomic.Int64
	executor.synthesize = func(_ context.Context, _ opentopologysynthesis.Requirement, _ opentopologysynthesis.PrimitiveInventory, _ opentopologysynthesis.SimulationEnvironment, _ opentopologysynthesis.Policy) opentopologysynthesis.SynthesisRun {
		current := active.Add(1)
		for prior := maximum.Load(); current > prior && !maximum.CompareAndSwap(prior, current); prior = maximum.Load() {
		}
		if calls.Add(1) <= ParallelCaseLimit {
			started <- struct{}{}
			<-release
		}
		active.Add(-1)
		return opentopologysynthesis.SynthesisRun{Hash: testDigest("parallel-synthesis"), Report: opentopologysynthesis.Report{Status: opentopologysynthesis.StatusUnsupported}}
	}
	done := make(chan error, 1)
	request := testRequest(t, filepath.Join(t.TempDir(), "parallel"))
	go func() {
		_, err := executor.Run(context.Background(), request)
		done <- err
	}()
	for index := 0; index < ParallelCaseLimit; index++ {
		select {
		case <-started:
		case <-time.After(2 * time.Second):
			close(release)
			t.Fatalf("parallel workers started = %d, want %d", index, ParallelCaseLimit)
		}
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if maximum.Load() != ParallelCaseLimit {
		t.Fatalf("maximum parallel cases = %d, want %d", maximum.Load(), ParallelCaseLimit)
	}
}

func TestRunRejectsUndiagnosedPhysicalFailure(t *testing.T) {
	executor := testExecutor(capabilityfeedback.OutcomePass)
	executor.promote = func(_ context.Context, _ opentopologysynthesis.SynthesisRun, _ opentopologysynthesis.SimulationEnvironment, _ opentopologysynthesis.PhysicalPromotionOptions) opentopologysynthesis.PhysicalPromotionResult {
		return opentopologysynthesis.PhysicalPromotionResult{Status: opentopologysynthesis.PhysicalPromotionFailed, Hash: testDigest("failed-promotion")}
	}
	_, err := executor.Run(context.Background(), testRequest(t, filepath.Join(t.TempDir(), "undiagnosed")))
	if err == nil || !strings.Contains(err.Error(), "lacks diagnostics") {
		t.Fatalf("undiagnosed promotion error = %v", err)
	}
}

func TestFeedbackDomainMapsProtectionToPower(t *testing.T) {
	domain, err := feedbackDomain("protection_power_integrity")
	if err != nil || domain != capabilityevaluation.DomainPower {
		t.Fatalf("protection domain = %q, %v", domain, err)
	}
}

func TestRunFailsClosedOnReplayDriftAndExistingOutput(t *testing.T) {
	executor := testExecutor(capabilityfeedback.OutcomeUnsupported)
	var calls atomic.Int64
	executor.synthesize = func(_ context.Context, _ opentopologysynthesis.Requirement, _ opentopologysynthesis.PrimitiveInventory, _ opentopologysynthesis.SimulationEnvironment, _ opentopologysynthesis.Policy) opentopologysynthesis.SynthesisRun {
		current := calls.Add(1)
		return opentopologysynthesis.SynthesisRun{Hash: testDigest(fmt.Sprintf("run-%d", current)), Report: opentopologysynthesis.Report{Status: opentopologysynthesis.StatusUnsupported}}
	}
	if _, err := executor.Run(context.Background(), testRequest(t, filepath.Join(t.TempDir(), "drift"))); err == nil || !strings.Contains(err.Error(), "replay differs") {
		t.Fatalf("replay drift error = %v", err)
	}

	existing := filepath.Join(t.TempDir(), "existing")
	if err := os.Mkdir(existing, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := testExecutor(capabilityfeedback.OutcomeUnsupported).Run(context.Background(), testRequest(t, existing)); err == nil || !strings.Contains(err.Error(), "fresh V10 evaluator output root") {
		t.Fatalf("existing-root error = %v", err)
	}

	policyDrift := testRequest(t, filepath.Join(t.TempDir(), "policy-drift"))
	policyDrift.Environment.Policy.MaxGeneratedGraphs++
	if _, err := testExecutor(capabilityfeedback.OutcomeUnsupported).Run(context.Background(), policyDrift); err == nil || !strings.Contains(err.Error(), "environment") {
		t.Fatalf("policy-drift error = %v", err)
	}
}

func testExecutor(outcome capabilityfeedback.Outcome) Executor {
	executor := New()
	executor.decode = func([]byte) (opentopologysynthesis.Requirement, error) {
		return opentopologysynthesis.Requirement{}, nil
	}
	executor.synthesize = func(_ context.Context, _ opentopologysynthesis.Requirement, _ opentopologysynthesis.PrimitiveInventory, _ opentopologysynthesis.SimulationEnvironment, _ opentopologysynthesis.Policy) opentopologysynthesis.SynthesisRun {
		status := opentopologysynthesis.StatusUnsupported
		if outcome == capabilityfeedback.OutcomePass {
			status = opentopologysynthesis.StatusPassed
		}
		return opentopologysynthesis.SynthesisRun{Hash: testDigest("synthesis"), Report: opentopologysynthesis.Report{
			Status: status, ModelRegistryHash: testDigest("models"), Consumption: opentopologysynthesis.Consumption{CandidateSimulations: 1},
		}}
	}
	executor.observe = func(meta capabilityfeedback.CaseMeta, _ opentopologysynthesis.Requirement, _ opentopologysynthesis.SynthesisRun, _ *opentopologysynthesis.PhysicalPromotionResult) (capabilityfeedback.CaseEvidence, error) {
		evidence := capabilityfeedback.CaseEvidence{Case: meta, Outcome: outcome}
		if outcome != capabilityfeedback.OutcomePass {
			evidence.Gaps = []capabilityfeedback.Gap{{
				Stage: "topology", Scope: capabilityfeedback.ScopeTopology, Capability: "generic_topology_search",
				Code: "no_complete_graph", RequiredEvidence: []string{"complete_candidate_graph"},
				EvidenceHashes: []string{testDigest("gap-" + meta.ID)},
			}}
		}
		return evidence, nil
	}
	return executor
}

func testRequest(t *testing.T, output string) Request {
	t.Helper()
	source := []byte("{}")
	models, diagnostics := modelprovenance.LoadDefault()
	if len(diagnostics) != 0 {
		t.Fatal(diagnostics)
	}
	modelHash, err := modelprovenance.Hash(models)
	if err != nil {
		t.Fatal(err)
	}
	request := Request{
		CorpusManifestSHA256: testDigest("manifest"), OutputRoot: output,
		Environment: Environment{
			Inventory: opentopologysynthesis.PrimitiveInventory{
				Hash: testDigest("inventory"), CatalogHash: testDigest("catalog"), ModelRegistryHash: modelHash,
			},
			Simulation: opentopologysynthesis.SimulationEnvironment{Catalog: &components.Catalog{}, CatalogHash: testDigest("catalog"), ModelRegistry: models},
			Policy:     opentopologysynthesis.DefaultPolicy(), LibraryIndex: &libraryresolver.LibraryIndex{}, KiCadCLI: "/test/kicad-cli",
			KiCadCLISHA256: testDigest("kicad-cli"), PromotionEnvironmentSHA256: testDigest("promotion-environment"),
			EvaluatorManifestSHA256: testDigest("evaluator"),
		},
	}
	for index := 1; index <= 24; index++ {
		id := fmt.Sprintf("v10_case_%03d", index)
		request.Cases = append(request.Cases, CaseInput{
			Entry: corpuspublication.EntryV10{
				ID: id, Role: "discovery", Domain: testDomains[(index-1)%len(testDomains)],
				CircuitRole: testRoles[(index-1)%len(testRoles)], SafetyImpact: "review_required",
				RequirementSHA256: testRawDigest(source),
			},
			RequirementSource: source,
			Obligations: []corpuspublication.ObligationV10{{
				Anchor: testDigest("anchor-" + id), Role: "discovery", CaseID: id,
				OperatingCaseID: "nominal", AssertionID: "assertion", ObservationKind: "port", ObservationID: "output", OutputID: "output",
			}},
		})
	}
	return request
}

var testDomains = []string{"analog_signal_path", "power_energy_conversion", "digital_control", "mixed_signal_data_conversion", "sensing_instrumentation", "protection_power_integrity"}
var testRoles = []string{"source_bias", "amplification_conditioning", "conversion_regulation", "sensing_measurement", "interface_control", "protection_supervision"}

func testDigest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func testRawDigest(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}
