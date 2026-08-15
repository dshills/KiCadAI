package capabilityexecutorv10

import (
	"kicadai/internal/capabilityfeedback"
	"kicadai/internal/opentopologysynthesis"
)

// NewV15 binds the V15 evaluator to bounded value-trial and failed-graph
// retention without changing any historically frozen constructor.
func NewV15() Executor {
	return Executor{
		decode:     decodeRequirement,
		synthesize: opentopologysynthesis.SynthesizeV15,
		promote:    opentopologysynthesis.PromoteSynthesisRun,
		observe:    capabilityfeedback.ObserveRealizabilityAware,
	}
}
