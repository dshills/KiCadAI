package circuitgraph

import (
	"os"
	"strings"
	"testing"

	"kicadai/internal/kicadfiles/checks"
	"kicadai/internal/libraryresolver"
	"kicadai/internal/testsupport/tolerancecorpus"
)

func TestFrozenToleranceCorpusOptionalKiCadPromotion(t *testing.T) {
	cliPath := strings.TrimSpace(os.Getenv(checks.EnvKiCadCLI))
	symbolsRoot := strings.TrimSpace(os.Getenv(libraryresolver.EnvSymbolsRoot))
	footprintsRoot := strings.TrimSpace(os.Getenv(libraryresolver.EnvFootprintsRoot))
	if cliPath == "" || symbolsRoot == "" || footprintsRoot == "" {
		t.Skipf("set %s, %s, and %s to run tolerance-corpus KiCad promotion", checks.EnvKiCadCLI, libraryresolver.EnvSymbolsRoot, libraryresolver.EnvFootprintsRoot)
	}

	manifest, _, err := tolerancecorpus.Load()
	if err != nil {
		t.Fatal(err)
	}
	functionFixtures := map[string]bool{}
	adversarialFixtures := map[string]bool{}
	passingCases := 0
	for _, entry := range manifest.Cases {
		if entry.Expected != "worst_case_pass" {
			continue
		}
		passingCases++
		switch entry.PromotionCorpus {
		case "function":
			functionFixtures[entry.PromotionFixture] = true
		case "adversarial":
			adversarialFixtures[entry.PromotionFixture] = true
		default:
			t.Fatalf("passing tolerance case %s has unsupported promotion corpus %q", entry.ID, entry.PromotionCorpus)
		}
	}
	if passingCases == 0 {
		t.Fatal("tolerance corpus contains no reported passes")
	}

	if len(functionFixtures) != 0 {
		t.Run("function_carriers", func(t *testing.T) {
			runFunctionLevelCorpusKiCadPromotion(t, functionFixtures)
		})
	}
	if len(adversarialFixtures) != 0 {
		t.Run("adversarial_carriers", func(t *testing.T) {
			report := evaluateAdversarialCorpus(t, true, adversarialFixtures)
			if report.Aggregate.Circuits != len(adversarialFixtures) || report.Aggregate.Blocked != 0 {
				t.Fatalf("tolerance promotion carriers did not pass complete KiCad gates: %#v", report)
			}
		})
	}
}
