package blocks

import (
	"strings"

	"kicadai/internal/components"
	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

const classABOutputStageID = "class_ab_output_stage"

func classABOutputStageDefinition() BlockDefinition {
	return BlockDefinition{
		ID:          classABOutputStageID,
		Name:        "Class AB Output Stage",
		Description: "Catalog-selected complementary BJT emitter follower output stage for bounded headphone and reviewed power-amplifier loads.",
		Version:     "0.1.0",
		Category:    "analog",
		Parameters: []BlockParameter{
			{Name: "topology", Type: ParameterEnum, Default: "diode_string", Allowed: []any{"diode_string", "vbe_multiplier"}, Description: "Bias topology."},
			{Name: "application", Type: ParameterEnum, Default: "headphone", Allowed: []any{"headphone", "power"}, Description: "Output-load application used by catalog pair selection and validation."},
			{Name: "supply_voltage", Type: ParameterVoltage, Default: "9V", Description: "Rail-to-rail supply voltage used for initial output-device screening."},
			{Name: "load_impedance", Type: ParameterResistance, Default: "32Ω", Description: "Nominal headphone load impedance."},
			{Name: "upper_output_component_id", Type: ParameterString, Description: "Optional explicit NPN override; normal synthesis selects a catalog pair from the operating envelope."},
			{Name: "lower_output_component_id", Type: ParameterString, Description: "Optional explicit PNP override; normal synthesis selects a catalog pair from the operating envelope."},
			{Name: "emitter_resistor_value", Type: ParameterResistance, Default: "0.47Ω", Description: "Small emitter resistor used to stabilize quiescent current."},
			{Name: "emitter_resistor_footprint", Type: ParameterFootprintID, Default: "Resistor_SMD:R_1206_3216Metric", Description: "Footprint for output emitter resistors."},
			{Name: "bias_feed_resistor_value", Type: ParameterResistance, Default: "10kΩ", Description: "Resistor that provides bias-string current from the rails."},
			{Name: "target_quiescent_current", Type: ParameterCurrent, Default: "5mA", Description: "Target output-pair idle current used to calculate VBE-multiplier bias voltage."},
			{Name: "output_vbe_voltage", Type: ParameterVoltage, Default: "0.65V", Description: "Conservative output-device VBE assumption at the target idle current."},
			{Name: "bias_multiplier_vbe_voltage", Type: ParameterVoltage, Default: "0.65V", Description: "Conservative VBE of the small-signal multiplier transistor at its bias-network current, independent of the power-device VBE assumption."},
			{Name: "bias_multiplier_lower_resistor_value", Type: ParameterResistance, Default: "1kΩ", Description: "Lower VBE-multiplier resistor; the upper resistor is calculated from idle current and emitter resistance."},
			{Name: "bias_multiplier_component_id", Type: ParameterString, Default: "bjt.onsemi.mmbt3904.sot23", Description: "Catalog-backed thermally coupled NPN VBE-multiplier device."},
			{Name: "bias_multiplier_footprint", Type: ParameterFootprintID, Default: "Package_TO_SOT_SMD:SOT-23", Description: "Footprint for the VBE-multiplier transistor."},
			{Name: "bias_feed_resistor_footprint", Type: ParameterFootprintID, Default: "Resistor_SMD:R_0805_2012Metric", Description: "Footprint for bias feed resistors."},
			{Name: "bias_diode_footprint", Type: ParameterFootprintID, Default: "Diode_SMD:D_SOD-123", Description: "Footprint for each bias diode."},
			{Name: "output_footprint", Type: ParameterFootprintID, Default: "Package_TO_SOT_SMD:SOT-23", Description: "Footprint for the complementary output BJTs."},
		},
		Ports: []BlockPort{
			{Name: "DRIVER_OUT", Direction: PortInput, Description: "Small-signal driver output feeding the bias string."},
			{Name: "VCC", Direction: PortPower, Voltage: "supply_voltage", Description: "Positive output-device collector rail."},
			{Name: "VEE", Direction: PortPower, Description: "Negative rail or ground return for single-supply headphone stages."},
			{Name: "AMP_OUT", Direction: PortOutput, Description: "Emitter-follower output node."},
			{Name: "LOAD_REF", Direction: PortPassive, Description: "Load return or virtual ground reference."},
		},
		RequiredLibraries: []LibraryRequirement{
			{Kind: "symbol", ID: "Device:D", Required: true, Description: "Bias diode symbol."},
			{Kind: "symbol", ID: "Device:Q_NPN_BEC", Required: true, Description: "NPN output transistor symbol."},
			{Kind: "symbol", ID: "Device:Q_PNP_BEC", Required: true, Description: "PNP output transistor symbol."},
			{Kind: "symbol", ID: "Device:R", Required: true, Description: "Emitter degeneration resistor symbol."},
			{Kind: "symbol", ID: "Connector:TestPoint", Required: true, Description: "Load-reference anchor symbol."},
			{Kind: "footprint", ID: "Diode_SMD:D_SOD-123", Required: true, Description: "Default bias diode footprint."},
			{Kind: "footprint", ID: "Resistor_SMD:R_1206_3216Metric", Required: true, Description: "Default emitter resistor footprint."},
			{Kind: "footprint", ID: "Package_TO_SOT_SMD:SOT-23", Required: true, Description: "Default small-signal output BJT footprint."},
			{Kind: "footprint", ID: "TestPoint:TestPoint_Pad_D1.0mm", Required: true, Description: "Default load-reference test point footprint."},
		},
		Components:     classABOutputStageComponents(),
		PCBRealization: classABOutputStagePCBRealization(),
		Nets: []BlockNet{
			{NameTemplate: "upper_drive", Visibility: "local", Role: "upper_output_drive", Pins: []NetPin{{ComponentRole: "upper_bias_feed", Pin: "2"}, {ComponentRole: "bias_upper", Pin: "1"}, {ComponentRole: "upper_output", Pin: "1"}}},
			{NameTemplate: "driver_out", Visibility: "exported", Role: "driver_output", Pins: []NetPin{{ComponentRole: "bias_upper", Pin: "2"}, {ComponentRole: "bias_lower", Pin: "1"}}},
			{NameTemplate: "lower_drive", Visibility: "exported", Role: "lower_output_drive", Pins: []NetPin{{ComponentRole: "bias_lower", Pin: "2"}, {ComponentRole: "lower_output", Pin: "1"}, {ComponentRole: "lower_bias_feed", Pin: "1"}}},
			{NameTemplate: "upper_emitter", Visibility: "local", Role: "upper_emitter_resistor_input", Pins: []NetPin{{ComponentRole: "upper_output", Pin: "2"}, {ComponentRole: "upper_emitter_resistor", Pin: "1"}}},
			{NameTemplate: "lower_emitter", Visibility: "local", Role: "lower_emitter_resistor_input", Pins: []NetPin{{ComponentRole: "lower_output", Pin: "2"}, {ComponentRole: "lower_emitter_resistor", Pin: "1"}}},
			{NameTemplate: "amp_out", Visibility: "exported", Role: "amplifier_output", Pins: []NetPin{{ComponentRole: "upper_emitter_resistor", Pin: "2"}, {ComponentRole: "lower_emitter_resistor", Pin: "2"}}},
			{NameTemplate: "vcc", Visibility: "exported", Role: "positive_rail", Pins: []NetPin{{ComponentRole: "upper_output", Pin: "3"}, {ComponentRole: "upper_bias_feed", Pin: "1"}}},
			{NameTemplate: "vee", Visibility: "exported", Role: "negative_rail", Pins: []NetPin{{ComponentRole: "lower_output", Pin: "3"}, {ComponentRole: "lower_bias_feed", Pin: "2"}}},
			{NameTemplate: "load_ref", Visibility: "exported", Role: "load_reference", Pins: []NetPin{{ComponentRole: "load_reference", Pin: "1"}}},
		},
		SchematicHints: []SchematicHint{
			{Kind: "signal_flow", ComponentRole: "upper_bias_feed", XMM: 8, YMM: -7.62, Note: "Feed the upper bias node from the positive rail."},
			{Kind: "signal_flow", ComponentRole: "bias_upper", XMM: 20, YMM: -2.54, Note: "Keep driver and bias string left of the output pair."},
			{Kind: "signal_flow", ComponentRole: "bias_lower", XMM: 20, YMM: 7.62, Note: "Keep lower-bias path below the upper-bias path."},
			{Kind: "signal_flow", ComponentRole: "lower_bias_feed", XMM: 8, YMM: 12.7, Note: "Feed the lower bias node from the negative rail."},
			{Kind: "output_pair", ComponentRole: "upper_output", XMM: 45, YMM: 0, Note: "Place NPN device above the output node."},
			{Kind: "output_pair", ComponentRole: "lower_output", XMM: 45, YMM: 5.08, Note: "Place PNP device below the output node."},
			{Kind: "output_pair", ComponentRole: "upper_emitter_resistor", XMM: 56, YMM: 0, Note: "Keep the upper emitter resistor close to the output transistor emitter."},
			{Kind: "output_pair", ComponentRole: "lower_emitter_resistor", XMM: 56, YMM: 5.08, Note: "Keep the lower emitter resistor close to the output transistor emitter."},
			{Kind: "load_reference", ComponentRole: "load_reference", XMM: 65, YMM: 12, Note: "Keep load return separate from the negative rail unless the parent design intentionally ties them."},
		},
		ValidationRules: []BlockValidationRule{
			{ID: "class_ab.topology.bias_contract", Severity: BlockValidationSeverityBlocked, Description: "Bias topology must provide a deterministic diode-string or calculated VBE-multiplier quiescent-current contract."},
			{ID: "class_ab.supply_voltage.positive", Severity: BlockValidationSeverityBlocked, Description: "Supply voltage must be a positive voltage literal."},
			{ID: "class_ab.load.headphone_class", Severity: BlockValidationSeverityBlocked, Description: "Initial realization is limited to headphone loads with peak current below verified output-device ratings."},
			{ID: "class_ab.output_pair.catalog_selection", Severity: BlockValidationSeverityBlocked, Description: "Output devices are selected together from catalog evidence for the requested supply, load, application, and acceptance level."},
			{ID: "class_ab.output_pair.evidence", Severity: BlockValidationSeverityBlocked, Description: "Complementary output devices must have verified polarity, role, pinmap, and SOA evidence."},
			{ID: "class_ab.emitter_resistors.required", Severity: BlockValidationSeverityBlocked, Description: "Emitter resistors must be present and positive for the diode-biased Class AB stage."},
			{ID: "class_ab.thermal.review", Severity: BlockValidationSeverityBlocked, Description: "Power amplifier and speaker-load variants require thermal and PCB-current evidence before fabrication."},
		},
		Verification: VerificationRecord{
			Level: VerificationStructural,
			Evidence: []string{
				"component:bjt.onsemi.mmbt3904.sot23",
				"component:bjt.onsemi.mmbt3906.sot23",
				"amplifier.output_pair:headphone_class",
			},
			Notes: []string{
				"Structural realization emits either a diode-string or calculated VBE-multiplier complementary pair for reviewable headphone-class schematics.",
				"Thermal design, quiescent-current trim, stability compensation, and high-current PCB evidence are still blocked for fabrication claims.",
			},
		},
	}
}

func classABOutputStagePCBRealization() *PCBRealization {
	diodeString := RealizationWhen{Params: map[string]any{"topology": "diode_string"}}
	vbeMultiplier := RealizationWhen{Params: map[string]any{"topology": "vbe_multiplier"}}
	return &PCBRealization{
		Version:           "0.1.0",
		VerificationLevel: PCBVerificationUnrealized,
		Components: []PCBComponentRealization{
			{ComponentRole: "bias_upper", FootprintParam: "bias_diode_footprint", Placement: RelativePlacement{XMM: 6, YMM: -3, Layer: "F.Cu"}, When: diodeString},
			{ComponentRole: "bias_lower", FootprintParam: "bias_diode_footprint", Placement: RelativePlacement{XMM: 6, YMM: 3, Layer: "F.Cu"}, When: diodeString},
			{ComponentRole: "bias_multiplier", FootprintParam: "bias_multiplier_footprint", Placement: RelativePlacement{XMM: 6, YMM: 0, Layer: "F.Cu"}, When: vbeMultiplier},
			{ComponentRole: "bias_multiplier_upper", FootprintParam: "bias_feed_resistor_footprint", Placement: RelativePlacement{XMM: 3, YMM: -3, Layer: "F.Cu"}, When: vbeMultiplier},
			{ComponentRole: "bias_multiplier_lower", FootprintParam: "bias_feed_resistor_footprint", Placement: RelativePlacement{XMM: 3, YMM: 3, Layer: "F.Cu"}, When: vbeMultiplier},
			{ComponentRole: "upper_output", FootprintParam: "output_footprint", Placement: RelativePlacement{XMM: 11, YMM: -4, Layer: "F.Cu"}},
			{ComponentRole: "lower_output", FootprintParam: "output_footprint", Placement: RelativePlacement{XMM: 11, YMM: 4, Layer: "F.Cu"}},
			{ComponentRole: "upper_emitter_resistor", FootprintParam: "emitter_resistor_footprint", Placement: RelativePlacement{XMM: 17, YMM: -3, Layer: "F.Cu"}},
			{ComponentRole: "lower_emitter_resistor", FootprintParam: "emitter_resistor_footprint", Placement: RelativePlacement{XMM: 17, YMM: 3, Layer: "F.Cu"}},
			{ComponentRole: "upper_bias_feed", FootprintParam: "bias_feed_resistor_footprint", Placement: RelativePlacement{XMM: 2, YMM: -3, Layer: "F.Cu"}},
			{ComponentRole: "lower_bias_feed", FootprintParam: "bias_feed_resistor_footprint", Placement: RelativePlacement{XMM: 2, YMM: 3, Layer: "F.Cu"}},
			{ComponentRole: "load_reference", FootprintID: "TestPoint:TestPoint_Pad_D1.0mm", Placement: RelativePlacement{XMM: 20, YMM: 9, Layer: "F.Cu"}},
		},
		EntryAnchors: []PCBEntryAnchor{
			{ID: "driver", Port: "DRIVER_OUT", NetTemplate: "driver_out", Placement: RelativePlacement{XMM: -3, YMM: 0, Layer: "F.Cu"}, Description: "Small-signal driver entry kept left of the bias network."},
			{ID: "vcc", Port: "VCC", NetTemplate: "vcc", Placement: RelativePlacement{XMM: 11, YMM: -7, Layer: "F.Cu"}, Description: "Positive-rail entry above the upper output device."},
			{ID: "vee", Port: "VEE", NetTemplate: "vee", Placement: RelativePlacement{XMM: 11, YMM: 9, Layer: "F.Cu"}, Description: "Negative-rail entry below the lower output device."},
			{ID: "amp_out", Port: "AMP_OUT", NetTemplate: "amp_out", Placement: RelativePlacement{XMM: 18.2, YMM: -3, Layer: "F.Cu"}, Description: "Shared output-current node at the upper emitter-resistor output pad."},
			{ID: "load_ref", Port: "LOAD_REF", NetTemplate: "load_ref", Placement: RelativePlacement{XMM: 20, YMM: 9, Layer: "F.Cu"}, Description: "Independent load-return entry at its physical test point."},
		},
		PlacementGroups: []PCBPlacementGroup{
			{ID: "bias_string", ComponentRoles: []string{"upper_bias_feed", "bias_upper", "bias_lower", "bias_multiplier", "bias_multiplier_upper", "bias_multiplier_lower", "lower_bias_feed"}, AnchorRole: "upper_bias_feed", Bounds: &RelativeBounds{MinXMM: 0, MinYMM: -5, MaxXMM: 9, MaxYMM: 5}, TranslateAsUnit: true, Description: "Keep the selected bias network thermally adjacent to the output pair."},
			{ID: "output_pair", ComponentRoles: []string{"upper_output", "lower_output", "upper_emitter_resistor", "lower_emitter_resistor"}, AnchorRole: "upper_output", Bounds: &RelativeBounds{MinXMM: 8, MinYMM: -7, MaxXMM: 21, MaxYMM: 7}, Description: "Keep complementary output devices and emitter resistors symmetric around the amplifier output node."},
		},
		LocalRoutes: []PCBLocalRoute{
			{ID: "diode_upper_feed", NetTemplate: "upper_drive", From: RouteEndpoint{ComponentRole: "upper_bias_feed", Pin: "2"}, To: RouteEndpoint{ComponentRole: "bias_upper", Pin: "1"}, Layer: "B.Cu", WidthMM: 0.25, Required: true, When: diodeString},
			{ID: "diode_upper_output_drive", NetTemplate: "upper_drive", From: RouteEndpoint{ComponentRole: "bias_upper", Pin: "1"}, To: RouteEndpoint{ComponentRole: "upper_output", Pin: "1"}, Waypoints: []RelativePoint{{XMM: 4.8, YMM: -5.8}, {XMM: 10.05, YMM: -5.8}}, Layer: "B.Cu", WidthMM: 0.25, Required: true, When: diodeString},
			{ID: "diode_driver_join", NetTemplate: "driver_out", From: RouteEndpoint{ComponentRole: "bias_upper", Pin: "2"}, To: RouteEndpoint{ComponentRole: "bias_lower", Pin: "1"}, Waypoints: []RelativePoint{{XMM: 8, YMM: -3}, {XMM: 8, YMM: 0}, {XMM: 3.5, YMM: 0}, {XMM: 3.5, YMM: 3}}, Layer: "B.Cu", WidthMM: 0.25, Required: true, When: diodeString},
			{ID: "diode_lower_output_drive", NetTemplate: "lower_drive", From: RouteEndpoint{ComponentRole: "bias_lower", Pin: "2"}, To: RouteEndpoint{ComponentRole: "lower_output", Pin: "1"}, Waypoints: []RelativePoint{{XMM: 7.2, YMM: 5.8}, {XMM: 10.05, YMM: 5.8}}, Layer: "B.Cu", WidthMM: 0.25, Required: true, When: diodeString},
			{ID: "diode_lower_feed", NetTemplate: "lower_drive", From: RouteEndpoint{ComponentRole: "bias_lower", Pin: "2"}, To: RouteEndpoint{ComponentRole: "lower_bias_feed", Pin: "1"}, Waypoints: []RelativePoint{{XMM: 7.2, YMM: 5.8}, {XMM: 1.4, YMM: 5.8}}, Layer: "B.Cu", WidthMM: 0.25, Required: true, When: diodeString},
			{ID: "multiplier_upper_feed", NetTemplate: "upper_drive", From: RouteEndpoint{ComponentRole: "upper_bias_feed", Pin: "2"}, To: RouteEndpoint{ComponentRole: "bias_multiplier", Pin: "3"}, Layer: "F.Cu", WidthMM: 0.25, Required: true, When: vbeMultiplier},
			{ID: "multiplier_upper_output_drive", NetTemplate: "upper_drive", From: RouteEndpoint{ComponentRole: "bias_multiplier", Pin: "3"}, To: RouteEndpoint{ComponentRole: "upper_output", Pin: "1"}, Layer: "F.Cu", WidthMM: 0.25, Required: true, When: vbeMultiplier},
			{ID: "multiplier_upper_resistor", NetTemplate: "upper_drive", From: RouteEndpoint{ComponentRole: "bias_multiplier", Pin: "3"}, To: RouteEndpoint{ComponentRole: "bias_multiplier_upper", Pin: "1"}, Layer: "F.Cu", WidthMM: 0.25, Required: true, When: vbeMultiplier},
			{ID: "multiplier_upper_resistor_driver", NetTemplate: "driver_out", From: RouteEndpoint{ComponentRole: "bias_multiplier_upper", Pin: "2"}, To: RouteEndpoint{ComponentRole: "bias_multiplier", Pin: "1"}, Layer: "F.Cu", WidthMM: 0.25, Required: true, When: vbeMultiplier},
			{ID: "multiplier_lower_resistor_driver", NetTemplate: "driver_out", From: RouteEndpoint{ComponentRole: "bias_multiplier_lower", Pin: "1"}, To: RouteEndpoint{ComponentRole: "bias_multiplier", Pin: "1"}, Layer: "F.Cu", WidthMM: 0.25, Required: true, When: vbeMultiplier},
			{ID: "multiplier_lower_output_drive", NetTemplate: "lower_drive", From: RouteEndpoint{ComponentRole: "bias_multiplier", Pin: "2"}, To: RouteEndpoint{ComponentRole: "lower_output", Pin: "1"}, Layer: "F.Cu", WidthMM: 0.25, Required: true, When: vbeMultiplier},
			{ID: "multiplier_lower_resistor", NetTemplate: "lower_drive", From: RouteEndpoint{ComponentRole: "bias_multiplier", Pin: "2"}, To: RouteEndpoint{ComponentRole: "bias_multiplier_lower", Pin: "2"}, Layer: "F.Cu", WidthMM: 0.25, Required: true, When: vbeMultiplier},
			{ID: "multiplier_lower_feed", NetTemplate: "lower_drive", From: RouteEndpoint{ComponentRole: "bias_multiplier", Pin: "2"}, To: RouteEndpoint{ComponentRole: "lower_bias_feed", Pin: "1"}, Layer: "F.Cu", WidthMM: 0.25, Required: true, When: vbeMultiplier},
			{ID: "upper_emitter", NetTemplate: "upper_emitter", From: RouteEndpoint{ComponentRole: "upper_output", Pin: "2"}, To: RouteEndpoint{ComponentRole: "upper_emitter_resistor", Pin: "1"}, Waypoints: []RelativePoint{{XMM: 8.5, YMM: -4.95}, {XMM: 8.5, YMM: -2}, {XMM: 16.4, YMM: -2}}, Layer: "F.Cu", WidthMM: 0.4, Required: true},
			{ID: "lower_emitter", NetTemplate: "lower_emitter", From: RouteEndpoint{ComponentRole: "lower_output", Pin: "2"}, To: RouteEndpoint{ComponentRole: "lower_emitter_resistor", Pin: "1"}, Layer: "F.Cu", WidthMM: 0.4, Required: true},
			{ID: "amp_out_join", NetTemplate: "amp_out", From: RouteEndpoint{ComponentRole: "upper_emitter_resistor", Pin: "2"}, To: RouteEndpoint{ComponentRole: "lower_emitter_resistor", Pin: "2"}, Layer: "F.Cu", WidthMM: 0.5, Required: true},
			{ID: "amp_out_port", NetTemplate: "amp_out", From: RouteEndpoint{ComponentRole: "upper_emitter_resistor", Pin: "2"}, To: RouteEndpoint{Port: "AMP_OUT"}, Layer: "F.Cu", WidthMM: 0.5, Required: true, DisableEntryAnchorVia: true},
			{ID: "vcc_output", NetTemplate: "vcc", From: RouteEndpoint{Port: "VCC"}, To: RouteEndpoint{ComponentRole: "upper_output", Pin: "3"}, Layer: "F.Cu", WidthMM: 0.5, Required: true, DisableEntryAnchorVia: true},
			{ID: "vcc_bias", NetTemplate: "vcc", From: RouteEndpoint{ComponentRole: "upper_output", Pin: "3"}, To: RouteEndpoint{ComponentRole: "upper_bias_feed", Pin: "1"}, Waypoints: []RelativePoint{{XMM: 11, YMM: -7}, {XMM: 2, YMM: -7}}, Layer: "F.Cu", WidthMM: 0.4, Required: true},
			{ID: "vee_output", NetTemplate: "vee", From: RouteEndpoint{Port: "VEE"}, To: RouteEndpoint{ComponentRole: "lower_output", Pin: "3"}, Waypoints: []RelativePoint{{XMM: 11, YMM: 11}, {XMM: 22, YMM: 11}, {XMM: 22, YMM: 5}, {XMM: 11.95, YMM: 5}}, Layer: "F.Cu", WidthMM: 0.5, Required: true, DisableEntryAnchorVia: true},
			{ID: "vee_bias", NetTemplate: "vee", From: RouteEndpoint{ComponentRole: "lower_output", Pin: "3"}, To: RouteEndpoint{ComponentRole: "lower_bias_feed", Pin: "2"}, Waypoints: []RelativePoint{{XMM: 11.95, YMM: 5}, {XMM: 22, YMM: 5}, {XMM: 22, YMM: 11}, {XMM: 11, YMM: 11}, {XMM: 11, YMM: 9}, {XMM: 2, YMM: 9}}, Layer: "F.Cu", WidthMM: 0.4, Required: true},
			{ID: "load_reference", NetTemplate: "load_ref", From: RouteEndpoint{Port: "LOAD_REF"}, To: RouteEndpoint{ComponentRole: "load_reference", Pin: "1"}, Layer: "F.Cu", WidthMM: 0.4, Required: true, DisableEntryAnchorVia: true},
		},
		Constraints: []PCBConstraint{
			{ID: "upper_bias_thermal_coupling", Kind: "max_spacing", Category: PCBConstraintThermalCoupling, AppliesTo: []string{"bias_upper", "upper_output"}, MaxLengthMM: 5.2, Description: "Upper bias diode must stay thermally near the NPN output device while preserving package clearance.", When: diodeString},
			{ID: "lower_bias_thermal_coupling", Kind: "max_spacing", Category: PCBConstraintThermalCoupling, AppliesTo: []string{"bias_lower", "lower_output"}, MaxLengthMM: 5.2, Description: "Lower bias diode must stay thermally near the PNP output device while preserving package clearance.", When: diodeString},
			{ID: "multiplier_thermal_coupling", Kind: "max_spacing", Category: PCBConstraintThermalCoupling, AppliesTo: []string{"bias_multiplier", "upper_output", "lower_output"}, MaxLengthMM: 5.2, Description: "VBE multiplier must track the complementary output-pair thermal region.", When: vbeMultiplier},
			{ID: "class_ab_output_current_width", Kind: "route_width", Category: PCBConstraintCurrentPath, NetTemplate: "amp_out", AppliesTo: []string{"upper_emitter_resistor", "lower_emitter_resistor"}, MinWidthMM: 0.5, Description: "Carry bounded headphone output current on the declared wider path."},
			{ID: "class_ab_output_pair_symmetry", Kind: "max_spacing", Category: PCBConstraintDeviceSymmetry, AppliesTo: []string{"upper_output", "lower_output", "upper_emitter_resistor", "lower_emitter_resistor"}, MaxLengthMM: 4, Description: "Keep complementary devices and emitter resistors geometrically symmetric around the output join."},
			{ID: "class_ab_load_return_topology", Kind: "return_topology", Category: PCBConstraintReturnTopology, NetTemplate: "load_ref", AppliesTo: []string{"load_reference", "upper_emitter_resistor", "lower_emitter_resistor"}, Description: "Keep load return distinct from small-signal reference until the parent design declares the star point."},
			{ID: "bias_thermal_tracking_review", Kind: "review_required", AppliesTo: []string{"bias_upper", "bias_lower", "upper_output", "lower_output"}, Description: "Generated SMD placement is connectivity-valid only; production Class AB bias stability requires thermal tracking review or a mechanically coupled bias sensor."},
		},
		Validation: PCBValidationExpectations{
			RequiredNets:   []string{"upper_drive", "driver_out", "lower_drive", "upper_emitter", "lower_emitter", "amp_out", "vcc", "vee", "load_ref"},
			RequiredRoutes: []string{"upper_emitter", "lower_emitter", "amp_out_join", "amp_out_port", "vcc_output", "vcc_bias", "vee_output", "vee_bias", "load_reference"},
			RequiresDRC:    true,
		},
		UnsupportedBehaviors: []string{
			"thermal placement and copper-area sizing are not verified",
			"quiescent-current trimming and VBE multiplier placement are not implemented",
			"speaker-load and power-amplifier routing constraints are intentionally blocked",
		},
	}
}

func classABOutputStageComponents() []BlockComponent {
	diodeString := RealizationWhen{Params: map[string]any{"topology": "diode_string"}}
	vbeMultiplier := RealizationWhen{Params: map[string]any{"topology": "vbe_multiplier"}}
	return []BlockComponent{
		{
			Role:                 "bias_upper",
			RefPrefix:            "D",
			Value:                "1N4148",
			SymbolID:             "Device:D",
			FootprintID:          "Diode_SMD:D_SOD-123",
			Pins:                 twoTerminalHorizontalPins(),
			PreferResolverSymbol: true,
			ComponentID:          "diode.onsemi.1n4148w.sod_123",
			ComponentVariant:     "sod_123",
			MinimumConfidence:    components.ConfidenceVerified,
			Acceptance:           components.AcceptanceConnectivity,
			PinmapRequired:       true,
			PlacementGroup:       "bias_string",
			When:                 diodeString,
		},
		{
			Role:                 "bias_lower",
			RefPrefix:            "D",
			Value:                "1N4148",
			SymbolID:             "Device:D",
			FootprintID:          "Diode_SMD:D_SOD-123",
			Pins:                 twoTerminalHorizontalPins(),
			PreferResolverSymbol: true,
			ComponentID:          "diode.onsemi.1n4148w.sod_123",
			ComponentVariant:     "sod_123",
			MinimumConfidence:    components.ConfidenceVerified,
			Acceptance:           components.AcceptanceConnectivity,
			PinmapRequired:       true,
			PlacementGroup:       "bias_string",
			When:                 diodeString,
		},
		{
			Role: "bias_multiplier", RefPrefix: "Q", Value: "VBE MULTIPLIER", SymbolID: "Device:Q_NPN_BEC", FootprintID: "Package_TO_SOT_SMD:SOT-23", Pins: bjtBECPins(),
			ComponentIDParam: "bias_multiplier_component_id", MinimumConfidence: components.ConfidenceVerified, Acceptance: components.AcceptanceConnectivity, PinmapRequired: true, PlacementGroup: "bias_string", When: vbeMultiplier,
		},
		{Role: "bias_multiplier_upper", RefPrefix: "R", Value: "1kΩ", SymbolID: "Device:R", FootprintID: "Resistor_SMD:R_0805_2012Metric", Pins: twoTerminalHorizontalPins(), ComponentQuery: &components.Query{Family: "resistor", ValueKind: "resistance"}, MinimumConfidence: components.ConfidenceRuleInferred, Acceptance: components.AcceptanceConnectivity, PlacementGroup: "bias_string", When: vbeMultiplier},
		{Role: "bias_multiplier_lower", RefPrefix: "R", Value: "1kΩ", SymbolID: "Device:R", FootprintID: "Resistor_SMD:R_0805_2012Metric", Pins: twoTerminalHorizontalPins(), ComponentQuery: &components.Query{Family: "resistor", ValueKind: "resistance"}, ComponentValueParam: "bias_multiplier_lower_resistor_value", MinimumConfidence: components.ConfidenceRuleInferred, Acceptance: components.AcceptanceConnectivity, PlacementGroup: "bias_string", When: vbeMultiplier},
		{
			Role:           "upper_bias_feed",
			RefPrefix:      "R",
			Value:          "10kΩ",
			SymbolID:       "Device:R",
			FootprintID:    "Resistor_SMD:R_0805_2012Metric",
			Pins:           twoTerminalHorizontalPins(),
			PlacementGroup: "bias_string",
		},
		{
			Role:           "lower_bias_feed",
			RefPrefix:      "R",
			Value:          "10kΩ",
			SymbolID:       "Device:R",
			FootprintID:    "Resistor_SMD:R_0805_2012Metric",
			Pins:           twoTerminalHorizontalPins(),
			PlacementGroup: "bias_string",
		},
		{
			Role:           "upper_emitter_resistor",
			RefPrefix:      "R",
			Value:          "0.47Ω",
			SymbolID:       "Device:R",
			FootprintID:    "Resistor_SMD:R_1206_3216Metric",
			Pins:           twoTerminalHorizontalPins(),
			PlacementGroup: "output_pair",
		},
		{
			Role:           "lower_emitter_resistor",
			RefPrefix:      "R",
			Value:          "0.47Ω",
			SymbolID:       "Device:R",
			FootprintID:    "Resistor_SMD:R_1206_3216Metric",
			Pins:           twoTerminalHorizontalPins(),
			PlacementGroup: "output_pair",
		},
		{
			Role:              "upper_output",
			RefPrefix:         "Q",
			Value:             "MMBT3904",
			SymbolID:          "Device:Q_NPN_BEC",
			FootprintID:       "Package_TO_SOT_SMD:SOT-23",
			Pins:              bjtBECPins(),
			ComponentIDParam:  "upper_output_component_id",
			MinimumConfidence: components.ConfidenceVerified,
			Acceptance:        components.AcceptanceConnectivity,
			PinmapRequired:    true,
			PlacementGroup:    "output_pair",
		},
		{
			Role:              "lower_output",
			RefPrefix:         "Q",
			Value:             "MMBT3906",
			SymbolID:          "Device:Q_PNP_BEC",
			FootprintID:       "Package_TO_SOT_SMD:SOT-23",
			Pins:              pnpBECPins(),
			ComponentIDParam:  "lower_output_component_id",
			MinimumConfidence: components.ConfidenceVerified,
			Acceptance:        components.AcceptanceConnectivity,
			PinmapRequired:    true,
			PlacementGroup:    "output_pair",
		},
		{
			Role:                 "load_reference",
			RefPrefix:            "TP",
			Value:                "LOAD_REF",
			SymbolID:             "Connector:TestPoint",
			FootprintID:          "TestPoint:TestPoint_Pad_D1.0mm",
			Pins:                 []transactions.PinSpec{{Number: "1"}},
			PreferResolverSymbol: true,
			PlacementGroup:       "load_reference",
		},
	}
}

func instantiateClassABOutputStage(definition BlockDefinition, request BlockRequest, params map[string]any, issues []reports.Issue) BlockOutput {
	if hasBlockingIssues(issues) {
		return dryRunBlockOutput(definition, request, nil, issues)
	}
	topology := strings.TrimSpace(stringParam(params, "topology"))
	if topology != "diode_string" && topology != "vbe_multiplier" {
		issues = append(issues, blockIssue("params.topology", "topology must be diode_string or vbe_multiplier"))
	}
	if application := strings.TrimSpace(stringParam(params, "application")); application != "headphone" {
		issues = append(issues, blockIssue("params.application", "the current BEC output-stage realization is limited to the bounded headphone application; power-pair catalog selection is available but requires a BCE-aware topology realization"))
	}
	supplyVoltage, supplyOK := parseUnit(params["supply_voltage"], "V", voltageMultipliers())
	if !supplyOK {
		issues = append(issues, blockIssue("params.supply_voltage", "supply_voltage must be a voltage literal"))
	} else if supplyVoltage <= 0 {
		issues = append(issues, blockIssue("params.supply_voltage", "supply_voltage must be positive"))
	}
	loadOhms, loadOK := parseUnit(params["load_impedance"], "Ω", resistanceMultipliers())
	if !loadOK {
		issues = append(issues, blockIssue("params.load_impedance", "load_impedance must be a resistance literal"))
	} else if loadOhms < 16 {
		issues = append(issues, blockIssue("params.load_impedance", "Class AB output stage is currently limited to headphone loads of 16Ω or greater"))
	}
	emitterOhms, emitterOK := parseUnit(params["emitter_resistor_value"], "Ω", resistanceMultipliers())
	if !emitterOK {
		issues = append(issues, blockIssue("params.emitter_resistor_value", "emitter_resistor_value must be a resistance literal"))
	} else if emitterOhms <= 0 {
		issues = append(issues, blockIssue("params.emitter_resistor_value", "emitter_resistor_value must be positive"))
	}
	if supplyOK && loadOK && emitterOK && loadOhms > 0 && emitterOhms > 0 {
		estimatedPeakCurrent := ((supplyVoltage / 2) - 0.7) / (loadOhms + emitterOhms)
		if estimatedPeakCurrent < 0 {
			estimatedPeakCurrent = 0
		}
		params["estimated_peak_current_a"] = estimatedPeakCurrent
		params["estimated_peak_emitter_resistor_dissipation_w"] = estimatedPeakCurrent * estimatedPeakCurrent * emitterOhms
	}
	biasFeedOhms, biasFeedOK := parseUnit(params["bias_feed_resistor_value"], "Ω", resistanceMultipliers())
	if !biasFeedOK {
		issues = append(issues, blockIssue("params.bias_feed_resistor_value", "bias_feed_resistor_value must be a resistance literal"))
	} else if biasFeedOhms <= 0 {
		issues = append(issues, blockIssue("params.bias_feed_resistor_value", "bias_feed_resistor_value must be positive"))
	}
	biasMultiplierUpperOhms := 0.0
	biasMultiplierLowerOhms := 0.0
	if topology == "vbe_multiplier" {
		idleCurrent, idleOK := parseUnit(params["target_quiescent_current"], "A", currentMultipliers())
		outputVBE, vbeOK := parseUnit(params["output_vbe_voltage"], "V", voltageMultipliers())
		multiplierVBE, multiplierVBEOK := parseUnit(params["bias_multiplier_vbe_voltage"], "V", voltageMultipliers())
		lowerOhms, lowerOK := parseUnit(params["bias_multiplier_lower_resistor_value"], "Ω", resistanceMultipliers())
		if !idleOK || idleCurrent <= 0 {
			issues = append(issues, blockIssue("params.target_quiescent_current", "VBE-multiplier target_quiescent_current must be a positive current literal"))
		}
		if !vbeOK || outputVBE <= 0 {
			issues = append(issues, blockIssue("params.output_vbe_voltage", "output_vbe_voltage must be a positive voltage literal"))
		}
		if !multiplierVBEOK || multiplierVBE <= 0 {
			issues = append(issues, blockIssue("params.bias_multiplier_vbe_voltage", "bias_multiplier_vbe_voltage must be a positive voltage literal"))
		}
		if !lowerOK || lowerOhms <= 0 {
			issues = append(issues, blockIssue("params.bias_multiplier_lower_resistor_value", "bias_multiplier_lower_resistor_value must be positive"))
		}
		if stringParam(params, "bias_multiplier_component_id") == "" || stringParam(params, "bias_multiplier_footprint") == "" {
			issues = append(issues, blockIssue("params.bias_multiplier_component_id", "VBE multiplier requires a catalog component ID and footprint"))
		}
		if idleOK && vbeOK && multiplierVBEOK && lowerOK && idleCurrent > 0 && outputVBE > 0 && multiplierVBE > 0 && lowerOhms > 0 && emitterOK && emitterOhms > 0 {
			targetBiasVoltage := 2 * (outputVBE + idleCurrent*emitterOhms)
			biasMultiplierLowerOhms = lowerOhms
			biasMultiplierUpperOhms = lowerOhms * (targetBiasVoltage/multiplierVBE - 1)
			if biasMultiplierUpperOhms <= 0 || !finite(biasMultiplierUpperOhms) {
				issues = append(issues, blockIssue("params.target_quiescent_current", "calculated VBE-multiplier upper resistance must be positive and finite"))
			}
			params["calculated_bias_voltage_v"] = targetBiasVoltage
			params["calculated_bias_multiplier_vbe_voltage_v"] = multiplierVBE
			params["calculated_bias_multiplier_upper_ohms"] = biasMultiplierUpperOhms
			params["calculated_quiescent_current_a"] = idleCurrent
		}
	}
	biasFootprint := stringParam(params, "bias_diode_footprint")
	emitterFootprint := stringParam(params, "emitter_resistor_footprint")
	biasFeedFootprint := stringParam(params, "bias_feed_resistor_footprint")
	outputFootprint := stringParam(params, "output_footprint")
	if biasFootprint == "" {
		issues = append(issues, blockIssue("params.bias_diode_footprint", "bias_diode_footprint is required"))
	}
	if emitterFootprint == "" {
		issues = append(issues, blockIssue("params.emitter_resistor_footprint", "emitter_resistor_footprint is required"))
	}
	if biasFeedFootprint == "" {
		issues = append(issues, blockIssue("params.bias_feed_resistor_footprint", "bias_feed_resistor_footprint is required"))
	}
	if outputFootprint == "" {
		issues = append(issues, blockIssue("params.output_footprint", "output_footprint is required"))
	}
	if hasBlockingIssues(issues) {
		return dryRunBlockOutput(definition, request, nil, issues)
	}

	allocator := NewInstanceReferenceAllocator(request.InstanceID)
	componentsByRole := blockComponentByRole(classABOutputStageComponents())
	roles := classABOutputStageRoleOrder(topology)
	refs := make(map[string]string, len(roles))
	for _, role := range roles {
		refs[role] = allocator.Next(componentsByRole[role].RefPrefix)
	}
	points := map[string]transactions.Point{
		"bias_upper":             {XMM: 20, YMM: -2.54},
		"bias_lower":             {XMM: 20, YMM: 7.62},
		"bias_multiplier":        {XMM: 20, YMM: 2.54},
		"bias_multiplier_upper":  {XMM: 13, YMM: -2.54},
		"bias_multiplier_lower":  {XMM: 13, YMM: 7.62},
		"upper_bias_feed":        {XMM: 8, YMM: -7.62},
		"lower_bias_feed":        {XMM: 8, YMM: 12.7},
		"upper_emitter_resistor": {XMM: 56, YMM: 0},
		"lower_emitter_resistor": {XMM: 56, YMM: 10.16},
		"upper_output":           {XMM: 45, YMM: 0},
		"lower_output":           {XMM: 45, YMM: 10.16},
		"load_reference":         {XMM: 65, YMM: 12},
	}
	var operations []transactions.Operation
	for _, role := range roles {
		component := componentsByRole[role]
		switch role {
		case "bias_upper", "bias_lower":
			component.FootprintID = biasFootprint
		case "bias_multiplier":
			component.FootprintID = stringParam(params, "bias_multiplier_footprint")
			component.ComponentID = stringParam(params, "bias_multiplier_component_id")
		case "bias_multiplier_upper":
			component.Value = formatOhms(biasMultiplierUpperOhms)
			component.FootprintID = biasFeedFootprint
		case "bias_multiplier_lower":
			component.Value = formatOhms(biasMultiplierLowerOhms)
			component.FootprintID = biasFeedFootprint
		case "upper_emitter_resistor", "lower_emitter_resistor":
			component.Value = formatOhms(emitterOhms)
			component.FootprintID = emitterFootprint
		case "upper_bias_feed", "lower_bias_feed":
			component.Value = formatOhms(biasFeedOhms)
			component.FootprintID = biasFeedFootprint
		case "upper_output", "lower_output":
			component.FootprintID = outputFootprint
		case "load_reference":
		default:
			issues = append(issues, blockIssue("component."+role, "unsupported Class AB component role"))
			continue
		}
		componentOps, componentIssues := ComponentOperations(component, refs[role], points[role])
		issues = append(issues, componentIssues...)
		operations = append(operations, componentOps...)
	}

	upperDriveNet := InstanceNetName(request.InstanceID, "upper_drive")
	driverNet := InstanceNetName(request.InstanceID, "driver_out")
	lowerDriveNet := InstanceNetName(request.InstanceID, "lower_drive")
	upperEmitterNet := InstanceNetName(request.InstanceID, "upper_emitter")
	lowerEmitterNet := InstanceNetName(request.InstanceID, "lower_emitter")
	ampOutNet := InstanceNetName(request.InstanceID, "amp_out")
	vccNet := InstanceNetName(request.InstanceID, "vcc")
	veeNet := InstanceNetName(request.InstanceID, "vee")
	loadRefNet := InstanceNetName(request.InstanceID, "load_ref")
	if topology == "vbe_multiplier" {
		appendConnectOperation(&operations, &issues, refs["upper_bias_feed"], "2", refs["bias_multiplier"], "3", upperDriveNet)
		appendConnectOperation(&operations, &issues, refs["bias_multiplier"], "3", refs["upper_output"], "1", upperDriveNet)
		appendConnectOperation(&operations, &issues, refs["bias_multiplier"], "3", refs["bias_multiplier_upper"], "1", upperDriveNet)
		appendConnectOperation(&operations, &issues, request.InstanceID, "DRIVER_OUT", refs["bias_multiplier"], "1", driverNet)
		appendConnectOperation(&operations, &issues, refs["bias_multiplier_upper"], "2", refs["bias_multiplier"], "1", driverNet)
		appendConnectOperation(&operations, &issues, refs["bias_multiplier_lower"], "1", refs["bias_multiplier"], "1", driverNet)
		appendConnectOperation(&operations, &issues, refs["bias_multiplier"], "2", refs["lower_output"], "1", lowerDriveNet)
		appendConnectOperation(&operations, &issues, refs["bias_multiplier"], "2", refs["bias_multiplier_lower"], "2", lowerDriveNet)
		appendConnectOperation(&operations, &issues, refs["bias_multiplier"], "2", refs["lower_bias_feed"], "1", lowerDriveNet)
	} else {
		appendConnectOperation(&operations, &issues, refs["bias_upper"], "1", refs["upper_output"], "1", upperDriveNet)
		appendConnectOperation(&operations, &issues, refs["upper_bias_feed"], "2", refs["bias_upper"], "1", upperDriveNet)
		appendConnectOperation(&operations, &issues, request.InstanceID, "DRIVER_OUT", refs["bias_upper"], "2", driverNet)
		appendConnectOperation(&operations, &issues, refs["bias_upper"], "2", refs["bias_lower"], "1", driverNet)
		appendConnectOperation(&operations, &issues, refs["bias_lower"], "2", refs["lower_output"], "1", lowerDriveNet)
		appendConnectOperation(&operations, &issues, refs["bias_lower"], "2", refs["lower_bias_feed"], "1", lowerDriveNet)
	}
	appendConnectOperation(&operations, &issues, refs["upper_output"], "2", refs["upper_emitter_resistor"], "1", upperEmitterNet)
	appendConnectOperation(&operations, &issues, refs["lower_output"], "2", refs["lower_emitter_resistor"], "1", lowerEmitterNet)
	appendConnectOperation(&operations, &issues, refs["upper_emitter_resistor"], "2", refs["lower_emitter_resistor"], "2", ampOutNet)
	appendConnectOperation(&operations, &issues, refs["upper_emitter_resistor"], "2", request.InstanceID, "AMP_OUT", ampOutNet)
	appendConnectOperation(&operations, &issues, request.InstanceID, "VCC", refs["upper_output"], "3", vccNet)
	appendConnectOperation(&operations, &issues, refs["upper_bias_feed"], "1", refs["upper_output"], "3", vccNet)
	appendConnectOperation(&operations, &issues, request.InstanceID, "VEE", refs["lower_output"], "3", veeNet)
	appendConnectOperation(&operations, &issues, refs["lower_bias_feed"], "2", refs["lower_output"], "3", veeNet)
	appendConnectOperation(&operations, &issues, request.InstanceID, "LOAD_REF", refs["load_reference"], "1", loadRefNet)

	output := dryRunBlockOutput(definition, request, operations, issues)
	output.Instance.Params = params
	for _, role := range roles {
		output.Instance.Refs = append(output.Instance.Refs, refs[role])
	}
	output.Instance.Nets = []string{upperDriveNet, driverNet, lowerDriveNet, upperEmitterNet, lowerEmitterNet, ampOutNet, vccNet, veeNet, loadRefNet}
	return output
}

func bjtBECPins() []transactions.PinSpec {
	return []transactions.PinSpec{
		{Number: "1", XMM: -2.54, YMM: 0},
		{Number: "2", XMM: 2.54, YMM: 2.54},
		{Number: "3", XMM: 2.54, YMM: -2.54},
	}
}

func pnpBECPins() []transactions.PinSpec {
	return []transactions.PinSpec{
		{Number: "1", XMM: -2.54, YMM: 0},
		{Number: "2", XMM: 2.54, YMM: -2.54},
		{Number: "3", XMM: 2.54, YMM: 2.54},
	}
}

func classABOutputStageRoleOrder(topology string) []string {
	if topology == "vbe_multiplier" {
		return []string{"upper_bias_feed", "bias_multiplier", "bias_multiplier_upper", "bias_multiplier_lower", "lower_bias_feed", "upper_emitter_resistor", "lower_emitter_resistor", "upper_output", "lower_output", "load_reference"}
	}
	return []string{"upper_bias_feed", "bias_upper", "bias_lower", "lower_bias_feed", "upper_emitter_resistor", "lower_emitter_resistor", "upper_output", "lower_output", "load_reference"}
}

func appendLabelOperation(operations *[]transactions.Operation, issues *[]reports.Issue, text string, at transactions.Point) {
	operation, err := wrapOperation(transactions.OpAddLabel, transactions.AddLabelOperation{
		Op:   transactions.OpAddLabel,
		Text: text,
		At:   at,
		Kind: "local",
	})
	if err != nil {
		*issues = append(*issues, blockIssue("label."+text, err.Error()))
		return
	}
	*operations = append(*operations, operation)
}
