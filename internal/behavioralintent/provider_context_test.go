package behavioralintent

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"kicadai/internal/architecturesearch"
)

func TestBuildProviderContextIsDeterministicAndFailClosed(t *testing.T) {
	capabilities := testInstalledCapabilities(t)
	first, err := BuildProviderContext("Filter at 20 kHz. Add gain.", capabilities)
	if err != nil {
		t.Fatal(err)
	}
	second, err := BuildProviderContext("Filter at 20 kHz. Add gain.", capabilities)
	if err != nil || first != second {
		t.Fatalf("provider context is not deterministic: err=%v", err)
	}
	var context ProviderContext
	if err := json.Unmarshal([]byte(first), &context); err != nil {
		t.Fatal(err)
	}
	if context.TargetContract != "kicadai.open-set-requirement.v3" || len(context.Source.Statements) != 2 || len(context.Policy) == 0 || len(context.CapabilitySHA256) != 64 {
		t.Fatalf("provider context = %#v", context)
	}
	if _, err := BuildProviderContext("", capabilities); !errors.Is(err, ErrSourceUnavailable) {
		t.Fatalf("empty source error = %v", err)
	}
	if _, err := BuildProviderContext("Filter at 20 kHz.", nil); !errors.Is(err, ErrCapabilitiesUnavailable) {
		t.Fatalf("missing capabilities error = %v", err)
	}
	if _, err := BuildProviderContext("Filter at 20 kHz.", json.RawMessage(`{"broken"`)); !errors.Is(err, ErrCapabilitiesUnavailable) {
		t.Fatalf("malformed capabilities error = %v", err)
	}
	if _, err := BuildProviderContext("Filter at 20 kHz.", json.RawMessage(`{"schema":"invented"}`)); !errors.Is(err, ErrCapabilitiesUnavailable) {
		t.Fatalf("untrusted capability object error = %v", err)
	}
}

func TestBuildInstalledCapabilitiesRequiresCatalogModelsAndAnalyses(t *testing.T) {
	registry, issues := architecturesearch.NewRegistry(capabilityTestProvider{})
	if len(issues) != 0 {
		t.Fatalf("registry issues = %#v", issues)
	}
	architecture, err := architecturesearch.EncodeSemanticCapabilities(registry, 0)
	if err != nil {
		t.Fatal(err)
	}
	analyses := testTrustedAnalyses()
	snapshot, err := BuildInstalledCapabilities(architecture, testCapabilitySHA256, testCapabilitySHA256, append(analyses, "ac_sweep"))
	if err != nil || !json.Valid(snapshot) {
		t.Fatalf("snapshot: err=%v data=%s", err, snapshot)
	}
	var decoded InstalledCapabilities
	if err := json.Unmarshal(snapshot, &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.TrustedAnalyses) != len(analyses) || decoded.TrustedAnalyses[0] != "ac_sweep" || decoded.CatalogSHA256 != testCapabilitySHA256 {
		t.Fatalf("snapshot = %#v", decoded)
	}
	for _, test := range []struct {
		architecture json.RawMessage
		catalog      string
		models       string
		analyses     []string
	}{
		{architecture: nil, catalog: testCapabilitySHA256, models: testCapabilitySHA256, analyses: analyses},
		{architecture: architecture, catalog: "", models: testCapabilitySHA256, analyses: analyses},
		{architecture: architecture, catalog: testCapabilitySHA256, models: "", analyses: analyses},
		{architecture: architecture, catalog: testCapabilitySHA256, models: testCapabilitySHA256},
		{architecture: json.RawMessage(`{"schema":"invented"}`), catalog: testCapabilitySHA256, models: testCapabilitySHA256, analyses: analyses},
	} {
		if _, err := BuildInstalledCapabilities(test.architecture, test.catalog, test.models, test.analyses); !errors.Is(err, ErrCapabilitiesUnavailable) {
			t.Fatalf("invalid snapshot error = %v", err)
		}
	}
}

type capabilityTestProvider struct{}

func (capabilityTestProvider) Descriptor() architecturesearch.ProviderDescriptor {
	return architecturesearch.ProviderDescriptor{
		ID: "test_filter", Revision: "1.0.0", Capabilities: []string{"frequency_filter"},
		Evidence: architecturesearch.ContractEvidence{Confidence: architecturesearch.EvidenceVerified, Sources: []string{"test"}},
	}
}

func (capabilityTestProvider) Expand(context.Context, architecturesearch.ProviderRequest) ([]architecturesearch.ProviderExpansion, error) {
	return nil, nil
}

func testTrustedAnalyses() []string {
	return []string{"ac_sweep", "dc_operating_point", "distortion", "noise", "stability", "startup", "thermal", "transient"}
}

func testInstalledCapabilities(t *testing.T) json.RawMessage {
	t.Helper()
	registry, issues := architecturesearch.NewRegistry(capabilityTestProvider{})
	if len(issues) != 0 {
		t.Fatalf("registry issues = %#v", issues)
	}
	architecture, err := architecturesearch.EncodeSemanticCapabilities(registry, 0)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := BuildInstalledCapabilities(architecture, testCapabilitySHA256, testCapabilitySHA256, testTrustedAnalyses())
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}
