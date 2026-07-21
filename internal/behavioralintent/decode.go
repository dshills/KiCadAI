package behavioralintent

import (
	"bytes"
	"encoding/json"
	"io"

	"kicadai/internal/reports"
)

func DecodeProposalStrict(reader io.Reader) (Proposal, []reports.Issue) {
	var buffer bytes.Buffer
	if _, err := io.Copy(&buffer, io.LimitReader(reader, MaxProposalBytes+1)); err != nil {
		return Proposal{}, []reports.Issue{compilerIssue(CodeProposalInvalid, "proposal", "read behavioral intent proposal: "+err.Error())}
	}
	if buffer.Len() > MaxProposalBytes {
		return Proposal{}, []reports.Issue{compilerIssue(CodeProposalLimit, "proposal", "behavioral intent proposal exceeds maximum encoded size")}
	}
	decoder := json.NewDecoder(bytes.NewReader(buffer.Bytes()))
	decoder.DisallowUnknownFields()
	var proposal Proposal
	if err := decoder.Decode(&proposal); err != nil {
		return Proposal{}, []reports.Issue{compilerIssue(CodeProposalInvalid, "proposal", "decode behavioral intent proposal: "+err.Error())}
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return Proposal{}, []reports.Issue{compilerIssue(CodeProposalInvalid, "proposal", "behavioral intent proposal must contain exactly one JSON object")}
	}
	return proposal, nil
}

func DecodeFollowUpStrict(reader io.Reader) (FollowUp, []reports.Issue) {
	var buffer bytes.Buffer
	if _, err := io.Copy(&buffer, io.LimitReader(reader, MaxProposalBytes+1)); err != nil {
		return FollowUp{}, []reports.Issue{compilerIssue(CodeFollowUpInvalid, "follow_up", "read behavioral clarification follow-up: "+err.Error())}
	}
	if buffer.Len() > MaxProposalBytes {
		return FollowUp{}, []reports.Issue{compilerIssue(CodeProposalLimit, "follow_up", "behavioral clarification follow-up exceeds maximum encoded size")}
	}
	decoder := json.NewDecoder(bytes.NewReader(buffer.Bytes()))
	decoder.DisallowUnknownFields()
	var followUp FollowUp
	if err := decoder.Decode(&followUp); err != nil {
		return FollowUp{}, []reports.Issue{compilerIssue(CodeFollowUpInvalid, "follow_up", "decode behavioral clarification follow-up: "+err.Error())}
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return FollowUp{}, []reports.Issue{compilerIssue(CodeFollowUpInvalid, "follow_up", "behavioral clarification follow-up must contain exactly one JSON object")}
	}
	return followUp, nil
}
