package componentonboarding

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"

	"kicadai/internal/components"
	"kicadai/internal/libraryresolver"
	"kicadai/internal/modelprovenance"
)

func Promote(
	candidate Candidate,
	documents []DocumentInput,
	gates []GateEvidence,
	approval Approval,
	base *components.Catalog,
	libraries libraryresolver.LibraryIndex,
) (Promotion, SupportedOverlay, error) {
	if err := validateCandidateForPromotion(candidate, documents, base, libraries); err != nil {
		return Promotion{}, SupportedOverlay{}, err
	}
	normalizedGates, err := validatePromotionGates(gates)
	if err != nil {
		return Promotion{}, SupportedOverlay{}, err
	}
	if approval.CandidateHash != candidate.Hash || approval.Decision != "approve" ||
		strings.TrimSpace(approval.Reviewer) == "" || strings.TrimSpace(approval.ReviewRef) == "" ||
		!validSHA256(approval.ReviewSHA256) {
		return Promotion{}, SupportedOverlay{}, fmt.Errorf("promotion requires a hash-bound independent approval")
	}
	promotion := Promotion{
		Schema: PromotionSchema, PolicyVersion: PolicyVersion,
		Candidate: candidate, Gates: normalizedGates, Approval: approval,
	}
	promotion.Hash, err = hashWithoutField(promotion)
	if err != nil {
		return Promotion{}, SupportedOverlay{}, err
	}
	records := make([]components.ComponentRecord, 0, len(candidate.Proposals))
	models := make([]modelprovenance.Record, 0, len(candidate.Proposals))
	for _, proposal := range candidate.Proposals {
		records = append(records, proposal.Record)
		models = append(models, proposal.Model.Provenance)
	}
	registry := modelprovenance.Normalize(modelprovenance.Registry{
		Schema: modelprovenance.Schema, Version: modelprovenance.Version, Records: models,
	})
	if diagnostics := modelprovenance.Validate(registry); len(diagnostics) != 0 {
		return Promotion{}, SupportedOverlay{}, fmt.Errorf("promoted model registry: %s: %s", diagnostics[0].Path, diagnostics[0].Message)
	}
	requirementHash, err := hashWithoutField(candidate.Requirement)
	if err != nil {
		return Promotion{}, SupportedOverlay{}, err
	}
	overlay := SupportedOverlay{
		Schema: OverlaySchema, PolicyVersion: PolicyVersion, Status: StatusSupported,
		RequirementHash: requirementHash, CandidateHash: candidate.Hash, PromotionHash: promotion.Hash,
		Records: records, Models: registry,
	}
	overlay.Hash, err = hashWithoutField(overlay)
	if err != nil {
		return Promotion{}, SupportedOverlay{}, err
	}
	if _, _, err := ApplyOverlay(base, modelprovenance.Registry{}, overlay, libraries); err != nil {
		return Promotion{}, SupportedOverlay{}, err
	}
	return promotion, overlay, nil
}

func BuildEvaluationEnvironment(
	candidate Candidate,
	documents []DocumentInput,
	base *components.Catalog,
	baseModels modelprovenance.Registry,
	libraries libraryresolver.LibraryIndex,
) (EvaluationEnvironment, error) {
	if err := validateCandidateForPromotion(candidate, documents, base, libraries); err != nil {
		return EvaluationEnvironment{}, err
	}
	records := make([]components.ComponentRecord, 0, len(candidate.Proposals))
	modelRecords := append([]modelprovenance.Record(nil), baseModels.Records...)
	for _, proposal := range candidate.Proposals {
		records = append(records, proposal.Record)
		modelRecords = append(modelRecords, proposal.Model.Provenance)
	}
	catalog, err := mergeCatalog(base, records)
	if err != nil {
		return EvaluationEnvironment{}, err
	}
	models := modelprovenance.Normalize(modelprovenance.Registry{
		Schema: modelprovenance.Schema, Version: modelprovenance.Version, Records: modelRecords,
	})
	if diagnostics := modelprovenance.Validate(models); len(diagnostics) != 0 {
		return EvaluationEnvironment{}, fmt.Errorf("evaluation model registry: %s: %s", diagnostics[0].Path, diagnostics[0].Message)
	}
	return EvaluationEnvironment{
		Status: StatusQuarantined, CandidateHash: candidate.Hash,
		Catalog: catalog, Models: models,
	}, nil
}

func ApplyOverlay(
	base *components.Catalog,
	baseModels modelprovenance.Registry,
	overlay SupportedOverlay,
	libraries libraryresolver.LibraryIndex,
) (*components.Catalog, modelprovenance.Registry, error) {
	if overlay.Schema != OverlaySchema || overlay.PolicyVersion != PolicyVersion ||
		overlay.Status != StatusSupported || overlay.Hash == "" {
		return nil, modelprovenance.Registry{}, fmt.Errorf("component overlay is not supported")
	}
	expectedHash, err := hashWithoutField(overlay)
	if err != nil || expectedHash != overlay.Hash {
		return nil, modelprovenance.Registry{}, fmt.Errorf("component overlay hash mismatch")
	}
	for _, record := range overlay.Records {
		if err := validateRecordLibraryBindings(record, libraries); err != nil {
			return nil, modelprovenance.Registry{}, fmt.Errorf("overlay component %q: %w", record.ID, err)
		}
	}
	catalog, err := mergeCatalog(base, overlay.Records)
	if err != nil {
		return nil, modelprovenance.Registry{}, err
	}
	modelRecords := append([]modelprovenance.Record(nil), baseModels.Records...)
	modelRecords = append(modelRecords, overlay.Models.Records...)
	models := modelprovenance.Normalize(modelprovenance.Registry{
		Schema: modelprovenance.Schema, Version: modelprovenance.Version, Records: modelRecords,
	})
	if diagnostics := modelprovenance.Validate(models); len(diagnostics) != 0 {
		return nil, modelprovenance.Registry{}, fmt.Errorf("combined model registry: %s: %s", diagnostics[0].Path, diagnostics[0].Message)
	}
	return catalog, models, nil
}

func validateCandidateForPromotion(
	candidate Candidate,
	documents []DocumentInput,
	base *components.Catalog,
	libraries libraryresolver.LibraryIndex,
) error {
	if candidate.Schema != CandidateSchema || candidate.PolicyVersion != PolicyVersion ||
		candidate.Status != StatusQuarantined || candidate.Hash == "" {
		return fmt.Errorf("only a quarantined onboarding candidate may be promoted")
	}
	expectedHash, err := hashWithoutField(candidate)
	if err != nil || expectedHash != candidate.Hash {
		return fmt.Errorf("candidate hash mismatch")
	}
	if err := ValidateRequirementAgainstCatalog(candidate.Requirement, base); err != nil {
		return err
	}
	records, content, err := ingestDocuments(documents)
	if err != nil {
		return err
	}
	if !slices.Equal(records, candidate.Documents) {
		return fmt.Errorf("promotion documents differ from candidate evidence")
	}
	claims, err := validateAndNormalizeClaims(candidate.Claims, records, content)
	if err != nil {
		return err
	}
	if !slices.Equal(claims, candidate.Claims) {
		return fmt.Errorf("candidate claims are not canonical")
	}
	for _, proposal := range candidate.Proposals {
		if err := validateProposal(candidate.Requirement, proposal, claims, records, base, libraries); err != nil {
			return fmt.Errorf("candidate %q: %w", proposal.Record.ID, err)
		}
	}
	if err := validateProposalSelections(candidate.Requirement, candidate.Proposals, base); err != nil {
		return err
	}
	scores := append([]CandidateScore(nil), candidate.Ranking...)
	rankScores(scores)
	if !slices.Equal(scores, candidate.Ranking) || len(scores) == 0 ||
		candidate.SelectedID != scores[0].ComponentID {
		return fmt.Errorf("candidate ranking or selection is not deterministic")
	}
	return nil
}

func validatePromotionGates(gates []GateEvidence) ([]GateEvidence, error) {
	normalized := append([]GateEvidence(nil), gates...)
	slices.SortStableFunc(normalized, func(left, right GateEvidence) int {
		if order := strings.Compare(left.Gate, right.Gate); order != 0 {
			return order
		}
		return left.Run - right.Run
	})
	expectedCount := len(RequiredPromotionGates) * 2
	if len(normalized) != expectedCount {
		return nil, fmt.Errorf("promotion requires exactly two runs of every physical gate")
	}
	for _, gate := range RequiredPromotionGates {
		var runHashes [2]string
		for run := 1; run <= 2; run++ {
			index, found := slices.BinarySearchFunc(normalized, GateEvidence{Gate: gate, Run: run}, func(value, target GateEvidence) int {
				if order := strings.Compare(value.Gate, target.Gate); order != 0 {
					return order
				}
				return value.Run - target.Run
			})
			if !found {
				return nil, fmt.Errorf("promotion lacks %s run %d", gate, run)
			}
			evidence := normalized[index]
			if !evidence.Passed || strings.TrimSpace(evidence.EvidencePath) == "" ||
				!validSHA256(evidence.EvidenceSHA256) {
				return nil, fmt.Errorf("promotion gate %s run %d did not pass with immutable evidence", gate, run)
			}
			runHashes[run-1] = evidence.EvidenceSHA256
		}
		if runHashes[0] != runHashes[1] {
			return nil, fmt.Errorf("promotion gate %s is not deterministic across two runs", gate)
		}
	}
	return normalized, nil
}

func validSHA256(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
