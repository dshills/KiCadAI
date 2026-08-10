package corpusfreeze

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

func DecodeAssignmentStrict(data []byte) (Assignment, error) {
	var assignment Assignment
	if err := decodeStrict(data, &assignment); err != nil {
		return Assignment{}, fmt.Errorf("decode assignment: %w", err)
	}
	return assignment, nil
}

func DecodeAuthorshipStrict(data []byte) (Authorship, error) {
	var authorship Authorship
	if err := decodeStrict(data, &authorship); err != nil {
		return Authorship{}, fmt.Errorf("decode authorship: %w", err)
	}
	return authorship, nil
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
