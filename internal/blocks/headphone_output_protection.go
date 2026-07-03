package blocks

import (
	"math"
	"strings"

	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

const headphoneOutputProtectionID = "headphone_output_protection"

var supportedHeadphoneLoadOhms = []float64{16, 32, 64}

func headphoneOutputProtectionDefinition() BlockDefinition {
	return BlockDefinition{
		ID:          headphoneOutputProtectionID,
		Name:        "Headphone Output Protection",
		Description: "Connectivity-level AC-coupled headphone output load-safety fragment.",
		Version:     "0.1.0",
		Category:    "analog",
		Parameters: []BlockParameter{
			{Name: "load_kind", Type: ParameterEnum, Default: "headphone", Allowed: []any{"headphone", "speaker", "bridge", "unknown"}, Description: "Output load kind. Only headphone is supported in this slice."},
			{Name: "nominal_load_ohms", Type: ParameterResistance, Default: "32Ω", Description: "Nominal headphone load impedance. Supported classes are 16Ω, 32Ω, and 64Ω."},
			{Name: "coupling", Type: ParameterEnum, Default: "ac_coupled_required", Allowed: []any{"ac_coupled_required", "ac_coupled_present", "dual_rail_direct_review_required"}, Description: "Output coupling policy."},
			{Name: "dc_blocking_capacitance", Type: ParameterCapacitance, Default: "220uF", Description: "DC-blocking capacitor value for single-supply AC output coupling."},
			{Name: "bleed_resistor_ohms", Type: ParameterResistance, Default: "100kΩ", Description: "Reference or bleed resistor value for the AC-coupled output."},
			{Name: "bleed_required", Type: ParameterBool, Default: true, Description: "Require an explicit bleed/reference resistor policy."},
			{Name: "series_resistor_ohms", Type: ParameterResistance, Default: "0Ω", Description: "Series output resistor value; 0Ω represents a populated jumper until omission support is implemented."},
			{Name: "connector_return_policy", Type: ParameterEnum, Default: "load_ref", Allowed: []any{"load_ref", "analog_ground", "unknown"}, Description: "How the headphone connector return is referenced."},
			{Name: "fault_protection_status", Type: ParameterEnum, Default: "placeholder_blocked", Allowed: []any{"not_modeled", "placeholder_blocked", "connectivity"}, Description: "Fault protection evidence status. Connectivity is reserved for future verified fault-protection work."},
		},
		Ports: []BlockPort{
			{Name: "AMP_OUT", Direction: PortInput, Description: "DC-biased amplifier output before AC coupling."},
			{Name: "HP_OUT", Direction: PortOutput, Description: "AC-coupled headphone signal output."},
			{Name: "LOAD_RET", Direction: PortPassive, Description: "Headphone connector return."},
			{Name: "LOAD_REF", Direction: PortPassive, Description: "Load reference or analog ground."},
		},
		RequiredLibraries: []LibraryRequirement{
			{Kind: "symbol", ID: "Device:C_Polarized", Required: true, Description: "Series DC-blocking capacitor symbol."},
			{Kind: "symbol", ID: "Device:R", Required: true, Description: "Bleed/reference and optional series output resistor symbols."},
			{Kind: "symbol", ID: "Connector:TestPoint", Required: true, Description: "Load-return structural anchor."},
			{Kind: "footprint", ID: "Capacitor_SMD:C_1210_3225Metric", Required: true, Description: "Default coupling capacitor footprint."},
			{Kind: "footprint", ID: "Resistor_SMD:R_0805_2012Metric", Required: true, Description: "Default protection resistor footprint."},
			{Kind: "footprint", ID: "TestPoint:TestPoint_Pad_D1.0mm", Required: true, Description: "Default load-return anchor footprint."},
		},
		Components: []BlockComponent{
			{Role: "dc_blocking_capacitor", RefPrefix: "C", Value: "220uF", SymbolID: "Device:C_Polarized", FootprintID: "Capacitor_SMD:C_1210_3225Metric", Pins: twoTerminalHorizontalPins(), PlacementGroup: "output_protection"},
			{Role: "bleed_resistor", RefPrefix: "R", Value: "100kΩ", SymbolID: "Device:R", FootprintID: "Resistor_SMD:R_0805_2012Metric", Pins: twoTerminalHorizontalPins(), PlacementGroup: "output_protection"},
			{Role: "series_resistor", RefPrefix: "R", Value: "0Ω", SymbolID: "Device:R", FootprintID: "Resistor_SMD:R_0805_2012Metric", Pins: twoTerminalHorizontalPins(), PlacementGroup: "output_protection"},
			{Role: "load_return_anchor", RefPrefix: "TP", Value: "LOAD_RET", SymbolID: "Connector:TestPoint", FootprintID: "TestPoint:TestPoint_Pad_D1.0mm", Pins: []transactions.PinSpec{{Number: "1"}}, PlacementGroup: "output_protection"},
		},
		// BlockNet captures component-pin membership only. Exported block ports
		// are tied to these nets by Connect operations during realization.
		Nets: []BlockNet{
			{NameTemplate: "amp_out_dc_biased", Visibility: "exported", Role: "dc_biased_amplifier_output", Pins: []NetPin{{ComponentRole: "dc_blocking_capacitor", Pin: "1"}}},
			{NameTemplate: "coupled_output", Visibility: "local", Role: "ac_coupled_pre_series_output", Pins: []NetPin{{ComponentRole: "dc_blocking_capacitor", Pin: "2"}, {ComponentRole: "bleed_resistor", Pin: "1"}, {ComponentRole: "series_resistor", Pin: "1"}}},
			{NameTemplate: "hp_out", Visibility: "exported", Role: "ac_coupled_headphone_output", Pins: []NetPin{{ComponentRole: "series_resistor", Pin: "2"}}},
			{NameTemplate: "load_ref", Visibility: "exported", Role: "headphone_load_reference", Pins: []NetPin{{ComponentRole: "bleed_resistor", Pin: "2"}}},
			{NameTemplate: "load_ret", Visibility: "exported", Role: "headphone_connector_return", Pins: []NetPin{{ComponentRole: "load_return_anchor", Pin: "1"}}},
		},
		ValidationRules: []BlockValidationRule{
			{ID: "headphone_protection.load_kind.headphone_only", Severity: BlockValidationSeverityBlocked, Description: "Only headphone loads are supported by this output-protection block."},
			{ID: "headphone_protection.load.supported_impedance", Severity: BlockValidationSeverityBlocked, Description: "Supported headphone load classes are 16Ω, 32Ω, and 64Ω."},
			{ID: "headphone_protection.coupling.ac_required", Severity: BlockValidationSeverityBlocked, Description: "Single-supply headphone outputs require AC coupling through a DC-blocking capacitor."},
			{ID: "headphone_protection.bleed.required", Severity: BlockValidationSeverityBlocked, Description: "Required bleed/reference resistor policy must include a positive resistance value."},
			{ID: "headphone_protection.return.reference", Severity: BlockValidationSeverityBlocked, Description: "Headphone connector return must reference load_ref or analog_ground."},
			{ID: "headphone_protection.fault.unverified", Severity: BlockValidationSeverityBlocked, Description: "Fault protection remains unverified and blocks higher amplifier readiness."},
		},
		PCBRealization: &PCBRealization{
			Version:           "0.1.0",
			VerificationLevel: PCBVerificationUnrealized,
			UnsupportedBehaviors: []string{
				"fault protection is not modeled",
				"speaker and bridge-tied output protection are intentionally blocked",
				"KiCad-backed load-safety proof is not yet available",
			},
		},
		Verification: VerificationRecord{
			Level: VerificationStructural,
			Notes: []string{
				"Phase 1 model validates supported headphone load-safety assumptions but does not emit schematic operations yet.",
				"Speaker, bridge-output, active fault protection, and power-amplifier safety remain blocked.",
			},
		},
	}
}

func instantiateHeadphoneOutputProtection(definition BlockDefinition, request BlockRequest, params map[string]any, issues []reports.Issue) BlockOutput {
	defaulted := ApplyParameterDefaults(definition, params)
	issues = append(issues, validateHeadphoneOutputProtectionParams(defaulted)...)
	definition = headphoneOutputProtectionDefinitionForParams(definition, defaulted)
	output := dryRunBlockOutput(definition, request, nil, issues)
	output.Instance.Params = defaulted
	return output
}

func headphoneOutputProtectionDefinitionForParams(definition BlockDefinition, params map[string]any) BlockDefinition {
	definition.Components = append([]BlockComponent(nil), definition.Components...)
	for index := range definition.Components {
		switch definition.Components[index].Role {
		case "dc_blocking_capacitor":
			if value := strings.TrimSpace(stringParam(params, "dc_blocking_capacitance")); value != "" {
				definition.Components[index].Value = value
			}
		case "bleed_resistor":
			if value := strings.TrimSpace(stringParam(params, "bleed_resistor_ohms")); value != "" {
				definition.Components[index].Value = value
			}
		case "series_resistor":
			if value := strings.TrimSpace(stringParam(params, "series_resistor_ohms")); value != "" {
				definition.Components[index].Value = value
			}
		}
	}
	return definition
}

func validateHeadphoneOutputProtectionParams(params map[string]any) []reports.Issue {
	var issues []reports.Issue
	if strings.TrimSpace(stringParam(params, "load_kind")) != "headphone" {
		issues = append(issues, blockIssue("params.load_kind", "only headphone loads are supported; speaker, bridge, and unknown outputs remain blocked"))
	}
	loadOhms, loadOK := parseUnit(params["nominal_load_ohms"], "Ω", resistanceMultipliers())
	if !loadOK {
		issues = append(issues, blockIssue("params.nominal_load_ohms", "nominal_load_ohms must be a resistance literal"))
	} else if !supportedHeadphoneLoad(loadOhms) {
		issues = append(issues, blockIssue("params.nominal_load_ohms", "supported headphone load classes are 16Ω, 32Ω, and 64Ω"))
	}
	coupling := strings.TrimSpace(stringParam(params, "coupling"))
	if coupling != "ac_coupled_required" && coupling != "ac_coupled_present" {
		issues = append(issues, blockIssue("params.coupling", "single-supply headphone outputs require AC coupling through a DC-blocking capacitor"))
	}
	if _, ok := parseUnit(params["dc_blocking_capacitance"], "F", capacitanceMultipliers()); !ok {
		issues = append(issues, blockIssue("params.dc_blocking_capacitance", "dc_blocking_capacitance must be a capacitance literal"))
	}
	if boolParam(params, "bleed_required", true) {
		bleedOhms, bleedOK := parseUnit(params["bleed_resistor_ohms"], "Ω", resistanceMultipliers())
		if !bleedOK {
			issues = append(issues, blockIssue("params.bleed_resistor_ohms", "bleed_resistor_ohms must be a resistance literal when bleed_required is true"))
		} else if bleedOhms <= 0 {
			issues = append(issues, blockIssue("params.bleed_resistor_ohms", "bleed_resistor_ohms must be positive when bleed_required is true"))
		}
	}
	seriesOhms, seriesOK := parseUnit(params["series_resistor_ohms"], "Ω", resistanceMultipliers())
	if !seriesOK {
		issues = append(issues, blockIssue("params.series_resistor_ohms", "series_resistor_ohms must be a resistance literal"))
	} else if seriesOhms < 0 {
		issues = append(issues, blockIssue("params.series_resistor_ohms", "series_resistor_ohms must not be negative"))
	}
	returnPolicy := strings.TrimSpace(stringParam(params, "connector_return_policy"))
	if returnPolicy != "load_ref" && returnPolicy != "analog_ground" {
		issues = append(issues, blockIssue("params.connector_return_policy", "connector_return_policy must be load_ref or analog_ground"))
	}
	if strings.TrimSpace(stringParam(params, "fault_protection_status")) == "connectivity" {
		issues = append(issues, blockIssue("params.fault_protection_status", "fault protection connectivity is not verified in this slice"))
	}
	return issues
}

func supportedHeadphoneLoad(loadOhms float64) bool {
	for _, supported := range supportedHeadphoneLoadOhms {
		if math.Abs(loadOhms-supported) < 0.001 {
			return true
		}
	}
	return false
}
