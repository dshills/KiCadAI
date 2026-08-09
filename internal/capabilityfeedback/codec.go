package capabilityfeedback

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"slices"

	"kicadai/internal/capabilityevaluation"
)

const maxArtifactBytes = 16 << 20

func DecodeCaseEvidence(reader io.Reader) (CaseEvidence, error) {
	var result CaseEvidence
	if err := decodeStrict(reader, &result); err != nil {
		return CaseEvidence{}, err
	}
	if err := ValidateCaseEvidence(result); err != nil {
		return CaseEvidence{}, err
	}
	return result, nil
}

func DecodeAggregateReport(reader io.Reader, registry capabilityevaluation.ImpactRegistry) (AggregateReport, error) {
	var result AggregateReport
	if err := decodeStrict(reader, &result); err != nil {
		return AggregateReport{}, err
	}
	if err := ValidateAggregateReport(result, registry); err != nil {
		return AggregateReport{}, err
	}
	return result, nil
}

func ValidateCaseEvidence(evidence CaseEvidence) error {
	if evidence.Schema != CaseEvidenceSchema || evidence.PolicyVersion != PolicyVersion {
		return fmt.Errorf("unsupported case-evidence schema or policy")
	}
	if err := validateCaseMeta(evidence.Case); err != nil {
		return err
	}
	if !validSHA256(evidence.RequirementHash) || !validSHA256(evidence.InventoryHash) ||
		!validSHA256(evidence.CatalogHash) || !validSHA256(evidence.ModelRegistryHash) ||
		!validSHA256(evidence.SynthesisPolicyHash) || !validSHA256(evidence.SynthesisHash) {
		return fmt.Errorf("case %q required content hashes are missing", evidence.Case.ID)
	}
	if !equalStrings(evidence.AnalysisKinds, normalizedStrings(evidence.AnalysisKinds)) ||
		!equalGaps(evidence.Gaps, normalizeGaps(evidence.Gaps)) {
		return fmt.Errorf("case %q evidence is not normalized", evidence.Case.ID)
	}
	for _, gap := range evidence.Gaps {
		switch gap.Scope {
		case ScopeTopology, ScopeComponent, ScopeModel, ScopeSimulation, ScopePhysical, ScopeRouting, ScopeVerification:
		default:
			return fmt.Errorf("case %q contains invalid gap scope %q", evidence.Case.ID, gap.Scope)
		}
		if gap.Stage == "" || gap.Capability == "" || gap.Code == "" || len(gap.RequiredEvidence) == 0 || len(gap.EvidenceHashes) == 0 {
			return fmt.Errorf("case %q contains an incomplete causal gap", evidence.Case.ID)
		}
		for _, hash := range gap.EvidenceHashes {
			if !validSHA256(hash) {
				return fmt.Errorf("case %q contains an invalid gap evidence hash", evidence.Case.ID)
			}
		}
	}
	switch evidence.Outcome {
	case OutcomePass:
		if len(evidence.Gaps) != 0 || !validSHA256(evidence.PromotionHash) || !validSHA256(evidence.ProjectHash) {
			return fmt.Errorf("passing case %q lacks complete promotion evidence", evidence.Case.ID)
		}
	case OutcomeUnsupported, OutcomeUnsafe, OutcomeExhausted:
		if len(evidence.Gaps) == 0 || evidence.ProjectHash != "" {
			return fmt.Errorf("non-passing case %q has invalid gap or project evidence", evidence.Case.ID)
		}
		if evidence.PromotionHash != "" && !validSHA256(evidence.PromotionHash) {
			return fmt.Errorf("non-passing case %q has an invalid promotion hash", evidence.Case.ID)
		}
	default:
		return fmt.Errorf("case %q has invalid outcome %q", evidence.Case.ID, evidence.Outcome)
	}
	expected, err := caseEvidenceHash(evidence)
	if err != nil || !validSHA256(evidence.Hash) || evidence.Hash != expected {
		return fmt.Errorf("case %q evidence hash mismatch", evidence.Case.ID)
	}
	return nil
}

func ValidateAggregateReport(report AggregateReport, registry capabilityevaluation.ImpactRegistry) error {
	if report.Schema != AggregateSchema || report.PolicyVersion != PolicyVersion || report.RankingPolicy != RankingPolicy {
		return fmt.Errorf("unsupported aggregate-report schema or policy")
	}
	rebuilt, err := Evaluate(report.CorpusRole, report.Cases, registry)
	if err != nil {
		return err
	}
	want, err := rebuilt.MarshalJSONStable()
	if err != nil {
		return err
	}
	got, err := report.MarshalJSONStable()
	if err != nil {
		return err
	}
	if !bytes.Equal(got, want) {
		return fmt.Errorf("aggregate report does not reproduce from case evidence and impact registry")
	}
	return nil
}

func decodeStrict(reader io.Reader, target any) error {
	limited := &io.LimitedReader{R: reader, N: maxArtifactBytes + 1}
	decoder := json.NewDecoder(limited)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		if limited.N == 0 {
			return fmt.Errorf("capability-feedback artifact exceeds %d-byte limit", maxArtifactBytes)
		}
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if limited.N == 0 {
			return fmt.Errorf("capability-feedback artifact exceeds %d-byte limit", maxArtifactBytes)
		}
		if err == nil {
			return fmt.Errorf("capability-feedback artifact contains trailing JSON")
		}
		return err
	}
	if limited.N == 0 {
		return fmt.Errorf("capability-feedback artifact exceeds %d-byte limit", maxArtifactBytes)
	}
	return nil
}

func equalStrings(left, right []string) bool {
	return slices.Equal(left, right)
}

func equalGaps(left, right []Gap) bool {
	return slices.EqualFunc(left, right, func(leftGap, rightGap Gap) bool {
		return leftGap.Stage == rightGap.Stage && leftGap.Scope == rightGap.Scope &&
			leftGap.Capability == rightGap.Capability && leftGap.Code == rightGap.Code &&
			equalStrings(leftGap.RequirementIDs, rightGap.RequirementIDs) &&
			equalStrings(leftGap.OperatingCases, rightGap.OperatingCases) &&
			equalStrings(leftGap.AnalysisKinds, rightGap.AnalysisKinds) &&
			equalStrings(leftGap.RequiredEvidence, rightGap.RequiredEvidence) &&
			equalStrings(leftGap.EvidenceHashes, rightGap.EvidenceHashes) &&
			equalStrings(leftGap.DownstreamSymptoms, rightGap.DownstreamSymptoms)
	})
}
