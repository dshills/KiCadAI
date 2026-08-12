package closedloopopensetcontract

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestVersionEightValidatorIsFrozenAndOutcomeNeutral(t *testing.T) {
	directory := v7ContractDirectory(t)
	var freeze struct {
		Schema                      string `json:"schema"`
		Version                     int    `json:"version"`
		ValidatorManifestSHA256     string `json:"validator_manifest_sha256"`
		HistoricalCommitmentsSHA256 string `json:"historical_commitments_sha256"`
		PacketSetSHA256             string `json:"packet_set_sha256"`
		HeldOutOpened               bool   `json:"held_out_opened"`
	}
	if err := json.Unmarshal(v7ReadFile(t, filepath.Join(directory, "V8_VALIDATOR_FREEZE.json")), &freeze); err != nil {
		t.Fatal(err)
	}
	if freeze.Schema != "kicadai.closed-loop-open-set-validator-freeze.v8" || freeze.Version != 8 || freeze.HeldOutOpened ||
		freeze.ValidatorManifestSHA256 != v7FileSHA256(t, filepath.Join(directory, "V8_VALIDATOR.sha256")) ||
		freeze.HistoricalCommitmentsSHA256 != v7FileSHA256(t, filepath.Join(directory, "V8_HISTORICAL_COMMITMENTS.json")) ||
		freeze.PacketSetSHA256 != v7FileSHA256(t, filepath.Join(directory, "v8-authoring-packet", "PACKET_SET.sha256")) {
		t.Fatalf("V8 validator freeze binding is invalid: %+v", freeze)
	}
	v8VerifyManifest(t, directory, "V8_VALIDATOR.sha256")
	for _, name := range []string{
		"../../cmd/kicadai-corpus-validate-v8/main.go",
		"../../internal/corpusfreezev8/validate.go",
		"../../internal/corpusfreezev8/policy.go",
	} {
		data := bytes.ToLower(v7ReadFile(t, filepath.Join(directory, filepath.FromSlash(name))))
		for _, forbidden := range [][]byte{
			[]byte("internal/closedloopsynthesis"), []byte("internal/capabilityfeedback"),
			[]byte("internal/capabilityrounds"), []byte("synthesize("), []byte("simulate("),
		} {
			if bytes.Contains(data, forbidden) {
				t.Fatalf("V8 validator source %s imports or names forbidden outcome path %q", name, forbidden)
			}
		}
	}
}

func TestVersionEightValidatorContractManifest(t *testing.T) {
	v8VerifyManifest(t, v7ContractDirectory(t), "V8_VALIDATOR_CONTRACT.sha256")
}

func v8VerifyManifest(t *testing.T, root, name string) {
	t.Helper()
	lines := strings.Split(strings.TrimSuffix(string(v7ReadFile(t, filepath.Join(root, name))), "\n"), "\n")
	if len(lines) == 0 || lines[0] == "" {
		t.Fatalf("V8 %s is empty", name)
	}
	previous := ""
	for index, line := range lines {
		if len(line) < 67 || line[64:66] != "  " {
			t.Fatalf("V8 %s entry %d is malformed", name, index)
		}
		digest, relative := line[:64], line[66:]
		decoded, err := hex.DecodeString(digest)
		if err != nil || len(decoded) != 32 || strings.ToLower(digest) != digest || relative <= previous {
			t.Fatalf("V8 %s entry %d is invalid or unordered", name, index)
		}
		if got := v7FileSHA256(t, filepath.Join(root, filepath.FromSlash(relative))); got != digest {
			t.Fatalf("V8 %s hash for %s = %s, want %s", name, relative, got, digest)
		}
		previous = relative
	}
}
