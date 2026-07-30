// Package componentonboarding turns untrusted manufacturer-document
// extractions into quarantined, reproducible component-catalog candidates.
//
// The package deliberately separates extraction from trust. An AI or another
// parser may propose claims and catalog records, but only deterministic
// validation and an explicitly approved promotion may make those records
// selectable.
package componentonboarding

import (
	"context"

	"kicadai/internal/components"
	"kicadai/internal/modelprovenance"
)

const (
	RequestSchema    = "kicadai.component-onboarding-request.v1"
	ExtractionSchema = "kicadai.component-onboarding-extraction.v1"
	CandidateSchema  = "kicadai.component-onboarding-candidate.v1"
	PromotionSchema  = "kicadai.component-onboarding-promotion.v1"
	OverlaySchema    = "kicadai.component-catalog-overlay.v1"
	PolicyVersion    = "evidence-backed-component-onboarding-v1"

	MaxDocuments        = 128
	MaxDocumentBytes    = 16 << 20
	MaxDocumentSetBytes = 32 << 20
	MaxClaims           = 8192
	MaxCandidates       = 256
)

type Status string

const (
	StatusQuarantined Status = "quarantined"
	StatusSupported   Status = "supported"
)

type DocumentKind string

const (
	DocumentDatasheet DocumentKind = "manufacturer_datasheet"
	DocumentModel     DocumentKind = "manufacturer_model"
	DocumentLibrary   DocumentKind = "kicad_library"
	DocumentPackage   DocumentKind = "manufacturer_package_drawing"
)

type ModelKind string

const (
	ModelManufacturer ModelKind = "manufacturer_model"
	ModelBounded      ModelKind = "bounded_analytic_substitute"
)

type BehavioralRequirement struct {
	Schema              string                            `json:"schema"`
	ID                  string                            `json:"id"`
	Family              string                            `json:"family"`
	RequiredFunctions   []string                          `json:"required_functions"`
	RequiredRatings     []components.RequiredRating       `json:"required_ratings"`
	RequiredTemperature components.TemperatureRequirement `json:"required_temperature"`
	RequiredAnalyses    []string                          `json:"required_analyses"`
	AllowedPackages     []string                          `json:"allowed_packages,omitempty"`
	MinimumDerating     float64                           `json:"minimum_derating_ratio"`
}

type DocumentInput struct {
	ID             string       `json:"id"`
	Kind           DocumentKind `json:"kind"`
	Publisher      string       `json:"publisher"`
	Locator        string       `json:"locator"`
	Revision       string       `json:"revision"`
	License        string       `json:"license,omitempty"`
	ExpectedSHA256 string       `json:"expected_sha256"`
	Content        []byte       `json:"-"`
}

type DocumentRecord struct {
	ID        string       `json:"id"`
	Kind      DocumentKind `json:"kind"`
	Publisher string       `json:"publisher"`
	Locator   string       `json:"locator"`
	Revision  string       `json:"revision"`
	License   string       `json:"license,omitempty"`
	SHA256    string       `json:"sha256"`
	Bytes     int          `json:"bytes"`
}

// Claim is untrusted extractor output. Excerpt must occur verbatim in the
// referenced immutable document; Field and Value are checked against the
// proposed catalog record by deterministic validators.
type Claim struct {
	ID         string `json:"id"`
	DocumentID string `json:"document_id"`
	Subject    string `json:"subject"`
	Field      string `json:"field"`
	Relation   string `json:"relation,omitempty"`
	Value      string `json:"value"`
	Unit       string `json:"unit,omitempty"`
	Excerpt    string `json:"excerpt"`
	Location   string `json:"location"`
}

type EvidenceBinding struct {
	Path     string   `json:"path"`
	ClaimIDs []string `json:"claim_ids"`
}

type ModelProposal struct {
	Kind               ModelKind              `json:"kind"`
	ModelID            string                 `json:"model_id"`
	Provenance         modelprovenance.Record `json:"provenance"`
	ClaimIDs           []string               `json:"claim_ids"`
	BoundedAssumptions []string               `json:"bounded_assumptions,omitempty"`
}

type ComponentProposal struct {
	Record   components.ComponentRecord `json:"record"`
	Evidence []EvidenceBinding          `json:"evidence"`
	Model    ModelProposal              `json:"model"`
}

type Extraction struct {
	Schema     string              `json:"schema"`
	Claims     []Claim             `json:"claims"`
	Candidates []ComponentProposal `json:"candidates"`
}

// Extractor is intentionally untrusted. Implementations may use an AI,
// OCR/PDF tooling, a vendor feed, or a deterministic parser.
type Extractor interface {
	Extract(context.Context, BehavioralRequirement, []DocumentInput) (Extraction, error)
}

type CandidateScore struct {
	ComponentID      string  `json:"component_id"`
	VariantID        string  `json:"variant_id"`
	MinimumMargin    float64 `json:"minimum_margin"`
	EvidenceCoverage int     `json:"evidence_coverage"`
	Rank             int     `json:"rank"`
}

type Candidate struct {
	Schema        string                `json:"schema"`
	PolicyVersion string                `json:"policy_version"`
	Status        Status                `json:"status"`
	Requirement   BehavioralRequirement `json:"requirement"`
	Documents     []DocumentRecord      `json:"documents"`
	Claims        []Claim               `json:"claims"`
	Proposals     []ComponentProposal   `json:"proposals"`
	Ranking       []CandidateScore      `json:"ranking"`
	SelectedID    string                `json:"selected_id"`
	Hash          string                `json:"hash"`
}

type GateEvidence struct {
	Gate           string `json:"gate"`
	Run            int    `json:"run"`
	Passed         bool   `json:"passed"`
	EvidencePath   string `json:"evidence_path"`
	EvidenceSHA256 string `json:"evidence_sha256"`
}

type Approval struct {
	CandidateHash string `json:"candidate_hash"`
	Decision      string `json:"decision"`
	Reviewer      string `json:"reviewer"`
	ReviewRef     string `json:"review_ref"`
	ReviewSHA256  string `json:"review_sha256"`
}

type Promotion struct {
	Schema        string         `json:"schema"`
	PolicyVersion string         `json:"policy_version"`
	Candidate     Candidate      `json:"candidate"`
	Gates         []GateEvidence `json:"gates"`
	Approval      Approval       `json:"approval"`
	Hash          string         `json:"hash"`
}

type SupportedOverlay struct {
	Schema          string                       `json:"schema"`
	PolicyVersion   string                       `json:"policy_version"`
	Status          Status                       `json:"status"`
	RequirementHash string                       `json:"requirement_hash"`
	CandidateHash   string                       `json:"candidate_hash"`
	PromotionHash   string                       `json:"promotion_hash"`
	Records         []components.ComponentRecord `json:"records"`
	Models          modelprovenance.Registry     `json:"models"`
	Hash            string                       `json:"hash"`
}

// EvaluationEnvironment is an in-memory promotion harness. It is not
// serializable as a supported overlay and cannot be loaded through the CLI
// component-overlay path.
type EvaluationEnvironment struct {
	Status        Status
	CandidateHash string
	Catalog       *components.Catalog
	Models        modelprovenance.Registry
}

var RequiredPromotionGates = []string{
	"connectivity",
	"drc_strict",
	"erc",
	"route_completion",
	"round_trip",
	"simulation",
	"writer_correctness",
}
