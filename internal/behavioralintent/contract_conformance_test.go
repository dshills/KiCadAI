package behavioralintent

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"kicadai/internal/architecturesearch"
	"kicadai/internal/reports"
)

// These examples are authored offline for contract integration. They neither
// load the practical-board corpus nor claim model generation or board synthesis.
func syntheticFilterProposal() (string, Proposal) {
	f := func(v float64) *float64 { return &v }
	r := &architecturesearch.Requirement{
		Schema: architecturesearch.SchemaIDV3, Version: architecturesearch.VersionV3,
		Project: architecturesearch.Project{Name: "diagnostic_filter", Title: "Diagnostic filter", Description: "A bounded analog diagnostic filter interface"},
		Requirements: architecturesearch.Requirements{
			Domains: []architecturesearch.Domain{
				{ID: "return_domain", Kind: "reference", Source: "external"},
				{ID: "instrument_supply", Kind: "supply", Source: "external", MinVoltageV: f(5), NominalVoltageV: 5.2, MaxVoltageV: f(5.4)},
			},
			Ports: []architecturesearch.Port{
				{ID: "return_port", Kind: "reference", Direction: "sink", Domain: "return_domain"},
				{ID: "supply_port", Kind: "power", Direction: "sink", Domain: "instrument_supply"},
				{ID: "diagnostic_input", Kind: "analog_voltage", Direction: "sink", Domain: "instrument_supply"},
				{ID: "filtered_output", Kind: "analog_voltage", Direction: "source", Domain: "instrument_supply"},
			},
			Objectives: []architecturesearch.Objective{{ID: "diagnostic_band", Capability: "frequency_filter", Bindings: []architecturesearch.Binding{
				{Role: "input", Port: "diagnostic_input"}, {Role: "output", Port: "filtered_output"},
			}, Constraints: []architecturesearch.Constraint{{Name: "cutoff_frequency", Relation: "target", Value: json.RawMessage(`2400`), Unit: "Hz", TolerancePercent: f(4)}}}},
			OperatingCases:         []architecturesearch.OperatingCase{{ID: "supply_span", Conditions: []architecturesearch.OperatingCondition{{Axis: "supply_voltage", Target: "instrument_supply", Min: f(5), Max: f(5.4), Unit: "V"}}}},
			BehavioralRequirements: []architecturesearch.BehavioralRequirement{{ID: "cutoff_bound", Metric: "cutoff_frequency", Analysis: "ac_sweep", Observation: architecturesearch.Observation{Kind: "port", ID: "filtered_output"}, Min: f(2300), Max: f(2500), Unit: "Hz", OperatingCases: []string{"supply_span"}}},
			Constraints:            architecturesearch.BoardLimits{MaxComponents: 22, MaxWidthMM: 43, MaxHeightMM: 29},
		},
	}
	applyMandatoryAcceptance(&r.Acceptance)
	refs := []Reference{}
	for _, id := range []string{"return_domain", "instrument_supply", "return_port", "supply_port", "diagnostic_input", "filtered_output", "diagnostic_band", "supply_span", "cutoff_bound"} {
		refs = append(refs, Reference{Kind: "requirement", ID: id})
	}
	return "Use a 5.0 to 5.4 V supply with reference return and analog input/output to filter a diagnostic signal at 2400 Hz within four percent, keep measured cutoff from 2300 to 2500 Hz across that supply range, and fit 22 components within 43 by 29 mm.", Proposal{
		Version: ProposalVersion, Requirement: r,
		Coverage: []CoverageRecord{{StatementID: "statement_001", Disposition: DispositionCompiled, Rationale: "The supplied interfaces, limits, filter behavior and operating range are retained", References: refs}},
	}
}

func syntheticClarificationProposal() Proposal {
	return Proposal{Version: ProposalVersion,
		Coverage:       []CoverageRecord{{StatementID: "statement_001", Disposition: DispositionClarification, Rationale: "The requested cutoff bound is missing", References: []Reference{{Kind: "clarification", ID: "cutoff_question"}, {Kind: "uncertainty", ID: "cutoff_unknown"}}}},
		Uncertainties:  []Uncertainty{{ID: "cutoff_unknown", Path: "requirements.objectives.diagnostic_band", Kind: "frequency_bound", Description: "No cutoff frequency or tolerance was supplied", Resolution: ResolutionClarification, ResolvedBy: "cutoff_question"}},
		Clarifications: []Clarification{{ID: "cutoff_question", Path: "requirements.objectives.diagnostic_band", Question: "What cutoff frequency and allowed tolerance should the filter meet?", WhyNeeded: "The response cannot be bounded without those values", UncertaintyIDs: []string{"cutoff_unknown"}}},
	}
}

func TestProviderContractSyntheticReadyRefusalAndClarification(t *testing.T) {
	prompt, ready := syntheticFilterProposal()
	refusal := Proposal{Version: ProposalVersion,
		Coverage:       []CoverageRecord{{StatementID: "statement_001", Disposition: DispositionCapabilityGap, Rationale: "No trusted dose-response model is installed", References: []Reference{{Kind: "capability_gap", ID: "dose_gap"}}}},
		CapabilityGaps: []CapabilityGap{{ID: "dose_gap", Capability: "radiation_dose_analysis", Path: "requirements.behavioral_requirements", Reason: "The requested dose measurement lacks trusted verification", RequiredEvidence: []string{"reviewed dose-response model"}}},
	}
	for _, test := range []struct {
		name, prompt string
		proposal     Proposal
		status       Status
	}{
		{"ready", prompt, ready, StatusReady},
		{"refusal", "Measure ionizing radiation dose with verified accuracy.", refusal, StatusUnsupported},
		{"clarification", "Filter the diagnostic input using the required cutoff.", syntheticClarificationProposal(), StatusNeedsClarification},
	} {
		t.Run(test.name, func(t *testing.T) {
			wire := providerWireValue(reflect.ValueOf(test.proposal))
			if !providerSchemaAccepts(t, ProposalSchema(), wire) {
				t.Fatal("valid synthetic wire proposal rejected by schema")
			}
			encoded, err := json.Marshal(wire)
			if err != nil {
				t.Fatal(err)
			}
			decoded, issues := DecodeProposalStrict(bytes.NewReader(encoded))
			if reports.HasBlockingIssue(issues) {
				t.Fatalf("decode issues: %#v", issues)
			}
			result := Compile(test.prompt, decoded, testCapabilitySHA256)
			if result.Status != test.status || reports.HasBlockingIssue(result.Issues) {
				t.Fatalf("status=%s issues=%#v", result.Status, result.Issues)
			}
			if (result.Requirement != nil) != (test.status == StatusReady) {
				t.Fatal("executable requirement escaped terminal outcome")
			}
		})
	}
}

func TestProviderContractRejectsMalformedLocalShapes(t *testing.T) {
	f := func(v float64) *float64 { return &v }
	for _, test := range []struct {
		name   string
		mutate func(*Proposal)
	}{
		{"later control", func(p *Proposal) {
			p.Requirement.Requirements.Ports[2].Control = &architecturesearch.ControlSemantics{Function: "enable"}
		}},
		{"later event", func(p *Proposal) {
			p.Requirement.Requirements.OperatingCases[0].Events = []architecturesearch.OperatingEvent{{ID: "event"}}
		}},
		{"later transition", func(p *Proposal) { p.Requirement.Requirements.BehavioralRequirements[0].Transition = "edge" }},
		{"domain vocabulary", func(p *Proposal) { p.Requirement.Requirements.Domains[1].Kind = "power" }},
		{"prose source", func(p *Proposal) { p.Requirement.Requirements.Domains[1].Source = "from external power" }},
		{"multiple endpoints", func(p *Proposal) { p.Requirement.Requirements.Objectives[0].Bindings[0].Signal = "hidden_signal" }},
		{"missing signal direction", func(p *Proposal) {
			p.Requirement.Requirements.Objectives[0].Bindings[0] = architecturesearch.Binding{Role: "input", Signal: "sense_signal"}
		}},
		{"wrong metric units", func(p *Proposal) { p.Requirement.Requirements.BehavioralRequirements[0].Unit = "kHz" }},
		{"wrong metric analysis", func(p *Proposal) { p.Requirement.Requirements.BehavioralRequirements[0].Analysis = "transient" }},
		{"missing behavior bounds", func(p *Proposal) {
			p.Requirement.Requirements.BehavioralRequirements[0].Min = nil
			p.Requirement.Requirements.BehavioralRequirements[0].Max = nil
		}},
		{"noncanonical axis unit", func(p *Proposal) { p.Requirement.Requirements.OperatingCases[0].Conditions[0].Unit = "mV" }},
		{"required false", func(p *Proposal) {
			p.Requirement.Requirements.Objectives[0].Constraints[0] = architecturesearch.Constraint{Name: "protection", Relation: "required", Value: json.RawMessage(`false`)}
		}},
		{"range arity", func(p *Proposal) {
			p.Requirement.Requirements.Objectives[0].Constraints[0] = architecturesearch.Constraint{Name: "range", Relation: "range", Value: json.RawMessage(`[1,2,3]`)}
		}},
		{"tolerance on minimum", func(p *Proposal) { p.Requirement.Requirements.Objectives[0].Constraints[0].Relation = "minimum" }},
		{"component bound", func(p *Proposal) { p.Requirement.Requirements.Constraints.MaxComponents = 65 }},
		{"dimension lower bound", func(p *Proposal) { p.Requirement.Requirements.Constraints.MaxWidthMM = 0.001 }},
		{"voltage bound", func(p *Proposal) { p.Requirement.Requirements.Domains[1].MaxVoltageV = f(1001) }},
		{"coverage vocabulary", func(p *Proposal) { p.Coverage[0].Disposition = "captured" }},
		{"coverage reference kind", func(p *Proposal) { p.Coverage[0].References[0].Kind = "domain" }},
		{"coverage cross kind", func(p *Proposal) { p.Coverage[0].References[0].Kind = "clarification" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			prompt, proposal := syntheticFilterProposal()
			test.mutate(&proposal)
			if providerSchemaAccepts(t, ProposalSchema(), providerWireValue(reflect.ValueOf(proposal))) {
				t.Fatal("malformed proposal accepted by emitted schema")
			}
			result := Compile(prompt, proposal, testCapabilitySHA256)
			if result.Status != StatusInvalid || result.Requirement != nil {
				t.Fatalf("compiler did not fail closed: %#v", result)
			}
		})
	}
}

func TestProviderContractRetainsCompilerOnlyChecks(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*Proposal)
	}{
		{"unknown endpoint", func(p *Proposal) { p.Requirement.Requirements.Objectives[0].Bindings[0].Port = "missing_port" }},
		{"reversed bounds", func(p *Proposal) {
			b := &p.Requirement.Requirements.BehavioralRequirements[0]
			b.Min, b.Max = b.Max, b.Min
		}},
		{"missing source coverage", func(p *Proposal) { p.Coverage[0].References = p.Coverage[0].References[1:] }},
	} {
		t.Run(test.name, func(t *testing.T) {
			prompt, p := syntheticFilterProposal()
			test.mutate(&p)
			if !providerSchemaAccepts(t, ProposalSchema(), providerWireValue(reflect.ValueOf(p))) {
				t.Fatal("test must reach compiler-only boundary")
			}
			if r := Compile(prompt, p, testCapabilitySHA256); r.Status != StatusInvalid || r.Requirement != nil {
				t.Fatal("semantic invalidity escaped compiler")
			}
		})
	}
	for _, version := range []int{1, 2, 4, 5, 6} {
		prompt, p := syntheticFilterProposal()
		p.Requirement.Version = version
		p.Requirement.Schema = map[int]string{1: architecturesearch.SchemaID, 2: architecturesearch.SchemaIDV2, 4: architecturesearch.SchemaIDV4, 5: architecturesearch.SchemaIDV5, 6: architecturesearch.SchemaIDV6}[version]
		if r := Compile(prompt, p, testCapabilitySHA256); !hasCode(r.Issues, CodeRequirementInvalid) || r.Requirement != nil {
			t.Fatalf("version %d escaped advertised v3 boundary", version)
		}
	}
}

func TestProviderContractBoundAnswerCompletesAndRejectsReplay(t *testing.T) {
	prompt := "Use a 5.0 to 5.4 V supply with reference return and analog input/output to filter the diagnostic signal at the required cutoff, within 22 components and 43 by 29 mm."
	p := syntheticClarificationProposal()
	prior := Compile(prompt, p, testCapabilitySHA256)
	answer, err := BindFollowUp(p, prior, []ClarificationAnswer{{ClarificationID: "cutoff_question", UncertaintyIDs: []string{"cutoff_unknown"}, Answer: "Use 2400 Hz within four percent, with measured cutoff from 2300 to 2500 Hz across the supply range."}})
	if err != nil {
		t.Fatal(err)
	}
	_, ready := syntheticFilterProposal()
	result := CompileFollowUp(prompt, p, prior, answer, ready, testCapabilitySHA256)
	if result.Status != StatusReady || result.Requirement == nil {
		t.Fatalf("bound answer failed: %#v", result.Issues)
	}
	if prior.Requirement != nil || result.Source.SHA256 != prior.Source.SHA256 {
		t.Fatal("answer mutated source or prior executable status")
	}
	for _, field := range []string{"SourceSHA256", "CapabilitySHA256", "PriorProposalSHA256", "PriorCompilationSHA256"} {
		bad := answer
		reflect.ValueOf(&bad).Elem().FieldByName(field).SetString(strings.Repeat("0", 64))
		if r := CompileFollowUp(prompt, p, prior, bad, ready, testCapabilitySHA256); r.Status != StatusInvalid || r.Requirement != nil {
			t.Fatalf("replay accepted after %s mutation", field)
		}
	}
}

// This is a local-shape check, not a claim that arbitrary endpoint identities or
// registered metrics are supported by the installed synthesis registry.
func TestProviderContractAllowsRegisteredLocalForms(t *testing.T) {
	accept := func(t *testing.T, value any) {
		t.Helper()
		if !providerSchemaAccepts(t, schemaForType(reflect.TypeOf(value)), providerWireValue(reflect.ValueOf(value))) {
			t.Fatalf("valid local form rejected: %#v", value)
		}
	}
	v := architecturesearch.V3ProviderVocabulary()
	t.Run("external binding", func(t *testing.T) {
		accept(t, architecturesearch.Binding{Role: "input", Port: "diagnostic_input"})
	})
	t.Run("participant binding", func(t *testing.T) {
		accept(t, architecturesearch.Binding{Role: "input", Participant: "probe", ParticipantPort: "sense"})
	})
	for _, direction := range v.Directions {
		t.Run("signal binding "+direction, func(t *testing.T) {
			accept(t, architecturesearch.Binding{Role: "input", Signal: "sense_signal", Direction: direction})
		})
	}
	for relation, value := range map[string]string{
		"equal": `"analog"`, "one_of": `["analog","digital"]`, "range": `[1,2]`,
		"required": `true`, "minimum": `1`, "maximum": `2`, "target": `1.5`,
	} {
		t.Run("constraint "+relation, func(t *testing.T) {
			accept(t, architecturesearch.Constraint{Name: "choice", Relation: relation, Value: json.RawMessage(value)})
		})
	}
	f := func(v float64) *float64 { return &v }
	for _, metric := range v.BehavioralMetrics {
		t.Run("metric "+metric.Metric, func(t *testing.T) {
			accept(t, architecturesearch.BehavioralRequirement{ID: "behavior", Metric: metric.Metric, Analysis: metric.Analysis, Unit: metric.Unit, Observation: architecturesearch.Observation{Kind: "port", ID: "filtered_output"}, Max: f(2), OperatingCases: []string{"operating_case"}})
		})
	}
	for _, axis := range v.OperatingAxes {
		t.Run("axis "+axis.Axis, func(t *testing.T) {
			condition := architecturesearch.OperatingCondition{Axis: axis.Axis, Target: "operating_target", Unit: axis.Unit, Min: f(1)}
			if axis.Selection {
				condition.Min, condition.Selection = nil, "nominal"
			}
			accept(t, condition)
		})
	}
}

// Serialize all reflected wire fields, including explicit nulls/empty arrays
// required by strict provider schemas, without borrowing production emission.
func providerWireValue(value reflect.Value) any {
	if value.Type() == reflect.TypeOf(json.RawMessage{}) {
		var v any
		_ = json.Unmarshal(value.Bytes(), &v)
		return v
	}
	switch value.Kind() {
	case reflect.Pointer:
		if value.IsNil() {
			return nil
		}
		return providerWireValue(value.Elem())
	case reflect.Struct:
		result := map[string]any{}
		for i := 0; i < value.NumField(); i++ {
			name := strings.Split(value.Type().Field(i).Tag.Get("json"), ",")[0]
			if name != "" && name != "-" {
				result[name] = providerWireValue(value.Field(i))
			}
		}
		return result
	case reflect.Slice, reflect.Array:
		result := []any{}
		for i := 0; i < value.Len(); i++ {
			result = append(result, providerWireValue(value.Index(i)))
		}
		return result
	case reflect.String:
		return value.String()
	case reflect.Int:
		return float64(value.Int())
	default:
		return value.Interface()
	}
}
