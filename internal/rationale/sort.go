package rationale

import (
	"cmp"
	"slices"
	"strings"

	"kicadai/internal/intentdraft"
	"kicadai/internal/reports"
)

func evidenceIDsForClarification(clarification intentdraft.Clarification) []string {
	var ids []string
	for _, field := range clarification.Evidence {
		if field.Path == "" {
			continue
		}
		ids = append(ids, "draft_path:"+field.Path)
	}
	slices.Sort(ids)
	return ids
}

func cloneEvidence(values []EvidenceRecord) []EvidenceRecord {
	out := append([]EvidenceRecord(nil), values...)
	for i := range out {
		out[i].Notes = append([]string(nil), out[i].Notes...)
	}
	return out
}

func cloneDecisions(values []Decision) []Decision {
	out := append([]Decision(nil), values...)
	for i := range out {
		out[i].RequirementIDs = append([]string(nil), out[i].RequirementIDs...)
		out[i].EvidenceIDs = append([]string(nil), out[i].EvidenceIDs...)
	}
	return out
}

func cloneNotes(values []RationaleNote) []RationaleNote {
	out := append([]RationaleNote(nil), values...)
	for i := range out {
		out[i].EvidenceIDs = append([]string(nil), out[i].EvidenceIDs...)
	}
	return out
}

func cloneLimits(values []KnownLimit) []KnownLimit {
	out := append([]KnownLimit(nil), values...)
	for i := range out {
		out[i].EvidenceIDs = append([]string(nil), out[i].EvidenceIDs...)
	}
	return out
}

func cloneNextActions(values []NextAction) []NextAction {
	return append([]NextAction(nil), values...)
}

func compareEvidence(a, b EvidenceRecord) int {
	return cmp.Or(strings.Compare(a.ID, b.ID), strings.Compare(a.Kind, b.Kind), strings.Compare(a.Path, b.Path))
}

func compareDecisions(a, b Decision) int {
	return cmp.Or(strings.Compare(a.ID, b.ID), strings.Compare(a.Type, b.Type), strings.Compare(a.Path, b.Path))
}

func compareRationaleNotes(a, b RationaleNote) int {
	return cmp.Or(strings.Compare(a.ID, b.ID), strings.Compare(a.Path, b.Path), strings.Compare(a.Message, b.Message))
}

func compareKnownLimits(a, b KnownLimit) int {
	return cmp.Or(strings.Compare(a.ID, b.ID), strings.Compare(a.Category, b.Category), strings.Compare(a.Path, b.Path))
}

func compareNextActions(a, b NextAction) int {
	return cmp.Or(cmp.Compare(a.Priority, b.Priority), strings.Compare(a.ID, b.ID), strings.Compare(a.Action, b.Action))
}

func compareArtifacts(a, b reports.Artifact) int {
	return cmp.Or(strings.Compare(string(a.Kind), string(b.Kind)), strings.Compare(a.Path, b.Path), strings.Compare(a.Description, b.Description))
}
