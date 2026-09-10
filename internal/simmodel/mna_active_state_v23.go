package simmodel

// activeStateRecoveryV23 describes work outside the historical active-set path.
// Bounds are worst-case solve counts, not fabricated measurements of iterations.
type activeStateRecoveryV23 struct {
	attempted               bool
	accepted                bool
	maximumAdditionalSolves int
}

// solveBoundedOpAmpDCFromStateV23 preserves the historical result unless an
// inherited op-amp saturation state causes bounded active-set nonconvergence.
// It tries one alternative seed, never a new circuit, model, source, or bound.
func solveBoundedOpAmpDCFromStateV23(plan Plan, analysis Analysis, system mnaSystem, solution []complex128, initial map[string]float64) (mnaSystem, []complex128, map[string]float64, activeStateRecoveryV23, []Diagnostic) {
	priorSystem, priorSolution, priorStates, priorDiagnostics := solveBoundedOpAmpDCFromState(plan, analysis, system, solution, initial)
	evidence := activeStateRecoveryV23{}
	if !activeStateNonconvergenceV23(priorDiagnostics) {
		return priorSystem, priorSolution, priorStates, evidence, priorDiagnostics
	}
	released := cloneOpAmpClamps(initial)
	releasedCount, activeCount, opAmpCount := 0, 0, 0
	for _, device := range plan.Devices {
		switch device.PrimitiveModel {
		case PrimitiveOpAmpV1:
			activeCount++
			opAmpCount++
			if _, exists := released[device.Component]; exists {
				delete(released, device.Component)
				releasedCount++
			}
		case PrimitiveComparatorOpenCollectorV1, PrimitiveCurrentSenseAmplifierV1:
			activeCount++
		}
	}
	// Do not convert other nonconvergence into a generic cold-start retry. A
	// comparator's inherited state and every unrelated clamp remain untouched.
	if releasedCount == 0 || len(compileNonlinearDevices(plan)) != 0 {
		return priorSystem, priorSolution, priorStates, evidence, priorDiagnostics
	}
	evidence.attempted = true
	// One stability solve per op-amp, one explicit rebuild/solve, up to one
	// rebuild for retained non-op-amp clamps, then the historical iteration bound.
	evidence.maximumAdditionalSolves = opAmpCount + 2 + min(activeCount*6+2, maxOpAmpActiveSetIterations)
	if diagnostics := validateOpAmpStability(plan, analysis); len(diagnostics) != 0 {
		return priorSystem, priorSolution, priorStates, evidence, priorDiagnostics
	}
	rebuilt, diagnostics := buildMNASystemWithOpAmpClamps(plan, analysis, 0, released)
	if len(diagnostics) != 0 {
		return priorSystem, priorSolution, priorStates, evidence, priorDiagnostics
	}
	resolved, diagnostic := solveMNA(rebuilt)
	if diagnostic != nil {
		return priorSystem, priorSolution, priorStates, evidence, priorDiagnostics
	}
	rebuilt, resolved, states, diagnostics := solveBoundedOpAmpDCFromState(plan, analysis, rebuilt, resolved, released)
	if len(diagnostics) != 0 || len(validateResolvedOperatingLimits(plan, rebuilt, resolved, false)) != 0 {
		return priorSystem, priorSolution, priorStates, evidence, priorDiagnostics
	}
	consistent, _, diagnostic := resolvedActiveDeviceStates(plan, rebuilt, resolved)
	if diagnostic != nil || !sameOpAmpClamps(states, consistent) {
		return priorSystem, priorSolution, priorStates, evidence, priorDiagnostics
	}
	evidence.accepted = true
	return rebuilt, resolved, states, evidence, nil
}

// Historical simmodel diagnostics do not carry a dedicated code for these two
// active-set stops. Match their exact generic contract, not a loose substring or
// a fixture/device identity. Any other refusal is returned unchanged.
func activeStateNonconvergenceV23(diagnostics []Diagnostic) bool {
	if len(diagnostics) != 1 || diagnostics[0].Path != "devices" {
		return false
	}
	switch diagnostics[0].Message {
	case "bounded op-amp operating-point states did not converge",
		"bounded op-amp operating-point iteration exceeded its deterministic limit":
		return true
	default:
		return false
	}
}
