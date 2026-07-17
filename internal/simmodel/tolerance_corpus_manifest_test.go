package simmodel

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"

	"kicadai/internal/testsupport/tolerancecorpus"
	capabilityspec "kicadai/specs/tolerance-sensitivity"
)

type toleranceCorpusManifest = tolerancecorpus.Manifest
type toleranceCorpusManifestCase = tolerancecorpus.Case

type toleranceCapabilityReport struct {
	Schema               string                       `json:"schema"`
	GeneratedAt          string                       `json:"generated_at"`
	CorpusManifestSHA256 string                       `json:"corpus_manifest_sha256"`
	RegistryVersion      string                       `json:"registry_version"`
	RegistrySHA256       string                       `json:"registry_sha256"`
	PromotionGateProfile map[string]string            `json:"promotion_gate_profile"`
	Cases                []toleranceCapabilityCase    `json:"cases"`
	Aggregate            toleranceCapabilityAggregate `json:"aggregate"`
}

type toleranceCapabilityCase struct {
	ID                    string                        `json:"id"`
	Category              string                        `json:"category"`
	NominalStatus         string                        `json:"nominal_status"`
	WorstCaseStatus       string                        `json:"worst_case_status"`
	FailureTaxonomy       string                        `json:"failure_taxonomy,omitempty"`
	PlanSHA256            string                        `json:"plan_sha256"`
	CatalogEvidenceSHA256 string                        `json:"catalog_evidence_sha256"`
	NominalReportSHA256   string                        `json:"nominal_report_sha256"`
	WorstReportSHA256     string                        `json:"worst_report_sha256"`
	DominantSensitivity   *toleranceDominantSensitivity `json:"dominant_sensitivity,omitempty"`
	PromotionCorpus       string                        `json:"promotion_corpus,omitempty"`
	PromotionFixture      string                        `json:"promotion_fixture,omitempty"`
}

type toleranceDominantSensitivity struct {
	Assertion string  `json:"assertion"`
	Target    string  `json:"target"`
	Corner    string  `json:"corner"`
	Margin    float64 `json:"margin"`
}

type toleranceCapabilityAggregate struct {
	Cases            int            `json:"cases"`
	NominalPassed    int            `json:"nominal_passed"`
	WorstCasePassed  int            `json:"worst_case_passed"`
	WorstCaseBlocked int            `json:"worst_case_blocked"`
	FailureTaxonomy  map[string]int `json:"failure_taxonomy"`
	ByCategory       map[string]int `json:"by_category"`
}

func TestFrozenToleranceCorpusManifestAndCapabilityReport(t *testing.T) {
	manifest, manifestBytes := loadToleranceCorpusManifest(t)
	if manifest.Schema != "kicadai.tolerance-corpus.v1" || manifest.Version != 1 {
		t.Fatalf("unexpected manifest identity: %#v", manifest)
	}
	if manifest.RegistryVersion != RegistryVersion || manifest.RegistrySHA256 == "" {
		t.Fatalf("manifest registry evidence is stale: %#v", manifest)
	}

	fixtures := map[string]toleranceCorpusCase{}
	for _, fixture := range toleranceCorpusCases() {
		fixtures[fixture.ID] = fixture
	}
	if len(manifest.Cases) != len(fixtures) {
		t.Fatalf("manifest cases=%d fixtures=%d", len(manifest.Cases), len(fixtures))
	}
	seen := map[string]bool{}
	for _, entry := range manifest.Cases {
		fixture, ok := fixtures[entry.ID]
		if !ok || seen[entry.ID] {
			t.Fatalf("unknown or duplicate manifest case %q", entry.ID)
		}
		seen[entry.ID] = true
		plan := fixture.Build(t, false)
		if fixture.Category != entry.Category || fixture.Expected != entry.Expected {
			t.Fatalf("%s metadata drift: fixture=%#v manifest=%#v", entry.ID, fixture, entry)
		}
		if got := canonicalPlanHash(t, plan); got != entry.PlanSHA256 {
			t.Fatalf("%s plan hash=%s want=%s", entry.ID, got, entry.PlanSHA256)
		}
		if plan.CatalogHash != entry.CatalogEvidenceSHA256 || plan.RegistryHash != manifest.RegistrySHA256 {
			t.Fatalf("%s evidence drift: plan=%#v manifest=%#v", entry.ID, plan, entry)
		}
		assertTolerancePromotionLink(t, entry)
	}

	want := buildToleranceCapabilityReport(t, manifest, manifestBytes, fixtures)
	var got toleranceCapabilityReport
	if err := json.Unmarshal(capabilityspec.CapabilityReport, &got); err != nil {
		t.Fatal(err)
	}
	gotComparable := got
	wantComparable := want
	gotComparable.Cases = append([]toleranceCapabilityCase(nil), got.Cases...)
	wantComparable.Cases = append([]toleranceCapabilityCase(nil), want.Cases...)
	if len(gotComparable.Cases) != len(wantComparable.Cases) {
		t.Fatalf("capability report cases=%d want=%d", len(gotComparable.Cases), len(wantComparable.Cases))
	}
	for index := range wantComparable.Cases {
		gotSensitivity := gotComparable.Cases[index].DominantSensitivity
		wantSensitivity := wantComparable.Cases[index].DominantSensitivity
		if (gotSensitivity == nil) != (wantSensitivity == nil) {
			t.Fatalf("%s dominant sensitivity presence changed", wantComparable.Cases[index].ID)
		}
		if gotSensitivity != nil {
			gotSensitivityCopy := *gotSensitivity
			wantSensitivityCopy := *wantSensitivity
			gotComparable.Cases[index].DominantSensitivity = &gotSensitivityCopy
			wantComparable.Cases[index].DominantSensitivity = &wantSensitivityCopy
			gotSensitivity = &gotSensitivityCopy
			wantSensitivity = &wantSensitivityCopy
			scale := math.Max(1, math.Abs(wantSensitivity.Margin))
			if math.Abs(gotSensitivity.Margin-wantSensitivity.Margin) > 1e-12*scale {
				t.Fatalf("%s sensitivity margin=%g want=%g", wantComparable.Cases[index].ID, gotSensitivity.Margin, wantSensitivity.Margin)
			}
			gotSensitivity.Margin = 0
			wantSensitivity.Margin = 0
		}
	}
	if !reflect.DeepEqual(gotComparable, wantComparable) {
		wantBytes, _ := json.MarshalIndent(want, "", "  ")
		t.Fatalf("capability report is stale; regenerate specs/tolerance-sensitivity/CAPABILITY_REPORT.json\n%s", wantBytes)
	}
}

func buildToleranceCapabilityReport(t *testing.T, manifest toleranceCorpusManifest, manifestBytes []byte, fixtures map[string]toleranceCorpusCase) toleranceCapabilityReport {
	t.Helper()
	report := toleranceCapabilityReport{
		Schema: "kicadai.tolerance-capability.v1", GeneratedAt: manifest.FrozenAt,
		CorpusManifestSHA256: hashToleranceBytes(manifestBytes), RegistryVersion: manifest.RegistryVersion, RegistrySHA256: manifest.RegistrySHA256,
		PromotionGateProfile: map[string]string{
			"byte_identical_replay": "required", "connectivity": "required", "kicad_erc": "required",
			"route_completion": "required", "strict_kicad_drc": "required", "writer_correctness": "required", "zero_round_trip_diffs": "required",
		},
		Aggregate: toleranceCapabilityAggregate{FailureTaxonomy: map[string]int{}, ByCategory: map[string]int{}},
	}
	for _, entry := range manifest.Cases {
		plan := fixtures[entry.ID].Build(t, false)
		nominalPlan := ClonePlan(plan)
		nominalPlan.WorstCase = false
		nominalPlan.Uncertainties = nil
		nominal, nominalDiagnostics := Evaluate(nominalPlan)
		worst, worstDiagnostics := Evaluate(plan)
		if len(nominalDiagnostics) != 0 || nominal.Status != "pass" {
			t.Fatalf("%s nominal proof changed: report=%#v diagnostics=%#v", entry.ID, nominal, nominalDiagnostics)
		}
		assertFailedCornersAttributed(t, entry.ID, worst)
		item := toleranceCapabilityCase{
			ID: entry.ID, Category: entry.Category, NominalStatus: nominal.Status, WorstCaseStatus: worst.Status,
			PlanSHA256: entry.PlanSHA256, CatalogEvidenceSHA256: entry.CatalogEvidenceSHA256,
			NominalReportSHA256: canonicalReportHash(t, nominal), WorstReportSHA256: canonicalReportHash(t, worst),
			PromotionCorpus: entry.PromotionCorpus, PromotionFixture: entry.PromotionFixture,
		}
		if dominant, ok := dominantToleranceSensitivity(worst.Sensitivity); ok {
			item.DominantSensitivity = &toleranceDominantSensitivity{Assertion: dominant.Assertion, Target: dominant.Target, Corner: dominant.Corner, Margin: dominant.Margin}
		}
		if worst.Status == "blocked" {
			if len(worstDiagnostics) == 0 {
				t.Fatalf("%s blocked without diagnostics", entry.ID)
			}
			item.FailureTaxonomy = "assertion_out_of_bounds"
			report.Aggregate.WorstCaseBlocked++
			report.Aggregate.FailureTaxonomy[item.FailureTaxonomy]++
		} else {
			if len(worstDiagnostics) != 0 || worst.Status != "pass" {
				t.Fatalf("%s unexpected worst-case proof: report=%#v diagnostics=%#v", entry.ID, worst, worstDiagnostics)
			}
			report.Aggregate.WorstCasePassed++
		}
		report.Aggregate.Cases++
		report.Aggregate.NominalPassed++
		report.Aggregate.ByCategory[entry.Category]++
		report.Cases = append(report.Cases, item)
	}
	return report
}

func dominantToleranceSensitivity(results []SensitivityResult) (SensitivityResult, bool) {
	if len(results) == 0 {
		return SensitivityResult{}, false
	}
	dominant := results[0]
	for _, candidate := range results[1:] {
		if candidate.Margin < dominant.Margin || (candidate.Margin == dominant.Margin && compareToleranceSensitivity(candidate, dominant) < 0) {
			dominant = candidate
		}
	}
	return dominant, true
}

func compareToleranceSensitivity(left, right SensitivityResult) int {
	if compared := strings.Compare(left.Assertion, right.Assertion); compared != 0 {
		return compared
	}
	if compared := strings.Compare(left.Target, right.Target); compared != 0 {
		return compared
	}
	return strings.Compare(left.Corner, right.Corner)
}

func assertFailedCornersAttributed(t *testing.T, id string, report Report) {
	t.Helper()
	for _, corner := range report.Corners {
		for _, assertion := range corner.Assertions {
			if assertion.Pass {
				continue
			}
			attributed := false
			identity := assertionID(assertion)
			for _, sensitivity := range report.Sensitivity {
				if sensitivity.Assertion == identity && sensitivity.Target != "" && sensitivity.Corner != "" {
					attributed = true
					break
				}
			}
			if !attributed {
				t.Fatalf("%s failed assertion %#v at corner %q has no dominant target/corner attribution: %#v", id, assertion, corner.ID, report.Sensitivity)
			}
		}
	}
}

func assertTolerancePromotionLink(t *testing.T, entry toleranceCorpusManifestCase) {
	t.Helper()
	if entry.Expected != "worst_case_pass" {
		if entry.PromotionCorpus != "" || entry.PromotionFixture != "" {
			t.Fatalf("blocked case %s must not claim fabrication promotion", entry.ID)
		}
		return
	}
	validCorpus := entry.PromotionCorpus == "function" || entry.PromotionCorpus == "adversarial"
	if !validCorpus || entry.PromotionFixture == "" {
		t.Fatalf("passing case %s has no reviewed promotion carrier", entry.ID)
	}
}

func loadToleranceCorpusManifest(t *testing.T) (toleranceCorpusManifest, []byte) {
	t.Helper()
	manifest, contents, err := tolerancecorpus.Load()
	if err != nil {
		t.Fatal(err)
	}
	return manifest, contents
}

func hashToleranceBytes(contents []byte) string {
	digest := sha256.Sum256(contents)
	return hex.EncodeToString(digest[:])
}
