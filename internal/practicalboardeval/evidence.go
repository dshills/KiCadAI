// Package practicalboardeval contains the isolated, fail-closed evaluator for
// the practical board experiment. It is not a production synthesis policy.
package practicalboardeval

import (
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"kicadai/internal/architecturesearch"
	"kicadai/internal/designworkflow"
	"kicadai/internal/kicadfiles/roundtrip"
	"kicadai/internal/reports"
)

const Model = "gpt-5.6-sol"
const MaxOutputTokens = 16384
const MaxRequestBytes = 131072
const MaxRequests = 72
const MaxCampaignRequests = 36
const MaxSpendUSD = 50.0

// FreezeSHA256 is set by the documented -ldflags build after the freeze commit.
// An ordinary development build cannot execute a live corpus campaign.
var FreezeSHA256 string

type Case struct {
	ID                  string   `json:"id"`
	Kind                string   `json:"kind"`
	Reserved            bool     `json:"reserved,omitempty"`
	Title               string   `json:"title"`
	Prompt              string   `json:"prompt"`
	Expected            string   `json:"expected,omitempty"`
	ClarificationAnswer string   `json:"clarification_answer,omitempty"`
	Acceptance          []string `json:"acceptance"`
}

type Paraphrase struct {
	ID     string `json:"id"`
	Of     string `json:"of"`
	Prompt string `json:"prompt"`
}

type Corpus struct {
	Schema           string       `json:"schema"`
	Status           string       `json:"status"`
	Authorship       string       `json:"authorship"`
	CommonPrompt     string       `json:"positive_common_prompt"`
	CommonAcceptance []string     `json:"positive_common_acceptance"`
	Cases            []Case       `json:"cases"`
	Paraphrases      []Paraphrase `json:"paraphrases"`
}

func (c Corpus) Inputs() ([]Case, error) {
	if c.Schema != "kicadai.practical-board-corpus.v1" || c.CommonPrompt == "" || len(c.CommonAcceptance) == 0 {
		return nil, fmt.Errorf("invalid corpus identity or common acceptance")
	}
	seen := map[string]Case{}
	counts := map[string]int{}
	reserved := 0
	var out []Case
	for _, item := range c.Cases {
		if !SafeName(item.ID) || item.Prompt == "" || len(item.Acceptance) == 0 || seen[item.ID].ID != "" {
			return nil, fmt.Errorf("invalid or duplicate case %q", item.ID)
		}
		if !slices.Contains([]string{"positive", "refusal", "clarification"}, item.Kind) {
			return nil, fmt.Errorf("invalid case kind %q", item.Kind)
		}
		if item.Kind != "positive" && item.Expected == "" {
			return nil, fmt.Errorf("missing predetermined expectation: %s", item.ID)
		}
		if item.Kind == "clarification" && item.ClarificationAnswer == "" {
			return nil, fmt.Errorf("missing fixed clarification answer: %s", item.ID)
		}
		if item.Reserved {
			if item.Kind != "positive" {
				return nil, fmt.Errorf("reserved nonpositive case")
			}
			reserved++
		}
		counts[item.Kind]++
		seen[item.ID] = item
		if item.Kind == "positive" {
			item.Prompt += "\n\n" + c.CommonPrompt
			item.Acceptance = append(slices.Clone(item.Acceptance), c.CommonAcceptance...)
		}
		out = append(out, item)
	}
	if counts["positive"] != 8 || counts["refusal"] != 4 || counts["clarification"] != 2 || reserved != 2 || len(c.Paraphrases) != 2 {
		return nil, fmt.Errorf("corpus denominator or reserved membership invalid")
	}
	for _, paraphrase := range c.Paraphrases {
		parent, ok := seen[paraphrase.Of]
		if !ok || parent.Kind != "positive" || parent.Reserved || !SafeName(paraphrase.ID) || seen[paraphrase.ID].ID != "" || paraphrase.Prompt == "" {
			return nil, fmt.Errorf("invalid paraphrase %q", paraphrase.ID)
		}
		parent.ID, parent.Kind, parent.Prompt = paraphrase.ID, "paraphrase", paraphrase.Prompt+"\n\n"+c.CommonPrompt
		parent.Acceptance = append(slices.Clone(parent.Acceptance), c.CommonAcceptance...)
		seen[parent.ID] = parent
		out = append(out, parent)
	}
	return out, nil
}

func SafeName(s string) bool {
	if s == "" || s == "." || s == ".." {
		return false
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
			return false
		}
	}
	return true
}

func SHA(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func WriteNew(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	_, writeErr := f.Write(data)
	syncErr := f.Sync()
	closeErr := f.Close()
	if writeErr != nil {
		return writeErr
	}
	if syncErr != nil {
		return syncErr
	}
	return closeErr
}

func WriteJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return WriteNew(path, append(data, '\n'))
}

// Compression retains the complete stock-library evidence without writing
// gigabytes of repeated indentation. Gzip's default zero timestamp is stable.
func WriteGzipJSON(path string, value any) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	z := gzip.NewWriter(f)
	encodeErr := json.NewEncoder(z).Encode(value)
	gzipErr := z.Close()
	syncErr := f.Sync()
	closeErr := f.Close()
	if encodeErr != nil {
		return encodeErr
	}
	if gzipErr != nil {
		return gzipErr
	}
	if syncErr != nil {
		return syncErr
	}
	return closeErr
}

func ReadJSON(path string, value any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return fmt.Errorf("trailing JSON in %s", path)
	}
	return nil
}

type FileRecord struct {
	Path   string `json:"path"`
	Bytes  int64  `json:"bytes"`
	SHA256 string `json:"sha256"`
}

func Inventory(root string) ([]FileRecord, error) {
	var records []FileRecord
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink evidence: %s", path)
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("nonregular evidence: %s", path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		records = append(records, FileRecord{filepath.ToSlash(rel), int64(len(data)), SHA(data)})
		return nil
	})
	return records, err
}

type Freeze struct {
	Schema         string       `json:"schema"`
	BaselineCommit string       `json:"baseline_commit"`
	Files          []FileRecord `json:"files"`
}

func VerifyFreeze(root string, freeze Freeze) error {
	if freeze.Schema != "kicadai.practical-board-freeze.v1" || freeze.BaselineCommit != "87c411b7a13bdeb0efc6ff36b35e9a69c4e2706d" || len(freeze.Files) == 0 {
		return fmt.Errorf("invalid freeze identity")
	}
	seen := map[string]bool{}
	for _, record := range freeze.Files {
		if !filepath.IsLocal(record.Path) || filepath.Clean(record.Path) != record.Path || seen[record.Path] {
			return fmt.Errorf("invalid freeze path %q", record.Path)
		}
		seen[record.Path] = true
		path := filepath.Join(root, record.Path)
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("nonregular frozen file: %s", record.Path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if int64(len(data)) != record.Bytes || SHA(data) != record.SHA256 {
			return fmt.Errorf("freeze mismatch: %s", record.Path)
		}
	}
	return nil
}

var requiredStages = []designworkflow.StageName{
	designworkflow.StageSchematic, designworkflow.StageSchematicElectrical,
	designworkflow.StagePlacement, designworkflow.StageRouting,
	designworkflow.StageProjectWrite, designworkflow.StageWriterCorrect,
	designworkflow.StageValidation, designworkflow.StageKiCadChecks,
	designworkflow.StageSimulation,
}

// FirstFailedStage never equates missing, warning, or skipped native checks
// with a complete pass. A clean result still needs the independent clause and
// readability audits; this function cannot declare milestone success.
func FirstFailedStage(result designworkflow.WorkflowResult) string {
	for _, name := range requiredStages {
		count := 0
		for _, stage := range result.Stages {
			if stage.Name == name {
				count++
				if stage.Status != designworkflow.StageStatusOK || reports.HasBlockingIssue(stage.Issues) {
					return string(name)
				}
			}
		}
		if count != 1 {
			return string(name)
		}
	}
	if result.Feedback.Summary.BlockingCount != 0 {
		return "workflow_feedback"
	}
	return ""
}

// CommonRequirementGate is an acceptance check, not an automatic correction.
// A model that drops a required gate must not receive a silently strengthened
// contract from this evaluator.
func CommonRequirementGate(r architecturesearch.Requirement) string {
	a := r.Acceptance
	checks := []struct {
		name    string
		enabled bool
	}{
		{"require_erc", a.RequireERC}, {"require_strict_drc", a.RequireStrictDRC},
		{"require_complete_routing", a.RequireCompleteRouting}, {"require_connectivity", a.RequireConnectivity},
		{"require_writer_correctness", a.RequireWriterCorrectness}, {"require_round_trip_zero_diff", a.RequireRoundTripZeroDiff},
		{"require_deterministic_replay", a.RequireDeterministicReplay}, {"require_simulation", a.RequireSimulation},
		{"require_all_corners", a.RequireAllCorners}, {"require_model_provenance", a.RequireModelProvenance},
		{"require_closed_loop_evidence", a.RequireClosedLoopEvidence},
	}
	for _, check := range checks {
		if !check.enabled {
			return "acceptance." + check.name
		}
	}
	b := r.Requirements.Constraints
	if b.MaxWidthMM <= 0 || b.MaxWidthMM > 100 || b.MaxHeightMM <= 0 || b.MaxHeightMM > 80 {
		return "requirements.constraints.board_enclosure"
	}
	ambientCovered := false
	for _, operatingCase := range r.Requirements.OperatingCases {
		for _, condition := range operatingCase.Conditions {
			if condition.Axis == "ambient_temperature" && condition.Unit == "degC" && condition.Min != nil && condition.Max != nil && *condition.Min <= -10 && *condition.Max >= 50 {
				ambientCovered = true
			}
		}
	}
	if !ambientCovered {
		return "requirements.operating_cases.ambient_temperature"
	}
	return ""
}

var nativeTemporaryPath = regexp.MustCompile(`(<case-output>/project/\.kicadai/(?:checks|roundtrip)/[^/\s]+)-[0-9]+(/)`)

// NormalizeEvidence substitutes a known output root, native temporary directory
// suffixes, and only the native ERC/DRC command durations declared in SPEC.
// Electrical measurements, statuses and persistent identities are unchanged.
func NormalizeEvidence(value any, root string) ([]byte, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	if root == "" {
		return nil, fmt.Errorf("empty normalization root")
	}
	var decoded any
	if err := json.Unmarshal(data, &decoded); err != nil {
		return nil, err
	}
	if top, ok := decoded.(map[string]any); ok {
		if workflow, ok := top["workflow"].(map[string]any); ok {
			if stages, ok := workflow["stages"].([]any); ok {
				for _, raw := range stages {
					stage, ok := raw.(map[string]any)
					if !ok || stage["name"] != "kicad_checks" {
						continue
					}
					summary, ok := stage["summary"].(map[string]any)
					if !ok {
						continue
					}
					for _, name := range []string{"erc", "drc"} {
						if command, ok := summary[name].(map[string]any); ok {
							if duration, ok := command["duration_ms"].(float64); ok && duration >= 0 {
								command["duration_ms"] = "<native-command-elapsed>"
							}
						}
					}
				}
			}
		}
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		realRoot = root
	}
	var normalize func(any) any
	normalize = func(v any) any {
		switch v := v.(type) {
		case string:
			v = strings.ReplaceAll(v, realRoot, "<case-output>")
			v = strings.ReplaceAll(v, root, "<case-output>")
			return nativeTemporaryPath.ReplaceAllString(v, `${1}-<temporary>${2}`)
		case []any:
			for i := range v {
				v[i] = normalize(v[i])
			}
			return v
		case map[string]any:
			for key := range v {
				v[key] = normalize(v[key])
			}
			return v
		default:
			return v
		}
	}
	return json.Marshal(normalize(decoded))
}

func ProjectIdentity(root string) (map[string]string, error) {
	identity := map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("project contains symlink: %s", path)
		}
		if entry.IsDir() {
			if entry.Name() == ".kicadai" {
				return filepath.SkipDir
			}
			return nil
		}
		ext := filepath.Ext(path)
		if !slices.Contains([]string{".kicad_sch", ".kicad_pcb", ".kicad_pro", ".kicad_sym", ".kicad_mod"}, ext) && entry.Name() != "sym-lib-table" && entry.Name() != "fp-lib-table" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		identity[filepath.ToSlash(rel)] = SHA([]byte(roundtrip.NormalizeBytes(data)))
		return nil
	})
	return identity, err
}
