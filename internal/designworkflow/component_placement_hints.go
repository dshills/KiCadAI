package designworkflow

import (
	"fmt"
	"hash/fnv"
	"io"
	"strconv"
	"strings"

	"kicadai/internal/placement"
)

const defaultComponentHintNearDistanceMM = 5.0
const defaultComponentHintProximityWeight = 4

var placementRuleIDReplacer = strings.NewReplacer(
	"component_hint:", "",
	":", ".",
	"%00", "none",
	"%3A", "_",
	"%25", "pct",
	" ", "_",
)

type componentPlacementHintResult struct {
	Rules    []placement.ProximityRule
	Evidence []ComponentHintEvidence
}

func componentPlacementHintRules(selections []ComponentSelectionEntry, fragments PCBFragmentResult) componentPlacementHintResult {
	hints := NormalizeComponentHints(selections)
	if len(hints) == 0 {
		return componentPlacementHintResult{}
	}
	fragmentByInstance := map[string]*BlockFragment{}
	for index := range fragments.Fragments {
		fragment := &fragments.Fragments[index]
		fragmentByInstance[fragment.InstanceID] = fragment
	}
	result := componentPlacementHintResult{Evidence: make([]ComponentHintEvidence, 0, len(hints))}
	for _, hint := range hints {
		if hint.Type != ComponentHintPlacement {
			result.Evidence = append(result.Evidence, hint)
			continue
		}
		evidence := hint
		switch {
		case hint.Status == ComponentHintUnsupported:
			result.Evidence = append(result.Evidence, evidence)
			continue
		case hint.Kind != "near":
			evidence.Status = ComponentHintSkipped
			evidence.Message = "placement hint is not consumed by the current placement model"
			result.Evidence = append(result.Evidence, evidence)
			continue
		}
		fragment, ok := fragmentByInstance[hint.InstanceID]
		if !ok {
			evidence.Status = ComponentHintSkipped
			evidence.Message = "placement hint skipped because block instance was not realized"
			result.Evidence = append(result.Evidence, evidence)
			continue
		}
		anchorRef := strings.TrimSpace(fragment.Realization.RoleRefs[hint.Role])
		targetRef := strings.TrimSpace(fragment.Realization.RoleRefs[hint.Target])
		if anchorRef == "" || targetRef == "" {
			evidence.Status = ComponentHintSkipped
			evidence.Message = fmt.Sprintf("placement hint skipped because role refs were missing for %q or %q", hint.Role, hint.Target)
			result.Evidence = append(result.Evidence, evidence)
			continue
		}
		maxDistance, ok, message := componentHintDistanceMM(hint)
		if !ok {
			evidence.Status = ComponentHintSkipped
			evidence.Message = message
			result.Evidence = append(result.Evidence, evidence)
			continue
		}
		result.Rules = append(result.Rules, placement.ProximityRule{
			ID:            "component_hint." + stablePlacementRuleID(hint.ID),
			Source:        "component_hint",
			Role:          componentHintPlacementIntentRole(hint),
			AnchorRef:     anchorRef,
			TargetRefs:    []string{targetRef},
			MaxDistanceMM: maxDistance,
			Weight:        defaultComponentHintProximityWeight,
			Required:      false,
		})
		evidence.Status = ComponentHintEnforced
		evidence.Message = "placement hint enforced as a proximity rule"
		result.Evidence = append(result.Evidence, evidence)
	}
	return result
}

func componentHintDistanceMM(hint ComponentHintEvidence) (float64, bool, string) {
	valueText := strings.TrimSpace(hint.Value)
	unit := strings.ToLower(strings.TrimSpace(hint.Unit))
	if unit != "" && unit != "mm" {
		return 0, false, "placement hint skipped because distance unit is not supported"
	}
	if valueText == "" {
		return defaultComponentHintNearDistanceMM, true, ""
	}
	value, err := strconv.ParseFloat(valueText, 64)
	if err != nil {
		return 0, false, "placement hint skipped because distance value is not numeric"
	}
	if value < 0 {
		return 0, false, "placement hint skipped because distance value is negative"
	}
	return value, true, ""
}

func componentHintPlacementIntentRole(hint ComponentHintEvidence) placement.IntentRole {
	text := normalizeRoleName(hint.Role + " " + hint.Target)
	switch {
	case containsToken(text, "clock"), containsToken(text, "crystal"), containsToken(text, "oscillator"):
		return placement.IntentClock
	case containsToken(text, "feedback"):
		return placement.IntentFeedback
	case containsToken(text, "connector"), containsToken(text, "jack"), containsToken(text, "header"), containsToken(text, "port"):
		return placement.IntentConnector
	case containsToken(text, "thermal"), containsToken(text, "heat"):
		return placement.IntentThermal
	case containsToken(text, "reset"):
		return placement.IntentReset
	case containsToken(text, "programming"), containsToken(text, "debug"):
		return placement.IntentProgramming
	case containsToken(text, "power"), containsToken(text, "regulator"), containsToken(text, "supply"):
		return placement.IntentPowerPath
	default:
		return placement.IntentDecoupling
	}
}

func stablePlacementRuleID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "empty"
	}
	hash := fnv.New64a()
	_, _ = io.WriteString(hash, value)
	readable := placementRuleIDReplacer.Replace(value)
	readableRunes := []rune(readable)
	if len(readableRunes) > 80 {
		readable = string(readableRunes[:80])
	}
	return fmt.Sprintf("%s.%016x", readable, hash.Sum64())
}
