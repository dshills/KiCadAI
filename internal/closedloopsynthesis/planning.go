package closedloopsynthesis

import (
	"fmt"
	"math"
	"slices"
	"strings"

	"kicadai/internal/architecturesearch"
	"kicadai/internal/simmodel"
)

const (
	AnalysisPlanSchema = "kicadai.behavioral-analysis-plan.v1"
	maxPlannedCorners  = 256
)

type SemanticBinding struct {
	Kind   string `json:"kind"`
	ID     string `json:"id"`
	Target string `json:"target"`
}

type AnalysisPlan struct {
	Schema          string             `json:"schema"`
	RequirementHash string             `json:"requirement_hash"`
	PlanHash        string             `json:"plan_hash"`
	Bindings        []SemanticBinding  `json:"bindings"`
	Analyses        []PlannedAnalysis  `json:"analyses"`
	Corners         []PlannedCorner    `json:"corners"`
	Events          []PlannedEvent     `json:"events,omitempty"`
	Assertions      []PlannedAssertion `json:"assertions"`
	ModelDecisions  []ModelDecision    `json:"model_decisions"`
}

type PlannedAnalysis struct {
	ID            string   `json:"id"`
	Kind          string   `json:"kind"`
	OperatingCase string   `json:"operating_case"`
	Requirements  []string `json:"requirements"`
	Events        []string `json:"events,omitempty"`
}

type PlannedEvent struct {
	ID            string   `json:"id"`
	Kind          string   `json:"kind"`
	OperatingCase string   `json:"operating_case"`
	Target        string   `json:"target"`
	TriggerTimeS  float64  `json:"trigger_time_s"`
	DurationS     float64  `json:"duration_s"`
	Initial       *float64 `json:"initial,omitempty"`
	Applied       float64  `json:"applied"`
	Recovered     *float64 `json:"recovered,omitempty"`
	Unit          string   `json:"unit"`
}

type PlannedCorner struct {
	ID            string             `json:"id"`
	OperatingCase string             `json:"operating_case"`
	Assignments   []CornerAssignment `json:"assignments"`
}

type CornerAssignment struct {
	Axis      string   `json:"axis"`
	Target    string   `json:"target"`
	Value     *float64 `json:"value,omitempty"`
	Unit      string   `json:"unit,omitempty"`
	Selection string   `json:"selection,omitempty"`
}

type PlannedAssertion struct {
	RequirementID     string                   `json:"requirement_id"`
	AnalysisID        string                   `json:"analysis_id"`
	OperatingCase     string                   `json:"operating_case"`
	Metric            string                   `json:"metric"`
	Target            string                   `json:"target"`
	Min               *float64                 `json:"min,omitempty"`
	Max               *float64                 `json:"max,omitempty"`
	Unit              string                   `json:"unit"`
	Critical          bool                     `json:"critical"`
	TransitionID      string                   `json:"transition_id,omitempty"`
	ResponseDirection string                   `json:"response_direction,omitempty"`
	Dependencies      []PlannedStateDependency `json:"dependencies,omitempty"`
}

type PlannedStateDependency struct {
	Observation architecturesearch.Observation `json:"observation"`
	Target      string                         `json:"target"`
	State       string                         `json:"state"`
	StableForS  float64                        `json:"stable_for_s,omitempty"`
}

// BuildAnalysisPlan binds v3/v4/v5/v6 semantic behavior to resolved trusted targets and
// expands bounded named operating corners. It does not accept equations,
// solver settings, or provider model content.
func BuildAnalysisPlan(requirement architecturesearch.Requirement, bindings []SemanticBinding, modelDecisions []ModelDecision) (AnalysisPlan, []Diagnostic) {
	plan := AnalysisPlan{Schema: AnalysisPlanSchema, Bindings: append([]SemanticBinding(nil), bindings...), ModelDecisions: cloneModelDecisions(modelDecisions)}
	requirementHash, err := architecturesearch.CanonicalHash(requirement)
	v3 := requirement.Schema == architecturesearch.SchemaIDV3 && requirement.Version == architecturesearch.VersionV3
	v4 := requirement.Schema == architecturesearch.SchemaIDV4 && requirement.Version == architecturesearch.VersionV4
	v5 := requirement.Schema == architecturesearch.SchemaIDV5 && requirement.Version == architecturesearch.VersionV5
	v6 := requirement.Schema == architecturesearch.SchemaIDV6 && requirement.Version == architecturesearch.VersionV6
	if err != nil || (!v3 && !v4 && !v5 && !v6) {
		message := "analysis planning requires a valid v3, v4, v5, or v6 behavioral requirement"
		if err != nil {
			message = err.Error()
		}
		return plan, []Diagnostic{{Path: "requirement", Message: message}}
	}
	plan.RequirementHash = requirementHash
	for index := range plan.Bindings {
		plan.Bindings[index].Kind = strings.TrimSpace(plan.Bindings[index].Kind)
		plan.Bindings[index].ID = strings.TrimSpace(plan.Bindings[index].ID)
		plan.Bindings[index].Target = strings.TrimSpace(plan.Bindings[index].Target)
	}
	slices.SortStableFunc(plan.Bindings, compareSemanticBindings)
	bindingTargets, diagnostics := validateSemanticBindings(plan.Bindings)

	for _, operatingCase := range requirement.Requirements.OperatingCases {
		corners, cornerDiagnostics := expandOperatingCase(operatingCase, bindingTargets)
		diagnostics = append(diagnostics, cornerDiagnostics...)
		plan.Corners = append(plan.Corners, corners...)
		for eventIndex, event := range operatingCase.Events {
			target, exists := observationTarget(event.Target, bindingTargets)
			if !exists {
				diagnostics = append(diagnostics, Diagnostic{
					Path:    fmt.Sprintf("operating_cases.%s.events[%d].target", operatingCase.ID, eventIndex),
					Message: "resolved operating-event target binding is missing",
				})
				continue
			}
			if event.Applied == nil {
				diagnostics = append(diagnostics, Diagnostic{
					Path:    fmt.Sprintf("operating_cases.%s.events[%d].applied", operatingCase.ID, eventIndex),
					Message: "planned operating event lacks its required applied value",
				})
				continue
			}
			plan.Events = append(plan.Events, PlannedEvent{
				ID: event.ID, Kind: event.Kind, OperatingCase: operatingCase.ID, Target: target,
				TriggerTimeS: event.TriggerTimeS, DurationS: event.DurationS,
				Initial: cloneFloat(event.Initial), Applied: *event.Applied, Recovered: cloneFloat(event.Recovered), Unit: event.Unit,
			})
			bindingTargets["event\x00"+event.ID] = "event:" + event.ID
		}
	}
	slices.SortStableFunc(plan.Events, comparePlannedEvents)

	analysisRequirements := map[string][]string{}
	analysisKinds := map[string]string{}
	analysisCases := map[string]string{}
	analysisEvents := map[string][]string{}
	observedEvents := map[string]bool{}
	caseKinds := map[string]map[string]bool{}
	transitions := make(map[string]architecturesearch.ControlTransition, len(requirement.Requirements.ControlTransitions))
	for _, transition := range requirement.Requirements.ControlTransitions {
		transitions[transition.ID] = transition
	}
	registerAnalysis := func(kind, caseID, eventID string) string {
		analysisID := kind + ":" + caseID
		if eventID != "" {
			analysisID += ":event:" + eventID
			analysisEvents[analysisID] = append(analysisEvents[analysisID], eventID)
		}
		if _, exists := analysisRequirements[analysisID]; !exists {
			analysisRequirements[analysisID] = nil
		}
		analysisKinds[analysisID] = kind
		analysisCases[analysisID] = caseID
		if caseKinds[caseID] == nil {
			caseKinds[caseID] = map[string]bool{}
		}
		caseKinds[caseID][kind] = true
		return analysisID
	}
	for _, behavior := range requirement.Requirements.BehavioralRequirements {
		target, exists := observationTarget(behavior.Observation, bindingTargets)
		if !exists {
			diagnostics = append(diagnostics, Diagnostic{Path: "behavioral_requirements." + behavior.ID + ".observation", Message: "resolved semantic observation binding is missing"})
			continue
		}
		transition := transitions[behavior.Transition]
		dependencies := make([]PlannedStateDependency, 0, len(transition.Dependencies))
		for index, dependency := range transition.Dependencies {
			target, exists := observationTarget(dependency.Target, bindingTargets)
			if !exists {
				diagnostics = append(diagnostics, Diagnostic{
					Path:    fmt.Sprintf("control_transitions.%s.dependencies[%d].target", transition.ID, index),
					Message: "resolved control-state dependency target binding is missing",
				})
				continue
			}
			dependencies = append(dependencies, PlannedStateDependency{
				Observation: dependency.Target, Target: target, State: dependency.State, StableForS: dependency.StableForS,
			})
		}
		for _, caseID := range behavior.OperatingCases {
			eventID := ""
			if behavior.Observation.Kind == "event" {
				eventID = behavior.Observation.ID
				observedEvents[caseID+"\x00"+eventID] = true
			}
			analysisID := registerAnalysis(behavior.Analysis, caseID, eventID)
			analysisRequirements[analysisID] = append(analysisRequirements[analysisID], behavior.ID)
			plan.Assertions = append(plan.Assertions, PlannedAssertion{
				RequirementID: behavior.ID, AnalysisID: analysisID, OperatingCase: caseID, Metric: behavior.Metric,
				Target: target, Min: cloneFloat(behavior.Min), Max: cloneFloat(behavior.Max), Unit: behavior.Unit, Critical: behavior.Critical,
				TransitionID: behavior.Transition, ResponseDirection: transition.Direction, Dependencies: append([]PlannedStateDependency(nil), dependencies...),
			})
		}
	}
	for _, event := range plan.Events {
		if observedEvents[event.OperatingCase+"\x00"+event.ID] {
			continue
		}
		kind := simmodel.AnalysisTransient
		switch event.Kind {
		case "blocked_airflow", "inductive_turn_off", "overload", "short_circuit":
			if caseKinds[event.OperatingCase][simmodel.AnalysisElectrothermal] {
				kind = simmodel.AnalysisElectrothermal
			}
		default:
			if !caseKinds[event.OperatingCase][kind] && caseKinds[event.OperatingCase][simmodel.AnalysisElectrothermal] {
				kind = simmodel.AnalysisElectrothermal
			}
		}
		registerAnalysis(kind, event.OperatingCase, event.ID)
	}
	analysisIDs := make([]string, 0, len(analysisRequirements))
	for analysisID := range analysisRequirements {
		analysisIDs = append(analysisIDs, analysisID)
	}
	slices.Sort(analysisIDs)
	for _, analysisID := range analysisIDs {
		requirements := analysisRequirements[analysisID]
		slices.Sort(requirements)
		events := append([]string(nil), analysisEvents[analysisID]...)
		slices.Sort(events)
		plan.Analyses = append(plan.Analyses, PlannedAnalysis{
			ID: analysisID, Kind: analysisKinds[analysisID], OperatingCase: analysisCases[analysisID],
			Requirements: requirements, Events: events,
		})
	}
	slices.SortStableFunc(plan.Assertions, func(left, right PlannedAssertion) int {
		if order := strings.Compare(left.RequirementID, right.RequirementID); order != 0 {
			return order
		}
		return strings.Compare(left.OperatingCase, right.OperatingCase)
	})

	normalizedDecisions, _, modelDiagnostics := validateModelDecisions(requirement, plan.ModelDecisions)
	plan.ModelDecisions = normalizedDecisions
	diagnostics = append(diagnostics, modelDiagnostics...)
	diagnostics = append(diagnostics, validateModelTemperatureDomains(requirement, plan.ModelDecisions)...)
	slices.SortStableFunc(diagnostics, compareDiagnostics)
	if len(diagnostics) != 0 {
		return plan, diagnostics
	}
	plan.PlanHash = analysisPlanHash(plan)
	if !validHash(plan.PlanHash) {
		return plan, []Diagnostic{{Path: "plan", Message: "behavioral analysis plan could not be canonically hashed"}}
	}
	return plan, nil
}

func validateSemanticBindings(bindings []SemanticBinding) (map[string]string, []Diagnostic) {
	targets := map[string]string{"circuit\x00circuit": "circuit"}
	var diagnostics []Diagnostic
	for index, binding := range bindings {
		path := fmt.Sprintf("bindings[%d]", index)
		if binding.Kind != "port" && binding.Kind != "signal" && binding.Kind != "domain" && binding.Kind != "participant_port" {
			diagnostics = append(diagnostics, Diagnostic{Path: path + ".kind", Message: "semantic binding kind must be port, signal, domain, or participant_port"})
		}
		if binding.ID == "" || binding.Target == "" {
			diagnostics = append(diagnostics, Diagnostic{Path: path, Message: "semantic binding requires identity and resolved target"})
		}
		key := binding.Kind + "\x00" + binding.ID
		if _, duplicate := targets[key]; duplicate {
			diagnostics = append(diagnostics, Diagnostic{Path: path, Message: "semantic binding is duplicated"})
		}
		targets[key] = binding.Target
	}
	return targets, diagnostics
}

func expandOperatingCase(operatingCase architecturesearch.OperatingCase, bindings map[string]string) ([]PlannedCorner, []Diagnostic) {
	assignments := [][]CornerAssignment{{}}
	var diagnostics []Diagnostic
	for conditionIndex, condition := range operatingCase.Conditions {
		path := fmt.Sprintf("operating_cases.%s.conditions[%d]", operatingCase.ID, conditionIndex)
		target, exists := operatingConditionTarget(condition, bindings)
		if !exists {
			diagnostics = append(diagnostics, Diagnostic{Path: path + ".target", Message: "resolved operating-condition binding is missing"})
			continue
		}
		values := conditionAssignments(condition, target)
		if len(values) == 0 {
			diagnostics = append(diagnostics, Diagnostic{Path: path, Message: "operating condition produced no bounded corner assignments"})
			continue
		}
		if len(assignments)*len(values) > maxPlannedCorners {
			diagnostics = append(diagnostics, Diagnostic{Path: "operating_cases." + operatingCase.ID, Message: fmt.Sprintf("corner product exceeds bounded maximum %d", maxPlannedCorners), Suggestion: "partition the operating case into smaller named cases"})
			return nil, diagnostics
		}
		next := make([][]CornerAssignment, 0, len(assignments)*len(values))
		for _, prefix := range assignments {
			for _, value := range values {
				combined := append([]CornerAssignment(nil), prefix...)
				combined = append(combined, value)
				next = append(next, combined)
			}
		}
		assignments = next
	}
	corners := make([]PlannedCorner, 0, len(assignments))
	seen := map[string]bool{}
	for _, assignment := range assignments {
		slices.SortStableFunc(assignment, compareCornerAssignments)
		hash := hashJSON(assignment)
		if !validHash(hash) {
			diagnostics = append(diagnostics, Diagnostic{Path: "operating_cases." + operatingCase.ID, Message: "corner assignment could not be canonically hashed"})
			continue
		}
		if seen[hash] {
			continue
		}
		seen[hash] = true
		corners = append(corners, PlannedCorner{ID: operatingCase.ID + ":" + hash[:16], OperatingCase: operatingCase.ID, Assignments: assignment})
	}
	slices.SortStableFunc(corners, func(left, right PlannedCorner) int { return strings.Compare(left.ID, right.ID) })
	return corners, diagnostics
}

func conditionAssignments(condition architecturesearch.OperatingCondition, target string) []CornerAssignment {
	if condition.Selection != "" {
		selections := []string{condition.Selection}
		if condition.Selection == "all" && condition.Axis != "tolerance" && condition.Axis != "model_parameter" {
			selections = []string{"minimum", "nominal", "maximum"}
		}
		result := make([]CornerAssignment, 0, len(selections))
		for _, selection := range selections {
			result = append(result, CornerAssignment{Axis: condition.Axis, Target: target, Selection: selection})
		}
		return result
	}
	values := []float64{}
	if condition.Min != nil {
		values = append(values, *condition.Min)
	}
	if condition.Max != nil && (condition.Min == nil || *condition.Max != *condition.Min) {
		values = append(values, *condition.Max)
	}
	result := make([]CornerAssignment, 0, len(values))
	for _, value := range values {
		value := value
		result = append(result, CornerAssignment{Axis: condition.Axis, Target: target, Value: &value, Unit: condition.Unit})
	}
	return result
}

func observationTarget(observation architecturesearch.Observation, bindings map[string]string) (string, bool) {
	if observation.Kind == "circuit" && observation.ID == "circuit" {
		return "circuit", true
	}
	target, exists := bindings[observation.Kind+"\x00"+observation.ID]
	return target, exists
}

func operatingConditionTarget(condition architecturesearch.OperatingCondition, bindings map[string]string) (string, bool) {
	if condition.Target == "circuit" {
		return "circuit", true
	}
	if (condition.Axis == "tolerance" || condition.Axis == "model_parameter") &&
		(condition.Target == "all_components" || condition.Target == "all_passives") {
		return condition.Target, true
	}
	if condition.Axis == "supply_voltage" {
		target, exists := bindings["domain\x00"+condition.Target]
		return target, exists
	}
	for _, kind := range []string{"port", "signal", "domain"} {
		if target, exists := bindings[kind+"\x00"+condition.Target]; exists {
			return target, true
		}
	}
	return "", false
}

func validateModelTemperatureDomains(requirement architecturesearch.Requirement, decisions []ModelDecision) []Diagnostic {
	minimum, maximum := math.Inf(1), math.Inf(-1)
	for _, operatingCase := range requirement.Requirements.OperatingCases {
		for _, condition := range operatingCase.Conditions {
			if condition.Axis != "ambient_temperature" {
				continue
			}
			if condition.Min != nil {
				minimum = math.Min(minimum, *condition.Min)
			}
			if condition.Max != nil {
				maximum = math.Max(maximum, *condition.Max)
			}
		}
	}
	if math.IsInf(minimum, 1) && math.IsInf(maximum, -1) {
		return nil
	}
	var diagnostics []Diagnostic
	for index, decision := range decisions {
		if decision.Status != "used" || decision.Provenance == nil {
			continue
		}
		path := fmt.Sprintf("model_decisions[%d].provenance", index)
		identity := fmt.Sprintf("component %q model %q", decision.Component, decision.Claim.ModelID)
		if !math.IsInf(minimum, 1) && (decision.Provenance.MinTemperatureC == nil || *decision.Provenance.MinTemperatureC > minimum) {
			diagnostics = append(diagnostics, Diagnostic{Path: path + ".min_temperature_c", Message: identity + " reviewed temperature domain does not cover the required minimum ambient corner"})
		}
		if !math.IsInf(maximum, -1) && (decision.Provenance.MaxTemperatureC == nil || *decision.Provenance.MaxTemperatureC < maximum) {
			diagnostics = append(diagnostics, Diagnostic{Path: path + ".max_temperature_c", Message: identity + " reviewed temperature domain does not cover the required maximum ambient corner"})
		}
	}
	return diagnostics
}

func analysisPlanHash(plan AnalysisPlan) string {
	copy := plan
	copy.PlanHash = ""
	return hashJSON(copy)
}

func compareSemanticBindings(left, right SemanticBinding) int {
	if order := strings.Compare(left.Kind, right.Kind); order != 0 {
		return order
	}
	if order := strings.Compare(left.ID, right.ID); order != 0 {
		return order
	}
	return strings.Compare(left.Target, right.Target)
}

func compareCornerAssignments(left, right CornerAssignment) int {
	if order := strings.Compare(left.Axis, right.Axis); order != 0 {
		return order
	}
	if order := strings.Compare(left.Target, right.Target); order != 0 {
		return order
	}
	if order := strings.Compare(left.Selection, right.Selection); order != 0 {
		return order
	}
	return strings.Compare(left.Unit, right.Unit)
}

func comparePlannedEvents(left, right PlannedEvent) int {
	if order := strings.Compare(left.OperatingCase, right.OperatingCase); order != 0 {
		return order
	}
	if left.TriggerTimeS < right.TriggerTimeS {
		return -1
	}
	if left.TriggerTimeS > right.TriggerTimeS {
		return 1
	}
	return strings.Compare(left.ID, right.ID)
}
