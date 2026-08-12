package closedloopopensetcontract

import (
	"bytes"
	"encoding/hex"
	"path/filepath"
	"strings"
	"testing"
)

const v7ValidatorManifestHash = "cdb36868a54e22403531cdcb9631b3b8de1b4046383fda9a9d5668e18a3817f5"

func TestVersionSevenValidatorIsFrozenAndOutcomeNeutral(t *testing.T) {
	directory := v7ContractDirectory(t)
	paths := []string{
		"../../cmd/kicadai-corpus-validate-v7/main.go",
		"../../cmd/kicadai-corpus-validate-v7/main_test.go",
		"../../internal/corpusfreeze/decode.go",
		"../../internal/corpusfreeze/diversity.go",
		"../../internal/corpusfreeze/history.go",
		"../../internal/corpusfreeze/history_test.go",
		"../../internal/corpusfreeze/load.go",
		"../../internal/corpusfreeze/load_test.go",
		"../../internal/corpusfreeze/model.go",
		"../../internal/corpusfreeze/normalize.go",
		"../../internal/corpusfreeze/output.go",
		"../../internal/corpusfreeze/output_test.go",
		"../../internal/corpusfreeze/policy.go",
		"../../internal/corpusfreeze/secure_open_unix.go",
		"../../internal/corpusfreeze/secure_open_windows.go",
		"../../internal/corpusfreeze/validate.go",
		"../../internal/corpusfreeze/validate_test.go",
		"../../internal/corpusfreezev6/history.go",
		"../../internal/corpusfreezev6/history_test.go",
		"../../internal/corpusfreezev6/policy.go",
		"../../internal/corpusfreezev6/policy_test.go",
		"../../internal/corpusfreezev6/validate.go",
		"../../internal/corpusfreezev6/validate_test.go",
		"../../internal/corpusfreezev7/history.go",
		"../../internal/corpusfreezev7/history_test.go",
		"../../internal/corpusfreezev7/policy.go",
		"../../internal/corpusfreezev7/policy_test.go",
		"../../internal/corpusfreezev7/validate.go",
		"../../internal/corpusfreezev7/validate_test.go",
		"V7_HISTORICAL_COMMITMENTS.json",
		"V7_AUTHOR_PACKET.sha256",
	}
	if got := v7FileSHA256(t, filepath.Join(directory, "V7_VALIDATOR.sha256")); got != v7ValidatorManifestHash {
		t.Fatalf("V7 validator manifest SHA-256 = %s, want %s", got, v7ValidatorManifestHash)
	}
	v7VerifyValidatorManifest(t, directory, "V7_VALIDATOR.sha256", paths)
	for _, name := range []string{
		"../../cmd/kicadai-corpus-validate-v7/main.go",
		"../../internal/corpusfreezev7/policy.go",
		"../../internal/corpusfreezev7/history.go",
		"../../internal/corpusfreezev7/validate.go",
	} {
		data := bytes.ToLower(v7ReadFile(t, filepath.Join(directory, filepath.FromSlash(name))))
		for _, forbidden := range [][]byte{
			[]byte("internal/closedloopsynthesis"), []byte("internal/capabilityfeedback"),
			[]byte("internal/capabilityrounds"), []byte("internal/opentopologysynthesis/synth"),
		} {
			if bytes.Contains(data, forbidden) {
				t.Fatalf("V7 validator source %s imports or names forbidden outcome path %q", name, forbidden)
			}
		}
	}
}

func v7VerifyValidatorManifest(t *testing.T, root, name string, wantPaths []string) {
	t.Helper()
	lines := strings.Split(strings.TrimSuffix(string(v7ReadFile(t, filepath.Join(root, name))), "\n"), "\n")
	if len(lines) != len(wantPaths) {
		t.Fatalf("V7 %s entry count = %d, want %d", name, len(lines), len(wantPaths))
	}
	for index, line := range lines {
		if len(line) < 67 || line[64:66] != "  " || line[66:] != wantPaths[index] {
			t.Fatalf("V7 %s entry %d = %q, want path %q", name, index, line, wantPaths[index])
		}
		digest := line[:64]
		decoded, err := hex.DecodeString(digest)
		if err != nil || len(decoded) != 32 || strings.ToLower(digest) != digest {
			t.Fatalf("V7 %s digest %q is invalid", name, digest)
		}
		if got := v7FileSHA256(t, filepath.Join(root, filepath.FromSlash(wantPaths[index]))); got != digest {
			t.Fatalf("V7 %s hash for %s = %s, want %s", name, wantPaths[index], got, digest)
		}
	}
}

func TestVersionSevenValidatorRootManifest(t *testing.T) {
	v7VerifyPacketChecksumManifest(t, v7ContractDirectory(t), "V7_VALIDATOR_CONTRACT.sha256", []string{
		"V7_VALIDATOR.sha256",
		"v7_validator_contract_test.go",
	})
}
