package architecturesearch

import (
	"encoding/json"
	"reflect"
	"slices"
	"testing"

	"kicadai/internal/reports"
)

func TestSemanticCapabilitiesAreRegistryDerivedDeterministicAndFresh(t *testing.T) {
	firstProvider := staticTestProvider{descriptor: validProviderDescriptor("filter_provider", "frequency_filter", "signal_amplification")}
	secondProvider := staticTestProvider{descriptor: validProviderDescriptor("power_provider", "transient_protection", "voltage_regulation")}
	firstRegistry, issues := NewRegistry(firstProvider, secondProvider)
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("first registry issues = %#v", issues)
	}
	secondRegistry, issues := NewRegistry(secondProvider, firstProvider)
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("second registry issues = %#v", issues)
	}
	first, err := firstRegistry.SemanticCapabilities()
	if err != nil {
		t.Fatal(err)
	}
	second, err := secondRegistry.SemanticCapabilities()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) || first.DocumentHash == "" || first.RegistryHash != firstRegistry.Hash() {
		t.Fatalf("capability documents differ: first=%#v second=%#v", first, second)
	}
	wantObjectives := []string{"frequency_filter", "signal_amplification", "transient_protection", "voltage_regulation"}
	if !slices.Equal(first.ObjectiveKinds, wantObjectives) || len(first.BehavioralMetrics) == 0 || len(first.OperatingAxes) == 0 {
		t.Fatalf("semantic capabilities = %#v", first)
	}
	first.ObjectiveKinds[0] = "mutated"
	fresh, err := firstRegistry.SemanticCapabilities()
	if err != nil || fresh.ObjectiveKinds[0] == "mutated" {
		t.Fatalf("capability document reused mutable state: err=%v document=%#v", err, fresh)
	}
}

func TestEncodeSemanticCapabilitiesFailsClosed(t *testing.T) {
	if _, err := EncodeSemanticCapabilities(nil, 0); err == nil {
		t.Fatal("nil registry did not fail closed")
	}
	provider := staticTestProvider{descriptor: validProviderDescriptor("filter_provider", "frequency_filter")}
	registry, issues := NewRegistry(provider)
	if reports.HasBlockingIssue(issues) {
		t.Fatalf("registry issues = %#v", issues)
	}
	encoded, err := EncodeSemanticCapabilities(registry, 0)
	if err != nil || !json.Valid(encoded) {
		t.Fatalf("encoded capabilities: err=%v data=%s", err, encoded)
	}
	if _, err := EncodeSemanticCapabilities(registry, len(encoded)-1); err == nil {
		t.Fatal("oversized capability context did not fail closed")
	}
}

func TestSemanticRegistriesRemainValidatorAuthoritative(t *testing.T) {
	for _, metric := range registeredBehavioralMetrics {
		analysis, unit, ok := behavioralMetricContract(metric.Metric)
		if !ok || analysis != metric.Analysis || unit != metric.Unit {
			t.Fatalf("metric contract %q = %q/%q/%t", metric.Metric, analysis, unit, ok)
		}
	}
	for _, axis := range registeredOperatingAxes {
		unit, selection := operatingAxisContract(axis.Axis)
		if unit != axis.Unit || selection != axis.Selection {
			t.Fatalf("axis contract %q = %q/%t", axis.Axis, unit, selection)
		}
	}
}
