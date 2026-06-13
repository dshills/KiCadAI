package checks

import "strings"

type AllowlistEntry struct {
	Reason         string         `json:"reason"`
	Kind           CheckKind      `json:"kind,omitempty"`
	Target         string         `json:"target,omitempty"`
	Severity       string         `json:"severity,omitempty"`
	Rule           string         `json:"rule,omitempty"`
	Code           string         `json:"code,omitempty"`
	Message        string         `json:"message,omitempty"`
	Reference      string         `json:"reference,omitempty"`
	Net            string         `json:"net,omitempty"`
	Layer          string         `json:"layer,omitempty"`
	RepairCategory RepairCategory `json:"repair_category,omitempty"`
}

func FilterAllowedFindings(findings []CheckFinding, entries []AllowlistEntry) ([]CheckFinding, []CheckFinding) {
	if len(entries) == 0 || len(findings) == 0 {
		return findings, nil
	}
	remaining := make([]CheckFinding, 0, len(findings))
	allowed := make([]CheckFinding, 0, len(findings))
	for _, finding := range findings {
		if findingAllowed(finding, entries) {
			allowed = append(allowed, finding)
		} else {
			remaining = append(remaining, finding)
		}
	}
	return remaining, allowed
}

func findingAllowed(finding CheckFinding, entries []AllowlistEntry) bool {
	for _, entry := range entries {
		if entry.matches(finding) {
			return true
		}
	}
	return false
}

func (entry AllowlistEntry) matches(finding CheckFinding) bool {
	if entry.Kind != "" && entry.Kind != finding.Kind {
		return false
	}
	if entry.Target != "" && !strings.Contains(normalizeKey(finding.File), normalizeKey(entry.Target)) {
		return false
	}
	if entry.Severity != "" && normalizeKey(entry.Severity) != normalizeKey(finding.Severity) {
		return false
	}
	if entry.Rule != "" && normalizeKey(entry.Rule) != normalizeKey(finding.Rule) {
		return false
	}
	if entry.Code != "" && normalizeKey(entry.Code) != normalizeKey(finding.Code) {
		return false
	}
	if entry.Message != "" && !strings.Contains(normalizeKey(finding.Message), normalizeKey(entry.Message)) {
		return false
	}
	if entry.Reference != "" && !containsString(finding.References, entry.Reference) {
		return false
	}
	if entry.Net != "" && normalizeKey(entry.Net) != normalizeKey(finding.Net) && !containsString(finding.Nets, entry.Net) {
		return false
	}
	if entry.Layer != "" && normalizeKey(entry.Layer) != normalizeKey(finding.Layer) {
		return false
	}
	if entry.RepairCategory != "" && entry.RepairCategory != finding.RepairCategory {
		return false
	}
	return entry.hasMatcher()
}

func (entry AllowlistEntry) hasMatcher() bool {
	return entry.Kind != "" ||
		entry.Target != "" ||
		entry.Severity != "" ||
		entry.Rule != "" ||
		entry.Code != "" ||
		entry.Message != "" ||
		entry.Reference != "" ||
		entry.Net != "" ||
		entry.Layer != "" ||
		entry.RepairCategory != ""
}

func containsString(values []string, want string) bool {
	want = normalizeKey(want)
	for _, value := range values {
		if normalizeKey(value) == want {
			return true
		}
	}
	return false
}
