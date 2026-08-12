package closedloopopensetcontract

import (
	"bytes"
	"path/filepath"
	"testing"
)

func TestVersionEightPublisherIsOutcomeNeutralAndFrozen(t *testing.T) {
	directory := v7ContractDirectory(t)
	v8VerifyManifest(t, filepath.Join(directory, "..", ".."), filepath.Join("specs", "closed-loop-open-set-capability-expansion", "V8_PUBLISHER.sha256"))
	for _, name := range []string{
		"../../cmd/kicadai-corpus-publish-v8/main.go",
		"../../internal/corpuspublication/v8.go",
		"../../internal/corpuspublication/v8_seal.go",
		"../../internal/corpuspublication/v8_obligations.go",
		"../../internal/obligationanchor/anchor.go",
	} {
		data := bytes.ToLower(v7ReadFile(t, filepath.Join(directory, filepath.FromSlash(name))))
		for _, forbidden := range [][]byte{
			[]byte("internal/closedloopsynthesis"), []byte("internal/capabilityfeedback"),
			[]byte("internal/capabilityrounds"), []byte("internal/capabilitybundles"),
			[]byte("synthesize("), []byte("simulate("), []byte("classify("), []byte("rank("),
		} {
			if bytes.Contains(data, forbidden) {
				t.Fatalf("V8 publisher %s names forbidden outcome path %q", name, forbidden)
			}
		}
	}
}
