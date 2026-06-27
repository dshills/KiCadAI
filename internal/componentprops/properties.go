package componentprops

import (
	"fmt"
	"strings"

	"kicadai/internal/kicadfiles"
	"kicadai/internal/kicadfiles/schematic"
	"kicadai/internal/reports"
)

const (
	PropertyComponentID         = "KiCadAI Component ID"
	PropertyVariantID           = "KiCadAI Variant ID"
	PropertyComponentRole       = "KiCadAI Component Role"
	PropertyBlockID             = "KiCadAI Block ID"
	PropertyManufacturer        = "Manufacturer"
	PropertyMPN                 = "MPN"
	PropertyComponentClass      = "Component Class"
	PropertyComponentConfidence = "Component Confidence"
	PropertyComponentSource     = "Component Source"
	PropertyLifecycleStatus     = "Lifecycle Status"
	PropertyAvailabilityStatus  = "Availability Status"
	PropertyPinmapID            = "Pinmap ID"
)

const (
	SourceCatalog         = "catalog"
	SourceCatalogSnapshot = "catalog+source_snapshot"
	SourceGeneric         = "generic"
	SourcePolicyAllowed   = "policy_allowed"
)

var orderedPropertyNames = []string{
	PropertyComponentID,
	PropertyVariantID,
	PropertyComponentRole,
	PropertyBlockID,
	PropertyManufacturer,
	PropertyMPN,
	PropertyComponentClass,
	PropertyComponentConfidence,
	PropertyComponentSource,
	PropertyLifecycleStatus,
	PropertyAvailabilityStatus,
	PropertyPinmapID,
}

var ownedPropertyNames = func() map[string]struct{} {
	names := make(map[string]struct{}, len(orderedPropertyNames))
	for _, name := range orderedPropertyNames {
		names[normalizeName(name)] = struct{}{}
	}
	return names
}()

var canonicalPropertyNames = func() map[string]string {
	names := make(map[string]string, len(orderedPropertyNames))
	for _, name := range orderedPropertyNames {
		names[normalizeName(name)] = name
	}
	return names
}()

// Evidence contains selected component metadata that can be written into
// schematic symbol properties.
type Evidence struct {
	ComponentID         string
	VariantID           string
	ComponentRole       string
	BlockID             string
	Manufacturer        string
	MPN                 string
	ComponentClass      string
	ComponentConfidence string
	ComponentSource     string
	LifecycleStatus     string
	AvailabilityStatus  string
	PinmapID            string
}

// MergePolicy controls how existing identity-property conflicts are handled.
type MergePolicy string

const (
	PolicyGeneratedReplace MergePolicy = "generated_replace"
	PolicyPreserveBlock    MergePolicy = "preserve_block"
)

type MergeOptions struct {
	Policy   MergePolicy
	Ref      string
	Position kicadfiles.Point
	Rotation kicadfiles.Angle
	Path     string
}

// MergeIdentityProperties merges KiCadAI-owned component identity properties
// into a schematic property slice while preserving unrelated properties.
func MergeIdentityProperties(existing []schematic.Property, evidence Evidence, opts MergeOptions) ([]schematic.Property, []reports.Issue) {
	policy := opts.Policy
	if policy == "" {
		policy = PolicyGeneratedReplace
	}
	path := strings.TrimSpace(opts.Path)
	if path == "" {
		path = "component_properties"
	}
	values := evidence.PropertyValues()
	merged := make([]schematic.Property, 0, len(existing)+len(values))
	present := map[string]struct{}{}
	var issues []reports.Issue
	for _, property := range existing {
		name := strings.TrimSpace(property.Name)
		normalized := normalizeName(name)
		if _, owned := ownedPropertyNames[normalized]; !owned {
			merged = append(merged, property)
			continue
		}
		canonical := canonicalNameFromNormalized(normalized, name)
		want, ok := values[canonical]
		if !ok {
			property.Name = canonical
			merged = append(merged, property)
			present[canonical] = struct{}{}
			continue
		}
		if strings.TrimSpace(property.Value) != want {
			issue := reports.Issue{
				Code:     reports.CodeValidationFailed,
				Severity: reports.SeverityWarning,
				Path:     path + "." + propertyPathName(name),
				Message:  fmt.Sprintf("%s identity property changed from %q to %q", name, property.Value, want),
			}
			if opts.Ref != "" {
				issue.Refs = []string{opts.Ref}
			}
			if policy == PolicyPreserveBlock {
				issue.Severity = reports.SeverityBlocked
				merged = append(merged, property)
				present[canonical] = struct{}{}
			}
			issues = append(issues, issue)
			if policy == PolicyGeneratedReplace {
				property.Name = canonical
				property.Value = want
				merged = append(merged, property)
				present[canonical] = struct{}{}
			}
			continue
		}
		property.Name = canonical
		merged = append(merged, property)
		present[canonical] = struct{}{}
	}
	if policy == PolicyPreserveBlock && reports.HasBlockingIssue(issues) {
		return append([]schematic.Property(nil), existing...), issues
	}
	for _, name := range orderedPropertyNames {
		value := values[name]
		if value == "" {
			continue
		}
		if _, ok := present[name]; ok {
			continue
		}
		merged = append(merged, schematic.Property{
			Name:           name,
			Value:          value,
			Hidden:         true,
			ShowName:       boolPtr(false),
			DoNotAutoplace: boolPtr(true),
			Position:       opts.Position,
			Rotation:       opts.Rotation,
		})
	}
	return merged, issues
}

// PropertyValues returns non-empty evidence values keyed by schematic property
// name.
func (e Evidence) PropertyValues() map[string]string {
	values := make(map[string]string, len(orderedPropertyNames))
	add := func(name, value string) {
		value = strings.TrimSpace(value)
		if value != "" {
			values[name] = value
		}
	}
	add(PropertyComponentID, e.ComponentID)
	add(PropertyVariantID, e.VariantID)
	add(PropertyComponentRole, e.ComponentRole)
	add(PropertyBlockID, e.BlockID)
	add(PropertyManufacturer, e.Manufacturer)
	add(PropertyMPN, e.MPN)
	add(PropertyComponentClass, e.ComponentClass)
	add(PropertyComponentConfidence, e.ComponentConfidence)
	add(PropertyComponentSource, e.ComponentSource)
	add(PropertyLifecycleStatus, e.LifecycleStatus)
	add(PropertyAvailabilityStatus, e.AvailabilityStatus)
	add(PropertyPinmapID, e.PinmapID)
	return values
}

// IsOwnedPropertyName reports whether a property name is managed by KiCadAI
// component identity propagation.
func IsOwnedPropertyName(name string) bool {
	_, ok := ownedPropertyNames[normalizeName(name)]
	return ok
}

func canonicalNameFromNormalized(normalized string, fallback string) string {
	if name, ok := canonicalPropertyNames[normalized]; ok {
		return name
	}
	return strings.TrimSpace(fallback)
}

func normalizeName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func propertyPathName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	name = strings.ReplaceAll(name, " ", "_")
	return name
}

func boolPtr(value bool) *bool {
	return &value
}
