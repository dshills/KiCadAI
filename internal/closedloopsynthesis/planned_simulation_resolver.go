package closedloopsynthesis

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strings"
	"unicode"

	"kicadai/internal/simmodel"
)

const (
	OperatingSourceDCValue     = "source_dc_value"
	OperatingDeviceValueSI     = "device_value_si"
	OperatingModelParameter    = "device_model_parameter"
	OperatingAnalysisCondition = "analysis_condition"
	OperatingWorstCase         = "worst_case"

	AssertionBoundsDirect   = "direct"
	AssertionBoundsAbsolute = "absolute"

	maxCompiledAssertionBound = 1e12
)

// FreshSimulationPlanResolver re-resolves candidate variables, catalog
// identities, primitive claims, and topology on every closed-loop attempt.
// Each returned plan is keyed by executable analysis kind because dynamic,
// nonlinear, and linear workflows can require different trusted primitives.
type FreshSimulationPlanResolver interface {
	ResolveSimulationPlans(context.Context, CandidateState) (map[string]simmodel.Plan, error)
}

type FreshSimulationPlanSet struct {
	Plans             map[string]simmodel.Plan
	AnalysisPlan      AnalysisPlan
	Templates         []SimulationAnalysisTemplate
	Assertions        []SimulationAssertionBinding
	OperatingBindings []SimulationOperatingBinding
}

// FreshSimulationPlanSetResolver additionally rebinds semantic requirements
// for each materially distinct candidate. Net names may differ between
// architectures, so a fixed cross-candidate analysis plan is not promotion
// evidence.
type FreshSimulationPlanSetResolver interface {
	ResolveSimulationPlanSet(context.Context, CandidateState) (FreshSimulationPlanSet, error)
}

type SimulationAnalysisTemplate struct {
	Kind     string            `json:"kind"`
	Analysis simmodel.Analysis `json:"analysis"`
}

// SimulationAssertionBinding is resolver-owned semantic evidence. Target is
// the resolved target emitted by BuildAnalysisPlan; prototypes contain only
// trusted structured quantities and resolved node/component identities.
type SimulationAssertionBinding struct {
	Metric     string               `json:"metric"`
	Target     string               `json:"target"`
	BoundsMode string               `json:"bounds_mode"`
	Prototypes []simmodel.Assertion `json:"prototypes"`
}

// SimulationOperatingBinding maps one semantic operating axis to one bounded
// scalar in a resolved plan. It has no expression, command, topology, model
// identity, terminal, or connectivity field.
type SimulationOperatingBinding struct {
	Axis      string `json:"axis"`
	Target    string `json:"target"`
	Kind      string `json:"kind"`
	Component string `json:"component,omitempty"`
	Parameter string `json:"parameter,omitempty"`
}

type PlannedSimulationResolver struct {
	Plan              AnalysisPlan                 `json:"plan"`
	Base              FreshSimulationPlanResolver  `json:"-"`
	Templates         []SimulationAnalysisTemplate `json:"templates"`
	Assertions        []SimulationAssertionBinding `json:"assertions"`
	OperatingBindings []SimulationOperatingBinding `json:"operating_bindings"`
}

func (resolver PlannedSimulationResolver) ResolveSimulation(ctx context.Context, state CandidateState) (SimulationResolution, error) {
	if resolver.Base == nil {
		return SimulationResolution{}, fmt.Errorf("fresh simulation plan resolver is required")
	}
	plans := map[string]simmodel.Plan{}
	analysisPlan := resolver.Plan
	templates := resolver.Templates
	assertionBindings := resolver.Assertions
	operatingBindings := resolver.OperatingBindings
	var err error
	if dynamic, ok := resolver.Base.(FreshSimulationPlanSetResolver); ok {
		planSet, resolveErr := dynamic.ResolveSimulationPlanSet(ctx, cloneState(state))
		if resolveErr != nil {
			return SimulationResolution{}, resolveErr
		}
		plans, analysisPlan = planSet.Plans, planSet.AnalysisPlan
		if len(planSet.Templates) != 0 {
			templates = planSet.Templates
		}
		if len(planSet.Assertions) != 0 {
			assertionBindings = planSet.Assertions
		}
		if len(planSet.OperatingBindings) != 0 {
			operatingBindings = planSet.OperatingBindings
		}
	} else {
		plans, err = resolver.Base.ResolveSimulationPlans(ctx, cloneState(state))
		if err != nil {
			return SimulationResolution{}, err
		}
	}
	resolution, diagnostics := CompileSimulationResolution(analysisPlan, plans, templates, assertionBindings, operatingBindings)
	if len(diagnostics) != 0 {
		return SimulationResolution{}, fmt.Errorf("compile behavioral simulation: %s", joinDiagnosticMessages(diagnostics))
	}
	return resolution, nil
}

func CompileSimulationResolution(
	analysisPlan AnalysisPlan,
	basePlans map[string]simmodel.Plan,
	templates []SimulationAnalysisTemplate,
	assertionBindings []SimulationAssertionBinding,
	operatingBindings []SimulationOperatingBinding,
) (SimulationResolution, []Diagnostic) {
	var diagnostics []Diagnostic
	if analysisPlan.Schema != AnalysisPlanSchema || !validHash(analysisPlan.PlanHash) {
		diagnostics = append(diagnostics, Diagnostic{Path: "analysis_plan", Message: "compiled simulation requires a canonical behavioral analysis plan"})
	}
	templateByKind := map[string]simmodel.Analysis{}
	for index, template := range templates {
		kind := strings.TrimSpace(template.Kind)
		if kind == "" || template.Analysis.Kind != kind || templateByKind[kind].Kind != "" {
			diagnostics = append(diagnostics, Diagnostic{Path: fmt.Sprintf("templates[%d]", index), Message: "analysis templates require a unique matching executable kind"})
			continue
		}
		templateByKind[kind] = cloneSimulationAnalysis(template.Analysis)
	}
	assertionByKey := map[string]SimulationAssertionBinding{}
	for index, binding := range assertionBindings {
		key := binding.Metric + "\x00" + binding.Target
		if binding.Metric == "" || binding.Target == "" || len(binding.Prototypes) == 0 || assertionByKey[key].Metric != "" || (binding.BoundsMode != AssertionBoundsDirect && binding.BoundsMode != AssertionBoundsAbsolute) {
			diagnostics = append(diagnostics, Diagnostic{Path: fmt.Sprintf("assertion_bindings[%d]", index), Message: "assertion binding requires unique metric/target, a supported bounds mode, and at least one structured prototype"})
			continue
		}
		assertionByKey[key] = binding
	}
	operatingByKey := map[string]SimulationOperatingBinding{}
	for index, binding := range operatingBindings {
		key := binding.Axis + "\x00" + binding.Target
		if binding.Axis == "" || binding.Target == "" || operatingByKey[key].Axis != "" || !validOperatingBinding(binding) {
			diagnostics = append(diagnostics, Diagnostic{Path: fmt.Sprintf("operating_bindings[%d]", index), Message: "operating binding is duplicate or structurally invalid"})
			continue
		}
		operatingByKey[key] = binding
	}
	if len(diagnostics) != 0 {
		slices.SortStableFunc(diagnostics, compareDiagnostics)
		return SimulationResolution{}, diagnostics
	}

	cornersByCase := map[string][]PlannedCorner{}
	for _, corner := range analysisPlan.Corners {
		cornersByCase[corner.OperatingCase] = append(cornersByCase[corner.OperatingCase], corner)
	}
	plannedByKind := map[string][]PlannedAnalysis{}
	for _, analysis := range analysisPlan.Analyses {
		plannedByKind[analysis.Kind] = append(plannedByKind[analysis.Kind], analysis)
	}
	kinds := make([]string, 0, len(plannedByKind))
	for kind := range plannedByKind {
		kinds = append(kinds, kind)
	}
	slices.Sort(kinds)

	resolution := SimulationResolution{}
	for _, kind := range kinds {
		base, baseExists := basePlans[kind]
		template, templateExists := templateByKind[kind]
		if !baseExists || !templateExists {
			diagnostics = append(diagnostics, Diagnostic{Path: "analyses." + kind, Message: "fresh resolved base plan or trusted analysis template is missing"})
			continue
		}
		if !simmodel.SupportsAnalysis(base.ModelID, kind) {
			diagnostics = append(diagnostics, Diagnostic{Path: "analyses." + kind, Message: "resolved workflow does not execute the planned analysis kind"})
			continue
		}
		compiledPlan := simmodel.ClonePlan(base)
		compiledPlan.Analyses = nil
		compiledPlan.Assertions = nil
		compiledPlan.WorstCase = false
		analysisForCorner := map[string]string{}
		for _, planned := range plannedByKind[kind] {
			corners := cornersByCase[planned.OperatingCase]
			if len(corners) == 0 {
				diagnostics = append(diagnostics, Diagnostic{Path: "analyses." + planned.ID, Message: "planned operating case has no bounded corners"})
				continue
			}
			for cornerIndex, corner := range corners {
				analysis := cloneSimulationAnalysis(template)
				analysis.ID = compiledAnalysisID(kind, planned.OperatingCase, corner.ID, cornerIndex)
				for _, assignment := range corner.Assignments {
					binding, exists := operatingByKey[assignment.Axis+"\x00"+assignment.Target]
					if !exists {
						diagnostics = append(diagnostics, Diagnostic{Path: "corners." + corner.ID + "." + assignment.Axis, Message: "resolved operating binding is missing"})
						continue
					}
					if diagnostic := applyOperatingAssignment(&analysis, &compiledPlan, binding, assignment); diagnostic != nil {
						diagnostic.Path = "corners." + corner.ID + "." + diagnostic.Path
						diagnostics = append(diagnostics, *diagnostic)
					}
				}
				compiledPlan.Analyses = append(compiledPlan.Analyses, analysis)
				analysisForCorner[planned.ID+"\x00"+corner.ID] = analysis.ID
			}
		}
		slices.SortStableFunc(compiledPlan.Analyses, func(left, right simmodel.Analysis) int { return strings.Compare(left.ID, right.ID) })

		type linkedAssertion struct {
			assertion       simmodel.Assertion
			requirementID   string
			operatingCaseID string
		}
		var linked []linkedAssertion
		for _, plannedAssertion := range analysisPlan.Assertions {
			plannedAnalysisKind := ""
			for _, planned := range plannedByKind[kind] {
				if planned.ID == plannedAssertion.AnalysisID {
					plannedAnalysisKind = planned.Kind
					break
				}
			}
			if plannedAnalysisKind != kind {
				continue
			}
			binding, exists := assertionByKey[plannedAssertion.Metric+"\x00"+plannedAssertion.Target]
			if !exists {
				diagnostics = append(diagnostics, Diagnostic{Path: "assertions." + plannedAssertion.RequirementID, Message: "resolved simulation assertion binding is missing"})
				continue
			}
			minimum, maximum := compiledAssertionBounds(plannedAssertion, binding.BoundsMode)
			for _, corner := range cornersByCase[plannedAssertion.OperatingCase] {
				analysisID := analysisForCorner[plannedAssertion.AnalysisID+"\x00"+corner.ID]
				for _, prototype := range binding.Prototypes {
					assertion := prototype
					assertion.AnalysisID, assertion.Min, assertion.Max = analysisID, minimum, maximum
					linked = append(linked, linkedAssertion{assertion: assertion, requirementID: plannedAssertion.RequirementID, operatingCaseID: plannedAssertion.OperatingCase})
				}
			}
		}
		slices.SortStableFunc(linked, func(left, right linkedAssertion) int {
			return strings.Compare(compiledAssertionKey(left.assertion), compiledAssertionKey(right.assertion))
		})
		linksByBehavior := map[string][]int{}
		for _, item := range linked {
			index := len(compiledPlan.Assertions)
			compiledPlan.Assertions = append(compiledPlan.Assertions, item.assertion)
			key := item.requirementID + "\x00" + item.operatingCaseID
			linksByBehavior[key] = append(linksByBehavior[key], index)
		}
		if planDiagnostics := simmodel.ValidatePlan(compiledPlan); len(planDiagnostics) != 0 {
			for _, diagnostic := range planDiagnostics {
				diagnostics = append(diagnostics, Diagnostic{Path: "plans." + kind + "." + diagnostic.Path, Message: diagnostic.Message, Suggestion: diagnostic.Suggestion})
			}
			continue
		}
		planIndex := len(resolution.Plans)
		resolution.Plans = append(resolution.Plans, compiledPlan)
		behaviorKeys := make([]string, 0, len(linksByBehavior))
		for key := range linksByBehavior {
			behaviorKeys = append(behaviorKeys, key)
		}
		slices.Sort(behaviorKeys)
		for _, key := range behaviorKeys {
			parts := strings.SplitN(key, "\x00", 2)
			resolution.Measurements = append(resolution.Measurements, SimulationMeasurementLink{RequirementID: parts[0], OperatingCase: parts[1], Plan: planIndex, Assertions: linksByBehavior[key]})
		}
	}
	slices.SortStableFunc(resolution.Measurements, func(left, right SimulationMeasurementLink) int {
		if order := strings.Compare(left.RequirementID, right.RequirementID); order != 0 {
			return order
		}
		return strings.Compare(left.OperatingCase, right.OperatingCase)
	})
	slices.SortStableFunc(diagnostics, compareDiagnostics)
	if len(diagnostics) != 0 {
		return SimulationResolution{}, diagnostics
	}
	return resolution, nil
}

func validOperatingBinding(binding SimulationOperatingBinding) bool {
	switch binding.Kind {
	case OperatingSourceDCValue, OperatingDeviceValueSI:
		return binding.Component != "" && binding.Parameter == ""
	case OperatingModelParameter:
		return binding.Component != "" && binding.Parameter != ""
	case OperatingAnalysisCondition:
		return binding.Component == "" && binding.Parameter != ""
	case OperatingWorstCase:
		return binding.Component == "" && binding.Parameter == ""
	default:
		return false
	}
}

func applyOperatingAssignment(analysis *simmodel.Analysis, plan *simmodel.Plan, binding SimulationOperatingBinding, assignment CornerAssignment) *Diagnostic {
	switch binding.Kind {
	case OperatingWorstCase:
		if assignment.Selection == "" {
			return &Diagnostic{Path: binding.Axis, Message: "worst-case operating binding requires a named selection"}
		}
		plan.WorstCase = true
		return nil
	case OperatingSourceDCValue:
		if assignment.Value == nil {
			return &Diagnostic{Path: binding.Axis, Message: "source operating binding requires a numeric corner"}
		}
		for index := range analysis.Excitations {
			if analysis.Excitations[index].Component == binding.Component {
				analysis.Excitations[index].DCValue = *assignment.Value
				return nil
			}
		}
		return &Diagnostic{Path: binding.Axis, Message: "source operating binding references a source absent from the trusted template"}
	case OperatingDeviceValueSI:
		if assignment.Value == nil || *assignment.Value <= 0 || !finite(*assignment.Value) {
			return &Diagnostic{Path: binding.Axis, Message: "device-value operating binding requires a finite positive corner"}
		}
		override := analysisDeviceOverride(analysis, binding.Component)
		value := *assignment.Value
		override.ValueSI = &value
		setAnalysisDeviceOverride(analysis, override)
		return nil
	case OperatingModelParameter:
		if assignment.Value == nil || !finite(*assignment.Value) {
			return &Diagnostic{Path: binding.Axis, Message: "model-parameter operating binding requires a finite numeric corner"}
		}
		override := analysisDeviceOverride(analysis, binding.Component)
		override.ModelParameters = setNamedValue(override.ModelParameters, binding.Parameter, *assignment.Value)
		setAnalysisDeviceOverride(analysis, override)
		return nil
	case OperatingAnalysisCondition:
		if assignment.Value == nil || !finite(*assignment.Value) {
			return &Diagnostic{Path: binding.Axis, Message: "analysis-condition operating binding requires a finite numeric corner"}
		}
		analysis.Conditions = setNamedValue(analysis.Conditions, binding.Parameter, *assignment.Value)
		return nil
	default:
		return &Diagnostic{Path: binding.Axis, Message: "unsupported operating binding kind"}
	}
}

func analysisDeviceOverride(analysis *simmodel.Analysis, component string) simmodel.DeviceOverride {
	for _, override := range analysis.DeviceOverrides {
		if override.Component == component {
			return override
		}
	}
	return simmodel.DeviceOverride{Component: component}
}

func setAnalysisDeviceOverride(analysis *simmodel.Analysis, override simmodel.DeviceOverride) {
	for index := range analysis.DeviceOverrides {
		if analysis.DeviceOverrides[index].Component == override.Component {
			analysis.DeviceOverrides[index] = override
			return
		}
	}
	analysis.DeviceOverrides = append(analysis.DeviceOverrides, override)
	slices.SortStableFunc(analysis.DeviceOverrides, func(left, right simmodel.DeviceOverride) int { return strings.Compare(left.Component, right.Component) })
}

func setNamedValue(values []simmodel.NamedValue, name string, value float64) []simmodel.NamedValue {
	for index := range values {
		if values[index].Name == name {
			values[index].Value = value
			return values
		}
	}
	values = append(values, simmodel.NamedValue{Name: name, Value: value})
	slices.SortStableFunc(values, func(left, right simmodel.NamedValue) int { return strings.Compare(left.Name, right.Name) })
	return values
}

func compiledAssertionBounds(assertion PlannedAssertion, mode string) (float64, float64) {
	if mode == AssertionBoundsAbsolute {
		maximum := 0.0
		if assertion.Min != nil {
			maximum = math.Max(maximum, math.Abs(*assertion.Min))
		}
		if assertion.Max != nil {
			maximum = math.Max(maximum, math.Abs(*assertion.Max))
		}
		if maximum == 0 && assertion.Min == nil && assertion.Max == nil {
			maximum = maxCompiledAssertionBound
		}
		return 0, maximum
	}
	minimum, maximum := -maxCompiledAssertionBound, maxCompiledAssertionBound
	if assertion.Min != nil {
		minimum = *assertion.Min
	}
	if assertion.Max != nil {
		maximum = *assertion.Max
	}
	return minimum, maximum
}

func compiledAnalysisID(kind, operatingCase, corner string, index int) string {
	prefix := canonicalID(kind + "_" + operatingCase)
	hash := hashJSON(struct {
		Corner string `json:"corner"`
		Index  int    `json:"index"`
	}{corner, index})
	if len(prefix) > 46 {
		prefix = prefix[:46]
	}
	return prefix + "_" + hash[:16]
}

func canonicalID(value string) string {
	var builder strings.Builder
	for _, r := range strings.ToLower(value) {
		if unicode.IsLower(r) || unicode.IsDigit(r) {
			builder.WriteRune(r)
		} else if builder.Len() != 0 && !strings.HasSuffix(builder.String(), "_") {
			builder.WriteByte('_')
		}
	}
	result := strings.Trim(builder.String(), "_")
	if result == "" || !unicode.IsLower(rune(result[0])) {
		result = "analysis_" + result
	}
	return result
}

func compiledAssertionKey(assertion simmodel.Assertion) string {
	return fmt.Sprintf("%s\x00%s\x00%s\x00%s\x00%s\x00%024.12e\x00%024.12e", assertion.AnalysisID, assertion.Node, assertion.Component, assertion.ReferenceNode, assertion.Quantity, assertion.FrequencyHz, assertion.TimeS)
}

func cloneSimulationAnalysis(source simmodel.Analysis) simmodel.Analysis {
	encoded, _ := json.Marshal(source)
	var clone simmodel.Analysis
	_ = json.Unmarshal(encoded, &clone)
	return clone
}
