package behavioralintent

import (
	"maps"
	"reflect"
	"slices"

	"kicadai/internal/architecturesearch"
)

// constrainProviderType encodes local shape/vocabulary invariants. Referential
// integrity, ordered bounds, provenance and engineering checks remain in Compile.
// Nested anyOf is supported by strict Structured Outputs; root anyOf is not.
func constrainProviderType(value reflect.Type, schema map[string]any) map[string]any {
	p := schema["properties"].(map[string]any)
	v := architecturesearch.V3ProviderVocabulary()
	for _, name := range []string{"id", "statement_id", "capability", "role"} {
		if _, ok := p[name]; ok {
			p[name] = semanticIDSchema(false)
		}
	}
	for _, name := range []string{"rationale", "path", "question", "why_needed", "reason", "description"} {
		if field, ok := p[name].(map[string]any); ok && field["type"] == "string" {
			field["minLength"] = 1
		}
	}
	switch value {
	case reflect.TypeOf(CoverageRecord{}):
		var branches []any
		for _, disposition := range []Disposition{DispositionCompiled, DispositionClarification, DispositionCapabilityGap, DispositionContext} {
			fields := maps.Clone(p)
			fields["disposition"] = schemaConstant("string", disposition)
			kinds := []string{}
			for _, kind := range []string{"requirement", "uncertainty", "clarification", "capability_gap"} {
				if referenceMatchesDisposition(disposition, kind) {
					kinds = append(kinds, kind)
				}
			}
			if disposition == DispositionContext {
				fields["references"] = emptyArraySchema()
			} else {
				fields["references"] = map[string]any{"type": "array", "minItems": 1, "items": strictSchemaObject(map[string]any{"kind": enumSchema(kinds...), "id": semanticIDSchema(false)})}
			}
			branches = append(branches, strictSchemaObject(fields))
		}
		return map[string]any{"anyOf": branches}
	case reflect.TypeOf(Uncertainty{}):
		p["kind"] = semanticIDSchema(false)
		var branches []any
		for _, resolution := range []UncertaintyResolution{ResolutionExplicit, ResolutionBounded, ResolutionClarification, ResolutionCapabilityGap} {
			fields := maps.Clone(p)
			fields["resolution"] = schemaConstant("string", resolution)
			fields["resolved_by"] = semanticIDSchema(false)
			if resolution == ResolutionExplicit || resolution == ResolutionBounded {
				fields["resolved_by"] = schemaConstant("string", "")
			}
			branches = append(branches, strictSchemaObject(fields))
		}
		return map[string]any{"anyOf": branches}
	case reflect.TypeOf(Clarification{}):
		p["uncertainty_ids"] = map[string]any{"type": "array", "minItems": 1, "items": semanticIDSchema(false)}
	case reflect.TypeOf(CapabilityGap{}):
		p["required_evidence"].(map[string]any)["minItems"] = 1
		p["required_evidence"].(map[string]any)["items"] = map[string]any{"type": "string", "minLength": 1}
	case reflect.TypeOf(architecturesearch.Project{}):
		p["name"] = semanticIDSchema(false)
		p["title"] = boundedStringSchema(1, 128)
		p["description"] = boundedStringSchema(1, 512)
	case reflect.TypeOf(architecturesearch.Requirements{}):
		for name, maximum := range map[string]int{
			"domains": architecturesearch.MaxDomains, "ports": architecturesearch.MaxPorts,
			"signals": architecturesearch.MaxSignals, "participants": architecturesearch.MaxParticipants,
			"objectives": architecturesearch.MaxObjectives, "system_constraints": architecturesearch.MaxConstraints,
			"operating_cases": architecturesearch.MaxOperatingCases, "behavioral_requirements": architecturesearch.MaxBehavioralRequirements,
		} {
			p[name].(map[string]any)["maxItems"] = maximum
		}
		for _, name := range []string{"domains", "ports", "objectives", "operating_cases", "behavioral_requirements"} {
			p[name].(map[string]any)["minItems"] = 1
		}
		p["control_transitions"] = emptyArraySchema()
	case reflect.TypeOf(architecturesearch.Domain{}):
		p["kind"] = enumSchema("reference", "supply")
		p["source"] = map[string]any{"anyOf": []any{semanticIDSchema(false), map[string]any{"type": "string", "pattern": `^port:[a-z][a-z0-9_]{0,63}$`, "maxLength": 69}}, "description": "external, a declared supply signal ID, or port:<id> for a same-domain generated power output with one objective output-role producer; never a component ID"}
		p["reference_domain"] = semanticIDSchema(true)
		p["reference_domain"].(map[string]any)["description"] = "Supply return reference-domain ID; empty only for an unambiguous single reference or legacy form. Reference domains use empty."
		p["nominal_voltage_v"] = numberSchema(-1000, 1000)
		for _, name := range []string{"min_voltage_v", "max_voltage_v"} {
			p[name] = nullableSchema(numberSchema(-1000, 1000))
		}
		p["max_current_a"] = nullableSchema(numberSchema(0, 10000))
	case reflect.TypeOf(architecturesearch.Port{}), reflect.TypeOf(architecturesearch.Signal{}), reflect.TypeOf(architecturesearch.ParticipantPort{}):
		p["kind"] = enumSchema(v.PortKinds...)
		if _, ok := p["direction"]; ok {
			p["direction"] = enumSchema(v.Directions...)
		}
		if _, ok := p["domain"]; ok {
			p["domain"] = semanticIDSchema(false)
			p["control"] = map[string]any{"type": "null"}
		}
	case reflect.TypeOf(architecturesearch.Electrical{}):
		for _, name := range []string{"min_voltage_v", "nominal_voltage_v", "max_voltage_v"} {
			p[name] = nullableSchema(numberSchema(-1000, 1000))
		}
		for name, maximum := range map[string]float64{"max_current_a": 10000, "max_source_current_ma": 1000000, "input_impedance_min_ohm": 1e15, "frequency_max_hz": 1e15} {
			p[name] = nullableSchema(numberSchema(0, maximum))
		}
		p["default_state"] = semanticIDSchema(true)
	case reflect.TypeOf(architecturesearch.Protocol{}):
		p["name"] = semanticIDSchema(false)
		p["mode"] = enumSchema(v.ProtocolModes...)
		p["max_frequency_hz"] = numberSchema(0.000001, 1e15)
	case reflect.TypeOf(architecturesearch.Participant{}):
		p["domain"] = semanticIDSchema(false)
		p["required_ports"].(map[string]any)["minItems"] = 1
		p["required_ports"].(map[string]any)["maxItems"] = architecturesearch.MaxParticipantPorts
		p["constraints"].(map[string]any)["maxItems"] = architecturesearch.MaxConstraints
	case reflect.TypeOf(architecturesearch.Objective{}):
		p["bindings"].(map[string]any)["minItems"] = 1
		p["bindings"].(map[string]any)["maxItems"] = architecturesearch.MaxBindings
		p["constraints"].(map[string]any)["maxItems"] = architecturesearch.MaxConstraints
	case reflect.TypeOf(architecturesearch.Binding{}):
		var branches []any
		for _, form := range [][]string{{"port"}, {"signal", "direction"}, {"participant", "participant_port"}} {
			fields := maps.Clone(p)
			for _, name := range []string{"port", "signal", "direction", "participant", "participant_port"} {
				fields[name] = schemaConstant("string", "")
				if slices.Contains(form, name) {
					fields[name] = semanticIDSchema(false)
				}
			}
			if slices.Contains(form, "direction") {
				fields["direction"] = enumSchema(v.Directions...)
			}
			branches = append(branches, strictSchemaObject(fields))
		}
		return map[string]any{"anyOf": branches}
	case reflect.TypeOf(architecturesearch.Constraint{}):
		return constraintProviderSchema(p, v.CanonicalUnits, v.ConstraintRelations)
	case reflect.TypeOf(architecturesearch.BoardLimits{}):
		p["max_components"] = map[string]any{"type": "integer", "minimum": 1, "maximum": architecturesearch.MaxComponents}
		for _, name := range []string{"max_width_mm", "max_height_mm"} {
			p[name] = numberSchema(0.01, architecturesearch.MaxBoardDimensionMM)
		}
	case reflect.TypeOf(architecturesearch.OperatingCase{}):
		p["conditions"].(map[string]any)["minItems"] = 1
		p["conditions"].(map[string]any)["maxItems"] = architecturesearch.MaxCaseConditions
		p["events"] = emptyArraySchema()
	case reflect.TypeOf(architecturesearch.OperatingCondition{}):
		return operatingConditionProviderSchema(p, v.OperatingAxes)
	case reflect.TypeOf(architecturesearch.BehavioralRequirement{}):
		p["transition"] = schemaConstant("string", "")
		p["operating_cases"] = map[string]any{"type": "array", "minItems": 1, "maxItems": architecturesearch.MaxOperatingCases, "items": semanticIDSchema(false)}
		var branches []any
		for _, metric := range v.BehavioralMetrics {
			fields := maps.Clone(p)
			fields["metric"] = schemaConstant("string", metric.Metric)
			fields["analysis"] = schemaConstant("string", metric.Analysis)
			fields["unit"] = schemaConstant("string", metric.Unit)
			branches = append(branches, boundedPairBranches(fields)...)
		}
		return map[string]any{"anyOf": branches}
	case reflect.TypeOf(architecturesearch.Observation{}):
		endpoint := maps.Clone(p)
		endpoint["kind"] = enumSchema("port", "signal", "domain")
		circuit := maps.Clone(p)
		circuit["kind"] = schemaConstant("string", "circuit")
		circuit["id"] = schemaConstant("string", "circuit")
		participant := maps.Clone(p)
		participant["kind"] = schemaConstant("string", "participant_port")
		participant["id"] = map[string]any{"type": "string", "pattern": `^[a-z][a-z0-9_]{0,63}\.[a-z][a-z0-9_]{0,63}$`, "maxLength": 129}
		return map[string]any{"anyOf": []any{strictSchemaObject(endpoint), strictSchemaObject(circuit), strictSchemaObject(participant)}}
	case reflect.TypeOf(architecturesearch.Acceptance{}):
		var mandatory architecturesearch.Acceptance
		applyMandatoryAcceptance(&mandatory)
		fields := reflect.ValueOf(mandatory)
		for i := 0; i < value.NumField(); i++ {
			name := value.Field(i).Tag.Get("json")
			for n := range p {
				if name == n || name == n+",omitempty" {
					p[n] = schemaConstant("boolean", fields.Field(i).Bool())
				}
			}
		}
	}
	return strictSchemaObject(p)
}

func constraintProviderSchema(p map[string]any, units, relations []string) map[string]any {
	var branches []any
	for _, relation := range relations {
		fields := maps.Clone(p)
		fields["name"] = semanticIDSchema(false)
		fields["relation"] = schemaConstant("string", relation)
		fields["unit"] = enumSchema(append([]string{""}, units...)...)
		fields["tolerance_percent"] = map[string]any{"type": "null"}
		scalar := map[string]any{"anyOf": []any{map[string]any{"type": "number"}, map[string]any{"type": "string"}, map[string]any{"type": "boolean"}}}
		switch relation {
		case "equal":
			fields["value"] = scalar
		case "one_of":
			fields["value"] = map[string]any{"type": "array", "minItems": 1, "items": scalar}
		case "range":
			fields["value"] = map[string]any{"type": "array", "minItems": 2, "maxItems": 2, "items": map[string]any{"type": "number"}}
		case "required":
			fields["value"] = schemaConstant("boolean", true)
		default:
			fields["value"] = map[string]any{"type": "number"}
		}
		if relation == "target" {
			fields["tolerance_percent"] = nullableSchema(numberSchema(0, 100))
		}
		branches = append(branches, strictSchemaObject(fields))
	}
	return map[string]any{"anyOf": branches}
}

func operatingConditionProviderSchema(p map[string]any, axes []architecturesearch.OperatingAxisCapability) map[string]any {
	var branches []any
	for _, axis := range axes {
		fields := maps.Clone(p)
		fields["axis"] = schemaConstant("string", axis.Axis)
		fields["target"] = semanticIDSchema(false)
		fields["unit"] = schemaConstant("string", axis.Unit)
		if axis.Selection {
			fields["min"], fields["max"] = map[string]any{"type": "null"}, map[string]any{"type": "null"}
			choices := []string{"all", "nominal", "minimum", "maximum"}
			if axis.Axis == "tolerance" {
				choices = append(choices, "minimum_nominal_maximum")
			}
			fields["selection"] = enumSchema(choices...)
			branches = append(branches, strictSchemaObject(fields))
		} else {
			fields["selection"] = schemaConstant("string", "")
			branches = append(branches, boundedPairBranches(fields)...)
		}
	}
	return map[string]any{"anyOf": branches}
}

// Express at least one numeric bound without unsupported allOf/if keywords.
func boundedPairBranches(p map[string]any) []any {
	minimum, maximum := maps.Clone(p), maps.Clone(p)
	minimum["min"], minimum["max"] = numberSchema(-1e15, 1e15), nullableSchema(numberSchema(-1e15, 1e15))
	maximum["min"], maximum["max"] = map[string]any{"type": "null"}, numberSchema(-1e15, 1e15)
	return []any{strictSchemaObject(minimum), strictSchemaObject(maximum)}
}

func semanticIDSchema(optional bool) map[string]any {
	pattern := semanticIDPattern.String()
	if optional {
		pattern = "^(?:[a-z][a-z0-9_]{0,63})?$"
	}
	return map[string]any{"type": "string", "pattern": pattern}
}

func schemaConstant(kind string, value any) map[string]any {
	return map[string]any{"type": kind, "const": value}
}

func enumSchema(values ...string) map[string]any {
	return map[string]any{"type": "string", "enum": slices.Clone(values)}
}

func numberSchema(minimum, maximum float64) map[string]any {
	return map[string]any{"type": "number", "minimum": minimum, "maximum": maximum}
}

func nullableSchema(schema map[string]any) map[string]any {
	return map[string]any{"anyOf": []any{schema, map[string]any{"type": "null"}}}
}

func emptyArraySchema() map[string]any {
	return map[string]any{"type": "array", "maxItems": 0, "items": map[string]any{"type": "string"}}
}

func boundedStringSchema(minimum, maximum int) map[string]any {
	return map[string]any{"type": "string", "minLength": minimum, "maxLength": maximum}
}
