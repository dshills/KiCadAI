package runtimebudget

import "testing"

func TestConfiguredCapacityIsBoundedByProcessors(t *testing.T) {
	for _, test := range []struct {
		value      string
		processors int
		want       int
	}{
		{processors: 4, want: 4},
		{value: "2", processors: 8, want: 2},
		{value: "16", processors: 4, want: 4},
		{value: "invalid", processors: 8, want: 1},
		{value: "0", processors: 8, want: 1},
		{processors: 0, want: 1},
	} {
		if got := configuredCapacity(test.value, test.processors); got != test.want {
			t.Fatalf("configuredCapacity(%q, %d)=%d, want %d", test.value, test.processors, got, test.want)
		}
	}
}

func TestValidateRejectsInvalidExplicitBudget(t *testing.T) {
	t.Setenv(environmentVariable, "many")
	if err := Validate(); err == nil {
		t.Fatal("invalid worker budget was accepted")
	}
}

func TestLimitHonorsWorkAndLocalCaps(t *testing.T) {
	if got := Limit(0, 8, 4); got != 0 {
		t.Fatalf("zero-work limit=%d", got)
	}
	if got := Limit(2, 8, 4); got != 2 {
		t.Fatalf("work-limited workers=%d, want 2", got)
	}
	if got := Limit(20, 8, 3); got != min(3, Capacity()) {
		t.Fatalf("local-cap workers=%d, want %d", got, min(3, Capacity()))
	}
}

func TestNestedLimitReservesInnerFanout(t *testing.T) {
	want := max(1, Capacity()/min(4, Capacity()))
	if got := NestedLimit(100, 100, 4); got != want {
		t.Fatalf("nested workers=%d, want %d", got, want)
	}
}
