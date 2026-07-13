package circuitgraph

import (
	"strings"
	"testing"
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
