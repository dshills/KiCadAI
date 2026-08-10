package corpusfreezev6

import "kicadai/internal/corpusfreeze"

func Policy() corpusfreeze.Policy {
	policy := corpusfreeze.V5Policy()
	policy.AssignmentSchema = "kicadai.closed-loop-open-set-author-assignment.v6"
	policy.AuthorshipSchema = "kicadai.closed-loop-open-set-authorship.v6"
	policy.Version = 6
	policy.PacketSetSHA256 = "664b6d20ceb1433509e20016e0fbe3ddf98f6c8c1da01f5aeca7f50f2db6f31a"
	policy.HistoricalCommitmentsSHA256 = "eb329517366df07d5364bdc43424a8caf2f86d8bd806086b0af8ea68f02f5807"
	policy.ProhibitedIdentityPrefixes = []string{"v6_case_", "v6_source_"}
	return policy
}
