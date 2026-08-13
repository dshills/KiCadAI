package capabilityselectionv10

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"

	"kicadai/internal/capabilitybaselinepublicationv10"
	"kicadai/internal/capabilitybaselinev10"
	"kicadai/internal/capabilityroundsv10"
)

var (
	ErrInvalidPlanSet = errors.New("invalid V10 effect-plan set")
	digestPattern     = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

const maximumInputBytes = 32 << 20

func DecodePlanSet(data []byte) (PlanSet, string, error) {
	if len(data) == 0 || len(data) > maximumInputBytes {
		return PlanSet{}, "", fmt.Errorf("%w: input size", ErrInvalidPlanSet)
	}
	var value PlanSet
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return PlanSet{}, "", fmt.Errorf("%w: decode: %v", ErrInvalidPlanSet, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return PlanSet{}, "", fmt.Errorf("%w: trailing JSON", ErrInvalidPlanSet)
	}
	canonical, err := canonicalJSON(value)
	if err != nil || !bytes.Equal(data, canonical) {
		return PlanSet{}, "", fmt.Errorf("%w: noncanonical JSON", ErrInvalidPlanSet)
	}
	return value, hashBytes(data), nil
}

func Build(
	baseline capabilitybaselinepublicationv10.Verification,
	planSet PlanSet,
	planSetSHA256 string,
	repositoryRoot string,
) (Ranking, error) {
	if err := capabilitybaselinev10.Validate(baseline.Report); err != nil {
		return Ranking{}, fmt.Errorf("validate V10 public baseline: %w", err)
	}
	if !digestPattern.MatchString(baseline.ManifestSHA256) ||
		baseline.ManifestSHA256 != planSet.BaselineManifestSHA256 ||
		baseline.Report.Hash != planSet.BaselineReportSHA256 ||
		!digestPattern.MatchString(planSetSHA256) {
		return Ranking{}, fmt.Errorf("%w: baseline or plan-set binding", ErrInvalidPlanSet)
	}
	plans, err := validatePlans(planSet, repositoryRoot)
	if err != nil {
		return Ranking{}, err
	}
	active := make([]string, 0)
	cases := make([]capabilityroundsv10.Case, len(baseline.Report.Cases))
	for index, record := range baseline.Report.Cases {
		cases[index] = record.Case
		if record.Case.Outcome == "unsupported" || record.Case.Outcome == "exhausted" {
			active = append(active, record.Case.ID)
		}
	}
	slices.Sort(active)
	state := capabilityroundsv10.RoundState{Generation: 0, ActiveCohortIDs: active}
	selection, err := capabilityroundsv10.Select(cases, plans, state, capabilityroundsv10.FrozenPolicy())
	if err != nil {
		return Ranking{}, fmt.Errorf("rank V10 generic capability plans: %w", err)
	}
	result := Ranking{
		Schema: RankingSchema, Version: Version, Generation: 0,
		BaselineManifestSHA256:     baseline.ManifestSHA256,
		BaselineReportSHA256:       baseline.Report.Hash,
		PlanSetSHA256:              planSetSHA256,
		EffectExposureEngineSHA256: EffectExposureEngineManifestSHA256,
		State:                      state, Selection: selection,
	}
	result.Hash, err = rankingHash(result)
	if err != nil {
		return Ranking{}, err
	}
	return result, nil
}

func Validate(
	value Ranking,
	baseline capabilitybaselinepublicationv10.Verification,
	planSet PlanSet,
	planSetSHA256 string,
	repositoryRoot string,
) error {
	rebuilt, err := Build(baseline, planSet, planSetSHA256, repositoryRoot)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(value, rebuilt) {
		return fmt.Errorf("V10 ranking does not reproduce")
	}
	return nil
}

func MarshalPlanSet(value PlanSet) ([]byte, error) {
	return canonicalJSON(value)
}

func MarshalRanking(value Ranking) ([]byte, error) {
	if !digestPattern.MatchString(value.Hash) {
		return nil, fmt.Errorf("invalid V10 ranking hash")
	}
	want, err := rankingHash(value)
	if err != nil || want != value.Hash {
		return nil, fmt.Errorf("invalid V10 ranking commitment")
	}
	return canonicalJSON(value)
}

func ReadRegularFile(filename string) ([]byte, error) {
	info, err := os.Lstat(filename)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > maximumInputBytes {
		return nil, fmt.Errorf("V10 selection input is not a bounded regular file")
	}
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || opened.Size() > maximumInputBytes || !os.SameFile(info, opened) {
		return nil, fmt.Errorf("V10 selection input changed while opening")
	}
	data, err := io.ReadAll(io.LimitReader(file, maximumInputBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read bounded V10 selection input: %w", err)
	}
	if len(data) > maximumInputBytes {
		return nil, fmt.Errorf("V10 selection input exceeds its size bound")
	}
	return data, nil
}

func WriteAtomicNoReplace(filename string, data []byte) error {
	if _, err := os.Lstat(filename); err == nil {
		return fmt.Errorf("V10 ranking output already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	parent := filepath.Dir(filename)
	info, err := os.Lstat(parent)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("V10 ranking output parent is not a real directory")
	}
	temp, err := os.CreateTemp(parent, ".v10-ranking-*")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if err := temp.Chmod(0o600); err != nil {
		temp.Close()
		return err
	}
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Link(tempName, filename); err != nil {
		return fmt.Errorf("install V10 ranking without replacement: %w", err)
	}
	directory, err := os.Open(parent)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

func validatePlans(value PlanSet, repositoryRoot string) ([]capabilityroundsv10.EffectPlan, error) {
	if value.Schema != PlanSetSchema || value.Version != Version || value.Generation != 0 ||
		!digestPattern.MatchString(value.BaselineManifestSHA256) || !digestPattern.MatchString(value.BaselineReportSHA256) ||
		value.EffectExposureEngineSHA256 != EffectExposureEngineManifestSHA256 || len(value.Plans) == 0 || value.Plans == nil {
		return nil, fmt.Errorf("%w: envelope", ErrInvalidPlanSet)
	}
	root, err := filepath.Abs(repositoryRoot)
	if err != nil {
		return nil, err
	}
	result := make([]capabilityroundsv10.EffectPlan, len(value.Plans))
	priorHash := ""
	for index, current := range value.Plans {
		planHash, err := validatePlan(current, root)
		if err != nil {
			return nil, fmt.Errorf("%w: plan %d: %v", ErrInvalidPlanSet, index, err)
		}
		if priorHash != "" && priorHash >= planHash {
			return nil, fmt.Errorf("%w: plan order or duplicate", ErrInvalidPlanSet)
		}
		priorHash = planHash
		result[index] = capabilityroundsv10.EffectPlan{
			DirectAtomKeys: current.DirectAtomKeys, DirectMemberKeys: current.DirectMemberKeys,
			ClosureAtoms: current.ClosureAtoms, ClosureMembers: current.ClosureMembers,
			PlannedMemberKeys: current.PlannedMemberKeys, RequiredEvidence: current.RequiredEvidence,
			Executable: current.Executable, MechanicallyProven: current.MechanicallyProven,
			UnboundedDynamicLookup: current.UnboundedDynamicLookup, UnmappedConsumers: current.UnmappedConsumers,
			PlanSHA256: planHash,
		}
	}
	return result, nil
}

func validatePlan(value Plan, repositoryRoot string) (string, error) {
	policy := capabilityroundsv10.FrozenPolicy()
	if !value.Executable || !value.MechanicallyProven || value.UnboundedDynamicLookup || len(value.UnmappedConsumers) != 0 ||
		!sortedUnique(value.DirectAtomKeys) || !sortedUnique(value.DirectMemberKeys) ||
		!sortedUnique(value.PlannedMemberKeys) || !slices.Equal(value.RequiredEvidence, policy.MechanicalEvidence) ||
		value.ClosureAtoms == nil || value.ClosureMembers == nil || value.UnmappedConsumers == nil {
		return "", fmt.Errorf("execution or mechanical-evidence claim")
	}
	if len(value.StaticEvidence.ProductionFiles) == 0 || len(value.StaticEvidence.VerificationFiles) == 0 ||
		len(value.StaticEvidence.ReverseCallGraph) == 0 || len(value.StaticEvidence.FocusedNonCorpusRuntimeConsumers) == 0 {
		return "", fmt.Errorf("incomplete static evidence")
	}
	if value.StaticEvidence.RegistryReferences == nil || value.StaticEvidence.ConfigurationLoaderReferences == nil ||
		value.StaticEvidence.CatalogModelReferences == nil || value.StaticEvidence.DataReferences == nil {
		return "", fmt.Errorf("noncanonical null static-evidence collection")
	}
	seenPaths := map[string]bool{}
	for _, group := range [][]FileEvidence{value.StaticEvidence.ProductionFiles, value.StaticEvidence.VerificationFiles} {
		prior := ""
		for _, file := range group {
			if file.Path == "" || path.IsAbs(file.Path) || filepath.IsAbs(file.Path) || path.Clean(file.Path) != file.Path ||
				strings.Contains(file.Path, `\`) || file.Path == ".." || strings.HasPrefix(file.Path, "../") ||
				prior >= file.Path && prior != "" || seenPaths[file.Path] || !digestPattern.MatchString(file.SHA256) {
				return "", fmt.Errorf("invalid static-evidence file")
			}
			prior, seenPaths[file.Path] = file.Path, true
			fullPath := filepath.Join(repositoryRoot, filepath.FromSlash(file.Path))
			currentHash, err := regularFileSHA256(fullPath)
			if err != nil || currentHash != file.SHA256 {
				return "", fmt.Errorf("static-evidence file %q drifted", file.Path)
			}
		}
	}
	for _, values := range [][]string{
		value.StaticEvidence.ReverseCallGraph,
		value.StaticEvidence.RegistryReferences,
		value.StaticEvidence.ConfigurationLoaderReferences,
		value.StaticEvidence.CatalogModelReferences,
		value.StaticEvidence.DataReferences,
		value.StaticEvidence.FocusedNonCorpusRuntimeConsumers,
	} {
		if !sortedUniqueAllowEmpty(values) {
			return "", fmt.Errorf("noncanonical static-evidence references")
		}
	}
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return hashBytes(data), nil
}

func canonicalJSON(value any) ([]byte, error) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func rankingHash(value Ranking) (string, error) {
	value.Hash = ""
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return hashBytes(data), nil
}

func hashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func regularFileSHA256(filename string) (string, error) {
	info, err := os.Lstat(filename)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() || info.Size() > maximumInputBytes {
		return "", fmt.Errorf("static-evidence path is not a bounded regular file")
	}
	file, err := os.Open(filename)
	if err != nil {
		return "", err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || opened.Size() > maximumInputBytes || !os.SameFile(info, opened) {
		return "", fmt.Errorf("static-evidence file changed while opening")
	}
	hasher := sha256.New()
	written, err := io.Copy(hasher, io.LimitReader(file, maximumInputBytes+1))
	if err != nil {
		return "", err
	}
	if written > maximumInputBytes {
		return "", fmt.Errorf("static-evidence file exceeds its size bound")
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func sortedUnique(values []string) bool {
	return len(values) > 0 && sortedUniqueAllowEmpty(values)
}

func sortedUniqueAllowEmpty(values []string) bool {
	return slices.IsSorted(values) && !slices.Contains(values, "") && len(values) == len(slices.Compact(slices.Clone(values)))
}
