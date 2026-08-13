package capabilityfeedback

import (
	"path/filepath"
	"testing"

	"kicadai/internal/blindbaseline"
)

const (
	closedLoopV8HeldOutBaselineRoot = "testdata/closed_loop_open_set_v8_held_out_baseline"

	closedLoopV8SelectionFreezeCommit     = "dbc6a70f5bc4f7d868473707a09bf1f19cb4ae21"
	closedLoopV8SelectionHash             = "13299d97ba0ca4eace70cd2fb2cb47e935de584cac8b94ac8a2b4e37d163cb13"
	closedLoopV8HeldOutManifestCommitment = "7cd26715adb3665fe074c99c9eac800b15c039c1624c7512755d2dbc349bc992"
	closedLoopV8HeldOutManifestFileSHA256 = "6e41095a1fe29a0ee33a1466bd1c3fdef58799adb0eedb54fad53dd12ee38ab0"
	closedLoopV8HeldOutCiphertextSHA256   = "0d33cc879f0ff9144e25b2385d9d6abb0eea413b0d33a1dae350ab7f340fd8ed"
	closedLoopV8HeldOutCorpusSHA256       = "548d8f38cdbc6186a737d9c1cfdea73906a25f6b1948b9a367e00897f7c66f1c"
	closedLoopV8HeldOutGapPolicySHA256    = "ba73b2db190f48c70b31bc77b7689240df122f73b41e8b63624e540635139aa8"
	closedLoopV8HeldOutResourceSHA256     = "4b067326445c90ac125ee5bf61ab7d57d96118806a83e02e7675ea2905038df4"
	closedLoopV8HeldOutCaseCount          = 18
)

func TestClosedLoopV8HeldOutBaselineSealIsFrozen(t *testing.T) {
	manifest, err := blindbaseline.VerifyV8(closedLoopV8HeldOutBaselineRoot)
	if err != nil {
		t.Fatalf("verify V8 held-out baseline seal: %v", err)
	}
	manifestSource := mustCorpusRead(t, filepath.Join(closedLoopV8HeldOutBaselineRoot, blindbaseline.ManifestFile))
	if got := corpusHash(manifestSource); got != closedLoopV8HeldOutManifestFileSHA256 {
		t.Fatalf("V8 held-out baseline manifest file hash = %s, want %s", got, closedLoopV8HeldOutManifestFileSHA256)
	}
	if got := manifest.Hash; got != closedLoopV8HeldOutManifestCommitment {
		t.Fatalf("V8 held-out baseline manifest commitment = %s, want %s", got, closedLoopV8HeldOutManifestCommitment)
	}
	if got := manifest.CiphertextSHA256; got != closedLoopV8HeldOutCiphertextSHA256 {
		t.Fatalf("V8 held-out baseline ciphertext commitment = %s, want %s", got, closedLoopV8HeldOutCiphertextSHA256)
	}
	if got := manifest.CaseCount; got != closedLoopV8HeldOutCaseCount {
		t.Fatalf("V8 held-out baseline case count = %d, want %d", got, closedLoopV8HeldOutCaseCount)
	}
	if got := manifest.Binding.SelectionFreezeCommit; got != closedLoopV8SelectionFreezeCommit {
		t.Fatalf("V8 held-out baseline selection freeze commit = %s, want %s", got, closedLoopV8SelectionFreezeCommit)
	}
	if got := manifest.Binding.PublisherParentCommit; got != closedLoopV8SelectionFreezeCommit {
		t.Fatalf("V8 held-out baseline publisher parent commit = %s, want %s", got, closedLoopV8SelectionFreezeCommit)
	}
	if got := manifest.Binding.SelectionSHA256; got != closedLoopV8SelectionHash {
		t.Fatalf("V8 held-out baseline selection commitment = %s, want %s", got, closedLoopV8SelectionHash)
	}
	if got := manifest.Binding.CorpusManifestSHA256; got != closedLoopV8HeldOutCorpusSHA256 {
		t.Fatalf("V8 held-out baseline corpus commitment = %s, want %s", got, closedLoopV8HeldOutCorpusSHA256)
	}
	// These pairs intentionally bind one artifact under two semantic roles.
	// V4_GAP_TRANSITION_POLICY.json is the frozen gap registry and policy;
	// V4_SYNTHESIS_POLICY.json contains the complete resource ceilings.
	if got := manifest.Binding.GapRegistrySHA256; got != closedLoopV8HeldOutGapPolicySHA256 {
		t.Fatalf("V8 held-out baseline gap registry commitment = %s, want %s", got, closedLoopV8HeldOutGapPolicySHA256)
	}
	if got := manifest.Binding.GapPolicySHA256; got != closedLoopV8HeldOutGapPolicySHA256 {
		t.Fatalf("V8 held-out baseline gap policy commitment = %s, want %s", got, closedLoopV8HeldOutGapPolicySHA256)
	}
	if got := manifest.Binding.EnvironmentPolicySHA256; got != closedLoopV8HeldOutResourceSHA256 {
		t.Fatalf("V8 held-out baseline environment commitment = %s, want %s", got, closedLoopV8HeldOutResourceSHA256)
	}
	if got := manifest.Binding.ResourceCeilingsSHA256; got != closedLoopV8HeldOutResourceSHA256 {
		t.Fatalf("V8 held-out baseline resource-ceilings commitment = %s, want %s", got, closedLoopV8HeldOutResourceSHA256)
	}
}
