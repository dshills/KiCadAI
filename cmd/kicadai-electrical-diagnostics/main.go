// This developer diagnostic command is not a public capability evaluator. It
// observes the unchanged V21 boundary and never publishes promotion/pass claims.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"kicadai/internal/capabilitybaselinev10"
	"kicadai/internal/capabilityexecutorv10"
	"kicadai/internal/circuitgraph"
	"kicadai/internal/components"
	"kicadai/internal/electricaldiagnostics"
	"kicadai/internal/modelprovenance"
	ot "kicadai/internal/opentopologysynthesis"
	"kicadai/internal/reports"
)

type options struct {
	repository, corpus, baseline, output, cases string
	timeout                                     time.Duration
}

type measurement struct {
	CaseID             string  `json:"case_id"`
	RequirementHash    string  `json:"requirement_sha256"`
	ExpectedReplayHash string  `json:"expected_replay_sha256"`
	ReplayHash         string  `json:"replay_sha256"`
	ReplayMatches      bool    `json:"replay_matches"`
	TraceHash          string  `json:"trace_file_sha256"`
	TraceBytes         int     `json:"trace_bytes"`
	RawSerializedBytes int64   `json:"raw_serialized_bytes"`
	SynthesisSeconds   float64 `json:"synthesis_seconds"`
	ProjectionSeconds  float64 `json:"projection_seconds"`
}

type receipt struct {
	Schema                     string        `json:"schema"`
	SourceCommit               string        `json:"source_commit"`
	GoVersion                  string        `json:"go_version"`
	Platform                   string        `json:"platform"`
	CorpusHash                 string        `json:"corpus_manifest_sha256"`
	BaselineHash               string        `json:"baseline_file_sha256"`
	Policy                     ot.Policy     `json:"policy"`
	RunsPerCase                int           `json:"runs_per_case"`
	PhysicalPromotionPerformed bool          `json:"physical_promotion_performed"`
	Cases                      []measurement `json:"cases"`
}

func main() {
	if err := run(context.Background(), os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(parent context.Context, args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("kicadai-electrical-diagnostics", flag.ContinueOnError)
	var opts options
	flags.StringVar(&opts.repository, "repository-root", ".", "clean committed repository")
	flags.StringVar(&opts.corpus, "corpus-root", "", "authenticated public discovery corpus")
	flags.StringVar(&opts.baseline, "baseline-report", "", "committed public V21 maintenance comparison")
	flags.StringVar(&opts.output, "output-root", "", "fresh absolute diagnostic output root")
	flags.StringVar(&opts.cases, "cases", "", "comma-separated public case IDs; evaluated once each")
	flags.DurationVar(&opts.timeout, "timeout", 3*time.Hour, "whole diagnostic run deadline")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 || opts.timeout <= 0 || !filepath.IsAbs(opts.output) {
		return fmt.Errorf("require positive timeout, absolute fresh output root, and no positional arguments")
	}
	ids, err := caseIDs(opts.cases)
	if err != nil {
		return err
	}
	opts.repository, err = filepath.Abs(opts.repository)
	if err != nil {
		return err
	}
	if opts.corpus == "" {
		opts.corpus = filepath.Join(opts.repository, "internal/capabilityfeedback/testdata/closed_loop_open_set_v10_corpus")
	}
	if opts.baseline == "" {
		opts.baseline = filepath.Join(opts.repository, "internal/capabilityfeedback/testdata/closed_loop_open_set_v21_maintenance_1/report.json")
	}
	revision, err := cleanRevision(parent, opts.repository)
	if err != nil {
		return err
	}
	corpus, err := capabilityexecutorv10.LoadPublicDiscovery(opts.corpus)
	if err != nil {
		return err
	}
	baselineData, err := os.ReadFile(opts.baseline)
	if err != nil {
		return err
	}
	baseline, err := decodeBaseline(baselineData)
	if err != nil {
		return err
	}
	if baseline.CorpusManifestSHA256 != corpus.ManifestSHA256 {
		return fmt.Errorf("baseline corpus binding differs")
	}
	inputs := map[string]capabilityexecutorv10.CaseInput{}
	references := map[string]capabilitybaselinev10.CaseEvidence{}
	for _, input := range corpus.Cases {
		inputs[input.Entry.ID] = input
	}
	for _, record := range baseline.Cases {
		references[record.Case.ID] = record
	}
	for _, id := range ids {
		input, found := inputs[id]
		reference, recorded := references[id]
		if !found || !recorded || input.Entry.RequirementSHA256 != reference.RequirementSHA256 {
			return fmt.Errorf("case %q is not bound to the authenticated public comparison", id)
		}
	}
	if err := os.Mkdir(opts.output, 0o755); err != nil {
		return fmt.Errorf("create fresh diagnostic output: %w", err)
	}
	ctx, cancel := context.WithTimeout(parent, opts.timeout)
	defer cancel()
	legacyInventory, legacySimulation, err := loadEnvironment(ctx, false)
	if err != nil {
		return err
	}
	v18Inventory, v18Simulation, err := loadEnvironment(ctx, true)
	if err != nil {
		return err
	}
	result := receipt{
		Schema: "kicadai.electrical-diagnostic-measurements.v1", SourceCommit: revision,
		GoVersion: runtime.Version(), Platform: runtime.GOOS + "/" + runtime.GOARCH,
		CorpusHash: corpus.ManifestSHA256, BaselineHash: digest(baselineData), Policy: ot.DefaultPolicy(),
		RunsPerCase: 1, Cases: []measurement{},
	}
	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			return err
		}
		input := inputs[id]
		requirement, issues := ot.DecodeStrict(bytes.NewReader(input.RequirementSource))
		if len(issues) != 0 {
			return fmt.Errorf("public requirement %q violates frozen contract", id)
		}
		if _, err := fmt.Fprintf(stdout, "diagnostic start: %s\n", id); err != nil {
			return err
		}
		started := time.Now()
		synthesis := ot.SynthesizeV21WithLegacy(ctx, requirement, v18Inventory, v18Simulation, v18Inventory, v18Simulation, v18Inventory, v18Simulation, legacyInventory, legacySimulation, result.Policy)
		synthesisSeconds := time.Since(started).Seconds()
		if err := ctx.Err(); err != nil {
			return err
		}
		started = time.Now()
		trace, err := electricaldiagnostics.ProjectSynthesis(synthesis)
		if err != nil {
			return fmt.Errorf("project %s: %w", id, err)
		}
		data, err := electricaldiagnostics.Marshal(trace)
		if err != nil {
			return err
		}
		projectionSeconds := time.Since(started).Seconds()
		if err := writeNew(filepath.Join(opts.output, id+".trace.json"), data); err != nil {
			return err
		}
		reference := references[id]
		entry := measurement{
			CaseID: id, RequirementHash: input.Entry.RequirementSHA256, ExpectedReplayHash: reference.ReplaySHA256[0],
			ReplayHash: trace.ReplayHash, ReplayMatches: trace.ReplayHash == reference.ReplaySHA256[0],
			TraceHash: digest(data), TraceBytes: len(data), RawSerializedBytes: trace.RawSerializedBytes,
			SynthesisSeconds: synthesisSeconds, ProjectionSeconds: projectionSeconds,
		}
		if err := writeJSON(filepath.Join(opts.output, id+".measurements.json"), entry); err != nil {
			return err
		}
		result.Cases = append(result.Cases, entry)
		if _, err := fmt.Fprintf(stdout, "diagnostic complete: %s historical_replay_match=%t trace_bytes=%d raw_bytes=%d synthesis_seconds=%.3f projection_seconds=%.3f\n", id, entry.ReplayMatches, entry.TraceBytes, entry.RawSerializedBytes, synthesisSeconds, projectionSeconds); err != nil {
			return err
		}
		if !entry.ReplayMatches {
			return fmt.Errorf("%s differs from the frozen replay; retain evidence and diagnose before continuing", id)
		}
	}
	current, err := cleanRevision(parent, opts.repository)
	if err != nil || current != revision {
		return fmt.Errorf("diagnostic source changed during execution: %v", err)
	}
	return writeJSON(filepath.Join(opts.output, "MEASUREMENTS.json"), result)
}

func caseIDs(text string) ([]string, error) {
	if strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("public case IDs are required")
	}
	ids := strings.Split(text, ",")
	seen := map[string]bool{}
	for _, id := range ids {
		if id == "" || strings.TrimSpace(id) != id || filepath.Base(id) != id || id == "." || id == ".." || strings.ContainsAny(id, "/\\") || seen[id] {
			return nil, fmt.Errorf("invalid or duplicate case ID")
		}
		seen[id] = true
	}
	slices.Sort(ids)
	return ids, nil
}

func cleanRevision(ctx context.Context, root string) (string, error) {
	command := exec.CommandContext(ctx, "git", "-C", root, "status", "--porcelain=v1", "--untracked-files=all")
	status, err := command.Output()
	if err != nil || len(status) != 0 {
		return "", fmt.Errorf("diagnostics require a clean committed source tree")
	}
	command = exec.CommandContext(ctx, "git", "-C", root, "rev-parse", "HEAD")
	data, err := command.Output()
	return strings.TrimSpace(string(data)), err
}

func decodeBaseline(data []byte) (capabilitybaselinev10.Report, error) {
	var result capabilitybaselinev10.Report
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return result, err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return result, fmt.Errorf("baseline has trailing data")
	}
	if err := capabilitybaselinev10.Validate(result); err != nil {
		return result, err
	}
	for _, record := range result.Cases {
		validated, err := capabilitybaselinev10.ValidateCase(record)
		if err != nil || validated.Hash != record.Hash {
			return result, fmt.Errorf("baseline case hash differs")
		}
	}
	return result, nil
}

func loadEnvironment(ctx context.Context, v18 bool) (ot.PrimitiveInventory, ot.SimulationEnvironment, error) {
	var catalog *components.Catalog
	var err error
	if v18 {
		catalog, err = components.LoadCatalogV18(ctx)
	} else {
		catalog, err = components.LoadCatalog(ctx, components.LoadOptions{})
	}
	if err != nil {
		return ot.PrimitiveInventory{}, ot.SimulationEnvironment{}, err
	}
	var models modelprovenance.Registry
	var diagnostics []modelprovenance.Diagnostic
	if v18 {
		models, diagnostics = modelprovenance.LoadV18()
	} else {
		models, diagnostics = modelprovenance.LoadDefault()
	}
	if len(diagnostics) != 0 {
		return ot.PrimitiveInventory{}, ot.SimulationEnvironment{}, fmt.Errorf("trusted model registry failed validation")
	}
	catalogHash := circuitgraph.NewResolver(circuitgraph.ResolveOptions{Catalog: catalog}).CatalogHash()
	inventory, issues := ot.BuildPrimitiveInventory(catalog, catalogHash, models)
	if reports.HasBlockingIssue(issues) {
		return ot.PrimitiveInventory{}, ot.SimulationEnvironment{}, fmt.Errorf("trusted inventory failed validation")
	}
	return inventory, ot.SimulationEnvironment{Catalog: catalog, CatalogHash: catalogHash, ModelRegistry: models}, nil
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return writeNew(path, append(data, '\n'))
}

func writeNew(path string, data []byte) (result error) {
	temporary, err := os.CreateTemp(filepath.Dir(path), ".diagnostic-")
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, os.Remove(temporary.Name())) }()
	if _, err := temporary.Write(data); err != nil {
		return errors.Join(err, temporary.Close())
	}
	if err := temporary.Sync(); err != nil {
		return errors.Join(err, temporary.Close())
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Chmod(temporary.Name(), 0o444); err != nil {
		return err
	}
	return os.Link(temporary.Name(), path)
}

func digest(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }
