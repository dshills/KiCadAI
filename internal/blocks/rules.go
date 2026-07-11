package blocks

import (
	"fmt"
	"slices"
	"strings"

	"kicadai/internal/components"
	"kicadai/internal/reports"
)

type RuleKind string

const (
	RuleKindElectrical RuleKind = "electrical"
	RuleKindPCB        RuleKind = "pcb"
	RuleKindEvidence   RuleKind = "evidence"
)

type RuleSeverity string

const (
	RuleSeverityInfo    RuleSeverity = "info"
	RuleSeverityWarning RuleSeverity = "warning"
	RuleSeverityBlocker RuleSeverity = "blocker"
)

type RuleOutcome struct {
	ID         string       `json:"id"`
	Kind       RuleKind     `json:"kind"`
	Severity   RuleSeverity `json:"severity"`
	Path       string       `json:"path,omitempty"`
	Message    string       `json:"message"`
	Refs       []string     `json:"refs,omitempty"`
	Nets       []string     `json:"nets,omitempty"`
	Suggestion string       `json:"suggestion,omitempty"`
}

type RuleReport struct {
	BlockID   string         `json:"block_id"`
	Readiness BlockReadiness `json:"readiness"`
	Outcomes  []RuleOutcome  `json:"outcomes,omitempty"`
}

func (report RuleReport) Blocking() bool {
	for _, outcome := range report.Outcomes {
		if outcome.Severity == RuleSeverityBlocker {
			return true
		}
	}
	return false
}

func (report RuleReport) Issues() []reports.Issue {
	issues := make([]reports.Issue, 0, len(report.Outcomes))
	for _, outcome := range report.Outcomes {
		issues = append(issues, outcome.Issue())
	}
	return issues
}

func (outcome RuleOutcome) Issue() reports.Issue {
	severity := reports.SeverityInfo
	switch outcome.Severity {
	case RuleSeverityWarning:
		severity = reports.SeverityWarning
	case RuleSeverityBlocker:
		severity = reports.SeverityBlocked
	}
	return reports.Issue{
		Code:       reports.CodeValidationFailed,
		Severity:   severity,
		Path:       outcome.Path,
		Message:    outcome.Message,
		Refs:       slices.Clone(outcome.Refs),
		Nets:       slices.Clone(outcome.Nets),
		Suggestion: outcome.Suggestion,
	}
}

type RuleContext struct {
	Definition     BlockDefinition
	Acceptance     components.AcceptanceLevel
	Selections     []BlockComponentSelection
	Realization    *BlockPCBRealizationResult
	RequiredPorts  []string
	RequiredNets   []string
	RequiredRoutes []string
	RequiredRoles  []string
}

func EvaluateCommonRules(ctx RuleContext) RuleReport {
	report := RuleReport{BlockID: ctx.Definition.ID, Readiness: BlockReadinessReady}
	report.Outcomes = append(report.Outcomes, requireComponentRoles(ctx.Definition, ctx.RequiredRoles)...)
	report.Outcomes = append(report.Outcomes, requirePorts(ctx.Definition, ctx.RequiredPorts)...)
	report.Outcomes = append(report.Outcomes, requireNets(ctx.Definition, ctx.RequiredNets)...)
	report.Outcomes = append(report.Outcomes, requireComponentEvidence(ctx)...)
	report.Outcomes = append(report.Outcomes, requireLocalRoutes(ctx)...)
	sortRuleOutcomes(report.Outcomes)
	switch {
	case hasRuleSeverity(report.Outcomes, RuleSeverityBlocker):
		report.Readiness = BlockReadinessUnsupported
	case hasRuleSeverity(report.Outcomes, RuleSeverityWarning):
		report.Readiness = BlockReadinessPartial
	default:
		report.Readiness = BlockReadinessReady
	}
	return report
}

func requireComponentRoles(definition BlockDefinition, required []string) []RuleOutcome {
	if len(required) == 0 {
		return nil
	}
	available := map[string]struct{}{}
	for _, component := range definition.Components {
		available[component.Role] = struct{}{}
	}
	var outcomes []RuleOutcome
	for _, role := range sortedUniqueStrings(required) {
		if _, ok := available[role]; ok {
			continue
		}
		outcomes = append(outcomes, ruleBlocker("required_role."+role, RuleKindElectrical, "block."+definition.ID+".components", "required component role "+role+" is missing"))
	}
	return outcomes
}

func requirePorts(definition BlockDefinition, required []string) []RuleOutcome {
	if len(required) == 0 {
		return nil
	}
	available := map[string]struct{}{}
	for _, port := range definition.Ports {
		available[port.Name] = struct{}{}
	}
	var outcomes []RuleOutcome
	for _, port := range sortedUniqueStrings(required) {
		if _, ok := available[port]; ok {
			continue
		}
		outcomes = append(outcomes, ruleBlocker("required_port."+port, RuleKindElectrical, "block."+definition.ID+".ports", "required port "+port+" is missing"))
	}
	return outcomes
}

func requireNets(definition BlockDefinition, required []string) []RuleOutcome {
	if len(required) == 0 {
		return nil
	}
	available := map[string]struct{}{}
	for _, net := range definition.Nets {
		available[net.NameTemplate] = struct{}{}
		available[net.Role] = struct{}{}
	}
	var outcomes []RuleOutcome
	for _, net := range sortedUniqueStrings(required) {
		if _, ok := available[net]; ok {
			continue
		}
		outcomes = append(outcomes, ruleBlocker("required_net."+net, RuleKindElectrical, "block."+definition.ID+".nets", "required net "+net+" is missing"))
	}
	return outcomes
}

func requireComponentEvidence(ctx RuleContext) []RuleOutcome {
	if len(ctx.Selections) == 0 {
		if acceptanceRequiresComponentEvidence(ctx.Acceptance) {
			return []RuleOutcome{ruleBlocker("component_evidence.none", RuleKindEvidence, "block."+ctx.Definition.ID+".components", "component evidence is required for "+string(ctx.Acceptance)+" acceptance")}
		}
		return nil
	}
	selectionByRole := map[string]BlockComponentSelection{}
	for _, selection := range ctx.Selections {
		selectionByRole[selection.Role] = selection
	}
	var outcomes []RuleOutcome
	for _, component := range ctx.Definition.Components {
		if component.ComponentID == "" && component.ComponentIDParam == "" && component.ComponentQuery == nil {
			continue
		}
		selection, ok := selectionByRole[component.Role]
		if !ok {
			if component.ComponentIDParam != "" && component.ComponentID == "" && component.ComponentQuery == nil {
				continue
			}
			outcomes = append(outcomes, ruleBlocker("component_evidence.missing."+component.Role, RuleKindEvidence, "block."+ctx.Definition.ID+".components."+component.Role, "component role "+component.Role+" has no selected part evidence"))
			continue
		}
		confidence := selection.Selection.Candidate.Confidence
		if !components.AcceptanceAllows(ctx.Acceptance, confidence) {
			outcomes = append(outcomes, ruleBlocker("component_evidence.unsafe."+component.Role, RuleKindEvidence, "block."+ctx.Definition.ID+".components."+component.Role, fmt.Sprintf("component confidence %s is not allowed for %s acceptance", confidence, ctx.Acceptance)))
		}
		if component.PinmapRequired && !selection.Selection.Component.Verification.PinMapChecked && selection.Selection.Variant.PinMapID == "" {
			outcomes = append(outcomes, ruleBlocker("component_evidence.pinmap."+component.Role, RuleKindEvidence, "block."+ctx.Definition.ID+".components."+component.Role, "component role "+component.Role+" requires pinmap evidence"))
		}
		if selection.Selection.Candidate.FootprintID == "" {
			outcomes = append(outcomes, ruleBlocker("component_evidence.footprint."+component.Role, RuleKindEvidence, "block."+ctx.Definition.ID+".components."+component.Role, "component role "+component.Role+" has no footprint evidence"))
		}
	}
	return outcomes
}

func acceptanceRequiresComponentEvidence(acceptance components.AcceptanceLevel) bool {
	switch acceptance {
	case components.AcceptanceConnectivity, components.AcceptanceERCDRC, components.AcceptanceFabricationCandidate:
		return true
	default:
		return false
	}
}

func requireLocalRoutes(ctx RuleContext) []RuleOutcome {
	if len(ctx.RequiredRoutes) == 0 {
		return nil
	}
	if ctx.Realization == nil {
		return []RuleOutcome{ruleBlocker("pcb_routes.unrealized", RuleKindPCB, "block."+ctx.Definition.ID+".pcb_realization", "PCB realization is required to prove local routes")}
	}
	routes := map[string]struct{}{}
	for _, route := range ctx.Realization.LocalRoutes {
		routes[route.ID] = struct{}{}
	}
	var outcomes []RuleOutcome
	for _, route := range sortedUniqueStrings(ctx.RequiredRoutes) {
		if _, ok := routes[route]; ok {
			continue
		}
		outcomes = append(outcomes, ruleBlocker("pcb_routes.missing."+route, RuleKindPCB, "block."+ctx.Definition.ID+".pcb_realization.local_routes", "required local route "+route+" was not realized"))
	}
	return outcomes
}

func ruleBlocker(id string, kind RuleKind, path string, message string) RuleOutcome {
	return RuleOutcome{ID: id, Kind: kind, Severity: RuleSeverityBlocker, Path: path, Message: message}
}

func hasRuleSeverity(outcomes []RuleOutcome, severity RuleSeverity) bool {
	for _, outcome := range outcomes {
		if outcome.Severity == severity {
			return true
		}
	}
	return false
}

func sortRuleOutcomes(outcomes []RuleOutcome) {
	slices.SortFunc(outcomes, func(a, b RuleOutcome) int {
		if a.Severity != b.Severity {
			return ruleSeverityRank(a.Severity) - ruleSeverityRank(b.Severity)
		}
		if a.Kind != b.Kind {
			return strings.Compare(string(a.Kind), string(b.Kind))
		}
		if a.Path != b.Path {
			return strings.Compare(a.Path, b.Path)
		}
		return strings.Compare(a.ID, b.ID)
	})
}

func ruleSeverityRank(severity RuleSeverity) int {
	switch severity {
	case RuleSeverityBlocker:
		return 0
	case RuleSeverityWarning:
		return 1
	case RuleSeverityInfo:
		return 2
	default:
		return 3
	}
}
