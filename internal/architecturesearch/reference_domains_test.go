package architecturesearch

import (
	"bytes"
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"

	"kicadai/internal/reports"
)

func TestExplicitReferenceCommonAndSideSpecificBindings(t *testing.T) {
	b, err := os.ReadFile("testdata/power_interface_synthesis_corpus/regulated_mcu_sensor_subsystem.json")
	if err != nil {
		t.Fatal(err)
	}
	original, issues := DecodeStrict(bytes.NewReader(b))
	if reports.HasBlockingIssue(issues) {
		t.Fatal(issues)
	}
	for _, mode := range []string{"common", "side_override", "side_mismatch", "side_nonreference", "common_mismatch", "missing"} {
		t.Run(mode, func(t *testing.T) {
			r := Normalize(original)
			for i := range r.Requirements.Domains {
				if r.Requirements.Domains[i].Kind == "supply" {
					r.Requirements.Domains[i].ReferenceDomain = "ground"
				}
			}
			r.Requirements.Domains = append(r.Requirements.Domains, Domain{ID: "isolated_return", Kind: "reference", Source: "external"})
			r.Requirements.Ports = append(r.Requirements.Ports, Port{ID: "isolated_return_port", Kind: "reference", Direction: "bidirectional", Domain: "isolated_return"})
			for i := range r.Requirements.Objectives {
				o := &r.Requirements.Objectives[i]
				if o.Capability != "logic_level_translation" {
					continue
				}
				if mode == "side_override" || mode == "side_mismatch" || mode == "side_nonreference" {
					p := "ground"
					if mode == "side_mismatch" {
						p = "isolated_return_port"
					}
					if mode == "side_nonreference" {
						p = "power"
					}
					o.Bindings = append(o.Bindings, Binding{Role: "reference_a", Port: p})
				}
				if mode == "common_mismatch" || mode == "missing" {
					for j := range o.Bindings {
						if o.Bindings[j].Role == "reference" {
							if mode == "missing" {
								o.Bindings = append(o.Bindings[:j], o.Bindings[j+1:]...)
							} else {
								o.Bindings[j].Port = "isolated_return_port"
							}
							break
						}
					}
				}
			}
			blocked := reports.HasBlockingIssue(Validate(Normalize(r)))
			wantBlocked := mode == "side_mismatch" || mode == "side_nonreference" || mode == "common_mismatch" || mode == "missing"
			if blocked != wantBlocked {
				t.Fatalf("blocked=%t want=%t: %v", blocked, wantBlocked, Validate(Normalize(r)))
			}
		})
	}
}

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
