package closedloopopensetcontract

import (
	"bytes"
	"path/filepath"
	"testing"
)

func TestVersionEightEvaluatorIsOutcomeNeutralAndFrozen(t *testing.T) {
	directory := v8ContractDirectory(t)
	repository := filepath.Clean(filepath.Join(directory, "..", ".."))
	v8VerifyManifest(t, repository, filepath.Join("specs", "closed-loop-open-set-capability-expansion", "V8_EVALUATOR.sha256"))
	for _, name := range []string{
		"../../internal/capabilityroundsv8/model.go",
		"../../internal/capabilityroundsv8/identity.go",
		"../../internal/capabilityroundsv8/select.go",
		"../../internal/capabilityroundsv8/advance.go",
	} {
		data := bytes.ToLower(v8ReadFile(t, filepath.Join(directory, filepath.FromSlash(name))))
		for _, forbidden := range [][]byte{
			[]byte("capabilityfeedback/testdata"), []byte("closed_loop_open_set_v8_corpus"),
			[]byte("corpuspublication"), []byte("closedloopsynthesis"), []byte("opentopologysynthesis"),
		} {
			if bytes.Contains(data, forbidden) {
				t.Fatalf("V8 evaluator %s names forbidden corpus/outcome path %q", name, forbidden)
			}
		}
	}
}
