package routing

import (
	"math"
	"strings"

	"kicadai/internal/pcbrules"
	"kicadai/internal/reports"
)

type clearanceObjectKind uint8

const (
	clearanceObjectTrace clearanceObjectKind = iota
	clearanceObjectVia
)

type clearancePolicy struct {
	request  Request
	rules    pcbrules.RuleSet
	resolver *pcbrules.Resolver
	byNet    map[string]pcbrules.EffectiveRule
	issues   []reports.Issue
}

func newClearancePolicy(request Request) *clearancePolicy {
	rules := toPCBRules(request.Rules, request.Strategy)
	policy := &clearancePolicy{
		request:  request,
		rules:    rules,
		resolver: pcbrules.NewResolver(rules),
		byNet:    make(map[string]pcbrules.EffectiveRule, len(request.Nets)+1),
	}
	for _, net := range request.Nets {
		policy.resolve(net)
	}
	policy.resolve(Net{})
	return policy
}

func clearanceNetKey(netName string) string {
	// Routing net identity is intentionally case-insensitive. Validate uses
	// the same canonical key and rejects case-only duplicate declarations.
	return normalizeKey(netName)
}

func (policy *clearancePolicy) resolve(net Net) pcbrules.EffectiveRule {
	key := clearanceNetKey(net.Name)
	if rule, ok := policy.byNet[key]; ok {
		return rule
	}
	rule, issues := ResolveNetRuleWithResolver(policy.resolver, net)
	policy.issues = append(policy.issues, issues...)
	if reports.HasBlockingIssue(issues) {
		// Invalid routing rules must never weaken geometry. Normal request
		// validation reports the precise issue; the policy fails closed until
		// that validation can be surfaced by the caller.
		failClosedMM := max(1, 2*math.Hypot(policy.request.Board.WidthMM, policy.request.Board.HeightMM))
		rule.ClearanceMM = failClosedMM
		rule.ViaClearanceMM = failClosedMM
	}
	policy.byNet[key] = rule
	return rule
}

func (policy *clearancePolicy) ruleForNet(netName string) pcbrules.EffectiveRule {
	key := clearanceNetKey(netName)
	if rule, ok := policy.byNet[key]; ok {
		return rule
	}
	return policy.resolve(Net{Name: strings.TrimSpace(netName)})
}

func (policy *clearancePolicy) pair(leftNet string, leftKind clearanceObjectKind, rightNet string, rightKind clearanceObjectKind) float64 {
	left := policy.ruleForNet(leftNet)
	right := policy.ruleForNet(rightNet)
	fallback := max(left.ClearanceMM, right.ClearanceMM)
	clearanceMM := pcbrules.ClearanceBetween(
		policy.rules,
		left.ClassName,
		right.ClassName,
		fallback,
	)
	if leftKind == clearanceObjectVia {
		clearanceMM = max(clearanceMM, left.ViaClearanceMM)
	}
	if rightKind == clearanceObjectVia {
		clearanceMM = max(clearanceMM, right.ViaClearanceMM)
	}
	return clearanceMM
}

func (policy *clearancePolicy) object(netName string, kind clearanceObjectKind) float64 {
	rule := policy.ruleForNet(netName)
	if kind == clearanceObjectVia {
		return max(rule.ClearanceMM, rule.ViaClearanceMM)
	}
	return rule.ClearanceMM
}

func (policy *clearancePolicy) obstacle(netName string, kind clearanceObjectKind, obstacle Obstacle) float64 {
	return max(policy.object(netName, kind), obstacle.Clearance)
}

func (policy *clearancePolicy) pad(netName string, kind clearanceObjectKind, pad Pad) float64 {
	clearanceMM := policy.pair(netName, kind, pad.Net, clearanceObjectTrace)
	if pad.Clearance != nil {
		clearanceMM = max(clearanceMM, *pad.Clearance)
	}
	return clearanceMM
}

func existingCopperKind(kind CopperKind) clearanceObjectKind {
	if kind == CopperVia {
		return clearanceObjectVia
	}
	return clearanceObjectTrace
}

func (policy *clearancePolicy) maximumClearance() float64 {
	maximum := max(policy.request.Rules.ClearanceMM, policy.request.Rules.ViaClearanceMM)
	for _, clearanceMM := range policy.request.Rules.ClearanceMatrix {
		maximum = max(maximum, clearanceMM)
	}
	for _, rule := range policy.byNet {
		maximum = max(maximum, rule.ClearanceMM, rule.ViaClearanceMM)
	}
	return maximum
}

func (policy *clearancePolicy) firstBlockingIssue() (reports.Issue, bool) {
	for _, issue := range policy.issues {
		if issue.Blocking() {
			return issue, true
		}
	}
	return reports.Issue{}, false
}
