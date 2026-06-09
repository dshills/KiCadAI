package workflows

import (
	"errors"
	"strings"
	"testing"

	"kicadai/internal/kiapi"
	"kicadai/internal/schematic"
)

func TestPlanLEDDemo(t *testing.T) {
	plan, err := PlanLEDDemo(LEDDemoIntent{
		Document: schematic.DocumentRef{Type: kiapi.DocumentTypeSchematic, Identifier: "/"},
		Origin:   schematic.Point{X: 1000, Y: 2000},
		Prefix:   "STATUS",
	})
	if err != nil {
		t.Fatalf("PlanLEDDemo returned error: %v", err)
	}
	if len(plan.Operations) != 8 {
		t.Fatalf("operation count = %d, want 8", len(plan.Operations))
	}
	if plan.Operations[0].Kind != schematic.OpKindAddSymbol {
		t.Fatalf("first operation = %q", plan.Operations[0].Kind)
	}
	if plan.Operations[len(plan.Operations)-1].Kind != schematic.OpKindAddLabel {
		t.Fatalf("last operation = %q", plan.Operations[len(plan.Operations)-1].Kind)
	}
}

func TestPlanLEDDemoRequiresDocument(t *testing.T) {
	_, err := PlanLEDDemo(LEDDemoIntent{})
	if !errors.Is(err, schematic.ErrMissingDocument) {
		t.Fatalf("PlanLEDDemo error = %v, want %v", err, schematic.ErrMissingDocument)
	}
}

func TestPlanLEDDemoLibraryOverrides(t *testing.T) {
	plan, err := PlanLEDDemo(LEDDemoIntent{
		Document: schematic.DocumentRef{Type: kiapi.DocumentTypeSchematic, Identifier: "/"},
		Libraries: LEDDemoLibraries{
			Resistor: "Custom:R_US",
		},
	})
	if err != nil {
		t.Fatalf("PlanLEDDemo returned error: %v", err)
	}
	if got := plan.Operations[1].Summary; !strings.Contains(got, "Custom:R_US") {
		t.Fatalf("resistor summary = %q", got)
	}
}

func TestPlanLEDDemoSanitizesPrefix(t *testing.T) {
	plan, err := PlanLEDDemo(LEDDemoIntent{
		Document: schematic.DocumentRef{Type: kiapi.DocumentTypeSchematic, Identifier: "/"},
		Prefix:   "status led!",
	})
	if err != nil {
		t.Fatalf("PlanLEDDemo returned error: %v", err)
	}
	if got := plan.Operations[len(plan.Operations)-1].Summary; !strings.Contains(got, "status_led_OUT") {
		t.Fatalf("label summary = %q", got)
	}
}

func TestPlanLEDDemoUsesDefaultPrefixWhenSanitizedPrefixIsEmpty(t *testing.T) {
	plan, err := PlanLEDDemo(LEDDemoIntent{
		Document: schematic.DocumentRef{Type: kiapi.DocumentTypeSchematic, Identifier: "/"},
		Prefix:   " ! ",
	})
	if err != nil {
		t.Fatalf("PlanLEDDemo returned error: %v", err)
	}
	if len(plan.Operations) == 0 {
		t.Fatalf("expected at least one operation")
	}
	if got := plan.Operations[len(plan.Operations)-1].Summary; !strings.Contains(got, DefaultLEDDemoPrefix+"_OUT") {
		t.Fatalf("label summary = %q", got)
	}
}
