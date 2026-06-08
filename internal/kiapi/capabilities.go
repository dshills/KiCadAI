package kiapi

import (
	"fmt"

	commontypes "kicadai/internal/kiapi/gen/common/types"
)

// Capability names a high-level KiCad API feature used by schematic workflows.
type Capability string

const (
	// CapabilitySchematicRead means hierarchy/netlist schematic read commands are available.
	CapabilitySchematicRead Capability = "schematic.read"
	// CapabilitySchematicWrite means generic schematic mutation commands are available.
	CapabilitySchematicWrite Capability = "schematic.write"
	// CapabilitySymbolPlace means a workflow can place schematic symbols through the API.
	CapabilitySymbolPlace Capability = "schematic.symbol.place"
	// CapabilityWirePlace means a workflow can place schematic wires through the API.
	CapabilityWirePlace Capability = "schematic.wire.place"
	// CapabilityLabelPlace means a workflow can place schematic labels through the API.
	CapabilityLabelPlace Capability = "schematic.label.place"
)

type Capabilities struct {
	KiCadVersion string       `json:"kicad_version"`
	Supported    []Capability `json:"supported"`
	Missing      []Capability `json:"missing"`
	Notes        []string     `json:"notes"`
	Error        string       `json:"error,omitempty"`
}

// CapabilitiesForVersion evaluates supported workflow capabilities for a KiCad version.
func CapabilitiesForVersion(version *commontypes.KiCadVersion) Capabilities {
	capabilities := Capabilities{
		KiCadVersion: versionString(version),
		Supported:    []Capability{},
		Missing:      []Capability{},
		Notes: []string{
			"Capability decisions are based on the generated KiCad API surface pinned in this repository.",
			"Generated bindings do not expose schematic mutation commands for placing symbols, wires, or labels.",
		},
	}
	if version != nil && version.GetMajor() >= 9 {
		capabilities.Supported = append(capabilities.Supported, CapabilitySchematicRead)
		capabilities.Notes = append(capabilities.Notes, "Generated KiCad schematic API bindings expose hierarchy and netlist read commands.")
	} else {
		capabilities.Missing = append(capabilities.Missing, CapabilitySchematicRead)
	}
	capabilities.Missing = append(capabilities.Missing,
		CapabilitySchematicWrite,
		CapabilitySymbolPlace,
		CapabilityWirePlace,
		CapabilityLabelPlace,
	)
	return capabilities
}

func versionString(version *commontypes.KiCadVersion) string {
	if version == nil {
		return "unknown"
	}
	if version.GetFullVersion() != "" {
		return version.GetFullVersion()
	}
	return fmt.Sprintf("%d.%d.%d", version.GetMajor(), version.GetMinor(), version.GetPatch())
}

func (c Capabilities) Supports(capability Capability) bool {
	for _, supported := range c.Supported {
		if supported == capability {
			return true
		}
	}
	return false
}
