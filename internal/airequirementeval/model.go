// Package airequirementeval is the isolated, single-run interface evaluation.
// It never synthesizes a board or changes production or historical evaluation policy.
package airequirementeval

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"slices"
	"strings"

	"kicadai/internal/aiprovider"
	"kicadai/internal/architecturesearch"
	"kicadai/internal/behavioralintent"
	"kicadai/internal/circuitgraph"
	"kicadai/internal/components"
	"kicadai/internal/modelprovenance"
	"kicadai/internal/practicalboardeval"
	"kicadai/internal/reports"
)

const (
	Model           = "gpt-5.6-sol"
	MaxOutputTokens = 16384
	MaxRequestBytes = 131072
	MaxRequests     = 20
	MaxSpendUSD     = 25.0
	CaptureBytes    = 8 << 20
	EvidenceBytes   = 1 << 30
	SpecPath        = "specs/ai-requirement-contract-integration/live-v1"
	EvidenceRoot    = "/tmp/kicadai-ai-requirement-interface-v1"
	BinaryPath      = "/tmp/kicadai-ai-requirement-eval-v1"
)

// Injected only into the sealed post-freeze build. Development builds cannot run live.
var FreezeSHA256 string

type Case struct {
	ID                 string   `json:"id"`
	Kind               string   `json:"kind"`
	Title              string   `json:"title"`
	Prompt             string   `json:"prompt"`
	Expected           string   `json:"expected"`
	Answer             string   `json:"answer"`
	RequiredObjectives []string `json:"required_objectives"`
	RequiredAnalyses   []string `json:"required_analyses"`
	Clauses            []string `json:"clauses"`
}

type Corpus struct {
	Schema     string `json:"schema"`
	Authorship string `json:"authorship"`
	Cases      []Case `json:"cases"`
}

func (c Corpus) Validate() error {
	if c.Schema != "kicadai.interface-evaluation-corpus.v1" || c.Authorship == "" || len(c.Cases) != 8 {
		return fmt.Errorf("invalid interface corpus")
	}
	wantIDs := []string{"I01", "I02", "I03", "I04", "R01", "R02", "C01", "C02"}
	for i, item := range c.Cases {
		kind, expectation := "ready", "ready"
		if i >= 4 {
			kind, expectation = "refusal", "unsupported"
		}
		if i >= 6 {
			kind, expectation = "clarification", "needs_clarification_then_ready"
		}
		if item.ID != wantIDs[i] || item.Kind != kind || item.Expected != expectation || item.Prompt == "" || len(item.Prompt) > 8000 || item.Title == "" || len(item.Clauses) == 0 || (item.Answer != "") != (kind == "clarification") {
			return fmt.Errorf("invalid case contract: %s", item.ID)
		}
		if len(item.Answer) > behavioralintent.MaxAnswerBytes {
			return fmt.Errorf("answer exceeds production bound")
		}
		for _, clause := range item.Clauses {
			if strings.TrimSpace(clause) == "" {
				return fmt.Errorf("empty acceptance clause")
			}
		}
	}
	return nil
}

type Freeze struct {
	Schema       string                          `json:"schema"`
	SourceCommit string                          `json:"source_commit"`
	EvidenceRoot string                          `json:"evidence_root"`
	BinaryPath   string                          `json:"binary_path"`
	Files        []practicalboardeval.FileRecord `json:"files"`
}

func sha(b []byte) string { return practicalboardeval.SHA(b) }
func writeJSON(path string, value any) error {
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return writeNew(path, append(b, '\n'))
}
func writeNew(path string, data []byte) error {
	if strings.HasPrefix(filepath.Clean(path), EvidenceRoot+string(os.PathSeparator)) {
		size, err := directoryBytes(EvidenceRoot)
		if err != nil {
			return err
		}
		if size+int64(len(data)) > EvidenceBytes {
			return fmt.Errorf("evidence byte ceiling reached")
		}
	}
	return practicalboardeval.WriteNew(path, data)
}
func readJSON(path string, value any) error { return practicalboardeval.ReadJSON(path, value) }

var providerKeyPattern = regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{20,}`)

func redact(b []byte, secret string) []byte {
	if secret != "" {
		b = bytes.ReplaceAll(b, []byte(secret), []byte("<redacted>"))
	}
	return providerKeyPattern.ReplaceAll(b, []byte("<redacted>"))
}

func loadCapabilities(ctx context.Context) (json.RawMessage, error) {
	catalog, err := components.LoadCatalog(ctx, components.LoadOptions{})
	if err != nil {
		return nil, err
	}
	registry, issues := architecturesearch.NewCatalogRegistry(catalog)
	if reports.HasBlockingIssue(issues) {
		return nil, fmt.Errorf("catalog registry invalid")
	}
	models, diagnostics := modelprovenance.LoadDefault()
	if len(diagnostics) != 0 {
		return nil, fmt.Errorf("model registry invalid")
	}
	modelHash, err := modelprovenance.Hash(models)
	if err != nil {
		return nil, err
	}
	resolver := circuitgraph.NewResolver(circuitgraph.ResolveOptions{Catalog: catalog, CatalogID: "installed"})
	semantic, err := architecturesearch.EncodeSemanticCapabilities(registry, 0)
	if err != nil {
		return nil, err
	}
	analyses := []string{}
	for _, record := range models.Records {
		analyses = append(analyses, record.Provenance.AllowedAnalyses...)
	}
	slices.Sort(analyses)
	return behavioralintent.BuildInstalledCapabilities(semantic, resolver.CatalogHash(), modelHash, slices.Compact(analyses))
}

func validateCapabilities(c Corpus, capabilities json.RawMessage) error {
	installed, err := behavioralintent.ValidateInstalledCapabilities(capabilities)
	if err != nil {
		return err
	}
	var semantic architecturesearch.SemanticCapabilityDocument
	if err := json.Unmarshal(installed.Architecture, &semantic); err != nil {
		return err
	}
	for _, item := range c.Cases {
		for _, kind := range item.RequiredObjectives {
			if !slices.Contains(semantic.ObjectiveKinds, kind) {
				return fmt.Errorf("%s requires unregistered capability %s", item.ID, kind)
			}
		}
		for _, analysis := range item.RequiredAnalyses {
			if !slices.Contains(installed.TrustedAnalyses, analysis) {
				return fmt.Errorf("%s requires untrusted analysis %s", item.ID, analysis)
			}
		}
	}
	return nil
}

func git(repo string, args ...string) (string, error) {
	command := exec.Command("git", args...)
	command.Dir = repo
	b, err := command.Output()
	return strings.TrimSpace(string(b)), err
}

// VerifyFreeze checks exact bytes and the clean evaluated source, not merely a manifest label.
func VerifyFreeze(repo string) (Freeze, Corpus, json.RawMessage, error) {
	var freeze Freeze
	var corpus Corpus
	freezePath := filepath.Join(repo, SpecPath, "freeze.json")
	b, err := os.ReadFile(freezePath)
	if err != nil {
		return freeze, corpus, nil, err
	}
	if len(FreezeSHA256) != 64 || sha(b) != FreezeSHA256 {
		return freeze, corpus, nil, fmt.Errorf("unsealed or mismatched evaluator freeze")
	}
	if err := readJSON(freezePath, &freeze); err != nil {
		return freeze, corpus, nil, err
	}
	if freeze.Schema != "kicadai.interface-evaluation-freeze.v1" || len(freeze.SourceCommit) != 40 || freeze.EvidenceRoot != EvidenceRoot || freeze.BinaryPath != BinaryPath || len(freeze.Files) == 0 {
		return freeze, corpus, nil, fmt.Errorf("invalid freeze identity")
	}
	seen := map[string]bool{}
	for _, file := range freeze.Files {
		if filepath.IsAbs(file.Path) || filepath.Clean(file.Path) != file.Path || strings.HasPrefix(file.Path, "..") || seen[file.Path] {
			return freeze, corpus, nil, fmt.Errorf("unsafe/duplicate frozen path")
		}
		seen[file.Path] = true
		info, err := os.Lstat(filepath.Join(repo, file.Path))
		if err != nil || !info.Mode().IsRegular() {
			return freeze, corpus, nil, fmt.Errorf("frozen file is not regular: %s", file.Path)
		}
		b, err := os.ReadFile(filepath.Join(repo, file.Path))
		if err != nil || int64(len(b)) != file.Bytes || sha(b) != file.SHA256 {
			return freeze, corpus, nil, fmt.Errorf("frozen file mismatch: %s", file.Path)
		}
	}
	for _, required := range []string{"corpus.json", "SPEC.md", "AUTHORIZATION.md", "pricing.json", "FEASIBILITY.md", "snapshot/installed-capabilities.json", "snapshot/provider-schema.json", "snapshot/preflight.json"} {
		if !seen[filepath.Join(SpecPath, required)] {
			return freeze, corpus, nil, fmt.Errorf("missing required frozen file %s", required)
		}
	}
	status, err := git(repo, "status", "--porcelain", "--untracked-files=no")
	if err != nil || status != "" {
		return freeze, corpus, nil, fmt.Errorf("evaluated checkout is not clean")
	}
	diff, err := git(repo, "diff", "--name-only", freeze.SourceCommit, "HEAD", "--", ".", ":(exclude)"+SpecPath+"/freeze.json")
	if err != nil || diff != "" {
		return freeze, corpus, nil, fmt.Errorf("source differs from reviewed freeze source")
	}
	head, err := git(repo, "rev-parse", "HEAD")
	if err != nil {
		return freeze, corpus, nil, err
	}
	build, ok := debug.ReadBuildInfo()
	revision, modified := "", ""
	if ok {
		for _, item := range build.Settings {
			if item.Key == "vcs.revision" {
				revision = item.Value
			}
			if item.Key == "vcs.modified" {
				modified = item.Value
			}
		}
	}
	if !ok || revision != head || modified != "false" {
		return freeze, corpus, nil, fmt.Errorf("binary is not a clean build of the evaluated checkout")
	}
	executable, err := os.Executable()
	resolvedExecutable, resolveErr := filepath.EvalSymlinks(executable)
	resolvedExpected, expectedErr := filepath.EvalSymlinks(BinaryPath)
	if err != nil || resolveErr != nil || expectedErr != nil || resolvedExecutable != resolvedExpected {
		return freeze, corpus, nil, fmt.Errorf("unexpected sealed evaluator path")
	}
	if err := readJSON(filepath.Join(repo, SpecPath, "corpus.json"), &corpus); err != nil {
		return freeze, corpus, nil, err
	}
	if err := corpus.Validate(); err != nil {
		return freeze, corpus, nil, err
	}
	capabilities, err := os.ReadFile(filepath.Join(repo, SpecPath, "snapshot/installed-capabilities.json"))
	if err != nil {
		return freeze, corpus, nil, err
	}
	return freeze, corpus, bytes.TrimSpace(capabilities), validateCapabilities(corpus, bytes.TrimSpace(capabilities))
}

func requestFor(prompt, contextJSON string, attempt int, diagnostics []aiprovider.Diagnostic) aiprovider.GenerateRequest {
	profile := aiprovider.BehavioralIntentProfile(contextJSON)
	return aiprovider.GenerateRequest{Prompt: prompt, CapabilityContext: contextJSON, OutputSchemaName: profile.SchemaName, OutputSchema: profile.IntentEnvelopeSchema(), SchemaVersion: aiprovider.EnvelopeSchemaV1, Attempt: attempt, Diagnostics: diagnostics, MaxOutputTokens: MaxOutputTokens}
}
