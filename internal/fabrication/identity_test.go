package fabrication

import (
	"testing"

	"kicadai/internal/reports"
)

func TestNormalizeComponentIdentityTrimsAndInfersSource(t *testing.T) {
	identity := NormalizeComponentIdentity(ComponentIdentity{
		Reference:      " R1 ",
		Manufacturer:   " Yageo ",
		MPN:            " RC0603FR-0710KL ",
		ComponentClass: " Passive ",
	})
	if identity.Reference != "R1" || identity.Manufacturer != "Yageo" || identity.MPN != "RC0603FR-0710KL" {
		t.Fatalf("identity = %#v", identity)
	}
	if identity.ComponentClass != "passive" || identity.Source != IdentitySourceSchematicProperty || identity.Status != IdentityPass || !identity.ExactPartPresent {
		t.Fatalf("identity evidence = %#v", identity)
	}
}

func TestNormalizeComponentIdentityRequiresExactPart(t *testing.T) {
	identity := NormalizeComponentIdentity(ComponentIdentity{
		Reference:         "U1",
		Value:             "MCU",
		ExactPartRequired: true,
	})
	if identity.Status != IdentityMissing || identity.Source != IdentitySourceSchematicProperty || identity.ExactPartPresent {
		t.Fatalf("identity = %#v", identity)
	}
}

func TestNormalizeComponentIdentityUsesValueAndFootprintAsEvidence(t *testing.T) {
	identity := NormalizeComponentIdentity(ComponentIdentity{
		Reference:   "R1",
		Value:       "10k",
		FootprintID: "Resistor_SMD:R_0603",
	})
	if identity.Source != IdentitySourceSchematicProperty || identity.Status != IdentityWarning {
		t.Fatalf("identity = %#v", identity)
	}
}

func TestNormalizeComponentIdentityComponentIDCountsAsExactPart(t *testing.T) {
	identity := NormalizeComponentIdentity(ComponentIdentity{
		Reference:         "U1",
		ComponentID:       "mcu.atmega328p-au",
		ExactPartRequired: true,
	})
	if !identity.ExactPartPresent || identity.Status != IdentityPass {
		t.Fatalf("identity = %#v", identity)
	}
}

func TestNormalizeComponentIdentityUsesIssueSeverity(t *testing.T) {
	identity := NormalizeComponentIdentity(ComponentIdentity{
		Reference:    "J1",
		Manufacturer: "Example",
		MPN:          "EX-1",
		Issues: []reports.Issue{{
			Code:     reports.CodeValidationFailed,
			Severity: reports.SeverityBlocked,
			Message:  "conflict",
		}},
	})
	if identity.Status != IdentityFail {
		t.Fatalf("identity = %#v", identity)
	}
	total, blocking := IdentityIssueCounts(identity.Issues)
	if total != 1 || blocking != 1 {
		t.Fatalf("issue counts = %d/%d", total, blocking)
	}
}
