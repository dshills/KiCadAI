package simmodel

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
)

type toleranceCorpusCase struct {
	ID       string
	Category string
	Expected string
	Build    func(*testing.T, bool) Plan
}

func TestFrozenToleranceCorpusNominalWorstCaseAndReordering(t *testing.T) {
	for _, fixture := range toleranceCorpusCases() {
		t.Run(fixture.ID, func(t *testing.T) {
			plan := fixture.Build(t, false)
			reordered := fixture.Build(t, true)
			firstHash := canonicalPlanHash(t, plan)
			secondHash := canonicalPlanHash(t, reordered)
			if firstHash != secondHash {
				t.Fatalf("resolved plan changes under evidence reordering\nfirst=%s\nsecond=%s", firstHash, secondHash)
			}
			nominalPlan := ClonePlan(plan)
			nominalPlan.WorstCase = false
			nominalPlan.Uncertainties = nil
			nominal, nominalDiagnostics := Evaluate(nominalPlan)
			if len(nominalDiagnostics) != 0 || nominal.Status != "pass" {
				t.Fatalf("nominal report=%+v diagnostics=%+v", nominal, nominalDiagnostics)
			}
			worst, worstDiagnostics := Evaluate(plan)
			switch fixture.Expected {
			case "worst_case_pass":
				if len(worstDiagnostics) != 0 || worst.Status != "pass" || len(worst.Corners) < 3 {
					t.Fatalf("worst-case report=%+v diagnostics=%+v", worst, worstDiagnostics)
				}
			case "nominal_only_blocked":
				if len(worstDiagnostics) == 0 || worst.Status != "blocked" || len(worst.Sensitivity) == 0 {
					t.Fatalf("nominal-only design was not attributed: report=%+v diagnostics=%+v", worst, worstDiagnostics)
				}
			default:
				t.Fatalf("unknown expectation %q", fixture.Expected)
			}
		})
	}
}

func toleranceCorpusCases() []toleranceCorpusCase {
	return []toleranceCorpusCase{
		{ID: "regulator_near_boundary", Category: "regulator_output", Expected: "worst_case_pass", Build: regulatorTolerancePlan(false)},
		{ID: "regulator_nominal_only", Category: "regulator_output", Expected: "nominal_only_blocked", Build: regulatorTolerancePlan(true)},
		{ID: "divider_near_boundary", Category: "resistor_divider", Expected: "worst_case_pass", Build: dividerTolerancePlan(false)},
		{ID: "divider_nominal_only", Category: "resistor_divider", Expected: "nominal_only_blocked", Build: dividerTolerancePlan(true)},
		{ID: "rc_filter_near_boundary", Category: "rc_timing_filter", Expected: "worst_case_pass", Build: rcTolerancePlan(false)},
		{ID: "rc_filter_nominal_only", Category: "rc_timing_filter", Expected: "nominal_only_blocked", Build: rcTolerancePlan(true)},
		{ID: "transistor_bias_near_boundary", Category: "transistor_bias", Expected: "worst_case_pass", Build: transistorTolerancePlan(false)},
		{ID: "transistor_bias_nominal_only", Category: "transistor_bias", Expected: "nominal_only_blocked", Build: transistorTolerancePlan(true)},
		{ID: "opamp_gain_near_boundary", Category: "opamp_gain", Expected: "worst_case_pass", Build: opAmpTolerancePlan(false)},
		{ID: "opamp_gain_nominal_only", Category: "opamp_gain", Expected: "nominal_only_blocked", Build: opAmpTolerancePlan(true)},
		{ID: "comparator_threshold_near_boundary", Category: "comparator_threshold", Expected: "worst_case_pass", Build: comparatorTolerancePlan(false)},
		{ID: "comparator_threshold_nominal_only", Category: "comparator_threshold", Expected: "nominal_only_blocked", Build: comparatorTolerancePlan(true)},
		{ID: "protection_clamp_near_boundary", Category: "protection_network", Expected: "worst_case_pass", Build: protectionTolerancePlan(false)},
		{ID: "protection_clamp_nominal_only", Category: "protection_network", Expected: "nominal_only_blocked", Build: protectionTolerancePlan(true)},
	}
}

func regulatorTolerancePlan(unsafe bool) func(*testing.T, bool) Plan {
	return func(t *testing.T, reverse bool) Plan {
		claim := CatalogEvidence{ModelID: ModelLinearRegulatorIdealV1, Parameters: []NamedValue{{Name: "max_load_current_ma", Value: 500}, {Name: "min_headroom_v", Value: 1}, {Name: "output_voltage_v", Value: 3.3}}}
		minimum, maximum := 3.27, 3.33
		if unsafe {
			minimum, maximum = 3.2, 3.4
		}
		uncertainty := Uncertainty{Target: "model_parameters.output_voltage_v", Source: "reviewed:output_accuracy", Nominal: 3.3, Minimum: minimum, Maximum: maximum}
		components := []ComponentEvidence{{InstanceID: "regulator", CatalogID: "regulator.reviewed", Family: "regulator", ModelClaims: []CatalogEvidence{claim}, Uncertainties: []Uncertainty{uncertainty}}}
		intent := Intent{ModelID: ModelLinearRegulatorIdealV1, WorstCase: true, Bindings: []Binding{{Role: "regulator", Component: "regulator"}}, Inputs: []NamedValue{{Name: "input_voltage_v", Value: 5}, {Name: "load_current_ma", Value: 100}}, Assertions: []Assertion{{Metric: "output_voltage_v", Min: 3.25, Max: 3.35}}}
		return mustResolveTolerance(t, intent, reorderedEvidence(components, reverse), nil)
	}
}

func dividerTolerancePlan(unsafe bool) func(*testing.T, bool) Plan {
	return func(t *testing.T, reverse bool) Plan {
		components := []ComponentEvidence{
			passiveEvidence("upper", "resistor", ModelResistorDividerDCV1, 10000, .01),
			passiveEvidence("lower", "resistor", ModelResistorDividerDCV1, 10000, .01),
		}
		minimum, maximum := 2.47, 2.53
		if unsafe {
			minimum, maximum = 2.49, 2.51
		}
		intent := Intent{ModelID: ModelResistorDividerDCV1, WorstCase: true, Bindings: []Binding{{Role: "upper_resistor", Component: "upper"}, {Role: "lower_resistor", Component: "lower"}}, Inputs: []NamedValue{{Name: "input_voltage_v", Value: 5}}, Assertions: []Assertion{{Metric: "output_voltage_v", Min: minimum, Max: maximum}}}
		return mustResolveTolerance(t, intent, reorderedEvidence(components, reverse), nil)
	}
}

func rcTolerancePlan(unsafe bool) func(*testing.T, bool) Plan {
	return func(t *testing.T, reverse bool) Plan {
		components := []ComponentEvidence{
			passiveEvidence("resistor", "resistor", ModelRCLowpassACV1, 10000, .01),
			passiveEvidence("capacitor", "capacitor", ModelRCLowpassACV1, 100e-9, .10),
		}
		minimum, maximum := 140.0, 180.0
		if unsafe {
			minimum, maximum = 150, 170
		}
		intent := Intent{ModelID: ModelRCLowpassACV1, WorstCase: true, Bindings: []Binding{{Role: "resistor", Component: "resistor"}, {Role: "capacitor", Component: "capacitor"}}, Inputs: []NamedValue{{Name: "frequency_hz", Value: 1000}}, Assertions: []Assertion{{Metric: "cutoff_frequency_hz", Min: minimum, Max: maximum}}}
		return mustResolveTolerance(t, intent, reorderedEvidence(components, reverse), nil)
	}
}

func transistorTolerancePlan(unsafe bool) func(*testing.T, bool) Plan {
	return func(t *testing.T, reverse bool) Plan {
		components := []ComponentEvidence{toleranceVoltageSourceEvidence("supply", "5V", "GND"), toleranceResistorEvidence("base_bias", 470000, "5V", "BASE"), toleranceResistorEvidence("collector_load", 1000, "5V", "COLLECTOR")}
		parameters := toleranceBJTParameters(.2, 40)
		components = append(components, ComponentEvidence{InstanceID: "q1", CatalogID: "bjt.reviewed", Family: "bjt", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveBJTNPNV1, Parameters: parameters}}, Uncertainties: []Uncertainty{{Target: "model_parameters.forward_beta", Source: "reviewed:beta", Nominal: 100, Minimum: 80, Maximum: 120}, {Target: "model_parameters.junction_temperature_k", Source: "reviewed:temperature", Nominal: 300.15, Minimum: 273.15, Maximum: 358.15}}, Connections: []ConnectionEvidence{{Function: "BASE", Net: "BASE"}, {Function: "COLLECTOR", Net: "COLLECTOR"}, {Function: "EMITTER", Net: "GND"}}})
		collectorMin, collectorMax := 3.0, 5.0
		if unsafe {
			collectorMin, collectorMax = 3.9, 4.4
		}
		intent := toleranceNonlinearIntent([]Assertion{{AnalysisID: "bias", Node: "COLLECTOR", Quantity: QuantityVoltageV, Min: collectorMin, Max: collectorMax}})
		intent.WorstCase = true
		return mustResolveTolerance(t, intent, reorderedEvidence(components, reverse), []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "5V"}, {Name: "BASE"}, {Name: "COLLECTOR"}})
	}
}

func opAmpTolerancePlan(unsafe bool) func(*testing.T, bool) Plan {
	return func(t *testing.T, reverse bool) Plan {
		parameters := standardOpAmpParameters()
		components := []ComponentEvidence{
			toleranceVoltageSourceEvidence("positive_supply", "VP", "GND"), toleranceVoltageSourceEvidence("negative_supply", "VN", "GND"), toleranceVoltageSourceEvidence("signal", "IN", "GND"),
			{InstanceID: "opamp", CatalogID: "opamp.reviewed", Family: "opamp", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveOpAmpV1, Parameters: parameters}}, Uncertainties: []Uncertainty{{Target: "model_parameters.dc_open_loop_gain", Source: "reviewed:open_loop_gain", Nominal: 100000, Minimum: 50000, Maximum: 150000}}, Connections: []ConnectionEvidence{{Function: "IN_PLUS", Net: "IN"}, {Function: "IN_MINUS", Net: "OUT"}, {Function: "OUT", Net: "OUT"}, {Function: "V_PLUS", Net: "VP"}, {Function: "V_MINUS", Net: "VN"}}},
		}
		minimum := .99997
		if unsafe {
			minimum = .999985
		}
		intent := Intent{ModelID: ModelLinearCircuitMNAV1, WorstCase: true, Analyses: []Analysis{{ID: "gain", Kind: AnalysisDCOperatingPoint, Excitations: []SourceExcitation{{Component: "positive_supply", DCValue: 5}, {Component: "negative_supply", DCValue: -5}, {Component: "signal", DCValue: 1}}}}, Assertions: []Assertion{{AnalysisID: "gain", Node: "OUT", Quantity: QuantityVoltageV, Min: minimum, Max: 1}}}
		return mustResolveTolerance(t, intent, reorderedEvidence(components, reverse), []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "IN"}, {Name: "OUT"}, {Name: "VN"}, {Name: "VP"}})
	}
}

func comparatorTolerancePlan(unsafe bool) func(*testing.T, bool) Plan {
	return func(t *testing.T, reverse bool) Plan {
		parameters := standardOpAmpParameters()
		threshold := toleranceVoltageSourceEvidence("threshold", "THRESH", "GND")
		threshold.Uncertainties = []Uncertainty{{Target: "excitation_dc_value", Source: "reviewed:threshold", Nominal: 2.5, Minimum: 2.4, Maximum: 2.6}}
		components := []ComponentEvidence{toleranceVoltageSourceEvidence("supply", "VP", "GND"), threshold, toleranceVoltageSourceEvidence("signal", "IN", "GND"), {InstanceID: "comparator", CatalogID: "comparator.reviewed", Family: "opamp", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveOpAmpV1, Parameters: parameters}}, Connections: []ConnectionEvidence{{Function: "IN_PLUS", Net: "IN"}, {Function: "IN_MINUS", Net: "THRESH"}, {Function: "OUT", Net: "OUT"}, {Function: "V_PLUS", Net: "VP"}, {Function: "V_MINUS", Net: "GND"}}}}
		signal := 3.0
		if unsafe {
			signal = 2.55
		}
		intent := Intent{ModelID: ModelLinearCircuitMNAV1, WorstCase: true, Analyses: []Analysis{{ID: "decision", Kind: AnalysisDCOperatingPoint, Excitations: []SourceExcitation{{Component: "supply", DCValue: 5}, {Component: "threshold", DCValue: 2.5}, {Component: "signal", DCValue: signal}}}}, Assertions: []Assertion{{AnalysisID: "decision", Node: "OUT", Quantity: QuantityVoltageV, Min: 4.89, Max: 4.91}}}
		return mustResolveTolerance(t, intent, reorderedEvidence(components, reverse), []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "IN"}, {Name: "OUT"}, {Name: "THRESH"}, {Name: "VP"}})
	}
}

func protectionTolerancePlan(unsafe bool) func(*testing.T, bool) Plan {
	return func(t *testing.T, reverse bool) Plan {
		supply := toleranceVoltageSourceEvidence("supply", "5V", "GND")
		supply.Uncertainties = []Uncertainty{{Target: "excitation_dc_value", Source: "reviewed:supply", Nominal: 5, Minimum: 4.75, Maximum: 5.25}}
		limit := toleranceResistorEvidence("limit", 1000, "5V", "OUT")
		limit.Uncertainties = []Uncertainty{{Target: "value_si", Source: "reviewed:resistance", Nominal: 1000, Minimum: 990, Maximum: 1010}}
		diode := ComponentEvidence{InstanceID: "clamp", CatalogID: "diode.reviewed", Family: "diode", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveDiodeShockleyV1, Parameters: toleranceDiodeParameters(.2, 100)}}, Uncertainties: []Uncertainty{{Target: "model_parameters.junction_temperature_k", Source: "reviewed:temperature", Nominal: 300.15, Minimum: 273.15, Maximum: 358.15}}, Connections: []ConnectionEvidence{{Function: "ANODE", Net: "OUT"}, {Function: "CATHODE", Net: "GND"}}}
		minimum, maximum := .45, 1.0
		if unsafe {
			minimum, maximum = .63, .70
		}
		intent := toleranceNonlinearIntent([]Assertion{{AnalysisID: "bias", Node: "OUT", Quantity: QuantityVoltageV, Min: minimum, Max: maximum}})
		intent.WorstCase = true
		return mustResolveTolerance(t, intent, reorderedEvidence([]ComponentEvidence{supply, limit, diode}, reverse), []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "5V"}, {Name: "OUT"}})
	}
}

func passiveEvidence(id, family, model string, nominal, fraction float64) ComponentEvidence {
	return ComponentEvidence{InstanceID: id, CatalogID: family + ".reviewed", Family: family, ValueSI: nominal, HasValueSI: true, ModelClaims: []CatalogEvidence{{ModelID: model}}, Uncertainties: []Uncertainty{{Target: "value_si", Source: "reviewed:" + family + "_tolerance", Nominal: nominal, Minimum: nominal * (1 - fraction), Maximum: nominal * (1 + fraction)}}}
}

func toleranceNonlinearIntent(assertions []Assertion) Intent {
	return Intent{ModelID: ModelNonlinearCircuitDCV1, Analyses: []Analysis{{ID: "bias", Kind: AnalysisDCOperatingPoint, Excitations: []SourceExcitation{{Component: "supply", DCValue: 5}}}}, Assertions: assertions}
}

func toleranceVoltageSourceEvidence(id, positive, negative string) ComponentEvidence {
	return ComponentEvidence{InstanceID: id, CatalogID: "source.voltage", Family: "voltage_source", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveVoltageSourceV1}}, Connections: []ConnectionEvidence{{Function: "POSITIVE", Net: positive}, {Function: "NEGATIVE", Net: negative}}}
}

func toleranceResistorEvidence(id string, value float64, a, b string) ComponentEvidence {
	return ComponentEvidence{InstanceID: id, CatalogID: "resistor", Family: "resistor", ValueSI: value, HasValueSI: true, ModelClaims: []CatalogEvidence{{ModelID: PrimitiveResistorV1}}, Connections: []ConnectionEvidence{{Function: "A", Net: a}, {Function: "B", Net: b}}}
}

func toleranceDiodeParameters(maxCurrent, maxReverse float64) []NamedValue {
	return []NamedValue{{Name: "saturation_current_a", Value: 4e-9}, {Name: "emission_coefficient", Value: 1.9}, {Name: "junction_temperature_k", Value: 300.15}, {Name: "max_forward_current_a", Value: maxCurrent}, {Name: "max_reverse_voltage_v", Value: maxReverse}}
}

func toleranceBJTParameters(maxCurrent, maxVoltage float64) []NamedValue {
	return []NamedValue{{Name: "saturation_current_a", Value: 1e-14}, {Name: "forward_beta", Value: 100}, {Name: "reverse_beta", Value: 1}, {Name: "emission_coefficient", Value: 1}, {Name: "junction_temperature_k", Value: 300.15}, {Name: "max_collector_current_a", Value: maxCurrent}, {Name: "max_collector_emitter_voltage_v", Value: maxVoltage}}
}

func standardOpAmpParameters() []NamedValue {
	return []NamedValue{{Name: "dc_open_loop_gain", Value: 100000}, {Name: "gain_bandwidth_hz", Value: 1000000}, {Name: "output_high_margin_v", Value: .1}, {Name: "output_low_margin_v", Value: .1}, {Name: "supply_max_v", Value: 30}, {Name: "supply_min_v", Value: 3}}
}

func mustResolveTolerance(t *testing.T, intent Intent, components []ComponentEvidence, nodes []NodeEvidence) Plan {
	t.Helper()
	catalogHash := canonicalComponentEvidenceHash(t, components)
	var plan Plan
	var diagnostics []Diagnostic
	if len(nodes) == 0 {
		plan, diagnostics = Resolve(intent, "tolerance-corpus", catalogHash, components)
	} else {
		plan, diagnostics = ResolveWithTopology(intent, "tolerance-corpus", catalogHash, components, nodes)
	}
	if len(diagnostics) != 0 {
		t.Fatalf("resolve diagnostics=%+v", diagnostics)
	}
	return plan
}

func canonicalComponentEvidenceHash(t testing.TB, components []ComponentEvidence) string {
	t.Helper()
	canonical := append([]ComponentEvidence(nil), components...)
	seen := map[string]bool{}
	for _, component := range canonical {
		if component.InstanceID == "" || seen[component.InstanceID] {
			t.Fatalf("canonical tolerance catalog requires unique non-empty component instance IDs; got %q", component.InstanceID)
		}
		seen[component.InstanceID] = true
	}
	slices.SortFunc(canonical, func(left, right ComponentEvidence) int {
		return strings.Compare(left.InstanceID, right.InstanceID)
	})
	return canonicalValueHash(t, canonical)
}

func reorderedEvidence(source []ComponentEvidence, reverse bool) []ComponentEvidence {
	result := append([]ComponentEvidence(nil), source...)
	if reverse {
		slices.Reverse(result)
	}
	return result
}

func canonicalPlanHash(t testing.TB, plan Plan) string {
	t.Helper()
	return canonicalValueHash(t, plan)
}

func canonicalValueHash(t testing.TB, value any) string {
	t.Helper()
	var encoded bytes.Buffer
	writeCanonicalHashValue(t, &encoded, reflect.ValueOf(value), map[canonicalHashVisit]bool{})
	digest := sha256.Sum256(encoded.Bytes())
	return hex.EncodeToString(digest[:])
}

type canonicalHashVisit struct {
	Type    reflect.Type
	Pointer uintptr
}

func writeCanonicalHashValue(t testing.TB, target *bytes.Buffer, value reflect.Value, visiting map[canonicalHashVisit]bool) {
	t.Helper()
	if !value.IsValid() {
		target.WriteString("nil;")
		return
	}
	if value.Kind() == reflect.Interface || value.Kind() == reflect.Pointer {
		if value.IsNil() {
			target.WriteString("nil;")
			return
		}
		if value.Kind() == reflect.Pointer {
			visit := canonicalHashVisit{Type: value.Type(), Pointer: value.Pointer()}
			if visiting[visit] {
				t.Fatalf("canonical tolerance hash encountered a pointer cycle at %s", value.Type())
			}
			visiting[visit] = true
			defer delete(visiting, visit)
		}
		target.WriteString(value.Kind().String())
		target.WriteByte('{')
		writeCanonicalHashValue(t, target, value.Elem(), visiting)
		target.WriteString("};")
		return
	}
	if (value.Kind() == reflect.Slice || value.Kind() == reflect.Map) && !value.IsNil() {
		visit := canonicalHashVisit{Type: value.Type(), Pointer: value.Pointer()}
		if visiting[visit] {
			t.Fatalf("canonical tolerance hash encountered a %s cycle at %s", value.Kind(), value.Type())
		}
		visiting[visit] = true
		defer delete(visiting, visit)
	}
	target.WriteString(value.Kind().String())
	target.WriteByte(':')
	switch value.Kind() {
	case reflect.Bool:
		target.WriteString(strconv.FormatBool(value.Bool()))
	case reflect.String:
		text := value.String()
		target.WriteString(strconv.Itoa(len(text)))
		target.WriteByte(':')
		target.WriteString(text)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		target.WriteString(strconv.FormatInt(value.Int(), 10))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		target.WriteString(strconv.FormatUint(value.Uint(), 10))
	case reflect.Float32, reflect.Float64:
		target.WriteString(strconv.FormatFloat(value.Float(), 'x', -1, value.Type().Bits()))
	case reflect.Struct:
		target.WriteString(value.Type().PkgPath())
		target.WriteByte('.')
		target.WriteString(value.Type().Name())
		target.WriteByte('{')
		for index := 0; index < value.NumField(); index++ {
			field := value.Type().Field(index)
			if !field.IsExported() {
				continue
			}
			target.WriteString(field.Name)
			target.WriteByte('=')
			writeCanonicalHashValue(t, target, value.Field(index), visiting)
		}
		target.WriteByte('}')
	case reflect.Slice, reflect.Array:
		target.WriteString(strconv.Itoa(value.Len()))
		target.WriteByte('[')
		for index := 0; index < value.Len(); index++ {
			writeCanonicalHashValue(t, target, value.Index(index), visiting)
		}
		target.WriteByte(']')
	case reflect.Map:
		if value.Type().Key().Kind() != reflect.String {
			t.Fatalf("canonical tolerance hash does not support map key type %s", value.Type().Key())
		}
		keys := make([]string, 0, value.Len())
		for _, key := range value.MapKeys() {
			keys = append(keys, key.String())
		}
		slices.Sort(keys)
		target.WriteString(strconv.Itoa(len(keys)))
		target.WriteByte('{')
		for _, key := range keys {
			writeCanonicalHashValue(t, target, reflect.ValueOf(key), visiting)
			writeCanonicalHashValue(t, target, value.MapIndex(reflect.ValueOf(key)), visiting)
		}
		target.WriteByte('}')
	default:
		t.Fatalf("canonical tolerance hash does not support %s", value.Kind())
	}
	target.WriteByte(';')
}
