package repair

import "testing"

func TestTransactionCrossStageHashRejectsUnmarshalableValue(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected unmarshalable transaction state to violate the hash invariant")
		}
	}()
	transactionCrossStageHash(make(chan struct{}))
}
