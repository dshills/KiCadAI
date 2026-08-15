package capabilityexecutorv10

import (
	"kicadai/internal/capabilityfeedback"
	"kicadai/internal/opentopologysynthesis"
)

// NewV14 binds the V14 evaluator to lazy value-trial graph materialization
// without changing the historically frozen production constructor.
func NewV14() Executor {
	return Executor{
		decode:     decodeRequirement,
		synthesize: opentopologysynthesis.SynthesizeV14,
		promote:    opentopologysynthesis.PromoteSynthesisRun,
		observe:    capabilityfeedback.ObserveRealizabilityAware,
	}
}
