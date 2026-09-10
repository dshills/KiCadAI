package electricaldiagnostics

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"

	"kicadai/internal/canonicaljsonstream"
	ot "kicadai/internal/opentopologysynthesis"
)

const MaximumTraceBytes = 16 << 20

type CertifiedCandidate struct {
	Fingerprint    string                        `json:"fingerprint"`
	PlanHash       string                        `json:"topology_plan_sha256"`
	Certificate    ot.TopologyInvariantReportV21 `json:"certificate"`
	Graph          ot.CandidateGraph             `json:"graph"`
	OperationCount int                           `json:"operation_count"`
	Evaluation     Evaluation                    `json:"evaluation"`
}

// Synthesis is an explicitly partial diagnostic projection: it retains the
// evaluation attached to each complete V21 structural certificate, not every
// attempted candidate waveform. Run/replay hashes bind the complete source.
type Synthesis struct {
	Schema             string `json:"schema"`
	RunHash            string `json:"run_sha256"`
	ReplayHash         string `json:"replay_sha256"`
	RawSerializedBytes int64  `json:"raw_serialized_bytes"`
	RequirementHash    string `json:"requirement_sha256"`
	// InventoryHash is inherited report provenance, not the inventory of every
	// successor evaluation. Each certificate binds its own exact inventory.
	InventoryHash       string               `json:"inventory_sha256"`
	Status              ot.Status            `json:"status"`
	StopReason          ot.StopReason        `json:"stop_reason"`
	Consumption         ot.Consumption       `json:"consumption"`
	CandidateCount      int                  `json:"candidate_count"`
	CertifiedCandidates []CertifiedCandidate `json:"certified_candidates"`
	Hash                string               `json:"hash"`
}

func ProjectSynthesis(run ot.SynthesisRun) (Synthesis, error) {
	unsigned := run
	unsigned.Hash = ""
	digest, _, err := streamedHash(unsigned)
	if err != nil || run.Hash == "" || digest != run.Hash {
		return Synthesis{}, fmt.Errorf("synthesis content hash differs")
	}
	if run.Schema != ot.SynthesisRunSchema || run.Version != ot.SynthesisRunVersion {
		return Synthesis{}, fmt.Errorf("unsupported synthesis schema")
	}
	result := Synthesis{
		Schema: "kicadai.post-topology-electrical-diagnostics.v1", RunHash: run.Hash,
		RequirementHash: run.Report.RequirementHash, InventoryHash: run.Report.PrimitiveInventoryHash,
		Status: run.Report.Status, StopReason: run.Report.StopReason, Consumption: run.Report.Consumption,
		CandidateCount: len(run.Candidates), CertifiedCandidates: []CertifiedCandidate{},
	}
	result.ReplayHash, result.RawSerializedBytes, err = streamedHash(run)
	if err != nil {
		return Synthesis{}, err
	}
	for _, candidate := range run.Candidates {
		repair := candidate.Repair
		if repair == nil || repair.TopologyCompletionV21 == nil || repair.TopologyCompletionV21.Selected == nil {
			continue
		}
		plan := repair.TopologyCompletionV21
		selected := plan.Selected
		if !selected.Invariant.Complete {
			continue
		}
		if plan.Status != "complete" || selected.Invariant.Contradictory || len(selected.Invariant.Obligations) != 0 {
			return Synthesis{}, fmt.Errorf("inconsistent structural certificate")
		}
		graphHash, err := ot.GraphHash(selected.Graph)
		if err != nil || graphHash != selected.GraphHash || graphHash != selected.Invariant.GraphHash || selected.Invariant.RequirementHash != result.RequirementHash {
			return Synthesis{}, fmt.Errorf("structural certificate binding differs: graph=%s selected=%s certificate=%s requirement=%s inherited=%s: %v", graphHash, selected.GraphHash, selected.Invariant.GraphHash, selected.Invariant.RequirementHash, result.RequirementHash, err)
		}
		certificate := selected.Invariant
		certificate.Hash = ""
		certificateHash, err := hash(certificate)
		if err != nil || certificateHash != selected.Invariant.Hash {
			return Synthesis{}, fmt.Errorf("structural certificate hash differs")
		}
		evaluationHash := repair.InitialEvaluationHash
		if len(selected.Operations) != 0 {
			evaluationHash = ""
			for _, attempt := range repair.Attempts {
				if attempt.GraphHash == graphHash {
					evaluationHash = attempt.Evaluation.Hash
				}
			}
		}
		var evaluation *ot.SimulationEvaluation
		for i := range candidate.Evaluations {
			if candidate.Evaluations[i].Hash == evaluationHash {
				evaluation = &candidate.Evaluations[i]
			}
		}
		if evaluation == nil || evaluation.GraphHash != graphHash || evaluation.RequirementHash != result.RequirementHash || evaluation.InventoryHash != selected.Invariant.InventoryHash {
			return Synthesis{}, fmt.Errorf("complete certificate lacks its exact retained evaluation")
		}
		projected, err := ProjectEvaluation(*evaluation)
		if err != nil {
			return Synthesis{}, err
		}
		result.CertifiedCandidates = append(result.CertifiedCandidates, CertifiedCandidate{
			Fingerprint: candidate.Fingerprint, PlanHash: plan.Hash, Certificate: selected.Invariant,
			Graph: selected.Graph, OperationCount: len(selected.Operations), Evaluation: projected,
		})
	}
	result.Hash, err = hash(result)
	if err != nil {
		return Synthesis{}, err
	}
	return result, nil
}

// Marshal refuses oversized diagnostic output rather than silently truncating
// evidence. This diagnostic limit never changes synthesis search limits.
func Marshal(trace Synthesis) ([]byte, error) {
	data, err := json.MarshalIndent(trace, "", "  ")
	if err != nil {
		return nil, err
	}
	if len(data)+1 > MaximumTraceBytes {
		return nil, fmt.Errorf("diagnostic trace exceeds %d bytes", MaximumTraceBytes)
	}
	return append(data, '\n'), nil
}

type countingWriter struct{ bytes int64 }

func (writer *countingWriter) Write(data []byte) (int, error) {
	writer.bytes += int64(len(data))
	return len(data), nil
}

// Use the identical stream encoding as the frozen replay spool writer, but
// measure/hash without creating a duplicate multi-GB diagnostic file.
func streamedHash(value any) (string, int64, error) {
	digest := sha256.New()
	count := &countingWriter{}
	if err := canonicaljsonstream.Encode(io.MultiWriter(digest, count), value); err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(digest.Sum(nil)), count.bytes, nil
}
