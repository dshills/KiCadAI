package corpuspublication

import (
	"bytes"
	"fmt"
	"sort"

	"kicadai/internal/obligationanchor"
	ots "kicadai/internal/opentopologysynthesis"
)

func deriveObligationsV8(manifestHash string, manifest ManifestV8, discovery map[string][]byte, heldOut []heldOutCaseV8) (DiscoveryObligationsV8, HeldOutObligationCommitmentV8, error) {
	discoveryObligations, heldOutAnchors := []ObligationV8{}, []string{}
	seen := map[string]bool{}
	for _, entry := range manifest.Entries {
		obligations, err := obligationsForEntryV8(manifestHash, entry, discovery[entry.StablePath], seen)
		if err != nil {
			return DiscoveryObligationsV8{}, HeldOutObligationCommitmentV8{}, err
		}
		discoveryObligations = append(discoveryObligations, obligations...)
	}
	for _, item := range heldOut {
		obligations, err := obligationsForEntryV8(manifestHash, item.Entry, item.Source, seen)
		if err != nil {
			return DiscoveryObligationsV8{}, HeldOutObligationCommitmentV8{}, err
		}
		for _, obligation := range obligations {
			heldOutAnchors = append(heldOutAnchors, obligation.Anchor)
		}
	}
	sort.Slice(discoveryObligations, func(i, j int) bool { return discoveryObligations[i].Anchor < discoveryObligations[j].Anchor })
	sort.Strings(heldOutAnchors)
	return DiscoveryObligationsV8{Schema: "kicadai.closed-loop-open-set-discovery-obligations.v8", Version: 8,
			CorpusManifestSHA256: manifestHash, Obligations: discoveryObligations},
		HeldOutObligationCommitmentV8{Schema: "kicadai.closed-loop-open-set-held-out-obligation-commitment.v8", Version: 8,
			CorpusManifestSHA256: manifestHash, ObligationCount: len(heldOutAnchors), AggregateSHA256: aggregateDigestsV8(heldOutAnchors)}, nil
}

func obligationsForEntryV8(manifestHash string, entry EntryV8, source []byte, seen map[string]bool) ([]ObligationV8, error) {
	if len(source) == 0 {
		return nil, fmt.Errorf("V8 obligation source missing")
	}
	requirement, issues := ots.DecodeStrict(bytes.NewReader(source))
	if len(issues) != 0 {
		return nil, fmt.Errorf("V8 obligation source invalid")
	}
	obligations := []ObligationV8{}
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		outputID := assertion.Observation.ID
		if assertion.Observation.Kind == "circuit" {
			outputID = obligationanchor.CircuitOutput
		}
		for _, operatingCase := range assertion.OperatingCases {
			input := obligationanchor.Input{CorpusManifestSHA256: manifestHash, Role: entry.Role, CaseID: entry.ID,
				OperatingCaseID: operatingCase, AssertionID: assertion.ID, ObservationKind: assertion.Observation.Kind,
				ObservationID: assertion.Observation.ID, OutputID: outputID}
			anchor, err := obligationanchor.Derive(input)
			if err != nil {
				return nil, err
			}
			if seen[anchor] {
				return nil, fmt.Errorf("V8 duplicate obligation anchor")
			}
			seen[anchor] = true
			obligations = append(obligations, ObligationV8{Anchor: anchor, Role: entry.Role, CaseID: entry.ID, OperatingCaseID: operatingCase,
				AssertionID: assertion.ID, ObservationKind: assertion.Observation.Kind, ObservationID: assertion.Observation.ID, OutputID: outputID})
		}
	}
	return obligations, nil
}
