// Package numericalconformance compares the production Go MNA evaluator with
// independent analytical equations and optionally pinned ngspice 47.
package numericalconformance

import (
	"fmt"
	"kicadai/internal/evidencebundle"
	"kicadai/internal/simmodel"
	"math"
	"math/cmplx"
)

const Schema = "kicadai.numerical-conformance.v1"
const NgspiceVersion = "47"
const NgspiceSourceSHA256 = "894e649651f1838a14095e5a5439e7d3aa63e87ede14d283173fda4fcdef675f"

type Fixture struct {
	ID             string  `json:"id"`
	Kind           string  `json:"kind"`
	Vin            float64 `json:"vin"`
	R1             float64 `json:"r1_ohms"`
	R2             float64 `json:"r2_ohms"`
	C              float64 `json:"capacitance_f"`
	Load           float64 `json:"load_ohms"`
	FrequencyRatio float64 `json:"frequency_ratio"`
}
type Value struct {
	Real      float64 `json:"real"`
	Imaginary float64 `json:"imaginary"`
}

// Fixtures is a frozen, bounded corpus, not a claim about arbitrary circuits.
func Fixtures() []Fixture {
	result := []Fixture{
		{ID: "divider-equal", Kind: "dc", Vin: 5, R1: 10000, R2: 10000},
		{ID: "divider-loaded", Kind: "dc", Vin: 5, R1: 10000, R2: 10000, Load: 10000},
		{ID: "divider-asymmetric", Kind: "dc", Vin: 3.3, R1: 4700, R2: 2200},
		{ID: "divider-high-resistance", Kind: "dc", Vin: .1, R1: 1e6, R2: 1000, Load: 1e9},
	}
	for _, load := range []float64{0, 10000} {
		for _, ratio := range []float64{.1, 1, 10} {
			result = append(result, Fixture{ID: fmt.Sprintf("rc-ac-load-%.0f-ratio-%g", load, ratio), Kind: "ac", Vin: 5, R1: 10000, C: 1e-7, Load: load, FrequencyRatio: ratio})
		}
		result = append(result, Fixture{ID: fmt.Sprintf("rc-step-load-%.0f", load), Kind: "transient", Vin: 5, R1: 10000, C: 1e-7, Load: load})
	}
	return result
}
func Lookup(id string) (Fixture, bool) {
	for _, f := range Fixtures() {
		if f.ID == id {
			return f, true
		}
	}
	return Fixture{}, false
}
func CorpusSHA256() string {
	raw, _ := evidencebundle.JSON(struct {
		Fixtures                                                               []Fixture
		AbsoluteTolerance, LinearRelativeTolerance, TransientRelativeTolerance float64
	}{Fixtures(), 1e-8, 1e-7, .005})
	return evidencebundle.Digest(raw)
}
func (f Fixture) tau() float64 {
	r := f.R1
	if f.Load > 0 {
		r = 1 / (1/f.R1 + 1/f.Load)
	}
	return r * f.C
}
func (f Fixture) gain() float64 {
	if f.Load > 0 {
		return f.Load / (f.R1 + f.Load)
	}
	return 1
}
func (f Fixture) frequency() float64 { return f.FrequencyRatio / (2 * math.Pi * f.tau()) }
func (f Fixture) Expected() Value {
	if f.Kind == "dc" {
		conductance := 1 / f.R2
		if f.Load > 0 {
			conductance += 1 / f.Load
		}
		return Value{Real: f.Vin / (1 + f.R1*conductance)}
	}
	if f.Kind == "transient" {
		return Value{Real: f.Vin * f.gain() * (-math.Expm1(-1))}
	}
	z := complex(f.gain(), 0) / complex(1, f.FrequencyRatio)
	return Value{Real: real(z), Imaginary: imag(z)}
}
func (f Fixture) Tolerance() float64 {
	if f.Kind == "transient" {
		return .005
	}
	return 1e-7
}
func Within(a, b Value, relative float64) bool {
	for _, pair := range [][2]float64{{a.Real, b.Real}, {a.Imaginary, b.Imaginary}} {
		if math.IsNaN(pair[0]) || math.IsInf(pair[0], 0) || math.IsNaN(pair[1]) || math.IsInf(pair[1], 0) || math.Abs(pair[0]-pair[1]) > 1e-8+relative*math.Abs(pair[1]) {
			return false
		}
	}
	return true
}

func (f Fixture) Plan() (simmodel.Plan, []simmodel.Diagnostic) {
	primitive := func(id, family, model, a, b string, value float64, hasValue bool) simmodel.ComponentEvidence {
		termA, termB := "A", "B"
		if family == "voltage_source" {
			termA, termB = "POSITIVE", "NEGATIVE"
		}
		return simmodel.ComponentEvidence{InstanceID: id, CatalogID: "conformance." + family, Family: family, ValueSI: value, HasValueSI: hasValue, ModelClaims: []simmodel.CatalogEvidence{{ModelID: model}}, Connections: []simmodel.ConnectionEvidence{{Function: termA, Net: a}, {Function: termB, Net: b}}}
	}
	components := []simmodel.ComponentEvidence{primitive("source", "voltage_source", simmodel.PrimitiveVoltageSourceV1, "VIN", "GND", 0, false), primitive("r1", "resistor", simmodel.PrimitiveResistorV1, "VIN", "OUT", f.R1, true)}
	if f.Kind == "dc" {
		components = append(components, primitive("r2", "resistor", simmodel.PrimitiveResistorV1, "OUT", "GND", f.R2, true))
	} else {
		capModel := simmodel.PrimitiveCapacitorV1
		if f.Kind == "transient" {
			capModel = simmodel.PrimitiveCapacitorTransientV1
		}
		capacitor := primitive("c1", "capacitor", capModel, "OUT", "GND", f.C, true)
		if f.Kind == "transient" {
			capacitor.ModelClaims[0].Parameters = []simmodel.NamedValue{{Name: "max_voltage_v", Value: 25}}
		}
		components = append(components, capacitor)
	}
	if f.Load > 0 {
		components = append(components, primitive("load", "resistor", simmodel.PrimitiveResistorV1, "OUT", "GND", f.Load, true))
	}
	excitation := simmodel.SourceExcitation{Component: "source", DCValue: f.Vin}
	analysis := simmodel.Analysis{ID: "comparison", Kind: simmodel.AnalysisDCOperatingPoint, Excitations: []simmodel.SourceExcitation{excitation}}
	model := simmodel.ModelLinearCircuitMNAV1
	if f.Kind == "ac" {
		analysis.Kind = simmodel.AnalysisACSweep
		analysis.StartFrequencyHz = f.frequency()
		analysis.StopFrequencyHz = f.frequency() * 10
		analysis.Points = 2
		analysis.Excitations[0].ACMagnitude = 1
	}
	if f.Kind == "transient" {
		model = simmodel.ModelTransientCircuitV1
		analysis.Kind = simmodel.AnalysisTransient
		analysis.TimeStepS = f.tau() / 1000
		analysis.DurationS = 1.1 * f.tau()
		analysis.Excitations = []simmodel.SourceExcitation{{Component: "source", PulseInitialValue: 0, PulseValue: f.Vin, PulseDelayS: .1 * f.tau(), PulseWidthS: 2 * f.tau(), PulsePeriodS: 4 * f.tau()}}
	}
	// The production resolver requires a node assertion. This broad passive
	// envelope is separate from the stricter independent comparison oracle.
	assertion := simmodel.Assertion{AnalysisID: analysis.ID, Node: "OUT", Quantity: simmodel.QuantityVoltageV, Min: 0, Max: f.Vin}
	if f.Kind == "ac" {
		assertion.Quantity = simmodel.QuantityVoltageMagnitudeV
		assertion.FrequencyHz = f.frequency()
		assertion.Max = 1
	}
	if f.Kind == "transient" {
		assertion.TimeS = analysis.DurationS
	}
	return simmodel.ResolveWithTopology(simmodel.Intent{ModelID: model, Analyses: []simmodel.Analysis{analysis}, Assertions: []simmodel.Assertion{assertion}}, "conformance-ideal-passives-v1", CorpusSHA256(), components, []simmodel.NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "VIN", Role: "signal"}, {Name: "OUT", Role: "signal"}})
}

func (f Fixture) Netlist() string {
	source := fmt.Sprintf("DC %.17g AC 1", f.Vin)
	if f.Kind == "transient" {
		source = fmt.Sprintf("PULSE(0 %.17g %.17g %.17g %.17g %.17g %.17g)", f.Vin, .1*f.tau(), f.tau()*1e-6, f.tau()*1e-6, 2*f.tau(), 4*f.tau())
	}
	text := fmt.Sprintf("KiCadAI frozen conformance %s\nV1 vin 0 %s\nR1 vin out %.17g\n", f.ID, source, f.R1)
	if f.Kind == "dc" {
		text += fmt.Sprintf("R2 out 0 %.17g\n", f.R2)
	} else {
		text += fmt.Sprintf("C1 out 0 %.17g\n", f.C)
	}
	if f.Load > 0 {
		text += fmt.Sprintf("RL out 0 %.17g\n", f.Load)
	}
	text += ".options reltol=1e-9 abstol=1e-12 vntol=1e-10\n.control\nset noaskquit\nset numdgt=17\n"
	switch f.Kind {
	case "dc":
		text += "op\nlet observed_real=v(out)\nlet observed_imag=0\nprint observed_real\nprint observed_imag\n"
	case "ac":
		text += fmt.Sprintf("ac lin 2 %.17g %.17g\nlet observed_real=real(v(out)[0])\nlet observed_imag=imag(v(out)[0])\nprint observed_real\nprint observed_imag\n", f.frequency(), f.frequency()*10)
	case "transient":
		text += fmt.Sprintf("tran %.17g %.17g 0 %.17g\nmeas tran observed_real FIND v(out) AT=%.17g\nlet observed_imag=0\nprint observed_imag\n", f.tau()/1000, 1.1*f.tau(), f.tau()/1000, 1.1*f.tau())
	}
	return text + "quit\n.endc\n.end\n"
}

func observe(f Fixture, report simmodel.Report) (Value, error) {
	if report.Status != "pass" || len(report.Analyses) != 1 || len(report.Analyses[0].Points) == 0 {
		return Value{}, fmt.Errorf("Go evaluator did not return one passing analysis")
	}
	points := report.Analyses[0].Points
	point := points[0]
	if f.Kind == "transient" {
		point = points[len(points)-1]
		if math.Abs(point.TimeS-1.1*f.tau()) > 1e-12 {
			return Value{}, fmt.Errorf("transient observation grid differs")
		}
	}
	for _, node := range point.Nodes {
		if node.Node == "OUT" {
			z := complex(node.Real, node.Imaginary)
			if math.IsNaN(cmplx.Abs(z)) || math.IsInf(cmplx.Abs(z), 0) {
				break
			}
			return Value{node.Real, node.Imaginary}, nil
		}
	}
	return Value{}, fmt.Errorf("Go output node absent or nonfinite")
}
