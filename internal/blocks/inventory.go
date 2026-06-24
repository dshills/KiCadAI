package blocks

import (
	"cmp"
	"fmt"
	"slices"
)

type BlockReadiness string

const (
	BlockReadinessReady       BlockReadiness = "ready"
	BlockReadinessPartial     BlockReadiness = "partial"
	BlockReadinessUnsupported BlockReadiness = "unsupported"
)

type BlockFamilyInventory struct {
	ID                string               `json:"id"`
	Name              string               `json:"name"`
	Category          string               `json:"category,omitempty"`
	Readiness         BlockReadiness       `json:"readiness"`
	Implemented       bool                 `json:"implemented"`
	VerificationLevel VerificationLevel    `json:"verification_level,omitempty"`
	PCBLevel          PCBVerificationLevel `json:"pcb_level,omitempty"`
	SupportedVariants []string             `json:"supported_variants,omitempty"`
	RequiredParams    []string             `json:"required_params,omitempty"`
	RequiredRoles     []string             `json:"required_roles,omitempty"`
	ExportedPorts     []string             `json:"exported_ports,omitempty"`
	ElectricalRules   []string             `json:"electrical_rules,omitempty"`
	PCBRules          []string             `json:"pcb_rules,omitempty"`
	Gaps              []string             `json:"gaps,omitempty"`
}

type BlockLibraryInventory struct {
	Families []BlockFamilyInventory `json:"families"`
}

type roadmapBlockFamily struct {
	ID       string
	Name     string
	Category string
	Gaps     []string
}

var roadmapBlockFamilies = []roadmapBlockFamily{
	{ID: "led_indicator", Name: "LED Indicator", Category: "indicator"},
	{ID: "voltage_regulator", Name: "Voltage Regulator", Category: "power"},
	{ID: "mcu_minimal", Name: "MCU Minimal System", Category: "digital"},
	{ID: "usb_c_power", Name: "USB-C Power Input", Category: "power"},
	{ID: "i2c_sensor", Name: "I2C Sensor", Category: "sensor"},
	{ID: "opamp_gain_stage", Name: "Op-Amp Gain Stage", Category: "analog"},
	{ID: "connector_breakout", Name: "Connector Breakout", Category: "interconnect"},
	{
		ID:       "crystal_oscillator",
		Name:     "Crystal And Oscillator",
		Category: "timing",
		Gaps: []string{
			"requires MCU clock-pin composition rules",
			"oscillator startup margin is not simulated",
			"high-quality oscillator layout still requires human review",
		},
	},
	{
		ID:       "canned_oscillator",
		Name:     "Canned Oscillator",
		Category: "timing",
		Gaps: []string{
			"oscillator startup and jitter are not simulated",
			"consumer IC clock-pin composition rules are not automatic yet",
			"signal-integrity review is still required before fabrication",
		},
	},
	{
		ID:       "reset_programming_header",
		Name:     "Reset And Programming Header",
		Category: "digital",
		Gaps: []string{
			"requires target-specific programming interface metadata",
			"SWD/JTAG support is not implemented",
			"header edge placement still requires layout review",
		},
	},
	{
		ID:       "esd_protection",
		Name:     "ESD Protection",
		Category: "protection",
		Gaps: []string{
			"requires route-through placement rules",
			"requires signal-class-specific surge and capacitance selection",
		},
	},
	{
		ID:       "reverse_polarity_protection",
		Name:     "Reverse-Polarity Protection",
		Category: "protection",
		Gaps: []string{
			"requires ideal-diode MOSFET controller metadata",
			"requires current and thermal rating checks beyond the generic 1A Schottky seed",
		},
	},
}

func (registry BuiltinRegistry) Inventory() BlockLibraryInventory {
	families := make([]BlockFamilyInventory, 0, len(roadmapBlockFamilies))
	for _, family := range roadmapBlockFamilies {
		definition, ok := registry.GetBlock(family.ID)
		if !ok {
			families = append(families, BlockFamilyInventory{
				ID:          family.ID,
				Name:        family.Name,
				Category:    family.Category,
				Readiness:   BlockReadinessUnsupported,
				Implemented: false,
				Gaps:        slices.Clone(family.Gaps),
			})
			continue
		}
		families = append(families, inventoryForDefinition(definition, family.Gaps))
	}
	slices.SortFunc(families, func(a, b BlockFamilyInventory) int {
		return cmp.Compare(a.ID, b.ID)
	})
	return BlockLibraryInventory{Families: families}
}

func inventoryForDefinition(definition BlockDefinition, roadmapGaps []string) BlockFamilyInventory {
	inventory := BlockFamilyInventory{
		ID:                definition.ID,
		Name:              definition.Name,
		Category:          definition.Category,
		Implemented:       true,
		VerificationLevel: definition.Verification.Level,
		SupportedVariants: supportedBlockVariants(definition),
		RequiredParams:    requiredParameterNames(definition.Parameters),
		RequiredRoles:     componentRoles(definition.Components),
		ExportedPorts:     portNames(definition.Ports),
		ElectricalRules:   electricalRuleSummaries(definition),
		PCBRules:          pcbRuleSummaries(definition),
	}
	if definition.PCBRealization != nil {
		inventory.PCBLevel = definition.PCBRealization.VerificationLevel
		inventory.Gaps = append(inventory.Gaps, definition.PCBRealization.UnsupportedBehaviors...)
	}
	inventory.Gaps = append(inventory.Gaps, roadmapGaps...)
	switch {
	case definition.Verification.Level.AllowsFabricationReadinessClaim():
		inventory.Readiness = BlockReadinessReady
	case definition.Verification.Level != VerificationExperimental && definition.PCBRealization != nil:
		inventory.Readiness = BlockReadinessPartial
	default:
		inventory.Readiness = BlockReadinessUnsupported
	}
	if len(inventory.ElectricalRules) == 0 {
		inventory.Gaps = append(inventory.Gaps, "no electrical rules declared")
	}
	if len(inventory.PCBRules) == 0 {
		inventory.Gaps = append(inventory.Gaps, "no PCB rules declared")
	}
	inventory.Gaps = sortedUniqueStrings(inventory.Gaps)
	return inventory
}

func supportedBlockVariants(definition BlockDefinition) []string {
	var variants []string
	for _, parameter := range definition.Parameters {
		if parameter.Type != ParameterEnum || len(parameter.Allowed) == 0 {
			continue
		}
		for _, allowed := range parameter.Allowed {
			variants = append(variants, parameter.Name+"="+fmt.Sprint(allowed))
		}
	}
	return sortedUniqueStrings(variants)
}

func requiredParameterNames(parameters []BlockParameter) []string {
	var names []string
	for _, parameter := range parameters {
		if parameter.Required {
			names = append(names, parameter.Name)
		}
	}
	return sortedUniqueStrings(names)
}

func componentRoles(components []BlockComponent) []string {
	roles := make([]string, 0, len(components))
	for _, component := range components {
		if component.Role != "" {
			roles = append(roles, component.Role)
		}
	}
	return sortedUniqueStrings(roles)
}

func portNames(ports []BlockPort) []string {
	names := make([]string, 0, len(ports))
	for _, port := range ports {
		if port.Name != "" {
			names = append(names, port.Name)
		}
	}
	return sortedUniqueStrings(names)
}

func electricalRuleSummaries(definition BlockDefinition) []string {
	rules := make([]string, 0, len(definition.ValidationRules)+2*len(definition.Components)+len(definition.Nets))
	for _, rule := range definition.ValidationRules {
		rules = append(rules, rule.ID)
	}
	for _, component := range definition.Components {
		if component.PinmapRequired && component.Role != "" {
			rules = append(rules, "pinmap_required:"+component.Role)
		}
		if component.Role != "" && (component.ComponentID != "" || component.ComponentQuery != nil) {
			rules = append(rules, "component_evidence:"+component.Role)
		}
	}
	for _, net := range definition.Nets {
		if net.Role != "" {
			rules = append(rules, "net:"+net.Role)
		}
	}
	return sortedUniqueStrings(rules)
}

func pcbRuleSummaries(definition BlockDefinition) []string {
	if definition.PCBRealization == nil {
		return nil
	}
	realization := definition.PCBRealization
	rules := make([]string, 0, len(realization.PlacementGroups)+len(realization.LocalRoutes)+len(realization.Zones)+len(realization.Keepouts)+len(realization.Constraints))
	for _, group := range realization.PlacementGroups {
		rules = append(rules, "placement_group:"+group.ID)
	}
	for _, route := range realization.LocalRoutes {
		if route.Required {
			rules = append(rules, "required_route:"+route.ID)
		} else {
			rules = append(rules, "route:"+route.ID)
		}
	}
	for _, zone := range realization.Zones {
		rules = append(rules, "zone:"+zone.ID)
	}
	for _, keepout := range realization.Keepouts {
		rules = append(rules, "keepout:"+keepout.ID)
	}
	for _, constraint := range realization.Constraints {
		rules = append(rules, "constraint:"+constraint.ID)
	}
	return sortedUniqueStrings(rules)
}

func sortedUniqueStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	slices.Sort(values)
	return slices.Compact(values)
}
