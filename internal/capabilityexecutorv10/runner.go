package capabilityexecutorv10

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"sync"

	"kicadai/internal/capabilitybaselinev10"
	"kicadai/internal/capabilityevaluation"
	"kicadai/internal/capabilityfeedback"
	"kicadai/internal/modelprovenance"
	"kicadai/internal/opentopologysynthesis"
)

var digestPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

func (executor Executor) Run(ctx context.Context, request Request) (capabilitybaselinev10.Report, error) {
	if executor.decode == nil || executor.synthesize == nil || executor.promote == nil || executor.observe == nil {
		return capabilitybaselinev10.Report{}, fmt.Errorf("V10 evaluator engine is incomplete")
	}
	environmentSHA256, err := validateEnvironment(request.Environment)
	if err != nil {
		return capabilitybaselinev10.Report{}, err
	}
	if !digestPattern.MatchString(request.CorpusManifestSHA256) || len(request.Cases) != 24 {
		return capabilitybaselinev10.Report{}, fmt.Errorf("V10 corpus binding or discovery cohort is invalid")
	}
	if err := prepareOutputRoot(request.OutputRoot); err != nil {
		return capabilitybaselinev10.Report{}, err
	}
	records := make([]capabilitybaselinev10.CaseEvidence, len(request.Cases))
	for index, input := range request.Cases {
		wantID := fmt.Sprintf("v10_case_%03d", index+1)
		if input.Entry.ID != wantID {
			return capabilitybaselinev10.Report{}, fmt.Errorf("V10 case order differs at %d", index)
		}
	}
	errorsByIndex := make([]error, len(request.Cases))
	jobs := make(chan int, ParallelCaseLimit)
	var workers sync.WaitGroup
	for worker := 0; worker < ParallelCaseLimit; worker++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for index := range jobs {
				result, runErr := executor.runCase(ctx, request.OutputRoot, request.CorpusManifestSHA256, request.Cases[index], request.Environment, environmentSHA256)
				if runErr != nil {
					errorsByIndex[index] = runErr
					continue
				}
				records[index] = result.evidence
			}
		}()
	}
	for index := range request.Cases {
		jobs <- index
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

func (executor Executor) runCase(ctx context.Context, outputRoot string, corpusManifestSHA256 string, input CaseInput, environment Environment, environmentSHA256 string) (caseResult, error) {
	if err := validateEntry(input); err != nil {
		return caseResult{}, err
	}
	requirement, err := executor.decode(input.RequirementSource)
	if err != nil {
		return caseResult{}, err
	}

	rootSHA256 := make([]string, 0, 2)
	runs := make([]opentopologysynthesis.SynthesisRun, 0, 2)
	replaySHA256 := make([]string, 0, 2)
	caseRoot := filepath.Join(outputRoot, input.Entry.ID)
	if err := os.Mkdir(caseRoot, 0o755); err != nil {
		return caseResult{}, fmt.Errorf("create case root: %w", err)
	}
	for replay := 1; replay <= 2; replay++ {
		root := filepath.Join(caseRoot, fmt.Sprintf("replay-%d", replay))
		commitment, err := prepareCleanRoot(root, cleanRootMarker{
			Schema: cleanRootSchema, Version: 10, CaseID: input.Entry.ID, Replay: replay,
			CorpusManifestSHA256: corpusManifestSHA256, RequirementSHA256: input.Entry.RequirementSHA256,
			EnvironmentSHA256: environmentSHA256, EvaluatorManifestSHA256: environment.EvaluatorManifestSHA256,
		})
		if err != nil {
			return caseResult{}, err
		}
		run := executor.synthesize(ctx, requirement, environment.Inventory, environment.Simulation, environment.Policy)
		if run.Report.Status == opentopologysynthesis.StatusInvalid || run.Report.Status == opentopologysynthesis.StatusCanceled {
			return caseResult{}, fmt.Errorf("synthesis terminated with non-capability status %q", run.Report.Status)
		}
		data, err := json.Marshal(run)
		if err != nil {
			return caseResult{}, fmt.Errorf("marshal synthesis replay: %w", err)
		}
		rootSHA256 = append(rootSHA256, commitment)
		replaySHA256 = append(replaySHA256, hashBytes(data))
		runs = append(runs, run)
	}
	if replaySHA256[0] != replaySHA256[1] {
		return caseResult{}, fmt.Errorf("synthesis replay differs across clean roots")
	}

	var observedPromotion *opentopologysynthesis.PhysicalPromotionResult
	promotions := []capabilitybaselinev10.PromotionEvidence{}
	if runs[0].Report.Status == opentopologysynthesis.StatusPassed {
		results := make([]opentopologysynthesis.PhysicalPromotionResult, 0, 2)
		for replay := 1; replay <= 2; replay++ {
			result := executor.promote(ctx, runs[replay-1], environment.Simulation, opentopologysynthesis.PhysicalPromotionOptions{
				OutputRoot: filepath.Join(caseRoot, fmt.Sprintf("replay-%d", replay), "promotion"),
				KiCadCLI:   environment.KiCadCLI, LibraryIndex: environment.LibraryIndex,
				Timeout: environment.PromotionTimeout, KeepArtifacts: environment.KeepPhysicalPromotionArtifacts,
			})
			if err := validatePromotionCompletion(result); err != nil {
				return caseResult{}, err
			}
			results = append(results, result)
		}
		if results[0].Hash != results[1].Hash || results[0].Status != results[1].Status || results[0].ProjectHash != results[1].ProjectHash {
			return caseResult{}, fmt.Errorf("physical promotion replay differs across clean roots")
		}
		observedPromotion = &results[0]
		if results[0].Status == opentopologysynthesis.PhysicalPromotionPassed {
			for index, result := range results {
				promotions = append(promotions, capabilitybaselinev10.PromotionEvidence{
					CleanRootSHA256: rootSHA256[index], RunSHA256: result.Hash, ProjectSHA256: result.ProjectHash,
					InstalledKiCad: true, ReplayIdentical: result.ReplayIdentical,
				})
			}
		}
	}

	domain, err := feedbackDomain(input.Entry.Domain)
	if err != nil {
		return caseResult{}, err
	}
	observation, err := executor.observe(capabilityfeedback.CaseMeta{
		ID: input.Entry.ID, Role: capabilityfeedback.RoleDiscovery, Domain: domain,
		SafetyImpact: capabilityevaluation.SafetyImpact(input.Entry.SafetyImpact),
	}, requirement, runs[0], observedPromotion)
	if err != nil {
		return caseResult{}, fmt.Errorf("observe capability evidence: %w", err)
	}
	roundCase, err := buildRoundCase(input, observation)
	if err != nil {
		return caseResult{}, err
	}
	gates := synthesisGates(runs[0], observation, len(promotions) == 2)
	return caseResult{evidence: capabilitybaselinev10.CaseEvidence{
		Schema: capabilitybaselinev10.CaseEvidenceSchema, Version: capabilitybaselinev10.Version,
		Case: roundCase, RequirementSHA256: input.Entry.RequirementSHA256,
		EnvironmentSHA256: environmentSHA256, EvaluatorManifestSHA256: environment.EvaluatorManifestSHA256,
		ReplaySHA256: replaySHA256, ReplayRootSHA256: rootSHA256, Gates: gates, Promotions: promotions,
	}}, nil
}

func validPassingPromotion(result opentopologysynthesis.PhysicalPromotionResult) bool {
	if result.Status != opentopologysynthesis.PhysicalPromotionPassed || !result.ReplayIdentical ||
		!digestPattern.MatchString(result.Hash) || !digestPattern.MatchString(result.ProjectHash) || len(result.Runs) != 2 {
		return false
	}
	return result.Runs[0].Number == 1 && result.Runs[1].Number == 2 &&
		result.Runs[0].ProjectHash == result.ProjectHash && result.Runs[1].ProjectHash == result.ProjectHash
}

func validatePromotionCompletion(result opentopologysynthesis.PhysicalPromotionResult) error {
	if !digestPattern.MatchString(result.Hash) {
		return fmt.Errorf("physical promotion lacks a canonical evidence hash")
	}
	switch result.Status {
	case opentopologysynthesis.PhysicalPromotionPassed:
		if !validPassingPromotion(result) {
			return fmt.Errorf("physical promotion pass lacks two clean-root production runs")
		}
	case opentopologysynthesis.PhysicalPromotionFailed:
		if len(result.Issues) == 0 {
			return fmt.Errorf("physical promotion failure lacks diagnostics")
		}
	default:
		return fmt.Errorf("physical promotion returned nonterminal status %q", result.Status)
	}
	return nil
}

func validateEnvironment(environment Environment) (string, error) {
	modelRegistrySHA256, modelErr := modelprovenance.Hash(environment.Simulation.ModelRegistry)
	if !digestPattern.MatchString(environment.Inventory.Hash) || !digestPattern.MatchString(environment.Inventory.CatalogHash) ||
		!digestPattern.MatchString(environment.Inventory.ModelRegistryHash) || !digestPattern.MatchString(environment.PromotionEnvironmentSHA256) ||
		!digestPattern.MatchString(environment.KiCadCLISHA256) || !digestPattern.MatchString(environment.EvaluatorManifestSHA256) || environment.Simulation.Catalog == nil ||
		environment.Simulation.CatalogHash != environment.Inventory.CatalogHash || environment.LibraryIndex == nil || strings.TrimSpace(environment.KiCadCLI) == "" ||
		modelErr != nil || modelRegistrySHA256 != environment.Inventory.ModelRegistryHash ||
		!reflect.DeepEqual(environment.Policy, opentopologysynthesis.DefaultPolicy()) {
		return "", fmt.Errorf("V10 evaluator environment is incomplete or inconsistent")
	}
	type commitment struct {
		InventorySHA256            string                       `json:"inventory_sha256"`
		CatalogSHA256              string                       `json:"catalog_sha256"`
		ModelRegistrySHA256        string                       `json:"model_registry_sha256"`
		Policy                     opentopologysynthesis.Policy `json:"policy"`
		KiCadCLISHA256             string                       `json:"kicad_cli_sha256"`
		PromotionEnvironmentSHA256 string                       `json:"promotion_environment_sha256"`
		ParallelCaseLimit          int                          `json:"parallel_case_limit"`
	}
	data, err := json.Marshal(commitment{
		InventorySHA256: environment.Inventory.Hash, CatalogSHA256: environment.Inventory.CatalogHash,
		ModelRegistrySHA256: environment.Inventory.ModelRegistryHash, Policy: environment.Policy,
		KiCadCLISHA256:             environment.KiCadCLISHA256,
		PromotionEnvironmentSHA256: environment.PromotionEnvironmentSHA256,
		ParallelCaseLimit:          ParallelCaseLimit,
	})
	if err != nil {
		return "", err
	}
	return hashBytes(data), nil
}

func validateEntry(input CaseInput) error {
	entry := input.Entry
	if entry.Role != "discovery" || entry.Sealed || entry.ID == "" || entry.Domain == "" || entry.CircuitRole == "" ||
		entry.SafetyImpact == "" || !digestPattern.MatchString(entry.RequirementSHA256) || hashBytes(input.RequirementSource) != entry.RequirementSHA256 ||
		len(input.Obligations) == 0 {
		return fmt.Errorf("discovery input does not match its public manifest entry")
	}
	for _, obligation := range input.Obligations {
		if obligation.Role != "discovery" || obligation.CaseID != entry.ID || !digestPattern.MatchString(obligation.Anchor) {
			return fmt.Errorf("discovery obligation does not match case %q", entry.ID)
		}
	}
	return nil
}

func prepareOutputRoot(root string) error {
	if strings.TrimSpace(root) == "" {
		return fmt.Errorf("V10 evaluator output root is required")
	}
	clean := filepath.Clean(root)
	if err := os.MkdirAll(filepath.Dir(clean), 0o755); err != nil {
		return err
	}
	if err := os.Mkdir(clean, 0o755); err != nil {
		return fmt.Errorf("create fresh V10 evaluator output root: %w", err)
	}
	return nil
}

func prepareCleanRoot(root string, marker cleanRootMarker) (string, error) {
	if err := os.Mkdir(root, 0o755); err != nil {
		return "", fmt.Errorf("create clean replay root: %w", err)
	}
	data, err := json.Marshal(marker)
	if err != nil {
		return "", err
	}
	path := filepath.Join(root, "CLEAN_ROOT.json")
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o444)
	if err != nil {
		return "", fmt.Errorf("create clean-root commitment: %w", err)
	}
	_, writeErr := file.Write(data)
	closeErr := file.Close()
	if writeErr != nil {
		return "", writeErr
	}
	if closeErr != nil {
		return "", closeErr
	}
	return hashBytes(data), nil
}

func decodeRequirement(source []byte) (opentopologysynthesis.Requirement, error) {
	requirement, issues := opentopologysynthesis.DecodeStrict(bytes.NewReader(source))
	if len(issues) != 0 {
		return opentopologysynthesis.Requirement{}, fmt.Errorf("requirement violates the frozen contract")
	}
	return requirement, nil
}

func synthesisGates(run opentopologysynthesis.SynthesisRun, observation capabilityfeedback.CaseEvidence, promoted bool) capabilitybaselinev10.GateEvidence {
	if observation.Outcome == capabilityfeedback.OutcomePass && promoted {
		return capabilitybaselinev10.GateEvidence{
			PrimitiveOnly: true, TopologySearch: true, Simulation: true, AllCorners: true, ModelProvenance: true,
			ClosedLoopEvidence: true, CompleteRouting: true, Connectivity: true, WriterCorrectness: true,
			RoundTripZeroDiff: true, ERC: true, StrictDRC: true, DeterministicReplay: true, FailClosed: true,
		}
	}
	simulation := run.Report.Consumption.CandidateSimulations > 0
	return capabilitybaselinev10.GateEvidence{
		PrimitiveOnly: true, TopologySearch: true, Simulation: simulation, AllCorners: false,
		ModelProvenance:    simulation && digestPattern.MatchString(run.Report.ModelRegistryHash),
		ClosedLoopEvidence: true, DeterministicReplay: true, FailClosed: true,
	}
}

func feedbackDomain(value string) (capabilityevaluation.Domain, error) {
	switch value {
	case "analog_signal_path":
		return capabilityevaluation.DomainAnalog, nil
	case "power_energy_conversion":
		return capabilityevaluation.DomainPower, nil
	case "digital_control":
		return capabilityevaluation.DomainDigital, nil
	case "mixed_signal_data_conversion":
		return capabilityevaluation.DomainMixedSignal, nil
	case "sensing_instrumentation":
		return capabilityevaluation.DomainSensor, nil
	case "protection_power_integrity":
		return capabilityevaluation.DomainPower, nil
	default:
		return "", fmt.Errorf("unknown V10 reporting domain %q", value)
	}
}

func hashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
