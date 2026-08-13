package capabilityadvancementv10

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"kicadai/internal/atomicdir"
	"kicadai/internal/capabilitybaselinepublicationv10"
	"kicadai/internal/capabilitybaselinev10"
	"kicadai/internal/capabilityroundsv10"
	"kicadai/internal/capabilityselectionv10"
)

var (
	digestPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
	commitPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)
)

const maximumArtifactBytes = 32 << 20

func BuildSeal(
	repositoryRoot string,
	selection capabilityselectionv10.Ranking,
	planSet capabilityselectionv10.PlanSet,
	planSetSHA256 string,
	request ImplementationSeal,
) (ImplementationSeal, error) {
	if _, err := capabilityselectionv10.MarshalRanking(selection); err != nil {
		return ImplementationSeal{}, fmt.Errorf("validate V10 selection: %w", err)
	}
	planBytes, err := capabilityselectionv10.MarshalPlanSet(planSet)
	if err != nil || hashBytes(planBytes) != planSetSHA256 || selection.PlanSetSHA256 != planSetSHA256 {
		return ImplementationSeal{}, fmt.Errorf("V10 implementation plan-set binding is invalid")
	}
	if request.Schema != ImplementationSealSchema || request.Version != Version ||
		request.SelectionSHA256 != selection.Hash || request.PlanSetSHA256 != selection.PlanSetSHA256 ||
		request.SelectedEffectPlanSHA256 != selection.Selection.Selected.EffectPlanSHA256 ||
		!commitPattern.MatchString(request.BaseCommit) || !commitPattern.MatchString(request.ImplementationCommit) ||
		request.BaseCommit == request.ImplementationCommit || !request.FullLocalRegression || !request.InstalledKiCadChecks ||
		!request.PrismReviewComplete || request.FixtureSpecificContent || len(request.Transitions) == 0 || len(request.FocusedTests) == 0 ||
		!sortedUnique(request.FocusedTests) || request.Hash != "" {
		return ImplementationSeal{}, fmt.Errorf("invalid V10 implementation-seal envelope")
	}
	root, err := filepath.Abs(repositoryRoot)
	if err != nil {
		return ImplementationSeal{}, err
	}
	if err := requireCommit(root, "HEAD", request.ImplementationCommit); err != nil {
		return ImplementationSeal{}, err
	}
	if status, statusErr := exec.Command("git", "-C", root, "status", "--porcelain=v1", "--untracked-files=all").Output(); statusErr != nil || len(bytes.TrimSpace(status)) != 0 {
		return ImplementationSeal{}, fmt.Errorf("V10 implementation seal requires a clean worktree")
	}
	if err := exec.Command("git", "-C", root, "merge-base", "--is-ancestor", request.BaseCommit, request.ImplementationCommit).Run(); err != nil {
		return ImplementationSeal{}, fmt.Errorf("V10 implementation base is not an ancestor of the implementation commit")
	}
	changed, err := changedPaths(root, request.BaseCommit, request.ImplementationCommit)
	if err != nil {
		return ImplementationSeal{}, err
	}
	if len(changed) != len(request.Transitions) {
		return ImplementationSeal{}, fmt.Errorf("V10 implementation transition set is incomplete")
	}
	allowed := allowedFiles(planSet, selection.Selection.Selected.EffectPlanSHA256)
	priorPath := ""
	for index, transition := range request.Transitions {
		before, found := allowed[transition.Path]
		if !found || transition.BeforeSHA256 != before.SHA256 || transition.Kind != before.Kind ||
			!validRelativePath(transition.Path) || !digestPattern.MatchString(transition.BeforeSHA256) ||
			!digestPattern.MatchString(transition.AfterSHA256) || priorPath >= transition.Path && priorPath != "" ||
			index >= len(changed) || changed[index] != transition.Path {
			return ImplementationSeal{}, fmt.Errorf("invalid V10 implementation transition %q", transition.Path)
		}
		priorPath = transition.Path
		beforeHash, err := gitBlobSHA256(root, request.BaseCommit, transition.Path)
		if err != nil || beforeHash != transition.BeforeSHA256 {
			return ImplementationSeal{}, fmt.Errorf("V10 implementation before-byte drift at %s", transition.Path)
		}
		afterHash, err := regularFileSHA256(filepath.Join(root, filepath.FromSlash(transition.Path)))
		if err != nil || afterHash != transition.AfterSHA256 || afterHash == transition.BeforeSHA256 {
			return ImplementationSeal{}, fmt.Errorf("V10 implementation after-byte drift at %s", transition.Path)
		}
	}
	request.Hash = ""
	request.Hash, err = sealHash(request)
	return request, err
}

func BuildRound(
	baseline capabilitybaselinepublicationv10.Verification,
	next capabilitybaselinev10.Report,
	selection capabilityselectionv10.Ranking,
	seal ImplementationSeal,
) (Round, error) {
	if err := capabilitybaselinev10.Validate(baseline.Report); err != nil {
		return Round{}, err
	}
	if err := capabilitybaselinev10.Validate(next); err != nil {
		return Round{}, err
	}
	if _, err := capabilityselectionv10.MarshalRanking(selection); err != nil {
		return Round{}, fmt.Errorf("validate V10 selection: %w", err)
	}
	if baseline.ManifestSHA256 != selection.BaselineManifestSHA256 || baseline.Report.Hash != selection.BaselineReportSHA256 ||
		seal.SelectionSHA256 != selection.Hash || seal.SelectedEffectPlanSHA256 != selection.Selection.Selected.EffectPlanSHA256 ||
		baseline.Report.CorpusManifestSHA256 != next.CorpusManifestSHA256 || baseline.Report.EnvironmentSHA256 != next.EnvironmentSHA256 ||
		baseline.Report.EvaluatorManifestSHA256 != next.EvaluatorManifestSHA256 {
		return Round{}, fmt.Errorf("V10 public round binding or environment drift")
	}
	wantSealHash, err := sealHash(seal)
	if err != nil || wantSealHash != seal.Hash || !validSealEnvelope(seal) {
		return Round{}, fmt.Errorf("V10 implementation seal commitment is invalid")
	}
	previousCases := make([]capabilityroundsv10.Case, len(baseline.Report.Cases))
	nextCases := make([]capabilityroundsv10.Case, len(next.Cases))
	for index := range previousCases {
		previousCases[index], nextCases[index] = baseline.Report.Cases[index].Case, next.Cases[index].Case
	}
	evaluation, err := capabilityroundsv10.EvaluateRound(previousCases, nextCases, selection.Selection.Selected, selection.State,
		capabilityroundsv10.RoundEvidence{DeterministicReplayComplete: true, PhysicalPromotionComplete: true, SealEnvironmentValid: true, EffectClosureValid: true},
		capabilityroundsv10.FrozenPolicy())
	if err != nil {
		return Round{}, err
	}
	result := Round{Schema: RoundSchema, Version: Version, Generation: selection.State.Generation + 1,
		BaselineManifestSHA256: baseline.ManifestSHA256, BaselineReportSHA256: baseline.Report.Hash,
		NextReportSHA256: next.Hash, SelectionSHA256: selection.Hash, ImplementationSealSHA256: seal.Hash,
		NextOutcomeCounts: slices.Clone(next.OutcomeCounts), Evaluation: evaluation}
	result.Hash, err = roundHash(result)
	return result, err
}

func DecodeSeal(data []byte) (ImplementationSeal, error) {
	var value ImplementationSeal
	if err := decodeCanonical(data, &value); err != nil {
		return ImplementationSeal{}, err
	}
	want, err := sealHash(value)
	if err != nil || want != value.Hash || !validSealEnvelope(value) {
		return ImplementationSeal{}, fmt.Errorf("V10 implementation seal commitment is invalid")
	}
	return value, nil
}

func DecodeSealRequest(data []byte) (ImplementationSeal, error) {
	var value ImplementationSeal
	if err := decodeCanonical(data, &value); err != nil {
		return ImplementationSeal{}, err
	}
	if value.Hash != "" {
		return ImplementationSeal{}, fmt.Errorf("V10 implementation-seal request must not preclaim a hash")
	}
	return value, nil
}

func DecodeRanking(data []byte) (capabilityselectionv10.Ranking, error) {
	var value capabilityselectionv10.Ranking
	if err := decodeCanonical(data, &value); err != nil {
		return capabilityselectionv10.Ranking{}, err
	}
	want, err := capabilityselectionv10.MarshalRanking(value)
	if err != nil || !bytes.Equal(data, want) {
		return capabilityselectionv10.Ranking{}, fmt.Errorf("V10 ranking commitment is invalid")
	}
	return value, nil
}

func DecodeReport(data []byte) (capabilitybaselinev10.Report, error) {
	var value capabilitybaselinev10.Report
	if err := decodeCanonical(data, &value); err != nil {
		return capabilitybaselinev10.Report{}, err
	}
	want, err := capabilitybaselinev10.MarshalJSONStable(value)
	want = append(want, '\n')
	if err != nil || !bytes.Equal(data, want) {
		return capabilitybaselinev10.Report{}, fmt.Errorf("V10 successor report is invalid")
	}
	return value, nil
}

func MarshalSeal(value ImplementationSeal) ([]byte, error) {
	want, err := sealHash(value)
	if err != nil || want != value.Hash || !validSealEnvelope(value) {
		return nil, fmt.Errorf("V10 implementation seal commitment is invalid")
	}
	return canonicalJSON(value)
}

func MarshalRound(value Round) ([]byte, error) {
	want, err := roundHash(value)
	if err != nil || want != value.Hash {
		return nil, fmt.Errorf("V10 round commitment is invalid")
	}
	return canonicalJSON(value)
}

func Publish(root string, seal ImplementationSeal, round Round) error {
	sealBytes, err := MarshalSeal(seal)
	if err != nil {
		return err
	}
	roundBytes, err := MarshalRound(round)
	if err != nil {
		return err
	}
	audit := []byte(fmt.Sprintf("# V10 Public Capability Round\n\n- status: `%s`\n- discovery passes: %d -> %d\n- new active-cohort passes: %d\n- advanced cases: %d\n- advanced domains: %d\n- advanced circuit roles: %d\n- implementation seal: `%s`\n- result: `%s`\n",
		round.Evaluation.Status, round.Evaluation.DiscoveryPassBefore, round.Evaluation.DiscoveryPassAfter,
		round.Evaluation.NewActiveCohortPasses, len(round.Evaluation.AdvancedCaseIDs), len(round.Evaluation.AdvancedReportingDomains),
		len(round.Evaluation.AdvancedCircuitRoles), seal.Hash, round.Hash))
	checksums := []byte(fmt.Sprintf("%s  IMPLEMENTATION_SEAL.json\n%s  ROUND.json\n%s  ROUND_AUDIT.md\n", hashBytes(sealBytes), hashBytes(roundBytes), hashBytes(audit)))
	return atomicdir.Publish(root, func(staging string) error {
		for name, data := range map[string][]byte{"IMPLEMENTATION_SEAL.json": sealBytes, "ROUND.json": roundBytes, "ROUND_AUDIT.md": audit, "CHECKSUMS.sha256": checksums} {
			if err := os.WriteFile(filepath.Join(staging, name), data, 0o644); err != nil {
				return err
			}
		}
		return nil
	})
}

type allowedFile struct{ SHA256, Kind string }

func allowedFiles(planSet capabilityselectionv10.PlanSet, selectedHash string) map[string]allowedFile {
	result := map[string]allowedFile{}
	for _, plan := range planSet.Plans {
		data, _ := json.Marshal(plan)
		if hashBytes(data) != selectedHash {
			continue
		}
		for _, file := range plan.StaticEvidence.ProductionFiles {
			result[file.Path] = allowedFile{SHA256: file.SHA256, Kind: "production"}
		}
		for _, file := range plan.StaticEvidence.VerificationFiles {
			result[file.Path] = allowedFile{SHA256: file.SHA256, Kind: "verification"}
		}
	}
	return result
}

func validSealEnvelope(value ImplementationSeal) bool {
	return value.Schema == ImplementationSealSchema && value.Version == Version &&
		digestPattern.MatchString(value.SelectionSHA256) && digestPattern.MatchString(value.PlanSetSHA256) &&
		digestPattern.MatchString(value.SelectedEffectPlanSHA256) && digestPattern.MatchString(value.Hash) &&
		commitPattern.MatchString(value.BaseCommit) && commitPattern.MatchString(value.ImplementationCommit) &&
		value.BaseCommit != value.ImplementationCommit && len(value.Transitions) > 0 && len(value.FocusedTests) > 0 &&
		sortedUnique(value.FocusedTests) && value.FullLocalRegression && value.InstalledKiCadChecks &&
		value.PrismReviewComplete && !value.FixtureSpecificContent
}

func changedPaths(root, before, after string) ([]string, error) {
	output, err := exec.Command("git", "-C", root, "diff", "--name-only", "-z", "--diff-filter=ACDMRTUXB", before+".."+after, "--").Output()
	if err != nil {
		return nil, fmt.Errorf("list V10 implementation paths: %w", err)
	}
	fields := bytes.Split(bytes.TrimSuffix(output, []byte{0}), []byte{0})
	lines := make([]string, 0, len(fields))
	for _, field := range fields {
		if len(field) != 0 {
			lines = append(lines, string(field))
		}
	}
	slices.Sort(lines)
	if len(lines) != len(slices.Compact(slices.Clone(lines))) {
		return nil, fmt.Errorf("duplicate V10 implementation path")
	}
	return lines, nil
}

func requireCommit(root, revision, want string) error {
	output, err := exec.Command("git", "-C", root, "rev-parse", revision+"^{commit}").Output()
	if err != nil || strings.TrimSpace(string(output)) != want {
		return fmt.Errorf("V10 implementation commit binding mismatch")
	}
	return nil
}

func gitBlobSHA256(root, revision, relative string) (string, error) {
	output, err := exec.Command("git", "-C", root, "show", revision+":"+relative).Output()
	if err != nil {
		return "", err
	}
	return hashBytes(output), nil
}

func regularFileSHA256(filename string) (string, error) {
	info, err := os.Lstat(filename)
	if err != nil || !info.Mode().IsRegular() || info.Size() > maximumArtifactBytes {
		return "", fmt.Errorf("not a bounded regular file")
	}
	file, err := os.Open(filename)
	if err != nil {
		return "", err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return "", fmt.Errorf("file changed while opening")
	}
	hasher := sha256.New()
	written, err := io.Copy(hasher, io.LimitReader(file, maximumArtifactBytes+1))
	if err != nil || written > maximumArtifactBytes {
		return "", fmt.Errorf("hash bounded file")
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func validRelativePath(value string) bool {
	return value != "" && !path.IsAbs(value) && !filepath.IsAbs(value) && path.Clean(value) == value &&
		!strings.Contains(value, `\`) && value != ".." && !strings.HasPrefix(value, "../")
}

func decodeCanonical(data []byte, value any) error {
	if len(data) == 0 || len(data) > maximumArtifactBytes {
		return fmt.Errorf("invalid V10 artifact size")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return fmt.Errorf("trailing V10 artifact JSON")
	}
	want, err := canonicalJSON(value)
	if err != nil || !bytes.Equal(data, want) {
		return fmt.Errorf("noncanonical V10 artifact JSON")
	}
	return nil
}

func canonicalJSON(value any) ([]byte, error) {
	data, err := json.MarshalIndent(value, "", "  ")
	return append(data, '\n'), err
}
func hashBytes(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }
func sealHash(value ImplementationSeal) (string, error) {
	value.Hash = ""
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return hashBytes(data), nil
}
func roundHash(value Round) (string, error) {
	value.Hash = ""
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return hashBytes(data), nil
}
func sortedUnique(values []string) bool {
	return slices.IsSorted(values) && !slices.Contains(values, "") && len(values) == len(slices.Compact(slices.Clone(values)))
}
