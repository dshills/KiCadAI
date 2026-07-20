package closedloopsynthesis

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"kicadai/internal/modelprovenance"
	"kicadai/internal/simmodel"
)

// SimulationResolver is the trusted boundary between a candidate state and a
// fully resolved simulation plan. Implementations must apply every variable,
// re-resolve catalog identities and connectivity, and return a fresh plan on
// every call.
type SimulationResolver interface {
	ResolveSimulation(context.Context, CandidateState) (SimulationResolution, error)
}

type SimulationResolution struct {
	Plan           simmodel.Plan               `json:"plan"`
	Measurements   []SimulationMeasurementLink `json:"measurements"`
	ModelDecisions []ModelDecision             `json:"model_decisions"`
}

// SimulationMeasurementLink maps one behavioral assertion to one trusted
// simmodel assertion result. The index is resolver-owned and refers to the
// final validated plan returned in the same resolution.
type SimulationMeasurementLink struct {
	RequirementID string `json:"requirement_id"`
	OperatingCase string `json:"operating_case"`
	Assertion     int    `json:"assertion"`
}

// SimModelEvaluator executes resolved plans only through the registered
// simmodel evaluator and converts its assertion results into closed-loop
// measurements. Provider-authored diagnostics never enter this boundary.
type SimModelEvaluator struct {
	Resolver           SimulationResolver
	ProvenanceRegistry modelprovenance.Registry
}

func (evaluator SimModelEvaluator) Evaluate(ctx context.Context, state CandidateState) (Evaluation, error) {
	if evaluator.Resolver == nil {
		return Evaluation{}, fmt.Errorf("simulation resolver is required")
	}
	resolution, err := evaluator.Resolver.ResolveSimulation(ctx, cloneState(state))
	if err != nil {
		return Evaluation{}, fmt.Errorf("resolve simulation: %w", err)
	}
	if diagnostics := validateSimulationResolution(resolution); len(diagnostics) != 0 {
		return Evaluation{}, fmt.Errorf("invalid simulation resolution: %s", joinDiagnosticMessages(diagnostics))
	}
	modelDecisions, modelDiagnostics := ResolvePlanModelDecisions(resolution.Plan, evaluator.ProvenanceRegistry)
	if len(modelDiagnostics) != 0 {
		return Evaluation{}, fmt.Errorf("model trust resolution failed: %s", joinDiagnosticMessages(modelDiagnostics))
	}
	// Provenance is derived after trusted resolution. Any resolver-supplied
	// decisions are replaced so they cannot become promotion evidence.
	resolution.ModelDecisions = modelDecisions
	report, diagnostics := simmodel.Evaluate(simmodel.ClonePlan(resolution.Plan))
	if len(diagnostics) != 0 && !onlyAssertionFailures(report, diagnostics) {
		return Evaluation{}, fmt.Errorf("trusted simulation failed: %s", joinSimModelDiagnostics(diagnostics))
	}
	measurements := make([]Measurement, 0, len(resolution.Measurements))
	for _, link := range resolution.Measurements {
		measurements = append(measurements, Measurement{
			RequirementID: link.RequirementID,
			OperatingCase: link.OperatingCase,
			Actual:        report.Assertions[link.Assertion].Actual,
		})
	}
	slices.SortStableFunc(measurements, compareMeasurements)
	evidenceHash, err := simulationEvidenceHash(resolution, report)
	if err != nil {
		return Evaluation{}, fmt.Errorf("hash simulation evidence: %w", err)
	}
	return Evaluation{EvidenceHash: evidenceHash, Measurements: measurements, ModelDecisions: cloneModelDecisions(resolution.ModelDecisions)}, nil
}

func validateSimulationResolution(resolution SimulationResolution) []Diagnostic {
	var diagnostics []Diagnostic
	for _, diagnostic := range simmodel.ValidatePlan(resolution.Plan) {
		diagnostics = append(diagnostics, Diagnostic{Path: "plan." + diagnostic.Path, Message: diagnostic.Message, Suggestion: diagnostic.Suggestion})
	}
	seenBehavior := map[string]bool{}
	seenAssertion := map[int]bool{}
	for index, link := range resolution.Measurements {
		path := fmt.Sprintf("measurements[%d]", index)
		if strings.TrimSpace(link.RequirementID) == "" || strings.TrimSpace(link.OperatingCase) == "" {
			diagnostics = append(diagnostics, Diagnostic{Path: path, Message: "simulation measurement link requires requirement and operating-case identities"})
		}
		if link.Assertion < 0 || link.Assertion >= len(resolution.Plan.Assertions) {
			diagnostics = append(diagnostics, Diagnostic{Path: path + ".assertion", Message: "simulation measurement link references an out-of-range assertion"})
		}
		behaviorKey := link.RequirementID + "\x00" + link.OperatingCase
		if seenBehavior[behaviorKey] {
			diagnostics = append(diagnostics, Diagnostic{Path: path, Message: "simulation measurement link duplicates a behavioral assertion"})
		}
		if seenAssertion[link.Assertion] {
			diagnostics = append(diagnostics, Diagnostic{Path: path + ".assertion", Message: "simulation assertion is mapped more than once"})
		}
		seenBehavior[behaviorKey] = true
		seenAssertion[link.Assertion] = true
	}
	if len(resolution.Measurements) == 0 {
		diagnostics = append(diagnostics, Diagnostic{Path: "measurements", Message: "simulation resolution requires behavioral measurement links"})
	}
	slices.SortStableFunc(diagnostics, compareDiagnostics)
	return diagnostics
}

func onlyAssertionFailures(report simmodel.Report, diagnostics []simmodel.Diagnostic) bool {
	if len(report.Assertions) == 0 {
		return false
	}
	for _, diagnostic := range diagnostics {
		if !strings.HasPrefix(diagnostic.Path, "assertions.") {
			return false
		}
	}
	return true
}

func simulationEvidenceHash(resolution SimulationResolution, report simmodel.Report) (string, error) {
	payload := struct {
		Resolution SimulationResolution `json:"resolution"`
		Report     simmodel.Report      `json:"report"`
	}{Resolution: resolution, Report: report}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

func joinDiagnosticMessages(diagnostics []Diagnostic) string {
	parts := make([]string, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		parts = append(parts, diagnostic.Path+": "+diagnostic.Message)
	}
	return strings.Join(parts, "; ")
}

func joinSimModelDiagnostics(diagnostics []simmodel.Diagnostic) string {
	parts := make([]string, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		parts = append(parts, diagnostic.Path+": "+diagnostic.Message)
	}
	return strings.Join(parts, "; ")
}

func compareMeasurements(left, right Measurement) int {
	if order := strings.Compare(left.RequirementID, right.RequirementID); order != 0 {
		return order
	}
	return strings.Compare(left.OperatingCase, right.OperatingCase)
}
