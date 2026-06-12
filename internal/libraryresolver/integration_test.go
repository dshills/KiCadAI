package libraryresolver

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"kicadai/internal/reports"
)

const libraryIntegrationTimeout = 2 * time.Minute

var integrationIndexOnce sync.Once
var integrationIndex LibraryIndex
var integrationIndexIssues []reports.Issue

func TestLibraryResolverIntegrationKnownLookups(t *testing.T) {
	index := loadIntegrationIndex(t)
	for _, id := range []string{"Device:R", "Device:C", "Device:LED"} {
		t.Run(id, func(t *testing.T) {
			if _, ok := ResolveSymbol(index, id); !ok {
				t.Errorf("missing symbol %s", id)
			}
		})
	}
	for _, id := range []string{
		"Resistor_SMD:R_0805_2012Metric",
		"Connector_PinHeader_2.54mm:PinHeader_1x02_P2.54mm_Vertical",
	} {
		t.Run(id, func(t *testing.T) {
			if _, ok := ResolveFootprint(index, id); !ok {
				t.Errorf("missing footprint %s", id)
			}
		})
	}
}

func TestLibraryResolverIntegrationCompatibility(t *testing.T) {
	index := loadIntegrationIndex(t)
	for _, tc := range []struct {
		symbol    string
		footprint string
	}{
		{symbol: "Device:R", footprint: "Resistor_SMD:R_0805_2012Metric"},
		{symbol: "Connector_Generic:Conn_01x02", footprint: "Connector_PinHeader_2.54mm:PinHeader_1x02_P2.54mm_Vertical"},
		{symbol: "Device:Q_NPN_BEC", footprint: "Package_TO_SOT_THT:TO-92_Inline"},
	} {
		t.Run(tc.symbol+"_"+tc.footprint, func(t *testing.T) {
			result := ValidateAssignment(index, tc.symbol, tc.footprint)
			if result.Status == CompatibilityIncompatible || result.Status == CompatibilityUnknown {
				t.Errorf("%s -> %s status = %s issues=%#v", tc.symbol, tc.footprint, result.Status, result.Issues)
			}
		})
	}
}

func TestLibraryResolverIntegrationTemplates(t *testing.T) {
	roots := integrationRoots(t)
	ctx, cancel := context.WithTimeout(context.Background(), libraryIntegrationTimeout)
	defer cancel()
	records, issues := DiscoverTemplates(ctx, roots)
	if hasBlockingIntegrationIssue(issues) {
		t.Fatalf("template issues = %#v", issues)
	}
	if len(records) == 0 {
		t.Fatalf("expected at least one template")
	}
}

func loadIntegrationIndex(t *testing.T) LibraryIndex {
	t.Helper()
	roots := integrationRoots(t)
	integrationIndexOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), libraryIntegrationTimeout)
		defer cancel()
		integrationIndex, integrationIndexIssues = Load(ctx, roots, LoadOptions{})
	})
	if hasBlockingIntegrationIssue(integrationIndexIssues) {
		t.Fatalf("integration load issues = %#v", integrationIndexIssues)
	}
	return integrationIndex
}

func integrationRoots(t *testing.T) LibraryRoots {
	t.Helper()
	if os.Getenv("KICADAI_RUN_LIBRARY_INTEGRATION") != "1" {
		t.Skip("set KICADAI_RUN_LIBRARY_INTEGRATION=1 to run local KiCad library integration tests")
	}
	roots := LibraryRoots{
		KLCRoot:        os.Getenv(EnvKLCRoot),
		SymbolsRoot:    os.Getenv(EnvSymbolsRoot),
		FootprintsRoot: os.Getenv(EnvFootprintsRoot),
		TemplatesRoot:  os.Getenv(EnvTemplatesRoot),
	}
	missing := []string{}
	if roots.SymbolsRoot == "" {
		missing = append(missing, EnvSymbolsRoot)
	}
	if roots.FootprintsRoot == "" {
		missing = append(missing, EnvFootprintsRoot)
	}
	if roots.KLCRoot == "" {
		missing = append(missing, EnvKLCRoot)
	}
	if roots.TemplatesRoot == "" {
		missing = append(missing, EnvTemplatesRoot)
	}
	if len(missing) != 0 {
		t.Fatalf("missing integration environment variables: %v", missing)
	}
	return roots
}

func hasBlockingIntegrationIssue(issues []reports.Issue) bool {
	for _, issue := range issues {
		if issue.Blocking() {
			return true
		}
	}
	return false
}
