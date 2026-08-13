// Package capabilityroundsv10 exposes the V10 public selection and causal-
// round boundary. V10 deliberately adopts the frozen V9 algorithms while
// keeping its public type boundary version-separated.
package capabilityroundsv10

import (
	"errors"
	"fmt"

	"kicadai/internal/capabilityroundsv9"
)

var (
	ErrInvalidInput      = errors.New("invalid V10 capability-round input")
	ErrCandidateOverflow = errors.New("V10 candidate closure exceeds its frozen ceiling")
	ErrNoEligibleBundle  = errors.New("no eligible V10 capability bundle")
	ErrRoundGate         = errors.New("V10 public round gate failed")
)

type Policy = capabilityroundsv9.Policy
type Leaf = capabilityroundsv9.Leaf
type Gap = capabilityroundsv9.Gap
type Case = capabilityroundsv9.Case
type Atom = capabilityroundsv9.Atom
type Member = capabilityroundsv9.Member
type EffectPlan = capabilityroundsv9.EffectPlan
type Candidate = capabilityroundsv9.Candidate
type CaseExposure = capabilityroundsv9.CaseExposure
type CaseCommitment = capabilityroundsv9.CaseCommitment
type RoundState = capabilityroundsv9.RoundState
type Selection = capabilityroundsv9.Selection
type RoundEvidence = capabilityroundsv9.RoundEvidence
type Successor = capabilityroundsv9.Successor
type EvaluationStatus = capabilityroundsv9.EvaluationStatus
type Evaluation = capabilityroundsv9.Evaluation

const (
	EvaluationContinue       = capabilityroundsv9.EvaluationContinue
	EvaluationPublicAdmitted = capabilityroundsv9.EvaluationPublicAdmitted
)

func FrozenPolicy() Policy { return capabilityroundsv9.FrozenPolicy() }

func PathHash(gap Gap) (string, error) {
	value, err := capabilityroundsv9.PathHash(gap)
	return value, translateError(err)
}

func Select(cases []Case, plans []EffectPlan, state RoundState, policy Policy) (Selection, error) {
	value, err := capabilityroundsv9.Select(cases, plans, state, policy)
	return value, translateError(err)
}

func EvaluateRound(previous []Case, next []Case, selected Candidate, state RoundState, evidence RoundEvidence, policy Policy) (Evaluation, error) {
	value, err := capabilityroundsv9.EvaluateRound(previous, next, selected, state, evidence, policy)
	return value, translateError(err)
}

func translateError(err error) error {
	if err == nil {
		return nil
	}
	for _, value := range []struct {
		predecessor error
		current     error
	}{
		{capabilityroundsv9.ErrInvalidInput, ErrInvalidInput},
		{capabilityroundsv9.ErrCandidateOverflow, ErrCandidateOverflow},
		{capabilityroundsv9.ErrNoEligibleBundle, ErrNoEligibleBundle},
		{capabilityroundsv9.ErrRoundGate, ErrRoundGate},
	} {
		if errors.Is(err, value.predecessor) {
			return fmt.Errorf("%w: predecessor rejected input", value.current)
		}
	}
	return err
}
