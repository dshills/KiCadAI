package capabilitybaselinev10

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"

	"kicadai/internal/capabilityroundsv10"
)

var ErrInvalidEvidence = errors.New("invalid V10 generation-zero evidence")
var digestPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

var outcomeOrder = []string{"pass", "unsupported", "unsafe", "exhausted"}

func Build(corpusManifestSHA256 string, records []CaseEvidence) (Report, error) {
	if !digestPattern.MatchString(corpusManifestSHA256) || len(records) != capabilityroundsv10.FrozenPolicy().ExpectedDiscoveryCases {
		return Report{}, fmt.Errorf("%w: corpus binding or case count", ErrInvalidEvidence)
	}
	result := Report{Schema: ReportSchema, Version: Version, CorpusManifestSHA256: corpusManifestSHA256,
		CaseCount: len(records), Cases: make([]CaseEvidence, len(records))}
	counts := map[string]int{}
	for index, record := range records {
		wantID := fmt.Sprintf("v10_case_%03d", index+1)
		if record.Case.ID != wantID {
			return Report{}, fmt.Errorf("%w: case order", ErrInvalidEvidence)
		}
		validated, err := validateCase(record)
		if err != nil {
			return Report{}, fmt.Errorf("%s: %w", record.Case.ID, err)
		}
		if index == 0 {
			result.EnvironmentSHA256 = validated.EnvironmentSHA256
			result.EvaluatorManifestSHA256 = validated.EvaluatorManifestSHA256
		} else if validated.EnvironmentSHA256 != result.EnvironmentSHA256 || validated.EvaluatorManifestSHA256 != result.EvaluatorManifestSHA256 {
			return Report{}, fmt.Errorf("%w: environment or evaluator drift", ErrInvalidEvidence)
		}
		counts[validated.Case.Outcome]++
		result.Cases[index] = validated
	}
	for _, outcome := range outcomeOrder {
		result.OutcomeCounts = append(result.OutcomeCounts, OutcomeCount{Outcome: outcome, Count: counts[outcome]})
	}
	hash, err := reportHash(result)
	if err != nil {
		return Report{}, err
	}
	result.Hash = hash
	return result, nil
}

func Validate(report Report) error {
	if report.Schema != ReportSchema || report.Version != Version || report.CaseCount != capabilityroundsv10.FrozenPolicy().ExpectedDiscoveryCases ||
		!digestPattern.MatchString(report.CorpusManifestSHA256) || !digestPattern.MatchString(report.Hash) {
		return fmt.Errorf("%w: report envelope", ErrInvalidEvidence)
	}
	rebuilt, err := Build(report.CorpusManifestSHA256, report.Cases)
	if err != nil {
		return err
	}
	if report.EnvironmentSHA256 != rebuilt.EnvironmentSHA256 || report.EvaluatorManifestSHA256 != rebuilt.EvaluatorManifestSHA256 ||
		!slices.Equal(report.OutcomeCounts, rebuilt.OutcomeCounts) || report.Hash != rebuilt.Hash {
		return fmt.Errorf("%w: report does not reproduce", ErrInvalidEvidence)
	}
	return nil
}

func MarshalJSONStable(report Report) ([]byte, error) {
	if err := Validate(report); err != nil {
		return nil, err
	}
	return json.MarshalIndent(report, "", "  ")
}

func validateCase(record CaseEvidence) (CaseEvidence, error) {
	policy := capabilityroundsv10.FrozenPolicy()
	if record.Schema != CaseEvidenceSchema || record.Version != Version || record.Case.Role != "discovery" ||
		!policy.ReportingDomains[record.Case.ReportingDomain] || !policy.CircuitRoles[record.Case.CircuitRole] {
		return CaseEvidence{}, fmt.Errorf("%w: case envelope", ErrInvalidEvidence)
	}
	if _, exists := policy.SafetyWeights[record.Case.SafetyImpact]; !exists {
		return CaseEvidence{}, fmt.Errorf("%w: safety impact", ErrInvalidEvidence)
	}
	if !slices.Contains(outcomeOrder, record.Case.Outcome) {
		return CaseEvidence{}, fmt.Errorf("%w: terminal classification", ErrInvalidEvidence)
	}
	if !digestPattern.MatchString(record.RequirementSHA256) || !digestPattern.MatchString(record.EnvironmentSHA256) ||
		!digestPattern.MatchString(record.EvaluatorManifestSHA256) {
		return CaseEvidence{}, fmt.Errorf("%w: evidence binding digest", ErrInvalidEvidence)
	}
	if len(record.ReplaySHA256) != 2 || !digestPattern.MatchString(record.ReplaySHA256[0]) ||
		!digestPattern.MatchString(record.ReplaySHA256[1]) || record.ReplaySHA256[0] != record.ReplaySHA256[1] {
		return CaseEvidence{}, fmt.Errorf("%w: deterministic replay", ErrInvalidEvidence)
	}
	if len(record.ReplayRootSHA256) != 2 || !digestPattern.MatchString(record.ReplayRootSHA256[0]) ||
		!digestPattern.MatchString(record.ReplayRootSHA256[1]) || record.ReplayRootSHA256[0] == record.ReplayRootSHA256[1] {
		return CaseEvidence{}, fmt.Errorf("%w: distinct replay roots", ErrInvalidEvidence)
	}
	if err := validateFrontier(record.Case, policy); err != nil {
		return CaseEvidence{}, err
	}
	if !record.Gates.DeterministicReplay || !record.Gates.FailClosed {
		return CaseEvidence{}, fmt.Errorf("%w: replay or fail-closed gate", ErrInvalidEvidence)
	}
	if record.Case.Outcome == "pass" {
		if !record.Gates.allPassed() || len(record.Promotions) != 2 {
			return CaseEvidence{}, fmt.Errorf("%w: passing gate or promotion evidence", ErrInvalidEvidence)
		}
		if err := validatePromotions(record.Promotions); err != nil {
			return CaseEvidence{}, err
		}
	} else if record.Gates.allPassed() || len(record.Promotions) != 0 {
		return CaseEvidence{}, fmt.Errorf("%w: nonpass claims complete gates or promotion", ErrInvalidEvidence)
	}
	record.Hash = ""
	hash, err := caseHash(record)
	if err != nil {
		return CaseEvidence{}, err
	}
	record.Hash = hash
	return record, nil
}

func validateFrontier(value capabilityroundsv10.Case, policy capabilityroundsv10.Policy) error {
	if !sortedUnique(value.SatisfiedObligations) {
		return fmt.Errorf("%w: satisfied obligations", ErrInvalidEvidence)
	}
	satisfied := map[string]bool{}
	for _, anchor := range value.SatisfiedObligations {
		if !digestPattern.MatchString(anchor) {
			return fmt.Errorf("%w: satisfied obligation identity", ErrInvalidEvidence)
		}
		satisfied[anchor] = true
	}
	if value.Outcome == "pass" {
		if len(value.Frontier) != 0 {
			return fmt.Errorf("%w: passing case has a frontier", ErrInvalidEvidence)
		}
		return nil
	}
	if len(value.Frontier) == 0 {
		return fmt.Errorf("%w: nonpassing case has no frontier", ErrInvalidEvidence)
	}
	pathHashes := make([]string, 0, len(value.Frontier))
	for _, gap := range value.Frontier {
		if len(gap.Path) != 1 || satisfied[gap.ObligationAnchor] || !sortedUnique(gap.Diagnostics) || len(gap.Diagnostics) == 0 {
			return fmt.Errorf("%w: generation-zero root gap", ErrInvalidEvidence)
		}
		leaf := gap.Path[0]
		if policy.StageOrdinal[leaf.Stage] == 0 || !policy.GapCategories[leaf.Category] || leaf.Stage != leaf.Category ||
			!sortedUnique(leaf.RequiredEvidence) || len(leaf.RequiredEvidence) == 0 {
			return fmt.Errorf("%w: typed root leaf", ErrInvalidEvidence)
		}
		pathHash, err := capabilityroundsv10.PathHash(gap)
		if err != nil {
			return fmt.Errorf("%w: root path identity", ErrInvalidEvidence)
		}
		pathHashes = append(pathHashes, pathHash)
	}
	if !sortedUnique(pathHashes) {
		return fmt.Errorf("%w: frontier order or uniqueness", ErrInvalidEvidence)
	}
	return nil
}

func (value GateEvidence) allPassed() bool {
	return value.PrimitiveOnly && value.TopologySearch && value.Simulation && value.AllCorners && value.ModelProvenance &&
		value.ClosedLoopEvidence && value.CompleteRouting && value.Connectivity && value.WriterCorrectness && value.RoundTripZeroDiff &&
		value.ERC && value.StrictDRC && value.DeterministicReplay && value.FailClosed
}

func validatePromotions(promotions []PromotionEvidence) error {
	if len(promotions) != 2 || promotions[0].CleanRootSHA256 == promotions[1].CleanRootSHA256 ||
		promotions[0].RunSHA256 != promotions[1].RunSHA256 || promotions[0].ProjectSHA256 != promotions[1].ProjectSHA256 {
		return fmt.Errorf("%w: promotion replay identity", ErrInvalidEvidence)
	}
	for _, promotion := range promotions {
		if !digestPattern.MatchString(promotion.CleanRootSHA256) || !digestPattern.MatchString(promotion.RunSHA256) ||
			!digestPattern.MatchString(promotion.ProjectSHA256) || !promotion.InstalledKiCad || !promotion.ReplayIdentical {
			return fmt.Errorf("%w: promotion evidence", ErrInvalidEvidence)
		}
	}
	return nil
}

func caseHash(record CaseEvidence) (string, error) {
	data, err := json.Marshal(record)
	if err != nil {
		return "", err
	}
	return hashBytes(data), nil
}

func reportHash(report Report) (string, error) {
	report.Hash = ""
	data, err := json.Marshal(report)
	if err != nil {
		return "", err
	}
	return hashBytes(data), nil
}

func hashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func sortedUnique(values []string) bool {
	return slices.IsSorted(values) && !slices.Contains(values, "") && len(values) == len(slices.Compact(slices.Clone(values)))
}
