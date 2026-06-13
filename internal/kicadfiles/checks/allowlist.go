package checks

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
