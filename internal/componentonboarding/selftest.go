package componentonboarding

import (
	"context"
	"errors"
	"fmt"
	"kicadai/internal/circuitgraph"
	"kicadai/internal/components"
	"kicadai/internal/evidencebundle"
	"kicadai/internal/libraryresolver"
	"kicadai/internal/modelprovenance"
	"kicadai/internal/reports"
	"kicadai/internal/simmodel"
	"slices"
	"sort"
	"time"
)

const TestKitSchema = "kicadai.component-test-kit.v1"

type RegressionFixture struct {
	ID    string                `json:"id"`
	Graph circuitgraph.Document `json:"graph"`
}
type TestKit struct {
	Schema          string              `json:"schema"`
	CandidateSHA256 string              `json:"candidate_sha256"`
	Fixtures        []RegressionFixture `json:"fixtures"`
}
type FixtureResult struct {
	ID         string `json:"id"`
	Status     string `json:"status"`
	Stage      string `json:"stage"`
	Detail     string `json:"detail,omitempty"`
	ModelID    string `json:"model_id,omitempty"`
	PlanSHA256 string `json:"plan_sha256,omitempty"`
}
type SelfTestReport struct {
	Schema              string          `json:"schema"`
	Status              string          `json:"status"`
	CandidateSHA256     string          `json:"candidate_sha256"`
	CandidateStatus     Status          `json:"candidate_status"`
	RegistrySHA256      string          `json:"registry_sha256"`
	CandidateValidation string          `json:"candidate_validation"`
	Fixtures            []FixtureResult `json:"fixtures"`
	CoveredComponents   []string        `json:"covered_components"`
	Limitations         []string        `json:"limitations"`
}

// SelfTest revalidates evidence/pin maps/ratings and executes small candidate-
// bound regression circuits. It cannot promote a record or grant approval.
func SelfTest(ctx context.Context, candidate Candidate, documents []DocumentInput, base *components.Catalog, models modelprovenance.Registry, libraries libraryresolver.LibraryIndex, kit TestKit, output string) (SelfTestReport, error) {
	report := SelfTestReport{Schema: TestKitSchema + ".report", Status: "failed", CandidateSHA256: candidate.Hash, CandidateStatus: StatusQuarantined, RegistrySHA256: simmodel.RegistryHash(), CandidateValidation: "not_checked", Fixtures: []FixtureResult{}, CoveredComponents: []string{}, Limitations: []string{"Software tests only; candidate remains quarantined. No promotion, independent approval, physical validation, ERC/DRC or fabrication approval is produced.", "Pin functions, footprint pads, ratings, parameter bounds, excerpts and provenance are revalidated through the existing onboarding validator.", "Only supplied bounded regression cases are exercised; assertions are authored test expectations, not independent proof of a manufacturer's model."}}
	if err := evidencebundle.CheckDestination(output); err != nil {
		return report, err
	}
	if kit.Schema != TestKitSchema || kit.CandidateSHA256 != candidate.Hash || len(kit.Fixtures) < 1 || len(kit.Fixtures) > 8 {
		return report, errors.New("test kit requires matching candidate hash and 1–8 fixtures")
	}
	if len(documents) < 1 || len(documents) > 8 {
		return report, errors.New("self-test requires 1–8 source documents")
	}
	for _, document := range documents {
		if len(document.Content) > evidencebundle.MaxFile {
			return report, errors.New("self-test document exceeds 2 MiB")
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	files := map[string][]byte{}
	put := func(name string, value any) error {
		raw, err := evidencebundle.JSON(value)
		if err == nil {
			files[name] = raw
		}
		return err
	}
	if err := put("candidate.json", candidate); err != nil {
		return report, err
	}
	if err := put("test-kit.json", kit); err != nil {
		return report, err
	}
	for i, document := range documents {
		files[fmt.Sprintf("documents/%03d.txt", i)] = document.Content
	}
	environment, validationErr := BuildEvaluationEnvironment(candidate, documents, base, models, libraries)
	if validationErr == nil {
		report.CandidateValidation = "checked"
		report.Status = "passed"
		symbols, footprints := circuitgraph.LibraryEvidenceFromIndex(libraries)
		resolver := circuitgraph.NewResolver(circuitgraph.ResolveOptions{Catalog: environment.Catalog, CatalogID: "quarantined-self-test", LibrarySymbols: symbols, LibraryFootprints: footprints, RequireLibraryEvidence: true})
		covered := map[string]bool{}
		seen := map[string]bool{}
		for _, fixture := range kit.Fixtures {
			if err := ctx.Err(); err != nil {
				return report, err
			}
			result := FixtureResult{ID: fixture.ID, Status: "failed", Stage: "fixture_contract"}
			if !canonicalIDPattern.MatchString(fixture.ID) || seen[fixture.ID] || len(fixture.Graph.Components) > 16 || len(fixture.Graph.Nets) > 32 || fixture.Graph.Synthesis != nil || fixture.Graph.Simulation == nil || len(fixture.Graph.Simulation.Assertions) == 0 || len(fixture.Graph.Simulation.Analyses) < 1 || len(fixture.Graph.Simulation.Analyses) > 4 {
				result.Detail = "fixtures need unique canonical IDs, <=16 components/32 nets, no synthesis, and bounded simulation assertions"
				report.Status = "failed"
				report.Fixtures = append(report.Fixtures, result)
				continue
			}
			seen[fixture.ID] = true
			bounded := true
			for _, analysis := range fixture.Graph.Simulation.Analyses {
				switch analysis.Kind {
				case simmodel.AnalysisDCOperatingPoint:
				case simmodel.AnalysisACSweep:
					if analysis.Points > 256 {
						bounded = false
					}
				case simmodel.AnalysisTransient:
					if analysis.TimeStepS <= 0 || analysis.DurationS/analysis.TimeStepS > 1024 {
						bounded = false
					}
				default:
					bounded = false
				}
			}
			if !bounded {
				result.Detail = "self-test supports DC, <=256 AC points or <=1024 transient steps only"
				report.Status = "failed"
				report.Fixtures = append(report.Fixtures, result)
				continue
			}
			resolved, issues := resolver.Resolve(ctx, fixture.Graph)
			result.Stage = "catalog_resolution"
			if reports.HasBlockingIssue(issues) || resolved.Simulation == nil {
				result.Detail = fmt.Sprintf("%v", issues)
				report.Status = "failed"
				report.Fixtures = append(report.Fixtures, result)
				continue
			}
			plan := *resolved.Simulation
			allowed := true
			used := map[string]bool{}
			for _, device := range plan.Devices {
				used[device.CatalogID] = true
			}
			for _, binding := range plan.Bindings {
				used[binding.CatalogID] = true
			}
			candidateUsed := false
			for _, proposal := range candidate.Proposals {
				if used[proposal.Record.ID] {
					candidateUsed = true
					for _, analysis := range plan.Analyses {
						if !slices.Contains(proposal.Model.Provenance.Provenance.AllowedAnalyses, analysis.Kind) {
							allowed = false
						}
					}
				}
			}
			if !allowed {
				result.Detail = "fixture analysis exceeds candidate provenance allowance"
				report.Status = "failed"
				report.Fixtures = append(report.Fixtures, result)
				continue
			}
			if !candidateUsed {
				result.Detail = "fixture does not execute any candidate component"
				report.Status = "failed"
				report.Fixtures = append(report.Fixtures, result)
				continue
			}
			result.Stage = "simulation"
			simulation, diagnostics := simmodel.Evaluate(plan)
			if err := put("fixtures/"+fixture.ID+"/plan.json", plan); err != nil {
				return report, err
			}
			if err := put("fixtures/"+fixture.ID+"/simulation.json", simulation); err != nil {
				return report, err
			}
			if simulation.Status != "pass" || len(diagnostics) > 0 || len(simulation.Assertions) == 0 {
				result.Detail = fmt.Sprintf("%v", diagnostics)
				report.Status = "failed"
			} else {
				result.Status = "passed"
				result.Stage = "complete"
				for _, proposal := range candidate.Proposals {
					if used[proposal.Record.ID] {
						covered[proposal.Record.ID] = true
					}
				}
			}
			result.PlanSHA256 = evidencebundle.Digest(files["fixtures/"+fixture.ID+"/plan.json"])
			result.ModelID = plan.ModelID
			report.Fixtures = append(report.Fixtures, result)
		}
		for _, proposal := range candidate.Proposals {
			if covered[proposal.Record.ID] {
				report.CoveredComponents = append(report.CoveredComponents, proposal.Record.ID)
			} else {
				report.Status = "failed"
				report.Fixtures = append(report.Fixtures, FixtureResult{ID: proposal.Record.ID, Status: "failed", Stage: "candidate_coverage", Detail: "candidate lacks an executing regression fixture"})
			}
		}
		sort.Strings(report.CoveredComponents)
	} else {
		report.CandidateValidation = "failed"
		report.Fixtures = append(report.Fixtures, FixtureResult{ID: "candidate", Status: "failed", Stage: "candidate_validation", Detail: validationErr.Error()})
	}
	if err := ctx.Err(); err != nil {
		return report, err
	}
	if err := put("report.json", report); err != nil {
		return report, err
	}
	if err := evidencebundle.Publish(output, files); err != nil {
		return report, err
	}
	if report.Status != "passed" {
		return report, errors.New("component self-test failed; diagnostic bundle saved")
	}
	return report, nil
}

// VerifySelfTest checks portable integrity and identities, not numerical results
// or compatibility with the current model registry.
func VerifySelfTest(root string) error {
	if err := evidencebundle.Verify(root); err != nil {
		return err
	}
	raw, err := evidencebundle.Read(root, "report.json")
	if err != nil {
		return err
	}
	var report SelfTestReport
	if err = evidencebundle.Decode(raw, &report); err != nil {
		return err
	}
	if report.Schema != TestKitSchema+".report" || report.CandidateStatus != StatusQuarantined {
		return errors.New("unsupported component test report")
	}
	raw, err = evidencebundle.Read(root, "candidate.json")
	if err != nil {
		return err
	}
	var candidate Candidate
	if err = evidencebundle.Decode(raw, &candidate); err != nil {
		return err
	}
	if candidate.Hash != report.CandidateSHA256 {
		return errors.New("candidate/report identity differs")
	}
	raw, err = evidencebundle.Read(root, "test-kit.json")
	if err != nil {
		return err
	}
	var kit TestKit
	if err = evidencebundle.Decode(raw, &kit); err != nil {
		return err
	}
	if kit.Schema != TestKitSchema || kit.CandidateSHA256 != candidate.Hash {
		return errors.New("kit/candidate identity differs")
	}
	for _, fixture := range report.Fixtures {
		if fixture.PlanSHA256 != "" {
			raw, err = evidencebundle.Read(root, "fixtures/"+fixture.ID+"/plan.json")
			if err != nil || evidencebundle.Digest(raw) != fixture.PlanSHA256 {
				return errors.New("fixture plan identity differs")
			}
		}
	}
	return nil
}
