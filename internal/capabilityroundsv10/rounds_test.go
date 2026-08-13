package capabilityroundsv10

import (
	"errors"
	"reflect"
	"testing"

	"kicadai/internal/capabilityroundsv9"
)

func TestFrozenPolicyExactlyAdoptsV9Semantics(t *testing.T) {
	got := FrozenPolicy()
	want := capabilityroundsv9.FrozenPolicy()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("V10 policy drifted from its frozen predecessor: got=%+v want=%+v", got, want)
	}
	if got.ExpectedDiscoveryCases != 24 || got.MaximumRounds != 2 || got.MaximumRoundAtoms != 3 || got.MaximumRoundMembers != 9 {
		t.Fatalf("unexpected V10 bounds: %+v", got)
	}
}

func TestVersionTenErrorsDoNotExposePredecessorIdentity(t *testing.T) {
	_, err := Select(nil, nil, RoundState{}, Policy{})
	if !errors.Is(err, ErrInvalidInput) || errors.Is(err, capabilityroundsv9.ErrInvalidInput) {
		t.Fatalf("error boundary = %v", err)
	}
}
