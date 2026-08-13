// Package capabilityexecutorv10 executes the frozen V10 public discovery
// baseline through the production open-topology synthesis and physical-
// promotion paths.
package capabilityexecutorv10

import (
	"context"
	"time"

	"kicadai/internal/capabilitybaselinev10"
	"kicadai/internal/capabilityfeedback"
	"kicadai/internal/corpuspublication"
	"kicadai/internal/libraryresolver"
	"kicadai/internal/opentopologysynthesis"
)

const cleanRootSchema = "kicadai.closed-loop-open-set-clean-root.v10"
const ParallelCaseLimit = 4

type CaseInput struct {
	Entry             corpuspublication.EntryV10
	RequirementSource []byte
	Obligations       []corpuspublication.ObligationV10
}

type Environment struct {
	Inventory                      opentopologysynthesis.PrimitiveInventory
	Simulation                     opentopologysynthesis.SimulationEnvironment
	Policy                         opentopologysynthesis.Policy
	LibraryIndex                   *libraryresolver.LibraryIndex
	KiCadCLI                       string
	KiCadCLISHA256                 string
	PromotionEnvironmentSHA256     string
	EvaluatorManifestSHA256        string
	PromotionTimeout               time.Duration
	KeepPhysicalPromotionArtifacts bool
}

type Request struct {
	CorpusManifestSHA256 string
	OutputRoot           string
	Cases                []CaseInput
	Environment          Environment
}

type PublicCorpus struct {
	ManifestSHA256 string
	Cases          []CaseInput
}

type decoderFunc func([]byte) (opentopologysynthesis.Requirement, error)
type synthesisFunc func(context.Context, opentopologysynthesis.Requirement, opentopologysynthesis.PrimitiveInventory, opentopologysynthesis.SimulationEnvironment, opentopologysynthesis.Policy) opentopologysynthesis.SynthesisRun
type promotionFunc func(context.Context, opentopologysynthesis.SynthesisRun, opentopologysynthesis.SimulationEnvironment, opentopologysynthesis.PhysicalPromotionOptions) opentopologysynthesis.PhysicalPromotionResult
type observerFunc func(capabilityfeedback.CaseMeta, opentopologysynthesis.Requirement, opentopologysynthesis.SynthesisRun, *opentopologysynthesis.PhysicalPromotionResult) (capabilityfeedback.CaseEvidence, error)

type Executor struct {
	decode     decoderFunc
	synthesize synthesisFunc
	promote    promotionFunc
	observe    observerFunc
}

func New() Executor {
	return Executor{
		decode:     decodeRequirement,
		synthesize: opentopologysynthesis.Synthesize,
		promote:    opentopologysynthesis.PromoteSynthesisRun,
		observe:    capabilityfeedback.ObserveRealizabilityAware,
	}
}

type cleanRootMarker struct {
	Schema                  string `json:"schema"`
	Version                 int    `json:"version"`
	CaseID                  string `json:"case_id"`
	Replay                  int    `json:"replay"`
	CorpusManifestSHA256    string `json:"corpus_manifest_sha256"`
	RequirementSHA256       string `json:"requirement_sha256"`
	EnvironmentSHA256       string `json:"environment_sha256"`
	EvaluatorManifestSHA256 string `json:"evaluator_manifest_sha256"`
}

type caseResult struct {
	evidence capabilitybaselinev10.CaseEvidence
}
