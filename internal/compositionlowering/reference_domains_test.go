package compositionlowering

import (
	"context"
	"strings"
	"testing"

	"kicadai/internal/architecturesearch"
	"kicadai/internal/circuitgraph"
	"kicadai/internal/components"
	"kicadai/internal/reports"
)

func TestExplicitReferenceReachesParticipantPhysicalReturn(t *testing.T) {
	r := referencePromotionRequirement(t, true)
	r = architecturesearch.Normalize(r)
	if issues := architecturesearch.Validate(r); reports.HasBlockingIssue(issues) {
		t.Fatal(issues)
	}
	catalog, err := components.LoadCatalog(context.Background(), components.LoadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	registry, issues := architecturesearch.NewCatalogRegistry(catalog)
	if reports.HasBlockingIssue(issues) {
		t.Fatal(issues)
	}
	resolver := circuitgraph.NewResolver(circuitgraph.ResolveOptions{Catalog: catalog, CatalogID: "checked-in"})
	search := architecturesearch.Search(context.Background(), r, registry, architecturesearch.SearchOptions{CatalogHash: resolver.CatalogHash()})
	if search.Status != architecturesearch.SearchSelected {
		t.Fatal(search.Issues, search.Rejections)
	}
	lowered, issues := Lower(r, search)
	if reports.HasBlockingIssue(issues) {
		t.Fatal(issues)
	}
	targets := map[string]string{}
	for _, binding := range lowered.Evidence.SemanticBindings {
		targets[binding.Kind+"\x00"+binding.ID] = binding.Target
	}
	ground := targets["domain\x00ground"]
	if ground == "" || ground == targets["domain\x00aaa_sensor_ground"] {
		t.Fatal("separate references collapsed")
	}
	resolved, issues := resolver.Resolve(context.Background(), lowered.Document)
	if reports.HasBlockingIssue(issues) {
		t.Fatal(issues)
	}
	assertSelectedRoleOnPhysicalNet(t, search, resolved, "participant:controller", "reference", ground)
	assertSelectedRoleOnPhysicalNet(t, search, resolved, "objective:regulate", "reference", ground)
	assertSelectedRoleOnPhysicalNet(t, search, resolved, "objective:condition_adc", "reference", ground)
	if got, ok := operatingConditionReferenceTarget(r, architecturesearch.OperatingCondition{Target: "regulated_output"}, targets); !ok || got != ground {
		t.Fatalf("load reference %q %v", got, ok)
	}
	stimulus, err := transientStimulusHarnessDevice(r, lowered.Evidence.SemanticBindings, behavioralTransientStimulus{Node: targets["port\x00analog_input"], SemanticID: "analog_input"})
	if err != nil || stimulus.Device.Connections[1].Net != ground {
		t.Fatalf("stimulus used wrong return: %#v %v", stimulus, err)
	}
	// Distinct physical grounds still require reviewed isolation primitives.
	// An explicit identity is not authority to bypass that simulation gate.
	// The other return sorts first and shares the supply's name tokens.
	r.Requirements.Domains = append(r.Requirements.Domains, architecturesearch.Domain{ID: "aaa_sensor_ground", Kind: "reference", Source: "external"})
	r.Requirements.Ports = append(r.Requirements.Ports,
		architecturesearch.Port{ID: "unused_return", Kind: "reference", Direction: "bidirectional", Domain: "aaa_sensor_ground"},
		architecturesearch.Port{ID: "unused_return_peer", Kind: "reference", Direction: "bidirectional", Domain: "aaa_sensor_ground"})
	r = architecturesearch.Normalize(r)
	separate := architecturesearch.Search(context.Background(), r, registry, architecturesearch.SearchOptions{CatalogHash: resolver.CatalogHash()})
	if separate.Status != architecturesearch.SearchSelected {
		t.Fatal(separate.Issues, separate.Rejections)
	}
	for _, selection := range separate.Selected.Selections {
		if selection.ObligationPath != "participant:controller" {
			continue
		}
		for _, port := range selection.Ports {
			if port.Role == "reference" && port.Contract.Domain != "ground" {
				t.Fatal("participant reference guessed from spelling/order")
			}
		}
	}
	separateLowered, separateIssues := Lower(r, separate)
	if reports.HasBlockingIssue(separateIssues) {
		t.Fatal(separateIssues)
	}
	_, separateIssues = resolver.Resolve(context.Background(), separateLowered.Document)
	isolationRejected := false
	for _, issue := range separateIssues {
		if strings.Contains(issue.Message, "multiple reference domains require every ground node to terminate a reviewed isolation primitive") {
			isolationRejected = true
		}
	}
	if !isolationRejected {
		t.Fatal("unreviewed separate ground topology escaped isolation gate", separateIssues)
	}
	for i := range r.Requirements.Domains {
		if r.Requirements.Domains[i].ID == "sensor_3v3" {
			r.Requirements.Domains[i].ReferenceDomain = "missing"
		}
	}
	if referenceDomainForPower(r, "sensor_3v3") != "" {
		t.Fatal("invalid reference fell back to ground name")
	}
	if _, ok := operatingConditionReferenceTarget(r, architecturesearch.OperatingCondition{Target: "regulated_output"}, targets); ok {
		t.Fatal("invalid load reference fell back to objective")
	}
}
