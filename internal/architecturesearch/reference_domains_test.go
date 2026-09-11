package architecturesearch

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"kicadai/internal/reports"
)

func TestExplicitReferenceDomainIdentityAndValidation(t *testing.T) {
	r := standaloneOutputRequirement(t)
	for i := range r.Requirements.Domains {
		if r.Requirements.Domains[i].Kind == "supply" {
			r.Requirements.Domains[i].ReferenceDomain = "ground"
		}
	}
	r.Requirements.Domains = append(r.Requirements.Domains, Domain{ID: "aaa_sensor_ground", Kind: "reference", Source: "external"})
	for _, reverse := range []bool{false, true} {
		if reverse {
			slices.Reverse(r.Requirements.Domains)
		}
		if issues := Validate(Normalize(r)); reports.HasBlockingIssue(issues) {
			t.Fatal(issues)
		}
		if got, ok := ResolveReferenceDomain(r, "sensor_3v3"); !ok || got != "ground" {
			t.Fatalf("wrong reference %q %v", got, ok)
		}
	}
	for _, bad := range []string{"", "missing", "sensor_3v3", "input_5v"} {
		t.Run("invalid_"+bad, func(t *testing.T) {
			candidate := Normalize(r)
			for i := range candidate.Requirements.Domains {
				if candidate.Requirements.Domains[i].ID == "sensor_3v3" {
					candidate.Requirements.Domains[i].ReferenceDomain = bad
				}
			}
			if _, ok := ResolveReferenceDomain(candidate, "sensor_3v3"); ok {
				t.Fatal("invalid or ambiguous reference resolved")
			}
			if !reports.HasBlockingIssue(Validate(candidate)) {
				t.Fatal("invalid or ambiguous reference accepted")
			}
		})
	}
	r.Version = VersionV2
	if !reports.HasBlockingIssue(Validate(r)) {
		t.Fatal("explicit reference enabled in legacy schema")
	}
}

func TestExplicitReferenceRejectsContradictoryObjectiveReturn(t *testing.T) {
	r := standaloneOutputRequirement(t)
	r.Requirements.Domains = append(r.Requirements.Domains, Domain{ID: "other_ground", Kind: "reference", Source: "external"})
	for i := range r.Requirements.Domains {
		if r.Requirements.Domains[i].Kind == "supply" {
			r.Requirements.Domains[i].ReferenceDomain = "other_ground"
		}
	}
	if !reports.HasBlockingIssue(Validate(r)) {
		t.Fatal("objective ground disagrees with explicit supply return")
	}
}

func TestReferenceDomainCanonicalizationPreservesLegacyEncoding(t *testing.T) {
	r := standaloneOutputRequirement(t)
	b, err := json.Marshal(r)
	if err != nil || strings.Contains(string(b), "reference_domain") {
		t.Fatal("legacy encoding changed", err)
	}
	r.Requirements.Domains[0].ReferenceDomain = " GROUND "
	n := Normalize(r)
	for _, d := range n.Requirements.Domains {
		if d.ID == r.Requirements.Domains[0].ID && d.ReferenceDomain != "ground" {
			t.Fatal("reference not normalized")
		}
	}
}
