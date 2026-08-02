package repairloop

import "testing"

func TestCrossStageHashRejectsUnmarshalableValue(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected unmarshalable cross-stage state to violate the hash invariant")
		}
	}()
	crossStageHash(make(chan struct{}))
}
