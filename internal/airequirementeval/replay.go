package airequirementeval

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"

	"kicadai/internal/behavioralintent"
	"kicadai/internal/practicalboardeval"
	"kicadai/internal/reports"
)

// canonicalCompilation preserves all values and array orders except the issue
// list: existing validation traverses maps, so diagnostic order is not stable.
// Sort full encoded issues, preserving duplicates and every diagnostic field.
// Raw recorded bytes and correction-diagnostic order remain untouched.
func canonicalCompilation(result behavioralintent.Result) ([]byte, error) {
	encoded, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		return nil, err
	}
	var issues []json.RawMessage
	// Result omits issues when empty. Preserve that absence instead of decoding
	// a nil RawMessage (which reports unexpected end of JSON input).
	if raw, present := fields["issues"]; present {
		if err := json.Unmarshal(raw, &issues); err != nil {
			return nil, err
		}
	}
	if len(issues) > 1 {
		slices.SortFunc(issues, func(a, b json.RawMessage) int { return bytes.Compare(a, b) })
		fields["issues"], err = json.Marshal(issues)
		if err != nil {
			return nil, err
		}
	}
	return json.Marshal(fields)
}

// VerifyReady checks the sealed local environment without creating a campaign,
// reading a credential or contacting a provider.
func VerifyReady(ctx context.Context, repo string) error {
	_, _, capabilities, err := VerifyFreeze(repo)
	if err != nil {
		return err
	}
	current, err := loadCapabilities(ctx)
	if err != nil || !bytes.Equal(current, capabilities) {
		return fmt.Errorf("installed capabilities changed since freeze")
	}
	if _, err := os.Lstat(EvidenceRoot); !os.IsNotExist(err) {
		return fmt.Errorf("live root must not already exist")
	}
	rss, err := processTreeRSS(ctx)
	if err != nil {
		return err
	}
	if rss > 16<<30 {
		return fmt.Errorf("pre-live RSS exceeds ceiling")
	}
	emit(map[string]any{"event": "sealed_preflight_verified", "freeze_sha256": FreezeSHA256, "capability_sha256": sha(capabilities), "process_tree_rss_bytes": rss, "live_root_absent": true, "provider_requests": 0})
	return nil
}

// Replay authenticates and recompiles every recorded intent offline. Output is
// outside the immutable terminal tree; no provider implementation is constructed.
func Replay(root, output string) error {
	root, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	output, err = filepath.Abs(output)
	if err != nil {
		return err
	}
	if output == root || strings.HasPrefix(output, root+string(os.PathSeparator)) {
		return fmt.Errorf("replay report must be outside the immutable raw tree")
	}
	if err := practicalboardeval.VerifyInventory(root); err != nil {
		return err
	}
	var end struct {
		Outcomes []caseResult `json:"outcomes"`
	}
	var start struct {
		BinarySHA256 string `json:"binary_sha256"`
	}
	startBytes, err := os.ReadFile(filepath.Join(root, "campaign-start.json"))
	if err != nil {
		return err
	}
	if err := json.Unmarshal(startBytes, &start); err != nil {
		return err
	}
	self, err := os.Executable()
	if err != nil {
		return err
	}
	binary, err := os.ReadFile(self)
	if err != nil {
		return err
	}
	if sha(binary) != start.BinarySHA256 {
		return fmt.Errorf("offline replay must use the same sealed evaluator binary")
	}
	// End metadata has other authenticated fields; the outer inventory binds them.
	endBytes, err := os.ReadFile(filepath.Join(root, "campaign-end.json"))
	if err != nil {
		return err
	}
	if err := json.Unmarshal(endBytes, &end); err != nil {
		return err
	}
	var corpus Corpus
	if err := readJSON(filepath.Join(root, "frozen-inputs", SpecPath, "corpus.json"), &corpus); err != nil {
		return err
	}
	if err := corpus.Validate(); err != nil {
		return err
	}
	capabilities, err := os.ReadFile(filepath.Join(root, "frozen-inputs", SpecPath, "snapshot", "installed-capabilities.json"))
	if err != nil {
		return err
	}
	capabilities = bytes.TrimSpace(capabilities)
	if err := validateCapabilities(corpus, capabilities); err != nil {
		return err
	}
	if len(end.Outcomes) != len(corpus.Cases) {
		return fmt.Errorf("terminal denominator mismatch")
	}
	replayed := []map[string]any{}
	tampered := []map[string]any{}
	for i, item := range corpus.Cases {
		if end.Outcomes[i].CaseID != item.ID || end.Outcomes[i].Kind != item.Kind {
			return fmt.Errorf("terminal case identity mismatch")
		}
		if end.Outcomes[i].Status == "not_run" {
			continue
		}
		for _, leg := range []string{"initial", "follow-up"} {
			dir := filepath.Join(root, item.ID, leg)
			if _, err := os.Stat(dir); os.IsNotExist(err) {
				continue
			} else if err != nil {
				return err
			}
			var prior legResult
			var input behavioralintent.FollowUp
			if leg == "follow-up" {
				if err := readJSON(filepath.Join(root, item.ID, "initial", "selected.json"), &prior); err != nil {
					return err
				}
				if err := readJSON(filepath.Join(root, item.ID, "bound-answer.json"), &input); err != nil {
					return err
				}
				if _, issues := behavioralintent.ValidateFollowUp(item.Prompt, prior.Proposal, prior.Compilation, input, sha(capabilities)); reports.HasBlockingIssue(issues) {
					return fmt.Errorf("retained bound answer does not validate")
				}
				for _, field := range []string{"SourceSHA256", "CapabilitySHA256", "PriorProposalSHA256", "PriorCompilationSHA256"} {
					bad := input
					reflect.ValueOf(&bad).Elem().FieldByName(field).SetString(strings.Repeat("0", 64))
					if _, issues := behavioralintent.ValidateFollowUp(item.Prompt, prior.Proposal, prior.Compilation, bad, sha(capabilities)); !reports.HasBlockingIssue(issues) {
						return fmt.Errorf("tampered answer binding escaped: %s", field)
					}
					tampered = append(tampered, map[string]any{"case_id": item.ID, "field": field, "rejected": true})
				}
			}
			for attempt := 1; attempt <= 2; attempt++ {
				prefix := filepath.Join(dir, fmt.Sprintf("attempt-%d", attempt))
				intent, err := os.ReadFile(prefix + ".intent.json")
				if os.IsNotExist(err) {
					continue
				}
				if err != nil {
					return err
				}
				proposal, issues := behavioralintent.DecodeProposalStrict(bytes.NewReader(intent))
				compiled := behavioralintent.Compile(item.Prompt, proposal, sha(capabilities))
				if leg == "follow-up" {
					compiled = behavioralintent.CompileFollowUp(item.Prompt, prior.Proposal, prior.Compilation, input, proposal, sha(capabilities))
				}
				compiled.Issues = append(issues, compiled.Issues...)
				if reports.HasBlockingIssue(compiled.Issues) {
					compiled.Status = behavioralintent.StatusInvalid
					compiled.Requirement = nil
				}
				var recorded behavioralintent.Result
				if err := readJSON(prefix+".compilation.json", &recorded); err != nil {
					return err
				}
				a, err := canonicalCompilation(compiled)
				if err != nil {
					return err
				}
				b, err := canonicalCompilation(recorded)
				if err != nil {
					return err
				}
				if !bytes.Equal(a, b) {
					return fmt.Errorf("offline compilation differs: %s %s %d (replayed %s, recorded %s)", item.ID, leg, attempt, sha(a), sha(b))
				}
				replayed = append(replayed, map[string]any{"case_id": item.ID, "leg": leg, "attempt": attempt, "intent_sha256": sha(intent), "canonical_compilation_sha256": sha(a), "status": compiled.Status, "issues": len(compiled.Issues)})
			}
		}
	}
	return writeJSON(output, map[string]any{"schema": "kicadai.interface-offline-replay.v1", "raw_root": root, "binary_sha256": sha(binary), "raw_inventory_authenticated": true, "provider_calls": 0, "comparison": "exact typed compilation values with only full issue objects sorted; duplicate issues preserved", "replayed_attempts": replayed, "tampered_answer_controls": tampered, "semantic_faithfulness_audited_by_this_command": false, "board_generation_performed": false})
}
