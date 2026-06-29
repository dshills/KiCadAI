package checks

import (
	"math"
	"strings"
)

// KiCad reports malformed generated symbol and wire endpoints near the origin
// when fallback geometry is unresolved; report coordinates are normalized to mm.
const ercNearOriginThresholdMM = 1.0

type ERCFailureSource string

const (
	ERCFailureSourceWriterSemantics ERCFailureSource = "writer_semantics"
	ERCFailureSourceElectrical      ERCFailureSource = "electrical_design"
	ERCFailureSourceUnknown         ERCFailureSource = "unknown"
)

type ERCFailureStage string

const (
	ERCFailureStageSymbolResolution      ERCFailureStage = "symbol_resolution"
	ERCFailureStageSchematicConnectivity ERCFailureStage = "schematic_connectivity"
	ERCFailureStagePowerPolicy           ERCFailureStage = "power_policy"
	ERCFailureStageDesignIntent          ERCFailureStage = "design_intent"
	ERCFailureStageUnknown               ERCFailureStage = "unknown"
)

type ERCFindingClassification struct {
	ID             string           `json:"id,omitempty"`
	Rule           string           `json:"rule,omitempty"`
	Code           string           `json:"code,omitempty"`
	NormalizedRule string           `json:"normalized_rule,omitempty"`
	RepairCategory RepairCategory   `json:"repair_category,omitempty"`
	Source         ERCFailureSource `json:"source"`
	Stage          ERCFailureStage  `json:"stage"`
	Signature      string           `json:"signature,omitempty"`
	Reason         string           `json:"reason,omitempty"`
}

type ERCClassificationSummary struct {
	Total    int                        `json:"total"`
	BySource map[ERCFailureSource]int   `json:"by_source"`
	ByStage  map[ERCFailureStage]int    `json:"by_stage"`
	ByRule   map[string]int             `json:"by_rule"`
	Items    []ERCFindingClassification `json:"items,omitempty"`
}

func ClassifyERCFinding(finding CheckFinding) ERCFindingClassification {
	classification := ERCFindingClassification{
		ID:             finding.ID,
		Rule:           finding.Rule,
		Code:           finding.Code,
		RepairCategory: finding.RepairCategory,
		Source:         ERCFailureSourceUnknown,
		Stage:          ERCFailureStageUnknown,
	}
	if finding.Kind != "" && finding.Kind != CheckKindERC {
		classification.Reason = "finding is not an ERC finding"
		return classification
	}
	rule := normalizedFindingRule(finding)
	classification.NormalizedRule = rule
	classification.Signature = ercFindingSignature(finding, rule)
	if isNearOriginFinding(finding) {
		switch rule {
		case "pin_not_connected":
			classification.Source = ERCFailureSourceWriterSemantics
			classification.Stage = ERCFailureStageSymbolResolution
			classification.Reason = "near-origin disconnected pin points to unresolved or malformed generated symbol pin anchors"
			return classification
		case "wire_dangling", "unconnected_wire_endpoint", "label_dangling":
			classification.Source = ERCFailureSourceWriterSemantics
			classification.Stage = ERCFailureStageSchematicConnectivity
			classification.Reason = "near-origin dangling connection points to generated wire endpoints missing KiCad pin anchors"
			return classification
		}
	}
	switch rule {
	case "pin_not_connected", "wire_dangling", "unconnected_wire_endpoint", "label_dangling":
		classification.Source = ERCFailureSourceElectrical
		classification.Stage = ERCFailureStageSchematicConnectivity
		classification.Reason = "ERC reports an ordinary schematic connectivity failure"
	case "power_pin_not_driven":
		classification.Source = ERCFailureSourceElectrical
		classification.Stage = ERCFailureStagePowerPolicy
		classification.Reason = "power input needs a KiCad-recognized driven source or explicit power policy"
	case "no_connect_connected", "pin_to_pin":
		classification.Source = ERCFailureSourceElectrical
		classification.Stage = ERCFailureStageDesignIntent
		classification.Reason = "ERC reports a design-intent conflict"
	}
	return classification
}

func ClassifyERCFindings(findings []CheckFinding) ERCClassificationSummary {
	summary := ERCClassificationSummary{
		BySource: map[ERCFailureSource]int{},
		ByStage:  map[ERCFailureStage]int{},
		ByRule:   map[string]int{},
		Items:    make([]ERCFindingClassification, 0, len(findings)),
	}
	for _, finding := range findings {
		if finding.Kind != "" && finding.Kind != CheckKindERC {
			continue
		}
		item := ClassifyERCFinding(finding)
		summary.Items = append(summary.Items, item)
		summary.Total++
		summary.BySource[item.Source]++
		summary.ByStage[item.Stage]++
		if item.NormalizedRule != "" {
			summary.ByRule[item.NormalizedRule]++
		}
	}
	return summary
}

func normalizedFindingRule(finding CheckFinding) string {
	return normalizeKey(firstNonEmpty(finding.Rule, finding.Code))
}

func isNearOriginFinding(finding CheckFinding) bool {
	if finding.Location == nil {
		return false
	}
	return math.Abs(finding.Location.X) <= ercNearOriginThresholdMM && math.Abs(finding.Location.Y) <= ercNearOriginThresholdMM
}

func ercFindingSignature(finding CheckFinding, normalizedRule string) string {
	parts := []string{
		normalizedRule,
		string(finding.RepairCategory),
		normalizeLocation(finding.Location),
	}
	objectTypes := make([]string, 0, len(finding.Objects))
	for _, object := range finding.Objects {
		if object.Type != "" {
			objectTypes = append(objectTypes, normalizeKey(object.Type))
		}
	}
	if len(objectTypes) > 0 {
		parts = append(parts, strings.Join(sortedStrings(objectTypes), ","))
	}
	return strings.Join(parts, "|")
}
