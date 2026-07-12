package circuitgraph

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"kicadai/internal/components"
)

type providerCapability struct {
	Schema     string                        `json:"schema"`
	Rules      []string                      `json:"rules"`
	Components []providerCapabilityComponent `json:"components"`
}

type providerCapabilityComponent struct {
	ID           string                       `json:"id"`
	Family       string                       `json:"family"`
	Manufacturer string                       `json:"manufacturer,omitempty"`
	MPN          string                       `json:"mpn,omitempty"`
	Values       []components.ValueConstraint `json:"values,omitempty"`
	Variants     []providerCapabilityVariant  `json:"variants"`
	Functions    []string                     `json:"functions"`
}

type providerCapabilityVariant struct {
	ID      string `json:"id"`
	Package string `json:"package,omitempty"`
}

func ProviderCapabilityContext(catalog *components.Catalog, maxBytes int) (string, error) {
	if catalog == nil {
		return "", fmt.Errorf("component catalog is required")
	}
	capability := providerCapability{
		Schema: ProviderProfileID,
		Rules: []string{
			"Use only listed component IDs and variant IDs, or constrained catalog queries.",
			"Use listed logical functions or verified symbol pins for net endpoints.",
			"Put nominal resistance, capacitance, frequency, and similar design values in component value or query; do not repeat them as required ratings.",
			"Use required ratings only for limits such as voltage, current, power, thermal, or frequency stability that the selected catalog record explicitly proves.",
			"Do not invent symbols, footprints, pins, pads, paths, commands, URLs, or coordinates.",
			"Describe electrical and relative layout intent only; KiCadAI resolves physical evidence and geometry.",
			"Leave net_class empty for role-based defaults, or use only signal, clock, power, or ground.",
			"Prefer bounded PCB regions for left-to-right placement; do not use hard PCB edge constraints unless the prompt explicitly requires an edge-mounted part.",
		},
	}
	records := append([]components.ComponentRecord(nil), catalog.Records...)
	sort.SliceStable(records, func(i, j int) bool { return records[i].ID < records[j].ID })
	for _, record := range records {
		entry := providerCapabilityComponent{ID: record.ID, Family: record.Family, Manufacturer: record.Manufacturer, MPN: record.MPN, Values: append([]components.ValueConstraint(nil), record.Values...)}
		functionSet := map[string]struct{}{}
		for _, symbol := range record.Symbols {
			for _, pin := range symbol.FunctionPins {
				if function := strings.TrimSpace(pin.Function); function != "" {
					functionSet[function] = struct{}{}
				}
			}
		}
		for function := range functionSet {
			entry.Functions = append(entry.Functions, function)
		}
		sort.Strings(entry.Functions)
		for _, variant := range record.Packages {
			entry.Variants = append(entry.Variants, providerCapabilityVariant{ID: variant.ID, Package: variant.PackageType})
		}
		sort.SliceStable(entry.Variants, func(i, j int) bool { return entry.Variants[i].ID < entry.Variants[j].ID })
		capability.Components = append(capability.Components, entry)
	}
	data, err := json.Marshal(capability)
	if err != nil {
		return "", fmt.Errorf("encode generic circuit capability: %w", err)
	}
	if maxBytes > 0 && len(data) > maxBytes {
		return "", fmt.Errorf("generic circuit capability is %d bytes, exceeds %d-byte provider limit", len(data), maxBytes)
	}
	return string(data), nil
}
