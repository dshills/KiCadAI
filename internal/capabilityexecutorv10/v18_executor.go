package capabilityexecutorv10

import (
	"kicadai/internal/capabilityfeedback"
	"kicadai/internal/opentopologysynthesis"
)

// NewV18 binds only the explicit V18 constructor. Historical constructors and
// runners remain byte-identical and retain their frozen synthesis functions.
func NewV18() Executor {
	return Executor{
		decode:     decodeRequirement,
		synthesize: opentopologysynthesis.SynthesizeV18,
		promote:    opentopologysynthesis.PromoteSynthesisRun,
		observe:    capabilityfeedback.ObserveRealizabilityAware,
	}
}
