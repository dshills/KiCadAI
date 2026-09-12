// Post-run, offline-only supplementary audit. It does not replace the sealed replay.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
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

func sha(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func read(path string, v any) error {
	b, e := os.ReadFile(path)
	if e != nil {
		return e
	}
	return json.Unmarshal(b, v)
}

// The frozen comparator fails when the omitempty issues member is absent.
// Preserve that absence; otherwise use its exact full-issue sorting semantics.
func canonical(result behavioralintent.Result) ([]byte, error) {
	b, e := json.Marshal(result)
	if e != nil {
		return nil, e
	}
	var f map[string]json.RawMessage
	if e = json.Unmarshal(b, &f); e != nil {
		return nil, e
	}
	if raw, exists := f["issues"]; exists {
		var issues []json.RawMessage
		if e = json.Unmarshal(raw, &issues); e != nil {
			return nil, e
		}
		if len(issues) > 1 {
			slices.SortFunc(issues, func(a, b json.RawMessage) int { return bytes.Compare(a, b) })
			f["issues"], e = json.Marshal(issues)
			if e != nil {
				return nil, e
			}
		}
	}
	return json.Marshal(f)
}

type selection struct {
	Proposal    behavioralintent.Proposal `json:"proposal"`
	Compilation behavioralintent.Result   `json:"compilation"`
}

func audit(root, output string) error {
	root, e := filepath.Abs(root)
	if e != nil {
		return e
	}
	output, e = filepath.Abs(output)
	if e != nil {
		return e
	}
	if output == root || strings.HasPrefix(output, root+string(os.PathSeparator)) {
		return fmt.Errorf("output must be outside raw root")
	}
	if e = practicalboardeval.VerifyInventory(root); e != nil {
		return e
	}
	var start struct {
		BinarySHA string `json:"binary_sha256"`
		FreezeSHA string `json:"freeze_sha256"`
	}
	if e = read(filepath.Join(root, "campaign-start.json"), &start); e != nil {
		return e
	}
	sealed, e := os.ReadFile("/tmp/kicadai-ai-requirement-eval-v1")
	if e != nil {
		return e
	}
	if sha(sealed) != start.BinarySHA {
		return fmt.Errorf("sealed binary changed")
	}
	fb, e := os.ReadFile(filepath.Join(root, "freeze.json"))
	if e != nil {
		return e
	}
	if sha(fb) != start.FreezeSHA {
		return fmt.Errorf("freeze identity mismatch")
	}
	var freeze struct {
		Files []struct {
			Path string `json:"path"`
			SHA  string `json:"sha256"`
		} `json:"files"`
	}
	if e = json.Unmarshal(fb, &freeze); e != nil {
		return e
	}
	for _, f := range freeze.Files {
		if filepath.IsAbs(f.Path) || filepath.Clean(f.Path) != f.Path || strings.HasPrefix(f.Path, "..") {
			return fmt.Errorf("unsafe frozen path")
		}
		for _, path := range []string{f.Path, filepath.Join(root, "frozen-inputs", f.Path)} {
			b, e := os.ReadFile(path)
			if e != nil {
				return e
			}
			if sha(b) != f.SHA {
				return fmt.Errorf("frozen input changed: %s", f.Path)
			}
		}
	}
	spec := "specs/ai-requirement-contract-integration/live-v1"
	var corpus struct {
		Cases []struct {
			ID     string `json:"id"`
			Kind   string `json:"kind"`
			Prompt string `json:"prompt"`
		} `json:"cases"`
	}
	if e = read(filepath.Join(root, "frozen-inputs", spec, "corpus.json"), &corpus); e != nil {
		return e
	}
	var end struct {
		Requests int `json:"generation_requests"`
		Outcomes []struct {
			ID      string `json:"case_id"`
			Kind    string `json:"kind"`
			Initial int    `json:"initial_attempts"`
			Follow  int    `json:"follow_up_attempts"`
		} `json:"outcomes"`
	}
	if e = read(filepath.Join(root, "campaign-end.json"), &end); e != nil {
		return e
	}
	if len(corpus.Cases) != 8 || len(end.Outcomes) != 8 {
		return fmt.Errorf("denominator mismatch")
	}
	capBytes, e := os.ReadFile(filepath.Join(root, "frozen-inputs", spec, "snapshot/installed-capabilities.json"))
	if e != nil {
		return e
	}
	capHash := sha(bytes.TrimSpace(capBytes))
	attempts := []map[string]any{}
	controls := []map[string]any{}
	for i, item := range corpus.Cases {
		outcome := end.Outcomes[i]
		if outcome.ID != item.ID || outcome.Kind != item.Kind {
			return fmt.Errorf("case identity mismatch")
		}
		for _, leg := range []struct {
			Name  string
			Count int
		}{{"initial", outcome.Initial}, {"follow-up", outcome.Follow}} {
			if leg.Count < 0 || leg.Count > 2 {
				return fmt.Errorf("attempt count outside protocol")
			}
			var prior selection
			var input behavioralintent.FollowUp
			if leg.Name == "follow-up" && leg.Count > 0 {
				if e = read(filepath.Join(root, item.ID, "initial/selected.json"), &prior); e != nil {
					return e
				}
				if e = read(filepath.Join(root, item.ID, "bound-answer.json"), &input); e != nil {
					return e
				}
				if _, issues := behavioralintent.ValidateFollowUp(item.Prompt, prior.Proposal, prior.Compilation, input, capHash); reports.HasBlockingIssue(issues) {
					return fmt.Errorf("bound answer rejected: %s", item.ID)
				}
				for _, field := range []string{"SourceSHA256", "CapabilitySHA256", "PriorProposalSHA256", "PriorCompilationSHA256"} {
					bad := input
					reflect.ValueOf(&bad).Elem().FieldByName(field).SetString(strings.Repeat("0", 64))
					if _, issues := behavioralintent.ValidateFollowUp(item.Prompt, prior.Proposal, prior.Compilation, bad, capHash); !reports.HasBlockingIssue(issues) {
						return fmt.Errorf("tampering escaped: %s %s", item.ID, field)
					}
					controls = append(controls, map[string]any{"case_id": item.ID, "field": field, "rejected": true})
				}
			}
			for n := 1; n <= leg.Count; n++ {
				relative := fmt.Sprintf("%s/%s/attempt-%d", item.ID, leg.Name, n)
				prefix := filepath.Join(root, relative)
				intent, e := os.ReadFile(prefix + ".intent.json")
				if e != nil {
					return e
				}
				proposal, issues := behavioralintent.DecodeProposalStrict(bytes.NewReader(intent))
				compiled := behavioralintent.Compile(item.Prompt, proposal, capHash)
				if leg.Name == "follow-up" {
					compiled = behavioralintent.CompileFollowUp(item.Prompt, prior.Proposal, prior.Compilation, input, proposal, capHash)
				}
				compiled.Issues = append(issues, compiled.Issues...)
				if reports.HasBlockingIssue(compiled.Issues) {
					compiled.Status = behavioralintent.StatusInvalid
					compiled.Requirement = nil
				}
				var recorded behavioralintent.Result
				if e = read(prefix+".compilation.json", &recorded); e != nil {
					return e
				}
				a, e := canonical(compiled)
				if e != nil {
					return e
				}
				b, e := canonical(recorded)
				if e != nil {
					return e
				}
				if !bytes.Equal(a, b) {
					return fmt.Errorf("compiler mismatch: %s", relative)
				}
				attempts = append(attempts, map[string]any{"case_id": item.ID, "leg": leg.Name, "attempt": n, "intent_sha256": sha(intent), "canonical_compilation_sha256": sha(a), "status": compiled.Status, "issues": len(compiled.Issues)})
			}
		}
	}
	if len(attempts) != end.Requests || len(controls) != 8 {
		return fmt.Errorf("incomplete supplementary audit")
	}
	self, e := os.Executable()
	if e != nil {
		return e
	}
	binary, e := os.ReadFile(self)
	if e != nil {
		return e
	}
	source, e := os.ReadFile("specs/ai-requirement-contract-integration/publication-live-v1/replay-audit/main.go")
	if e != nil {
		return e
	}
	result := map[string]any{
		"schema":   "kicadai.interface-supplementary-offline-audit.v1",
		"raw_root": root, "sealed_evaluator_binary_sha256": start.BinarySHA,
		"audit_binary_sha256": sha(binary), "audit_source_sha256": sha(source),
		"frozen_inputs_verified": len(freeze.Files), "raw_inventory_authenticated": true,
		"provider_calls": 0, "sealed_replay_passed": false,
		"comparison":        "exact typed compilation values; only full issue objects sorted, duplicates retained; absent issues remain absent",
		"replayed_attempts": attempts, "tampered_answer_controls": controls,
		"production_compiler_modified": false, "board_generation_performed": false,
		"limitation": "Post-run supplementary checker, not the sealed evaluator. The sealed replay failed on an omitted issues member and remains failed.",
	}
	b, e := json.MarshalIndent(result, "", "  ")
	if e != nil {
		return e
	}
	f, e := os.OpenFile(output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return e
	}
	if _, e = f.Write(append(b, '\n')); e != nil {
		f.Close()
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	fmt.Printf("supplementary offline audit: %d attempts matched; %d tampered bindings rejected; 0 provider calls\n", len(attempts), len(controls))
	return nil
}

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: replay-audit RAW_ROOT NEW_OUTPUT")
		os.Exit(2)
	}
	if e := audit(os.Args[1], os.Args[2]); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
