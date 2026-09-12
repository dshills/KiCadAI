// The same offline adapter is compiled against both source revisions. It does
// not call a provider, repair inputs, or declare a complete board pass.
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
	"path/filepath"
	"regexp"
	"runtime"
	"time"

	"kicadai/internal/architecturesearch"
	"kicadai/internal/circuitgraph"
	"kicadai/internal/closedloopsynthesis"
	"kicadai/internal/components"
	"kicadai/internal/compositionlowering"
	"kicadai/internal/modelprovenance"
	"kicadai/internal/reports"
)

// Populated by the source-authenticating build runner, never inferred from cwd.
var engineSource, adapterSHA256 string

func digest(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func writeNew(dir, name string, value any) error {
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return writeBytes(dir, name, append(b, '\n'))
}

func writeBytes(dir, name string, b []byte) error {
	f, err := os.OpenFile(filepath.Join(dir, name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, writeErr := f.Write(b)
	closeErr := f.Close()
	return errors.Join(writeErr, closeErr)
}

func readInput(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("input must be a regular file")
	}
	b, err := io.ReadAll(io.LimitReader(f, architecturesearch.MaxRequirementBytes+1))
	if err != nil {
		return nil, err
	}
	if len(b) > architecturesearch.MaxRequirementBytes {
		return nil, fmt.Errorf("input exceeds requirement byte limit")
	}
	return b, nil
}

func main() {
	mode := flag.String("mode", "", "snapshot, validate, electrical, or native; never calls a provider")
	input := flag.String("input", "", "immutable architecture requirement JSON")
	output := flag.String("output", "", "new evidence directory")
	timeout := flag.Duration("timeout", 20*time.Minute, "remaining case budget, at most 20m")
	flag.Parse()
	if err := run(*mode, *input, *output, *timeout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(mode, input, output string, timeout time.Duration) error {
	for _, key := range []string{"OPENAI_API_KEY", "ANTHROPIC_API_KEY", "GEMINI_API_KEY", "GOOGLE_API_KEY", "KICADAI_OPENAI_LIVE_TEST"} {
		if os.Getenv(key) != "" {
			return fmt.Errorf("offline engine refuses provider credentials/live switch")
		}
	}
	if !regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(engineSource) || !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(adapterSHA256) {
		return fmt.Errorf("source-bound build identity is required")
	}
	if mode != "snapshot" && mode != "validate" && mode != "electrical" && mode != "native" {
		return fmt.Errorf("unsupported offline mode")
	}
	if output == "" || timeout <= 0 || timeout > 20*time.Minute {
		return fmt.Errorf("new output and bounded timeout required")
	}
	if mode != "snapshot" && input == "" {
		return fmt.Errorf("immutable input required")
	}
	if err := os.Mkdir(output, 0o700); err != nil {
		return err
	}
	started := time.Now()
	if err := writeNew(output, "identity.json", map[string]any{"schema": "kicadai.completion-engine-identity.v1", "engine_source": engineSource, "adapter_sha256": adapterSHA256, "variant": engineVariant, "go": runtime.Version(), "platform": runtime.GOOS, "arch": runtime.GOARCH, "mode": mode, "started_utc": started.UTC().Format(time.RFC3339Nano), "timeout_ns": timeout.Nanoseconds(), "provider_calls": 0}); err != nil {
		return err
	}
	nativeExecuted := false
	inputDigest := ""
	finish := func(status, gate string) error {
		if inputDigest != "" {
			current, err := readInput(input)
			if err != nil || digest(current) != inputDigest {
				return fmt.Errorf("input changed during execution")
			}
		}
		return writeNew(output, "result.json", map[string]any{"schema": "kicadai.completion-engine-result.v1", "status": status, "first_failed_gate": gate, "wall_seconds": time.Since(started).Seconds(), "complete_board_pass": false, "native_executed": nativeExecuted, "provider_calls": 0})
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	var requirement architecturesearch.Requirement
	if mode != "snapshot" {
		b, err := readInput(input)
		if err != nil {
			return err
		}
		if err := writeBytes(output, "input.json", b); err != nil {
			return err
		}
		inputDigest = digest(b)
		if err := writeNew(output, "input-identity.json", map[string]any{"sha256": digest(b), "bytes": len(b)}); err != nil {
			return err
		}
		var issues []reports.Issue
		requirement, issues = architecturesearch.DecodeStrict(bytes.NewReader(b))
		if err := writeNew(output, "validation.json", issues); err != nil {
			return err
		}
		if reports.HasBlockingIssue(issues) {
			return finish("rejected", "requirement_decode_or_validation")
		}
		if err := writeNew(output, "normalized-requirement.json", requirement); err != nil {
			return err
		}
		if mode == "validate" {
			return finish("validated", "")
		}
	}
	catalog, err := components.LoadCatalog(ctx, components.LoadOptions{})
	if err != nil {
		return err
	}
	registry, issues := architecturesearch.NewCatalogRegistry(catalog)
	if err := writeNew(output, "registry-issues.json", issues); err != nil {
		return err
	}
	if reports.HasBlockingIssue(issues) {
		return finish("rejected", "catalog_registry")
	}
	models, diagnostics := modelprovenance.LoadDefault()
	if err := writeNew(output, "model-diagnostics.json", diagnostics); err != nil {
		return err
	}
	if len(diagnostics) != 0 {
		return finish("rejected", "model_registry")
	}
	modelHash, err := modelprovenance.Hash(models)
	if err != nil {
		return err
	}
	opts := circuitgraph.ResolveOptions{Catalog: catalog, CatalogID: "installed"}
	configureGraph(&opts)
	resolver := circuitgraph.NewResolver(opts)
	semantic, err := registry.SemanticCapabilities()
	if err != nil {
		return err
	}
	if err := writeNew(output, "capabilities.json", semantic); err != nil {
		return err
	}
	if err := writeNew(output, "snapshot.json", map[string]any{"catalog_sha256": resolver.CatalogHash(), "model_registry_sha256": modelHash, "architecture_registry_sha256": registry.Hash(), "policy": closedloopsynthesis.DefaultPolicy()}); err != nil {
		return err
	}
	if mode == "snapshot" {
		return finish("snapshot", "")
	}
	search := architecturesearch.Search(ctx, requirement, registry, architecturesearch.SearchOptions{CatalogHash: resolver.CatalogHash()})
	if err := writeNew(output, "search.json", search); err != nil {
		return err
	}
	if search.Status != architecturesearch.SearchSelected {
		return finish("rejected", "architecture_search")
	}
	plans := compositionlowering.ArchitectureSimulationPlanResolver{GraphResolver: resolver, ProvenanceRegistry: models}
	configurePromotion(&plans)
	promotion, issues := compositionlowering.SynthesizeClosedLoop(ctx, requirement, search, plans, modelHash, nil, closedloopsynthesis.DefaultPolicy())
	if err := writeNew(output, "promotion.json", promotion); err != nil {
		return err
	}
	if err := writeNew(output, "promotion-issues.json", issues); err != nil {
		return err
	}
	if reports.HasBlockingIssue(issues) || promotion.Report.Status != "pass" {
		return finish("rejected", "electrical_synthesis")
	}
	if promotion.Request.ExplicitCircuit == nil || promotion.Request.ExplicitCircuit.ClosedLoop == nil || promotion.Request.ExplicitCircuit.ClosedLoop.SelectedCircuitHash != promotion.Request.ExplicitCircuit.ResolutionHash {
		return finish("rejected", "electrical_binding")
	}
	if mode == "electrical" {
		return finish("electrical_candidate", "")
	}
	nativeExecuted = true
	gate, err := nativeWorkflow(ctx, output, promotion.Request)
	if err != nil {
		if writeErr := writeNew(output, "native-error.json", map[string]string{"error": err.Error()}); writeErr != nil {
			return errors.Join(err, writeErr)
		}
	}
	if gate != "" || err != nil {
		return finish("rejected", gate)
	}
	return finish("native_candidate_requires_replay_and_audit", "")
}
