package practicalboardeval

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"time"

	"kicadai/internal/aiprovider"
	"kicadai/internal/architecturesearch"
	"kicadai/internal/behavioralintent"
	"kicadai/internal/circuitgraph"
	"kicadai/internal/closedloopsynthesis"
	"kicadai/internal/components"
	"kicadai/internal/compositionlowering"
	"kicadai/internal/designworkflow"
	"kicadai/internal/libraryresolver"
	"kicadai/internal/modelprovenance"
	"kicadai/internal/reports"
	"kicadai/internal/writercorrectness"
)

type engine struct {
	registry     *architecturesearch.Registry
	resolver     *circuitgraph.Resolver
	models       modelprovenance.Registry
	modelHash    string
	capabilities json.RawMessage
}

func loadEngine(ctx context.Context) (*engine, error) {
	catalog, err := components.LoadCatalog(ctx, components.LoadOptions{})
	if err != nil {
		return nil, err
	}
	registry, issues := architecturesearch.NewCatalogRegistry(catalog)
	if reports.HasBlockingIssue(issues) {
		return nil, fmt.Errorf("catalog registry: %v", issues)
	}
	models, diagnostics := modelprovenance.LoadDefault()
	if len(diagnostics) != 0 {
		return nil, fmt.Errorf("model registry: %v", diagnostics)
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
	analysisSet := map[string]bool{}
	for _, record := range models.Records {
		for _, analysis := range record.Provenance.AllowedAnalyses {
			analysisSet[analysis] = true
		}
	}
	var analyses []string
	for analysis := range analysisSet {
		analyses = append(analyses, analysis)
	}
	slices.Sort(analyses)
	capabilities, err := behavioralintent.BuildInstalledCapabilities(semantic, resolver.CatalogHash(), modelHash, analyses)
	if err != nil {
		return nil, err
	}
	return &engine{registry, resolver, models, modelHash, capabilities}, nil
}

// Snapshot does not execute a corpus case or contact a provider.
func Snapshot(ctx context.Context, output string) error {
	if err := os.Mkdir(output, 0o700); err != nil {
		return err
	}
	e, err := loadEngine(ctx)
	if err != nil {
		return err
	}
	if err := WriteNew(filepath.Join(output, "installed-capabilities.json"), e.capabilities); err != nil {
		return err
	}
	profile := aiprovider.BehavioralIntentProfile("")
	if err := WriteJSON(filepath.Join(output, "provider-schema.json"), profile.IntentEnvelopeSchema()); err != nil {
		return err
	}
	if err := WriteJSON(filepath.Join(output, "closed-loop-policy.json"), closedloopsynthesis.DefaultPolicy()); err != nil {
		return err
	}
	roots, rootIssues := libraryresolver.ResolveRoots()
	index, loadIssues := libraryresolver.Load(ctx, roots, libraryresolver.LoadOptions{})
	if err := WriteGzipJSON(filepath.Join(output, "library-index.json.gz"), index); err != nil {
		return err
	}
	if err := WriteJSON(filepath.Join(output, "library-load-issues.json"), append(rootIssues, loadIssues...)); err != nil {
		return err
	}
	indexJSON, err := json.Marshal(index)
	if err != nil {
		return err
	}
	return WriteJSON(filepath.Join(output, "snapshot.json"), map[string]any{
		"catalog_sha256": e.resolver.CatalogHash(), "models_sha256": e.modelHash,
		"capabilities_sha256": SHA(e.capabilities), "capabilities_bytes": len(e.capabilities),
		"model": Model, "max_output_tokens": MaxOutputTokens, "live_requests": 0,
		"library_index_sha256": SHA(indexJSON), "library_symbols": len(index.Symbols), "library_footprints": len(index.Footprints),
	})
}

type RunOptions struct {
	Output      string
	Journal     string
	Campaign    string
	KiCadCLI    string
	PairedInput string
}

type Summary struct {
	CaseID                      string                  `json:"case_id"`
	Kind                        string                  `json:"kind"`
	Campaign                    string                  `json:"campaign"`
	Status                      string                  `json:"status"`
	FirstFailedGate             string                  `json:"first_failed_gate"`
	CompilationStatus           behavioralintent.Status `json:"compilation_status,omitempty"`
	ProviderAttempts            int                     `json:"provider_attempts"`
	FollowUpAttempts            int                     `json:"follow_up_attempts"`
	FollowUpStatus              behavioralintent.Status `json:"follow_up_status,omitempty"`
	ReplaysCompleted            int                     `json:"replays_completed"`
	ReplayIdentity              string                  `json:"replay_identity,omitempty"`
	Error                       string                  `json:"error,omitempty"`
	WallSeconds                 float64                 `json:"wall_seconds"`
	ManualImplementationRepairs int                     `json:"manual_implementation_repairs"`
	IndependentAuditRequired    bool                    `json:"independent_acceptance_audit_required"`
}

func RunCase(ctx context.Context, item Case, opts RunOptions) (returnedErr error) {
	if !SafeName(item.ID) {
		return fmt.Errorf("invalid case ID")
	}
	if opts.Campaign != "baseline" && opts.Campaign != "final" && opts.Campaign != "paired" {
		return fmt.Errorf("invalid campaign")
	}
	if (opts.Campaign == "paired") != (opts.PairedInput != "") {
		return fmt.Errorf("paired campaign needs retained input only")
	}
	if err := os.Mkdir(opts.Output, 0o700); err != nil {
		return err
	}
	started := time.Now()
	summary := Summary{CaseID: item.ID, Kind: item.Kind, Campaign: opts.Campaign, Status: "failed", FirstFailedGate: "environment", IndependentAuditRequired: true}
	defer func() {
		summary.WallSeconds = time.Since(started).Seconds()
		if returnedErr != nil {
			summary.Error = string(Redact([]byte(returnedErr.Error()), os.Getenv("OPENAI_API_KEY")))
		}
		if err := WriteJSON(filepath.Join(opts.Output, "result.json"), summary); err != nil {
			returnedErr = errors.Join(returnedErr, err)
		}
		files, err := Inventory(opts.Output)
		if err == nil {
			err = WriteJSON(filepath.Join(opts.Output, "inventory.json"), files)
		}
		returnedErr = errors.Join(returnedErr, err)
	}()
	if err := WriteJSON(filepath.Join(opts.Output, "case.json"), item); err != nil {
		return err
	}
	if err := WriteNew(filepath.Join(opts.Output, "prompt.txt"), []byte(item.Prompt)); err != nil {
		return err
	}
	e, err := loadEngine(ctx)
	if err != nil {
		return err
	}
	if err := WriteNew(filepath.Join(opts.Output, "installed-capabilities.json"), e.capabilities); err != nil {
		return err
	}
	var proposal behavioralintent.Proposal
	var compilation behavioralintent.Result
	summary.FirstFailedGate = "provider"
	if opts.PairedInput == "" {
		proposal, compilation, summary.ProviderAttempts, err = e.generate(ctx, item.Prompt, opts.Output, opts, item.ID, nil)
	} else {
		originalPrompt, readErr := os.ReadFile(filepath.Join(opts.PairedInput, "prompt.txt"))
		if readErr != nil {
			return readErr
		}
		if !bytes.Equal(originalPrompt, []byte(item.Prompt)) {
			return fmt.Errorf("paired source prompt mismatch")
		}
		intent, readErr := os.ReadFile(filepath.Join(opts.PairedInput, "selected-intent.json"))
		if readErr != nil {
			return readErr
		}
		if err := WriteNew(filepath.Join(opts.Output, "selected-intent.json"), intent); err != nil {
			return err
		}
		var issues []reports.Issue
		proposal, issues = behavioralintent.DecodeProposalStrict(bytes.NewReader(intent))
		compilation = behavioralintent.Compile(item.Prompt, proposal, SHA(e.capabilities))
		compilation.Issues = append(issues, compilation.Issues...)
		if reports.HasBlockingIssue(compilation.Issues) {
			compilation.Status = behavioralintent.StatusInvalid
		}
		// Preserve the exact previously accepted contract when one exists. This
		// control does not allow a final model response to change its requirements.
		retained, readErr := os.ReadFile(filepath.Join(opts.PairedInput, "compiled-requirement.json"))
		if readErr == nil {
			requirement, issues := architecturesearch.DecodeStrict(bytes.NewReader(retained))
			if reports.HasBlockingIssue(issues) {
				return fmt.Errorf("retained requirement invalid: %v", issues)
			}
			if compilation.Status != behavioralintent.StatusReady {
				return fmt.Errorf("retained proposal no longer compiles ready")
			}
			compilation.Requirement = &requirement
		} else if !os.IsNotExist(readErr) {
			return readErr
		}
	}
	if err != nil {
		return err
	}
	summary.CompilationStatus = compilation.Status
	summary.FirstFailedGate = "requirement_compilation"
	if err := WriteJSON(filepath.Join(opts.Output, "compilation.json"), compilation); err != nil {
		return err
	}
	if item.Kind == "clarification" {
		if compilation.Status != behavioralintent.StatusNeedsClarification {
			return nil
		}
		summary.Status, summary.FirstFailedGate = "clarification_candidate", "independent_acceptance_audit"
		if opts.Campaign == "paired" {
			return nil
		}
		var answers []behavioralintent.ClarificationAnswer
		for _, question := range compilation.Clarifications {
			answers = append(answers, behavioralintent.ClarificationAnswer{ClarificationID: question.ID, UncertaintyIDs: question.UncertaintyIDs, Answer: item.ClarificationAnswer})
		}
		followUp, err := behavioralintent.BindFollowUp(proposal, compilation, answers)
		if err != nil {
			return err
		}
		followOutput := filepath.Join(opts.Output, "follow-up")
		if err := os.Mkdir(followOutput, 0o700); err != nil {
			return err
		}
		if err := WriteJSON(filepath.Join(followOutput, "bound-answer.json"), followUp); err != nil {
			return err
		}
		follow := &followState{proposal, compilation, followUp}
		_, result, attempts, err := e.generate(ctx, item.Prompt, followOutput, opts, item.ID+"-answer", follow)
		summary.FollowUpAttempts, summary.FollowUpStatus = attempts, result.Status
		if err != nil {
			return err
		}
		return WriteJSON(filepath.Join(followOutput, "compilation.json"), result)
	}
	if compilation.Status != behavioralintent.StatusReady || compilation.Requirement == nil || reports.HasBlockingIssue(compilation.Issues) {
		if item.Kind == "refusal" && compilation.Status == behavioralintent.StatusUnsupported && !reports.HasBlockingIssue(compilation.Issues) {
			summary.Status, summary.FirstFailedGate = "refusal_candidate", "independent_acceptance_audit"
		}
		return nil
	}
	if item.Kind == "refusal" {
		// Do not physically implement an unsafe input. A ready interpretation is
		// retained as a failed refusal, not converted into a successful refusal.
		summary.FirstFailedGate = "refusal_behavior"
		return nil
	}
	if err := WriteJSON(filepath.Join(opts.Output, "compiled-requirement.json"), compilation.Requirement); err != nil {
		return err
	}
	if missing := CommonRequirementGate(*compilation.Requirement); missing != "" {
		summary.FirstFailedGate = "requirement_faithfulness"
		return WriteJSON(filepath.Join(opts.Output, "common-acceptance-failure.json"), map[string]string{"missing_or_weakened": missing})
	}
	var identities []string
	inputBytes, err := json.Marshal(compilation.Requirement)
	if err != nil {
		return err
	}
	for replay := 1; replay <= 2; replay++ {
		root := filepath.Join(opts.Output, fmt.Sprintf("replay-%d", replay))
		if err := os.Mkdir(root, 0o700); err != nil {
			return err
		}
		// Re-decode the immutable accepted input and recreate the catalog,
		// resolver and simulation state for each clean downstream run.
		input, decodeIssues := architecturesearch.DecodeStrict(bytes.NewReader(inputBytes))
		if reports.HasBlockingIssue(decodeIssues) {
			return fmt.Errorf("replay input decode: %v", decodeIssues)
		}
		replayEngine, err := loadEngine(ctx)
		if err != nil {
			return err
		}
		identity, failed, err := replayEngine.downstream(ctx, input, root, opts.KiCadCLI)
		if failed != "" {
			summary.FirstFailedGate = failed
		}
		if err != nil {
			return err
		}
		if failed != "" {
			return nil
		}
		summary.ReplaysCompleted++
		identities = append(identities, identity)
	}
	summary.FirstFailedGate = "deterministic_replay"
	if identities[0] != identities[1] {
		return nil
	}
	summary.ReplayIdentity = identities[0]
	summary.Status, summary.FirstFailedGate = "complete_candidate", "independent_acceptance_audit"
	return nil
}

type followState struct {
	proposal    behavioralintent.Proposal
	compilation behavioralintent.Result
	input       behavioralintent.FollowUp
}

func (e *engine) generate(ctx context.Context, prompt, output string, opts RunOptions, caseID string, follow *followState) (behavioralintent.Proposal, behavioralintent.Result, int, error) {
	contextJSON, err := behavioralintent.BuildProviderContext(prompt, e.capabilities)
	if err != nil {
		return behavioralintent.Proposal{}, behavioralintent.Result{}, 0, err
	}
	if follow != nil {
		var issues []reports.Issue
		contextJSON, issues = behavioralintent.BuildFollowUpProviderContext(prompt, e.capabilities, follow.proposal, follow.compilation, follow.input)
		if reports.HasBlockingIssue(issues) {
			return behavioralintent.Proposal{}, behavioralintent.Result{}, 0, fmt.Errorf("bound follow-up: %v", issues)
		}
	}
	profile := aiprovider.BehavioralIntentProfile(contextJSON)
	if err := WriteNew(filepath.Join(output, "provider-context.json"), []byte(contextJSON)); err != nil {
		return behavioralintent.Proposal{}, behavioralintent.Result{}, 0, err
	}
	if err := WriteJSON(filepath.Join(output, "provider-schema.json"), profile.IntentEnvelopeSchema()); err != nil {
		return behavioralintent.Proposal{}, behavioralintent.Result{}, 0, err
	}
	recorder := &RecordingTransport{Journal: opts.Journal, Output: output, Campaign: opts.Campaign, CaseID: caseID, Secret: os.Getenv("OPENAI_API_KEY")}
	provider, err := aiprovider.NewOpenAIProvider(aiprovider.OpenAIOptions{APIKey: recorder.Secret, Model: Model, MaxOutputTokens: MaxOutputTokens, HTTPClient: &http.Client{Transport: recorder, Timeout: 2 * time.Minute, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}})
	if err != nil {
		return behavioralintent.Proposal{}, behavioralintent.Result{}, 0, err
	}
	var diagnostics []aiprovider.Diagnostic
	for attempt := 1; attempt <= 2; attempt++ {
		recorder.Last = nil // A client-side validation error must not reuse an earlier receipt.
		result, providerErr := provider.GenerateIntent(ctx, aiprovider.GenerateRequest{Prompt: prompt, CapabilityContext: contextJSON, OutputSchemaName: profile.SchemaName, OutputSchema: profile.IntentEnvelopeSchema(), SchemaVersion: aiprovider.EnvelopeSchemaV1, Attempt: attempt, Diagnostics: diagnostics, MaxOutputTokens: MaxOutputTokens})
		prefix := filepath.Join(output, fmt.Sprintf("attempt-%d", attempt))
		usage := result.Usage
		var typedErr *aiprovider.ProviderError
		if errors.As(providerErr, &typedErr) {
			usage = typedErr.Usage
		}
		if err := recorder.Reconcile(usage); err != nil {
			return behavioralintent.Proposal{}, behavioralintent.Result{}, attempt, err
		}
		metadata := result
		metadata.IntentJSON = nil
		if err := WriteJSON(prefix+".metadata.json", metadata); err != nil {
			return behavioralintent.Proposal{}, behavioralintent.Result{}, attempt, err
		}
		if err := WriteJSON(prefix+".diagnostics.json", diagnostics); err != nil {
			return behavioralintent.Proposal{}, behavioralintent.Result{}, attempt, err
		}
		if providerErr != nil {
			if err := WriteJSON(prefix+".error.json", map[string]any{"code": aiprovider.ErrorCodeOf(providerErr), "error": string(Redact([]byte(providerErr.Error()), recorder.Secret)), "usage": usage}); err != nil {
				return behavioralintent.Proposal{}, behavioralintent.Result{}, attempt, err
			}
			return behavioralintent.Proposal{}, behavioralintent.Result{}, attempt, providerErr
		}
		if err := WriteNew(prefix+".intent.txt", Redact(result.IntentJSON, recorder.Secret)); err != nil {
			return behavioralintent.Proposal{}, behavioralintent.Result{}, attempt, err
		}
		proposal, issues := behavioralintent.DecodeProposalStrict(bytes.NewReader(result.IntentJSON))
		compiled := behavioralintent.Compile(prompt, proposal, SHA(e.capabilities))
		if follow != nil {
			compiled = behavioralintent.CompileFollowUp(prompt, follow.proposal, follow.compilation, follow.input, proposal, SHA(e.capabilities))
		}
		compiled.Issues = append(issues, compiled.Issues...)
		if reports.HasBlockingIssue(compiled.Issues) {
			compiled.Status = behavioralintent.StatusInvalid
		}
		if err := WriteJSON(prefix+".compilation.json", compiled); err != nil {
			return proposal, compiled, attempt, err
		}
		if compiled.Status != behavioralintent.StatusInvalid || attempt == 2 {
			if err := WriteNew(filepath.Join(output, "selected-intent.json"), Redact(result.IntentJSON, recorder.Secret)); err != nil {
				return proposal, compiled, attempt, err
			}
			return proposal, compiled, attempt, nil
		}
		for _, issue := range compiled.Issues {
			if !issue.Blocking() {
				continue
			}
			message := issue.Message
			if len(message) > aiprovider.MaxDiagnosticLen {
				message = message[:aiprovider.MaxDiagnosticLen]
			}
			diagnostics = append(diagnostics, aiprovider.Diagnostic{Code: string(issue.Code), Path: issue.Path, Message: message})
			if len(diagnostics) >= aiprovider.MaxDiagnostics {
				break
			}
		}
	}
	return behavioralintent.Proposal{}, behavioralintent.Result{}, 2, fmt.Errorf("unreachable attempt loop")
}

func (e *engine) downstream(ctx context.Context, requirement architecturesearch.Requirement, root, cli string) (string, string, error) {
	search := architecturesearch.Search(ctx, requirement, e.registry, architecturesearch.SearchOptions{CatalogHash: e.resolver.CatalogHash()})
	if err := WriteJSON(filepath.Join(root, "architecture-search.json"), search); err != nil {
		return "", "architecture_search", err
	}
	if search.Status != architecturesearch.SearchSelected {
		return "", "architecture_search", nil
	}
	promotion, issues := compositionlowering.SynthesizeClosedLoop(ctx, requirement, search, compositionlowering.ArchitectureSimulationPlanResolver{GraphResolver: e.resolver, ProvenanceRegistry: e.models}, e.modelHash, nil, closedloopsynthesis.DefaultPolicy())
	if err := WriteJSON(filepath.Join(root, "electrical-promotion.json"), promotion); err != nil {
		return "", "electrical_synthesis", err
	}
	if err := WriteJSON(filepath.Join(root, "electrical-issues.json"), issues); err != nil {
		return "", "electrical_synthesis", err
	}
	if reports.HasBlockingIssue(issues) || promotion.Report.Status != "pass" {
		return "", "electrical_synthesis", nil
	}
	if promotion.Request.ExplicitCircuit == nil || promotion.Request.ExplicitCircuit.ClosedLoop == nil || promotion.Request.ExplicitCircuit.ClosedLoop.SelectedCircuitHash != promotion.Request.ExplicitCircuit.ResolutionHash {
		return "", "electrical_binding", nil
	}
	if cli == "" {
		return "", "native_environment", fmt.Errorf("installed KiCad is mandatory")
	}
	roots, rootIssues := libraryresolver.ResolveRoots()
	if roots.SymbolsRoot == "" || roots.FootprintsRoot == "" || reports.HasBlockingIssue(rootIssues) {
		return "", "native_environment", fmt.Errorf("installed library roots: %v", rootIssues)
	}
	index, loadIssues := libraryresolver.Load(ctx, roots, libraryresolver.LoadOptions{})
	if err := WriteJSON(filepath.Join(root, "library-load-issues.json"), loadIssues); err != nil {
		return "", "native_environment", err
	}
	// The stock library contains unrelated symbols outside this design's
	// selected dependency closure. Preserve their diagnostics, as the existing
	// promotion harness does, but validate the selected references through the
	// resolver/writer/KiCad gates rather than rejecting every design globally.
	if len(index.Symbols) == 0 || len(index.Footprints) == 0 {
		return "", "native_environment", fmt.Errorf("installed library index is empty; see library-load-issues.json")
	}
	if err := WriteGzipJSON(filepath.Join(root, "library-index.json.gz"), index); err != nil {
		return "", "native_environment", err
	}
	projectDir := filepath.Join(root, "project")
	result := designworkflow.Create(ctx, promotion.Request, designworkflow.CreateOptions{
		OutputDir: projectDir, LibraryIndex: &index,
		Validation:  designworkflow.ValidationOptions{StrictUnrouted: true, RequireDRC: true, KiCadCLI: cli, KeepArtifacts: true, ArtifactDir: filepath.Join(projectDir, ".kicadai", "validation")},
		KiCadChecks: designworkflow.KiCadCheckOptions{KiCadCLI: cli, RequireERC: true, RequireDRC: true, EnforceRequirements: true, KeepArtifacts: true, ArtifactDir: filepath.Join(projectDir, ".kicadai", "checks")},
		Writer:      writercorrectness.Options{RequireKiCadRoundTrip: true, StrictDiffs: true, KiCadCLI: cli, KeepArtifacts: true, ArtifactDir: filepath.Join(projectDir, ".kicadai", "roundtrip"), LibraryIndex: index, HasLibraryIndex: true, LibraryResolutionUsed: true},
	})
	if err := WriteJSON(filepath.Join(root, "workflow.json"), result); err != nil {
		return "", "physical_workflow", err
	}
	if failed := FirstFailedStage(result); failed != "" {
		return "", failed, nil
	}
	preview := filepath.Join(root, "previews")
	if err := os.Mkdir(preview, 0o700); err != nil {
		return "", "preview_generation", err
	}
	previewCommands := [][]string{
		{"sch", "export", "svg", "--output", filepath.Join(preview, "schematic"), filepath.Join(projectDir, promotion.Request.Name+".kicad_sch")},
		{"pcb", "export", "svg", "--mode-single", "--layers", "F.Cu,B.Cu,F.SilkS,Edge.Cuts", "--page-size-mode", "2", "--output", filepath.Join(preview, "board.svg"), filepath.Join(projectDir, promotion.Request.Name+".kicad_pcb")},
	}
	for i, args := range previewCommands {
		log, runErr := exec.CommandContext(ctx, cli, args...).CombinedOutput()
		if err := WriteNew(filepath.Join(preview, fmt.Sprintf("export-%d.log", i+1)), log); err != nil {
			return "", "preview_generation", err
		}
		if runErr != nil {
			return "", "preview_generation", runErr
		}
	}
	for _, suffix := range []string{".kicad_sch", ".kicad_pcb", ".kicad_pro"} {
		_, err := os.Stat(filepath.Join(projectDir, promotion.Request.Name+suffix))
		if err != nil {
			return "", "project_identity", err
		}
	}
	files, err := ProjectIdentity(projectDir)
	if err != nil {
		return "", "project_identity", err
	}
	normalized, err := NormalizeEvidence(map[string]any{"search": search, "electrical": promotion, "workflow": result, "files": files}, root)
	if err != nil {
		return "", "replay_identity", err
	}
	if err := WriteNew(filepath.Join(root, "normalized-evidence.json"), normalized); err != nil {
		return "", "replay_identity", err
	}
	return SHA(normalized), "", nil
}
