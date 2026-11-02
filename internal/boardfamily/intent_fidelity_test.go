package boardfamily

import (
	"bytes"
	"strings"
	"testing"
)

// These are independently worded synthetic regressions, not live model results.
func TestFidelityExplicitAcceptanceAndRestrictions(t *testing.T) {
	for _, prompt := range []string{
		"Use BMP280. The fast profile is acceptable.",
		"Use SHT31. The standard profile is fine.",
	} {
		t.Run(prompt, func(t *testing.T) {
			_, f := coverageFixture(t, prompt)
			f["version"] = FidelityEvidenceVersion
			d, err := DecodeFidelityEvidenceIntent(prompt, addressedJSON(t, f))
			if err != nil || d.Disposition != "supported" {
				t.Fatal(d, err)
			}
			if strings.Contains(prompt, "fast") && d.Configuration.Profile != "fast" {
				t.Fatal("accepted profile lost")
			}
		})
	}
	for _, restriction := range []string{
		"Do not omit any requested capability.",
		"No external converter may be added.",
		"Neither requirement may be changed.",
	} {
		prompt := "Use SHT31 with wireless telemetry. " + restriction
		input, f := coverageFixture(t, prompt)
		f["version"] = FidelityEvidenceVersion
		r := input.Residuals[len(input.Residuals)-1]
		f["coverage"].(map[string]any)[r.ID] = coverageConstraint(input.CoverageReferences[r.ID], "requested")
		raw := addressedJSON(t, f)
		before := bytes.Clone(raw)
		compiled, err := CompileFidelityEvidence(prompt, raw)
		if err != nil || !bytes.Contains(compiled, []byte(restriction)) {
			t.Fatal("restriction lost", err)
		}
		d, err := DecodeFidelityEvidenceIntent(prompt, raw)
		if err != nil || d.Disposition != "unsupported" || !bytes.Equal(raw, before) {
			t.Fatal(d, err)
		}
	}
}

func TestFidelityRefusalDoesNotPrescribeSubstitution(t *testing.T) {
	prompt := "Use BMP280 with USB power and wireless telemetry. No external adapter is allowed."
	input, f := coverageFixture(t, prompt)
	r := input.Residuals[len(input.Residuals)-1]
	f["coverage"].(map[string]any)[r.ID] = coverageConstraint(input.CoverageReferences[r.ID], "requested")
	f["version"] = CoverageEvidenceVersion
	legacy, err := DecodeCoverageEvidenceIntent(prompt, addressedJSON(t, f))
	if err != nil || !strings.Contains(legacy.Message, "use external regulated") {
		t.Fatal("legacy changed", legacy, err)
	}
	f["version"] = FidelityEvidenceVersion
	raw := addressedJSON(t, f)
	before := bytes.Clone(raw)
	d, err := DecodeFidelityEvidenceIntent(prompt, raw)
	if err != nil || d.Disposition != "unsupported" || d.Configuration != nil {
		t.Fatal(d, err)
	}
	if strings.Contains(d.Message, "use external") || !strings.Contains(d.Message, "No USB") || !strings.Contains(d.Message, "Wireless operation is unsupported") || !strings.Contains(d.Message, "No external adapter") {
		t.Fatal("refusal changed requirements", d)
	}
	for _, c := range d.Clauses {
		if strings.Contains(c.Reason, "use external") {
			t.Fatal("clause still prescribes substitute")
		}
	}
	if !bytes.Equal(before, raw) {
		t.Fatal("raw evidence modified")
	}
	if _, err := DecodeCoverageEvidenceIntent(prompt, raw); err == nil {
		t.Fatal("v12 accepted by v11")
	}
}

func TestFidelityInstructionsDoNotChangeFrozenCoverage(t *testing.T) {
	old, _, err := prepareCoverageGenerateRequest("Use SHT31.")
	if err != nil {
		t.Fatal(err)
	}
	new, _, err := prepareFidelityGenerateRequest("Use SHT31.")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(old.CapabilityContext, "Scope distinctions:") || !strings.Contains(new.CapabilityContext, "Scope distinctions:") {
		t.Fatal("prompt isolation failed")
	}
	if !strings.Contains(new.CapabilityContext, "Accepting a specific named choice") || !strings.Contains(new.CapabilityContext, "no-substitution restriction") || !strings.Contains(new.CapabilityContext, "detail must ask the targeted question") {
		t.Fatal("scope instructions absent")
	}
}

func TestFidelityExplanationPreservesQuotedCatalogText(t *testing.T) {
	quote := intentFeatureReasons["usb"]
	prompt := "Use SHT31. Which label should be used?"
	input, f := coverageFixture(t, prompt)
	f["version"] = FidelityEvidenceVersion
	// An adversarial untrusted detail contains text identical to a catalog
	// reason. This is not a semantically correct model fixture: the property
	// under test is that application explanation policy cannot rewrite it.
	for _, r := range input.Residuals {
		if r.ClauseID > 0 {
			f["coverage"].(map[string]any)[r.ID] = map[string]any{
				"kind": "constraints", "references": input.CoverageReferences[r.ID],
				"facts": []any{map[string]any{"kind": "unclear", "state": "unresolved_choice", "context": []string{}, "detail": quote}},
			}
		}
	}
	raw := addressedJSON(t, f)
	before := bytes.Clone(raw)
	d, err := DecodeFidelityEvidenceIntent(prompt, raw)
	if err != nil || d.Disposition != "clarify" {
		t.Fatal(d, err)
	}
	// The source clause retains the catalog-looking string verbatim.
	if !strings.Contains(d.Message, quote) {
		t.Fatal("source quotation was rewritten", d.Message)
	}
	if !bytes.Equal(before, raw) {
		t.Fatal("raw source evidence changed")
	}
}
