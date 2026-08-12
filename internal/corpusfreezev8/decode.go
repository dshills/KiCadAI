package corpusfreezev8

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

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
	var value Assignment
	if err := decodeStrict(data, &value); err != nil {
		return Assignment{}, fmt.Errorf("decode assignment: %w", err)
	}
	for _, entry := range value.Entries {
		if entry.ID == "" || entry.Role == "" || entry.Domain == "" || entry.CircuitRole == "" ||
			entry.SafetyImpact == "" || entry.SourceID == "" || entry.RequirementFile == "" {
			return Assignment{}, fmt.Errorf("decode assignment: required entry field is missing")
		}
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
