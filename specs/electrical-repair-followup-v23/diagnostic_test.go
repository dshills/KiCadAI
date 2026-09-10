package electricalfollowup

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"kicadai/internal/capabilityexecutorv10"
	"kicadai/internal/circuitgraph"
	"kicadai/internal/components"
	diag "kicadai/internal/electricaldiagnostics"
	"kicadai/internal/modelprovenance"
	ot "kicadai/internal/opentopologysynthesis"
	"kicadai/internal/reports"
	"kicadai/internal/simulationadmission"
)

type sidecar struct {
	Schema          string                        `json:"schema"`
	Selected        bool                          `json:"selected"`
	PredecessorHash string                        `json:"predecessor_sha256"`
	SynthesisHash   string                        `json:"synthesis_sha256"`
	ElectricalHash  string                        `json:"electrical_sha256"`
	Repair          *ot.ElectricalRepairResultV22 `json:"repair"`
}

// This opt-in diagnostic is not a test-suite corpus evaluation. Its runner
// requires a clean committed freeze, authenticates all inputs, and allocates a
// new output root before invoking this test binary exactly once.
func TestFrozenResidualDiagnostic(t *testing.T) {
	output := os.Getenv("KICADAI_V23_DIAGNOSTIC_OUTPUT")
	if output == "" {
		t.Skip("explicit frozen diagnostic invocation only")
	}
	if runtime.Version() != "go1.26.8" || runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		t.Fatal("frozen Go environment differs")
	}
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	corpus, err := capabilityexecutorv10.LoadPublicDiscovery(filepath.Join(root, "internal/capabilityfeedback/testdata/closed_loop_open_set_v10_corpus"))
	if err != nil {
		t.Fatal(err)
	}
	if corpus.ManifestSHA256 != "0ec3834c832246e659b417dcef4aaae6d1634cbcd19c734518990280b124dc94" {
		t.Fatal("public corpus commitment differs")
	}
	catalog, err := components.LoadCatalogV18(ctx)
	if err != nil {
		t.Fatal(err)
	}
	models, diagnostics := modelprovenance.LoadV18()
	if len(diagnostics) != 0 {
		t.Fatal("V18 model load failed")
	}
	catalogHash := circuitgraph.NewResolver(circuitgraph.ResolveOptions{Catalog: catalog}).CatalogHash()
	inventory, issues := ot.BuildPrimitiveInventory(catalog, catalogHash, models)
	if reports.HasBlockingIssue(issues) {
		t.Fatal("V18 inventory load failed")
	}
	environment := ot.SimulationEnvironment{Catalog: catalog, CatalogHash: catalogHash, ModelRegistry: models}
	source, err := simulationadmission.NewSource("embedded-v20-model-provenance", simulationadmission.SourceBundled, models)
	if err != nil {
		t.Fatal(err)
	}
	admission := simulationadmission.Environment{Sources: []simulationadmission.Source{source}, EnabledSolvers: simulationadmission.EnabledBuiltinSolverIDs()}
	for _, selected := range []struct {
		id, phase string
		trials    int
	}{
		{"v10_case_004", "initial", 0}, {"v10_case_017", "initial", 114},
		{"v10_case_018", "recovery-1", 4}, {"v10_case_021", "recovery-1", 0},
	} {
		var requirement ot.Requirement
		found := false
		for _, input := range corpus.Cases {
			if input.Entry.ID == selected.id {
				if err := decode(input.RequirementSource, &requirement); err != nil {
					t.Fatal(err)
				}
				found = true
				break
			}
		}
		if !found || len(ot.Validate(requirement)) != 0 {
			t.Fatal("selected public requirement missing or invalid")
		}
		requirement = ot.Normalize(requirement)
		requirementHash, err := ot.CanonicalHash(requirement)
		if err != nil {
			t.Fatal(err)
		}
		var prior diag.Synthesis
		readJSON(t, filepath.Join(root, "specs/post-topology-electrical-blockers/evidence", selected.phase, "results", selected.id+".trace.json"), &prior)
		unsigned := prior
		unsigned.Hash = ""
		if digest(unsigned) != prior.Hash || prior.RequirementHash != requirementHash || len(prior.CertifiedCandidates) != 1 {
			t.Fatal("retained diagnostic trace binding differs")
		}
		var recorded sidecar
		readJSON(t, filepath.Join(root, "internal/capabilityfeedback/testdata/closed_loop_open_set_v22_public_1/replays", selected.id, "replay-1/ELECTRICAL_REPAIR.json"), &recorded)
		if !recorded.Selected || recorded.Repair == nil || recorded.PredecessorHash != prior.RunHash || recorded.SynthesisHash != prior.RunHash {
			t.Fatal("V22 predecessor binding differs")
		}
		repair := *recorded.Repair
		candidate := prior.CertifiedCandidates[0]
		if repair.RequirementHash != requirementHash || repair.InventoryHash != inventory.Hash || repair.InitialEvaluationHash != candidate.Evaluation.EvaluationHash || repair.InitialGraphHash != candidate.Evaluation.GraphHash || len(repair.Trials) != selected.trials {
			t.Fatal("V22 seed/environment/population differs")
		}
		if selected.trials == 0 && repair.StopReason != "critical_failure" {
			t.Fatal("critical refusal differs")
		}
		started, invocations := time.Now(), 0
		result, replayErr := replayRecorded(ctx, candidate.Graph, repair, ot.DefaultPolicy(), func(ctx context.Context, graph ot.CandidateGraph, policy ot.Policy) ot.SimulationEvaluation {
			invocations++
			return ot.EvaluateElectricalCandidateV22(ctx, requirement, graph, inventory, environment, admission, policy)
		})
		elapsed := time.Since(started)
		failure := ""
		if replayErr != nil {
			failure = replayErr.Error()
		}
		writeJSON(t, filepath.Join(output, selected.id+".diagnostic.json"), struct {
			Schema          string            `json:"schema"`
			Case            string            `json:"case"`
			SourceTraceHash string            `json:"source_trace_sha256"`
			RepairHash      string            `json:"repair_sha256"`
			InitialFailure  *diag.Failure     `json:"initial_failure"`
			StopReason      string            `json:"v22_stop_reason"`
			Trials          []trialDiagnostic `json:"trials"`
			Error           string            `json:"error,omitempty"`
		}{"kicadai.residual-electrical-diagnostic.v23", selected.id, prior.Hash, repair.Hash, candidate.Evaluation.FirstFailure, repair.StopReason, result, failure})
		writeJSON(t, filepath.Join(output, selected.id+".metrics.json"), struct {
			Invocations   int   `json:"evaluation_invocations"`
			Authenticated int   `json:"authenticated_trials"`
			Nanoseconds   int64 `json:"elapsed_nanoseconds"`
		}{invocations, len(result), elapsed.Nanoseconds()})
		t.Logf("%s: %d/%d authenticated trials; %d invocations; %s", selected.id, len(result), selected.trials, invocations, elapsed)
		if replayErr != nil {
			t.Fatal(replayErr)
		}
	}
}

func digest(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		panic(fmt.Sprintf("diagnostic digest: %v", err))
	}
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

func decode(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return fmt.Errorf("JSON has trailing content")
	}
	return nil
}

func readJSON(t *testing.T, path string, target any) {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 16<<20 {
		t.Fatalf("invalid bounded regular file: %s", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := decode(data, target); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
}

func writeJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, writeErr := file.Write(append(data, '\n'))
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		t.Fatalf("write diagnostic: %v, %v", writeErr, closeErr)
	}
}
