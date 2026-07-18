package amplifiers

import (
	"fmt"
	"sort"

	"kicadai/internal/blocks"
)

type LayoutProfile string

const (
	LayoutProfileClassALine       LayoutProfile = "class_a_line"
	LayoutProfileClassABHeadphone LayoutProfile = "class_ab_headphone"
	LayoutProfileOpAmpFeedback    LayoutProfile = "opamp_feedback"
)

type LayoutPolicyEvidence struct {
	Profile           LayoutProfile                  `json:"profile"`
	PresentCategories []blocks.PCBConstraintCategory `json:"present_categories,omitempty"`
	MissingCategories []blocks.PCBConstraintCategory `json:"missing_categories,omitempty"`
	InvalidCategories []blocks.PCBConstraintCategory `json:"invalid_categories,omitempty"`
	Blockers          []string                       `json:"blockers,omitempty"`
}

func (e LayoutPolicyEvidence) OK() bool {
	return len(e.MissingCategories) == 0 && len(e.InvalidCategories) == 0 && len(e.Blockers) == 0
}

func ValidateLayoutPolicy(profile LayoutProfile, realizations ...*blocks.PCBRealization) LayoutPolicyEvidence {
	required := layoutProfileCategories(profile)
	found := map[blocks.PCBConstraintCategory]bool{}
	invalid := map[blocks.PCBConstraintCategory]bool{}
	for _, realization := range realizations {
		if realization == nil {
			continue
		}
		for _, constraint := range realization.Constraints {
			if constraint.Category == "" {
				continue
			}
			found[constraint.Category] = true
			if !layoutConstraintQuantified(constraint) {
				invalid[constraint.Category] = true
			}
		}
	}
	evidence := LayoutPolicyEvidence{Profile: profile}
	for category := range found {
		evidence.PresentCategories = append(evidence.PresentCategories, category)
	}
	for _, category := range required {
		if !found[category] {
			evidence.MissingCategories = append(evidence.MissingCategories, category)
			evidence.Blockers = append(evidence.Blockers, fmt.Sprintf("missing amplifier layout category %s", category))
		}
		if invalid[category] {
			evidence.InvalidCategories = append(evidence.InvalidCategories, category)
			evidence.Blockers = append(evidence.Blockers, fmt.Sprintf("amplifier layout category %s lacks the required quantitative or topology contract", category))
		}
	}
	sort.Slice(evidence.PresentCategories, func(i, j int) bool { return evidence.PresentCategories[i] < evidence.PresentCategories[j] })
	sort.Slice(evidence.MissingCategories, func(i, j int) bool { return evidence.MissingCategories[i] < evidence.MissingCategories[j] })
	sort.Slice(evidence.InvalidCategories, func(i, j int) bool { return evidence.InvalidCategories[i] < evidence.InvalidCategories[j] })
	sort.Strings(evidence.Blockers)
	return evidence
}

func layoutProfileCategories(profile LayoutProfile) []blocks.PCBConstraintCategory {
	switch profile {
	case LayoutProfileClassALine:
		return []blocks.PCBConstraintCategory{blocks.PCBConstraintAnalogInputSeparation, blocks.PCBConstraintReturnTopology, blocks.PCBConstraintDecouplingProximity, blocks.PCBConstraintThermalKeepout}
	case LayoutProfileClassABHeadphone:
		return []blocks.PCBConstraintCategory{blocks.PCBConstraintReturnTopology, blocks.PCBConstraintCurrentPath, blocks.PCBConstraintThermalCoupling, blocks.PCBConstraintDeviceSymmetry}
	case LayoutProfileOpAmpFeedback:
		return []blocks.PCBConstraintCategory{blocks.PCBConstraintAnalogInputSeparation, blocks.PCBConstraintFeedbackSense, blocks.PCBConstraintDecouplingProximity, blocks.PCBConstraintCurrentPath, blocks.PCBConstraintThermalKeepout}
	default:
		return nil
	}
}

func layoutConstraintQuantified(constraint blocks.PCBConstraint) bool {
	switch constraint.Category {
	case blocks.PCBConstraintAnalogInputSeparation, blocks.PCBConstraintThermalKeepout:
		return constraint.ClearanceMM > 0 && len(constraint.AppliesTo) >= 2
	case blocks.PCBConstraintDecouplingProximity, blocks.PCBConstraintFeedbackSense, blocks.PCBConstraintThermalCoupling, blocks.PCBConstraintDeviceSymmetry, blocks.PCBConstraintKelvinSense:
		return constraint.MaxLengthMM > 0 && len(constraint.AppliesTo) >= 2
	case blocks.PCBConstraintCurrentPath:
		return constraint.MinWidthMM > 0 && constraint.NetTemplate != ""
	case blocks.PCBConstraintReturnTopology:
		return constraint.NetTemplate != "" && len(constraint.AppliesTo) >= 2
	case blocks.PCBConstraintPolarizedOrientation:
		return len(constraint.AppliesTo) > 0
	default:
		return false
	}
}
