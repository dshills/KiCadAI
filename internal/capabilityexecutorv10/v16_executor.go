package capabilityexecutorv10

import (
	"kicadai/internal/capabilityfeedback"
	"kicadai/internal/opentopologysynthesis"
)

// NewV16 binds the V16 evaluator to bounded value-trial and failed-graph
// retention without changing any historically frozen constructor.
func NewV16() Executor {
	return Executor{
		decode:     decodeRequirement,
		synthesize: opentopologysynthesis.SynthesizeV16,
		promote:    opentopologysynthesis.PromoteSynthesisRun,
		observe:    capabilityfeedback.ObserveRealizabilityAware,
	}
}
