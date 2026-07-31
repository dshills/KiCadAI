package opentopologysynthesis

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"kicadai/internal/reports"
)

func DecodeStrict(reader io.Reader) (Requirement, []reports.Issue) {
	data, err := io.ReadAll(io.LimitReader(reader, MaxRequirementBytes+1))
	if err != nil {
		return Requirement{}, []reports.Issue{requirementIssue("document", "read open-topology requirement: "+err.Error())}
	}
	if len(data) > MaxRequirementBytes {
		return Requirement{}, []reports.Issue{requirementIssue("document", fmt.Sprintf("open-topology requirement exceeds %d bytes", MaxRequirementBytes))}
	}

	var requirement Requirement
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&requirement); err != nil {
		return Requirement{}, []reports.Issue{requirementIssue("document", "decode open-topology requirement: "+err.Error())}
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return requirement, []reports.Issue{requirementIssue("document", "open-topology requirement contains trailing JSON value")}
		}
		return requirement, []reports.Issue{requirementIssue("document", "decode trailing open-topology requirement data: "+err.Error())}
	}
	requirement = Normalize(requirement)
	return requirement, Validate(requirement)
}

func requirementIssue(path, message string) reports.Issue {
	return reports.Issue{
		Code:     CodeRequirementInvalid,
		Severity: reports.SeverityError,
		Stage:    "open_topology_requirement",
		Path:     path,
		Message:  message,
	}
}
