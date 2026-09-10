package capabilityexecutorv10

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"kicadai/internal/capabilitybaselinev10"
	"kicadai/internal/capabilityevaluation"
	"kicadai/internal/capabilityfeedback"
	"kicadai/internal/opentopologysynthesis"
)

const v23CleanRootSchema = "kicadai.closed-loop-open-set-clean-root.v23"

type replayResultV23 struct {
	rootSHA256    string
	replaySHA256  string
	sidecarSHA256 string
	promotion     opentopologysynthesis.PhysicalPromotionResult
	promoted      bool
	observation   capabilityfeedback.CaseEvidence
	gates         capabilitybaselinev10.GateEvidence
}

// RunV23 uses fresh roots only, exactly two serial replays, and full canonical
// synthesis hashes. Timing is recorded separately from deterministic evidence.
func (executor ExecutorV23) RunV23(ctx context.Context, request Request) (capabilitybaselinev10.Report, error) {
	if request.Resume || executor.predecessor.decode == nil || executor.predecessor.synthesize == nil || executor.predecessor.promote == nil || executor.predecessor.observe == nil || executor.successor == nil {
		return capabilitybaselinev10.Report{}, fmt.Errorf("V23 requires a complete executor and a fresh non-resumed run")
	}
	environmentSHA256, err := validateEnvironment(request.Environment)
	if err != nil {
		return capabilitybaselinev10.Report{}, err
	}
	if !digestPattern.MatchString(request.CorpusManifestSHA256) || len(request.Cases) != 24 || len(executor.cohort) != 24 {
		return capabilitybaselinev10.Report{}, fmt.Errorf("V23 requires its exact bound 24-case public cohort")
	}
	for index, input := range request.Cases {
		binding, err := inputBindingV22(input)
		if err != nil || input.Entry.ID != fmt.Sprintf("v10_case_%03d", index+1) || executor.cohort[input.Entry.ID] != binding {
			return capabilitybaselinev10.Report{}, fmt.Errorf("V23 public cohort binding differs at %d", index)
		}
		if err := validateEntry(input); err != nil {
			return capabilitybaselinev10.Report{}, err
		}
	}
	if err := ctx.Err(); err != nil {
		return capabilitybaselinev10.Report{}, err
	}
	if err := os.Mkdir(request.OutputRoot, 0o755); err != nil {
		return capabilitybaselinev10.Report{}, err
	}
	if err := os.Mkdir(filepath.Join(request.OutputRoot, checkpointDirectoryName), 0o755); err != nil {
		return capabilitybaselinev10.Report{}, err
	}
	marker := evaluationRootMarker{Schema: "kicadai.closed-loop-open-set-evaluation-root.v23", Version: 23,
		CorpusManifestSHA256: request.CorpusManifestSHA256, EnvironmentSHA256: environmentSHA256,
		EvaluatorManifestSHA256: request.Environment.EvaluatorManifestSHA256, CaseCount: 24, ReplaysPerCase: 2, ParallelCaseLimit: 1}
	if _, err := writeEvidenceV22(filepath.Join(request.OutputRoot, "V23_EVALUATION_ROOT.json"), marker); err != nil {
		return capabilitybaselinev10.Report{}, err
	}
	records := make([]capabilitybaselinev10.CaseEvidence, 0, 24)
	for _, input := range request.Cases {
		result, err := executor.runCaseV23(ctx, request.OutputRoot, request.CorpusManifestSHA256, input, request.Environment, environmentSHA256)
		if err != nil {
			return capabilitybaselinev10.Report{}, fmt.Errorf("%s: %w", input.Entry.ID, err)
		}
		if err := persistCaseCheckpoint(request.OutputRoot, result); err != nil {
			return capabilitybaselinev10.Report{}, err
		}
		records = append(records, result)
	}
	return capabilitybaselinev10.Build(request.CorpusManifestSHA256, records)
}

func (executor ExecutorV23) runCaseV23(ctx context.Context, outputRoot, corpusManifestSHA256 string, input CaseInput, environment Environment, environmentSHA256 string) (capabilitybaselinev10.CaseEvidence, error) {
	if err := validateEntry(input); err != nil {
		return capabilitybaselinev10.CaseEvidence{}, err
	}
	requirement, err := executor.predecessor.decode(input.RequirementSource)
	if err != nil {
		return capabilitybaselinev10.CaseEvidence{}, err
	}
	caseRoot := filepath.Join(outputRoot, input.Entry.ID)
	if err := os.Mkdir(caseRoot, 0o755); err != nil {
		return capabilitybaselinev10.CaseEvidence{}, fmt.Errorf("create case root: %w", err)
	}
	replays := make([]replayResultV23, 0, 2)
	for replay := 1; replay <= 2; replay++ {
		result, runErr := executor.runReplayV23(ctx, caseRoot, corpusManifestSHA256, input, requirement, environment, environmentSHA256, replay)
		releaseReplayMemoryV17()
		if runErr != nil {
			return capabilitybaselinev10.CaseEvidence{}, runErr
		}
		replays = append(replays, result)
	}
	if replays[0].replaySHA256 != replays[1].replaySHA256 || replays[0].sidecarSHA256 != replays[1].sidecarSHA256 {
		return capabilitybaselinev10.CaseEvidence{}, fmt.Errorf("synthesis replay differs across clean roots")
	}
	if replays[0].promoted != replays[1].promoted || (replays[0].promoted &&
		(replays[0].promotion.Hash != replays[1].promotion.Hash ||
			replays[0].promotion.Status != replays[1].promotion.Status ||
			replays[0].promotion.ProjectHash != replays[1].promotion.ProjectHash)) {
		return capabilitybaselinev10.CaseEvidence{}, fmt.Errorf("physical promotion replay differs across clean roots")
	}
	promotions := []capabilitybaselinev10.PromotionEvidence{}
	if replays[0].promoted && replays[0].promotion.Status == opentopologysynthesis.PhysicalPromotionPassed {
		for _, replay := range replays {
			promotions = append(promotions, capabilitybaselinev10.PromotionEvidence{
				CleanRootSHA256: replay.rootSHA256, RunSHA256: replay.promotion.Hash,
				ProjectSHA256: replay.promotion.ProjectHash, InstalledKiCad: true,
				ReplayIdentical: replay.promotion.ReplayIdentical,
			})
		}
	}
	roundCase, err := buildRoundCase(input, replays[0].observation)
	if err != nil {
		return capabilitybaselinev10.CaseEvidence{}, err
	}
	return capabilitybaselinev10.CaseEvidence{
		Schema: capabilitybaselinev10.CaseEvidenceSchema, Version: capabilitybaselinev10.Version,
		Case: roundCase, RequirementSHA256: input.Entry.RequirementSHA256,
		EnvironmentSHA256: environmentSHA256, EvaluatorManifestSHA256: environment.EvaluatorManifestSHA256,
		ReplaySHA256:     []string{replays[0].replaySHA256, replays[1].replaySHA256},
		ReplayRootSHA256: []string{replays[0].rootSHA256, replays[1].rootSHA256},
		Gates:            replays[0].gates, Promotions: promotions,
	}, nil
}

func (executor ExecutorV23) runReplayV23(ctx context.Context, caseRoot, corpusManifestSHA256 string, input CaseInput, requirement opentopologysynthesis.Requirement, environment Environment, environmentSHA256 string, replay int) (replayResultV23, error) {
	root := filepath.Join(caseRoot, fmt.Sprintf("replay-%d", replay))
	commitment, err := prepareCleanRoot(root, cleanRootMarker{
		Schema: v23CleanRootSchema, Version: 23, CaseID: input.Entry.ID, Replay: replay,
		CorpusManifestSHA256: corpusManifestSHA256, RequirementSHA256: input.Entry.RequirementSHA256,
		EnvironmentSHA256: environmentSHA256, EvaluatorManifestSHA256: environment.EvaluatorManifestSHA256,
	})
	if err != nil {
		return replayResultV23{}, err
	}

	if err := ctx.Err(); err != nil {
		return replayResultV23{}, err
	}
	started := time.Now()
	var run opentopologysynthesis.SynthesisRun
	sidecar := electricalSidecarV23{Schema: "kicadai.public-electrical-replay.v23", Selected: executor.selected[input.Entry.ID]}
	if sidecar.Selected {
		successor := executor.successor(ctx, requirement, environment.Inventory, environment.Simulation, environment.Policy)
		run = successor.Synthesis
		sidecar.SolverPolicyID, sidecar.SolverPolicyHash = successor.SolverPolicyID, successor.SolverPolicyHash
		sidecar.PredecessorHash, sidecar.ElectricalHash, sidecar.Repair = successor.PredecessorHash, successor.Hash, successor.ElectricalRepair
	} else {
		run = executor.predecessor.synthesize(ctx, requirement, environment.Inventory, environment.Simulation, environment.Policy)
		sidecar.PredecessorHash = run.Hash
	}
	metrics := replayMetricsV23{Schema: "kicadai.public-replay-metrics.v23", SynthesisNanoseconds: time.Since(started).Nanoseconds()}
	if err := ctx.Err(); err != nil {
		return replayResultV23{}, err
	}
	sidecar.SynthesisHash = run.Hash
	if run.Report.Status == opentopologysynthesis.StatusInvalid || run.Report.Status == opentopologysynthesis.StatusCanceled {
		return replayResultV23{}, fmt.Errorf("synthesis terminated with non-capability status %q", run.Report.Status)
	}

	started = time.Now()
	digest, canonicalBytes, err := hashReplayV22(ctx, &run)
	if err != nil {
		return replayResultV23{}, fmt.Errorf("stream synthesis replay: %w", err)
	}
	metrics.HashNanoseconds, metrics.CanonicalSynthesisBytes = time.Since(started).Nanoseconds(), canonicalBytes
	sidecarHash, err := writeEvidenceV22(filepath.Join(root, "ELECTRICAL_REPAIR.json"), sidecar)
	if err != nil {
		return replayResultV23{}, err
	}
	result := replayResultV23{rootSHA256: commitment, replaySHA256: digest, sidecarSHA256: sidecarHash}
	var observedPromotion *opentopologysynthesis.PhysicalPromotionResult
	if run.Report.Status == opentopologysynthesis.StatusPassed {
		started = time.Now()
		result.promotion = executor.predecessor.promote(ctx, run, environment.Simulation, opentopologysynthesis.PhysicalPromotionOptions{
			OutputRoot: filepath.Join(root, "promotion"), KiCadCLI: environment.KiCadCLI,
			LibraryIndex: environment.LibraryIndex, Timeout: environment.PromotionTimeout,
			KeepArtifacts: environment.KeepPhysicalPromotionArtifacts,
		})
		if err := validatePromotionCompletion(result.promotion); err != nil {
			return replayResultV23{}, err
		}
		metrics.PromotionNanoseconds = time.Since(started).Nanoseconds()
		if _, err := writeEvidenceV22(filepath.Join(root, "PROMOTION.json"), result.promotion); err != nil {
			return replayResultV23{}, err
		}
		result.promoted = true
		observedPromotion = &result.promotion
	}
	if replay == 1 {
		domain, err := feedbackDomain(input.Entry.Domain)
		if err != nil {
			return replayResultV23{}, err
		}
		result.observation, err = executor.predecessor.observe(capabilityfeedback.CaseMeta{
			ID: input.Entry.ID, Role: capabilityfeedback.RoleDiscovery, Domain: domain,
			SafetyImpact: capabilityevaluation.SafetyImpact(input.Entry.SafetyImpact),
		}, requirement, run, observedPromotion)
		if err != nil {
			return replayResultV23{}, fmt.Errorf("observe capability evidence: %w", err)
		}
		result.gates = synthesisGates(run, result.observation, result.promoted && result.promotion.Status == opentopologysynthesis.PhysicalPromotionPassed)
	}
	if _, err := writeEvidenceV22(filepath.Join(root, "METRICS.json"), metrics); err != nil {
		return replayResultV23{}, err
	}
	return result, nil
}

type electricalSidecarV23 struct {
	Schema           string                                           `json:"schema"`
	Selected         bool                                             `json:"selected"`
	PredecessorHash  string                                           `json:"predecessor_sha256"`
	SynthesisHash    string                                           `json:"synthesis_sha256"`
	ElectricalHash   string                                           `json:"electrical_sha256,omitempty"`
	SolverPolicyID   string                                           `json:"solver_policy_id,omitempty"`
	SolverPolicyHash string                                           `json:"solver_policy_sha256,omitempty"`
	Repair           *opentopologysynthesis.ElectricalRepairResultV23 `json:"repair,omitempty"`
}

type replayMetricsV23 struct {
	Schema                  string `json:"schema"`
	SynthesisNanoseconds    int64  `json:"synthesis_nanoseconds"`
	HashNanoseconds         int64  `json:"hash_nanoseconds"`
	PromotionNanoseconds    int64  `json:"promotion_nanoseconds"`
	CanonicalSynthesisBytes int64  `json:"canonical_synthesis_bytes"`
	SynthesisSpoolBytes     int64  `json:"synthesis_spool_bytes"`
}
