package simmodel

import "math"

// stampFixedBuckModule models an integrated, fixed-output switching module as
// a power-conserving controlled branch. Positive branch current is delivered
// to VOUT; the reviewed conversion-efficiency boundary reflects the
// corresponding input current into VIN and the conversion loss into GND.
func stampFixedBuckModule(
	system *mnaSystem,
	component string,
	terminals map[string]string,
	outputV, inputCurrentRatio float64,
) {
	branch := system.branchIndex[component]
	if output, exists := system.nodeIndex[terminals["VOUT"]]; exists {
		system.matrix[output][branch] -= 1
	}
	if input, exists := system.nodeIndex[terminals["VIN"]]; exists {
		system.matrix[input][branch] += complex(inputCurrentRatio, 0)
	}
	if ground, exists := system.nodeIndex[terminals["GND"]]; exists {
		system.matrix[ground][branch] += complex(1-inputCurrentRatio, 0)
	}
	if output, exists := system.nodeIndex[terminals["VOUT"]]; exists {
		system.matrix[branch][output] += 1
	}
	if ground, exists := system.nodeIndex[terminals["GND"]]; exists {
		system.matrix[branch][ground] -= 1
	}
	system.rhs[branch] += complex(outputV, 0)
}

func fixedBuckModuleInputCurrentRatio(parameters map[string]float64) float64 {
	ratio := parameters["output_voltage_v"] /
		parameters["input_current_reference_voltage_v"] /
		parameters["conversion_efficiency_fraction"]
	if !finite(ratio) {
		return 0
	}
	return math.Max(0, math.Min(1/parameters["conversion_efficiency_fraction"], ratio))
}

// stampProtectedIsolatedConverter preserves the reviewed conversion-power
// boundary while keeping the primary and secondary references galvanically
// independent. Positive branch current is the secondary load current.
func stampProtectedIsolatedConverter(
	system *mnaSystem,
	component string,
	terminals map[string]string,
	outputV, inputCurrentRatio float64,
) {
	branch := system.branchIndex[component]
	if output, exists := system.nodeIndex[terminals["VOUT_PLUS"]]; exists {
		system.matrix[output][branch] -= 1
	}
	if outputReturn, exists := system.nodeIndex[terminals["VOUT_MINUS"]]; exists {
		system.matrix[outputReturn][branch] += 1
	}
	if input, exists := system.nodeIndex[terminals["VIN_PLUS"]]; exists {
		system.matrix[input][branch] += complex(inputCurrentRatio, 0)
	}
	if inputReturn, exists := system.nodeIndex[terminals["VIN_MINUS"]]; exists {
		system.matrix[inputReturn][branch] -= complex(inputCurrentRatio, 0)
	}
	if output, exists := system.nodeIndex[terminals["VOUT_PLUS"]]; exists {
		system.matrix[branch][output] += 1
	}
	if outputReturn, exists := system.nodeIndex[terminals["VOUT_MINUS"]]; exists {
		system.matrix[branch][outputReturn] -= 1
	}
	system.rhs[branch] += complex(outputV, 0)
}

func protectedIsolatedConverterInputCurrentRatio(parameters map[string]float64) float64 {
	reference := parameters["input_current_reference_voltage_v"]
	if reference <= 0 {
		reference = (parameters["input_min_v"] + parameters["input_max_v"]) / 2
	}
	ratio := parameters["output_voltage_v"] / reference / parameters["efficiency_ratio"]
	if !finite(ratio) {
		return 0
	}
	return math.Max(0, ratio)
}
