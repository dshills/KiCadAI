package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"kicadai/internal/capabilityevaluation"
)

func main() {
	if err := run(os.Args[1:], os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(arguments []string, stderr io.Writer) error {
	flags := flag.NewFlagSet("kicadai-capability-eval", flag.ContinueOnError)
	flags.SetOutput(stderr)
	mode := flags.String("mode", "evaluate", "evaluate, compare, explain, or affected")
	corpusPath := flags.String("corpus", "", "frozen behavior corpus JSON")
	evidencePath := flags.String("evidence", "", "terminal evidence JSON")
	registryPath := flags.String("registry", "", "reviewed capability-impact registry JSON")
	baselinePath := flags.String("baseline", "", "baseline capability report JSON")
	finalPath := flags.String("final", "", "final capability report JSON")
	promotionsPath := flags.String("promotions", "", "physical promotion evidence JSON")
	reportPath := flags.String("report", "", "capability report JSON")
	capability := flags.String("capability", "", "semantic capability")
	rank := flags.Int("rank", 0, "positive cluster rank")
	requiredCapabilities := flags.String("required-capabilities", "", "comma-separated capabilities required to improve")
	outputPath := flags.String("output", "", "output report JSON")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if flags.NArg() != 0 || *outputPath == "" {
		return usageError()
	}
	switch *mode {
	case "evaluate":
		return runEvaluate(*corpusPath, *evidencePath, *registryPath, *outputPath)
	case "compare":
		return runCompare(*baselinePath, *finalPath, *promotionsPath, *requiredCapabilities, *outputPath)
	case "explain":
		return runExplain(*reportPath, *capability, *rank, *outputPath)
	case "affected":
		return runAffected(*reportPath, *capability, *outputPath)
	default:
		return usageError()
	}
}

func usageError() error {
	return fmt.Errorf("usage: kicadai-capability-eval -mode evaluate|compare|explain|affected [mode inputs] -output PATH")
}

func runEvaluate(corpusPath, evidencePath, registryPath, outputPath string) error {
	if corpusPath == "" || evidencePath == "" || registryPath == "" {
		return usageError()
	}
	corpus, err := capabilityevaluation.LoadCorpus(corpusPath)
	if err != nil {
		return err
	}
	evidence, err := capabilityevaluation.LoadEvidenceSet(evidencePath)
	if err != nil {
		return err
	}
	registry, err := capabilityevaluation.LoadImpactRegistry(registryPath)
	if err != nil {
		return err
	}
	report, err := capabilityevaluation.EvaluateEvidenceSet(
		corpus, evidence, registry, capabilityevaluation.DefaultRankingPolicy(),
	)
	if err != nil {
		return err
	}
	data, err := report.MarshalJSONStable()
	if err != nil {
		return fmt.Errorf("marshal capability evaluation report: %w", err)
	}
	data = append(data, '\n')
	if err := writeAtomic(outputPath, data); err != nil {
		return fmt.Errorf("write capability evaluation report: %w", err)
	}
	return nil
}

func runCompare(baselinePath, finalPath, promotionsPath, requiredCapabilities, outputPath string) error {
	if baselinePath == "" || finalPath == "" || promotionsPath == "" {
		return usageError()
	}
	baseline, err := capabilityevaluation.LoadReport(baselinePath)
	if err != nil {
		return err
	}
	final, err := capabilityevaluation.LoadReport(finalPath)
	if err != nil {
		return err
	}
	promotions, err := capabilityevaluation.LoadPromotionEvidenceSet(promotionsPath)
	if err != nil {
		return err
	}
	if promotions.CorpusRole != final.CorpusRole || promotions.CorpusSHA256 != final.CorpusSHA256 {
		return fmt.Errorf(
			"promotion evidence corpus identity does not match final report: evidence role=%q sha256=%q, report role=%q sha256=%q",
			promotions.CorpusRole, promotions.CorpusSHA256, final.CorpusRole, final.CorpusSHA256,
		)
	}
	var required []string
	for _, value := range strings.Split(requiredCapabilities, ",") {
		if value = strings.TrimSpace(value); value != "" {
			required = append(required, value)
		}
	}
	improvement, err := capabilityevaluation.VerifyImprovement(baseline, final, promotions.Cases, required)
	if err != nil {
		return err
	}
	return writeJSONAtomic(outputPath, improvement)
}

func runExplain(reportPath, capability string, rank int, outputPath string) error {
	if reportPath == "" {
		return usageError()
	}
	report, err := capabilityevaluation.LoadReport(reportPath)
	if err != nil {
		return err
	}
	cluster, err := capabilityevaluation.ExplainCluster(report, capability, rank)
	if err != nil {
		return err
	}
	return writeJSONAtomic(outputPath, cluster)
}

func runAffected(reportPath, capability, outputPath string) error {
	if reportPath == "" || capability == "" {
		return usageError()
	}
	report, err := capabilityevaluation.LoadReport(reportPath)
	if err != nil {
		return err
	}
	affected, err := capabilityevaluation.ListCasesAffectedByCapability(report, capability)
	if err != nil {
		return err
	}
	return writeJSONAtomic(outputPath, affected)
}

func writeJSONAtomic(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(path, append(data, '\n'))
}

func writeAtomic(path string, data []byte) error {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return err
	}
	file, err := os.CreateTemp(directory, "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	name := file.Name()
	defer os.Remove(name)
	if err := file.Chmod(0o644); err != nil {
		return errors.Join(err, file.Close())
	}
	if _, err := file.Write(data); err != nil {
		return errors.Join(err, file.Close())
	}
	if err := file.Sync(); err != nil {
		return errors.Join(err, file.Close())
	}
	if closeErr := file.Close(); closeErr != nil {
		return fmt.Errorf("close atomic output before rename: %w", closeErr)
	}
	if renameErr := os.Rename(name, path); renameErr != nil {
		return fmt.Errorf("replace atomic output: %w", renameErr)
	}
	return nil
}
