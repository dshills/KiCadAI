package schematicrules

import (
	"cmp"
	"slices"

	"kicadai/internal/reports"
)

// Status summarizes whether schematic electrical rules found blocking issues.
type Status string

const (
	StatusNotApplicable Status = "not_applicable"
	StatusClean         Status = "clean"
	StatusWarning       Status = "warning"
	StatusBlocked       Status = "blocked"
	StatusUnknown       Status = "unknown"
)

// Acceptance describes how strictly schematic electrical findings should block.
type Acceptance string

const (
	AcceptanceDraft                Acceptance = "draft"
	AcceptanceStructural           Acceptance = "structural"
	AcceptanceConnectivity         Acceptance = "connectivity"
	AcceptanceERCDRC               Acceptance = "erc-drc"
	AcceptanceFabricationCandidate Acceptance = "fabrication-candidate"
)

// Scope records whether a schematic came from KiCadAI generation or import.
type Scope string

const (
	ScopeGenerated Scope = "generated"
	ScopeImported  Scope = "imported"
)

// Category groups schematic electrical findings for repair and reporting.
type Category string

const (
	CategoryReference  Category = "reference"
	CategoryPin        Category = "pin"
	CategoryNet        Category = "net"
	CategoryPower      Category = "power"
	CategoryDecoupling Category = "decoupling"
	CategoryValue      Category = "value"
	CategoryRating     Category = "rating"
	CategoryLibrary    Category = "library"
	CategoryHierarchy  Category = "hierarchy"
)

// RuleID is a stable machine-readable schematic electrical rule identifier.
type RuleID string

const (
	RuleReferenceDuplicate          RuleID = "SCH_REF_DUPLICATE"
	RuleReferenceEmpty              RuleID = "SCH_REF_EMPTY"
	RuleReferencePowerCollision     RuleID = "SCH_REF_POWER_COLLISION"
	RulePinRequiredOpen             RuleID = "SCH_PIN_REQUIRED_OPEN"
	RulePinOptionalOpen             RuleID = "SCH_PIN_OPTIONAL_OPEN"
	RulePinNoConnectMissing         RuleID = "SCH_PIN_NC_MISSING"
	RulePinNoConnectOnRequired      RuleID = "SCH_PIN_NC_ON_REQUIRED"
	RulePinMetadataMissing          RuleID = "SCH_PIN_METADATA_MISSING"
	RuleLabelFloating               RuleID = "SCH_LABEL_FLOATING"
	RuleLabelEmpty                  RuleID = "SCH_LABEL_EMPTY"
	RuleLabelConflict               RuleID = "SCH_LABEL_CONFLICT"
	RuleLabelNormalizationCollision RuleID = "SCH_LABEL_NORMALIZATION_COLLISION"
	RulePowerSourceMissing          RuleID = "SCH_POWER_SOURCE_MISSING"
	RulePowerSinkMissing            RuleID = "SCH_POWER_SINK_MISSING"
	RulePowerExternalUndeclared     RuleID = "SCH_POWER_EXTERNAL_UNDECLARED"
	RulePowerFlagWithoutRail        RuleID = "SCH_POWER_FLAG_WITHOUT_RAIL"
	RulePowerMetadataMissing        RuleID = "SCH_POWER_METADATA_MISSING"
	RuleDecouplingMissing           RuleID = "SCH_DECOUPLING_MISSING"
	RuleDecouplingValueMismatch     RuleID = "SCH_DECOUPLING_VALUE_MISMATCH"
	RuleDecouplingRailMismatch      RuleID = "SCH_DECOUPLING_RAIL_MISMATCH"
	RuleDecouplingEvidenceDeferred  RuleID = "SCH_DECOUPLING_EVIDENCE_DEFERRED"
	RuleValueMissing                RuleID = "SCH_VALUE_MISSING"
	RuleValueParseFailed            RuleID = "SCH_VALUE_PARSE_FAILED"
	RuleValueOutOfPolicy            RuleID = "SCH_VALUE_OUT_OF_POLICY"
	RuleRatingInsufficient          RuleID = "SCH_RATING_INSUFFICIENT"
	RuleRatingEvidenceMissing       RuleID = "SCH_RATING_EVIDENCE_MISSING"
)

// Options configures schematic electrical rule evaluation.
type Options struct {
	Acceptance            Acceptance `json:"acceptance,omitempty"`
	Scope                 Scope      `json:"scope,omitempty"`
	RequireConfidence     bool       `json:"require_confidence,omitempty"`
	AcceptedExternalRails []string   `json:"accepted_external_rails,omitempty"`
}

// Finding is one schematic electrical rule result.
type Finding struct {
	RuleID    RuleID           `json:"rule_id"`
	Severity  reports.Severity `json:"severity"`
	Category  Category         `json:"category"`
	Path      string           `json:"path,omitempty"`
	Reference string           `json:"reference,omitempty"`
	Pin       string           `json:"pin,omitempty"`
	Net       string           `json:"net,omitempty"`
	Message   string           `json:"message"`
	Repair    string           `json:"repair,omitempty"`
}

// Report summarizes schematic electrical rule evidence.
type Report struct {
	Status                        Status    `json:"status"`
	CheckedSymbols                int       `json:"checked_symbols"`
	CheckedNets                   int       `json:"checked_nets"`
	CheckedPowerRails             int       `json:"checked_power_rails"`
	CheckedRequiredPins           int       `json:"checked_required_pins"`
	CheckedDecouplingRequirements int       `json:"checked_decoupling_requirements"`
	FindingCount                  int       `json:"finding_count"`
	Findings                      []Finding `json:"findings,omitempty"`
}

// Normalize sorts findings and computes count/status fields.
func (report *Report) Normalize() {
	if report == nil {
		return
	}
	SortFindings(report.Findings)
	report.FindingCount = len(report.Findings)
	if report.Status == StatusNotApplicable {
		return
	}
	report.Status = StatusForFindings(report.Findings)
}

// NewReport builds a normalized report from counts and findings.
func NewReport(report Report) Report {
	report.Findings = cloneFindings(report.Findings)
	report.Normalize()
	return report
}

// StatusForFindings returns the aggregate report status for findings.
func StatusForFindings(findings []Finding) Status {
	if len(findings) == 0 {
		return StatusClean
	}
	status := StatusClean
	for _, finding := range findings {
		switch finding.Severity {
		case reports.SeverityBlocked, reports.SeverityError:
			return StatusBlocked
		case reports.SeverityInfo:
			continue
		case reports.SeverityWarning:
			status = StatusWarning
		default:
			return StatusUnknown
		}
	}
	return status
}

// SortFindings orders findings deterministically for JSON and tests.
func SortFindings(findings []Finding) {
	slices.SortStableFunc(findings, func(a, b Finding) int {
		return cmp.Or(
			cmp.Compare(severityRank(a.Severity), severityRank(b.Severity)),
			cmp.Compare(a.Category, b.Category),
			cmp.Compare(a.RuleID, b.RuleID),
			cmp.Compare(a.Reference, b.Reference),
			cmp.Compare(a.Net, b.Net),
			cmp.Compare(a.Pin, b.Pin),
			cmp.Compare(a.Path, b.Path),
			cmp.Compare(a.Message, b.Message),
		)
	})
}

func cloneFindings(findings []Finding) []Finding {
	if findings == nil {
		return nil
	}
	clone := make([]Finding, len(findings))
	copy(clone, findings)
	return clone
}

func severityRank(severity reports.Severity) int {
	switch severity {
	case reports.SeverityBlocked:
		return 0
	case reports.SeverityError:
		return 1
	case reports.SeverityWarning:
		return 2
	case reports.SeverityInfo:
		return 3
	default:
		return 4
	}
}
