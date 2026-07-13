package circuitgraph

import "slices"

func ProviderGraphSchema() map[string]any {
	identifier := map[string]any{"type": "string", "pattern": "^[A-Za-z][A-Za-z0-9_-]{0,62}$"}
	stringValue := map[string]any{"type": "string", "maxLength": MaxStringBytes}
	boolValue := map[string]any{"type": "boolean"}
	numberValue := map[string]any{"type": "number"}
	// Circuit-graph PCB regions use board-local coordinates in the positive quadrant.
	positiveMM := map[string]any{"type": "number", "exclusiveMinimum": 0, "maximum": MaxBoardDimensionMM}
	nonnegativeMM := map[string]any{"type": "number", "minimum": 0, "maximum": MaxBoardDimensionMM}
	stringArray := func(limit int) map[string]any {
		return map[string]any{"type": "array", "maxItems": limit, "items": stringValue}
	}
	nullable := func(schema map[string]any) map[string]any {
		return map[string]any{"anyOf": []any{schema, map[string]any{"type": "null"}}}
	}
	emptyExtensions := strictObject(map[string]any{})

	query := strictObject(map[string]any{
		"text":               stringValue,
		"family":             stringValue,
		"package":            stringValue,
		"value_kind":         stringValue,
		"value":              stringValue,
		"min_voltage_v":      numberValue,
		"minimum_confidence": map[string]any{"type": "string", "enum": []string{"verified", "library_derived", "rule_inferred"}},
	})
	libraryConstraint := strictObject(map[string]any{"library_id": stringValue})
	rating := strictObject(map[string]any{"kind": stringValue, "value": stringValue, "unit": stringValue})
	parameterValue := map[string]any{"anyOf": []any{
		stringValue,
		numberValue,
		boolValue,
		stringArray(256),
	}}
	parameter := strictObject(map[string]any{"name": identifier, "value": parameterValue})
	component := strictObject(map[string]any{
		"id":                 identifier,
		"reference":          stringValue,
		"role":               map[string]any{"type": "string", "enum": componentRoleValues()},
		"component_id":       stringValue,
		"variant_id":         stringValue,
		"query":              nullable(query),
		"value":              stringValue,
		"parameters":         map[string]any{"type": "array", "items": parameter},
		"symbol":             nullable(libraryConstraint),
		"footprint":          nullable(libraryConstraint),
		"required_ratings":   map[string]any{"type": "array", "items": rating},
		"required_functions": stringArray(256),
		"manufacturer":       stringValue,
		"mpn":                stringValue,
		"population":         map[string]any{"type": "string", "enum": []string{string(PopulationPopulate), string(PopulationDoNotPopulate)}},
		"properties":         map[string]any{"type": "array", "items": strictObject(map[string]any{"name": identifier, "value": stringValue})},
		"extensions":         emptyExtensions,
	})
	endpoint := strictObject(map[string]any{
		"component":     identifier,
		"unit":          stringValue,
		"selector_kind": map[string]any{"type": "string", "enum": []string{string(SelectorFunction), string(SelectorAlias), string(SelectorSymbolPin)}},
		"selector":      stringValue,
	})
	net := strictObject(map[string]any{
		"name":              stringValue,
		"role":              map[string]any{"type": "string", "enum": netRoleValues()},
		"required":          boolValue,
		"voltage_domain":    stringValue,
		"net_class":         map[string]any{"type": "string", "enum": []string{"", "signal", "clock", "power", "ground"}},
		"current_ma":        numberValue,
		"width_mm":          numberValue,
		"clearance_mm":      numberValue,
		"differential_pair": stringValue,
		"endpoints":         map[string]any{"type": "array", "minItems": 2, "maxItems": MaxEndpointsPerNet, "items": endpoint},
	})
	busMember := strictObject(map[string]any{"net": stringValue, "label": stringValue, "polarity": stringValue})
	bus := strictObject(map[string]any{
		"id": identifier, "name": stringValue,
		"members": map[string]any{"type": "array", "minItems": 1, "items": busMember},
	})
	powerFlag := strictObject(map[string]any{"net": stringValue})
	group := strictObject(map[string]any{
		"id": identifier, "label": stringValue, "role": stringValue,
		"members": stringArray(MaxComponents), "rank": map[string]any{"type": "integer"},
		"side": map[string]any{"type": "string", "enum": []string{"", "left", "right", "top", "bottom"}},
	})
	schematicPlacement := strictObject(map[string]any{
		"component": identifier, "group": stringValue, "near": stringValue,
		"above": stringValue, "right_of": stringValue,
		"orientation": map[string]any{"type": "string", "enum": []string{"normal", "rotated_90", "rotated_180", "rotated_270"}},
		"mirror":      map[string]any{"type": "string", "enum": []string{"", "none", "x", "y"}},
	})
	bounds := strictObject(map[string]any{"x_mm": nonnegativeMM, "y_mm": nonnegativeMM, "width_mm": positiveMM, "height_mm": positiveMM})
	region := strictObject(map[string]any{"id": identifier, "role": stringValue, "bounds": bounds})
	pcbPlacement := strictObject(map[string]any{
		"component": identifier, "region": stringValue, "near": stringValue,
		"edge":     map[string]any{"type": "string", "enum": []string{"", "left", "right", "top", "bottom"}},
		"priority": map[string]any{"type": "integer"}, "max_distance_mm": numberValue,
	})
	keepout := strictObject(map[string]any{"id": identifier, "bounds": bounds, "layers": stringArray(32)})
	zone := strictObject(map[string]any{"net": stringValue, "layers": stringArray(32), "clearance_mm": numberValue})

	return strictObject(map[string]any{
		"schema":  map[string]any{"type": "string", "const": SchemaID},
		"version": map[string]any{"type": "integer", "const": Version},
		"project": strictObject(map[string]any{
			"name":        map[string]any{"type": "string", "pattern": "^[A-Za-z0-9][A-Za-z0-9_-]{0,127}$"},
			"title":       stringValue,
			"description": map[string]any{"type": "string", "maxLength": MaxDescriptionBytes},
			"acceptance":  map[string]any{"type": "string", "enum": []string{string(AcceptanceStructural), string(AcceptanceConnectivity), string(AcceptanceERCDRC), string(AcceptanceFabricationCandidate)}},
			"board": strictObject(map[string]any{
				"width_mm": positiveMM, "height_mm": positiveMM,
				"layers":            map[string]any{"type": "integer", "enum": []int{2, 4}},
				"edge_clearance_mm": nonnegativeMM,
			}),
		}),
		"components":  map[string]any{"type": "array", "minItems": 1, "maxItems": MaxComponents, "items": component},
		"nets":        map[string]any{"type": "array", "minItems": 1, "maxItems": MaxNets, "items": net},
		"no_connects": map[string]any{"type": "array", "maxItems": MaxNoConnects, "items": endpoint},
		"power_flags": map[string]any{"type": "array", "maxItems": MaxPowerFlags, "items": powerFlag},
		"buses":       map[string]any{"type": "array", "maxItems": MaxBuses, "items": bus},
		"schematic": strictObject(map[string]any{
			"flow":   map[string]any{"type": "string", "const": string(FlowLeftToRight)},
			"origin": map[string]any{"type": "string", "const": string(OriginCentered)},
			"groups": map[string]any{"type": "array", "items": group},
			"lanes": strictObject(map[string]any{
				"power":   map[string]any{"type": "string", "const": string(LaneTop)},
				"signals": map[string]any{"type": "string", "const": string(LaneMiddle)},
				"ground":  map[string]any{"type": "string", "const": string(LaneBottom)},
			}),
			"placements": map[string]any{"type": "array", "items": schematicPlacement},
			"rules": strictObject(map[string]any{
				"positive_power_top": boolValue, "ground_bottom": boolValue, "center_on_page": boolValue,
				"prefer_labels_for_long_nets": boolValue, "avoid_wire_crossings": boolValue,
				"min_group_spacing_mm": numberValue, "min_component_spacing_mm": numberValue,
			}),
			"hierarchy": strictObject(map[string]any{
				"mode":                     map[string]any{"type": "string", "enum": []string{"flat", "auto"}},
				"max_components_per_sheet": map[string]any{"type": "integer"},
			}),
		}),
		"pcb": strictObject(map[string]any{
			"regions":    map[string]any{"type": "array", "items": region},
			"placements": map[string]any{"type": "array", "items": pcbPlacement},
			"keepouts":   map[string]any{"type": "array", "items": keepout},
			"zones":      map[string]any{"type": "array", "items": zone},
		}),
		"policy": strictObject(map[string]any{
			"allow_reference_assignment": boolValue,
			"allow_value_normalization":  boolValue,
			"allow_layout_inference":     boolValue,
			"allow_spacing_adjustment":   boolValue,
			"allow_label_insertion":      boolValue,
			"allow_placement_adjustment": boolValue,
			"allow_route_retry":          boolValue,
		}),
		"extensions": emptyExtensions,
	})
}

func strictObject(properties map[string]any) map[string]any {
	required := make([]string, 0, len(properties))
	for name := range properties {
		required = append(required, name)
	}
	slices.Sort(required)
	return map[string]any{
		"type": "object", "properties": properties,
		"required": required, "additionalProperties": false,
	}
}

func componentRoleValues() []string {
	return []string{
		string(RoleConnector), string(RoleInputConnector), string(RoleOutputConnector), string(RoleResistor),
		string(RoleCurrentLimiter), string(RolePullup), string(RoleCapacitor), string(RoleDecouplingCapacitor),
		string(RoleBulkCapacitor), string(RoleInductor), string(RoleDiode), string(RoleIndicatorLED), string(RoleIC),
		string(RoleSensor), string(RoleRegulator), string(RoleTransistor), string(RoleBJT), string(RoleMOSFET),
		string(RoleSwitch), string(RoleCrystal), string(RoleOscillator), string(RoleProtection), string(RoleFuse),
		string(RoleTVS), string(RolePowerSymbol), string(RoleGroundSymbol), string(RoleTestpoint), string(RoleGeneric),
	}
}

func netRoleValues() []string {
	return []string{
		string(NetRoleSignal), string(NetRolePower), string(NetRolePowerPos), string(NetRolePowerNeg),
		string(NetRoleGround), string(NetRoleReturn), string(NetRoleFeedback), string(NetRoleBias), string(NetRoleShield),
	}
}
