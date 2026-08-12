package closedloopopensetcontract

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"kicadai/internal/capabilityevaluation"
	"kicadai/internal/capabilityfeedback"
	"kicadai/internal/opentopologysynthesis"
)

const (
	v3ImpactRegistryHash  = "64080fc37ce81747b6cf33b8919fb8e6a33a8c9182b0b2ce0174f190c11a9377"
	v3SynthesisPolicyHash = "4b067326445c90ac125ee5bf61ab7d57d96118806a83e02e7675ea2905038df4"
)

func TestVersionThreeContractIsFrozen(t *testing.T) {
	directory := v3ContractDirectory(t)
	manifest, err := os.Open(filepath.Join(directory, "V3_CONTRACT.sha256"))
	if err != nil {
		t.Fatal(err)
	}
	defer manifest.Close()

	seen := map[string]bool{}
	scanner := bufio.NewScanner(manifest)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2 || len(fields[0]) != sha256.Size*2 || seen[fields[1]] || filepath.Base(fields[1]) != fields[1] {
			t.Fatalf("invalid V3 contract entry %q", scanner.Text())
		}
		if got := v3FileSHA256(t, filepath.Join(directory, fields[1])); got != fields[0] {
			t.Fatalf("%s hash = %s, want frozen %s", fields[1], got, fields[0])
		}
		seen[fields[1]] = true
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"V3_SPEC_ADDENDUM.md", "V3_CORPUS_RULES.md", "V3_BASELINE_PROTOCOL.md",
		"V3_PLAN.md", "V3_IMPACT_REGISTRY.json", "V3_SYNTHESIS_POLICY.json", "V3_IMPLEMENTATION.sha256",
	} {
		if !seen[required] {
			t.Fatalf("V3 frozen contract omits %s", required)
		}
	}
}

func TestVersionThreeImplementationManifestIsFrozen(t *testing.T) {
	directory := v3ContractDirectory(t)
	seen := map[string]bool{}
	for _, name := range historicalManifestNames(t, filepath.Join(directory, "V3_IMPLEMENTATION.sha256")) {
		seen[name] = true
	}
	for _, required := range []string{
		"internal/opentopologysynthesis/realizability.go",
		"internal/capabilityfeedback/observe.go",
		"internal/capabilityfeedback/evaluate.go",
		"specs/behavioral-contract-feasibility-realizability/CONTRACT.sha256",
	} {
		if !seen[required] {
			t.Fatalf("V3 implementation manifest omits %s", required)
		}
	}
	if len(seen) != 4 {
		t.Fatalf("V3 implementation manifest entries = %d, want exactly 4", len(seen))
	}
}

func TestVersionThreePoliciesAreExactAndAcyclic(t *testing.T) {
	directory := v3ContractDirectory(t)
	var registry capabilityevaluation.ImpactRegistry
	v3DecodeStrictFile(t, filepath.Join(directory, "V3_IMPACT_REGISTRY.json"), &registry)
	report, err := capabilityfeedback.EvaluateRealizabilityAware(capabilityfeedback.RoleHeldOut, nil, registry)
	if err != nil {
		t.Fatalf("V3 impact registry: %v", err)
	}
	if report.PolicyVersion != capabilityfeedback.RealizabilityPolicyVersion ||
		report.ImpactRegistryHash != v3ImpactRegistryHash {
		t.Fatalf("V3 evaluator/registry = %q/%q", report.PolicyVersion, report.ImpactRegistryHash)
	}

	var policy opentopologysynthesis.Policy
	v3DecodeStrictFile(t, filepath.Join(directory, "V3_SYNTHESIS_POLICY.json"), &policy)
	policyHash, err := opentopologysynthesis.PolicyHash(policy)
	if err != nil || policyHash != v3SynthesisPolicyHash {
		t.Fatalf("V3 synthesis policy hash = %q err=%v", policyHash, err)
	}
}

func v3ContractDirectory(t *testing.T) string {
	t.Helper()
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate V3 contract test source")
	}
	return filepath.Dir(sourceFile)
}

func v3DecodeStrictFile(t *testing.T, path string, target any) {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		t.Fatal(err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		t.Fatalf("V3 contract JSON contains trailing data: %v", err)
	}
}

func v3FileSHA256(t *testing.T, path string) string {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(hash.Sum(nil))
}
