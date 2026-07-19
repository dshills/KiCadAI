package architecturesearch

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

func Normalize(requirement Requirement) Requirement {
	normalized := cloneRequirement(requirement)
	normalized.Schema = strings.TrimSpace(normalized.Schema)
	normalized.Project.Name = canonicalIdentifier(normalized.Project.Name)
	normalized.Project.Title = strings.TrimSpace(normalized.Project.Title)
	normalized.Project.Description = strings.TrimSpace(normalized.Project.Description)

	for index := range normalized.Requirements.Domains {
		domain := &normalized.Requirements.Domains[index]
		domain.ID = canonicalIdentifier(domain.ID)
		domain.Kind = canonicalIdentifier(domain.Kind)
		domain.Source = canonicalIdentifier(domain.Source)
	}
	slices.SortStableFunc(normalized.Requirements.Domains, func(left, right Domain) int {
		return strings.Compare(left.ID, right.ID)
	})

	for index := range normalized.Requirements.Ports {
		port := &normalized.Requirements.Ports[index]
		port.ID = canonicalIdentifier(port.ID)
		port.Kind = canonicalIdentifier(port.Kind)
		port.Direction = canonicalIdentifier(port.Direction)
		port.Domain = canonicalIdentifier(port.Domain)
		if port.Electrical != nil {
			port.Electrical.DefaultState = canonicalIdentifier(port.Electrical.DefaultState)
		}
		if port.Protocol != nil {
			normalizeProtocol(port.Protocol)
		}
	}
	slices.SortStableFunc(normalized.Requirements.Ports, func(left, right Port) int {
		return strings.Compare(left.ID, right.ID)
	})

	for index := range normalized.Requirements.Participants {
		participant := &normalized.Requirements.Participants[index]
		participant.ID = canonicalIdentifier(participant.ID)
		participant.Capability = canonicalIdentifier(participant.Capability)
		participant.Domain = canonicalIdentifier(participant.Domain)
		for portIndex := range participant.RequiredPorts {
			port := &participant.RequiredPorts[portIndex]
			port.ID = canonicalIdentifier(port.ID)
			port.Kind = canonicalIdentifier(port.Kind)
			port.Direction = canonicalIdentifier(port.Direction)
			normalizeProtocol(&port.Protocol)
		}
		slices.SortStableFunc(participant.RequiredPorts, func(left, right ParticipantPort) int {
			return strings.Compare(left.ID, right.ID)
		})
		normalizeConstraints(participant.Constraints)
	}
	slices.SortStableFunc(normalized.Requirements.Participants, func(left, right Participant) int {
		return strings.Compare(left.ID, right.ID)
	})

	for index := range normalized.Requirements.Objectives {
		objective := &normalized.Requirements.Objectives[index]
		objective.ID = canonicalIdentifier(objective.ID)
		objective.Capability = canonicalIdentifier(objective.Capability)
		for bindingIndex := range objective.Bindings {
			binding := &objective.Bindings[bindingIndex]
			binding.Role = canonicalIdentifier(binding.Role)
			binding.Port = canonicalIdentifier(binding.Port)
			binding.Participant = canonicalIdentifier(binding.Participant)
			binding.ParticipantPort = canonicalIdentifier(binding.ParticipantPort)
		}
		slices.SortStableFunc(objective.Bindings, func(left, right Binding) int {
			if order := strings.Compare(left.Role, right.Role); order != 0 {
				return order
			}
			if order := strings.Compare(left.Port, right.Port); order != 0 {
				return order
			}
			if order := strings.Compare(left.Participant, right.Participant); order != 0 {
				return order
			}
			return strings.Compare(left.ParticipantPort, right.ParticipantPort)
		})
		normalizeConstraints(objective.Constraints)
	}
	slices.SortStableFunc(normalized.Requirements.Objectives, func(left, right Objective) int {
		return strings.Compare(left.ID, right.ID)
	})
	return normalized
}

func CanonicalJSON(requirement Requirement) ([]byte, error) {
	normalized := Normalize(requirement)
	if issues := Validate(normalized); len(issues) != 0 {
		return nil, fmt.Errorf("architecture requirement has %d validation issue(s): %s", len(issues), issues[0].Message)
	}
	return json.Marshal(normalized)
}

func CanonicalHash(requirement Requirement) (string, error) {
	encoded, err := CanonicalJSON(requirement)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

func normalizeProtocol(protocol *Protocol) {
	protocol.Name = canonicalIdentifier(protocol.Name)
	protocol.Mode = canonicalIdentifier(protocol.Mode)
}

func normalizeConstraints(constraints []Constraint) {
	for index := range constraints {
		constraint := &constraints[index]
		constraint.Name = canonicalIdentifier(constraint.Name)
		constraint.Relation = canonicalIdentifier(constraint.Relation)
		constraint.Unit = canonicalUnit(constraint.Unit)
		constraint.Value = canonicalConstraintValue(constraint.Relation, constraint.Value)
	}
	slices.SortStableFunc(constraints, func(left, right Constraint) int {
		if order := strings.Compare(left.Name, right.Name); order != 0 {
			return order
		}
		if order := strings.Compare(left.Relation, right.Relation); order != 0 {
			return order
		}
		if order := strings.Compare(left.Unit, right.Unit); order != 0 {
			return order
		}
		if order := cmp.Compare(pointerFloat(left.TolerancePercent), pointerFloat(right.TolerancePercent)); order != 0 {
			return order
		}
		return strings.Compare(string(left.Value), string(right.Value))
	})
}

func canonicalConstraintValue(relation string, raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return append(json.RawMessage(nil), raw...)
	}
	value = normalizeConstraintScalar(value)
	if relation == "one_of" {
		if values, ok := value.([]any); ok {
			slices.SortStableFunc(values, func(left, right any) int {
				leftBytes, _ := json.Marshal(left)
				rightBytes, _ := json.Marshal(right)
				return strings.Compare(string(leftBytes), string(rightBytes))
			})
			value = values
		}
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return append(json.RawMessage(nil), raw...)
	}
	return encoded
}

func normalizeConstraintScalar(value any) any {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case []any:
		for index := range typed {
			typed[index] = normalizeConstraintScalar(typed[index])
		}
		return typed
	case map[string]any:
		for key, child := range typed {
			typed[key] = normalizeConstraintScalar(child)
		}
		return typed
	default:
		return value
	}
}

func canonicalIdentifier(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func canonicalUnit(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "":
		return ""
	case "v":
		return "V"
	case "a":
		return "A"
	case "hz":
		return "Hz"
	case "ohm":
		return "Ohm"
	case "us":
		return "us"
	case "ratio":
		return "ratio"
	case "db":
		return "dB"
	default:
		return strings.TrimSpace(value)
	}
}

func cloneRequirement(requirement Requirement) Requirement {
	encoded, err := json.Marshal(requirement)
	if err != nil {
		return requirement
	}
	var cloned Requirement
	if err := json.Unmarshal(encoded, &cloned); err != nil {
		return requirement
	}
	return cloned
}

func pointerFloat(value *float64) float64 {
	if value == nil {
		return -1
	}
	return *value
}
