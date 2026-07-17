package circuitgraph

import (
	"slices"
	"strings"
)

func normalizeFunctionIntent(intent *FunctionIntent) {
	if intent == nil {
		return
	}
	slices.SortStableFunc(intent.Functions, func(left, right FunctionRequirement) int {
		return strings.Compare(left.ID, right.ID)
	})
	for index := range intent.Functions {
		function := &intent.Functions[index]
		slices.SortStableFunc(function.Parameters, func(left, right Parameter) int {
			return strings.Compare(left.Name, right.Name)
		})
		slices.SortStableFunc(function.RequiredRatings, func(left, right RequiredRating) int {
			if left.Kind != right.Kind {
				return strings.Compare(left.Kind, right.Kind)
			}
			if left.Unit != right.Unit {
				return strings.Compare(left.Unit, right.Unit)
			}
			return strings.Compare(left.Value, right.Value)
		})
		slices.SortFunc(function.RequiredFunctions, strings.Compare)
	}
	slices.SortStableFunc(intent.Interfaces, func(left, right InterfaceRequirement) int {
		return strings.Compare(left.ID, right.ID)
	})
	for index := range intent.Interfaces {
		slices.SortStableFunc(intent.Interfaces[index].Signals, func(left, right InterfaceSignal) int {
			return strings.Compare(left.Name, right.Name)
		})
	}
	slices.SortStableFunc(intent.PowerDomains, func(left, right PowerDomainIntent) int {
		return strings.Compare(left.Name, right.Name)
	})
	slices.SortStableFunc(intent.Connections, func(left, right FunctionConnection) int {
		return strings.Compare(left.Name, right.Name)
	})
	for index := range intent.Connections {
		slices.SortStableFunc(intent.Connections[index].Endpoints, compareFunctionalEndpoints)
	}
}

func compareFunctionalEndpoints(left, right FunctionalEndpoint) int {
	if left.Function != right.Function {
		return strings.Compare(left.Function, right.Function)
	}
	if left.Port != right.Port {
		return strings.Compare(normalizedFunctionKey(left.Port), normalizedFunctionKey(right.Port))
	}
	if left.Interface != right.Interface {
		return strings.Compare(left.Interface, right.Interface)
	}
	return strings.Compare(left.Signal, right.Signal)
}

func cloneFunctionIntent(intent FunctionIntent) FunctionIntent {
	cloned := intent
	cloned.Functions = append([]FunctionRequirement(nil), intent.Functions...)
	for index := range cloned.Functions {
		function := &cloned.Functions[index]
		if intent.Functions[index].Query != nil {
			query := *intent.Functions[index].Query
			function.Query = &query
		}
		function.Parameters = append([]Parameter(nil), intent.Functions[index].Parameters...)
		for parameterIndex := range function.Parameters {
			function.Parameters[parameterIndex].Value = cloneParameterValue(intent.Functions[index].Parameters[parameterIndex].Value)
		}
		function.RequiredRatings = append([]RequiredRating(nil), intent.Functions[index].RequiredRatings...)
		function.RequiredFunctions = append([]string(nil), intent.Functions[index].RequiredFunctions...)
		function.Extensions = cloneRawMessages(intent.Functions[index].Extensions)
	}
	cloned.Interfaces = append([]InterfaceRequirement(nil), intent.Interfaces...)
	for index := range cloned.Interfaces {
		cloned.Interfaces[index].Signals = append([]InterfaceSignal(nil), intent.Interfaces[index].Signals...)
	}
	cloned.PowerDomains = append([]PowerDomainIntent(nil), intent.PowerDomains...)
	cloned.Connections = append([]FunctionConnection(nil), intent.Connections...)
	for index := range cloned.Connections {
		cloned.Connections[index].Endpoints = append([]FunctionalEndpoint(nil), intent.Connections[index].Endpoints...)
	}
	return cloned
}
