package opentopologysynthesis

import "testing"

func TestCausalCrossStageHashRejectsUnmarshalableValue(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected unmarshalable cross-stage state to violate the hash invariant")
		}
	}()
	causalCrossStageHash(make(chan struct{}))
}
