package corpusfreezev6

import (
	"fmt"
	"sort"
	"strings"

	"kicadai/internal/corpusfreeze"
)

func Validate(
	assignments map[string][]byte,
	bundles map[string]corpusfreeze.Bundle,
	binding corpusfreeze.Binding,
	historical HistoricalCommitments,
	policy corpusfreeze.Policy,
) (corpusfreeze.Report, error) {
	// Validate is an exported boundary and callers may construct HistoricalCommitments
	// directly instead of using LoadHistoricalCommitments. Recheck the added V6 map.
	if err := validateNormalizedCommitments(historical.NormalizedSemanticSHA256); err != nil {
		return corpusfreeze.Report{}, err
	}
	report, err := corpusfreeze.Validate(assignments, bundles, binding, historical.Base, policy)
	if err != nil {
		return corpusfreeze.Report{}, err
	}
	if err := rejectHistoricalNormalized(report, historical.NormalizedSemanticSHA256); err != nil {
		return corpusfreeze.Report{}, err
	}
	return report, nil
}

func rejectHistoricalNormalized(report corpusfreeze.Report, historical map[string]string) error {
	for _, entry := range report.Entries {
		if prior := historical[entry.NormalizedSemanticSHA256]; prior != "" {
			return fmt.Errorf("%s duplicates historical normalized semantic requirement %s", entry.ID, prior)
		}
	}
	return nil
}

func validateNormalizedCommitments(commitments map[string]string) error {
	digests := make([]string, 0, len(commitments))
	for digest := range commitments {
		digests = append(digests, digest)
	}
	sort.Strings(digests)
	for _, digest := range digests {
		if !validSHA256(digest) || strings.TrimSpace(commitments[digest]) == "" {
			return fmt.Errorf("V6 historical normalized semantic commitment is invalid")
		}
	}
	return nil
}
