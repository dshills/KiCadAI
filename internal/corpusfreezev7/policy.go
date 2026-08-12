// Package corpusfreezev7 adds the frozen V7 corpus policy and aggregate
// diversity boundary without invoking synthesis or outcome logic.
package corpusfreezev7

import "kicadai/internal/corpusfreeze"

const (
	PacketSetSHA256             = "7b0bffb5869cfc215aa97d333bfecb56ee87b730862bceb11fd619181a268451"
	HistoricalCommitmentsSHA256 = "bf39d127b950d0fb09c96a6ed34fdfd20258ee275ba5a49844be2aa2678af00d"
)

func Policy() corpusfreeze.Policy {
	policy := corpusfreeze.V5Policy()
	policy.AssignmentSchema = "kicadai.closed-loop-open-set-author-assignment.v7"
	policy.AuthorshipSchema = "kicadai.closed-loop-open-set-authorship.v7"
	policy.Version = 7
	policy.PacketSetSHA256 = PacketSetSHA256
	policy.HistoricalCommitmentsSHA256 = HistoricalCommitmentsSHA256
	policy.ProhibitedIdentityPrefixes = []string{"v7_case_", "v7_source_"}
	return policy
}
