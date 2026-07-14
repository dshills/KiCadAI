package circuitgraph

import (
	"strings"
	"testing"

	"kicadai/internal/components"
)

func TestProviderCapabilityContextIsDeterministicAndBounded(t *testing.T) {
	catalog := loadGraphCatalog(t)
	first, err := ProviderCapabilityContext(catalog, 64<<10)
	if err != nil {
		t.Fatal(err)
	}
	second, err := ProviderCapabilityContext(catalog, 64<<10)
	if err != nil {
		t.Fatal(err)
	}
	if first != second || !strings.Contains(first, "sensor.bosch.bmp280.lga8") || !strings.Contains(first, "define every region named by a PCB placement") {
		t.Fatalf("capability is not deterministic or complete: %s", first)
	}
	if _, err := ProviderCapabilityContext(catalog, 10); err == nil {
		t.Fatal("expected bounded capability failure")
	}
}

func TestProviderCapabilityContextIncludesNamedUnitFunctions(t *testing.T) {
	catalog := loadGraphCatalog(t)
	found := false
	for recordIndex := range catalog.Records {
		record := &catalog.Records[recordIndex]
		if record.ID != "opamp.ti.lmv321.sot23_5" {
			continue
		}
		found = true
		record.Symbols[0].Unit = 1
		record.Symbols[0].UnitID = "A"
		record.Symbols[0].UnitType = components.SymbolUnitFunctional
		record.Symbols[0].RequiredUnit = true
	}
	if !found {
		t.Fatal("LMV321 catalog record not found")
	}
	capability, err := ProviderCapabilityContext(catalog, 64<<10)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"units":[{"id":"A","type":"functional","required":true`, `"functions":["IN_MINUS","IN_PLUS","OUT","V_MINUS","V_PLUS"]`} {
		if !strings.Contains(capability, want) {
			t.Fatalf("capability missing %s: %s", want, capability)
		}
	}
}
