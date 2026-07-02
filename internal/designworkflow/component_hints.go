package designworkflow

import (
	"slices"
	"strings"
)

type ComponentHintKind string

const (
	ComponentHintPlacement ComponentHintKind = "placement"
	ComponentHintRouting   ComponentHintKind = "routing"
)

type ComponentHintStatus string

const (
	ComponentHintPending     ComponentHintStatus = "pending"
	ComponentHintEnforced    ComponentHintStatus = "enforced"
	ComponentHintSkipped     ComponentHintStatus = "skipped"
	ComponentHintUnsupported ComponentHintStatus = "unsupported"
)

type ComponentHintEvidence struct {
	ID          string              `json:"id"`
	InstanceID  string              `json:"instance_id,omitempty"`
	BlockID     string              `json:"block_id,omitempty"`
	Role        string              `json:"role,omitempty"`
	ComponentID string              `json:"component_id,omitempty"`
	Type        ComponentHintKind   `json:"type"`
	Kind        string              `json:"kind"`
	Target      string              `json:"target,omitempty"`
	NetRole     string              `json:"net_role,omitempty"`
	Value       string              `json:"value,omitempty"`
	Unit        string              `json:"unit,omitempty"`
	Status      ComponentHintStatus `json:"status"`
	Message     string              `json:"message,omitempty"`
}

type ComponentHintSummary struct {
	Total     int `json:"total"`
	Placement int `json:"placement"`
	Routing   int `json:"routing"`
	// Supported aggregates hints that are either pending enforcement or already enforced.
	Supported int `json:"supported"`
	// Unsupported aggregates unsupported and skipped hints.
	Unsupported int `json:"unsupported"`
	Pending     int `json:"pending"`
	Enforced    int `json:"enforced"`
	Skipped     int `json:"skipped"`
}

func NormalizeComponentHints(selections []ComponentSelectionEntry) []ComponentHintEvidence {
	capacity := 0
	for _, selection := range selections {
		capacity += len(selection.PlacementHints) + len(selection.RoutingHints)
	}
	hints := make([]ComponentHintEvidence, 0, capacity)
	for _, selection := range selections {
		for _, hint := range selection.PlacementHints {
			kind := strings.TrimSpace(hint.Kind)
			target := strings.TrimSpace(hint.Target)
			value := strings.TrimSpace(hint.Value)
			unit := strings.TrimSpace(hint.Unit)
			status, message := classifyPlacementHint(kind)
			hints = append(hints, ComponentHintEvidence{
				ID:          componentHintID(selection, ComponentHintPlacement, kind, target, "", value, unit),
				InstanceID:  selection.InstanceID,
				BlockID:     selection.BlockID,
				Role:        selection.Role,
				ComponentID: selection.ComponentID,
				Type:        ComponentHintPlacement,
				Kind:        kind,
				Target:      target,
				Value:       value,
				Unit:        unit,
				Status:      status,
				Message:     message,
			})
		}
		for _, hint := range selection.RoutingHints {
			kind := strings.TrimSpace(hint.Kind)
			netRole := strings.TrimSpace(hint.NetRole)
			value := strings.TrimSpace(hint.Value)
			unit := strings.TrimSpace(hint.Unit)
			status, message := classifyRoutingHint(kind)
			hints = append(hints, ComponentHintEvidence{
				ID:          componentHintID(selection, ComponentHintRouting, kind, "", netRole, value, unit),
				InstanceID:  selection.InstanceID,
				BlockID:     selection.BlockID,
				Role:        selection.Role,
				ComponentID: selection.ComponentID,
				Type:        ComponentHintRouting,
				Kind:        kind,
				NetRole:     netRole,
				Value:       value,
				Unit:        unit,
				Status:      status,
				Message:     message,
			})
		}
	}
	slices.SortFunc(hints, compareComponentHintEvidence)
	return slices.CompactFunc(hints, sameComponentHint)
}

func SummarizeComponentHints(hints []ComponentHintEvidence) ComponentHintSummary {
	var summary ComponentHintSummary
	for _, hint := range hints {
		summary.Total++
		switch hint.Type {
		case ComponentHintPlacement:
			summary.Placement++
		case ComponentHintRouting:
			summary.Routing++
		}
		switch hint.Status {
		case ComponentHintUnsupported:
			summary.Unsupported++
		case ComponentHintPending:
			summary.Pending++
		case ComponentHintEnforced:
			summary.Enforced++
		case ComponentHintSkipped:
			summary.Skipped++
			summary.Unsupported++
		}
		if componentHintStatusSupported(hint.Status) {
			summary.Supported++
		}
	}
	return summary
}

func componentHintStatusSupported(status ComponentHintStatus) bool {
	switch status {
	case ComponentHintPending, ComponentHintEnforced:
		return true
	default:
		return false
	}
}

func classifyPlacementHint(kind string) (ComponentHintStatus, string) {
	switch kind {
	case "near", "edge", "keepout":
		return ComponentHintPending, "supported placement hint pending enforcement"
	default:
		return ComponentHintUnsupported, "unsupported placement hint kind"
	}
}

func classifyRoutingHint(kind string) (ComponentHintStatus, string) {
	switch kind {
	case "net_class", "tie", "no_connect":
		return ComponentHintPending, "supported routing hint pending enforcement"
	default:
		return ComponentHintUnsupported, "unsupported routing hint kind"
	}
}

func componentHintID(selection ComponentSelectionEntry, hintType ComponentHintKind, hintKind string, target string, netRole string, value string, unit string) string {
	segments := []string{
		"component_hint",
		strings.TrimSpace(selection.InstanceID),
		strings.TrimSpace(selection.BlockID),
		strings.TrimSpace(selection.Role),
		strings.TrimSpace(selection.ComponentID),
		string(hintType),
		hintKind,
		target,
		netRole,
		value,
		unit,
	}
	for index := range segments {
		segments[index] = stableComponentHintIDSegment(segments[index])
	}
	return strings.Join(segments, ":")
}

func stableComponentHintIDSegment(value string) string {
	if value == "" {
		return "%00"
	}
	escaped := strings.ReplaceAll(value, "%", "%25")
	return strings.ReplaceAll(escaped, ":", "%3A")
}

func compareComponentHintEvidence(a, b ComponentHintEvidence) int {
	if cmp := strings.Compare(a.InstanceID, b.InstanceID); cmp != 0 {
		return cmp
	}
	if cmp := strings.Compare(a.BlockID, b.BlockID); cmp != 0 {
		return cmp
	}
	if cmp := strings.Compare(a.Role, b.Role); cmp != 0 {
		return cmp
	}
	if cmp := strings.Compare(a.ComponentID, b.ComponentID); cmp != 0 {
		return cmp
	}
	if cmp := strings.Compare(string(a.Type), string(b.Type)); cmp != 0 {
		return cmp
	}
	if cmp := strings.Compare(a.Kind, b.Kind); cmp != 0 {
		return cmp
	}
	if cmp := strings.Compare(a.Target, b.Target); cmp != 0 {
		return cmp
	}
	if cmp := strings.Compare(a.NetRole, b.NetRole); cmp != 0 {
		return cmp
	}
	if cmp := strings.Compare(a.Value, b.Value); cmp != 0 {
		return cmp
	}
	if cmp := strings.Compare(a.Unit, b.Unit); cmp != 0 {
		return cmp
	}
	return 0
}

func sameComponentHint(a, b ComponentHintEvidence) bool {
	return a.InstanceID == b.InstanceID &&
		a.BlockID == b.BlockID &&
		a.Role == b.Role &&
		a.ComponentID == b.ComponentID &&
		a.Type == b.Type &&
		a.Kind == b.Kind &&
		a.Target == b.Target &&
		a.NetRole == b.NetRole &&
		a.Value == b.Value &&
		a.Unit == b.Unit
}
