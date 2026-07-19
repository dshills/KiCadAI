package architecturesearch

import (
	"bytes"
	"encoding/json"
	"io"

	"kicadai/internal/reports"
)

func DecodeStrict(reader io.Reader) (Requirement, []reports.Issue) {
	var buffer bytes.Buffer
	limited := io.LimitReader(reader, MaxRequirementBytes+1)
	if _, err := io.Copy(&buffer, limited); err != nil {
		return Requirement{}, []reports.Issue{architectureIssue(CodeSchemaInvalid, "document", "read architecture requirement: "+err.Error())}
	}
	if buffer.Len() > MaxRequirementBytes {
		return Requirement{}, []reports.Issue{architectureIssue(CodeLimitExceeded, "document", "architecture requirement exceeds maximum encoded size")}
	}

	decoder := json.NewDecoder(bytes.NewReader(buffer.Bytes()))
	decoder.DisallowUnknownFields()
	var requirement Requirement
	if err := decoder.Decode(&requirement); err != nil {
		return Requirement{}, []reports.Issue{architectureIssue(CodeSchemaInvalid, "document", "decode architecture requirement: "+err.Error())}
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return requirement, []reports.Issue{architectureIssue(CodeSchemaInvalid, "document", "architecture requirement contains trailing JSON value")}
		}
		return requirement, []reports.Issue{architectureIssue(CodeSchemaInvalid, "document", "decode trailing architecture requirement data: "+err.Error())}
	}

	normalized := Normalize(requirement)
	issues := Validate(normalized)
	if reports.HasBlockingIssue(issues) {
		return normalized, issues
	}
	return normalized, nil
}
