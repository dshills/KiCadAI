package simmodel

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

const SolverExecutionSchemaV23 = "kicadai.solver-execution.v23"
const SolverIDV23 = "kicadai_linear_active_state_v23"

// SolverExecutionV23 separates numerical solver revision from primitive-model
// identity. The trusted model registry and all historical solver APIs remain
// unchanged. Consumers must authenticate this envelope along with the report;
// the legacy-format report alone is not a V23 execution claim.
type SolverExecutionV23 struct {
	Schema                          string `json:"schema"`
	SolverID                        string `json:"solver_id"`
	SolverSHA256                    string `json:"solver_sha256"`
	Path                            string `json:"path"`
	PlanSHA256                      string `json:"plan_sha256"`
	ReportSHA256                    string `json:"report_sha256"`
	DiagnosticsSHA256               string `json:"diagnostics_sha256"`
	MaximumRecoverySeedsPerPoint    int    `json:"maximum_recovery_seeds_per_point"`
	MaximumAdditionalSolvesPerPoint int    `json:"maximum_additional_solves_per_point"`
	Hash                            string `json:"hash"`
}

// SolverSHA256V23 identifies the frozen algorithm contract; the evaluation's
// source manifest separately binds its exact implementation bytes and build.
func SolverSHA256V23() string {
	hash, _ := hashSolverValueV23(struct {
		ID                      string
		Revision                int
		Algorithm               string
		MaximumSeeds            int
		ActiveSetIterations     int
		PreserveOtherStates     bool
		RequireStability        bool
		RequireStateConsistency bool
	}{SolverIDV23, 23, "historical-active-set-then-one-inherited-opamp-clamp-release", 1, maxOpAmpActiveSetIterations, true, true, true})
	return hash
}

// EvaluateWithSolverV23 is explicitly opt-in. All noneligible workflows delegate
// to the historical engine byte-for-byte. No registry or default is changed.
func EvaluateWithSolverV23(plan Plan) (Report, []Diagnostic, SolverExecutionV23) {
	if _, err := hashSolverValueV23(plan); err != nil {
		return Report{Schema: ReportSchema, Status: "blocked"}, []Diagnostic{{Path: "solver_execution.plan", Message: "simulation plan cannot be authenticated"}}, SolverExecutionV23{}
	}
	var report Report
	var diagnostics []Diagnostic
	if !usesSolverV23(plan) {
		report, diagnostics = Evaluate(plan)
	} else if plan.WorstCase && len(plan.Uncertainties) == 0 {
		report, _ = evaluateNominalV23(plan)
		diagnostics = []Diagnostic{{Path: "uncertainties", Message: "worst-case proof requires reviewed bounded catalog uncertainty evidence", Suggestion: "select catalog components and source conditions with compatible tolerance evidence"}}
	} else if plan.WorstCase {
		report, diagnostics = evaluateWorstCaseV23(plan)
	} else {
		report, diagnostics = evaluateNominalV23(plan)
	}
	execution, err := solverExecutionV23(plan, report, diagnostics)
	if err != nil {
		report.Status = "blocked"
		diagnostics = append(diagnostics, Diagnostic{Path: "solver_execution", Message: "solver output cannot be authenticated"})
		return report, diagnostics, SolverExecutionV23{}
	}
	return report, diagnostics, execution
}

func VerifySolverExecutionV23(plan Plan, report Report, diagnostics []Diagnostic, execution SolverExecutionV23) error {
	if issues := ValidatePlan(plan); len(issues) != 0 {
		return fmt.Errorf("V23 execution plan is invalid")
	}
	if report.Schema != ReportSchema || report.RegistryHash != plan.RegistryHash || report.RegistryVersion != plan.RegistryVersion || report.ModelID != plan.ModelID || report.CatalogID != plan.CatalogID || report.CatalogHash != plan.CatalogHash || report.TopologyHash != plan.TopologyHash {
		return fmt.Errorf("V23 execution report is bound to another plan")
	}
	expected, err := solverExecutionV23(plan, report, diagnostics)
	if err != nil || execution != expected {
		return fmt.Errorf("V23 solver execution identity differs")
	}
	return nil
}

func solverExecutionV23(plan Plan, report Report, diagnostics []Diagnostic) (SolverExecutionV23, error) {
	result := SolverExecutionV23{Schema: SolverExecutionSchemaV23, SolverID: SolverIDV23, SolverSHA256: SolverSHA256V23(), Path: "historical_delegate"}
	if usesSolverV23(plan) {
		result.Path = "linear_dc_sweep_clamp_recovery"
		result.MaximumRecoverySeedsPerPoint = 1
		opAmps, active := 0, 0
		for _, device := range plan.Devices {
			switch device.PrimitiveModel {
			case PrimitiveOpAmpV1:
				opAmps++
				active++
			case PrimitiveComparatorOpenCollectorV1, PrimitiveCurrentSenseAmplifierV1:
				active++
			}
		}
		result.MaximumAdditionalSolvesPerPoint = opAmps + 2 + min(active*6+2, maxOpAmpActiveSetIterations)
	}
	var err error
	result.PlanSHA256, err = hashSolverValueV23(plan)
	if err != nil {
		return SolverExecutionV23{}, err
	}
	result.ReportSHA256, err = hashSolverValueV23(report)
	if err != nil {
		return SolverExecutionV23{}, err
	}
	result.DiagnosticsSHA256, err = hashSolverValueV23(diagnostics)
	if err != nil {
		return SolverExecutionV23{}, err
	}
	result.Hash, err = hashSolverValueV23(result)
	return result, err
}

func usesSolverV23(plan Plan) bool {
	if plan.ModelID != ModelLinearCircuitMNAV1 || len(compileNonlinearDevices(plan)) != 0 {
		return false
	}
	for _, analysis := range plan.Analyses {
		if analysis.Kind == AnalysisDCOperatingPoint && analysis.DCSweep != nil {
			return true
		}
	}
	return false
}

func evaluateNominalV23(plan Plan) (Report, []Diagnostic) {
	if !usesSolverV23(plan) {
		return evaluateNominal(plan)
	}
	report := Report{Schema: ReportSchema, RegistryVersion: plan.RegistryVersion, RegistryHash: plan.RegistryHash, CatalogID: plan.CatalogID, CatalogHash: plan.CatalogHash, ModelID: plan.ModelID, Bindings: append([]ResolvedBinding(nil), plan.Bindings...), Inputs: append([]NamedValue(nil), plan.Inputs...), GroundNode: plan.GroundNode, Nodes: append([]string(nil), plan.Nodes...), Devices: cloneDevices(plan.Devices), TopologyHash: plan.TopologyHash, Status: "blocked"}
	plan = indexMNAPlanDevices(plan)
	if diagnostics := ValidatePlan(plan); len(diagnostics) != 0 {
		return report, diagnostics
	}
	return evaluateMNAWithSolverV23(plan, report)
}

func hashSolverValueV23(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}
