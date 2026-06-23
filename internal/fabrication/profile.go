package fabrication

import (
	"fmt"
	"slices"
	"strings"

	"kicadai/internal/reports"
)

const GenericAssemblyProfileID = "generic_assembly"

type ExactMPNPolicy string

const (
	ExactMPNRequireAssemblyCritical ExactMPNPolicy = "require_assembly_critical"
	ExactMPNRequireAll              ExactMPNPolicy = "require_all"
	ExactMPNWarnGenericPassives     ExactMPNPolicy = "warn_generic_passives"
)

var builtinManufacturerProfiles = []ManufacturerProfile{
	{
		ID:                   GenericAssemblyProfileID,
		DisplayName:          "Generic Assembly",
		RequiredBOMColumns:   []string{"References", "Quantity", "Value", "FootprintID", "Manufacturer", "MPN"},
		RequiredCPLColumns:   []string{"Reference", "Footprint", "X(mm)", "Y(mm)", "NormalizedSide", "NormalizedRotation"},
		AcceptedSides:        []string{cplSideTop, cplSideBottom},
		ExactMPNPolicy:       ExactMPNRequireAssemblyCritical,
		AllowGenericPassives: true,
		RotationConvention:   "KiCad footprint rotation normalized to degrees in [0, 360); manufacturer-specific zero-degree conventions are not guaranteed.",
	},
}

type ManufacturerProfile struct {
	ID                   string         `json:"id"`
	DisplayName          string         `json:"display_name"`
	RequiredBOMColumns   []string       `json:"required_bom_columns,omitempty"`
	RequiredCPLColumns   []string       `json:"required_cpl_columns,omitempty"`
	AcceptedSides        []string       `json:"accepted_sides,omitempty"`
	ExactMPNPolicy       ExactMPNPolicy `json:"exact_mpn_policy"`
	AllowGenericPassives bool           `json:"allow_generic_passives"`
	RotationConvention   string         `json:"rotation_convention,omitempty"`
}

func BuiltinManufacturerProfiles() []ManufacturerProfile {
	profiles := make([]ManufacturerProfile, len(builtinManufacturerProfiles))
	for index, profile := range builtinManufacturerProfiles {
		profiles[index] = cloneManufacturerProfile(profile)
	}
	return profiles
}

func LookupManufacturerProfile(id string) (ManufacturerProfile, bool) {
	id = strings.TrimSpace(id)
	for _, profile := range builtinManufacturerProfiles {
		if strings.EqualFold(profile.ID, id) {
			return cloneManufacturerProfile(profile), true
		}
	}
	return ManufacturerProfile{}, false
}

func cloneManufacturerProfile(profile ManufacturerProfile) ManufacturerProfile {
	profile.RequiredBOMColumns = slices.Clone(profile.RequiredBOMColumns)
	profile.RequiredCPLColumns = slices.Clone(profile.RequiredCPLColumns)
	profile.AcceptedSides = slices.Clone(profile.AcceptedSides)
	return profile
}

func ValidateManufacturerProfile(profile ManufacturerProfile, data ReportData) []reports.Issue {
	if issue := validateManufacturerProfileConfig(profile); issue != nil {
		return []reports.Issue{*issue}
	}
	var issues []reports.Issue
	acceptedSides := map[string]struct{}{}
	for _, side := range profile.AcceptedSides {
		acceptedSides[strings.ToLower(strings.TrimSpace(side))] = struct{}{}
	}
	for _, row := range data.BOM {
		if severity, required := exactMPNRequirement(profile, row); required && (strings.TrimSpace(row.Manufacturer) == "" || strings.TrimSpace(row.MPN) == "") {
			issues = append(issues, reports.Issue{
				Code:       reports.CodeValidationFailed,
				Severity:   severity,
				Path:       "profile." + profile.ID + ".bom." + firstReference(row.References),
				Message:    fmt.Sprintf("%s requires exact manufacturer and MPN for profile %s", firstReference(row.References), profile.ID),
				Refs:       slices.Clone(row.References),
				Suggestion: "select a catalog-backed component or add Manufacturer and MPN fields",
			})
		}
	}
	for _, row := range data.CPL {
		side := strings.ToLower(strings.TrimSpace(firstNonEmpty(row.NormalizedSide, row.Layer)))
		if side == "" || side == cplSideUnknown {
			issues = append(issues, profileCPLSideIssue(profile, row, "has no accepted assembly side"))
			continue
		}
		if _, ok := acceptedSides[side]; !ok {
			issues = append(issues, profileCPLSideIssue(profile, row, fmt.Sprintf("uses unsupported assembly side %q", side)))
		}
	}
	return issues
}

func validateManufacturerProfileConfig(profile ManufacturerProfile) *reports.Issue {
	id := strings.TrimSpace(profile.ID)
	if id == "" {
		return &reports.Issue{
			Code:       reports.CodeInvalidArgument,
			Severity:   reports.SeverityError,
			Path:       "manufacturer_profile.id",
			Message:    "manufacturer profile ID is required",
			Suggestion: "use a built-in profile such as generic_assembly or provide a complete custom profile",
		}
	}
	if strings.TrimSpace(profile.DisplayName) == "" {
		return &reports.Issue{
			Code:       reports.CodeInvalidArgument,
			Severity:   reports.SeverityError,
			Path:       "manufacturer_profile.display_name",
			Message:    fmt.Sprintf("manufacturer profile %q display name is required", id),
			Suggestion: "use a built-in profile such as generic_assembly or provide a display name",
		}
	}
	if len(profile.AcceptedSides) == 0 {
		return &reports.Issue{
			Code:       reports.CodeInvalidArgument,
			Severity:   reports.SeverityError,
			Path:       "manufacturer_profile.accepted_sides",
			Message:    fmt.Sprintf("manufacturer profile %q accepted sides are required", id),
			Suggestion: "include at least top and/or bottom side names",
		}
	}
	return nil
}

func unknownManufacturerProfileIssue(id string) reports.Issue {
	id = strings.TrimSpace(id)
	if id == "" {
		id = "<empty>"
	}
	return reports.Issue{
		Code:       reports.CodeInvalidArgument,
		Severity:   reports.SeverityError,
		Path:       "manufacturer_profile",
		Message:    fmt.Sprintf("unknown manufacturer profile %q", id),
		Suggestion: "use a built-in profile such as generic_assembly",
	}
}

func exactMPNRequirement(profile ManufacturerProfile, row BOMRow) (reports.Severity, bool) {
	switch profile.ExactMPNPolicy {
	case ExactMPNRequireAll:
		return reports.SeverityError, true
	case ExactMPNWarnGenericPassives:
		if genericPassive(row) {
			return reports.SeverityWarning, true
		}
		return reports.SeverityError, true
	case ExactMPNRequireAssemblyCritical, "":
		if profile.AllowGenericPassives && genericPassive(row) {
			return "", false
		}
		return reports.SeverityError, true
	default:
		return reports.SeverityError, true
	}
}

func genericPassive(row BOMRow) bool {
	class := strings.ToLower(strings.TrimSpace(row.ComponentClass))
	if class == "passive" || class == "resistor" || class == "capacitor" || class == "inductor" {
		return true
	}
	for _, ref := range row.References {
		prefix := referencePrefix(ref)
		if prefix == "R" || prefix == "C" || prefix == "L" {
			return true
		}
	}
	return false
}

func referencePrefix(ref string) string {
	ref = strings.ToUpper(strings.TrimSpace(ref))
	for index, char := range ref {
		if char >= '0' && char <= '9' {
			return ref[:index]
		}
	}
	return ref
}

func firstReference(references []string) string {
	for _, ref := range references {
		if strings.TrimSpace(ref) != "" {
			return strings.TrimSpace(ref)
		}
	}
	return "component"
}

func profileCPLSideIssue(profile ManufacturerProfile, row CPLRow, message string) reports.Issue {
	return reports.Issue{
		Code:       reports.CodeValidationFailed,
		Severity:   reports.SeverityError,
		Path:       "profile." + profile.ID + ".cpl." + row.Reference + ".side",
		Message:    fmt.Sprintf("%s %s for profile %s", row.Reference, message, profile.ID),
		Refs:       []string{row.Reference},
		Suggestion: "use an accepted assembly side for the selected manufacturer profile",
	}
}
