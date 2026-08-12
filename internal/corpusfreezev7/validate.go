package corpusfreezev7

import (
	"bytes"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"kicadai/internal/corpusfreeze"
	"kicadai/internal/corpusfreezev6"
	ots "kicadai/internal/opentopologysynthesis"
)

const (
	minimumSafetyCategoryTotal = 6
	maximumSafetyCategoryTotal = 12
	maximumPrimaryAnalysis     = 12
	maximumPrimaryMetric       = 9
	maximumInterfaceShape      = 6
)

func Validate(
	assignments map[string][]byte,
	bundles map[string]corpusfreeze.Bundle,
	binding corpusfreeze.Binding,
	historical HistoricalCommitments,
	policy corpusfreeze.Policy,
) (corpusfreeze.Report, error) {
	report, err := corpusfreezev6.Validate(assignments, bundles, binding, historical, policy)
	if err != nil {
		return corpusfreeze.Report{}, err
	}
	if err := validateV7Aggregate(report, bundles, policy); err != nil {
		return corpusfreeze.Report{}, err
	}
	return report, nil
}

func validateV7Aggregate(report corpusfreeze.Report, bundles map[string]corpusfreeze.Bundle, policy corpusfreeze.Policy) error {
	safetyCounts := make(map[string]int, len(policy.SafetyImpacts))
	primaryAnalyses, primaryMetrics, interfaceShapes := map[string]int{}, map[string]int{}, map[string]int{}
	behaviorSignatures := map[string]string{}
	for _, entry := range report.Entries {
		bundle, exists := bundles[entry.AuthorSlot]
		if !exists {
			return fmt.Errorf("V7_AGGREGATE_BUNDLE_MISSING: %s", entry.AuthorSlot)
		}
		data, exists := bundle.Requirements[entry.RequirementFile]
		if !exists {
			return fmt.Errorf("V7_AGGREGATE_REQUIREMENT_MISSING: %s", entry.RequirementFile)
		}
		requirement, issues := ots.DecodeStrict(bytes.NewReader(data))
		if len(issues) != 0 {
			return fmt.Errorf("V7_AGGREGATE_STRICT_DECODE: %s", entry.RequirementFile)
		}
		safetyCounts[entry.SafetyImpact]++
		primary, err := primaryBehavioralRequirement(requirement)
		if err != nil {
			return err
		}
		primaryAnalyses[primary.Analysis]++
		primaryMetrics[primary.Metric]++
		shape, err := interfaceShape(requirement)
		if err != nil {
			return err
		}
		interfaceShapes[shape]++
		signature, err := behaviorSignature(requirement, shape)
		if err != nil {
			return err
		}
		if prior := behaviorSignatures[signature]; prior != "" {
			return fmt.Errorf("V7_NEAR_DUPLICATE_SIGNATURE: %s and %s", prior, entry.RequirementFile)
		}
		behaviorSignatures[signature] = entry.RequirementFile
	}
	for _, category := range policy.SafetyImpacts {
		count := safetyCounts[category]
		if count < minimumSafetyCategoryTotal || count > maximumSafetyCategoryTotal {
			return fmt.Errorf("V7_SAFETY_CATEGORY_TOTAL: %s=%d want %d..%d", category, count, minimumSafetyCategoryTotal, maximumSafetyCategoryTotal)
		}
	}
	if key, count := firstLimitViolation(primaryAnalyses, maximumPrimaryAnalysis); key != "" {
		return fmt.Errorf("V7_PRIMARY_ANALYSIS_LIMIT: %s=%d want <=%d", key, count, maximumPrimaryAnalysis)
	}
	if key, count := firstLimitViolation(primaryMetrics, maximumPrimaryMetric); key != "" {
		return fmt.Errorf("V7_PRIMARY_METRIC_LIMIT: %s=%d want <=%d", key, count, maximumPrimaryMetric)
	}
	if key, count := firstLimitViolation(interfaceShapes, maximumInterfaceShape); key != "" {
		return fmt.Errorf("V7_INTERFACE_SHAPE_LIMIT: %s=%d want <=%d", key, count, maximumInterfaceShape)
	}
	return nil
}

func primaryBehavioralRequirement(requirement ots.Requirement) (ots.BehavioralAssertion, error) {
	assertions := requirement.Requirements.BehavioralRequirements
	if len(assertions) == 0 {
		return ots.BehavioralAssertion{}, fmt.Errorf("V7_PRIMARY_REQUIREMENT_EMPTY")
	}
	primary := assertions[0]
	for _, assertion := range assertions[1:] {
		// Go string order is unsigned UTF-8 byte order, exactly as frozen by V7.
		if assertion.ID < primary.ID {
			primary = assertion
		}
	}
	return primary, nil
}

func interfaceShape(requirement ots.Requirement) (string, error) {
	portsByDomain := make(map[string][]string, len(requirement.Requirements.Domains))
	domainIDs := make(map[string]bool, len(requirement.Requirements.Domains))
	for _, domain := range requirement.Requirements.Domains {
		domainIDs[domain.ID] = true
	}
	for _, port := range requirement.Requirements.Ports {
		if !domainIDs[port.Domain] {
			return "", fmt.Errorf("V7_INTERFACE_DOMAIN_REFERENCE: %s", port.Domain)
		}
		electrical := port.Electrical
		portsByDomain[port.Domain] = append(portsByDomain[port.Domain], portValueShape(port, electrical))
	}
	domainGroups := make([]string, 0, len(requirement.Requirements.Domains))
	for _, domain := range requirement.Requirements.Domains {
		ports := portsByDomain[domain.ID]
		slices.Sort(ports)
		domainGroups = append(domainGroups, domainValueShape(domain)+":["+strings.Join(ports, ",")+"]")
	}
	slices.Sort(domainGroups)
	return strings.Join(domainGroups, "|"), nil
}

func domainValueShape(domain ots.Domain) string {
	return strings.Join([]string{
		domain.Kind, floatPointerKey(domain.MinVoltageV), floatPointerKey(domain.NominalVoltageV),
		floatPointerKey(domain.MaxVoltageV), floatPointerKey(domain.MaxCurrentA), domain.Source,
	}, ":")
}

func portValueShape(port ots.Port, electrical ots.Electrical) string {
	return strings.Join([]string{
		port.Kind, port.Direction, floatPointerKey(electrical.MinVoltageV), floatPointerKey(electrical.NominalVoltageV),
		floatPointerKey(electrical.MaxVoltageV), floatPointerKey(electrical.MaxCurrentA),
		floatPointerKey(electrical.InputImpedanceMinOhm), electrical.DefaultState,
	}, ":")
}

func floatPointerKey(value *float64) string {
	if value == nil {
		return "-"
	}
	normalized := *value
	if normalized == 0 {
		normalized = 0 // Canonicalize IEEE-754 negative zero without changing nonzero signs.
	}
	return strconv.FormatFloat(normalized, 'g', -1, 64)
}

type targetShapeIndex struct {
	ports   map[string]string
	domains map[string]string
}

func behaviorSignature(requirement ots.Requirement, shape string) (string, error) {
	targets := buildTargetShapeIndex(requirement)
	conditions, events := []string{}, []string{}
	for _, operatingCase := range requirement.Requirements.OperatingCases {
		for _, condition := range operatingCase.Conditions {
			target, err := targets.any(condition.Target)
			if err != nil {
				return "", err
			}
			conditions = append(conditions, strings.Join([]string{condition.Axis, target,
				floatKey(condition.Min), floatKey(condition.Max), condition.Unit}, ":"))
		}
		for _, event := range operatingCase.Events {
			target, err := targets.port(event.Target)
			if err != nil {
				return "", err
			}
			events = append(events, strings.Join([]string{event.Kind, target,
				floatKey(event.TriggerTimeS), floatKey(event.Initial), floatKey(event.Applied), event.Unit}, ":"))
		}
	}
	assertions := make([]string, 0, len(requirement.Requirements.BehavioralRequirements))
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		excitationKind := "none"
		if assertion.Excitation != nil {
			var err error
			excitationKind, err = targets.observation(*assertion.Excitation)
			if err != nil {
				return "", err
			}
		}
		observation, err := targets.observation(assertion.Observation)
		if err != nil {
			return "", err
		}
		assertions = append(assertions, strings.Join([]string{assertion.Analysis, assertion.Metric, assertion.Unit,
			excitationKind + ">" + observation, floatPointerKey(assertion.Min),
			floatPointerKey(assertion.Max), floatPointerKey(assertion.FrequencyHz)}, ":"))
	}
	slices.Sort(conditions)
	slices.Sort(events)
	slices.Sort(assertions)
	return shape + "|conditions=" + strings.Join(conditions, ",") +
		"|events=" + strings.Join(events, ",") + "|assertions=" + strings.Join(assertions, ","), nil
}

func buildTargetShapeIndex(requirement ots.Requirement) targetShapeIndex {
	result := targetShapeIndex{ports: map[string]string{}, domains: map[string]string{}}
	for _, port := range requirement.Requirements.Ports {
		result.ports[port.ID] = "port:" + portValueShape(port, port.Electrical)
	}
	for _, domain := range requirement.Requirements.Domains {
		result.domains[domain.ID] = "domain:" + domainValueShape(domain)
	}
	return result
}

func (targets targetShapeIndex) observation(observation ots.Observation) (string, error) {
	if observation.Kind == "circuit" {
		return "circuit", nil
	}
	if observation.Kind == "port" {
		return targets.port(observation.ID)
	}
	if observation.Kind == "domain" {
		if shape := targets.domains[observation.ID]; shape != "" {
			return shape, nil
		}
		return "", fmt.Errorf("V7_SIGNATURE_TARGET_UNRESOLVED: domain:%s", observation.ID)
	}
	return "", fmt.Errorf("V7_SIGNATURE_TARGET_KIND: %s", observation.Kind)
}

func (targets targetShapeIndex) port(id string) (string, error) {
	if shape := targets.ports[id]; shape != "" {
		return shape, nil
	}
	return "", fmt.Errorf("V7_SIGNATURE_TARGET_UNRESOLVED: port:%s", id)
}

func (targets targetShapeIndex) any(id string) (string, error) {
	port, portExists := targets.ports[id]
	domain, domainExists := targets.domains[id]
	if portExists == domainExists {
		return "", fmt.Errorf("V7_SIGNATURE_TARGET_AMBIGUOUS_OR_UNRESOLVED: %s", id)
	}
	if portExists {
		return port, nil
	}
	return domain, nil
}

func floatKey(value float64) string {
	return floatPointerKey(&value)
}

func firstLimitViolation(values map[string]int, maximum int) (string, int) {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	for _, key := range keys {
		if values[key] > maximum {
			return key, values[key]
		}
	}
	return "", 0
}
