package airequirementeval

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"kicadai/internal/aiprovider"
	"kicadai/internal/architecturesearch"
	"kicadai/internal/behavioralintent"
	"kicadai/internal/practicalboardeval"
	"kicadai/internal/reports"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func testRequest(t *testing.T) (aiprovider.GenerateRequest, []byte, string) {
	t.Helper()
	capabilities, err := loadCapabilities(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	c, err := behavioralintent.BuildProviderContext("A synthetic offline requirement.", capabilities)
	if err != nil {
		t.Fatal(err)
	}
	request := requestFor("A synthetic offline requirement.", c, 1, nil)
	capture := &captureTransport{}
	provider, err := aiprovider.NewOpenAIProvider(aiprovider.OpenAIOptions{APIKey: "synthetic-test-key", Model: Model, HTTPClient: &http.Client{Transport: capture}, MaxOutputTokens: MaxOutputTokens})
	if err != nil {
		t.Fatal(err)
	}
	_, err = provider.GenerateIntent(context.Background(), request)
	if err == nil || capture.calls != 1 {
		t.Fatal("offline capture did not run")
	}
	var wire wireRequest
	if err := json.Unmarshal(capture.body, &wire); err != nil {
		t.Fatal(err)
	}
	return request, capture.body, sha([]byte(wire.Instructions))
}

func syntheticResponse(t *testing.T, proposal behavioralintent.Proposal) *http.Response {
	t.Helper()
	envelope, err := json.Marshal(map[string]any{"schema": aiprovider.EnvelopeSchemaV1, "intent": proposal})
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(map[string]any{"id": "resp_offline", "model": Model, "status": "completed", "output": []any{map[string]any{"type": "message", "content": []any{map[string]any{"type": "output_text", "text": string(envelope)}}}}, "usage": map[string]int{"input_tokens": 100, "output_tokens": 50, "total_tokens": 150}})
	if err != nil {
		t.Fatal(err)
	}
	event, err := json.Marshal(map[string]any{"type": "response.completed", "sequence_number": 0, "response": json.RawMessage(payload)})
	if err != nil {
		t.Fatal(err)
	}
	return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader("event: response.completed\ndata: " + string(event) + "\n\n"))}
}

func syntheticRefusal() behavioralintent.Proposal {
	return behavioralintent.Proposal{Version: behavioralintent.ProposalVersion, Coverage: []behavioralintent.CoverageRecord{{StatementID: "statement_001", Disposition: behavioralintent.DispositionCapabilityGap, Rationale: "Missing trusted synthetic capability", References: []behavioralintent.Reference{{Kind: "capability_gap", ID: "missing_capability"}}}}, CapabilityGaps: []behavioralintent.CapabilityGap{{ID: "missing_capability", Capability: "synthetic_verification", Path: "requirements.behavioral_requirements", Reason: "No trusted verification is installed", RequiredEvidence: []string{"Reviewed verification model"}}}}
}

func TestWirePreflightRejectsChangedSettingsAndUnapprovedFields(t *testing.T) {
	request, body, instructions := testRequest(t)
	if err := validateWire(body, request, instructions); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"model", "store", "stream", "background", "max_output_tokens", "instructions", "tools", "input", "text"} {
		t.Run(name, func(t *testing.T) {
			var v map[string]any
			if err := json.Unmarshal(body, &v); err != nil {
				t.Fatal(err)
			}
			switch name {
			case "model":
				v[name] = "other-model"
			case "store", "background":
				v[name] = true
			case "stream":
				v[name] = false
			case "max_output_tokens":
				v[name] = 32768
			case "instructions":
				v[name] = "different"
			case "tools":
				v[name] = []any{}
			case "input":
				v[name] = `{"prompt":"different","capability_context":"","attempt":1,"diagnostics":null}`
			case "text":
				v[name] = map[string]any{"format": map[string]any{"type": "json_object"}}
			}
			b, err := json.Marshal(v)
			if err != nil {
				t.Fatal(err)
			}
			if validateWire(b, request, instructions) == nil {
				t.Fatal("changed request accepted")
			}
		})
	}
	if validateWire(bytes.Repeat([]byte("x"), MaxRequestBytes+1), request, instructions) == nil {
		t.Fatal("oversized request accepted")
	}
}

func TestJournalConservativeReservationsAndFailClosedReconciliation(t *testing.T) {
	root := t.TempDir()
	receipt := reservation{Number: 1, CaseID: "I01", Leg: "initial", Attempt: 1, RequestBytes: MaxRequestBytes, RequestSHA256: strings.Repeat("1", 64), ReservedMicroUSD: reserveAmount(MaxRequestBytes)}
	if receipt.ReservedMicroUSD != 1_003_520 || 20*receipt.ReservedMicroUSD > 25_000_000 {
		t.Fatal("reservation arithmetic drift")
	}
	if err := writeJSON(filepath.Join(root, "001.reservation.json"), receipt); err != nil {
		t.Fatal(err)
	}
	_, cost, err := journalTotals(root)
	if err != nil || cost != receipt.ReservedMicroUSD {
		t.Fatal("missing usage did not retain reservation")
	}
	recorder := recordingTransport{Journal: root, Last: &receipt}
	if err := recorder.reconcile(aiprovider.Usage{InputTokens: 100, OutputTokens: 50, TotalTokens: 150}, false); err != nil {
		t.Fatal(err)
	}
	_, cost, err = journalTotals(root)
	if err != nil || cost != receipt.ReservedMicroUSD {
		t.Fatal("rejected response released reservation")
	}
	if err := recorder.reconcile(aiprovider.Usage{}, false); err == nil {
		t.Fatal("journal entry overwritten")
	}
	root2 := t.TempDir()
	recorder.Journal = root2
	if err := writeJSON(filepath.Join(root2, "001.reservation.json"), receipt); err != nil {
		t.Fatal(err)
	}
	if err := recorder.reconcile(aiprovider.Usage{InputTokens: 100, OutputTokens: 50, TotalTokens: 150}, true); err != nil {
		t.Fatal(err)
	}
	_, cost, err = journalTotals(root2)
	if err != nil || cost != 1500 {
		t.Fatal("accepted conservative usage arithmetic wrong")
	}
	root3 := t.TempDir()
	recorder.Journal = root3
	if err := writeJSON(filepath.Join(root3, "001.reservation.json"), receipt); err != nil {
		t.Fatal(err)
	}
	if err := recorder.reconcile(aiprovider.Usage{InputTokens: MaxRequestBytes + 4097, OutputTokens: 1, TotalTokens: MaxRequestBytes + 4098}, true); err == nil {
		t.Fatal("out-of-reservation usage accepted")
	}
	if _, _, err := journalTotals(root3); err == nil {
		t.Fatal("accounting violation ignored")
	}
}

func TestTransportCountsNoRetryAndStopsAtBoundaries(t *testing.T) {
	expected, body, instructions := testRequest(t)
	for _, name := range []string{"valid", "endpoint", "guard", "budget", "transport", "capture", "redaction", "case_id", "repeat"} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			journal := filepath.Join(root, "journal")
			output := filepath.Join(root, "out")
			for _, dir := range []string{journal, output} {
				if err := os.Mkdir(dir, 0o700); err != nil {
					t.Fatal(err)
				}
			}
			calls := 0
			recorder := &recordingTransport{Journal: journal, Output: output, CaseID: "I01", Leg: "initial", Secret: "synthetic-test-key", Expected: expected, InstructionSHA: instructions, Guard: func() error { return nil }}
			recorder.Base = roundTripFunc(func(*http.Request) (*http.Response, error) {
				calls++
				if name == "transport" {
					return nil, errors.New("synthetic network interruption")
				}
				text := "{}"
				if name == "capture" {
					text = strings.Repeat("x", CaptureBytes+2)
				}
				if name == "redaction" {
					text = "synthetic-test-key"
				}
				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(text))}, nil
			})
			if name == "guard" {
				recorder.Guard = func() error { return errors.New("freeze mismatch") }
			}
			if name == "case_id" {
				recorder.CaseID = "not_scheduled"
			}
			if name == "budget" {
				for i := 1; i <= 20; i++ {
					r := reservation{Number: i, CaseID: "I01", Leg: "initial", Attempt: 1, RequestBytes: len(body), RequestSHA256: sha(body), ReservedMicroUSD: reserveAmount(len(body))}
					if err := writeJSON(filepath.Join(journal, formatReceipt(i)), r); err != nil {
						t.Fatal(err)
					}
				}
			}
			request, err := http.NewRequest(http.MethodPost, "https://api.openai.com/v1/responses", bytes.NewReader(body))
			if err != nil {
				t.Fatal(err)
			}
			request.Header.Set("Authorization", "Bearer synthetic-test-key")
			if name == "endpoint" {
				request.URL.Host = "example.invalid"
			}
			response, err := recorder.RoundTrip(request)
			if response != nil {
				_ = response.Body.Close()
			}
			good := name == "valid" || name == "repeat"
			if (err == nil) != good {
				t.Fatalf("unexpected transport result: %v", err)
			}
			wantCalls := 1
			if name == "endpoint" || name == "guard" || name == "budget" || name == "case_id" {
				wantCalls = 0
			}
			if calls != wantCalls {
				t.Fatalf("requests=%d want=%d", calls, wantCalls)
			}
			if name == "repeat" {
				request, err := http.NewRequest(http.MethodPost, "https://api.openai.com/v1/responses", bytes.NewReader(body))
				if err != nil {
					t.Fatal(err)
				}
				request.Header.Set("Authorization", "Bearer synthetic-test-key")
				if _, err := recorder.RoundTrip(request); err == nil || calls != 1 {
					t.Fatal("consumed attempt retried")
				}
			}
		})
	}
}

func formatReceipt(number int) string { return fmt.Sprintf("%03d.reservation.json", number) }

func TestGenerateSingleCorrectionAndNoProviderRetry(t *testing.T) {
	_, _, instructions := testRequest(t)
	capabilities, err := loadCapabilities(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"corrected", "still_invalid", "transport_error"} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			journal := filepath.Join(root, "journal")
			if err := os.Mkdir(journal, 0o700); err != nil {
				t.Fatal(err)
			}
			calls := 0
			recorder := &recordingTransport{Journal: journal, Secret: "synthetic-test-key", InstructionSHA: instructions, Guard: func() error { return nil }, Base: roundTripFunc(func(*http.Request) (*http.Response, error) {
				calls++
				if name == "transport_error" {
					return nil, errors.New("synthetic interrupted call")
				}
				p := syntheticRefusal()
				if calls == 1 || name == "still_invalid" {
					p.Coverage[0].Disposition = "invalid_disposition"
				}
				return syntheticResponse(t, p), nil
			})}
			item := Case{ID: "R01", Kind: "refusal", Prompt: "Require a trusted synthetic verification."}
			result, err := generate(context.Background(), item, capabilities, filepath.Join(root, "initial"), "initial", nil, recorder)
			if name == "transport_error" {
				if err == nil || calls != 1 || result.Attempts != 1 {
					t.Fatal("transport error retried")
				}
			} else {
				if err != nil || calls != 2 || result.Attempts != 2 || result.FirstStatus != behavioralintent.StatusInvalid {
					t.Fatalf("bounded correction failed: %v", err)
				}
				want := behavioralintent.StatusUnsupported
				if name == "still_invalid" {
					want = behavioralintent.StatusInvalid
				}
				if result.Compilation.Status != want {
					t.Fatal("unexpected final compilation")
				}
			}
		})
	}
}

func syntheticClarification() behavioralintent.Proposal {
	return behavioralintent.Proposal{Version: behavioralintent.ProposalVersion, Coverage: []behavioralintent.CoverageRecord{{StatementID: "statement_001", Disposition: behavioralintent.DispositionClarification, Rationale: "Cutoff not provided", References: []behavioralintent.Reference{{Kind: "clarification", ID: "cutoff_question"}, {Kind: "uncertainty", ID: "cutoff_unknown"}}}}, Uncertainties: []behavioralintent.Uncertainty{{ID: "cutoff_unknown", Path: "requirements.objectives.filter", Kind: "cutoff_bound", Description: "Cutoff not supplied", Resolution: behavioralintent.ResolutionClarification, ResolvedBy: "cutoff_question"}}, Clarifications: []behavioralintent.Clarification{{ID: "cutoff_question", Path: "requirements.objectives.filter", Question: "What cutoff is required?", WhyNeeded: "A measurable cutoff is needed", UncertaintyIDs: []string{"cutoff_unknown"}}}}
}

func syntheticReady() behavioralintent.Proposal {
	f := func(x float64) *float64 { return &x }
	requirement := &architecturesearch.Requirement{Schema: architecturesearch.SchemaIDV3, Version: 3, Project: architecturesearch.Project{Name: "offline_filter", Title: "Offline filter", Description: "Independent offline transport integration fixture"}, Requirements: architecturesearch.Requirements{
		Domains:                []architecturesearch.Domain{{ID: "ground", Kind: "reference", Source: "external"}, {ID: "supply", Kind: "supply", Source: "external", NominalVoltageV: 3, MinVoltageV: f(2.9), MaxVoltageV: f(3.1)}},
		Ports:                  []architecturesearch.Port{{ID: "gnd", Kind: "reference", Direction: "sink", Domain: "ground"}, {ID: "vcc", Kind: "power", Direction: "sink", Domain: "supply"}, {ID: "input", Kind: "analog_voltage", Direction: "sink", Domain: "supply"}, {ID: "output", Kind: "analog_voltage", Direction: "source", Domain: "supply"}},
		Objectives:             []architecturesearch.Objective{{ID: "filter", Capability: "frequency_filter", Bindings: []architecturesearch.Binding{{Role: "input", Port: "input"}, {Role: "output", Port: "output"}}}},
		OperatingCases:         []architecturesearch.OperatingCase{{ID: "supply_case", Conditions: []architecturesearch.OperatingCondition{{Axis: "supply_voltage", Target: "supply", Min: f(2.9), Max: f(3.1), Unit: "V"}}}},
		BehavioralRequirements: []architecturesearch.BehavioralRequirement{{ID: "cutoff", Metric: "cutoff_frequency", Analysis: "ac_sweep", Unit: "Hz", Min: f(1350), Max: f(1650), OperatingCases: []string{"supply_case"}, Observation: architecturesearch.Observation{Kind: "port", ID: "output"}}},
		Constraints:            architecturesearch.BoardLimits{MaxComponents: 10, MaxWidthMM: 30, MaxHeightMM: 20},
	}}
	refs := []behavioralintent.Reference{}
	for _, id := range []string{"ground", "supply", "gnd", "vcc", "input", "output", "filter", "supply_case", "cutoff"} {
		refs = append(refs, behavioralintent.Reference{Kind: "requirement", ID: id})
	}
	return behavioralintent.Proposal{Version: behavioralintent.ProposalVersion, Requirement: requirement, Coverage: []behavioralintent.CoverageRecord{{StatementID: "statement_001", Disposition: behavioralintent.DispositionCompiled, Rationale: "Original interfaces and answered cutoff retained", References: refs}}}
}

func TestRunCaseRequiresBoundReviewBeforeFixedAnswer(t *testing.T) {
	_, _, instructions := testRequest(t)
	capabilities, err := loadCapabilities(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, approve := range []bool{false, true} {
		t.Run(fmt.Sprint(approve), func(t *testing.T) {
			parent := t.TempDir()
			root := filepath.Join(parent, "C01")
			journal := filepath.Join(parent, "journal")
			if err := os.Mkdir(journal, 0o700); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			reviewError := make(chan error, 1)
			go func() {
				ticker := time.NewTicker(10 * time.Millisecond)
				defer ticker.Stop()
				for {
					data, err := os.ReadFile(filepath.Join(root, "initial", "selected.json"))
					if err == nil && json.Valid(data) {
						reviewError <- writeJSON(filepath.Join(root, "answer-review.json"), ReviewDecision{"C01", sha(data), approve, "offline test", "Question matches only the missing cutoff"})
						return
					}
					select {
					case <-ctx.Done():
						reviewError <- ctx.Err()
						return
					case <-ticker.C:
					}
				}
			}()
			calls := 0
			recorder := &recordingTransport{Journal: journal, Secret: "synthetic-test-key", InstructionSHA: instructions, Guard: func() error { return nil }, Base: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				if calls == 1 {
					return syntheticResponse(t, syntheticClarification()), nil
				}
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Fatal(err)
				}
				var wire wireRequest
				if err := json.Unmarshal(body, &wire); err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(wire.Input, "prior_compilation_sha256") || !strings.Contains(wire.Input, "1500 Hz") {
					t.Fatal("fixed answer lacks bound context")
				}
				return syntheticResponse(t, syntheticReady()), nil
			})}
			item := Case{ID: "C01", Kind: "clarification", Prompt: "Use an external 3 V supply ranging from 2.9 to 3.1 V with ground and analog input/output, at most ten components within 30 by 20 mm, and ask for the filter cutoff.", Answer: "Use 1500 Hz within ten percent across the full supply range."}
			result, err := runCase(ctx, item, capabilities, root, recorder)
			if reviewErr := <-reviewError; reviewErr != nil {
				t.Fatal(reviewErr)
			}
			if err != nil {
				t.Fatal(err)
			}
			if approve {
				if calls != 2 || result.Status != "clarification_candidate" || result.FollowUpStatus != behavioralintent.StatusReady {
					t.Fatalf("bound-answer workflow failed: %#v", result)
				}
			} else if calls != 1 || result.Status != "clarification_review_rejected" {
				t.Fatal("rejected review dispatched an answer")
			}
		})
	}
}

func TestCorpusAndUnsealedBuildCannotStartLive(t *testing.T) {
	var corpus Corpus
	if err := readJSON(filepath.Join("..", "..", SpecPath, "corpus.json"), &corpus); err != nil {
		t.Fatal(err)
	}
	if err := corpus.Validate(); err != nil {
		t.Fatal(err)
	}
	corpus.Cases[7].ID = "I01"
	if corpus.Validate() == nil {
		t.Fatal("changed denominator accepted")
	}
	if _, _, _, err := VerifyFreeze(t.TempDir()); err == nil {
		t.Fatal("unsealed build accepted")
	}
}

func TestCredentialScanPreservesOrdinaryHyphenatedWords(t *testing.T) {
	text := []byte("task-specific and risk-aware are ordinary prose")
	if !bytes.Equal(redact(text, "synthetic-test-key"), text) {
		t.Fatal("ordinary prose was mistaken for a key")
	}
	if bytes.Contains(redact([]byte("key: "+"sk-"+strings.Repeat("a", 32)), "unrelated"), []byte("sk-")) {
		t.Fatal("key-shaped string escaped")
	}
	if bytes.Contains(redact([]byte("synthetic-test-key"), "synthetic-test-key"), []byte("synthetic-test-key")) {
		t.Fatal("exact credential escaped")
	}
}

func TestResourceTreeRejectsSymlinks(t *testing.T) {
	root := t.TempDir()
	if err := writeNew(filepath.Join(root, "plain"), []byte("abc")); err != nil {
		t.Fatal(err)
	}
	if size, err := directoryBytes(root); err != nil || size != 3 {
		t.Fatal("incorrect byte count")
	}
	if err := os.Symlink("plain", filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	if _, err := directoryBytes(root); err == nil {
		t.Fatal("symlink accepted in evidence tree")
	}
}

func TestOfflineReplayRecompilesAndRejectsInventoryTampering(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "raw")
	frozen := filepath.Join(root, "frozen-inputs", SpecPath)
	initial := filepath.Join(root, "I01", "initial")
	for _, dir := range []string{filepath.Join(frozen, "snapshot"), initial} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	var corpus Corpus
	if err := readJSON(filepath.Join("..", "..", SpecPath, "corpus.json"), &corpus); err != nil {
		t.Fatal(err)
	}
	capabilities, err := loadCapabilities(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(frozen, "corpus.json"), corpus); err != nil {
		t.Fatal(err)
	}
	if err := writeNew(filepath.Join(frozen, "snapshot", "installed-capabilities.json"), capabilities); err != nil {
		t.Fatal(err)
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	binary, err := os.ReadFile(self)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(root, "campaign-start.json"), map[string]any{"binary_sha256": sha(binary)}); err != nil {
		t.Fatal(err)
	}
	outcomes := []caseResult{}
	for i, item := range corpus.Cases {
		status := "not_run"
		if i == 0 {
			status = "failed"
		}
		outcomes = append(outcomes, caseResult{CaseID: item.ID, Kind: item.Kind, Status: status})
	}
	if err := writeJSON(filepath.Join(root, "campaign-end.json"), map[string]any{"outcomes": outcomes}); err != nil {
		t.Fatal(err)
	}
	proposal := syntheticReady()
	intent, err := json.Marshal(proposal)
	if err != nil {
		t.Fatal(err)
	}
	decoded, issues := behavioralintent.DecodeProposalStrict(bytes.NewReader(intent))
	compiled := behavioralintent.Compile(corpus.Cases[0].Prompt, decoded, sha(capabilities))
	compiled.Issues = append(issues, compiled.Issues...)
	if reports.HasBlockingIssue(compiled.Issues) {
		compiled.Status = behavioralintent.StatusInvalid
		compiled.Requirement = nil
	}
	if err := writeNew(filepath.Join(initial, "attempt-1.intent.json"), intent); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(initial, "attempt-1.compilation.json"), compiled); err != nil {
		t.Fatal(err)
	}
	files, err := practicalboardeval.Inventory(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(root, "inventory.json"), files); err != nil {
		t.Fatal(err)
	}
	report := filepath.Join(parent, "replay.json")
	if err := Replay(root, report); err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := readJSON(report, &result); err != nil {
		t.Fatal(err)
	}
	if len(result["replayed_attempts"].([]any)) != 1 || result["provider_calls"].(float64) != 0 {
		t.Fatal("replay did not verify the recorded attempt")
	}
	if err := Replay(root, filepath.Join(root, "mutating-report.json")); err == nil {
		t.Fatal("replay mutated immutable root")
	}
	if err := os.WriteFile(filepath.Join(initial, "attempt-1.intent.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Replay(root, filepath.Join(parent, "bad-replay.json")); err == nil {
		t.Fatal("tampered inventory accepted")
	}
}

func TestCanonicalCompilationOnlyIgnoresIssueOrder(t *testing.T) {
	a := behavioralintent.Result{Status: behavioralintent.StatusInvalid, Issues: []reports.Issue{
		{Code: "a", Severity: reports.SeverityError, Path: "first", Message: "first issue"},
		{Code: "b", Severity: reports.SeverityError, Path: "second", Message: "second issue"},
	}}
	b := a
	b.Issues = slices.Clone(a.Issues)
	slices.Reverse(b.Issues)
	left, err := canonicalCompilation(a)
	if err != nil {
		t.Fatal(err)
	}
	right, err := canonicalCompilation(b)
	if err != nil || !bytes.Equal(left, right) {
		t.Fatal("issue permutation changed the semantic compilation comparison")
	}
	for _, mutation := range []func(*behavioralintent.Result){
		func(r *behavioralintent.Result) { r.Issues[0].Message = "changed" },
		func(r *behavioralintent.Result) { r.Issues = append(r.Issues, r.Issues[0]) },
		func(r *behavioralintent.Result) { r.Status = behavioralintent.StatusReady },
		func(r *behavioralintent.Result) { r.CapabilitySHA256 = strings.Repeat("a", 64) },
	} {
		changed := a
		changed.Issues = slices.Clone(a.Issues)
		mutation(&changed)
		right, err := canonicalCompilation(changed)
		if err != nil || bytes.Equal(left, right) {
			t.Fatal("material compilation change was hidden")
		}
	}
}
