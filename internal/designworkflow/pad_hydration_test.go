package designworkflow

import (
	"reflect"
	"testing"

	"kicadai/internal/reports"
)

func TestSummarizePadHydrationOrdersMissingRefsAndCountsSources(t *testing.T) {
	summary := summarizePadHydration([]PadHydrationEntry{
		{Ref: "D1", FootprintID: "LED:D", Source: PadHydrationSourceResolver, PadCount: 2},
		{Ref: "R1", FootprintID: "R:R", Source: PadHydrationSourceVerifiedTemplate, PadCount: 2},
		{Ref: "J1", FootprintID: "Connector:J", Source: PadHydrationSourceMissing, MissingReason: "not found"},
	}, []reports.Issue{
		{Severity: reports.SeverityBlocked, Refs: []string{"U1"}},
		{Severity: reports.SeverityWarning, Refs: []string{"J1"}},
	})

	if summary.ComponentCount != 3 || summary.HydratedComponents != 2 || summary.MissingComponents != 1 || summary.PadCount != 4 {
		t.Fatalf("summary counts = %#v", summary)
	}
	wantSources := map[PadHydrationSource]int{PadHydrationSourceResolver: 1, PadHydrationSourceVerifiedTemplate: 1, PadHydrationSourceMissing: 1}
	if !reflect.DeepEqual(summary.SourceCounts, wantSources) {
		t.Fatalf("source counts = %#v, want %#v", summary.SourceCounts, wantSources)
	}
	if !reflect.DeepEqual(summary.MissingRefs, []string{"J1"}) {
		t.Fatalf("missing refs = %#v", summary.MissingRefs)
	}
	if summary.BlockingIssues != 1 {
		t.Fatalf("blocking issues = %d", summary.BlockingIssues)
	}
}

func TestSummarizePadHydrationTreatsEmptySourceAsMissing(t *testing.T) {
	summary := summarizePadHydration([]PadHydrationEntry{{Ref: "X1"}}, nil)
	if summary.SourceCounts["missing"] != 1 || summary.MissingComponents != 1 {
		t.Fatalf("summary = %#v", summary)
	}
}
