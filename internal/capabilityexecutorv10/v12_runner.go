package capabilityexecutorv10

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"

	"kicadai/internal/capabilitybaselinev10"
	"kicadai/internal/capabilityevaluation"
	"kicadai/internal/capabilityfeedback"
	"kicadai/internal/opentopologysynthesis"
)

const v12CleanRootSchema = "kicadai.closed-loop-open-set-clean-root.v12"
const v12EvaluationRootSchema = "kicadai.closed-loop-open-set-evaluation-root.v12"
const v12EvaluationRootMarkerName = "V12_EVALUATION_ROOT.json"
const v12ParallelCaseLimit = 2

// RunV12 preserves V11's one-live-replay rule and additionally bounds the
// aggregate number of live case graphs. It is separate from RunV11 so the
// retired V11 evaluator remains reproducible from its committed source.
func (executor Executor) RunV12(ctx context.Context, request Request) (capabilitybaselinev10.Report, error) {
	if executor.decode == nil || executor.synthesize == nil || executor.promote == nil || executor.observe == nil {
		return capabilitybaselinev10.Report{}, fmt.Errorf("V12 evaluator engine is incomplete")
	}
	environmentSHA256, err := validateEnvironment(request.Environment)
	if err != nil {
		return capabilitybaselinev10.Report{}, err
	}
	if !digestPattern.MatchString(request.CorpusManifestSHA256) || len(request.Cases) != 24 {
		return capabilitybaselinev10.Report{}, fmt.Errorf("V12 corpus binding or discovery cohort is invalid")
	}
	for index, input := range request.Cases {
		if wantID := fmt.Sprintf("v10_case_%03d", index+1); input.Entry.ID != wantID {
			return capabilitybaselinev10.Report{}, fmt.Errorf("V12 reused-corpus case order differs at %d", index)
		}
	}
	marker := evaluationRootMarker{
		Schema: v12EvaluationRootSchema, Version: 12,
		CorpusManifestSHA256: request.CorpusManifestSHA256, EnvironmentSHA256: environmentSHA256,
		EvaluatorManifestSHA256: request.Environment.EvaluatorManifestSHA256,
		CaseCount:               len(request.Cases), ReplaysPerCase: 2, ParallelCaseLimit: v12ParallelCaseLimit,
	}
	if err := prepareV12OutputRoot(request.OutputRoot, request.Resume, marker); err != nil {
		return capabilitybaselinev10.Report{}, err
	}
	records, completed, err := loadV12CaseCheckpoints(request.OutputRoot, request.Cases, marker, request.Resume)
	if err != nil {
		return capabilitybaselinev10.Report{}, err
	}
	errorsByIndex := make([]error, len(request.Cases))
	jobs := make(chan int, v12ParallelCaseLimit)
	var workers sync.WaitGroup
	for worker := 0; worker < v12ParallelCaseLimit; worker++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for index := range jobs {
				result, runErr := executor.runCaseV12(ctx, request.OutputRoot, request.CorpusManifestSHA256, request.Cases[index], request.Environment, environmentSHA256)
				if runErr == nil {
					runErr = persistCaseCheckpoint(request.OutputRoot, result.evidence)
				}
				if runErr == nil {
					runErr = removeReplaySpoolsV11(result.replaySpools)
				}
				if runErr != nil {
					errorsByIndex[index] = runErr
					continue
				}
				records[index] = result.evidence
			}
		}()
	}
	for index := range request.Cases {
		if !completed[index] {
			jobs <- index
		}
	}
	close(jobs)
	workers.Wait()
	for index, runErr := range errorsByIndex {
		if runErr != nil {
			return capabilitybaselinev10.Report{}, fmt.Errorf("%s: %w", request.Cases[index].Entry.ID, runErr)
		}
	}
	return capabilitybaselinev10.Build(request.CorpusManifestSHA256, records)
}

func (executor Executor) runCaseV12(ctx context.Context, outputRoot, corpusManifestSHA256 string, input CaseInput, environment Environment, environmentSHA256 string) (caseResultV11, error) {
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
	rootSHA256 := make([]string, 0, 2)
	replaySHA256 := make([]string, 0, 2)
	replaySpools := make([]string, 0, 2)
	promotionResults := make([]opentopologysynthesis.PhysicalPromotionResult, 0, 2)
	var firstObservation capabilityfeedback.CaseEvidence
	var firstGates capabilitybaselinev10.GateEvidence

	for replay := 1; replay <= 2; replay++ {
		root := filepath.Join(caseRoot, fmt.Sprintf("replay-%d", replay))
		commitment, err := prepareCleanRoot(root, cleanRootMarker{
			Schema: v12CleanRootSchema, Version: 12, CaseID: input.Entry.ID, Replay: replay,
			CorpusManifestSHA256: corpusManifestSHA256, RequirementSHA256: input.Entry.RequirementSHA256,
			EnvironmentSHA256: environmentSHA256, EvaluatorManifestSHA256: environment.EvaluatorManifestSHA256,
		})
		if err != nil {
			return caseResultV11{}, err
		}
		run := executor.synthesize(ctx, requirement, environment.Inventory, environment.Simulation, environment.Policy)
		if run.Report.Status == opentopologysynthesis.StatusInvalid || run.Report.Status == opentopologysynthesis.StatusCanceled {
			return caseResultV11{}, fmt.Errorf("synthesis terminated with non-capability status %q", run.Report.Status)
		}
		spoolPath := filepath.Join(root, replaySpoolNameV11)
		digest, err := writeReplaySpoolV11(spoolPath, &run)
		if err != nil {
			return caseResultV11{}, fmt.Errorf("stream synthesis replay: %w", err)
		}
		rootSHA256 = append(rootSHA256, commitment)
		replaySHA256 = append(replaySHA256, digest)
		replaySpools = append(replaySpools, spoolPath)

		var observedPromotion *opentopologysynthesis.PhysicalPromotionResult
		if run.Report.Status == opentopologysynthesis.StatusPassed {
			result := executor.promote(ctx, run, environment.Simulation, opentopologysynthesis.PhysicalPromotionOptions{
				OutputRoot: filepath.Join(root, "promotion"), KiCadCLI: environment.KiCadCLI,
				LibraryIndex: environment.LibraryIndex, Timeout: environment.PromotionTimeout,
				KeepArtifacts: environment.KeepPhysicalPromotionArtifacts,
			})
			if err := validatePromotionCompletion(result); err != nil {
				return caseResultV11{}, err
			}
			promotionResults = append(promotionResults, result)
			if replay == 1 {
				observedPromotion = &promotionResults[0]
			}
		}
		if replay == 1 {
			domain, err := feedbackDomain(input.Entry.Domain)
			if err != nil {
				return caseResultV11{}, err
			}
			firstObservation, err = executor.observe(capabilityfeedback.CaseMeta{
				ID: input.Entry.ID, Role: capabilityfeedback.RoleDiscovery, Domain: domain,
				SafetyImpact: capabilityevaluation.SafetyImpact(input.Entry.SafetyImpact),
			}, requirement, run, observedPromotion)
			if err != nil {
				return caseResultV11{}, fmt.Errorf("observe capability evidence: %w", err)
			}
			firstGates = synthesisGates(run, firstObservation, len(promotionResults) == 1 && promotionResults[0].Status == opentopologysynthesis.PhysicalPromotionPassed)
		}
		run = opentopologysynthesis.SynthesisRun{}
	}
	if replaySHA256[0] != replaySHA256[1] {
		return caseResultV11{}, fmt.Errorf("synthesis replay differs across clean roots")
	}
	if len(promotionResults) == 2 && (promotionResults[0].Hash != promotionResults[1].Hash || promotionResults[0].Status != promotionResults[1].Status || promotionResults[0].ProjectHash != promotionResults[1].ProjectHash) {
		return caseResultV11{}, fmt.Errorf("physical promotion replay differs across clean roots")
	}
	promotions := []capabilitybaselinev10.PromotionEvidence{}
	if len(promotionResults) == 2 && promotionResults[0].Status == opentopologysynthesis.PhysicalPromotionPassed {
		for index, result := range promotionResults {
			promotions = append(promotions, capabilitybaselinev10.PromotionEvidence{
				CleanRootSHA256: rootSHA256[index], RunSHA256: result.Hash, ProjectSHA256: result.ProjectHash,
				InstalledKiCad: true, ReplayIdentical: result.ReplayIdentical,
			})
		}
	}
	roundCase, err := buildRoundCase(input, firstObservation)
	if err != nil {
		return caseResultV11{}, err
	}
	return caseResultV11{evidence: capabilitybaselinev10.CaseEvidence{
		Schema: capabilitybaselinev10.CaseEvidenceSchema, Version: capabilitybaselinev10.Version,
		Case: roundCase, RequirementSHA256: input.Entry.RequirementSHA256,
		EnvironmentSHA256: environmentSHA256, EvaluatorManifestSHA256: environment.EvaluatorManifestSHA256,
		ReplaySHA256: replaySHA256, ReplayRootSHA256: rootSHA256, Gates: firstGates, Promotions: promotions,
	}, replaySpools: replaySpools}, nil
}

func prepareV12OutputRoot(root string, resume bool, marker evaluationRootMarker) error {
	if strings.TrimSpace(root) == "" {
		return fmt.Errorf("V12 evaluator output root is required")
	}
	clean := filepath.Clean(root)
	if resume {
		info, err := os.Lstat(clean)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("resume V12 evaluator output root: not a real directory")
		}
		checkpointInfo, err := os.Lstat(filepath.Join(clean, checkpointDirectoryName))
		if err != nil || !checkpointInfo.IsDir() || checkpointInfo.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("resume V12 evaluator checkpoint root: not a real directory")
		}
		data, err := readBoundedRegularFile(filepath.Join(clean, v12EvaluationRootMarkerName), 64*1024)
		if err != nil {
			return fmt.Errorf("resume V12 evaluator root commitment: %w", err)
		}
		var actual evaluationRootMarker
		if err := decodeStrictJSON(data, &actual); err != nil || !reflect.DeepEqual(actual, marker) {
			return fmt.Errorf("resume V12 evaluator root commitment differs")
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(clean), 0o755); err != nil {
		return err
	}
	if err := os.Mkdir(clean, 0o755); err != nil {
		return fmt.Errorf("create fresh V12 evaluator output root: %w", err)
	}
	if err := os.Mkdir(filepath.Join(clean, checkpointDirectoryName), 0o755); err != nil {
		return fmt.Errorf("create V12 evaluator checkpoint root: %w", err)
	}
	data, err := json.Marshal(marker)
	if err != nil {
		return err
	}
	return writeNewReadOnlyFile(filepath.Join(clean, v12EvaluationRootMarkerName), data)
}

func loadV12CaseCheckpoints(root string, cases []CaseInput, marker evaluationRootMarker, resume bool) ([]capabilitybaselinev10.CaseEvidence, []bool, error) {
	records := make([]capabilitybaselinev10.CaseEvidence, len(cases))
	completed := make([]bool, len(cases))
	if !resume {
		return records, completed, nil
	}
	entries, err := os.ReadDir(filepath.Join(root, checkpointDirectoryName))
	if err != nil {
		return nil, nil, fmt.Errorf("read V12 evaluator checkpoints: %w", err)
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
				return nil, nil, fmt.Errorf("remove incomplete V12 checkpoint: %w", err)
			}
			continue
		}
		return nil, nil, fmt.Errorf("unexpected V12 evaluator checkpoint entry %q", entry.Name())
	}
	for index, input := range cases {
		data, err := readBoundedRegularFile(caseCheckpointPath(root, input.Entry.ID), 16*1024*1024)
		if errors.Is(err, os.ErrNotExist) {
			if err := os.RemoveAll(filepath.Join(root, input.Entry.ID)); err != nil {
				return nil, nil, fmt.Errorf("remove incomplete V12 case root: %w", err)
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
		if err := validateV12CheckpointRoots(root, input, marker, record); err != nil {
			return nil, nil, err
		}
		records[index], completed[index] = validated, true
	}
	return records, completed, nil
}

func validateV12CheckpointRoots(root string, input CaseInput, marker evaluationRootMarker, record capabilitybaselinev10.CaseEvidence) error {
	for replay := 1; replay <= 2; replay++ {
		path := filepath.Join(root, input.Entry.ID, fmt.Sprintf("replay-%d", replay), "CLEAN_ROOT.json")
		data, err := readBoundedRegularFile(path, 64*1024)
		if err != nil || hashBytes(data) != record.ReplayRootSHA256[replay-1] {
			return fmt.Errorf("%s replay %d V12 checkpoint root is invalid", input.Entry.ID, replay)
		}
		var actual cleanRootMarker
		if err := decodeStrictJSON(data, &actual); err != nil {
			return fmt.Errorf("%s replay %d V12 checkpoint root is malformed", input.Entry.ID, replay)
		}
		expected := cleanRootMarker{
			Schema: v12CleanRootSchema, Version: 12, CaseID: input.Entry.ID, Replay: replay,
			CorpusManifestSHA256: marker.CorpusManifestSHA256, RequirementSHA256: input.Entry.RequirementSHA256,
			EnvironmentSHA256: marker.EnvironmentSHA256, EvaluatorManifestSHA256: marker.EvaluatorManifestSHA256,
		}
		if !reflect.DeepEqual(actual, expected) {
			return fmt.Errorf("%s replay %d V12 checkpoint root binding differs", input.Entry.ID, replay)
		}
	}
	return nil
}
