package components

import "testing"

func TestFableH15CanonicalEngineeringValuesMatchEquivalentSIPrefixes(t *testing.T) {
	for _, test := range []struct {
		left  string
		right string
	}{
		{left: "0.1u", right: "100n"},
		{left: "0.47u", right: "470n"},
		{left: "1.5n", right: "1500p"},
	} {
		record := ComponentRecord{Values: []ValueConstraint{{Kind: "capacitance", Typ: test.left, Unit: "F"}}}
		if !recordHasValue(record, "capacitance", test.right) {
			t.Fatalf("equivalent SI-prefix values %q and %q did not match", test.left, test.right)
		}
	}
}

func TestFableH15CanonicalEngineeringValuesRejectUnknownSuffix(t *testing.T) {
	record := ComponentRecord{Values: []ValueConstraint{{Kind: "capacitance", Typ: "100n", Unit: "F"}}}
	if recordHasValue(record, "capacitance", "100nX") {
		t.Fatal("unknown engineering suffix matched a catalog value")
	}
}
