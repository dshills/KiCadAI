package capabilitygate

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

const (
	AssessmentSchema        = "kicadai.capability-assessment.v1"
	AssessmentPolicyVersion = "capability-evidence-policy-v1"
)

// Classification is the evidence-backed generation envelope for one request.
type Classification string

const (
	ClassificationSupported    Classification = "supported"
	ClassificationExperimental Classification = "experimental"
	ClassificationUnsupported  Classification = "unsupported"
)

type RequirementKind string

const (
	RequirementDomain       RequirementKind = "domain"
	RequirementArchitecture RequirementKind = "architecture"
	RequirementComponent    RequirementKind = "component"
	RequirementModel        RequirementKind = "model"
	RequirementPhysical     RequirementKind = "physical"
	RequirementVerification RequirementKind = "verification"
)

type EvidenceStatus string

const (
	EvidenceVerified EvidenceStatus = "verified"
	EvidenceInferred EvidenceStatus = "inferred"
	EvidenceMissing  EvidenceStatus = "missing"
	EvidenceFailed   EvidenceStatus = "failed"
)

// Requirement records one normalized capability claim and the evidence IDs
// that justify it.
type Requirement struct {
	Kind        RequirementKind `json:"kind"`
	ID          string          `json:"id"`
	Description string          `json:"description,omitempty"`
	EvidenceIDs []string        `json:"evidence_ids"`
}

// Evidence links a capability claim to a reproducible source. Verified
// evidence requires a stable source and SHA-256 digest.
type Evidence struct {
	ID          string         `json:"id"`
	Kind        string         `json:"kind"`
	Status      EvidenceStatus `json:"status"`
	Advisory    bool           `json:"advisory,omitempty"`
	Source      string         `json:"source,omitempty"`
	Digest      string         `json:"digest,omitempty"`
	Stage       string         `json:"stage,omitempty"`
	Description string         `json:"description,omitempty"`
}

type Gap struct {
	Code   string          `json:"code"`
	Kind   RequirementKind `json:"kind,omitempty"`
	ID     string          `json:"id,omitempty"`
	Stage  string          `json:"stage,omitempty"`
	Reason string          `json:"reason"`
	Action string          `json:"action,omitempty"`
}

type Risk struct {
	Code       string `json:"code"`
	Stage      string `json:"stage,omitempty"`
	Summary    string `json:"summary"`
	Mitigation string `json:"mitigation,omitempty"`
}

type Checkpoint struct {
	Stage          string         `json:"stage"`
	Classification Classification `json:"classification"`
	EvidenceIDs    []string       `json:"evidence_ids,omitempty"`
	GapCodes       []string       `json:"gap_codes,omitempty"`
	RiskCodes      []string       `json:"risk_codes,omitempty"`
}

// Assessment is the stable capability decision embedded in workflow,
// promotion, and creation evidence.
type Assessment struct {
	Schema                   string         `json:"schema"`
	PolicyVersion            string         `json:"policy_version"`
	Classification           Classification `json:"classification"`
	ExperimentalOptIn        bool           `json:"experimental_opt_in"`
	FabricationReadyEligible bool           `json:"fabrication_ready_eligible"`
	Requirements             []Requirement  `json:"requirements"`
	Evidence                 []Evidence     `json:"evidence"`
	Gaps                     []Gap          `json:"gaps,omitempty"`
	Risks                    []Risk         `json:"risks,omitempty"`
	Checkpoints              []Checkpoint   `json:"checkpoints"`
	Hash                     string         `json:"hash,omitempty"`
}

type Input struct {
	Stage             string
	ExperimentalOptIn bool
	Requirements      []Requirement
	Evidence          []Evidence
	Gaps              []Gap
	Risks             []Risk
}

type CheckpointInput struct {
	Stage        string
	Requirements []Requirement
	Evidence     []Evidence
	Gaps         []Gap
	Risks        []Risk
}

// Assess creates a deterministic evidence-linked capability decision.
func Assess(input Input) (Assessment, error) {
	assessment := Assessment{
		Schema:            AssessmentSchema,
		PolicyVersion:     AssessmentPolicyVersion,
		ExperimentalOptIn: input.ExperimentalOptIn,
		Requirements:      append([]Requirement(nil), input.Requirements...),
		Evidence:          append([]Evidence(nil), input.Evidence...),
		Gaps:              append([]Gap(nil), input.Gaps...),
		Risks:             append([]Risk(nil), input.Risks...),
	}
	normalizeAssessment(&assessment)
	classification, err := deriveClassification(assessment)
	if err != nil {
		return Assessment{}, err
	}
	assessment.Classification = classification
	assessment.FabricationReadyEligible = classification == ClassificationSupported
	assessment.Checkpoints = []Checkpoint{checkpointForAssessment(strings.TrimSpace(input.Stage), assessment)}
	if assessment.Checkpoints[0].Stage == "" {
		assessment.Checkpoints[0].Stage = "initial"
	}
	if err := sealAssessment(&assessment); err != nil {
		return Assessment{}, err
	}
	return assessment, nil
}

// Reassess appends one deterministic checkpoint. Classification is monotonic:
// later evidence may preserve or reduce confidence, but never upgrade it.
func Reassess(previous Assessment, input CheckpointInput) (Assessment, error) {
	if err := Validate(previous); err != nil {
		return Assessment{}, fmt.Errorf("previous assessment: %w", err)
	}
	next := cloneAssessment(previous)
	next.Hash = ""
	next.Requirements = append(next.Requirements, input.Requirements...)
	next.Evidence = append(next.Evidence, input.Evidence...)
	next.Gaps = append(next.Gaps, input.Gaps...)
	next.Risks = append(next.Risks, input.Risks...)
	normalizeAssessment(&next)
	derived, err := deriveClassification(next)
	if err != nil {
		return Assessment{}, err
	}
	next.Classification = worseClassification(previous.Classification, derived)
	next.FabricationReadyEligible = next.Classification == ClassificationSupported
	checkpoint := checkpointForInput(strings.TrimSpace(input.Stage), next.Classification, input)
	if checkpoint.Stage == "" {
		return Assessment{}, fmt.Errorf("checkpoint stage is required")
	}
	next.Checkpoints = append(next.Checkpoints, checkpoint)
	if err := sealAssessment(&next); err != nil {
		return Assessment{}, err
	}
	return next, nil
}

func (assessment Assessment) AllowsFabricationReadyClaim() bool {
	return assessment.Classification == ClassificationSupported && assessment.FabricationReadyEligible
}

// Digest returns a deterministic SHA-256 digest for JSON-serializable evidence.
// Callers pass immutable evidence DTO snapshots. encoding/json sorts string map
// keys, providing a stable representation without an additional decode/encode
// pass.
func Digest(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// MarshalJSONStable validates and serializes an assessment deterministically.
func (assessment Assessment) MarshalJSONStable() ([]byte, error) {
	if err := Validate(assessment); err != nil {
		return nil, err
	}
	return json.Marshal(assessment)
}

func Validate(assessment Assessment) error {
	if assessment.Schema != AssessmentSchema {
		return fmt.Errorf("unsupported assessment schema %q", assessment.Schema)
	}
	if assessment.PolicyVersion != AssessmentPolicyVersion {
		return fmt.Errorf("unsupported assessment policy %q", assessment.PolicyVersion)
	}
	if !validClassification(assessment.Classification) {
		return fmt.Errorf("unsupported classification %q", assessment.Classification)
	}
	if assessment.FabricationReadyEligible != (assessment.Classification == ClassificationSupported) {
		return fmt.Errorf("fabrication eligibility disagrees with classification")
	}
	if len(assessment.Requirements) == 0 {
		return fmt.Errorf("at least one capability requirement is required")
	}
	if len(assessment.Checkpoints) == 0 {
		return fmt.Errorf("at least one capability checkpoint is required")
	}
	evidenceByID := make(map[string]Evidence, len(assessment.Evidence))
	for _, evidence := range assessment.Evidence {
		if strings.TrimSpace(evidence.ID) == "" || strings.TrimSpace(evidence.Kind) == "" {
			return fmt.Errorf("evidence id and kind are required")
		}
		if _, exists := evidenceByID[evidence.ID]; exists {
			return fmt.Errorf("duplicate evidence id %q", evidence.ID)
		}
		if !validEvidenceStatus(evidence.Status) {
			return fmt.Errorf("evidence %q has unsupported status %q", evidence.ID, evidence.Status)
		}
		if evidence.Status == EvidenceVerified && (strings.TrimSpace(evidence.Source) == "" || !validSHA256(evidence.Digest)) {
			return fmt.Errorf("verified evidence %q requires source and SHA-256 digest", evidence.ID)
		}
		evidenceByID[evidence.ID] = evidence
	}
	requiredEvidence := make(map[string]struct{})
	for _, requirement := range assessment.Requirements {
		if !validRequirementKind(requirement.Kind) || strings.TrimSpace(requirement.ID) == "" {
			return fmt.Errorf("requirement kind and id are required")
		}
		if len(requirement.EvidenceIDs) == 0 {
			return fmt.Errorf("requirement %s:%s has no evidence", requirement.Kind, requirement.ID)
		}
		for _, evidenceID := range requirement.EvidenceIDs {
			if _, ok := evidenceByID[evidenceID]; !ok {
				return fmt.Errorf("requirement %s:%s references missing evidence %q", requirement.Kind, requirement.ID, evidenceID)
			}
			requiredEvidence[evidenceID] = struct{}{}
		}
	}
	for _, evidence := range assessment.Evidence {
		_, required := requiredEvidence[evidence.ID]
		if evidence.Advisory && required {
			return fmt.Errorf("required evidence %q cannot be advisory", evidence.ID)
		}
		if evidence.Status == EvidenceInferred && !required && !evidence.Advisory {
			return fmt.Errorf("unlinked inferred evidence %q must be explicitly advisory", evidence.ID)
		}
	}
	derived, err := deriveClassification(assessment)
	if err != nil {
		return err
	}
	if classificationRank(assessment.Classification) < classificationRank(derived) {
		return fmt.Errorf("classification %q is stronger than its evidence-derived classification %q", assessment.Classification, derived)
	}
	for index, checkpoint := range assessment.Checkpoints {
		if strings.TrimSpace(checkpoint.Stage) == "" || !validClassification(checkpoint.Classification) {
			return fmt.Errorf("checkpoint %d is invalid", index)
		}
		if index > 0 && classificationRank(checkpoint.Classification) < classificationRank(assessment.Checkpoints[index-1].Classification) {
			return fmt.Errorf("checkpoint %q improves classification", checkpoint.Stage)
		}
	}
	if assessment.Checkpoints[len(assessment.Checkpoints)-1].Classification != assessment.Classification {
		return fmt.Errorf("final checkpoint classification disagrees with assessment")
	}
	if !validSHA256(assessment.Hash) {
		return fmt.Errorf("assessment hash is not SHA-256")
	}
	expected, err := assessmentHash(assessment)
	if err != nil {
		return err
	}
	if assessment.Hash != expected {
		return fmt.Errorf("assessment hash mismatch")
	}
	return nil
}

func normalizeAssessment(assessment *Assessment) {
	for index := range assessment.Requirements {
		assessment.Requirements[index].ID = strings.TrimSpace(assessment.Requirements[index].ID)
		assessment.Requirements[index].Description = strings.TrimSpace(assessment.Requirements[index].Description)
		assessment.Requirements[index].EvidenceIDs = normalizedStrings(assessment.Requirements[index].EvidenceIDs)
	}
	slices.SortStableFunc(assessment.Requirements, func(left, right Requirement) int {
		if order := strings.Compare(string(left.Kind), string(right.Kind)); order != 0 {
			return order
		}
		return strings.Compare(left.ID, right.ID)
	})
	assessment.Requirements = compactRequirements(assessment.Requirements)
	for index := range assessment.Evidence {
		assessment.Evidence[index].ID = strings.TrimSpace(assessment.Evidence[index].ID)
		assessment.Evidence[index].Kind = strings.TrimSpace(assessment.Evidence[index].Kind)
		assessment.Evidence[index].Source = strings.TrimSpace(assessment.Evidence[index].Source)
		assessment.Evidence[index].Digest = strings.ToLower(strings.TrimSpace(assessment.Evidence[index].Digest))
		assessment.Evidence[index].Stage = strings.TrimSpace(assessment.Evidence[index].Stage)
		assessment.Evidence[index].Description = strings.TrimSpace(assessment.Evidence[index].Description)
	}
	slices.SortStableFunc(assessment.Evidence, func(left, right Evidence) int {
		return strings.Compare(left.ID, right.ID)
	})
	assessment.Evidence = compactEvidence(assessment.Evidence)
	for index := range assessment.Gaps {
		assessment.Gaps[index].Code = strings.TrimSpace(assessment.Gaps[index].Code)
		assessment.Gaps[index].ID = strings.TrimSpace(assessment.Gaps[index].ID)
		assessment.Gaps[index].Stage = strings.TrimSpace(assessment.Gaps[index].Stage)
		assessment.Gaps[index].Reason = strings.TrimSpace(assessment.Gaps[index].Reason)
		assessment.Gaps[index].Action = strings.TrimSpace(assessment.Gaps[index].Action)
	}
	slices.SortStableFunc(assessment.Gaps, func(left, right Gap) int {
		if order := strings.Compare(left.Code, right.Code); order != 0 {
			return order
		}
		if order := strings.Compare(string(left.Kind), string(right.Kind)); order != 0 {
			return order
		}
		return strings.Compare(left.ID, right.ID)
	})
	assessment.Gaps = compactGaps(assessment.Gaps)
	for index := range assessment.Risks {
		assessment.Risks[index].Code = strings.TrimSpace(assessment.Risks[index].Code)
		assessment.Risks[index].Stage = strings.TrimSpace(assessment.Risks[index].Stage)
		assessment.Risks[index].Summary = strings.TrimSpace(assessment.Risks[index].Summary)
		assessment.Risks[index].Mitigation = strings.TrimSpace(assessment.Risks[index].Mitigation)
	}
	slices.SortStableFunc(assessment.Risks, func(left, right Risk) int {
		if order := strings.Compare(left.Code, right.Code); order != 0 {
			return order
		}
		return strings.Compare(left.Stage, right.Stage)
	})
	assessment.Risks = compactRisks(assessment.Risks)
}

func deriveClassification(assessment Assessment) (Classification, error) {
	if len(assessment.Gaps) != 0 {
		return ClassificationUnsupported, nil
	}
	evidenceByID := make(map[string]Evidence, len(assessment.Evidence))
	for _, evidence := range assessment.Evidence {
		evidenceByID[evidence.ID] = evidence
	}
	classification := ClassificationSupported
	for _, requirement := range assessment.Requirements {
		if len(requirement.EvidenceIDs) == 0 {
			return ClassificationUnsupported, nil
		}
		for _, evidenceID := range requirement.EvidenceIDs {
			evidence, ok := evidenceByID[evidenceID]
			if !ok {
				return ClassificationUnsupported, nil
			}
			switch evidence.Status {
			case EvidenceFailed, EvidenceMissing:
				return ClassificationUnsupported, nil
			case EvidenceInferred:
				classification = ClassificationExperimental
			case EvidenceVerified:
				if strings.TrimSpace(evidence.Source) == "" || !validSHA256(evidence.Digest) {
					return "", fmt.Errorf("verified evidence %q requires reproducible source and digest", evidence.ID)
				}
			default:
				return "", fmt.Errorf("unsupported evidence status %q", evidence.Status)
			}
		}
	}
	return classification, nil
}

func checkpointForAssessment(stage string, assessment Assessment) Checkpoint {
	return Checkpoint{
		Stage:          stage,
		Classification: assessment.Classification,
		EvidenceIDs:    evidenceIDs(assessment.Evidence),
		GapCodes:       gapCodes(assessment.Gaps),
		RiskCodes:      riskCodes(assessment.Risks),
	}
}

func checkpointForInput(stage string, classification Classification, input CheckpointInput) Checkpoint {
	return Checkpoint{
		Stage:          stage,
		Classification: classification,
		EvidenceIDs:    evidenceIDs(input.Evidence),
		GapCodes:       gapCodes(input.Gaps),
		RiskCodes:      riskCodes(input.Risks),
	}
}

func sealAssessment(assessment *Assessment) error {
	assessment.Hash = ""
	hash, err := assessmentHash(*assessment)
	if err != nil {
		return err
	}
	assessment.Hash = hash
	return Validate(*assessment)
}

func assessmentHash(assessment Assessment) (string, error) {
	assessment.Hash = ""
	data, err := json.Marshal(assessment)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func cloneAssessment(assessment Assessment) Assessment {
	assessment.Requirements = append([]Requirement(nil), assessment.Requirements...)
	for index := range assessment.Requirements {
		assessment.Requirements[index].EvidenceIDs = append([]string(nil), assessment.Requirements[index].EvidenceIDs...)
	}
	assessment.Evidence = append([]Evidence(nil), assessment.Evidence...)
	assessment.Gaps = append([]Gap(nil), assessment.Gaps...)
	assessment.Risks = append([]Risk(nil), assessment.Risks...)
	assessment.Checkpoints = append([]Checkpoint(nil), assessment.Checkpoints...)
	for index := range assessment.Checkpoints {
		assessment.Checkpoints[index].EvidenceIDs = append([]string(nil), assessment.Checkpoints[index].EvidenceIDs...)
		assessment.Checkpoints[index].GapCodes = append([]string(nil), assessment.Checkpoints[index].GapCodes...)
		assessment.Checkpoints[index].RiskCodes = append([]string(nil), assessment.Checkpoints[index].RiskCodes...)
	}
	return assessment
}

func worseClassification(left, right Classification) Classification {
	if classificationRank(left) >= classificationRank(right) {
		return left
	}
	return right
}

func classificationRank(value Classification) int {
	switch value {
	case ClassificationSupported:
		return 0
	case ClassificationExperimental:
		return 1
	case ClassificationUnsupported:
		return 2
	default:
		return 3
	}
}

func validClassification(value Classification) bool {
	return value == ClassificationSupported || value == ClassificationExperimental || value == ClassificationUnsupported
}

func validRequirementKind(value RequirementKind) bool {
	switch value {
	case RequirementDomain, RequirementArchitecture, RequirementComponent, RequirementModel, RequirementPhysical, RequirementVerification:
		return true
	default:
		return false
	}
}

func validEvidenceStatus(value EvidenceStatus) bool {
	return value == EvidenceVerified || value == EvidenceInferred || value == EvidenceMissing || value == EvidenceFailed
}

func validSHA256(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func normalizedStrings(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			result = append(result, value)
		}
	}
	slices.Sort(result)
	return slices.Compact(result)
}

func compactRequirements(values []Requirement) []Requirement {
	if len(values) < 2 {
		return values
	}
	result := values[:0]
	for _, value := range values {
		if len(result) != 0 && result[len(result)-1].Kind == value.Kind && result[len(result)-1].ID == value.ID {
			result[len(result)-1].EvidenceIDs = normalizedStrings(append(result[len(result)-1].EvidenceIDs, value.EvidenceIDs...))
			if result[len(result)-1].Description == "" {
				result[len(result)-1].Description = value.Description
			}
			continue
		}
		result = append(result, value)
	}
	return result
}

func compactEvidence(values []Evidence) []Evidence {
	if len(values) < 2 {
		return values
	}
	result := values[:0]
	for _, value := range values {
		if len(result) != 0 && result[len(result)-1].ID == value.ID {
			if result[len(result)-1] == value {
				continue
			}
		}
		result = append(result, value)
	}
	return result
}

func compactGaps(values []Gap) []Gap {
	if len(values) < 2 {
		return values
	}
	result := values[:0]
	for _, value := range values {
		if len(result) != 0 && result[len(result)-1] == value {
			continue
		}
		result = append(result, value)
	}
	return result
}

func compactRisks(values []Risk) []Risk {
	if len(values) < 2 {
		return values
	}
	result := values[:0]
	for _, value := range values {
		if len(result) != 0 && result[len(result)-1] == value {
			continue
		}
		result = append(result, value)
	}
	return result
}

func evidenceIDs(values []Evidence) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, value.ID)
	}
	return normalizedStrings(result)
}

func gapCodes(values []Gap) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, value.Code)
	}
	return normalizedStrings(result)
}

func riskCodes(values []Risk) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, value.Code)
	}
	return normalizedStrings(result)
}
