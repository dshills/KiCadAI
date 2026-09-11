package airequirementeval

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"kicadai/internal/aiprovider"
	"kicadai/internal/behavioralintent"
	"kicadai/internal/practicalboardeval"
	"kicadai/internal/reports"
)

type legResult struct {
	Proposal    behavioralintent.Proposal `json:"proposal"`
	Compilation behavioralintent.Result   `json:"compilation"`
	Attempts    int                       `json:"attempts"`
	FirstStatus behavioralintent.Status   `json:"first_status"`
}
type caseResult struct {
	CaseID                string                  `json:"case_id"`
	Kind                  string                  `json:"kind"`
	Status                string                  `json:"status"`
	InitialStatus         behavioralintent.Status `json:"initial_status"`
	FollowUpStatus        behavioralintent.Status `json:"follow_up_status"`
	InitialAttempts       int                     `json:"initial_attempts"`
	FollowUpAttempts      int                     `json:"follow_up_attempts"`
	FirstAttemptStatus    behavioralintent.Status `json:"first_attempt_status"`
	Error                 string                  `json:"error,omitempty"`
	WallSeconds           float64                 `json:"wall_seconds"`
	SemanticAuditRequired bool                    `json:"semantic_audit_required"`
	BoardPass             bool                    `json:"board_pass"`
}
type followState struct {
	Proposal    behavioralintent.Proposal
	Compilation behavioralintent.Result
	Input       behavioralintent.FollowUp
}

func emit(value any) {
	b, err := json.Marshal(value)
	if err == nil {
		fmt.Println(string(b))
	}
}

func generate(ctx context.Context, item Case, capabilities json.RawMessage, output, leg string, follow *followState, recorder *recordingTransport) (legResult, error) {
	var result legResult
	providerContext, err := behavioralintent.BuildProviderContext(item.Prompt, capabilities)
	if err != nil {
		return result, err
	}
	if follow != nil {
		var issues []reports.Issue
		providerContext, issues = behavioralintent.BuildFollowUpProviderContext(item.Prompt, capabilities, follow.Proposal, follow.Compilation, follow.Input)
		if reports.HasBlockingIssue(issues) {
			return result, fmt.Errorf("follow-up context rejected: %v", issues)
		}
	}
	if err := os.Mkdir(output, 0o700); err != nil {
		return result, err
	}
	if err := writeNew(filepath.Join(output, "provider-context.json"), []byte(providerContext)); err != nil {
		return result, err
	}
	recorder.Output, recorder.CaseID, recorder.Leg = output, item.ID, leg
	provider, err := aiprovider.NewOpenAIProvider(aiprovider.OpenAIOptions{APIKey: recorder.Secret, Model: Model, MaxOutputTokens: MaxOutputTokens, HTTPClient: &http.Client{Transport: recorder, Timeout: 5 * time.Minute, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}})
	if err != nil {
		return result, err
	}
	var diagnostics []aiprovider.Diagnostic
	for attempt := 1; attempt <= 2; attempt++ {
		started := time.Now()
		recorder.Last = nil
		recorder.Expected = requestFor(item.Prompt, providerContext, attempt, diagnostics)
		outputResult, providerErr := provider.GenerateIntent(ctx, recorder.Expected)
		if providerErr == nil && (outputResult.Model != Model || outputResult.Background) {
			providerErr = fmt.Errorf("returned model or background mode differs from the frozen provider identity")
		}
		result.Attempts = attempt
		prefix := filepath.Join(output, fmt.Sprintf("attempt-%d", attempt))
		accepted := providerErr == nil
		if err := recorder.reconcile(outputResult.Usage, accepted); err != nil {
			return result, err
		}
		metadata := outputResult
		metadata.IntentJSON = nil
		if err := writeJSON(prefix+".metadata.json", metadata); err != nil {
			return result, err
		}
		if err := writeJSON(prefix+".diagnostics.json", diagnostics); err != nil {
			return result, err
		}
		if err := writeJSON(prefix+".timing.json", map[string]any{"started_utc": started.UTC().Format(time.RFC3339Nano), "wall_seconds": time.Since(started).Seconds(), "reservation_created": recorder.Last != nil}); err != nil {
			return result, err
		}
		if providerErr != nil {
			message := string(redact([]byte(providerErr.Error()), recorder.Secret))
			if err := writeJSON(prefix+".error.json", map[string]any{"code": aiprovider.ErrorCodeOf(providerErr), "message": message, "accepted_usage": false, "retry_permitted": false}); err != nil {
				return result, err
			}
			return result, providerErr
		}
		if err := writeNew(prefix+".intent.json", redact(outputResult.IntentJSON, recorder.Secret)); err != nil {
			return result, err
		}
		proposal, issues := behavioralintent.DecodeProposalStrict(bytes.NewReader(outputResult.IntentJSON))
		compiled := behavioralintent.Compile(item.Prompt, proposal, sha(capabilities))
		if follow != nil {
			compiled = behavioralintent.CompileFollowUp(item.Prompt, follow.Proposal, follow.Compilation, follow.Input, proposal, sha(capabilities))
		}
		compiled.Issues = append(issues, compiled.Issues...)
		if reports.HasBlockingIssue(compiled.Issues) {
			compiled.Status = behavioralintent.StatusInvalid
			compiled.Requirement = nil
		}
		if err := writeJSON(prefix+".compilation.json", compiled); err != nil {
			return result, err
		}
		result.Proposal, result.Compilation = proposal, compiled
		if attempt == 1 {
			result.FirstStatus = compiled.Status
		}
		emit(map[string]any{"event": "attempt_complete", "case_id": item.ID, "leg": leg, "attempt": attempt, "compilation_status": compiled.Status, "issues": len(compiled.Issues)})
		if compiled.Status != behavioralintent.StatusInvalid || attempt == 2 {
			if err := writeJSON(filepath.Join(output, "selected.json"), result); err != nil {
				return result, err
			}
			return result, nil
		}
		diagnostics = nil
		for _, issue := range compiled.Issues {
			if !issue.Blocking() {
				continue
			}
			message := issue.Message
			if len(message) > aiprovider.MaxDiagnosticLen {
				message = message[:aiprovider.MaxDiagnosticLen]
			}
			diagnostics = append(diagnostics, aiprovider.Diagnostic{Code: string(issue.Code), Path: issue.Path, Message: message})
			if len(diagnostics) == aiprovider.MaxDiagnostics {
				break
			}
		}
	}
	return result, fmt.Errorf("unreachable correction state")
}

type ReviewDecision struct {
	CaseID             string `json:"case_id"`
	SelectedSHA256     string `json:"selected_sha256"`
	ApproveFixedAnswer bool   `json:"approve_fixed_answer"`
	Reviewer           string `json:"reviewer"`
	Reason             string `json:"reason"`
}

func awaitAnswerReview(ctx context.Context, item Case, root string) (bool, error) {
	selected, err := os.ReadFile(filepath.Join(root, "initial", "selected.json"))
	if err != nil {
		return false, err
	}
	decisionPath := filepath.Join(root, "answer-review.json")
	emit(map[string]any{"event": "awaiting_answer_review", "case_id": item.ID, "decision_path": decisionPath, "selected_sha256": sha(selected), "fixed_answer": item.Answer})
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		var decision ReviewDecision
		err := readJSON(decisionPath, &decision)
		if err == nil {
			if decision.CaseID != item.ID || decision.SelectedSHA256 != sha(selected) || decision.Reviewer == "" || strings.TrimSpace(decision.Reason) == "" {
				return false, fmt.Errorf("unbound or incomplete semantic answer review")
			}
			return decision.ApproveFixedAnswer, nil
		}
		if !os.IsNotExist(err) {
			return false, err
		}
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-ticker.C:
		}
	}
}

func runCase(ctx context.Context, item Case, capabilities json.RawMessage, root string, recorder *recordingTransport) (summary caseResult, returnedErr error) {
	started := time.Now()
	summary = caseResult{CaseID: item.ID, Kind: item.Kind, Status: "failed", SemanticAuditRequired: true}
	defer func() {
		summary.WallSeconds = time.Since(started).Seconds()
		if returnedErr != nil {
			summary.Error = string(redact([]byte(returnedErr.Error()), recorder.Secret))
		}
		returnedErr = errors.Join(returnedErr, writeJSON(filepath.Join(root, "result.json"), summary))
	}()
	if err := os.Mkdir(root, 0o700); err != nil {
		return summary, err
	}
	if err := writeJSON(filepath.Join(root, "case.json"), item); err != nil {
		return summary, err
	}
	initial, err := generate(ctx, item, capabilities, filepath.Join(root, "initial"), "initial", nil, recorder)
	summary.InitialAttempts, summary.InitialStatus, summary.FirstAttemptStatus = initial.Attempts, initial.Compilation.Status, initial.FirstStatus
	if err != nil {
		return summary, err
	}
	switch item.Kind {
	case "ready":
		if initial.Compilation.Status == behavioralintent.StatusReady && initial.Compilation.Requirement != nil {
			summary.Status = "ready_candidate"
		}
	case "refusal":
		if initial.Compilation.Status == behavioralintent.StatusUnsupported && initial.Compilation.Requirement == nil {
			summary.Status = "refusal_candidate"
		}
	case "clarification":
		if initial.Compilation.Status != behavioralintent.StatusNeedsClarification || initial.Compilation.Requirement != nil {
			return summary, nil
		}
		approved, err := awaitAnswerReview(ctx, item, root)
		if err != nil {
			return summary, err
		}
		if !approved {
			summary.Status = "clarification_review_rejected"
			return summary, nil
		}
		answers := []behavioralintent.ClarificationAnswer{}
		for _, question := range initial.Compilation.Clarifications {
			answers = append(answers, behavioralintent.ClarificationAnswer{ClarificationID: question.ID, UncertaintyIDs: question.UncertaintyIDs, Answer: item.Answer})
		}
		input, err := behavioralintent.BindFollowUp(initial.Proposal, initial.Compilation, answers)
		if err != nil {
			return summary, err
		}
		if _, issues := behavioralintent.ValidateFollowUp(item.Prompt, initial.Proposal, initial.Compilation, input, sha(capabilities)); reports.HasBlockingIssue(issues) {
			return summary, fmt.Errorf("bound answer invalid")
		}
		if err := writeJSON(filepath.Join(root, "bound-answer.json"), input); err != nil {
			return summary, err
		}
		follow, err := generate(ctx, item, capabilities, filepath.Join(root, "follow-up"), "follow-up", &followState{initial.Proposal, initial.Compilation, input}, recorder)
		summary.FollowUpAttempts, summary.FollowUpStatus = follow.Attempts, follow.Compilation.Status
		if err != nil {
			return summary, err
		}
		if follow.Compilation.Status == behavioralintent.StatusReady && follow.Compilation.Requirement != nil {
			summary.Status = "clarification_candidate"
		}
	}
	return summary, nil
}

// Run executes the sole frozen campaign. The fixed root is exclusively created,
// so neither a completed run nor an interrupted one can be restarted.
func Run(parent context.Context, repo string) (returnedErr error) {
	freeze, corpus, capabilities, err := VerifyFreeze(repo)
	if err != nil {
		return err
	}
	secret := strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	if secret == "" {
		return fmt.Errorf("approved existing OPENAI_API_KEY is unavailable")
	}
	current, err := loadCapabilities(parent)
	if err != nil || !bytes.Equal(current, capabilities) {
		return fmt.Errorf("installed capabilities changed since freeze")
	}
	var preflight Preflight
	if err := readJSON(filepath.Join(repo, SpecPath, "snapshot/preflight.json"), &preflight); err != nil {
		return err
	}
	schema, err := json.Marshal(aiprovider.BehavioralIntentProfile("").IntentEnvelopeSchema())
	if err != nil || sha(schema) != preflight.ProviderSchemaSHA256 || sha(capabilities) != preflight.CapabilitySHA256 || preflight.LiveRequests != 0 {
		return fmt.Errorf("preflight schema/capability identity changed")
	}
	if err := os.Mkdir(EvidenceRoot, 0o700); err != nil {
		return fmt.Errorf("campaign root already exists or cannot be exclusively created: %w", err)
	}
	started := time.Now()
	ctx, cancel := context.WithTimeout(parent, 90*time.Minute)
	defer cancel()
	monitor := &resourceMonitor{}
	done := make(chan struct{})
	outcomes := []caseResult{}
	stopReason := ""
	defer func() {
		cancel()
		<-done
		if returnedErr != nil {
			stopReason = string(redact([]byte(returnedErr.Error()), secret))
		}
		for len(outcomes) < len(corpus.Cases) {
			item := corpus.Cases[len(outcomes)]
			outcomes = append(outcomes, caseResult{CaseID: item.ID, Kind: item.Kind, Status: "not_run", SemanticAuditRequired: true})
		}
		count, cost, totalErr := journalTotals(filepath.Join(EvidenceRoot, "journal"))
		returnedErr = errors.Join(returnedErr, totalErr)
		if totalErr != nil && stopReason == "" {
			stopReason = "journal integrity/accounting failure"
		}
		end := map[string]any{"schema": "kicadai.interface-campaign-end.v1", "started_utc": started.UTC().Format(time.RFC3339Nano), "ended_utc": time.Now().UTC().Format(time.RFC3339Nano), "wall_seconds": time.Since(started).Seconds(), "stop_reason": stopReason, "outcomes": outcomes, "generation_requests": count, "estimated_or_reserved_microusd": cost, "estimated_or_reserved_usd": float64(cost) / 1e6, "actual_billed_usd": nil, "resources": monitor.snapshot(), "full_board_passes": 0, "semantic_audits_complete": false, "historical_results_modified": false}
		returnedErr = errors.Join(returnedErr, writeJSON(filepath.Join(EvidenceRoot, "campaign-end.json"), end))
		files, inventoryErr := practicalboardeval.Inventory(EvidenceRoot)
		if inventoryErr == nil {
			inventoryErr = writeJSON(filepath.Join(EvidenceRoot, "inventory.json"), files)
		}
		returnedErr = errors.Join(returnedErr, inventoryErr)
		emit(map[string]any{"event": "campaign_terminal", "requests": count, "estimated_or_reserved_usd": float64(cost) / 1e6, "stop_reason": stopReason})
	}()
	go monitor.watch(ctx, EvidenceRoot, cancel, done)
	if err := os.Mkdir(filepath.Join(EvidenceRoot, "journal"), 0o700); err != nil {
		return err
	}
	if err := monitor.sample(ctx, EvidenceRoot); err != nil {
		return err
	}
	for _, file := range freeze.Files {
		b, err := os.ReadFile(filepath.Join(repo, file.Path))
		if err != nil {
			return err
		}
		path := filepath.Join(EvidenceRoot, "frozen-inputs", file.Path)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return err
		}
		if err := writeNew(path, b); err != nil {
			return err
		}
	}
	freezeBytes, err := os.ReadFile(filepath.Join(repo, SpecPath, "freeze.json"))
	if err != nil {
		return err
	}
	if err := writeNew(filepath.Join(EvidenceRoot, "freeze.json"), freezeBytes); err != nil {
		return err
	}
	binary, err := os.ReadFile(BinaryPath)
	if err != nil {
		return err
	}
	build, _ := debug.ReadBuildInfo()
	head, err := git(repo, "rev-parse", "HEAD")
	if err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(EvidenceRoot, "campaign-start.json"), map[string]any{"started_utc": started.UTC().Format(time.RFC3339Nano), "freeze_sha256": FreezeSHA256, "source_commit": freeze.SourceCommit, "binary_source_commit": head, "binary_sha256": sha(binary), "binary_build_info": build.String(), "model": Model, "max_output_tokens": MaxOutputTokens, "max_requests": MaxRequests, "max_estimated_or_reserved_usd": MaxSpendUSD, "provider_calls_before_start": 0}); err != nil {
		return err
	}
	for _, item := range corpus.Cases {
		caseCtx, caseCancel := context.WithTimeout(ctx, 20*time.Minute)
		recorder := &recordingTransport{Journal: filepath.Join(EvidenceRoot, "journal"), Secret: secret, InstructionSHA: preflight.InstructionSHA256}
		recorder.Guard = func() error {
			if err := caseCtx.Err(); err != nil {
				return err
			}
			deadline, ok := caseCtx.Deadline()
			if !ok || time.Until(deadline) < 5*time.Minute {
				return fmt.Errorf("insufficient remaining case/campaign time for another request")
			}
			if _, _, _, err := VerifyFreeze(repo); err != nil {
				return err
			}
			if err := monitor.sample(caseCtx, EvidenceRoot); err != nil {
				return err
			}
			size, err := directoryBytes(EvidenceRoot)
			if err != nil {
				return err
			}
			if size+(32<<20) > EvidenceBytes {
				return fmt.Errorf("insufficient evidence headroom before request")
			}
			return nil
		}
		emit(map[string]any{"event": "case_start", "case_id": item.ID, "kind": item.Kind})
		summary, err := runCase(caseCtx, item, capabilities, filepath.Join(EvidenceRoot, item.ID), recorder)
		caseCancel()
		outcomes = append(outcomes, summary)
		emit(map[string]any{"event": "case_complete", "case_id": item.ID, "status": summary.Status, "error": summary.Error})
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	return nil
}
