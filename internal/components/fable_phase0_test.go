package components

import "testing"

func TestFableH15ReproductionEquivalentSIPrefixesFailExactMatch(t *testing.T) {
	record := ComponentRecord{Values: []ValueConstraint{{Kind: "capacitance", Typ: "0.1u", Unit: "F"}}}
	if recordHasValue(record, "capacitance", "100n") {
		t.Fatal("equivalent SI-prefix values matched; replace this reproduction with the canonical-value invariant")
	}
}
