package simmodel

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"math/cmplx"
	"slices"
)

type controlInfluenceEdge struct {
	to        string
	component string
}

type controlPath struct {
	nets       []string
	components []string
}

// DiscoverControlLoops derives canonical negative-feedback loops from the
// resolved primitive graph. It deliberately accepts no loop labels, cuts, or
// equations from the caller.
func DiscoverControlLoops(plan Plan) ([]ControlLoop, []Diagnostic) {
	adjacency := controlInfluenceGraph(plan)
	blocked := controlLoopBlockedNets(plan)
	devices := append([]ResolvedDevice(nil), plan.Devices...)
	slices.SortStableFunc(devices, func(left, right ResolvedDevice) int {
		return compareStrings(left.Component, right.Component)
	})

	outputOwners := map[string]string{}
	var loops []ControlLoop
	var diagnostics []Diagnostic
	for _, device := range devices {
		if device.PrimitiveModel != PrimitiveOpAmpV1 {
			continue
		}
		terminals := terminalMap(device)
		output := terminals["OUT"]
		if previous, exists := outputOwners[output]; exists {
			diagnostics = append(diagnostics, Diagnostic{
				Path:       "control_loops.outputs." + output,
				Message:    fmt.Sprintf("feedback-loop output is shared by active devices %s and %s", previous, device.Component),
				Suggestion: "select a topology with one observable active output per derived loop",
			})
			continue
		}
		outputOwners[output] = device.Component

		negative, hasNegative := shortestControlPath(adjacency, output, terminals["IN_MINUS"], device.Component, blocked)
		positive, hasPositive := shortestControlPath(adjacency, output, terminals["IN_PLUS"], device.Component, blocked)
		switch {
		case hasNegative && hasPositive && terminals["IN_MINUS"] == terminals["IN_PLUS"]:
			diagnostics = append(diagnostics, Diagnostic{
				Path:       "control_loops." + device.Component,
				Message:    "derived active stage has simultaneous positive- and negative-feedback return paths",
				Suggestion: "select an unambiguous negative-feedback topology",
			})
		case hasPositive && !hasNegative:
			diagnostics = append(diagnostics, Diagnostic{
				Path:       "control_loops." + device.Component,
				Message:    "derived active stage closes only a positive-feedback return path",
				Suggestion: "select a negative-feedback topology or omit stability promotion",
			})
		case !hasNegative:
			diagnostics = append(diagnostics, Diagnostic{
				Path:       "control_loops." + device.Component,
				Message:    "derived active stage has no observable feedback return path",
				Suggestion: "connect a catalog-modeled negative-feedback path or omit stability promotion",
			})
		default:
			members := append([]string{device.Component}, negative.components...)
			if hasPositive {
				members = append(members, positive.components...)
			}
			slices.Sort(members)
			members = slices.Compact(members)
			loops = append(loops, ControlLoop{
				ID:                "loop:" + device.Component + ":in_minus",
				ActiveComponent:   device.Component,
				PrimitiveModel:    device.PrimitiveModel,
				InjectionTerminal: "OUT",
				InjectionNet:      output,
				ObservationNet:    output,
				FeedbackTerminal:  "IN_MINUS",
				FeedbackNet:       terminals["IN_MINUS"],
				Polarity:          "negative",
				Members:           members,
				NetPath:           negative.nets,
				DCPreserved:       true,
			})
		}
	}
	for _, device := range devices {
		if device.PrimitiveModel != PrimitiveSynchronousBuckRegulatorV1 {
			continue
		}
		terminals := terminalMap(device)
		output, outputOK := synchronousBuckOutputNet(plan, device)
		negative, hasNegative := shortestControlPath(adjacency, terminals["SW"], terminals["FB"], device.Component, blocked)
		if !outputOK {
			diagnostics = append(diagnostics, Diagnostic{
				Path:       "control_loops." + device.Component,
				Message:    "current-mode buck loop has no unique catalog-modeled output inductor",
				Suggestion: "connect exactly one reviewed inductor from SW to the regulated output",
			})
			continue
		}
		if !hasNegative {
			diagnostics = append(diagnostics, Diagnostic{
				Path:       "control_loops." + device.Component,
				Message:    "current-mode buck stage has no observable feedback return path",
				Suggestion: "connect the regulated output to FB through a catalog-modeled passive network",
			})
			continue
		}
		if previous, exists := outputOwners[output]; exists {
			diagnostics = append(diagnostics, Diagnostic{
				Path:       "control_loops.outputs." + output,
				Message:    fmt.Sprintf("feedback-loop output is shared by active devices %s and %s", previous, device.Component),
				Suggestion: "select a topology with one observable active output per derived loop",
			})
			continue
		}
		outputOwners[output] = device.Component
		members := append([]string{device.Component}, negative.components...)
		slices.Sort(members)
		members = slices.Compact(members)
		loops = append(loops, ControlLoop{
			ID:                "loop:" + device.Component + ":fb",
			ActiveComponent:   device.Component,
			PrimitiveModel:    device.PrimitiveModel,
			InjectionTerminal: "SW",
			InjectionNet:      terminals["SW"],
			ObservationNet:    output,
			FeedbackTerminal:  "FB",
			FeedbackNet:       terminals["FB"],
			Polarity:          "negative",
			Members:           members,
			NetPath:           negative.nets,
			DCPreserved:       true,
		})
	}
	if len(loops) == 0 && len(diagnostics) == 0 {
		return discoverBJTLocalControlLoops(plan, adjacency, blocked)
	}
	slices.SortStableFunc(loops, func(left, right ControlLoop) int {
		return compareStrings(left.ID, right.ID)
	})
	slices.SortStableFunc(diagnostics, func(left, right Diagnostic) int {
		if compared := compareStrings(left.Path, right.Path); compared != 0 {
			return compared
		}
		return compareStrings(left.Message, right.Message)
	})
	return loops, diagnostics
}

func discoverBJTLocalControlLoops(plan Plan, adjacency map[string][]controlInfluenceEdge, blocked map[string]bool) ([]ControlLoop, []Diagnostic) {
	var loops []ControlLoop
	for _, device := range plan.Devices {
		if device.PrimitiveModel != PrimitiveBJTNPNV1 && device.PrimitiveModel != PrimitiveBJTPNPV1 {
			continue
		}
		parameters := deviceParameterMap(device)
		terminals := terminalMap(device)
		if parameters["transition_frequency_hz"] <= 0 || terminals["EMITTER"] == "" || terminals["EMITTER"] == plan.GroundNode {
			continue
		}
		path, found := shortestControlPath(adjacency, terminals["EMITTER"], plan.GroundNode, device.Component, blocked)
		if !found || len(path.components) == 0 {
			continue
		}
		members := append([]string{device.Component}, path.components...)
		slices.Sort(members)
		members = slices.Compact(members)
		loops = append(loops, ControlLoop{
			ID:                "loop:" + device.Component + ":emitter",
			ActiveComponent:   device.Component,
			PrimitiveModel:    device.PrimitiveModel,
			InjectionTerminal: "EMITTER",
			InjectionNet:      terminals["EMITTER"],
			ObservationNet:    terminals["COLLECTOR"],
			FeedbackTerminal:  "EMITTER",
			FeedbackNet:       terminals["EMITTER"],
			Polarity:          "negative",
			Members:           members,
			NetPath:           path.nets,
			DCPreserved:       true,
		})
	}
	slices.SortStableFunc(loops, func(left, right ControlLoop) int {
		return compareStrings(left.ID, right.ID)
	})
	if len(loops) == 0 {
		return nil, []Diagnostic{{
			Path:       "control_loops",
			Message:    "stability analysis found no observable catalog-modeled feedback loop",
			Suggestion: "include a reviewed active stage and a derived negative-feedback return path",
		}}
	}
	return loops, nil
}

func controlInfluenceGraph(plan Plan) map[string][]controlInfluenceEdge {
	adjacency := map[string][]controlInfluenceEdge{}
	add := func(from, to, component string) {
		if from == "" || to == "" || from == to {
			return
		}
		adjacency[from] = append(adjacency[from], controlInfluenceEdge{to: to, component: component})
	}
	for _, device := range plan.Devices {
		terminals := terminalMap(device)
		switch device.PrimitiveModel {
		case PrimitiveResistorV1, PrimitiveFuseClosedStateV1, PrimitiveFuseI2TClearingV1, PrimitiveCapacitorV1,
			PrimitiveCapacitorTransientV1, PrimitiveInductorTransientV1,
			PrimitiveRelayClosedV1, PrimitiveRelayNormallyOpenV1,
			PrimitiveBidirectionalTVSV1, PrimitiveUnidirectionalZenerV1,
			PrimitiveDiodeShockleyV1:
			var nets []string
			for _, terminal := range device.Terminals {
				nets = append(nets, terminal.Net)
			}
			slices.Sort(nets)
			nets = slices.Compact(nets)
			for left := range nets {
				for right := left + 1; right < len(nets); right++ {
					add(nets[left], nets[right], device.Component)
					add(nets[right], nets[left], device.Component)
				}
			}
		case PrimitiveOpAmpV1:
			add(terminals["IN_PLUS"], terminals["OUT"], device.Component)
			add(terminals["IN_MINUS"], terminals["OUT"], device.Component)
		case PrimitiveSynchronousBuckRegulatorV1:
			add(terminals["FB"], terminals["SW"], device.Component)
		case PrimitiveCurrentSenseAmplifierV1:
			add(terminals["IN_PLUS"], terminals["OUT"], device.Component)
			add(terminals["IN_MINUS"], terminals["OUT"], device.Component)
		case PrimitiveBJTNPNV1, PrimitiveBJTPNPV1:
			if terminals["BASE"] != "" && terminals["BASE"] == terminals["COLLECTOR"] {
				// A diode-connected transistor is a two-terminal incremental
				// junction. Its nonlinear DC direction remains enforced by the
				// device equations, while loop discovery must allow a bias
				// midpoint perturbation to propagate through either terminal.
				add(terminals["BASE"], terminals["EMITTER"], device.Component)
				add(terminals["EMITTER"], terminals["BASE"], device.Component)
				continue
			}
			add(terminals["BASE"], terminals["COLLECTOR"], device.Component)
			add(terminals["BASE"], terminals["EMITTER"], device.Component)
		case PrimitiveNMOSSwitchV1, PrimitivePMOSSwitchV1:
			add(terminals["GATE"], terminals["DRAIN"], device.Component)
			add(terminals["GATE"], terminals["SOURCE"], device.Component)
		}
	}
	for net := range adjacency {
		slices.SortStableFunc(adjacency[net], func(left, right controlInfluenceEdge) int {
			if compared := compareStrings(left.to, right.to); compared != 0 {
				return compared
			}
			return compareStrings(left.component, right.component)
		})
		adjacency[net] = slices.Compact(adjacency[net])
	}
	return adjacency
}

func controlLoopBlockedNets(plan Plan) map[string]bool {
	blocked := map[string]bool{plan.GroundNode: true}
	for _, device := range plan.Devices {
		switch device.PrimitiveModel {
		case PrimitiveVoltageSourceV1, PrimitiveConnectorVoltageSourceV1:
			for _, terminal := range device.Terminals {
				blocked[terminal.Net] = true
			}
		}
	}
	return blocked
}

func shortestControlPath(adjacency map[string][]controlInfluenceEdge, start, target, excludedComponent string, blocked map[string]bool) (controlPath, bool) {
	if start == "" || target == "" {
		return controlPath{}, false
	}
	if start == target {
		return controlPath{nets: []string{start}}, true
	}
	type state struct {
		net        string
		nets       []string
		components []string
	}
	queue := []state{{net: start, nets: []string{start}}}
	visited := map[string]bool{start: true}
	for len(queue) != 0 {
		current := queue[0]
		queue = queue[1:]
		for _, edge := range adjacency[current.net] {
			if edge.component == excludedComponent || visited[edge.to] || (blocked[edge.to] && edge.to != target) {
				continue
			}
			next := state{
				net:        edge.to,
				nets:       append(append([]string(nil), current.nets...), edge.to),
				components: append(append([]string(nil), current.components...), edge.component),
			}
			if edge.to == target {
				return controlPath{nets: next.nets, components: next.components}, true
			}
			visited[edge.to] = true
			queue = append(queue, next)
		}
	}
	return controlPath{}, false
}

func populateControlLoopMetrics(result *AnalysisResult) *Diagnostic {
	for loopIndex := range result.ControlLoops {
		loop := &result.ControlLoops[loopIndex]
		assertion := Assertion{AnalysisID: result.ID, Node: loop.ObservationNet}
		frequencies, magnitudes, phases, diagnostic := loopSeries(*result, assertion)
		if diagnostic != nil {
			return diagnostic
		}
		crossover, phaseMargin, found := returnRatioCrossover(frequencies, magnitudes, phases)
		if !found {
			return advancedAssertionDiagnostic(assertion, fmt.Sprintf("stability sweep %.12g..%.12g Hz does not bracket the unity loop-gain crossing", frequencies[0], frequencies[len(frequencies)-1]))
		}
		gainMargin, diagnostic := stabilityMargin(*result, Assertion{AnalysisID: result.ID, Node: loop.ObservationNet, Quantity: QuantityGainMarginDB})
		if diagnostic != nil {
			return diagnostic
		}
		peaking := -math.Inf(1)
		for index := range magnitudes {
			ratio := complex(magnitudes[index]*math.Cos(phases[index]*math.Pi/180), magnitudes[index]*math.Sin(phases[index]*math.Pi/180))
			value := 20 * math.Log10(cmplx.Abs(1/(1+ratio)))
			if value > peaking {
				peaking = value
			}
		}
		samples, err := json.Marshal(struct {
			Frequencies []float64 `json:"frequencies_hz"`
			Magnitudes  []float64 `json:"magnitudes"`
			Phases      []float64 `json:"phases_deg"`
		}{Frequencies: frequencies, Magnitudes: magnitudes, Phases: phases})
		if err != nil {
			return &Diagnostic{Path: "analyses." + result.ID + ".control_loops." + loop.ID, Message: "return-ratio evidence could not be canonically encoded"}
		}
		hash := sha256.Sum256(samples)
		loop.CrossoverFrequencyHz = normalizedMNAFloat(crossover)
		loop.PhaseMarginDeg = normalizedMNAFloat(phaseMargin)
		loop.GainMarginDB = normalizedMNAFloat(gainMargin)
		loop.ClosedLoopPeakingDB = normalizedMNAFloat(peaking)
		loop.ReturnRatioSamplesSHA256 = hex.EncodeToString(hash[:])
	}
	return nil
}

func returnRatioCrossover(frequencies, magnitudes, phases []float64) (float64, float64, bool) {
	for index := 1; index < len(magnitudes); index++ {
		if magnitudes[index-1] >= 1 && magnitudes[index] <= 1 {
			fraction := logarithmicCrossingFraction(magnitudes[index-1], magnitudes[index], 1)
			logFrequency := math.Log(frequencies[index-1]) + fraction*(math.Log(frequencies[index])-math.Log(frequencies[index-1]))
			phase := phases[index-1] + fraction*(phases[index]-phases[index-1])
			return math.Exp(logFrequency), 180 + phase, true
		}
	}
	return 0, 0, false
}

func controlLoopDerivedValue(result AnalysisResult, assertion Assertion) (float64, *Diagnostic) {
	for _, loop := range result.ControlLoops {
		if loop.ObservationNet != assertion.Node {
			continue
		}
		switch assertion.Quantity {
		case QuantityLoopCrossoverHz:
			return loop.CrossoverFrequencyHz, nil
		case QuantityClosedLoopPeakingDB:
			return loop.ClosedLoopPeakingDB, nil
		}
	}
	return 0, advancedAssertionDiagnostic(assertion, "stability assertion did not resolve to one derived control loop")
}

func compareStrings(left, right string) int {
	switch {
	case left < right:
		return -1
	case left > right:
		return 1
	default:
		return 0
	}
}
