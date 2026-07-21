package behavioralintent

import (
	"strings"
	"testing"
)

func TestDecodeProposalStrictRejectsUnknownAndTrailingData(t *testing.T) {
	for _, input := range []string{
		`{"version":1,"requirement":null,"coverage":[],"uncertainties":[],"clarifications":[],"capability_gaps":[],"unknown":true}`,
		`{"version":1,"requirement":null,"coverage":[],"uncertainties":[],"clarifications":[],"capability_gaps":[]} {}`,
	} {
		if _, issues := DecodeProposalStrict(strings.NewReader(input)); len(issues) == 0 {
			t.Fatalf("input unexpectedly decoded: %s", input)
		}
	}
}
