package capabilityexecutorv10

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"runtime/debug"
	"strings"

	"kicadai/internal/capabilitybaselinev10"
	"kicadai/internal/capabilityevaluation"
	"kicadai/internal/capabilityfeedback"
	"kicadai/internal/opentopologysynthesis"
)

const v16CleanRootSchema = "kicadai.closed-loop-open-set-clean-root.v16"
const v16EvaluationRootSchema = "kicadai.closed-loop-open-set-evaluation-root.v16"
const v16EvaluationRootMarkerName = "V16_EVALUATION_ROOT.json"
const v16ParallelCaseLimit = 1

func releaseReplayMemoryV16() {
	runtime.GC()
	debug.FreeOSMemory()
}

type replayResultV16 struct {
	rootSHA256   string
	replaySHA256 string
	spoolPath    string
	promotion    opentopologysynthesis.PhysicalPromotionResult
	promoted     bool
	observation  capabilityfeedback.CaseEvidence
	gates        capabilitybaselinev10.GateEvidence
}

// RunV16 executes one case and one replay at a time. Each replay is isolated in
// a helper frame and unreachable memory is returned to the operating system
// before the next replay starts. The retired V14 implementation remains
// unchanged and reproducible.
func (executor Executor) RunV16(ctx context.Context, request Request) (capabilitybaselinev10.Report, error) {
	return executor.runV16(ctx, request, releaseReplayMemoryV16)
}

func (executor Executor) runV16(ctx context.Context, request Request, releaseReplayMemory func()) (capabilitybaselinev10.Report, error) {
	if executor.decode == nil || executor.synthesize == nil || executor.promote == nil || executor.observe == nil {
		return capabilitybaselinev10.Report{}, fmt.Errorf("V16 evaluator engine is incomplete")
	}
	if releaseReplayMemory == nil {
		return capabilitybaselinev10.Report{}, fmt.Errorf("V16 replay memory release is required")
	}
	environmentSHA256, err := validateEnvironment(request.Environment)
	if err != nil {
		return capabilitybaselinev10.Report{}, err
	}
	if !digestPattern.MatchString(request.CorpusManifestSHA256) || len(request.Cases) != 24 {
		return capabilitybaselinev10.Report{}, fmt.Errorf("V16 corpus binding or discovery cohort is invalid")
	}
	for index, input := range request.Cases {
		if wantID := fmt.Sprintf("v10_case_%03d", index+1); input.Entry.ID != wantID {
			return capabilitybaselinev10.Report{}, fmt.Errorf("V16 reused-corpus case order differs at %d", index)
		}
	}
	marker := evaluationRootMarker{
		Schema: v16EvaluationRootSchema, Version: 16,
		CorpusManifestSHA256: request.CorpusManifestSHA256, EnvironmentSHA256: environmentSHA256,
		EvaluatorManifestSHA256: request.Environment.EvaluatorManifestSHA256,
		CaseCount:               len(request.Cases), ReplaysPerCase: 2, ParallelCaseLimit: v16ParallelCaseLimit,
	}
	if err := prepareV16OutputRoot(request.OutputRoot, request.Resume, marker); err != nil {
		return capabilitybaselinev10.Report{}, err
	}
	records, completed, err := loadV16CaseCheckpoints(request.OutputRoot, request.Cases, marker, request.Resume)
	if err != nil {
		return capabilitybaselinev10.Report{}, err
	}
	for index, input := range request.Cases {
		if completed[index] {
			continue
		}
		result, runErr := executor.runCaseV16(ctx, request.OutputRoot, request.CorpusManifestSHA256, input, request.Environment, environmentSHA256, releaseReplayMemory)
		if runErr == nil {
			runErr = persistCaseCheckpoint(request.OutputRoot, result.evidence)
		}
		if runErr == nil {
			runErr = removeReplaySpoolsV11(result.replaySpools)
		}
		if runErr != nil {
			return capabilitybaselinev10.Report{}, fmt.Errorf("%s: %w", input.Entry.ID, runErr)
		}
		records[index] = result.evidence
	}
	return capabilitybaselinev10.Build(request.CorpusManifestSHA256, records)
}

func (executor Executor) runCaseV16(ctx context.Context, outputRoot, corpusManifestSHA256 string, input CaseInput, environment Environment, environmentSHA256 string, releaseReplayMemory func()) (caseResultV11, error) {
	if err := validateEntry(input); err != nil {
		return caseResultV11{}, err
	}
	requirement, err := executor.decode(input.RequirementSource)
	if err != nil {
		return caseResultV11{}, err
	}
	caseRoot := filepath.Join(outputRoot, input.Entry.ID)
	if err := os.Mkdir(caseRoot, 0o755); err != nil {
		return caseResultV11{}, fmt.Errorf("create case root: %w", err)
	}
	replays := make([]replayResultV16, 0, 2)
	for replay := 1; replay <= 2; replay++ {
		result, runErr := executor.runReplayV16(ctx, caseRoot, corpusManifestSHA256, input, requirement, environment, environmentSHA256, replay)
		releaseReplayMemory()
		if runErr != nil {
			return caseResultV11{}, runErr
		}
		replays = append(replays, result)
	}
	if replays[0].replaySHA256 != replays[1].replaySHA256 {
		return caseResultV11{}, fmt.Errorf("synthesis replay differs across clean roots")
	}
	if replays[0].promoted != replays[1].promoted || (replays[0].promoted &&
		(replays[0].promotion.Hash != replays[1].promotion.Hash ||
			replays[0].promotion.Status != replays[1].promotion.Status ||
			replays[0].promotion.ProjectHash != replays[1].promotion.ProjectHash)) {
		return caseResultV11{}, fmt.Errorf("physical promotion replay differs across clean roots")
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
		return caseResultV11{}, err
	}
	return caseResultV11{evidence: capabilitybaselinev10.CaseEvidence{
		Schema: capabilitybaselinev10.CaseEvidenceSchema, Version: capabilitybaselinev10.Version,
		Case: roundCase, RequirementSHA256: input.Entry.RequirementSHA256,
		EnvironmentSHA256: environmentSHA256, EvaluatorManifestSHA256: environment.EvaluatorManifestSHA256,
		ReplaySHA256:     []string{replays[0].replaySHA256, replays[1].replaySHA256},
		ReplayRootSHA256: []string{replays[0].rootSHA256, replays[1].rootSHA256},
		Gates:            replays[0].gates, Promotions: promotions,
	}, replaySpools: []string{replays[0].spoolPath, replays[1].spoolPath}}, nil
}

func (executor Executor) runReplayV16(ctx context.Context, caseRoot, corpusManifestSHA256 string, input CaseInput, requirement opentopologysynthesis.Requirement, environment Environment, environmentSHA256 string, replay int) (replayResultV16, error) {
	root := filepath.Join(caseRoot, fmt.Sprintf("replay-%d", replay))
	commitment, err := prepareCleanRoot(root, cleanRootMarker{
		Schema: v16CleanRootSchema, Version: 16, CaseID: input.Entry.ID, Replay: replay,
		CorpusManifestSHA256: corpusManifestSHA256, RequirementSHA256: input.Entry.RequirementSHA256,
		EnvironmentSHA256: environmentSHA256, EvaluatorManifestSHA256: environment.EvaluatorManifestSHA256,
	})
	if err != nil {
		return replayResultV16{}, err
	}
	run := executor.synthesize(ctx, requirement, environment.Inventory, environment.Simulation, environment.Policy)
	if run.Report.Status == opentopologysynthesis.StatusInvalid || run.Report.Status == opentopologysynthesis.StatusCanceled {
		return replayResultV16{}, fmt.Errorf("synthesis terminated with non-capability status %q", run.Report.Status)
	}
	spoolPath := filepath.Join(root, replaySpoolNameV11)
	digest, err := writeReplaySpoolV11(spoolPath, &run)
	if err != nil {
		return replayResultV16{}, fmt.Errorf("stream synthesis replay: %w", err)
	}
	result := replayResultV16{rootSHA256: commitment, replaySHA256: digest, spoolPath: spoolPath}
	var observedPromotion *opentopologysynthesis.PhysicalPromotionResult
	if run.Report.Status == opentopologysynthesis.StatusPassed {
		result.promotion = executor.promote(ctx, run, environment.Simulation, opentopologysynthesis.PhysicalPromotionOptions{
			OutputRoot: filepath.Join(root, "promotion"), KiCadCLI: environment.KiCadCLI,
			LibraryIndex: environment.LibraryIndex, Timeout: environment.PromotionTimeout,
			KeepArtifacts: environment.KeepPhysicalPromotionArtifacts,
		})
		if err := validatePromotionCompletion(result.promotion); err != nil {
			return replayResultV16{}, err
		}
		result.promoted = true
		observedPromotion = &result.promotion
	}
	if replay == 1 {
		domain, err := feedbackDomain(input.Entry.Domain)
		if err != nil {
			return replayResultV16{}, err
		}
		result.observation, err = executor.observe(capabilityfeedback.CaseMeta{
			ID: input.Entry.ID, Role: capabilityfeedback.RoleDiscovery, Domain: domain,
			SafetyImpact: capabilityevaluation.SafetyImpact(input.Entry.SafetyImpact),
		}, requirement, run, observedPromotion)
		if err != nil {
			return replayResultV16{}, fmt.Errorf("observe capability evidence: %w", err)
		}
		result.gates = synthesisGates(run, result.observation, result.promoted && result.promotion.Status == opentopologysynthesis.PhysicalPromotionPassed)
	}
	return result, nil
}

func prepareV16OutputRoot(root string, resume bool, marker evaluationRootMarker) error {
	if strings.TrimSpace(root) == "" {
		return fmt.Errorf("V16 evaluator output root is required")
	}
	clean := filepath.Clean(root)
	if resume {
		info, err := os.Lstat(clean)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("resume V16 evaluator output root: not a real directory")
		}
		checkpointInfo, err := os.Lstat(filepath.Join(clean, checkpointDirectoryName))
		if err != nil || !checkpointInfo.IsDir() || checkpointInfo.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("resume V16 evaluator checkpoint root: not a real directory")
		}
		data, err := readBoundedRegularFile(filepath.Join(clean, v16EvaluationRootMarkerName), 64*1024)
		if err != nil {
			return fmt.Errorf("resume V16 evaluator root commitment: %w", err)
		}
		var actual evaluationRootMarker
		if err := decodeStrictJSON(data, &actual); err != nil || !reflect.DeepEqual(actual, marker) {
			return fmt.Errorf("resume V16 evaluator root commitment differs")
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(clean), 0o755); err != nil {
		return err
	}
	if err := os.Mkdir(clean, 0o755); err != nil {
		return fmt.Errorf("create fresh V16 evaluator output root: %w", err)
	}
	if err := os.Mkdir(filepath.Join(clean, checkpointDirectoryName), 0o755); err != nil {
		return fmt.Errorf("create V16 evaluator checkpoint root: %w", err)
	}
	data, err := json.Marshal(marker)
	if err != nil {
		return err
	}
	return writeNewReadOnlyFile(filepath.Join(clean, v16EvaluationRootMarkerName), data)
}

func loadV16CaseCheckpoints(root string, cases []CaseInput, marker evaluationRootMarker, resume bool) ([]capabilitybaselinev10.CaseEvidence, []bool, error) {
	records := make([]capabilitybaselinev10.CaseEvidence, len(cases))
	completed := make([]bool, len(cases))
	if !resume {
		return records, completed, nil
	}
	entries, err := os.ReadDir(filepath.Join(root, checkpointDirectoryName))
	if err != nil {
		return nil, nil, fmt.Errorf("read V16 evaluator checkpoints: %w", err)
	}
	allowed := make(map[string]bool, len(cases))
	for _, input := range cases {
		allowed[input.Entry.ID+".json"] = true
	}
	for _, entry := range entries {
		if allowed[entry.Name()] {
			continue
		}
		path := filepath.Join(root, checkpointDirectoryName, entry.Name())
		if strings.HasPrefix(entry.Name(), ".v10-checkpoint-") && entry.Type().IsRegular() {
			if err := os.Remove(path); err != nil {
				return nil, nil, fmt.Errorf("remove incomplete V16 checkpoint: %w", err)
			}
			continue
		}
		return nil, nil, fmt.Errorf("unexpected V16 evaluator checkpoint entry %q", entry.Name())
	}
	for index, input := range cases {
		data, err := readBoundedRegularFile(caseCheckpointPath(root, input.Entry.ID), 16*1024*1024)
		if errors.Is(err, os.ErrNotExist) {
			if err := os.RemoveAll(filepath.Join(root, input.Entry.ID)); err != nil {
				return nil, nil, fmt.Errorf("remove incomplete V16 case root: %w", err)
			}
			continue
		}
		if err != nil {
			return nil, nil, fmt.Errorf("read %s checkpoint: %w", input.Entry.ID, err)
		}
		var record capabilitybaselinev10.CaseEvidence
		if err := decodeStrictJSON(data, &record); err != nil {
			return nil, nil, fmt.Errorf("decode %s checkpoint: %w", input.Entry.ID, err)
		}
		validated, err := capabilitybaselinev10.ValidateCase(record)
		if err != nil || !reflect.DeepEqual(record, validated) || record.Case.ID != input.Entry.ID ||
			record.RequirementSHA256 != input.Entry.RequirementSHA256 || record.EnvironmentSHA256 != marker.EnvironmentSHA256 ||
			record.EvaluatorManifestSHA256 != marker.EvaluatorManifestSHA256 {
			return nil, nil, fmt.Errorf("%s checkpoint binding or evidence is invalid", input.Entry.ID)
		}
		if err := validateV16CheckpointRoots(root, input, marker, record); err != nil {
			return nil, nil, err
		}
		records[index], completed[index] = validated, true
	}
	return records, completed, nil
}

func validateV16CheckpointRoots(root string, input CaseInput, marker evaluationRootMarker, record capabilitybaselinev10.CaseEvidence) error {
	for replay := 1; replay <= 2; replay++ {
		path := filepath.Join(root, input.Entry.ID, fmt.Sprintf("replay-%d", replay), "CLEAN_ROOT.json")
		data, err := readBoundedRegularFile(path, 64*1024)
		if err != nil || hashBytes(data) != record.ReplayRootSHA256[replay-1] {
			return fmt.Errorf("%s replay %d V16 checkpoint root is invalid", input.Entry.ID, replay)
		}
		var actual cleanRootMarker
		if err := decodeStrictJSON(data, &actual); err != nil {
			return fmt.Errorf("%s replay %d V16 checkpoint root is malformed", input.Entry.ID, replay)
		}
		expected := cleanRootMarker{
			Schema: v16CleanRootSchema, Version: 16, CaseID: input.Entry.ID, Replay: replay,
			CorpusManifestSHA256: marker.CorpusManifestSHA256, RequirementSHA256: input.Entry.RequirementSHA256,
			EnvironmentSHA256: marker.EnvironmentSHA256, EvaluatorManifestSHA256: marker.EvaluatorManifestSHA256,
		}
		if !reflect.DeepEqual(actual, expected) {
			return fmt.Errorf("%s replay %d V16 checkpoint root binding differs", input.Entry.ID, replay)
		}
	}
	return nil
}
