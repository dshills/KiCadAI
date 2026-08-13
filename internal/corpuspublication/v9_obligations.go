package corpuspublication

import (
	"bytes"
	"fmt"
	"sort"

	"kicadai/internal/obligationanchor"
	ots "kicadai/internal/opentopologysynthesis"
)

func deriveObligationsV9(manifestHash string, manifest ManifestV9, discovery map[string][]byte, heldOut []heldOutCaseV9) (DiscoveryObligationsV9, HeldOutObligationCommitmentV9, error) {
	discoveryObligations, heldOutAnchors := []ObligationV9{}, []string{}
	seen := map[string]bool{}
	for _, entry := range manifest.Entries {
		obligations, err := obligationsForEntryV9(manifestHash, entry, discovery[entry.StablePath], seen)
		if err != nil {
			return DiscoveryObligationsV9{}, HeldOutObligationCommitmentV9{}, err
		}
		discoveryObligations = append(discoveryObligations, obligations...)
	}
	for _, item := range heldOut {
		obligations, err := obligationsForEntryV9(manifestHash, item.Entry, item.Source, seen)
		if err != nil {
			return DiscoveryObligationsV9{}, HeldOutObligationCommitmentV9{}, err
		}
		for _, obligation := range obligations {
			heldOutAnchors = append(heldOutAnchors, obligation.Anchor)
		}
	}
	sort.Slice(discoveryObligations, func(i, j int) bool { return discoveryObligations[i].Anchor < discoveryObligations[j].Anchor })
	sort.Strings(heldOutAnchors)
	return DiscoveryObligationsV9{Schema: "kicadai.closed-loop-open-set-discovery-obligations.v9", Version: 9,
			CorpusManifestSHA256: manifestHash, Obligations: discoveryObligations},
		HeldOutObligationCommitmentV9{Schema: "kicadai.closed-loop-open-set-held-out-obligation-commitment.v9", Version: 9,
			CorpusManifestSHA256: manifestHash, ObligationCount: len(heldOutAnchors), AggregateSHA256: aggregateDigestsV9(heldOutAnchors)}, nil
}

func obligationsForEntryV9(manifestHash string, entry EntryV9, source []byte, seen map[string]bool) ([]ObligationV9, error) {
	if len(source) == 0 {
		return nil, fmt.Errorf("V9 obligation source missing")
	}
	requirement, issues := ots.DecodeStrict(bytes.NewReader(source))
	if len(issues) != 0 {
		return nil, fmt.Errorf("V9 obligation source invalid")
	}
	obligations := []ObligationV9{}
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
				return nil, fmt.Errorf("V9 duplicate obligation anchor")
			}
			seen[anchor] = true
			obligations = append(obligations, ObligationV9{Anchor: anchor, Role: entry.Role, CaseID: entry.ID, OperatingCaseID: operatingCase,
				AssertionID: assertion.ID, ObservationKind: assertion.Observation.Kind, ObservationID: assertion.Observation.ID, OutputID: outputID})
		}
	}
	return obligations, nil
}
