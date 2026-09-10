package opentopologysynthesis

import (
	"context"
	"fmt"

	"kicadai/internal/simulationadmission"
)

// ElectricalCertificateV23 combines the unchanged structural, model-admission,
// all-corner, and electrical-bound checks with the explicit V23 execution policy.
// PrerequisiteProof does not claim historical solver execution or KiCad readiness.
type ElectricalCertificateV23 struct {
	Schema            string                   `json:"schema"`
	PrerequisiteProof ElectricalCertificateV22 `json:"prerequisite_proof"`
	EvaluationHash    string                   `json:"evaluation_sha256"`
	Hash              string                   `json:"hash"`
}

func certifyElectricalPathV23(ctx context.Context, requirement Requirement, before, after CandidateGraph, inventory PrimitiveInventory, environment SimulationEnvironment, admission simulationadmission.PreparedEnvironment, changes []GraphChange, evaluation ElectricalEvaluationV23) (ElectricalCertificateV23, error) {
	if err := verifyElectricalEvaluationPreparedV23(ctx, evaluation, requirement, after, inventory, environment, admission); err != nil {
		return ElectricalCertificateV23{}, err
	}
	// No diagnostic can be discarded merely because a report claims to pass.
	for _, attempt := range evaluation.SolverAttempts {
		if len(attempt.Diagnostics) != 0 {
			return ElectricalCertificateV23{}, fmt.Errorf("V23 certificate has a numerical refusal")
		}
	}
	proof, err := certifyElectricalPathV22(ctx, requirement, before, after, inventory, environment, admission, changes, evaluation.Evaluation)
	if err != nil {
		return ElectricalCertificateV23{}, err
	}
	result := ElectricalCertificateV23{Schema: "kicadai.electrical-rebinding-certificate.v23", PrerequisiteProof: proof, EvaluationHash: evaluation.Hash}
	result.Hash = causalCrossStageHash(result)
	if result.Hash == "" {
		return ElectricalCertificateV23{}, fmt.Errorf("V23 electrical certificate hashing failed")
	}
	return result, nil
}

func verifyElectricalCertificateV23(ctx context.Context, certificate ElectricalCertificateV23, requirement Requirement, before, after CandidateGraph, inventory PrimitiveInventory, environment SimulationEnvironment, admission simulationadmission.PreparedEnvironment, evaluation ElectricalEvaluationV23) error {
	expected, err := certifyElectricalPathV23(ctx, requirement, before, after, inventory, environment, admission, certificate.PrerequisiteProof.Changes, evaluation)
	if err != nil {
		return err
	}
	if certificate.Hash == "" || causalCrossStageHash(certificate) != causalCrossStageHash(expected) {
		return fmt.Errorf("V23 electrical certificate content differs")
	}
	return nil
}
