package closedloopopensetcontract

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"kicadai/internal/capabilitybundles"
)

const (
	v6StartingCommit        = "9b6f8be61006f7de179099feb0b38080ff18ecb3"
	v6SpecHash              = "eef26a916f633d897c5b966f809c5a8b8f06feae755ea0365305840ac4adf294"
	v6PlanHash              = "12d7112c564407ffda15944d131b5996b3b48e98a4be1eeb1f44b6e57d42b488"
	v6CorpusRulesHash       = "c5d37799982835c1c1bafc48c27e617eda8039fac7b046b9e7f1faaeba60720d"
	v6BaselineProtocolHash  = "67d4ce8d2c6c5abc07cd9f26646cdbab24261406bf57f5a718fa641f7fa505a7"
	v6SelectionPolicyHash   = "1d82387013cccff736b33b497194586254c25a395db97d25d923abf1b658e2f3"
	v6ModelHash             = "ab5ad9d83d11797a7e3f43b5293aac55aacf5454b130d316384b9cd981f37466"
	v6PolicyCodeHash        = "d5b544815fb42c063d725aa23821cf8e29c026bf9872f7c9c44f6bcc46d9aa0d"
	v6BuildHash             = "79633291912137f8c402082dce751c120d7ae46c714325dea6c04a5efe60f044"
	v6BuildTestHash         = "629dcf2879d017b0ebc6003f7bf1ab774edc012239da913a638147fff9399d1e"
	v6PolicyTestHash        = "b7d3a48956aadc33e9a7daf10ac65a93796704e6402b54033dab57f46f430975"
	v6V5RetirementAuditHash = "39f86504e07940732ed596fb3ffa849792465c7c8099aa023570d4ac22e2793c"
	v6V5RetirementSumHash   = "f2d0a99e4417f79fbcd55649113940534b903e565a53f8146a439fd5b742706a"
)

func TestVersionSixContractInputsAreFrozen(t *testing.T) {
	directory := v6ContractDirectory(t)
	repositoryRoot := filepath.Clean(filepath.Join(directory, "..", ".."))
	files := map[string]string{
		filepath.Join(directory, "V6_SPEC_ADDENDUM.md"):                            v6SpecHash,
		filepath.Join(directory, "V6_PLAN.md"):                                     v6PlanHash,
		filepath.Join(directory, "V6_CORPUS_RULES.md"):                             v6CorpusRulesHash,
		filepath.Join(directory, "V6_BASELINE_PROTOCOL.md"):                        v6BaselineProtocolHash,
		filepath.Join(directory, "V6_SELECTION_POLICY.json"):                       v6SelectionPolicyHash,
		filepath.Join(repositoryRoot, "internal/capabilitybundles/model.go"):       v6ModelHash,
		filepath.Join(repositoryRoot, "internal/capabilitybundles/policy.go"):      v6PolicyCodeHash,
		filepath.Join(repositoryRoot, "internal/capabilitybundles/build.go"):       v6BuildHash,
		filepath.Join(repositoryRoot, "internal/capabilitybundles/build_test.go"):  v6BuildTestHash,
		filepath.Join(repositoryRoot, "internal/capabilitybundles/policy_test.go"): v6PolicyTestHash,
		filepath.Join(directory, "V5_PUBLIC_ADMISSION_RETIREMENT_AUDIT.md"):        v6V5RetirementAuditHash,
		filepath.Join(directory, "V5_PUBLIC_ADMISSION_RETIREMENT_AUDIT.sha256"):    v6V5RetirementSumHash,
	}
	for path, want := range files {
		if got := v6FileSHA256(t, path); got != want {
			t.Fatalf("%s SHA-256 = %s, want %s", filepath.Base(path), got, want)
		}
	}
	spec := string(v6ReadFile(t, filepath.Join(directory, "V6_SPEC_ADDENDUM.md")))
	if !strings.Contains(spec, v6StartingCommit) {
		t.Fatal("V6 specification does not bind the starting commit")
	}
	audit := string(v6ReadFile(t, filepath.Join(directory, "V5_PUBLIC_ADMISSION_RETIREMENT_AUDIT.md")))
	if !strings.Contains(audit, "Held-out source opened: no") ||
		!strings.Contains(audit, "Held-out baseline opened: no") ||
		!strings.Contains(audit, "Final updater: permanently retired") {
		t.Fatal("V6 starting state does not bind the fail-closed V5 retirement")
	}
}

func TestVersionSixSelectionPolicyIsStrictAndCausal(t *testing.T) {
	policy := v6Policy(t)
	if policy.Schema != "kicadai.closed-loop-open-set-selection-policy.v6" || policy.Version != 6 {
		t.Fatal("V6 selection policy schema or version is invalid")
	}

	rawIncidence := []capabilitybundles.Case{
		v6Case("power_a", "power", 5, v6Gap("electrothermal"), v6Gap("dc"), v6Gap("output")),
		v6Case("power_b", "power", 4, v6Gap("electrothermal"), v6Gap("dc")),
		v6Case("analog_c", "analog", 5, v6Gap("electrothermal"), v6Gap("bandwidth"), v6Gap("noise"), v6Gap("transimpedance")),
	}
	result, err := capabilitybundles.Build(rawIncidence, policy)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := capabilitybundles.SelectRankOne(result); !errors.Is(err, capabilitybundles.ErrNoEligibleBundle) {
		t.Fatalf("V6 selected raw shared-gap incidence without causal unlock: %v", err)
	}

	causal := []capabilitybundles.Case{
		v6Case("power", "power", 4, v6Gap("solver"), v6Gap("evidence")),
		v6Case("analog", "analog", 3, v6Gap("solver"), v6Gap("evidence")),
		v6Case("sensor", "sensor", 2, v6Gap("solver"), v6Gap("unique")),
	}
	result, err = capabilitybundles.Build(causal, policy)
	if err != nil {
		t.Fatal(err)
	}
	selected, err := capabilitybundles.SelectRankOne(result)
	if err != nil {
		t.Fatal(err)
	}
	if len(selected.Atoms) != 2 || !slices.Equal(selected.UnlockedCases, []string{"analog", "power"}) {
		t.Fatalf("V6 causal rank one = %#v", selected)
	}
}

func TestVersionSixContractChecksumManifest(t *testing.T) {
	directory := v6ContractDirectory(t)
	manifest := filepath.Join(directory, "V6_CONTRACT.sha256")
	file, err := os.Open(manifest)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	wantPaths := []string{
		"V6_SPEC_ADDENDUM.md",
		"V6_PLAN.md",
		"V6_CORPUS_RULES.md",
		"V6_BASELINE_PROTOCOL.md",
		"V6_SELECTION_POLICY.json",
		"../../internal/capabilitybundles/model.go",
		"../../internal/capabilitybundles/policy.go",
		"../../internal/capabilitybundles/build.go",
		"../../internal/capabilitybundles/build_test.go",
		"../../internal/capabilitybundles/policy_test.go",
		"v6_contract_test.go",
		"V5_PUBLIC_ADMISSION_RETIREMENT_AUDIT.md",
		"V5_PUBLIC_ADMISSION_RETIREMENT_AUDIT.sha256",
	}
	actualPaths := make([]string, 0, len(wantPaths))
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 2 || len(fields[0]) != 64 {
			t.Fatalf("invalid V6 contract checksum line %q", scanner.Text())
		}
		path := filepath.Clean(filepath.Join(directory, filepath.FromSlash(fields[1])))
		if got := v6FileSHA256(t, path); got != fields[0] {
			t.Fatalf("V6 contract checksum for %s = %s, want %s", fields[1], got, fields[0])
		}
		actualPaths = append(actualPaths, fields[1])
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(actualPaths, wantPaths) {
		t.Fatalf("V6 contract paths = %q, want %q", actualPaths, wantPaths)
	}
}

func v6Policy(t *testing.T) capabilitybundles.Policy {
	t.Helper()
	file, err := os.Open(filepath.Join(v6ContractDirectory(t), "V6_SELECTION_POLICY.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	policy, err := capabilitybundles.DecodePolicy(file)
	if err != nil {
		t.Fatal(err)
	}
	return policy
}

func v6Case(id, domain string, safety int64, gaps ...capabilitybundles.Gap) capabilitybundles.Case {
	return capabilitybundles.Case{Role: "discovery", ID: id, ReportingDomain: domain, SafetyWeight: safety, Outcome: "unsupported", Gaps: gaps}
}

func v6Gap(capability string) capabilitybundles.Gap {
	return capabilitybundles.Gap{Stage: "simulation", Scope: "simulation", Capability: capability, Code: "SIMULATION_INVALID", RequiredEvidence: []string{"simulation"}}
}

func v6ContractDirectory(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve V6 contract directory")
	}
	return filepath.Dir(file)
}

func v6ReadFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func v6FileSHA256(t *testing.T, path string) string {
	t.Helper()
	sum := sha256.Sum256(v6ReadFile(t, path))
	return hex.EncodeToString(sum[:])
}
