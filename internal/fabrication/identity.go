package fabrication

import (
	"slices"
	"strings"

	"kicadai/internal/reports"
)

type IdentityStatus string

const (
	IdentityPass    IdentityStatus = "pass"
	IdentityWarning IdentityStatus = "warning"
	IdentityMissing IdentityStatus = "missing"
	// Conflict and skipped statuses are populated by BOM/CPL/profile phases.
	IdentityConflict IdentityStatus = "conflict"
	IdentitySkipped  IdentityStatus = "skipped"
	IdentityFail     IdentityStatus = "fail"
)

type IdentitySource string

const (
	IdentitySourceSchematicProperty IdentitySource = "schematic_property"
	IdentitySourceCatalogSelection  IdentitySource = "catalog_selection"
	IdentitySourcePCBFootprint      IdentitySource = "pcb_footprint"
	IdentitySourceInferred          IdentitySource = "inferred"
	IdentitySourceMissing           IdentitySource = "missing"
)

type ComponentIdentity struct {
	Reference         string          `json:"reference"`
	ComponentID       string          `json:"component_id,omitempty"`
	Value             string          `json:"value,omitempty"`
	SymbolID          string          `json:"symbol_id,omitempty"`
	FootprintID       string          `json:"footprint_id,omitempty"`
	Manufacturer      string          `json:"manufacturer,omitempty"`
	MPN               string          `json:"mpn,omitempty"`
	Package           string          `json:"package,omitempty"`
	ComponentClass    string          `json:"component_class,omitempty"`
	Lifecycle         string          `json:"lifecycle,omitempty"`
	Confidence        string          `json:"confidence,omitempty"`
	Source            IdentitySource  `json:"source"`
	Status            IdentityStatus  `json:"status"`
	ExactPartRequired bool            `json:"exact_part_required,omitempty"`
	ExactPartPresent  bool            `json:"exact_part_present,omitempty"`
	Issues            []reports.Issue `json:"issues,omitempty"`
}

func NormalizeComponentIdentity(identity ComponentIdentity) ComponentIdentity {
	identity.Reference = strings.TrimSpace(identity.Reference)
	identity.ComponentID = strings.TrimSpace(identity.ComponentID)
	identity.Value = strings.TrimSpace(identity.Value)
	identity.SymbolID = strings.TrimSpace(identity.SymbolID)
	identity.FootprintID = strings.TrimSpace(identity.FootprintID)
	identity.Manufacturer = strings.TrimSpace(identity.Manufacturer)
	identity.MPN = strings.TrimSpace(identity.MPN)
	identity.Package = strings.TrimSpace(identity.Package)
	identity.ComponentClass = strings.ToLower(strings.TrimSpace(identity.ComponentClass))
	identity.Lifecycle = strings.TrimSpace(identity.Lifecycle)
	identity.Confidence = strings.TrimSpace(identity.Confidence)
	if identity.Source == "" {
		if !identity.HasEvidence() {
			identity.Source = IdentitySourceMissing
		} else {
			identity.Source = IdentitySourceSchematicProperty
		}
	}
	identity.ExactPartPresent = (identity.Manufacturer != "" && identity.MPN != "") || identity.ComponentID != ""
	identity.Issues = slices.Clone(identity.Issues)
	if identity.Status == "" {
		identity.Status = identityStatusFor(identity)
	}
	return identity
}

func (identity ComponentIdentity) HasEvidence() bool {
	return identity.Manufacturer != "" ||
		identity.MPN != "" ||
		identity.ComponentID != "" ||
		identity.Value != "" ||
		identity.SymbolID != "" ||
		identity.FootprintID != "" ||
		identity.Package != ""
}

func identityStatusFor(identity ComponentIdentity) IdentityStatus {
	if identity.Source == IdentitySourceMissing {
		return IdentityMissing
	}
	if reports.HasBlockingIssue(identity.Issues) {
		return IdentityFail
	}
	if identity.ExactPartRequired && !identity.ExactPartPresent {
		return IdentityMissing
	}
	if len(identity.Issues) > 0 {
		return IdentityWarning
	}
	if !identity.ExactPartPresent {
		return IdentityWarning
	}
	return IdentityPass
}

func MissingIdentityIssue(path string, ref string, message string) reports.Issue {
	if strings.TrimSpace(message) == "" {
		message = "component identity evidence is missing"
	}
	return reports.Issue{
		Code:       reports.CodeValidationFailed,
		Severity:   reports.SeverityError,
		Path:       path,
		Message:    message,
		Refs:       []string{ref},
		Suggestion: "add component identity properties or select a catalog-backed part before fabrication release",
	}
}

func IdentityIssueCounts(issues []reports.Issue) (total int, blocking int) {
	total = len(issues)
	for _, issue := range issues {
		if issue.Blocking() {
			blocking++
		}
	}
	return total, blocking
}
