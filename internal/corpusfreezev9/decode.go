package corpusfreezev9

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

type assignmentWire struct {
	Schema     string                `json:"schema"`
	Version    int                   `json:"version"`
	AuthorSlot string                `json:"author_slot"`
	Entries    []assignmentEntryWire `json:"entries"`
}

type assignmentEntryWire struct {
	ID                      string `json:"id"`
	Role                    string `json:"role"`
	Domain                  string `json:"domain"`
	CircuitRole             string `json:"circuit_role"`
	SafetyImpact            string `json:"safety_impact"`
	PrimaryClass            string `json:"primary_class"`
	RequiredPrimaryAnalysis string `json:"required_primary_analysis"`
	OutputMultiplicity      string `json:"output_multiplicity"`
	RequireOffNominal       *bool  `json:"require_off_nominal"`
	SourceID                string `json:"source_id"`
	RequirementFile         string `json:"requirement_file"`
}

func decodeStrict(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("unexpected trailing JSON value")
		}
		return fmt.Errorf("decode trailing JSON: %w", err)
	}
	return nil
}

func decodeAssignment(data []byte) (Assignment, error) {
	var wire assignmentWire
	if err := decodeStrict(data, &wire); err != nil {
		return Assignment{}, fmt.Errorf("decode assignment: %w", err)
	}
	value := Assignment{Schema: wire.Schema, Version: wire.Version, AuthorSlot: wire.AuthorSlot, Entries: make([]AssignmentEntry, 0, len(wire.Entries))}
	for _, entry := range wire.Entries {
		// The frozen assignment contract requires an exact object shape.
		// require_off_nominal is therefore present even when false; accepting
		// its omission would make a packet produced with omitempty noncanonical.
		if entry.ID == "" || entry.Role == "" || entry.Domain == "" || entry.CircuitRole == "" ||
			entry.SafetyImpact == "" || entry.PrimaryClass == "" || entry.RequiredPrimaryAnalysis == "" ||
			entry.OutputMultiplicity == "" || entry.RequireOffNominal == nil || entry.SourceID == "" || entry.RequirementFile == "" {
			return Assignment{}, fmt.Errorf("decode assignment: required entry field is missing")
		}
		value.Entries = append(value.Entries, AssignmentEntry{
			ID: entry.ID, Role: entry.Role, Domain: entry.Domain, CircuitRole: entry.CircuitRole,
			SafetyImpact: entry.SafetyImpact, PrimaryClass: entry.PrimaryClass,
			RequiredPrimaryAnalysis: entry.RequiredPrimaryAnalysis, OutputMultiplicity: entry.OutputMultiplicity,
			RequireOffNominal: *entry.RequireOffNominal, SourceID: entry.SourceID, RequirementFile: entry.RequirementFile,
		})
	}
	return value, nil
}

func decodeAuthorship(data []byte) (Authorship, error) {
	var value Authorship
	if err := decodeStrict(data, &value); err != nil {
		return Authorship{}, fmt.Errorf("decode authorship: %w", err)
	}
	return value, nil
}
