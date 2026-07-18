package blocks

import (
	"math"
	"strings"

	"kicadai/internal/components"
	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

const classABSpeakerPowerStageID = "class_ab_speaker_power_stage"

func classABSpeakerPowerStageDefinition() BlockDefinition {
	return BlockDefinition{
		ID:          classABSpeakerPowerStageID,
		Name:        "Class AB Speaker Power Stage",
		Description: "BCE-aware complementary BJT speaker stage with drivers, thermally tracked bias string, emitter sensing, current limiting, and Zobel damping.",
		Version:     "0.1.0",
		Category:    "analog_power",
		Parameters: []BlockParameter{
			{Name: "supply_voltage", Type: ParameterVoltage, Default: "36V", Description: "Total positive-to-negative rail voltage."},
			{Name: "target_power", Type: ParameterNumber, Default: 10.0, Description: "Minimum RMS output power in watts."},
			{Name: "target_load", Type: ParameterResistance, Default: "8Ω", Description: "Nominal 2-32 ohm speaker load."},
			{Name: "minimum_load", Type: ParameterResistance, Default: "4Ω", Description: "Minimum validated resistive load, from 2 ohms through the nominal load."},
			{Name: "output_stage_loss", Type: ParameterVoltage, Default: "4V", Description: "Conservative rail-to-peak loss through the driver and output stage."},
			{Name: "current_limit", Type: ParameterCurrent, Default: "3A", Description: "Emitter-sense output current limit."},
			{Name: "current_limit_vbe", Type: ParameterVoltage, Default: "0.66V", Description: "Reviewed nominal room-temperature limiter-transistor base-emitter trigger voltage."},
			{Name: "current_limit_vbe_tolerance", Type: ParameterVoltage, Default: "0.05V", Description: "Allowed nominal circuit-model mismatch; the 50 mV default covers about 25 C of typical -2 mV/C VBE shift, while electrothermal SOA validation remains authoritative."},
			{Name: "emitter_resistor_value", Type: ParameterResistance, Default: "0.22Ω", Description: "Per-device emitter/sense resistance."},
			{Name: "bias_feed_resistor_value", Type: ParameterResistance, Default: "10kΩ", Description: "Rail-to-bias-string feed resistance."},
			{Name: "upper_driver_component_id", Type: ParameterString, Default: "bjt.onsemi.mje243g.to225", Description: "Concrete NPN driver component ID."},
			{Name: "lower_driver_component_id", Type: ParameterString, Default: "bjt.onsemi.mje253g.to225", Description: "Concrete PNP driver component ID."},
			{Name: "upper_output_component_id", Type: ParameterString, Default: "bjt.onsemi.njw0281g.to3p", Description: "Concrete NPN power-output component ID."},
			{Name: "lower_output_component_id", Type: ParameterString, Default: "bjt.onsemi.njw0302g.to3p", Description: "Concrete PNP power-output component ID."},
		},
		Ports: []BlockPort{
			{Name: "DRIVER_OUT", Direction: PortInput, Description: "Ground-referenced small-signal drive from the voltage amplifier."},
			{Name: "VCC", Direction: PortPower, Description: "Positive power rail."},
			{Name: "VEE", Direction: PortPower, Description: "Negative power rail."},
			{Name: "POWER_STAR", Direction: PortPower, Description: "Signal, Zobel, and speaker-return star point."},
			{Name: "RAW_OUT", Direction: PortOutput, NetClass: "power_output", Description: "Protected-stage input and Kelvin feedback source."},
		},
		RequiredLibraries: []LibraryRequirement{
			{Kind: "symbol", ID: "Device:D", Required: true, Description: "Thermally tracked bias diodes."},
			{Kind: "symbol", ID: "Device:R", Required: true, Description: "Bias, emitter, and Zobel resistors."},
			{Kind: "symbol", ID: "Device:C", Required: true, Description: "Zobel capacitor."},
			{Kind: "symbol", ID: "Transistor_BJT:Q_NPN_ECB", Required: true, Description: "NPN driver pin contract."},
			{Kind: "symbol", ID: "Transistor_BJT:Q_PNP_ECB", Required: true, Description: "PNP driver pin contract."},
			{Kind: "footprint", ID: "Capacitor_THT:C_Rect_L7.2mm_W3.0mm_P5.00mm_FKS2_FKP2_MKS2_MKP2", Required: true, Description: "Film Zobel capacitor footprint."},
			{Kind: "symbol", ID: "Transistor_BJT:Q_NPN_BCE", Required: true, Description: "NPN TO-3P output pin contract."},
			{Kind: "symbol", ID: "Transistor_BJT:Q_PNP_BCE", Required: true, Description: "PNP TO-3P output pin contract."},
			{Kind: "footprint", ID: "Package_TO_SOT_THT:TO-126-3_Vertical", Required: true, Description: "Driver footprint."},
			{Kind: "footprint", ID: "Package_TO_SOT_THT:TO-3P-3_Vertical", Required: true, Description: "Power-output footprint."},
			{Kind: "footprint", ID: "Resistor_THT:R_Axial_DIN0414_L11.9mm_D4.5mm_P20.32mm_Horizontal", Required: true, Description: "Emitter and Zobel power-resistor footprint."},
		},
		Components:     classABSpeakerPowerStageComponents(),
		Nets:           classABSpeakerPowerStageNets(),
		SchematicHints: classABSpeakerPowerStageSchematicHints(),
		PCBRealization: classABSpeakerPowerStagePCBRealization(),
		ValidationRules: []BlockValidationRule{
			{ID: "speaker_power.output.requested_power", Severity: BlockValidationSeverityBlocked, Description: "Rail, loss, current-limit, and load parameters must deliver the declared RMS power."},
			{ID: "speaker_power.pair.fabrication_evidence", Severity: BlockValidationSeverityBlocked, Description: "Driver and output pairs require complementary fabrication-proven semiconductor evidence."},
			{ID: "speaker_power.current_limit.emitter_sense", Severity: BlockValidationSeverityBlocked, Description: "Both polarities require emitter-resistor current limiting."},
			{ID: "speaker_power.bias.thermal_tracking", Severity: BlockValidationSeverityBlocked, Description: "The four-junction bias string requires quantified thermal placement."},
			{ID: "speaker_power.feedback.kelvin", Severity: BlockValidationSeverityBlocked, Description: "Feedback must originate at the emitter-resistor output join."},
			{ID: "speaker_power.high_current.width", Severity: BlockValidationSeverityBlocked, Description: "Rail, emitter, raw-output, and star-return paths require the speaker-power net class."},
			{ID: "speaker_power.heatsink.mechanical", Severity: BlockValidationSeverityBlocked, Description: "Output devices require shared-heatsink edge placement, keepout, and mounting access."},
		},
		Verification: VerificationRecord{
			Level: VerificationStructural,
			Evidence: []string{
				"component:bjt.onsemi.mje243g.to225",
				"component:bjt.onsemi.mje253g.to225",
				"component:bjt.onsemi.njw0281g.to3p",
				"component:bjt.onsemi.njw0302g.to3p",
				"component:resistor.vishay.ac03.0r22.axial",
			},
			Notes: []string{"BCE/ECB pin contracts, high-current paths, Kelvin sense, thermal coupling, and mechanical heatsink constraints are explicit and parameter driven."},
		},
	}
}

func classABSpeakerPowerStageComponents() []BlockComponent {
	verified := components.ConfidenceVerified
	fabrication := components.AcceptanceFabricationCandidate
	connectivity := components.AcceptanceConnectivity
	return []BlockComponent{
		{Role: "upper_bias_feed", RefPrefix: "R", Value: "10kΩ", SymbolID: "Device:R", FootprintID: "Resistor_SMD:R_0805_2012Metric", Pins: twoTerminalHorizontalPins(), ComponentID: "resistor.yageo.rc0805fr_0710kl.0805", ComponentVariant: "0805", MinimumConfidence: verified, Acceptance: connectivity, PinmapRequired: true, PlacementGroup: "bias_driver"},
		{Role: "bias_diode_1", RefPrefix: "D", Value: "1N4148W", SymbolID: "Device:D", FootprintID: "Diode_SMD:D_SOD-123", Pins: twoTerminalHorizontalPins(), PreferResolverSymbol: true, ComponentID: "diode.onsemi.1n4148w.sod_123", ComponentVariant: "sod_123", MinimumConfidence: verified, Acceptance: connectivity, PinmapRequired: true, PlacementGroup: "bias_driver"},
		{Role: "bias_diode_2", RefPrefix: "D", Value: "1N4148W", SymbolID: "Device:D", FootprintID: "Diode_SMD:D_SOD-123", Pins: twoTerminalHorizontalPins(), PreferResolverSymbol: true, ComponentID: "diode.onsemi.1n4148w.sod_123", ComponentVariant: "sod_123", MinimumConfidence: verified, Acceptance: connectivity, PinmapRequired: true, PlacementGroup: "bias_driver"},
		{Role: "bias_diode_3", RefPrefix: "D", Value: "1N4148W", SymbolID: "Device:D", FootprintID: "Diode_SMD:D_SOD-123", Pins: twoTerminalHorizontalPins(), PreferResolverSymbol: true, ComponentID: "diode.onsemi.1n4148w.sod_123", ComponentVariant: "sod_123", MinimumConfidence: verified, Acceptance: connectivity, PinmapRequired: true, PlacementGroup: "bias_driver"},
		{Role: "bias_diode_4", RefPrefix: "D", Value: "1N4148W", SymbolID: "Device:D", FootprintID: "Diode_SMD:D_SOD-123", Pins: twoTerminalHorizontalPins(), PreferResolverSymbol: true, ComponentID: "diode.onsemi.1n4148w.sod_123", ComponentVariant: "sod_123", MinimumConfidence: verified, Acceptance: connectivity, PinmapRequired: true, PlacementGroup: "bias_driver"},
		{Role: "lower_bias_feed", RefPrefix: "R", Value: "10kΩ", SymbolID: "Device:R", FootprintID: "Resistor_SMD:R_0805_2012Metric", Pins: twoTerminalHorizontalPins(), ComponentID: "resistor.yageo.rc0805fr_0710kl.0805", ComponentVariant: "0805", MinimumConfidence: verified, Acceptance: connectivity, PinmapRequired: true, PlacementGroup: "bias_driver"},
		{Role: "upper_driver", RefPrefix: "Q", Value: "MJE243G", SymbolID: "Transistor_BJT:Q_NPN_ECB", FootprintID: "Package_TO_SOT_THT:TO-126-3_Vertical", Pins: speakerDriverECBPins(), PreferResolverSymbol: true, ComponentIDParam: "upper_driver_component_id", MinimumConfidence: verified, Acceptance: fabrication, PinmapRequired: true, PlacementGroup: "driver_output"},
		{Role: "lower_driver", RefPrefix: "Q", Value: "MJE253G", SymbolID: "Transistor_BJT:Q_PNP_ECB", FootprintID: "Package_TO_SOT_THT:TO-126-3_Vertical", Pins: speakerDriverECBPins(), PreferResolverSymbol: true, ComponentIDParam: "lower_driver_component_id", MinimumConfidence: verified, Acceptance: fabrication, PinmapRequired: true, PlacementGroup: "driver_output"},
		{Role: "upper_driver_base_stopper", RefPrefix: "R", Value: "47Ω", SymbolID: "Device:R", FootprintID: "Resistor_SMD:R_0805_2012Metric", Pins: twoTerminalHorizontalPins(), PreferResolverSymbol: true, ComponentID: "resistor.yageo.rc0805fr_0747rl.0805", ComponentVariant: "0805", MinimumConfidence: verified, Acceptance: fabrication, PinmapRequired: true, PlacementGroup: "bias_driver"},
		{Role: "lower_driver_base_stopper", RefPrefix: "R", Value: "47Ω", SymbolID: "Device:R", FootprintID: "Resistor_SMD:R_0805_2012Metric", Pins: twoTerminalHorizontalPins(), PreferResolverSymbol: true, ComponentID: "resistor.yageo.rc0805fr_0747rl.0805", ComponentVariant: "0805", MinimumConfidence: verified, Acceptance: fabrication, PinmapRequired: true, PlacementGroup: "bias_driver"},
		{Role: "upper_output", RefPrefix: "Q", Value: "NJW0281G", SymbolID: "Transistor_BJT:Q_NPN_BCE", FootprintID: "Package_TO_SOT_THT:TO-3P-3_Vertical", Pins: speakerOutputBCEPins(), PreferResolverSymbol: true, ComponentIDParam: "upper_output_component_id", MinimumConfidence: verified, Acceptance: fabrication, PinmapRequired: true, PlacementGroup: "driver_output"},
		{Role: "lower_output", RefPrefix: "Q", Value: "NJW0302G", SymbolID: "Transistor_BJT:Q_PNP_BCE", FootprintID: "Package_TO_SOT_THT:TO-3P-3_Vertical", Pins: speakerOutputBCEPins(), PreferResolverSymbol: true, ComponentIDParam: "lower_output_component_id", MinimumConfidence: verified, Acceptance: fabrication, PinmapRequired: true, PlacementGroup: "driver_output"},
		{Role: "upper_emitter_resistor", RefPrefix: "R", Value: "0.22Ω", SymbolID: "Device:R", FootprintID: "Resistor_THT:R_Axial_DIN0414_L11.9mm_D4.5mm_P20.32mm_Horizontal", Pins: twoTerminalHorizontalPins(), PreferResolverSymbol: true, ComponentID: "resistor.vishay.ac03.0r22.axial", ComponentVariant: "axial_ac03", MinimumConfidence: verified, Acceptance: fabrication, PinmapRequired: true, PlacementGroup: "driver_output"},
		{Role: "lower_emitter_resistor", RefPrefix: "R", Value: "0.22Ω", SymbolID: "Device:R", FootprintID: "Resistor_THT:R_Axial_DIN0414_L11.9mm_D4.5mm_P20.32mm_Horizontal", Pins: twoTerminalHorizontalPins(), PreferResolverSymbol: true, ComponentID: "resistor.vishay.ac03.0r22.axial", ComponentVariant: "axial_ac03", MinimumConfidence: verified, Acceptance: fabrication, PinmapRequired: true, PlacementGroup: "driver_output"},
		{Role: "upper_current_limit", RefPrefix: "Q", Value: "MMBT3904", SymbolID: "Transistor_BJT:Q_NPN_BEC", FootprintID: "Package_TO_SOT_SMD:SOT-23", Pins: bjtBECPins(), PreferResolverSymbol: true, ComponentID: "bjt.onsemi.mmbt3904.sot23", ComponentVariant: "sot23", MinimumConfidence: verified, Acceptance: fabrication, PinmapRequired: true, PlacementGroup: "current_limit"},
		{Role: "lower_current_limit", RefPrefix: "Q", Value: "MMBT3906", SymbolID: "Transistor_BJT:Q_PNP_BEC", FootprintID: "Package_TO_SOT_SMD:SOT-23", Pins: pnpBECPins(), PreferResolverSymbol: true, ComponentID: "bjt.onsemi.mmbt3906.sot23", ComponentVariant: "sot23", MinimumConfidence: verified, Acceptance: fabrication, PinmapRequired: true, PlacementGroup: "current_limit"},
		{Role: "zobel_resistor", RefPrefix: "R", Value: "10Ω", SymbolID: "Device:R", FootprintID: "Resistor_THT:R_Axial_DIN0414_L11.9mm_D4.5mm_P20.32mm_Horizontal", Pins: twoTerminalHorizontalPins(), PreferResolverSymbol: true, ComponentID: "resistor.vishay.ac03.10r.axial", ComponentVariant: "axial_ac03", MinimumConfidence: verified, Acceptance: fabrication, PinmapRequired: true, PlacementGroup: "output_damping"},
		{Role: "zobel_capacitor", RefPrefix: "C", Value: "100nF", SymbolID: "Device:C", FootprintID: "Capacitor_THT:C_Rect_L7.2mm_W3.0mm_P5.00mm_FKS2_FKP2_MKS2_MKP2", Pins: twoTerminalHorizontalPins(), ComponentID: "capacitor.wima.mks2c031001a00kssd.tht", ComponentVariant: "mks2_pcm5", MinimumConfidence: verified, Acceptance: fabrication, PinmapRequired: true, PlacementGroup: "output_damping"},
	}
}

func classABSpeakerPowerStageNets() []BlockNet {
	return []BlockNet{
		{NameTemplate: "vcc", Visibility: "exported", Role: "positive_power_rail", Pins: []NetPin{{ComponentRole: "upper_bias_feed", Pin: "1"}, {ComponentRole: "upper_driver", Pin: "2"}, {ComponentRole: "upper_output", Pin: "2"}}},
		{NameTemplate: "vee", Visibility: "exported", Role: "negative_power_rail", Pins: []NetPin{{ComponentRole: "lower_bias_feed", Pin: "2"}, {ComponentRole: "lower_driver", Pin: "2"}, {ComponentRole: "lower_output", Pin: "2"}}},
		{NameTemplate: "upper_drive_source", Visibility: "local", Role: "upper_driver_base_source", Pins: []NetPin{{ComponentRole: "upper_bias_feed", Pin: "2"}, {ComponentRole: "bias_diode_1", Pin: "2"}, {ComponentRole: "upper_driver_base_stopper", Pin: "1"}}},
		{NameTemplate: "upper_drive", Visibility: "local", Role: "upper_driver_base", Pins: []NetPin{{ComponentRole: "upper_driver_base_stopper", Pin: "2"}, {ComponentRole: "upper_driver", Pin: "3"}, {ComponentRole: "upper_current_limit", Pin: "3"}}},
		{NameTemplate: "upper_bias_mid", Visibility: "local", Role: "bias_string", Pins: []NetPin{{ComponentRole: "bias_diode_1", Pin: "1"}, {ComponentRole: "bias_diode_2", Pin: "2"}}},
		{NameTemplate: "driver_center", Visibility: "exported", Role: "small_signal_driver", Pins: []NetPin{{ComponentRole: "bias_diode_2", Pin: "1"}, {ComponentRole: "bias_diode_3", Pin: "2"}}},
		{NameTemplate: "lower_bias_mid", Visibility: "local", Role: "bias_string", Pins: []NetPin{{ComponentRole: "bias_diode_3", Pin: "1"}, {ComponentRole: "bias_diode_4", Pin: "2"}}},
		{NameTemplate: "lower_drive_source", Visibility: "local", Role: "lower_driver_base_source", Pins: []NetPin{{ComponentRole: "bias_diode_4", Pin: "1"}, {ComponentRole: "lower_bias_feed", Pin: "1"}, {ComponentRole: "lower_driver_base_stopper", Pin: "1"}}},
		{NameTemplate: "lower_drive", Visibility: "local", Role: "lower_driver_base", Pins: []NetPin{{ComponentRole: "lower_driver_base_stopper", Pin: "2"}, {ComponentRole: "lower_driver", Pin: "3"}, {ComponentRole: "lower_current_limit", Pin: "3"}}},
		{NameTemplate: "upper_power_base", Visibility: "local", Role: "upper_output_base", Pins: []NetPin{{ComponentRole: "upper_driver", Pin: "1"}, {ComponentRole: "upper_output", Pin: "1"}}},
		{NameTemplate: "lower_power_base", Visibility: "local", Role: "lower_output_base", Pins: []NetPin{{ComponentRole: "lower_driver", Pin: "1"}, {ComponentRole: "lower_output", Pin: "1"}}},
		{NameTemplate: "upper_sense", Visibility: "local", Role: "upper_emitter_kelvin", Pins: []NetPin{{ComponentRole: "upper_output", Pin: "3"}, {ComponentRole: "upper_emitter_resistor", Pin: "1"}, {ComponentRole: "upper_current_limit", Pin: "1"}}},
		{NameTemplate: "lower_sense", Visibility: "local", Role: "lower_emitter_kelvin", Pins: []NetPin{{ComponentRole: "lower_output", Pin: "3"}, {ComponentRole: "lower_emitter_resistor", Pin: "1"}, {ComponentRole: "lower_current_limit", Pin: "1"}}},
		{NameTemplate: "raw_out", Visibility: "exported", Role: "speaker_raw_output", Pins: []NetPin{{ComponentRole: "upper_emitter_resistor", Pin: "2"}, {ComponentRole: "lower_emitter_resistor", Pin: "2"}, {ComponentRole: "upper_current_limit", Pin: "2"}, {ComponentRole: "lower_current_limit", Pin: "2"}, {ComponentRole: "zobel_resistor", Pin: "1"}}},
		{NameTemplate: "zobel_mid", Visibility: "local", Role: "output_damping", Pins: []NetPin{{ComponentRole: "zobel_resistor", Pin: "2"}, {ComponentRole: "zobel_capacitor", Pin: "1"}}},
		{NameTemplate: "power_star", Visibility: "exported", Role: "power_star_return", Pins: []NetPin{{ComponentRole: "zobel_capacitor", Pin: "2"}}},
	}
}

func classABSpeakerPowerStageSchematicHints() []SchematicHint {
	roles := []string{"upper_bias_feed", "bias_diode_1", "bias_diode_2", "bias_diode_3", "bias_diode_4", "lower_bias_feed", "upper_driver_base_stopper", "lower_driver_base_stopper", "upper_driver", "lower_driver", "upper_output", "lower_output", "upper_emitter_resistor", "lower_emitter_resistor", "upper_current_limit", "lower_current_limit", "zobel_resistor", "zobel_capacitor"}
	positions := [][2]float64{{10, -18}, {20, -12}, {20, -6}, {20, 6}, {20, 12}, {10, 18}, {31, -10}, {31, 10}, {40, -10}, {40, 10}, {57, -10}, {57, 10}, {74, -8}, {74, 8}, {63, -2}, {63, 2}, {86, 4}, {86, 14}}
	hints := make([]SchematicHint, 0, len(roles))
	for index, role := range roles {
		hints = append(hints, SchematicHint{Kind: "speaker_power_flow", ComponentRole: role, XMM: positions[index][0], YMM: positions[index][1], Note: "Deterministic speaker-power signal and return ordering."})
	}
	return hints
}

func classABSpeakerPowerStagePCBRealization() *PCBRealization {
	return &PCBRealization{
		Version:           "0.1.0",
		VerificationLevel: PCBVerificationConnectivityVerified,
		Components: []PCBComponentRealization{
			{ComponentRole: "upper_bias_feed", FootprintID: "Resistor_SMD:R_0805_2012Metric", Placement: RelativePlacement{XMM: 2, YMM: -10, Layer: "F.Cu"}},
			{ComponentRole: "bias_diode_1", FootprintID: "Diode_SMD:D_SOD-123", Placement: RelativePlacement{XMM: 8, YMM: -8, Layer: "F.Cu"}},
			{ComponentRole: "bias_diode_2", FootprintID: "Diode_SMD:D_SOD-123", Placement: RelativePlacement{XMM: 8, YMM: -3, Layer: "F.Cu"}},
			{ComponentRole: "bias_diode_3", FootprintID: "Diode_SMD:D_SOD-123", Placement: RelativePlacement{XMM: 8, YMM: 3, Layer: "F.Cu"}},
			{ComponentRole: "bias_diode_4", FootprintID: "Diode_SMD:D_SOD-123", Placement: RelativePlacement{XMM: 8, YMM: 8, Layer: "F.Cu"}},
			{ComponentRole: "lower_bias_feed", FootprintID: "Resistor_SMD:R_0805_2012Metric", Placement: RelativePlacement{XMM: 2, YMM: 10, Layer: "F.Cu"}},
			{ComponentRole: "upper_driver_base_stopper", FootprintID: "Resistor_SMD:R_0805_2012Metric", Placement: RelativePlacement{XMM: 16, YMM: -9, Layer: "F.Cu"}},
			{ComponentRole: "lower_driver_base_stopper", FootprintID: "Resistor_SMD:R_0805_2012Metric", Placement: RelativePlacement{XMM: 16, YMM: 9, Layer: "F.Cu"}},
			{ComponentRole: "upper_driver", FootprintID: "Package_TO_SOT_THT:TO-126-3_Vertical", Placement: RelativePlacement{XMM: 20, YMM: -9, Layer: "F.Cu"}},
			{ComponentRole: "lower_driver", FootprintID: "Package_TO_SOT_THT:TO-126-3_Vertical", Placement: RelativePlacement{XMM: 20, YMM: 9, Layer: "F.Cu"}},
			{ComponentRole: "upper_output", FootprintID: "Package_TO_SOT_THT:TO-3P-3_Vertical", Placement: RelativePlacement{XMM: 34, YMM: -10, Layer: "F.Cu"}},
			{ComponentRole: "lower_output", FootprintID: "Package_TO_SOT_THT:TO-3P-3_Vertical", Placement: RelativePlacement{XMM: 34, YMM: 10, Layer: "F.Cu"}},
			{ComponentRole: "upper_emitter_resistor", FootprintID: "Resistor_THT:R_Axial_DIN0414_L11.9mm_D4.5mm_P20.32mm_Horizontal", Placement: RelativePlacement{XMM: 51, YMM: -7, Layer: "F.Cu"}},
			{ComponentRole: "lower_emitter_resistor", FootprintID: "Resistor_THT:R_Axial_DIN0414_L11.9mm_D4.5mm_P20.32mm_Horizontal", Placement: RelativePlacement{XMM: 51, YMM: 7, Layer: "F.Cu"}},
			{ComponentRole: "upper_current_limit", FootprintID: "Package_TO_SOT_SMD:SOT-23", Placement: RelativePlacement{XMM: 44, YMM: -3, Layer: "F.Cu"}},
			{ComponentRole: "lower_current_limit", FootprintID: "Package_TO_SOT_SMD:SOT-23", Placement: RelativePlacement{XMM: 44, YMM: 3, Layer: "F.Cu"}},
			{ComponentRole: "zobel_resistor", FootprintID: "Resistor_THT:R_Axial_DIN0414_L11.9mm_D4.5mm_P20.32mm_Horizontal", Placement: RelativePlacement{XMM: 70, YMM: 9, RotationDeg: 90, Layer: "F.Cu"}},
			{ComponentRole: "zobel_capacitor", FootprintID: "Capacitor_THT:C_Rect_L7.2mm_W3.0mm_P5.00mm_FKS2_FKP2_MKS2_MKP2", Placement: RelativePlacement{XMM: 70, YMM: 22, Layer: "F.Cu"}},
		},
		EntryAnchors: []PCBEntryAnchor{
			{ID: "driver_out", Port: "DRIVER_OUT", NetTemplate: "driver_center", Placement: RelativePlacement{XMM: -4, YMM: 0, Layer: "F.Cu"}, Description: "Small-signal drive enters away from the output-current loop."},
			{ID: "vcc", Port: "VCC", NetTemplate: "vcc", Placement: RelativePlacement{XMM: 34, YMM: -18, Layer: "F.Cu"}, Description: "Positive high-current rail entry."},
			{ID: "vee", Port: "VEE", NetTemplate: "vee", Placement: RelativePlacement{XMM: 34, YMM: 18, Layer: "F.Cu"}, Description: "Negative high-current rail entry."},
			{ID: "raw_out", Port: "RAW_OUT", NetTemplate: "raw_out", Placement: RelativePlacement{XMM: 64, YMM: 0, Layer: "F.Cu"}, Description: "Emitter-resistor join and Kelvin feedback source."},
			{ID: "power_star", Port: "POWER_STAR", NetTemplate: "power_star", Placement: RelativePlacement{XMM: 70, YMM: 28, Layer: "F.Cu"}, Description: "Zobel and speaker-return star anchor."},
		},
		PlacementGroups: []PCBPlacementGroup{
			{ID: "bias_driver", ComponentRoles: []string{"upper_bias_feed", "bias_diode_1", "bias_diode_2", "bias_diode_3", "bias_diode_4", "lower_bias_feed", "upper_driver_base_stopper", "lower_driver_base_stopper", "upper_driver", "lower_driver"}, AnchorRole: "bias_diode_2", Bounds: &RelativeBounds{MinXMM: 0, MinYMM: -13, MaxXMM: 26, MaxYMM: 13}, Description: "Thermally couple the bias string and terminate each driver base locally."},
			{ID: "driver_output", ComponentRoles: []string{"upper_driver", "lower_driver", "upper_output", "lower_output", "upper_emitter_resistor", "lower_emitter_resistor"}, AnchorRole: "upper_output", Bounds: &RelativeBounds{MinXMM: 16, MinYMM: -15, MaxXMM: 64, MaxYMM: 15}, Description: "Symmetric driver/output group along the shared heatsink edge."},
			{ID: "current_limit", ComponentRoles: []string{"upper_current_limit", "lower_current_limit", "upper_emitter_resistor", "lower_emitter_resistor"}, AnchorRole: "upper_current_limit", Bounds: &RelativeBounds{MinXMM: 40, MinYMM: -9, MaxXMM: 56, MaxYMM: 9}, Description: "Kelvin current-sense devices at the emitter resistors."},
			{ID: "output_damping", ComponentRoles: []string{"zobel_resistor", "zobel_capacitor"}, AnchorRole: "zobel_resistor", Bounds: &RelativeBounds{MinXMM: 64, MinYMM: 3, MaxXMM: 76, MaxYMM: 27}, Description: "Keep the Zobel loop local to raw output and power star."},
		},
		LocalRoutes: classABSpeakerPowerStageRoutes(),
		Keepouts: []PCBKeepout{
			{ID: "shared_heatsink_mounting", Layer: "F.Cu", Bounds: RelativeBounds{MinXMM: 28, MinYMM: -17, MaxXMM: 41, MaxYMM: 17}, PlacementGroupID: "driver_output", AppliesTo: []string{"upper_output", "lower_output"}, BlocksRoute: boolPtr(false), Description: "Shared-heatsink body and mounting-tool access envelope above the PCB; copper may pass beneath it."},
		},
		Constraints: []PCBConstraint{
			{ID: "speaker_star_return", Kind: "return_topology", Category: PCBConstraintReturnTopology, NetTemplate: "power_star", AppliesTo: []string{"zobel_capacitor", "upper_emitter_resistor", "lower_emitter_resistor"}, Description: "Keep signal, Zobel, rail-decoupling, and speaker returns distinct until the declared star."},
			{ID: "speaker_feedback_kelvin", Kind: "kelvin_origin", Category: PCBConstraintFeedbackSense, NetTemplate: "raw_out", AppliesTo: []string{"upper_emitter_resistor", "lower_emitter_resistor"}, MaxLengthMM: 6, Description: "Feedback originates at the load side of the emitter resistors."},
			{ID: "speaker_upper_current_kelvin", Kind: "kelvin_sense", Category: PCBConstraintKelvinSense, NetTemplate: "upper_sense", AppliesTo: []string{"upper_emitter_resistor", "upper_current_limit"}, MaxLengthMM: 8, Description: "Sense the resistor pad independently of the output-current trace."},
			{ID: "speaker_lower_current_kelvin", Kind: "kelvin_sense", Category: PCBConstraintKelvinSense, NetTemplate: "lower_sense", AppliesTo: []string{"lower_emitter_resistor", "lower_current_limit"}, MaxLengthMM: 8, Description: "Sense the resistor pad independently of the output-current trace."},
			{ID: "speaker_output_width", Kind: "route_width", Category: PCBConstraintCurrentPath, NetTemplate: "raw_out", AppliesTo: []string{"upper_emitter_resistor", "lower_emitter_resistor"}, MinWidthMM: 2, ClearanceMM: 0.5, Description: "Carry the bounded 3 A peak speaker current."},
			{ID: "speaker_positive_rail_width", Kind: "route_width", Category: PCBConstraintCurrentPath, NetTemplate: "vcc", MinWidthMM: 2, ClearanceMM: 0.5, Description: "Positive rail high-current path."},
			{ID: "speaker_negative_rail_width", Kind: "route_width", Category: PCBConstraintCurrentPath, NetTemplate: "vee", MinWidthMM: 2, ClearanceMM: 0.5, Description: "Negative rail high-current path."},
			{ID: "speaker_bias_tracking", Kind: "max_spacing", Category: PCBConstraintThermalCoupling, AppliesTo: []string{"bias_diode_1", "bias_diode_2", "bias_diode_3", "bias_diode_4", "upper_output", "lower_output"}, MaxLengthMM: 30, Required: true, Description: "Mount the bias string to the reviewed output thermal region."},
			{ID: "speaker_output_symmetry", Kind: "max_spacing", Category: PCBConstraintDeviceSymmetry, AppliesTo: []string{"upper_driver", "lower_driver", "upper_output", "lower_output", "upper_emitter_resistor", "lower_emitter_resistor"}, MaxLengthMM: 24, Required: true, Description: "Preserve complementary electrical and thermal symmetry."},
			{ID: "speaker_heatsink_keepout", Kind: "mechanical_keepout", Category: PCBConstraintThermalKeepout, AppliesTo: []string{"upper_output", "lower_output"}, MaxLengthMM: 35, Description: "Preserve heatsink edge, mounting access, and component-height envelope."},
			{ID: "speaker_input_separation", Kind: "minimum_separation", Category: PCBConstraintAnalogInputSeparation, NetTemplate: "driver_center", AppliesTo: []string{"zobel_resistor", "upper_emitter_resistor", "lower_emitter_resistor"}, MaxLengthMM: 20, Description: "Keep the small-signal drive outside the output and Zobel current loops."},
		},
		Validation: PCBValidationExpectations{
			RequiredNets:   []string{"vcc", "vee", "upper_drive_source", "upper_drive", "upper_bias_mid", "driver_center", "lower_bias_mid", "lower_drive_source", "lower_drive", "upper_power_base", "lower_power_base", "upper_sense", "lower_sense", "raw_out", "zobel_mid", "power_star"},
			RequiredRoutes: []string{"driver_entry", "vcc_output", "vee_output", "upper_emitter", "lower_emitter", "raw_output_join", "raw_output_port", "zobel_series", "zobel_return"},
			RequiresDRC:    true,
		},
	}
}

func classABSpeakerPowerStageRoutes() []PCBLocalRoute {
	return []PCBLocalRoute{
		{ID: "driver_entry", NetTemplate: "driver_center", From: RouteEndpoint{Port: "DRIVER_OUT"}, To: RouteEndpoint{ComponentRole: "bias_diode_2", Pin: "1"}, Waypoints: []RelativePoint{{XMM: 3, YMM: 0}, {XMM: 8, YMM: 0}}, Layer: "B.Cu", WidthMM: 0.4, Required: true},
		{ID: "driver_center_join", NetTemplate: "driver_center", From: RouteEndpoint{ComponentRole: "bias_diode_2", Pin: "1"}, To: RouteEndpoint{ComponentRole: "bias_diode_3", Pin: "2"}, Layer: "B.Cu", WidthMM: 0.4, Required: true},
		{ID: "upper_bias_diode", NetTemplate: "upper_bias_mid", From: RouteEndpoint{ComponentRole: "bias_diode_1", Pin: "1"}, To: RouteEndpoint{ComponentRole: "bias_diode_2", Pin: "2"}, Layer: "F.Cu", WidthMM: 0.4, Required: true},
		{ID: "lower_bias_diode", NetTemplate: "lower_bias_mid", From: RouteEndpoint{ComponentRole: "bias_diode_3", Pin: "1"}, To: RouteEndpoint{ComponentRole: "bias_diode_4", Pin: "2"}, Layer: "F.Cu", WidthMM: 0.4, Required: true},
		{ID: "upper_drive_feed", NetTemplate: "upper_drive_source", From: RouteEndpoint{ComponentRole: "upper_bias_feed", Pin: "2"}, To: RouteEndpoint{ComponentRole: "bias_diode_1", Pin: "2"}, Layer: "B.Cu", WidthMM: 0.4, Required: true},
		{ID: "upper_drive_stopper", NetTemplate: "upper_drive_source", From: RouteEndpoint{ComponentRole: "bias_diode_1", Pin: "2"}, To: RouteEndpoint{ComponentRole: "upper_driver_base_stopper", Pin: "1"}, Layer: "B.Cu", WidthMM: 0.4, Required: true},
		{ID: "upper_drive_base", NetTemplate: "upper_drive", From: RouteEndpoint{ComponentRole: "upper_driver_base_stopper", Pin: "2"}, To: RouteEndpoint{ComponentRole: "upper_driver", Pin: "3"}, Layer: "F.Cu", WidthMM: 0.5, Required: true},
		{ID: "upper_limit_pull", NetTemplate: "upper_drive", From: RouteEndpoint{ComponentRole: "upper_driver", Pin: "3"}, To: RouteEndpoint{ComponentRole: "upper_current_limit", Pin: "3"}, Layer: "B.Cu", WidthMM: 0.4, Required: true},
		{ID: "lower_drive_feed", NetTemplate: "lower_drive_source", From: RouteEndpoint{ComponentRole: "lower_bias_feed", Pin: "1"}, To: RouteEndpoint{ComponentRole: "bias_diode_4", Pin: "1"}, Layer: "B.Cu", WidthMM: 0.4, Required: true},
		{ID: "lower_drive_stopper", NetTemplate: "lower_drive_source", From: RouteEndpoint{ComponentRole: "bias_diode_4", Pin: "1"}, To: RouteEndpoint{ComponentRole: "lower_driver_base_stopper", Pin: "1"}, Layer: "B.Cu", WidthMM: 0.4, Required: true},
		{ID: "lower_drive_base", NetTemplate: "lower_drive", From: RouteEndpoint{ComponentRole: "lower_driver_base_stopper", Pin: "2"}, To: RouteEndpoint{ComponentRole: "lower_driver", Pin: "3"}, Layer: "F.Cu", WidthMM: 0.5, Required: true},
		{ID: "lower_limit_pull", NetTemplate: "lower_drive", From: RouteEndpoint{ComponentRole: "lower_driver", Pin: "3"}, To: RouteEndpoint{ComponentRole: "lower_current_limit", Pin: "3"}, Layer: "B.Cu", WidthMM: 0.4, Required: true},
		{ID: "upper_driver_output", NetTemplate: "upper_power_base", From: RouteEndpoint{ComponentRole: "upper_driver", Pin: "1"}, To: RouteEndpoint{ComponentRole: "upper_output", Pin: "1"}, Layer: "F.Cu", WidthMM: 0.6, Required: true},
		{ID: "lower_driver_output", NetTemplate: "lower_power_base", From: RouteEndpoint{ComponentRole: "lower_driver", Pin: "1"}, To: RouteEndpoint{ComponentRole: "lower_output", Pin: "1"}, Layer: "F.Cu", WidthMM: 0.6, Required: true},
		{ID: "upper_emitter", NetTemplate: "upper_sense", From: RouteEndpoint{ComponentRole: "upper_output", Pin: "3"}, To: RouteEndpoint{ComponentRole: "upper_emitter_resistor", Pin: "1"}, Layer: "F.Cu", WidthMM: 2, Required: true},
		{ID: "upper_sense_kelvin", NetTemplate: "upper_sense", From: RouteEndpoint{ComponentRole: "upper_emitter_resistor", Pin: "1"}, To: RouteEndpoint{ComponentRole: "upper_current_limit", Pin: "1"}, Layer: "B.Cu", WidthMM: 0.35, Required: true},
		{ID: "lower_emitter", NetTemplate: "lower_sense", From: RouteEndpoint{ComponentRole: "lower_output", Pin: "3"}, To: RouteEndpoint{ComponentRole: "lower_emitter_resistor", Pin: "1"}, Layer: "F.Cu", WidthMM: 2, Required: true},
		{ID: "lower_sense_kelvin", NetTemplate: "lower_sense", From: RouteEndpoint{ComponentRole: "lower_emitter_resistor", Pin: "1"}, To: RouteEndpoint{ComponentRole: "lower_current_limit", Pin: "1"}, Layer: "B.Cu", WidthMM: 0.35, Required: true},
		{ID: "raw_output_join", NetTemplate: "raw_out", From: RouteEndpoint{ComponentRole: "upper_emitter_resistor", Pin: "2"}, To: RouteEndpoint{ComponentRole: "lower_emitter_resistor", Pin: "2"}, Waypoints: []RelativePoint{{XMM: 63, YMM: -7}, {XMM: 63, YMM: 7}}, Layer: "F.Cu", WidthMM: 2, Required: true},
		{ID: "upper_limit_return", NetTemplate: "raw_out", From: RouteEndpoint{ComponentRole: "upper_current_limit", Pin: "2"}, To: RouteEndpoint{ComponentRole: "upper_emitter_resistor", Pin: "2"}, Layer: "B.Cu", WidthMM: 0.35, Required: true},
		{ID: "lower_limit_return", NetTemplate: "raw_out", From: RouteEndpoint{ComponentRole: "lower_current_limit", Pin: "2"}, To: RouteEndpoint{ComponentRole: "lower_emitter_resistor", Pin: "2"}, Layer: "B.Cu", WidthMM: 0.35, Required: true},
		{ID: "raw_output_zobel", NetTemplate: "raw_out", From: RouteEndpoint{ComponentRole: "upper_emitter_resistor", Pin: "2"}, To: RouteEndpoint{ComponentRole: "zobel_resistor", Pin: "1"}, Layer: "F.Cu", WidthMM: 1, Required: true},
		{ID: "raw_output_port", NetTemplate: "raw_out", From: RouteEndpoint{ComponentRole: "upper_emitter_resistor", Pin: "2"}, To: RouteEndpoint{Port: "RAW_OUT"}, Layer: "F.Cu", WidthMM: 2, Required: true, DisableEntryAnchorVia: true},
		{ID: "zobel_series", NetTemplate: "zobel_mid", From: RouteEndpoint{ComponentRole: "zobel_resistor", Pin: "2"}, To: RouteEndpoint{ComponentRole: "zobel_capacitor", Pin: "1"}, Layer: "F.Cu", WidthMM: 0.8, Required: true},
		{ID: "zobel_return", NetTemplate: "power_star", From: RouteEndpoint{ComponentRole: "zobel_capacitor", Pin: "2"}, To: RouteEndpoint{Port: "POWER_STAR"}, Layer: "F.Cu", WidthMM: 1, Required: true, DisableEntryAnchorVia: true},
		{ID: "vcc_output", NetTemplate: "vcc", From: RouteEndpoint{Port: "VCC"}, To: RouteEndpoint{ComponentRole: "upper_output", Pin: "2"}, Layer: "F.Cu", WidthMM: 2, Required: true, DisableEntryAnchorVia: true},
		{ID: "vcc_driver", NetTemplate: "vcc", From: RouteEndpoint{ComponentRole: "upper_output", Pin: "2"}, To: RouteEndpoint{ComponentRole: "upper_driver", Pin: "2"}, Layer: "F.Cu", WidthMM: 1, Required: true},
		{ID: "vcc_bias", NetTemplate: "vcc", From: RouteEndpoint{ComponentRole: "upper_driver", Pin: "2"}, To: RouteEndpoint{ComponentRole: "upper_bias_feed", Pin: "1"}, Layer: "B.Cu", WidthMM: 0.5, Required: true},
		{ID: "vee_output", NetTemplate: "vee", From: RouteEndpoint{Port: "VEE"}, To: RouteEndpoint{ComponentRole: "lower_output", Pin: "2"}, Layer: "F.Cu", WidthMM: 2, Required: true, DisableEntryAnchorVia: true},
		{ID: "vee_driver", NetTemplate: "vee", From: RouteEndpoint{ComponentRole: "lower_output", Pin: "2"}, To: RouteEndpoint{ComponentRole: "lower_driver", Pin: "2"}, Layer: "F.Cu", WidthMM: 1, Required: true},
		{ID: "vee_bias", NetTemplate: "vee", From: RouteEndpoint{ComponentRole: "lower_driver", Pin: "2"}, To: RouteEndpoint{ComponentRole: "lower_bias_feed", Pin: "2"}, Layer: "B.Cu", WidthMM: 0.5, Required: true},
	}
}

func instantiateClassABSpeakerPowerStage(definition BlockDefinition, request BlockRequest, params map[string]any, issues []reports.Issue) BlockOutput {
	params = ApplyParameterDefaults(definition, params)
	if hasBlockingIssues(issues) {
		output := dryRunBlockOutput(definition, request, nil, issues)
		output.Instance.Params = params
		return output
	}
	supply, supplyOK := parseUnit(params["supply_voltage"], "V", voltageMultipliers())
	targetLoad, loadOK := parseUnit(params["target_load"], "Ω", resistanceMultipliers())
	minimumLoad, minimumLoadOK := parseUnit(params["minimum_load"], "Ω", resistanceMultipliers())
	loss, lossOK := parseUnit(params["output_stage_loss"], "V", voltageMultipliers())
	currentLimit, currentOK := parseUnit(params["current_limit"], "A", currentMultipliers())
	currentLimitVBE, currentLimitVBEOK := parseUnit(params["current_limit_vbe"], "V", voltageMultipliers())
	currentLimitVBETolerance, currentLimitVBEToleranceOK := parseUnit(params["current_limit_vbe_tolerance"], "V", voltageMultipliers())
	emitterResistance, emitterOK := parseUnit(params["emitter_resistor_value"], "Ω", resistanceMultipliers())
	targetPower, targetPowerOK := numericValue(params["target_power"])
	if !supplyOK || supply <= 0 {
		issues = append(issues, blockIssue("params.supply_voltage", "supply_voltage must be a positive total rail voltage"))
	}
	if !loadOK || targetLoad < 2 || targetLoad > 32 || !minimumLoadOK || minimumLoad < 2 || minimumLoad > targetLoad {
		issues = append(issues, blockIssue("params.target_load", "target and minimum loads must define a bounded 2-32 ohm speaker envelope"))
	}
	if !lossOK || loss < 0 || !currentOK || currentLimit <= 0 || !currentLimitVBEOK || currentLimitVBE < 0.55 || currentLimitVBE > 0.85 || !currentLimitVBEToleranceOK || currentLimitVBETolerance <= 0 || currentLimitVBETolerance > 0.2 || !emitterOK || emitterResistance <= 0 || !targetPowerOK || targetPower <= 0 {
		issues = append(issues, blockIssue("params.output_contract", "output loss, current limit, emitter resistance, and target power must be finite and positive"))
	}
	if currentOK && currentLimitVBEOK && currentLimitVBEToleranceOK && emitterOK && math.Abs(currentLimit*emitterResistance-currentLimitVBE) > currentLimitVBETolerance {
		issues = append(issues, blockIssue("params.current_limit", "emitter resistance and nominal room-temperature limiter-transistor VBE do not establish the declared current limit within its explicit thermal/model tolerance"))
	}
	if supplyOK && loadOK && lossOK && currentOK && targetPowerOK && targetLoad > 0 {
		peakVoltage := math.Min(supply/2-loss, currentLimit*targetLoad)
		availablePower := peakVoltage * peakVoltage / (2 * targetLoad)
		params["available_peak_output_v"] = peakVoltage
		params["available_output_power_w"] = availablePower
		params["sense_threshold_v"] = currentLimitVBE
		if peakVoltage <= 0 || availablePower < targetPower {
			issues = append(issues, blockIssue("params.target_power", "rail and current-limit envelope does not deliver the requested RMS speaker power"))
		}
	}
	for _, name := range []string{"upper_driver_component_id", "lower_driver_component_id", "upper_output_component_id", "lower_output_component_id"} {
		if strings.TrimSpace(stringParam(params, name)) == "" {
			issues = append(issues, blockIssue("params."+name, "concrete complementary component ID is required"))
		}
	}
	if hasBlockingIssues(issues) {
		output := dryRunBlockOutput(definition, request, nil, issues)
		output.Instance.Params = params
		return output
	}

	allocator := NewInstanceReferenceAllocator(request.InstanceID)
	refsByRole := map[string]string{}
	var refs []string
	var operations []transactions.Operation
	hints := map[string]SchematicHint{}
	for _, hint := range definition.SchematicHints {
		hints[hint.ComponentRole] = hint
	}
	for _, component := range definition.Components {
		component = speakerStageResolvedComponent(component, params)
		ref := allocator.Next(component.RefPrefix)
		refsByRole[component.Role] = ref
		refs = append(refs, ref)
		hint := hints[component.Role]
		componentOperations, componentIssues := ComponentOperations(component, ref, transactions.Point{XMM: hint.XMM, YMM: hint.YMM})
		operations = append(operations, componentOperations...)
		issues = append(issues, componentIssues...)
	}
	nets := make([]string, 0, len(definition.Nets))
	for _, net := range definition.Nets {
		netName := InstanceNetName(request.InstanceID, net.NameTemplate)
		nets = append(nets, netName)
		for index := 1; index < len(net.Pins); index++ {
			from, to := net.Pins[index-1], net.Pins[index]
			appendRoleConnectOperation(&operations, &issues, refsByRole, from, to, netName)
		}
	}
	appendConnectOperation(&operations, &issues, request.InstanceID, "DRIVER_OUT", refsByRole["bias_diode_2"], "1", InstanceNetName(request.InstanceID, "driver_center"))
	appendConnectOperation(&operations, &issues, request.InstanceID, "VCC", refsByRole["upper_output"], "2", InstanceNetName(request.InstanceID, "vcc"))
	appendConnectOperation(&operations, &issues, request.InstanceID, "VEE", refsByRole["lower_output"], "2", InstanceNetName(request.InstanceID, "vee"))
	appendConnectOperation(&operations, &issues, refsByRole["upper_emitter_resistor"], "2", request.InstanceID, "RAW_OUT", InstanceNetName(request.InstanceID, "raw_out"))
	appendConnectOperation(&operations, &issues, refsByRole["zobel_capacitor"], "2", request.InstanceID, "POWER_STAR", InstanceNetName(request.InstanceID, "power_star"))
	output := dryRunBlockOutput(definition, request, operations, issues)
	output.Instance.Params = params
	output.Instance.Refs = refs
	output.Instance.Nets = nets
	return output
}

func speakerStageResolvedComponent(component BlockComponent, params map[string]any) BlockComponent {
	if component.ComponentIDParam != "" {
		component.ComponentID = stringParam(params, component.ComponentIDParam)
	}
	if component.Role == "upper_emitter_resistor" || component.Role == "lower_emitter_resistor" {
		component.Value = stringParam(params, "emitter_resistor_value")
	}
	if component.Role == "upper_bias_feed" || component.Role == "lower_bias_feed" {
		component.Value = stringParam(params, "bias_feed_resistor_value")
	}
	return component
}

func speakerDriverECBPins() []transactions.PinSpec {
	return []transactions.PinSpec{{Number: "1", XMM: 2.54, YMM: 1.27}, {Number: "2", XMM: 0, YMM: -2.54}, {Number: "3", XMM: -2.54, YMM: 0}}
}

func speakerOutputBCEPins() []transactions.PinSpec {
	return []transactions.PinSpec{{Number: "1", XMM: -2.54, YMM: 0}, {Number: "2", XMM: 0, YMM: -2.54}, {Number: "3", XMM: 2.54, YMM: 1.27}}
}
