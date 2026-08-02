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
	OperatingSourceFrequencyHz = "source_frequency_hz"
	OperatingLoadCurrent       = "load_current"
	OperatingDeviceValueSI     = "device_value_si"
	OperatingModelParameter    = "device_model_parameter"
	OperatingAnalysisCondition = "analysis_condition"
	OperatingWorstCase         = "worst_case"
	OperatingGeneratedControl  = "generated_domain_control"

	AssertionBoundsDirect   = "direct"
	AssertionBoundsAbsolute = "absolute"

	maxCompiledAssertionBound        = 1e12
	maxStagedFallbackPrehistorySteps = 2048
	eventSupplyAxis                  = "event_supply"

	ShortCircuitHarnessOpenResistanceOhm   = 1e9
	ShortCircuitHarnessClosedResistanceOhm = 0.01
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
	Metric              string                         `json:"metric"`
	Target              string                         `json:"target"`
	BoundsMode          string                         `json:"bounds_mode"`
	Prototypes          []simmodel.Assertion           `json:"prototypes"`
	ExcitationOverrides []SimulationExcitationOverride `json:"excitation_overrides,omitempty"`
}

// SimulationExcitationOverride is a trusted metric-specific operating state.
// It can select a DC state for an already-resolved source, but cannot add a
// source, waveform, expression, or provider-controlled simulator directive.
type SimulationExcitationOverride struct {
	Component string  `json:"component"`
	DCValue   float64 `json:"dc_value"`
}

// SimulationOperatingBinding maps one semantic operating axis to one bounded
// scalar in a resolved plan. It has no expression, command, topology, model
// identity, terminal, or connectivity field.
type SimulationOperatingBinding struct {
	Axis               string  `json:"axis"`
	Target             string  `json:"target"`
	Kind               string  `json:"kind"`
	Component          string  `json:"component,omitempty"`
	ReferenceComponent string  `json:"reference_component,omitempty"`
	Parameter          string  `json:"parameter,omitempty"`
	Scale              float64 `json:"scale,omitempty"`
	Offset             float64 `json:"offset,omitempty"`
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
	if dynamic, ok := resolver.Base.(FreshSimulationPlanSetResolver); ok {
		planSet, resolveErr := dynamic.ResolveSimulationPlanSet(ctx, cloneState(state))
		if resolveErr != nil {
			return SimulationResolution{}, resolveErr
		}
		for id, plan := range planSet.Plans {
			plans[id] = plan
		}
		analysisPlan = planSet.AnalysisPlan
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
		resolvedPlans, err := resolver.Base.ResolveSimulationPlans(ctx, cloneState(state))
		if err != nil {
			return SimulationResolution{}, err
		}
		for id, plan := range resolvedPlans {
			plans[id] = plan
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
		for overrideIndex, override := range binding.ExcitationOverrides {
			if strings.TrimSpace(override.Component) == "" || math.IsNaN(override.DCValue) || math.IsInf(override.DCValue, 0) || (overrideIndex > 0 && binding.ExcitationOverrides[overrideIndex-1].Component >= override.Component) {
				diagnostics = append(diagnostics, Diagnostic{Path: fmt.Sprintf("assertion_bindings[%d].excitation_overrides", index), Message: "excitation overrides must be finite, unique, and canonically ordered"})
				break
			}
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
	measurementEvidence := map[string][]SimulationAssertionSet{}
	coveredEvents := map[string]bool{}
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
		analysisByExecutionKey := map[string]string{}
		eventAnalysisIDs := map[string]bool{}
		plannedCountByCase := map[string]int{}
		for _, planned := range plannedByKind[kind] {
			plannedCountByCase[planned.OperatingCase]++
		}
		for _, planned := range plannedByKind[kind] {
			corners := cornersByCase[planned.OperatingCase]
			if len(corners) == 0 {
				diagnostics = append(diagnostics, Diagnostic{Path: "analyses." + planned.ID, Message: "planned operating case has no bounded corners"})
				continue
			}
			for cornerIndex, corner := range corners {
				analysis := cloneSimulationAnalysis(template)
				discriminator := ""
				if plannedCountByCase[planned.OperatingCase] > 1 {
					discriminator = planned.ID
				}
				analysis.ID = compiledAnalysisID(kind, planned.OperatingCase, corner.ID, discriminator, cornerIndex)
				assignments := append([]CornerAssignment(nil), corner.Assignments...)
				slices.SortStableFunc(assignments, func(left, right CornerAssignment) int {
					leftBinding := operatingByKey[left.Axis+"\x00"+left.Target]
					rightBinding := operatingByKey[right.Axis+"\x00"+right.Target]
					leftDependent := leftBinding.Kind == OperatingLoadCurrent
					rightDependent := rightBinding.Kind == OperatingLoadCurrent
					switch {
					case leftDependent && !rightDependent:
						return 1
					case !leftDependent && rightDependent:
						return -1
					default:
						return 0
					}
				})
				for _, assignment := range assignments {
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
				appliedEvents, eventDiagnostics := applyPlannedEvents(&analysis, compiledPlan, planned, analysisPlan, operatingBindings)
				for _, eventID := range appliedEvents {
					coveredEvents[planned.OperatingCase+"\x00"+eventID] = true
				}
				for _, diagnostic := range eventDiagnostics {
					diagnostic.Path = "events." + planned.OperatingCase + "." + diagnostic.Path
					diagnostics = append(diagnostics, diagnostic)
				}
				executionKey, err := analysisExecutionKey(analysis)
				if err != nil {
					diagnostics = append(diagnostics, Diagnostic{Path: "analyses." + analysis.ID, Message: "compiled analysis execution key failed: " + err.Error()})
					continue
				}
				analysisID := analysis.ID
				if existing := analysisByExecutionKey[executionKey]; existing != "" {
					analysisID = existing
				} else {
					analysisByExecutionKey[executionKey] = analysis.ID
					compiledPlan.Analyses = append(compiledPlan.Analyses, analysis)
				}
				if len(appliedEvents) != 0 {
					eventAnalysisIDs[analysisID] = true
				}
				analysisForCorner[planned.ID+"\x00"+corner.ID] = analysisID
			}
		}
		type linkedAssertion struct {
			assertion       simmodel.Assertion
			requirementID   string
			operatingCaseID string
		}
		var linked []linkedAssertion
		overriddenAnalyses := map[string]string{}
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
			seenCompiledAssertions := map[string]bool{}
			for _, corner := range cornersByCase[plannedAssertion.OperatingCase] {
				analysisID := analysisForCorner[plannedAssertion.AnalysisID+"\x00"+corner.ID]
				if len(binding.ExcitationOverrides) != 0 {
					overrideKey := analysisID + "\x00" + plannedAssertion.RequirementID
					if existing := overriddenAnalyses[overrideKey]; existing != "" {
						analysisID = existing
					} else {
						baseIndex := slices.IndexFunc(compiledPlan.Analyses, func(analysis simmodel.Analysis) bool { return analysis.ID == analysisID })
						if baseIndex < 0 {
							diagnostics = append(diagnostics, Diagnostic{Path: "assertions." + plannedAssertion.RequirementID, Message: "metric-specific excitation base analysis is missing"})
							continue
						}
						overridden := cloneSimulationAnalysis(compiledPlan.Analyses[baseIndex])
						overridden.ID = analysisID + "_behavior_" + plannedAssertion.RequirementID
						if diagnostic := applyExcitationOverrides(&overridden, binding.ExcitationOverrides); diagnostic != nil {
							diagnostic.Path = "assertions." + plannedAssertion.RequirementID + "." + diagnostic.Path
							diagnostics = append(diagnostics, *diagnostic)
							continue
						}
						executionKey, err := analysisExecutionKey(overridden)
						if err != nil {
							diagnostics = append(diagnostics, Diagnostic{Path: "assertions." + plannedAssertion.RequirementID, Message: "metric-specific analysis execution key failed: " + err.Error()})
							continue
						}
						analysisID = analysisByExecutionKey[executionKey]
						if analysisID == "" {
							analysisID = overridden.ID
							analysisByExecutionKey[executionKey] = analysisID
							compiledPlan.Analyses = append(compiledPlan.Analyses, overridden)
						}
						overriddenAnalyses[overrideKey] = analysisID
					}
				}
				for _, prototype := range binding.Prototypes {
					analysisIndex := slices.IndexFunc(compiledPlan.Analyses, func(analysis simmodel.Analysis) bool { return analysis.ID == analysisID })
					if analysisIndex >= 0 && edgeTimeQuantity(prototype.Quantity) &&
						!analysisHasDynamicExcitation(compiledPlan.Analyses[analysisIndex]) &&
						!planHasAutonomousTransientDevice(compiledPlan) {
						continue
					}
					assertion := prototype
					assertion.AnalysisID, assertion.Min, assertion.Max = analysisID, minimum, maximum
					if event, ok := plannedAssertionEvent(analysisPlan.Events, plannedAssertion); ok {
						windowStart, windowDuration := event.TriggerTimeS, event.DurationS
						if analysisIndex >= 0 {
							windowStart, windowDuration = compiledEventWindow(compiledPlan.Analyses[analysisIndex], event)
						}
						assertion.WindowStartS = windowStart
						assertion.WindowEndS = windowStart + windowDuration
					}
					key := compiledAssertionKey(assertion)
					if seenCompiledAssertions[key] {
						continue
					}
					seenCompiledAssertions[key] = true
					linked = append(linked, linkedAssertion{assertion: assertion, requirementID: plannedAssertion.RequirementID, operatingCaseID: plannedAssertion.OperatingCase})
				}
			}
		}
		slices.SortStableFunc(compiledPlan.Analyses, func(left, right simmodel.Analysis) int { return strings.Compare(left.ID, right.ID) })
		slices.SortStableFunc(linked, func(left, right linkedAssertion) int {
			return strings.Compare(compiledAssertionKey(left.assertion), compiledAssertionKey(right.assertion))
		})
		referencedAnalyses := map[string]bool{}
		for _, item := range linked {
			referencedAnalyses[item.assertion.AnalysisID] = true
		}
		for analysisID := range eventAnalysisIDs {
			referencedAnalyses[analysisID] = true
		}
		compiledPlan.Analyses = slices.DeleteFunc(compiledPlan.Analyses, func(analysis simmodel.Analysis) bool { return !referencedAnalyses[analysis.ID] })
		voltageEventHarnesses := plannedVoltageEventHarnesses(analysisPlan)
		for batchIndex, analyses := range partitionAnalysesByDynamicWorkAndVoltageEventHarness(compiledPlan.Analyses, voltageEventHarnesses) {
			batchPlan := simmodel.ClonePlan(compiledPlan)
			batchPlan.Analyses = append([]simmodel.Analysis(nil), analyses...)
			pruneInactiveVoltageEventHarnesses(&batchPlan, activeVoltageEventHarnesses(analyses[0], voltageEventHarnesses), voltageEventHarnesses)
			batchPlan.Assertions = nil
			analysisIDs := map[string]bool{}
			batchAssertionKeys := map[string]bool{}
			for _, analysis := range analyses {
				analysisIDs[analysis.ID] = true
			}
			var batchLinked []linkedAssertion
			for _, item := range linked {
				if !analysisIDs[item.assertion.AnalysisID] {
					continue
				}
				assertionKey := compiledAssertionKey(item.assertion)
				if !batchAssertionKeys[assertionKey] {
					batchPlan.Assertions = append(batchPlan.Assertions, item.assertion)
					batchAssertionKeys[assertionKey] = true
				}
				batchLinked = append(batchLinked, item)
			}
			for _, analysis := range analyses {
				if !eventAnalysisIDs[analysis.ID] || slices.ContainsFunc(batchPlan.Assertions, func(assertion simmodel.Assertion) bool {
					return assertion.AnalysisID == analysis.ID
				}) {
					continue
				}
				assertion, ok := eventOnlyAnalysisAssertion(batchPlan, analysis)
				if !ok {
					diagnostics = append(diagnostics, Diagnostic{
						Path:       "events." + analysis.ID,
						Message:    "event-only analysis has no valid deterministic observation scope",
						Suggestion: "resolve a non-reference node for transient analysis or a reviewed thermal RC component for electrothermal analysis",
					})
					continue
				}
				batchPlan.Assertions = append(batchPlan.Assertions, assertion)
				batchAssertionKeys[compiledAssertionKey(assertion)] = true
			}
			slices.SortStableFunc(batchPlan.Assertions, func(left, right simmodel.Assertion) int {
				return strings.Compare(compiledAssertionKey(left), compiledAssertionKey(right))
			})
			assertionIndexByKey := make(map[string]int, len(batchPlan.Assertions))
			for index, assertion := range batchPlan.Assertions {
				assertionIndexByKey[compiledAssertionKey(assertion)] = index
			}
			linksByBehavior := map[string][]int{}
			for _, item := range batchLinked {
				index, exists := assertionIndexByKey[compiledAssertionKey(item.assertion)]
				if !exists {
					diagnostics = append(diagnostics, Diagnostic{
						Path:    "assertions." + item.requirementID,
						Message: "compiled measurement assertion is absent after canonical ordering",
					})
					continue
				}
				key := item.requirementID + "\x00" + item.operatingCaseID
				if !slices.Contains(linksByBehavior[key], index) {
					linksByBehavior[key] = append(linksByBehavior[key], index)
				}
			}
			if planDiagnostics := simmodel.ValidatePlan(batchPlan); len(planDiagnostics) != 0 {
				for _, diagnostic := range planDiagnostics {
					diagnostics = append(diagnostics, Diagnostic{Path: fmt.Sprintf("plans.%s[%d].%s", kind, batchIndex, diagnostic.Path), Message: diagnostic.Message, Suggestion: diagnostic.Suggestion})
				}
				continue
			}
			planIndex := len(resolution.Plans)
			resolution.Plans = append(resolution.Plans, batchPlan)
			for key, assertionIndices := range linksByBehavior {
				measurementEvidence[key] = append(measurementEvidence[key], SimulationAssertionSet{Plan: planIndex, Assertions: assertionIndices})
			}
		}
	}
	for _, event := range analysisPlan.Events {
		if !coveredEvents[event.OperatingCase+"\x00"+event.ID] {
			diagnostics = append(diagnostics, Diagnostic{
				Path:       "events." + event.OperatingCase + "." + event.ID,
				Message:    "planned event has no executable bounded source, device, or environmental binding",
				Suggestion: "select a candidate whose resolved testbench exposes the semantic event target through a reviewed dynamic primitive",
			})
		}
	}
	behaviorKeys := make([]string, 0, len(measurementEvidence))
	for key := range measurementEvidence {
		behaviorKeys = append(behaviorKeys, key)
	}
	slices.Sort(behaviorKeys)
	for _, key := range behaviorKeys {
		parts := strings.SplitN(key, "\x00", 2)
		sets := measurementEvidence[key]
		link := SimulationMeasurementLink{RequirementID: parts[0], OperatingCase: parts[1]}
		if len(sets) == 1 {
			link.Plan, link.Assertions = sets[0].Plan, sets[0].Assertions
		} else {
			link.Evidence = sets
		}
		resolution.Measurements = append(resolution.Measurements, link)
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

func eventOnlyAnalysisAssertion(plan simmodel.Plan, analysis simmodel.Analysis) (simmodel.Assertion, bool) {
	assertion := simmodel.Assertion{
		AnalysisID: analysis.ID,
		Min:        -maxCompiledAssertionBound,
		Max:        maxCompiledAssertionBound,
	}
	switch analysis.Kind {
	case simmodel.AnalysisTransient:
		node, ok := firstObservableNode(plan)
		if !ok {
			return simmodel.Assertion{}, false
		}
		assertion.Node = node
		assertion.Quantity = simmodel.QuantityVoltageV
		assertion.TimeS = analysis.DurationS
	case simmodel.AnalysisElectrothermal:
		components := dynamicThermalComponentsForTarget(plan, "circuit", false)
		if len(components) == 0 {
			return simmodel.Assertion{}, false
		}
		assertion.Component = components[0]
		assertion.Quantity = simmodel.QuantityJunctionTemperatureC
	default:
		return simmodel.Assertion{}, false
	}
	return assertion, true
}

func firstObservableNode(plan simmodel.Plan) (string, bool) {
	nodes := append([]string(nil), plan.Nodes...)
	slices.Sort(nodes)
	for _, node := range nodes {
		if node != "" && node != plan.GroundNode {
			return node, true
		}
	}
	return "", false
}

func partitionAnalysesByDynamicWork(analyses []simmodel.Analysis) [][]simmodel.Analysis {
	return partitionAnalysesByDynamicWorkAndVoltageEventHarness(analyses, nil)
}

func partitionAnalysesByDynamicWorkAndVoltageEventHarness(analyses []simmodel.Analysis, harnesses map[string]bool) [][]simmodel.Analysis {
	var batches [][]simmodel.Analysis
	var current []simmodel.Analysis
	currentHarnessKey := ""
	for _, analysis := range analyses {
		harnessKey := strings.Join(activeVoltageEventHarnesses(analysis, harnesses), "\x00")
		trial := append(append([]simmodel.Analysis(nil), current...), analysis)
		if len(current) != 0 && (harnessKey != currentHarnessKey || !simmodel.FitsPlanDynamicWork(trial)) {
			batches = append(batches, current)
			current = nil
		}
		if len(current) == 0 {
			currentHarnessKey = harnessKey
		}
		current = append(current, analysis)
	}
	if len(current) != 0 {
		batches = append(batches, current)
	}
	return batches
}

func plannedVoltageEventHarnesses(plan AnalysisPlan) map[string]bool {
	harnesses := map[string]bool{}
	for _, event := range plan.Events {
		if event.Unit == "V" && event.Kind != "short_circuit" && event.Target != "" {
			harnesses[OperatingHarnessComponentID("voltage_event", event.Target)] = true
		}
	}
	return harnesses
}

func activeVoltageEventHarnesses(analysis simmodel.Analysis, harnesses map[string]bool) []string {
	var active []string
	for _, event := range analysis.SourceValueEvents {
		if harnesses[event.Component] {
			active = append(active, event.Component)
		}
	}
	slices.Sort(active)
	return slices.Compact(active)
}

func pruneInactiveVoltageEventHarnesses(plan *simmodel.Plan, active []string, harnesses map[string]bool) {
	if plan == nil || len(harnesses) == 0 {
		return
	}
	activeSet := make(map[string]bool, len(active))
	for _, component := range active {
		activeSet[component] = true
	}
	inactive := func(component string) bool {
		return harnesses[component] && !activeSet[component]
	}
	plan.Devices = slices.DeleteFunc(plan.Devices, func(device simmodel.ResolvedDevice) bool {
		return inactive(device.Component)
	})
	for index := range plan.Analyses {
		analysis := &plan.Analyses[index]
		analysis.Excitations = slices.DeleteFunc(analysis.Excitations, func(excitation simmodel.SourceExcitation) bool {
			return inactive(excitation.Component)
		})
		analysis.DeviceOverrides = slices.DeleteFunc(analysis.DeviceOverrides, func(override simmodel.DeviceOverride) bool {
			return inactive(override.Component)
		})
		analysis.SourceValueEvents = slices.DeleteFunc(analysis.SourceValueEvents, func(event simmodel.SourceValueEvent) bool {
			return inactive(event.Component)
		})
	}
	simmodel.RefreshTopologyHash(plan)
}

func edgeTimeQuantity(quantity string) bool {
	switch quantity {
	case simmodel.QuantityRiseTimeS, simmodel.QuantityFallTimeS, simmodel.QuantitySettlingTimeS, simmodel.QuantityResponseTimeS:
		return true
	default:
		return false
	}
}

func analysisHasDynamicExcitation(analysis simmodel.Analysis) bool {
	if len(analysis.SourceValueEvents) != 0 || len(analysis.DeviceValueEvents) != 0 || len(analysis.ConditionValueEvents) != 0 {
		return true
	}
	for _, excitation := range analysis.Excitations {
		if excitation.SineFrequencyHz > 0 && excitation.SineAmplitude != 0 {
			return true
		}
		if excitation.PulsePeriodS > 0 && excitation.PulseWidthS > 0 && excitation.PulseInitialValue != excitation.PulseValue {
			return true
		}
	}
	return false
}

func applyPlannedEvents(
	analysis *simmodel.Analysis,
	plan simmodel.Plan,
	planned PlannedAnalysis,
	analysisPlan AnalysisPlan,
	operatingBindings []SimulationOperatingBinding,
) ([]string, []Diagnostic) {
	if analysis.Kind != simmodel.AnalysisTransient && analysis.Kind != simmodel.AnalysisElectrothermal {
		return nil, nil
	}
	responseAnalysis := plannedAnalysisObservesEventResponse(planned, analysisPlan)
	var stagedPulseComponents []string
	if len(planned.Events) != 0 && !responseAnalysis {
		stagedPulseComponents = stageFallbackDynamicExcitations(analysis, plannedAnalysisRequiresPeriodicStimulus(planned, analysisPlan))
	}
	eventByID := map[string]PlannedEvent{}
	for _, event := range analysisPlan.Events {
		if event.OperatingCase == planned.OperatingCase {
			eventByID[event.ID] = event
		}
	}
	var applied []string
	var diagnostics []Diagnostic
	seenEvents := map[string]bool{}
	for _, eventID := range planned.Events {
		if seenEvents[eventID] {
			continue
		}
		seenEvents[eventID] = true
		event, exists := eventByID[eventID]
		if !exists {
			diagnostics = append(diagnostics, Diagnostic{Path: eventID, Message: "planned analysis references an event absent from its operating case"})
			continue
		}
		eventApplied, diagnostic := applyPlannedEvent(analysis, plan, event, analysisPlan, operatingBindings)
		if diagnostic != nil {
			diagnostics = append(diagnostics, *diagnostic)
			continue
		}
		if eventApplied {
			if diagnostic := coupleGeneratedControlsToPowerEvent(analysis, event, operatingBindings); diagnostic != nil {
				diagnostics = append(diagnostics, *diagnostic)
				continue
			}
			applied = append(applied, event.ID)
		}
	}
	coupleLoadCurrentsToPowerEvents(analysis, operatingBindings)
	if len(applied) != 0 && responseAnalysis {
		compactPlannedEventWindow(analysis, applied)
		stagedPulseComponents = stageFallbackDynamicExcitations(analysis, true)
	}
	if len(applied) != 0 {
		retimeStagedFallbackPulses(analysis, stagedPulseComponents)
		extendStagedFallbackPulses(analysis)
	}
	truncateOperatingLoadPreludes(analysis)
	if len(applied) != 0 && !simmodel.NormalizeDynamicGrid(analysis) {
		diagnostics = append(diagnostics, Diagnostic{
			Path: analysis.ID,
			Message: fmt.Sprintf(
				"planned events cannot share a bounded dynamic grid that preserves every event boundary (duration_s=%.12g time_step_s=%.12g source_events=%d device_events=%d condition_events=%d excitations=%d)",
				analysis.DurationS, analysis.TimeStepS, len(analysis.SourceValueEvents), len(analysis.DeviceValueEvents), len(analysis.ConditionValueEvents), len(analysis.Excitations),
			),
		})
	}
	slices.Sort(applied)
	return slices.Compact(applied), diagnostics
}

func coupleGeneratedControlsToPowerEvent(analysis *simmodel.Analysis, event PlannedEvent, bindings []SimulationOperatingBinding) *Diagnostic {
	if event.Kind != "startup" && event.Kind != "shutdown" {
		return nil
	}
	for _, binding := range bindings {
		if binding.Kind != OperatingGeneratedControl {
			continue
		}
		nominal := sourceExcitationDCValue(*analysis, binding.Component)
		if !finite(nominal) {
			return &Diagnostic{Path: event.ID, Message: "generated-domain control event requires a finite resolved source level"}
		}
		initial := nominal
		if event.Initial != nil && *event.Initial == 0 {
			initial = 0
		}
		applied := nominal
		if event.Applied == 0 {
			applied = 0
		}
		var recovered *float64
		if event.Recovered != nil {
			value := nominal
			if *event.Recovered == 0 {
				value = 0
			}
			recovered = &value
		}
		controlEvent := event
		controlEvent.ID = event.ID + "_generated_control_" + binding.Component
		appended, diagnostic := appendSourceValueEvent(analysis, controlEvent, binding.Component, initial, applied, recovered)
		if diagnostic != nil {
			return diagnostic
		}
		if !appended {
			return &Diagnostic{Path: event.ID, Message: "generated-domain control source is absent from the compiled event analysis"}
		}
	}
	return nil
}

func coupleLoadCurrentsToPowerEvents(analysis *simmodel.Analysis, bindings []SimulationOperatingBinding) {
	if analysis == nil || len(analysis.SourceValueEvents) == 0 {
		return
	}
	supplyComponents := map[string]bool{}
	var loadComponents []string
	for _, binding := range bindings {
		switch {
		case binding.Kind == OperatingLoadCurrent && sourceExcitationIndex(*analysis, binding.Component) >= 0:
			loadComponents = append(loadComponents, binding.Component)
		case binding.Kind == OperatingSourceDCValue:
			supplyComponents[binding.Component] = true
		}
	}
	slices.Sort(loadComponents)
	loadComponents = slices.Compact(loadComponents)
	if len(loadComponents) == 0 || len(supplyComponents) == 0 {
		return
	}
	originalEvents := append([]simmodel.SourceValueEvent(nil), analysis.SourceValueEvents...)
	for _, event := range originalEvents {
		if !supplyComponents[event.Component] {
			continue
		}
		changesPowerState := event.Initial == 0 || event.Applied == 0 || event.Recovered != nil && *event.Recovered == 0
		if !changesPowerState {
			continue
		}
		for _, component := range loadComponents {
			nominal := sourceExcitationDCValue(*analysis, component)
			if !finite(nominal) || nominal < 0 {
				continue
			}
			initial, applied := nominal, nominal
			if event.Initial == 0 {
				initial = 0
			}
			if event.Applied == 0 {
				applied = 0
			}
			var recovered *float64
			if event.Recovered != nil {
				value := nominal
				if *event.Recovered == 0 {
					value = 0
				}
				recovered = &value
			}
			analysis.SourceValueEvents = append(analysis.SourceValueEvents, simmodel.SourceValueEvent{
				ID:                   compiledEventID("load_power_"+event.ID, component),
				Component:            component,
				TriggerTimeS:         event.TriggerTimeS,
				OriginalTriggerTimeS: event.OriginalTriggerTimeS,
				DurationS:            event.DurationS,
				Initial:              initial,
				Applied:              applied,
				Recovered:            recovered,
			})
		}
	}
	sortSourceValueEvents(analysis.SourceValueEvents)
}

func plannedAnalysisObservesEventResponse(planned PlannedAnalysis, analysisPlan AnalysisPlan) bool {
	if planned.Kind != simmodel.AnalysisTransient {
		return false
	}
	for _, assertion := range analysisPlan.Assertions {
		if assertion.AnalysisID != planned.ID || !strings.HasPrefix(assertion.Target, "event:") {
			continue
		}
		switch assertion.Metric {
		case "protection_response_time", "protection_recovery_time", "response_time", "sequence_delay":
			return true
		}
	}
	return false
}

func plannedAnalysisRequiresPeriodicStimulus(planned PlannedAnalysis, analysisPlan AnalysisPlan) bool {
	if planned.Kind != simmodel.AnalysisTransient {
		return false
	}
	for _, assertion := range analysisPlan.Assertions {
		if assertion.AnalysisID != planned.ID {
			continue
		}
		switch assertion.Metric {
		case "output_power", "output_swing":
			return true
		}
	}
	return false
}

func compactPlannedEventWindow(analysis *simmodel.Analysis, appliedEventIDs []string) {
	if analysis == nil || analysis.Kind != simmodel.AnalysisTransient || analysis.TimeStepS <= 0 {
		return
	}
	applied := map[string]bool{}
	for _, eventID := range appliedEventIDs {
		applied[eventID] = true
	}
	earliest := math.Inf(1)
	visit := func(id string, trigger float64) {
		for eventID := range applied {
			if strings.HasPrefix(id, compiledEventPrefix(eventID)) {
				earliest = math.Min(earliest, trigger)
				return
			}
		}
	}
	for _, event := range analysis.SourceValueEvents {
		visit(event.ID, event.TriggerTimeS)
	}
	for _, event := range analysis.DeviceValueEvents {
		visit(event.ID, event.TriggerTimeS)
	}
	for _, event := range analysis.ConditionValueEvents {
		visit(event.ID, event.TriggerTimeS)
	}
	if !finite(earliest) {
		return
	}
	preroll := 20 * analysis.TimeStepS
	for _, excitation := range analysis.Excitations {
		if excitation.SineAmplitude != 0 && excitation.SineFrequencyHz > 0 {
			preroll = math.Max(preroll, 4/excitation.SineFrequencyHz)
		}
	}
	preroll = math.Ceil(preroll/analysis.TimeStepS-1e-12) * analysis.TimeStepS
	offset := earliest - preroll
	if offset <= analysis.TimeStepS*1e-9 {
		return
	}
	duration := 0.0
	shift := func(id string, trigger, original *float64) {
		for eventID := range applied {
			if strings.HasPrefix(id, compiledEventPrefix(eventID)) {
				*original = *trigger
				*trigger -= offset
				return
			}
		}
	}
	for index := range analysis.SourceValueEvents {
		event := &analysis.SourceValueEvents[index]
		shift(event.ID, &event.TriggerTimeS, &event.OriginalTriggerTimeS)
		duration = math.Max(duration, event.TriggerTimeS+event.DurationS)
	}
	for index := range analysis.DeviceValueEvents {
		event := &analysis.DeviceValueEvents[index]
		shift(event.ID, &event.TriggerTimeS, &event.OriginalTriggerTimeS)
		duration = math.Max(duration, event.TriggerTimeS+event.DurationS)
	}
	for index := range analysis.ConditionValueEvents {
		event := &analysis.ConditionValueEvents[index]
		shift(event.ID, &event.TriggerTimeS, &event.OriginalTriggerTimeS)
		duration = math.Max(duration, event.TriggerTimeS+event.DurationS)
	}
	analysis.DurationS = duration
	sortSourceValueEvents(analysis.SourceValueEvents)
	sortDeviceValueEvents(analysis.DeviceValueEvents)
	sortConditionValueEvents(analysis.ConditionValueEvents)
}

func compiledEventWindow(analysis simmodel.Analysis, event PlannedEvent) (float64, float64) {
	prefix := compiledEventPrefix(event.ID)
	for _, candidate := range analysis.SourceValueEvents {
		if strings.HasPrefix(candidate.ID, prefix) {
			return candidate.TriggerTimeS, candidate.DurationS
		}
	}
	for _, candidate := range analysis.DeviceValueEvents {
		if strings.HasPrefix(candidate.ID, prefix) {
			return candidate.TriggerTimeS, candidate.DurationS
		}
	}
	for _, candidate := range analysis.ConditionValueEvents {
		if strings.HasPrefix(candidate.ID, prefix) {
			return candidate.TriggerTimeS, candidate.DurationS
		}
	}
	return event.TriggerTimeS, event.DurationS
}

func compiledEventPrefix(eventID string) string {
	prefix := canonicalID(eventID)
	if len(prefix) > 32 {
		prefix = prefix[:32]
	}
	return prefix + "_"
}

func stageFallbackDynamicExcitations(analysis *simmodel.Analysis, preservePeriodic bool) []string {
	var stagedPulseComponents []string
	for index := range analysis.Excitations {
		excitation := &analysis.Excitations[index]
		switch {
		case excitation.PulsePeriodS > 0:
			stagedPulseComponents = append(stagedPulseComponents, excitation.Component)
			active := excitation.PulseInitialValue
			if math.Abs(excitation.PulseValue) > math.Abs(active) {
				active = excitation.PulseValue
			}
			excitation.DCValue = 0
			excitation.PulseInitialValue = 0
			excitation.PulseValue = active
			excitation.PulseDelayS = 2 * analysis.TimeStepS
			excitation.PulseWidthS = analysis.DurationS
			excitation.PulsePeriodS = excitation.PulseDelayS + excitation.PulseWidthS + analysis.TimeStepS
		case excitation.SineFrequencyHz > 0:
			if preservePeriodic {
				continue
			}
			// Periodic behavioral stimuli are centered on DCValue.
			excitation.SineAmplitude = 0
			excitation.SineFrequencyHz = 0
			excitation.SinePhaseDeg = 0
			if excitation.DCValue != 0 {
				stagedPulseComponents = append(stagedPulseComponents, excitation.Component)
				excitation.PulseInitialValue = 0
				excitation.PulseValue = excitation.DCValue
				excitation.DCValue = 0
				excitation.PulseDelayS = 2 * analysis.TimeStepS
				excitation.PulseWidthS = analysis.DurationS
				excitation.PulsePeriodS = excitation.PulseDelayS + excitation.PulseWidthS + analysis.TimeStepS
			}
		}
	}
	slices.Sort(stagedPulseComponents)
	return slices.Compact(stagedPulseComponents)
}

func retimeStagedFallbackPulses(analysis *simmodel.Analysis, components []string) {
	if analysis == nil || len(components) == 0 {
		return
	}
	staged := make(map[string]bool, len(components))
	for _, component := range components {
		staged[component] = true
	}
	type pulseState struct {
		index                int
		initial, applied     float64
		delay, width, period float64
	}
	var pulses []pulseState
	for index := range analysis.Excitations {
		excitation := &analysis.Excitations[index]
		if !staged[excitation.Component] || excitation.PulsePeriodS <= 0 {
			continue
		}
		pulses = append(pulses, pulseState{
			index: index, initial: excitation.PulseInitialValue, applied: excitation.PulseValue,
			delay: excitation.PulseDelayS, width: excitation.PulseWidthS, period: excitation.PulsePeriodS,
		})
		excitation.PulseInitialValue = 0
		excitation.PulseValue = 0
		excitation.PulseDelayS = 0
		excitation.PulseWidthS = 0
		excitation.PulsePeriodS = 0
	}
	if !simmodel.NormalizeDynamicGrid(analysis) {
		for _, pulse := range pulses {
			excitation := &analysis.Excitations[pulse.index]
			excitation.PulseInitialValue = pulse.initial
			excitation.PulseValue = pulse.applied
			excitation.PulseDelayS = pulse.delay
			excitation.PulseWidthS = pulse.width
			excitation.PulsePeriodS = pulse.period
		}
		return
	}
	restoreStagedEventPrehistory(analysis)
	for _, pulse := range pulses {
		excitation := &analysis.Excitations[pulse.index]
		excitation.PulseInitialValue = pulse.initial
		excitation.PulseValue = pulse.applied
		excitation.PulseDelayS = 2 * analysis.TimeStepS
		excitation.PulseWidthS = analysis.DurationS
		excitation.PulsePeriodS = excitation.PulseDelayS + excitation.PulseWidthS + analysis.TimeStepS
	}
}

func restoreStagedEventPrehistory(analysis *simmodel.Analysis) {
	if analysis == nil || analysis.TimeStepS <= 0 {
		return
	}
	candidate := cloneSimulationAnalysis(*analysis)
	duration := candidate.DurationS
	restored := false
	restore := func(trigger *float64, original, eventDuration float64) {
		if original <= *trigger {
			return
		}
		*trigger = original
		duration = math.Max(duration, original+eventDuration)
		restored = true
	}
	for index := range candidate.SourceValueEvents {
		event := &candidate.SourceValueEvents[index]
		restore(&event.TriggerTimeS, event.OriginalTriggerTimeS, event.DurationS)
	}
	for index := range candidate.DeviceValueEvents {
		event := &candidate.DeviceValueEvents[index]
		restore(&event.TriggerTimeS, event.OriginalTriggerTimeS, event.DurationS)
	}
	for index := range candidate.ConditionValueEvents {
		event := &candidate.ConditionValueEvents[index]
		restore(&event.TriggerTimeS, event.OriginalTriggerTimeS, event.DurationS)
	}
	if !restored {
		return
	}
	candidate.DurationS = duration
	if candidate.DurationS/candidate.TimeStepS > maxStagedFallbackPrehistorySteps {
		return
	}
	if simmodel.NormalizeDynamicGrid(&candidate) {
		*analysis = candidate
	}
}

func extendStagedFallbackPulses(analysis *simmodel.Analysis) {
	for index := range analysis.Excitations {
		excitation := &analysis.Excitations[index]
		if excitation.PulsePeriodS <= 0 {
			continue
		}
		excitation.PulseWidthS = analysis.DurationS
		excitation.PulsePeriodS = excitation.PulseDelayS + excitation.PulseWidthS + analysis.TimeStepS
	}
}

func applyPlannedEvent(
	analysis *simmodel.Analysis,
	plan simmodel.Plan,
	event PlannedEvent,
	analysisPlan AnalysisPlan,
	operatingBindings []SimulationOperatingBinding,
) (bool, *Diagnostic) {
	if event.Kind == "short_circuit" {
		component := OperatingHarnessComponentID("short_circuit", event.Target)
		if !deviceComponentFamily(plan, component, "resistor") {
			return false, nil
		}
		var recovered *float64
		if event.Recovered != nil {
			value := ShortCircuitHarnessOpenResistanceOhm
			recovered = &value
		}
		return appendDeviceValueEvent(
			analysis,
			plan,
			event,
			component,
			ShortCircuitHarnessOpenResistanceOhm,
			ShortCircuitHarnessClosedResistanceOhm,
			recovered,
		)
	}
	if event.Kind == "blocked_airflow" {
		if analysis.Kind != simmodel.AnalysisElectrothermal {
			return false, nil
		}
		initial, ok := positiveReciprocalEventValue(event.Initial, 1)
		if !ok {
			return false, &Diagnostic{Path: event.ID, Message: "blocked-airflow event requires a positive finite initial cooling ratio"}
		}
		applied, ok := positiveReciprocal(event.Applied)
		if !ok {
			return false, &Diagnostic{Path: event.ID, Message: "blocked-airflow event requires a positive finite applied cooling ratio"}
		}
		recovered, ok := reciprocalOptional(event.Recovered)
		if !ok {
			return false, &Diagnostic{Path: event.ID, Message: "blocked-airflow recovery ratio must be positive and finite"}
		}
		extendAnalysisForEvent(analysis, event)
		analysis.ConditionValueEvents = append(analysis.ConditionValueEvents, simmodel.ConditionValueEvent{
			ID: compiledEventID(event.ID, "thermal_resistance_scale"), Name: "thermal_resistance_scale",
			TriggerTimeS: event.TriggerTimeS, DurationS: event.DurationS,
			Initial: initial, Applied: applied, Recovered: recovered,
		})
		sortConditionValueEvents(analysis.ConditionValueEvents)
		return true, nil
	}

	switch event.Unit {
	case "V":
		component, ok := eventSourceForTarget(plan, operatingBindings, event.Target)
		if !ok {
			return false, nil
		}
		initial := sourceExcitationDCValue(*analysis, component)
		if event.Initial != nil {
			initial = *event.Initial
		}
		applied := event.Applied
		recovered := cloneOptionalFloat(event.Recovered)
		eventHarness := OperatingHarnessComponentID("voltage_event", event.Target)
		_, eventSourceDrivesTarget := resolvedVoltageSourcePolarity(plan, component, event.Target)
		if event.Kind == "rail_loss" && component == eventHarness && !eventSourceDrivesTarget {
			if event.Initial == nil {
				return false, &Diagnostic{Path: event.ID, Message: "controlled rail-loss event requires a finite explicit initial rail voltage"}
			}
			initial = 0
			applied = math.Abs(*event.Initial - event.Applied)
			if !finite(applied) || applied <= 0 {
				return false, &Diagnostic{Path: event.ID, Message: "controlled rail-loss event requires a positive finite rail-voltage change"}
			}
			if event.Recovered != nil {
				recovery := 0.0
				recovered = &recovery
			}
		}
		return appendSourceValueEvent(analysis, event, component, initial, applied, recovered)
	case "A":
		if event.Target == "circuit" {
			return applyAggregateCurrentEvent(analysis, plan, event, analysisPlan, operatingBindings)
		}
		binding, ok := eventLoadBinding(operatingBindings, event.Target)
		if !ok {
			return false, nil
		}
		return applyCurrentEventBinding(analysis, plan, event, binding)
	case "Ohm":
		component, ok := eventResistanceComponent(plan, operatingBindings, event.Target)
		if !ok {
			return false, nil
		}
		initial := resolvedAnalysisDeviceValue(*analysis, plan, component)
		recovered := cloneOptionalFloat(event.Recovered)
		binding, reusedOperatingLoad := eventLoadBinding(operatingBindings, event.Target)
		reusedOperatingLoad = reusedOperatingLoad && binding.Axis == "load_current" && binding.Component == component
		if event.Initial != nil {
			switch {
			case !reusedOperatingLoad:
				initial = *event.Initial
			case binding.ReferenceComponent != "" && binding.Scale > 0:
				reference := math.Abs(sourceExcitationDCValue(*analysis, binding.ReferenceComponent))
				if finite(reference) && reference > 0 {
					initial = *event.Initial * reference / binding.Scale
				}
			}
		}
		if recovered != nil && reusedOperatingLoad {
			recovery := initial
			recovered = &recovery
		}
		return appendDeviceValueEvent(analysis, plan, event, component, initial, event.Applied, recovered)
	case "ratio":
		if event.Target != "circuit" || (event.Kind != "startup" && event.Kind != "shutdown") {
			return false, nil
		}
		components := eventSupplyComponents(operatingBindings)
		if len(components) == 0 {
			return false, nil
		}
		for _, component := range components {
			base := sourceExcitationDCValue(*analysis, component)
			initialRatio := 1.0
			if event.Initial != nil {
				initialRatio = *event.Initial
			}
			recovered := scaledOptional(event.Recovered, base)
			if _, diagnostic := appendSourceValueEvent(analysis, event, component, base*initialRatio, base*event.Applied, recovered); diagnostic != nil {
				return false, diagnostic
			}
		}
		return true, nil
	default:
		return false, nil
	}
}

func deviceComponentFamily(plan simmodel.Plan, component, family string) bool {
	for _, device := range plan.Devices {
		if device.Component == component {
			return device.Family == family
		}
	}
	return false
}

func applyAggregateCurrentEvent(
	analysis *simmodel.Analysis,
	plan simmodel.Plan,
	event PlannedEvent,
	analysisPlan AnalysisPlan,
	operatingBindings []SimulationOperatingBinding,
) (bool, *Diagnostic) {
	bindings := eventLoadBindings(operatingBindings)
	if len(bindings) == 0 {
		return false, nil
	}
	capacities := make([]float64, len(bindings))
	for index, binding := range bindings {
		capacity, ok := maximumOperatingAssignmentForCase(analysisPlan, event.OperatingCase, "load_current", binding.Target)
		if !ok {
			return false, &Diagnostic{Path: event.ID, Message: "aggregate current event requires a positive finite maximum for every resolved load target"}
		}
		capacities[index] = capacity
	}
	applied, ok := allocateAggregateCurrent(event.Applied, capacities)
	if !ok {
		return false, &Diagnostic{Path: event.ID, Message: "aggregate current event exceeds the declared resolved load capacity"}
	}
	initial := make([]float64, len(bindings))
	if event.Initial != nil {
		initial, ok = allocateAggregateCurrent(*event.Initial, capacities)
		if !ok {
			return false, &Diagnostic{Path: event.ID, Message: "aggregate initial current exceeds the declared resolved load capacity"}
		}
	} else {
		for index, binding := range bindings {
			initial[index], ok = resolvedLoadCurrent(*analysis, plan, binding)
			if !ok {
				return false, &Diagnostic{Path: event.ID, Message: "aggregate current event cannot derive every resolved initial load"}
			}
		}
	}
	var recovered []float64
	if event.Recovered != nil {
		recovered, ok = allocateAggregateCurrent(*event.Recovered, capacities)
		if !ok {
			return false, &Diagnostic{Path: event.ID, Message: "aggregate recovered current exceeds the declared resolved load capacity"}
		}
	}
	candidate := cloneSimulationAnalysis(*analysis)
	for index, binding := range bindings {
		member := event
		member.Target = binding.Target
		member.Initial = &initial[index]
		member.Applied = applied[index]
		if recovered != nil {
			member.Recovered = &recovered[index]
		} else {
			member.Recovered = nil
		}
		memberApplied, diagnostic := applyCurrentEventBinding(&candidate, plan, member, binding)
		if diagnostic != nil {
			return false, diagnostic
		}
		if !memberApplied {
			return false, nil
		}
	}
	*analysis = candidate
	return true, nil
}

func applyCurrentEventBinding(analysis *simmodel.Analysis, plan simmodel.Plan, event PlannedEvent, binding SimulationOperatingBinding) (bool, *Diagnostic) {
	if sourceExcitationIndex(*analysis, binding.Component) >= 0 {
		initial := sourceExcitationDCValue(*analysis, binding.Component)
		if event.Initial != nil {
			var ok bool
			initial, ok = physicalLoadCurrent(binding, *event.Initial)
			if !ok {
				return false, &Diagnostic{Path: event.ID, Message: "initial current event is below the catalog-backed parallel support load"}
			}
		}
		applied, ok := physicalLoadCurrent(binding, event.Applied)
		if !ok {
			return false, &Diagnostic{Path: event.ID, Message: "applied current event is below the catalog-backed parallel support load"}
		}
		recovered, ok := physicalLoadCurrentOptional(binding, event.Recovered)
		if !ok {
			return false, &Diagnostic{Path: event.ID, Message: "recovered current event is below the catalog-backed parallel support load"}
		}
		return appendSourceValueEvent(analysis, event, binding.Component, initial, applied, recovered)
	}
	if binding.Kind != OperatingLoadCurrent || binding.Scale <= 0 {
		return false, nil
	}
	scale, ok := resolvedLoadResistanceScale(*analysis, binding)
	if !ok {
		return false, &Diagnostic{Path: event.ID, Message: "current event requires a positive resolved reference-source voltage"}
	}
	initialCurrent := 0.0
	if event.Initial != nil {
		initialCurrent = *event.Initial
	}
	initialCurrent, ok = physicalLoadCurrent(binding, initialCurrent)
	if !ok {
		return false, &Diagnostic{Path: event.ID, Message: "initial current event is below the catalog-backed parallel support load"}
	}
	appliedCurrent, ok := physicalLoadCurrent(binding, event.Applied)
	if !ok {
		return false, &Diagnostic{Path: event.ID, Message: "applied current event is below the catalog-backed parallel support load"}
	}
	recoveredCurrent, ok := physicalLoadCurrentOptional(binding, event.Recovered)
	if !ok {
		return false, &Diagnostic{Path: event.ID, Message: "recovered current event is below the catalog-backed parallel support load"}
	}
	initial, ok := equivalentLoadResistance(scale, initialCurrent)
	if !ok {
		return false, &Diagnostic{Path: event.ID, Message: "current event cannot be represented by the resolved equivalent physical load"}
	}
	applied, ok := equivalentLoadResistance(scale, appliedCurrent)
	if !ok {
		return false, &Diagnostic{Path: event.ID, Message: "applied current event exceeds the resolved equivalent physical-load range"}
	}
	recovered, ok := equivalentLoadResistanceOptional(scale, recoveredCurrent)
	if !ok {
		return false, &Diagnostic{Path: event.ID, Message: "recovered current event exceeds the resolved equivalent physical-load range"}
	}
	return appendDeviceValueEvent(analysis, plan, event, binding.Component, initial, applied, recovered)
}

func resolvedLoadCurrent(analysis simmodel.Analysis, plan simmodel.Plan, binding SimulationOperatingBinding) (float64, bool) {
	if sourceExcitationIndex(analysis, binding.Component) >= 0 {
		current := semanticLoadCurrent(binding, sourceExcitationDCValue(analysis, binding.Component))
		return current, finite(current) && current >= 0
	}
	if binding.Kind != OperatingLoadCurrent || binding.Scale <= 0 {
		return 0, false
	}
	scale, ok := resolvedLoadResistanceScale(analysis, binding)
	if !ok {
		return 0, false
	}
	resistance := resolvedAnalysisDeviceValue(analysis, plan, binding.Component)
	if resistance <= 0 {
		return 0, false
	}
	current := semanticLoadCurrent(binding, scale/resistance)
	return current, finite(current) && current >= 0
}

func physicalLoadCurrent(binding SimulationOperatingBinding, semantic float64) (float64, bool) {
	if !finite(semantic) || !finite(binding.Offset) {
		return 0, false
	}
	physical := semantic + binding.Offset
	if physical < 0 && physical >= -1e-12*math.Max(1, math.Abs(semantic)) {
		physical = 0
	}
	return physical, physical >= 0 && finite(physical)
}

func physicalLoadCurrentOptional(binding SimulationOperatingBinding, semantic *float64) (*float64, bool) {
	if semantic == nil {
		return nil, true
	}
	physical, ok := physicalLoadCurrent(binding, *semantic)
	if !ok {
		return nil, false
	}
	return &physical, true
}

func semanticLoadCurrent(binding SimulationOperatingBinding, physical float64) float64 {
	return physical - binding.Offset
}

func resolvedLoadResistanceScale(analysis simmodel.Analysis, binding SimulationOperatingBinding) (float64, bool) {
	scale := binding.Scale
	if binding.ReferenceComponent != "" {
		scale = math.Abs(sourceExcitationDCValue(analysis, binding.ReferenceComponent))
	}
	return scale, finite(scale) && scale > 0
}

func eventLoadBindings(bindings []SimulationOperatingBinding) []SimulationOperatingBinding {
	var candidates []SimulationOperatingBinding
	for _, binding := range bindings {
		if binding.Axis == "load_current" && binding.Component != "" {
			candidates = append(candidates, binding)
		}
	}
	slices.SortStableFunc(candidates, func(left, right SimulationOperatingBinding) int {
		if order := strings.Compare(left.Target, right.Target); order != 0 {
			return order
		}
		return strings.Compare(left.Component, right.Component)
	})
	return candidates
}

func maximumOperatingAssignmentForCase(plan AnalysisPlan, operatingCase, axis, target string) (float64, bool) {
	maximum := 0.0
	found := false
	for _, corner := range plan.Corners {
		if corner.OperatingCase != operatingCase {
			continue
		}
		for _, assignment := range corner.Assignments {
			if assignment.Axis != axis || assignment.Target != target || assignment.Value == nil ||
				!finite(*assignment.Value) || *assignment.Value <= 0 {
				continue
			}
			maximum = math.Max(maximum, *assignment.Value)
			found = true
		}
	}
	return maximum, found
}

func allocateAggregateCurrent(total float64, capacities []float64) ([]float64, bool) {
	if !finite(total) || total < 0 || len(capacities) == 0 {
		return nil, false
	}
	result := make([]float64, len(capacities))
	active := make([]int, len(capacities))
	for index, capacity := range capacities {
		if !finite(capacity) || capacity <= 0 {
			return nil, false
		}
		active[index] = index
	}
	remaining := total
	for len(active) != 0 && remaining > 1e-12 {
		share := remaining / float64(len(active))
		next := active[:0]
		distributed := 0.0
		for _, index := range active {
			available := capacities[index] - result[index]
			value := math.Min(share, available)
			if value > 0 {
				result[index] += value
				distributed += value
			}
			if capacities[index]-result[index] > 1e-12 {
				next = append(next, index)
			}
		}
		remaining -= distributed
		if distributed <= 1e-12 {
			break
		}
		active = next
	}
	return result, remaining <= math.Max(1, total)*1e-12
}

func appendSourceValueEvent(analysis *simmodel.Analysis, event PlannedEvent, component string, initial, applied float64, recovered *float64) (bool, *Diagnostic) {
	index := sourceExcitationIndex(*analysis, component)
	if index < 0 {
		return false, nil
	}
	excitation := analysis.Excitations[index]
	if excitation.PulsePeriodS != 0 || excitation.SineFrequencyHz != 0 {
		excitation.DCValue = initial
		excitation.SineAmplitude = 0
		excitation.SineFrequencyHz = 0
		excitation.PulseInitialValue = 0
		excitation.PulseValue = 0
		excitation.PulseDelayS = 0
		excitation.PulseWidthS = 0
		excitation.PulsePeriodS = 0
		analysis.Excitations[index] = excitation
	}
	if !finite(initial) || !finite(applied) || recovered != nil && !finite(*recovered) {
		return false, &Diagnostic{Path: event.ID, Message: "source event levels must be finite"}
	}
	extendAnalysisForEvent(analysis, event)
	analysis.SourceValueEvents = append(analysis.SourceValueEvents, simmodel.SourceValueEvent{
		ID: compiledEventID(event.ID, component), Component: component,
		TriggerTimeS: event.TriggerTimeS, DurationS: event.DurationS,
		Initial: initial, Applied: applied, Recovered: recovered,
	})
	sortSourceValueEvents(analysis.SourceValueEvents)
	return true, nil
}

func appendDeviceValueEvent(analysis *simmodel.Analysis, plan simmodel.Plan, event PlannedEvent, component string, initial, applied float64, recovered *float64) (bool, *Diagnostic) {
	if resolvedAnalysisDeviceValue(*analysis, plan, component) <= 0 {
		return false, nil
	}
	if !finite(initial) || !finite(applied) || initial <= 0 || applied <= 0 || recovered != nil && (!finite(*recovered) || *recovered <= 0) {
		return false, &Diagnostic{Path: event.ID, Message: "device event values must be finite and positive"}
	}
	extendAnalysisForEvent(analysis, event)
	analysis.DeviceValueEvents = append(analysis.DeviceValueEvents, simmodel.DeviceValueEvent{
		ID: compiledEventID(event.ID, component), Component: component,
		TriggerTimeS: event.TriggerTimeS, DurationS: event.DurationS,
		InitialSI: initial, AppliedSI: applied, RecoveredSI: recovered,
	})
	sortDeviceValueEvents(analysis.DeviceValueEvents)
	return true, nil
}

func truncateOperatingLoadPreludes(analysis *simmodel.Analysis) {
	if analysis == nil || len(analysis.DeviceValueEvents) == 0 {
		return
	}
	earliestDeclared := map[string]float64{}
	preludeIDs := map[string]string{}
	for _, event := range analysis.DeviceValueEvents {
		preludeID, exists := preludeIDs[event.Component]
		if !exists {
			preludeID = compiledEventID("operating_load_current", event.Component)
			preludeIDs[event.Component] = preludeID
		}
		if event.ID == preludeID {
			continue
		}
		if trigger, exists := earliestDeclared[event.Component]; !exists || event.TriggerTimeS < trigger {
			earliestDeclared[event.Component] = event.TriggerTimeS
		}
	}
	retained := analysis.DeviceValueEvents[:0]
	for _, event := range analysis.DeviceValueEvents {
		trigger, exists := earliestDeclared[event.Component]
		if !exists || event.ID != preludeIDs[event.Component] {
			retained = append(retained, event)
			continue
		}
		tolerance := math.Max(analysis.TimeStepS, math.Abs(trigger)) * 1e-12
		if event.TriggerTimeS+event.DurationS <= trigger+tolerance {
			retained = append(retained, event)
			continue
		}
		duration := trigger - event.TriggerTimeS
		if duration <= tolerance {
			continue
		}
		event.DurationS = duration
		retained = append(retained, event)
	}
	analysis.DeviceValueEvents = retained
	sortDeviceValueEvents(analysis.DeviceValueEvents)
}

func extendAnalysisForEvent(analysis *simmodel.Analysis, event PlannedEvent) {
	required := event.TriggerTimeS + event.DurationS
	if required > analysis.DurationS {
		analysis.DurationS = required
	}
}

func eventSourceForTarget(plan simmodel.Plan, bindings []SimulationOperatingBinding, target string) (string, bool) {
	var candidates []string
	for _, binding := range bindings {
		if binding.Target == target && binding.Kind == OperatingSourceDCValue && binding.Component != "" {
			if _, ok := resolvedVoltageSourcePolarity(plan, binding.Component, target); !ok {
				continue
			}
			candidates = append(candidates, binding.Component)
		}
	}
	if len(candidates) == 0 {
		component := OperatingHarnessComponentID("voltage_event", target)
		if resolvedVoltageSourceComponent(plan, component) {
			candidates = append(candidates, component)
		}
	}
	if len(candidates) == 0 {
		if component, ok := uniqueVoltageSourceComponent(plan, target); ok {
			candidates = append(candidates, component)
		}
	}
	slices.Sort(candidates)
	return uniqueString(slices.Compact(candidates))
}

func resolvedVoltageSourceComponent(plan simmodel.Plan, component string) bool {
	for _, device := range plan.Devices {
		if device.Component == component && device.PrimitiveModel == simmodel.PrimitiveVoltageSourceV1 {
			return true
		}
	}
	return false
}

func eventLoadBinding(bindings []SimulationOperatingBinding, target string) (SimulationOperatingBinding, bool) {
	var candidates []SimulationOperatingBinding
	for _, binding := range bindings {
		if binding.Target == target && (binding.Axis == "load_current" || binding.Axis == "load_resistance") {
			candidates = append(candidates, binding)
		}
	}
	slices.SortStableFunc(candidates, func(left, right SimulationOperatingBinding) int {
		if order := strings.Compare(left.Axis, right.Axis); order != 0 {
			return order
		}
		return strings.Compare(left.Component, right.Component)
	})
	if len(candidates) != 1 {
		return SimulationOperatingBinding{}, false
	}
	return candidates[0], true
}

func eventResistanceComponent(plan simmodel.Plan, bindings []SimulationOperatingBinding, target string) (string, bool) {
	if binding, ok := eventLoadBinding(bindings, target); ok {
		if binding.Axis == "load_resistance" {
			return binding.Component, true
		}
		for _, device := range plan.Devices {
			if device.Component == binding.Component && device.Family == "resistor" {
				return binding.Component, true
			}
		}
	}
	return uniqueLoadComponent(plan, target)
}

func eventSupplyComponents(bindings []SimulationOperatingBinding) []string {
	var components []string
	for _, binding := range bindings {
		if (binding.Axis == "supply_voltage" || binding.Axis == eventSupplyAxis) &&
			binding.Kind == OperatingSourceDCValue && binding.Component != "" {
			components = append(components, binding.Component)
		}
	}
	slices.Sort(components)
	return slices.Compact(components)
}

func sourceExcitationIndex(analysis simmodel.Analysis, component string) int {
	return slices.IndexFunc(analysis.Excitations, func(excitation simmodel.SourceExcitation) bool {
		return excitation.Component == component
	})
}

func sourceExcitationDCValue(analysis simmodel.Analysis, component string) float64 {
	if index := sourceExcitationIndex(analysis, component); index >= 0 {
		return analysis.Excitations[index].DCValue
	}
	return 0
}

func resolvedAnalysisDeviceValue(analysis simmodel.Analysis, plan simmodel.Plan, component string) float64 {
	for _, override := range analysis.DeviceOverrides {
		if override.Component == component && override.ValueSI != nil {
			return *override.ValueSI
		}
	}
	for _, device := range plan.Devices {
		if device.Component == component && device.ValueSI != nil {
			return *device.ValueSI
		}
	}
	return 0
}

func equivalentLoadResistance(scale, current float64) (float64, bool) {
	if !finite(scale) || scale <= 0 || !finite(current) || current < 0 {
		return 0, false
	}
	if current == 0 {
		return maxCompiledAssertionBound, true
	}
	value := scale / current
	return value, finite(value) && value > 0 && value <= maxCompiledAssertionBound
}

func equivalentLoadResistanceOptional(scale float64, current *float64) (*float64, bool) {
	if current == nil {
		return nil, true
	}
	value, ok := equivalentLoadResistance(scale, *current)
	if !ok {
		return nil, false
	}
	return &value, true
}

func positiveReciprocalEventValue(value *float64, fallback float64) (float64, bool) {
	if value == nil {
		return positiveReciprocal(fallback)
	}
	return positiveReciprocal(*value)
}

func positiveReciprocal(value float64) (float64, bool) {
	if !finite(value) || value <= 0 {
		return 0, false
	}
	result := 1 / value
	return result, finite(result) && result >= 1 && result <= 10
}

func reciprocalOptional(value *float64) (*float64, bool) {
	if value == nil {
		return nil, true
	}
	result, ok := positiveReciprocal(*value)
	if !ok {
		return nil, false
	}
	return &result, true
}

func cloneOptionalFloat(value *float64) *float64 {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

func scaledOptional(value *float64, scale float64) *float64 {
	if value == nil {
		return nil
	}
	scaled := *value * scale
	return &scaled
}

func compiledEventID(eventID, target string) string {
	prefix := canonicalID(eventID)
	if len(prefix) > 32 {
		prefix = prefix[:32]
	}
	return prefix + "_" + hashJSON([]byte(eventID + "\x00" + target))[:16]
}

func sortSourceValueEvents(events []simmodel.SourceValueEvent) {
	slices.SortStableFunc(events, func(left, right simmodel.SourceValueEvent) int {
		return simmodel.CompareValueEventOrder(left.Component, left.TriggerTimeS, left.ID, right.Component, right.TriggerTimeS, right.ID)
	})
}

func sortDeviceValueEvents(events []simmodel.DeviceValueEvent) {
	slices.SortStableFunc(events, func(left, right simmodel.DeviceValueEvent) int {
		return simmodel.CompareValueEventOrder(left.Component, left.TriggerTimeS, left.ID, right.Component, right.TriggerTimeS, right.ID)
	})
}

func sortConditionValueEvents(events []simmodel.ConditionValueEvent) {
	slices.SortStableFunc(events, func(left, right simmodel.ConditionValueEvent) int {
		return simmodel.CompareValueEventOrder(left.Name, left.TriggerTimeS, left.ID, right.Name, right.TriggerTimeS, right.ID)
	})
}

func planHasAutonomousTransientDevice(plan simmodel.Plan) bool {
	return slices.ContainsFunc(plan.Devices, func(device simmodel.ResolvedDevice) bool {
		return device.PrimitiveModel == simmodel.PrimitiveFixedClockSourceV1 ||
			device.PrimitiveModel == simmodel.PrimitiveResistorProgrammedClockSourceV1
	})
}

func validOperatingBinding(binding SimulationOperatingBinding) bool {
	switch binding.Kind {
	case OperatingSourceDCValue, OperatingSourceFrequencyHz, OperatingDeviceValueSI, OperatingGeneratedControl:
		return binding.Component != "" && binding.ReferenceComponent == "" && binding.Parameter == "" && binding.Scale == 0 && binding.Offset == 0
	case OperatingLoadCurrent:
		return binding.Component != "" && binding.Parameter == "" && finite(binding.Scale) && binding.Scale >= 0 && finite(binding.Offset)
	case OperatingModelParameter:
		return binding.Component != "" && binding.ReferenceComponent == "" && binding.Parameter != "" && binding.Scale == 0 && binding.Offset == 0
	case OperatingAnalysisCondition:
		return binding.Component == "" && binding.ReferenceComponent == "" && binding.Parameter != "" && binding.Scale == 0 && binding.Offset == 0
	case OperatingWorstCase:
		return binding.Component == "" && binding.ReferenceComponent == "" && binding.Parameter == "" && binding.Scale == 0 && binding.Offset == 0
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
	case OperatingSourceDCValue, OperatingLoadCurrent:
		if assignment.Value == nil {
			return &Diagnostic{Path: binding.Axis, Message: "source operating binding requires a numeric corner"}
		}
		physicalValue := *assignment.Value
		if binding.Kind == OperatingLoadCurrent {
			var ok bool
			physicalValue, ok = physicalLoadCurrent(binding, *assignment.Value)
			if !ok {
				return &Diagnostic{Path: binding.Axis, Message: "load-current corner is below the catalog-backed parallel support load"}
			}
		}
		for index := range analysis.Excitations {
			if analysis.Excitations[index].Component == binding.Component {
				if binding.Axis == "load_current" {
					if delay, width, period, ok := operatingPulseWindow(*analysis, binding.Component); ok {
						analysis.Excitations[index].DCValue = 0
						analysis.Excitations[index].PulseInitialValue = 0
						analysis.Excitations[index].PulseValue = physicalValue
						analysis.Excitations[index].PulseDelayS = delay
						analysis.Excitations[index].PulseWidthS = width
						analysis.Excitations[index].PulsePeriodS = period
						return nil
					}
				}
				if analysis.Excitations[index].PulsePeriodS != 0 {
					analysis.Excitations[index].PulseValue = physicalValue
				} else {
					analysis.Excitations[index].DCValue = physicalValue
				}
				return nil
			}
		}
		if binding.Kind == OperatingLoadCurrent {
			for _, device := range plan.Devices {
				if device.Component != binding.Component || device.Family != "resistor" || device.ValueSI == nil {
					continue
				}
				if physicalValue < 0 || !finite(physicalValue) || binding.Scale <= 0 {
					return &Diagnostic{Path: binding.Axis, Message: "equivalent load-current resistance requires a finite nonnegative current and positive resolved voltage scale"}
				}
				scale := binding.Scale
				if binding.ReferenceComponent != "" {
					reference := math.Abs(sourceExcitationDCValue(*analysis, binding.ReferenceComponent))
					if !finite(reference) || reference <= 0 {
						return &Diagnostic{Path: binding.Axis, Message: "equivalent load-current resistance requires a positive resolved reference-source voltage"}
					}
					scale = reference
				}
				resistance := maxCompiledAssertionBound
				if physicalValue > 0 {
					resistance = scale / physicalValue
				}
				if !finite(resistance) || resistance <= 0 || resistance > maxCompiledAssertionBound {
					return &Diagnostic{Path: binding.Axis, Message: "equivalent load-current resistance exceeds the trusted numeric range"}
				}
				override := analysisDeviceOverride(analysis, binding.Component)
				if (analysis.Kind == simmodel.AnalysisTransient || analysis.Kind == simmodel.AnalysisElectrothermal) &&
					analysis.TimeStepS > 0 && analysis.DurationS > analysis.TimeStepS &&
					resistance < maxCompiledAssertionBound {
					initial := maxCompiledAssertionBound
					override.ValueSI = &initial
					analysis.DeviceValueEvents = append(analysis.DeviceValueEvents, simmodel.DeviceValueEvent{
						ID:           compiledEventID("operating_load_current", binding.Component),
						Component:    binding.Component,
						TriggerTimeS: analysis.TimeStepS,
						DurationS:    analysis.DurationS - analysis.TimeStepS,
						InitialSI:    initial,
						AppliedSI:    resistance,
					})
					sortDeviceValueEvents(analysis.DeviceValueEvents)
				} else {
					override.ValueSI = &resistance
				}
				setAnalysisDeviceOverride(analysis, override)
				return nil
			}
		}
		return &Diagnostic{Path: binding.Axis, Message: "source operating binding references a source absent from the trusted template"}
	case OperatingSourceFrequencyHz:
		if assignment.Value == nil || !finite(*assignment.Value) || *assignment.Value <= 0 {
			return &Diagnostic{Path: binding.Axis, Message: "source-frequency operating binding requires a positive finite numeric corner"}
		}
		for index := range analysis.Excitations {
			if analysis.Excitations[index].Component == binding.Component {
				if analysis.Excitations[index].SineAmplitude == 0 {
					return nil
				}
				if (analysis.Kind == simmodel.AnalysisDistortion || analysis.Kind == simmodel.AnalysisTransient) &&
					!retimePeriodicSineGrid(analysis, analysis.Excitations[index].SineFrequencyHz, *assignment.Value) {
					return &Diagnostic{Path: binding.Axis, Message: "source-frequency corner cannot preserve the trusted exact periodic grid"}
				}
				analysis.Excitations[index].SineFrequencyHz = *assignment.Value
				return nil
			}
		}
		return &Diagnostic{Path: binding.Axis, Message: "source-frequency operating binding references a source absent from the trusted template"}
	case OperatingDeviceValueSI:
		if assignment.Value == nil || !finite(*assignment.Value) || *assignment.Value < 0 || (*assignment.Value == 0 && !deviceValueAllowsZero(*plan, binding.Component)) {
			return &Diagnostic{Path: binding.Axis, Message: "device-value operating binding requires a finite positive corner, except that a capacitor may use zero to represent an absent load"}
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
		if binding.Parameter == "ambient_temperature_c" {
			junctionTemperatureK := *assignment.Value + 273.15
			if !finite(junctionTemperatureK) || junctionTemperatureK <= 0 {
				return &Diagnostic{Path: binding.Axis, Message: "ambient-temperature operating binding must remain above absolute zero"}
			}
			for _, device := range plan.Devices {
				if !hasNamedValue(device.ModelParameters, "junction_temperature_k") {
					continue
				}
				override := analysisDeviceOverride(analysis, device.Component)
				override.ModelParameters = setNamedValue(override.ModelParameters, "junction_temperature_k", junctionTemperatureK)
				setAnalysisDeviceOverride(analysis, override)
			}
		}
		return nil
	default:
		return &Diagnostic{Path: binding.Axis, Message: "unsupported operating binding kind"}
	}
}

func retimePeriodicSineGrid(analysis *simmodel.Analysis, previousFrequency, frequency float64) bool {
	if analysis == nil || !finite(previousFrequency) || previousFrequency <= 0 ||
		!finite(frequency) || frequency <= 0 || !finite(analysis.TimeStepS) ||
		analysis.TimeStepS <= 0 || !finite(analysis.DurationS) || analysis.DurationS <= 0 {
		return false
	}
	samplesPerCycle := math.Round(1 / (previousFrequency * analysis.TimeStepS))
	cycles := math.Round(previousFrequency * analysis.DurationS)
	if samplesPerCycle < 16 || cycles < 4 {
		return false
	}
	analysis.TimeStepS = 1 / (frequency * samplesPerCycle)
	analysis.DurationS = cycles / frequency
	return finite(analysis.TimeStepS) && finite(analysis.DurationS)
}

func hasNamedValue(values []simmodel.NamedValue, name string) bool {
	for _, value := range values {
		if value.Name == name {
			return true
		}
	}
	return false
}

func deviceValueAllowsZero(plan simmodel.Plan, component string) bool {
	for _, device := range plan.Devices {
		if device.Component == component {
			return device.Family == "capacitor"
		}
	}
	return false
}

// operatingPulseWindow makes an ideal external current load inactive while a
// pulsed supply is off. Without this coupling, a nonzero load corner forces a
// fictitious powered steady state at time zero and creates capacitor-discharge
// impulses when the real supply and regulator soft-start begin.
func operatingPulseWindow(analysis simmodel.Analysis, excludedComponent string) (float64, float64, float64, bool) {
	for _, excitation := range analysis.Excitations {
		if excitation.Component == excludedComponent || excitation.PulsePeriodS <= 0 || excitation.PulseWidthS <= 0 {
			continue
		}
		return excitation.PulseDelayS, excitation.PulseWidthS, excitation.PulsePeriodS, true
	}
	return 0, 0, 0, false
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

func compiledAnalysisID(kind, operatingCase, corner, discriminator string, index int) string {
	prefix := canonicalID(kind + "_" + operatingCase)
	hash := hashJSON(struct {
		Corner        string `json:"corner"`
		Discriminator string `json:"discriminator,omitempty"`
		Index         int    `json:"index"`
	}{corner, discriminator, index})
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
	return fmt.Sprintf("%s\x00%s\x00%s\x00%s\x00%s\x00%s\x00%024.12e\x00%024.12e\x00%024.12e\x00%024.12e", assertion.AnalysisID, assertion.Node, assertion.Component, strings.Join(assertion.Components, "\x1f"), assertion.ReferenceNode, assertion.Quantity, assertion.FrequencyHz, assertion.TimeS, assertion.WindowStartS, assertion.WindowEndS)
}

func plannedAssertionEvent(events []PlannedEvent, assertion PlannedAssertion) (PlannedEvent, bool) {
	const prefix = "event:"
	if !strings.HasPrefix(assertion.Target, prefix) {
		return PlannedEvent{}, false
	}
	eventID := strings.TrimPrefix(assertion.Target, prefix)
	for _, event := range events {
		if event.OperatingCase == assertion.OperatingCase && event.ID == eventID {
			return event, true
		}
	}
	return PlannedEvent{}, false
}

func applyExcitationOverrides(analysis *simmodel.Analysis, overrides []SimulationExcitationOverride) *Diagnostic {
	for _, override := range overrides {
		found := false
		for index := range analysis.Excitations {
			if analysis.Excitations[index].Component != override.Component {
				continue
			}
			excitation := &analysis.Excitations[index]
			excitation.DCValue = override.DCValue
			excitation.ACMagnitude = 0
			excitation.ACPhaseDeg = 0
			excitation.PulseInitialValue = 0
			excitation.PulseValue = 0
			excitation.PulseDelayS = 0
			excitation.PulseWidthS = 0
			excitation.PulsePeriodS = 0
			excitation.SineAmplitude = 0
			excitation.SineFrequencyHz = 0
			excitation.SinePhaseDeg = 0
			found = true
			break
		}
		if !found {
			return &Diagnostic{Path: "excitation_overrides." + override.Component, Message: "metric-specific excitation override references a source absent from the resolved analysis"}
		}
	}
	return nil
}

func analysisExecutionKey(analysis simmodel.Analysis) (string, error) {
	analysis.ID = ""
	encoded, err := json.Marshal(analysis)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func cloneSimulationAnalysis(source simmodel.Analysis) simmodel.Analysis {
	encoded, _ := json.Marshal(source)
	var clone simmodel.Analysis
	_ = json.Unmarshal(encoded, &clone)
	return clone
}
