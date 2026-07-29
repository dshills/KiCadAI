package blocks

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"testing"

	"kicadai/internal/components"
	"kicadai/internal/transactions"
)

type projectedBlockTopology struct {
	symbolsByRole   map[string]transactions.AddSymbolOperation
	pinNetsByRole   map[string]map[string]string
	functionNets    map[string]map[string]string
	portNets        map[string]string
	orderedEvidence []string
}

func projectBlockTopology(t *testing.T, definition BlockDefinition, instanceID string, params map[string]any, operations []transactions.Operation) projectedBlockTopology {
	t.Helper()
	catalog, err := components.LoadCatalog(context.Background(), components.LoadOptions{})
	if err != nil {
		t.Fatalf("load embedded component catalog: %v", err)
	}
	recordsByID := make(map[string]components.ComponentRecord, len(catalog.Records))
	for _, record := range catalog.Records {
		recordsByID[record.ID] = record
	}
	componentsByRole := blockComponentByRole(definition.Components)
	rolesByRef := map[string]string{}
	topology := projectedBlockTopology{
		symbolsByRole: map[string]transactions.AddSymbolOperation{},
		pinNetsByRole: map[string]map[string]string{},
		functionNets:  map[string]map[string]string{},
		portNets:      map[string]string{},
	}
	for index, operation := range operations {
		if operation.Op != transactions.OpAddSymbol {
			continue
		}
		var symbol transactions.AddSymbolOperation
		if err := decodeBlockOperation(operation, &symbol); err != nil {
			t.Fatalf("decode add_symbol operation %d: %v", index, err)
		}
		if symbol.Role == "" {
			t.Fatalf("add_symbol %s has no component role", symbol.Ref)
		}
		if prior := topology.symbolsByRole[symbol.Role]; prior.Ref != "" {
			t.Fatalf("component role %s emitted more than once: %s and %s", symbol.Role, prior.Ref, symbol.Ref)
		}
		topology.symbolsByRole[symbol.Role] = symbol
		rolesByRef[symbol.Ref] = symbol.Role
	}
	recordEndpoint := func(endpoint transactions.Endpoint, netName string) {
		if endpoint.Ref == instanceID {
			recordTopologyNet(t, topology.portNets, endpoint.Pin, netName, "port")
			return
		}
		role := rolesByRef[endpoint.Ref]
		if role == "" {
			t.Fatalf("connect endpoint %s.%s has no emitted component role", endpoint.Ref, endpoint.Pin)
		}
		if topology.pinNetsByRole[role] == nil {
			topology.pinNetsByRole[role] = map[string]string{}
		}
		recordTopologyNet(t, topology.pinNetsByRole[role], endpoint.Pin, netName, "component "+role)
	}
	for index, operation := range operations {
		if operation.Op != transactions.OpConnect {
			continue
		}
		var connect transactions.ConnectOperation
		if err := decodeBlockOperation(operation, &connect); err != nil {
			t.Fatalf("decode connect operation %d: %v", index, err)
		}
		recordEndpoint(connect.From, connect.NetName)
		recordEndpoint(connect.To, connect.NetName)
	}
	for role, component := range componentsByRole {
		symbol := topology.symbolsByRole[role]
		if symbol.Ref == "" {
			continue
		}
		componentID := component.ComponentID
		if component.ComponentIDParam != "" {
			if selectedID := stringParam(params, component.ComponentIDParam); selectedID != "" {
				componentID = selectedID
			}
		}
		var bindings []components.SymbolBinding
		if componentID != "" {
			record, ok := recordsByID[componentID]
			if !ok {
				t.Fatalf("component role %s references missing catalog component %s", role, componentID)
			}
			bindings = record.Symbols
		} else {
			signatures := map[string]bool{}
			for _, record := range catalog.Records {
				for _, binding := range record.Symbols {
					if binding.SymbolID == symbol.LibraryID {
						bindings = append(bindings, binding)
						signatures[symbolBindingFunctionSignature(binding)] = true
					}
				}
			}
			if len(signatures) != 1 {
				bindings = nil
			}
		}
		for _, binding := range bindings {
			if !symbolBindingMatches(binding.SymbolID, symbol.LibraryID) {
				continue
			}
			if topology.functionNets[role] == nil {
				topology.functionNets[role] = map[string]string{}
			}
			for _, functionPin := range binding.FunctionPins {
				netName := topology.pinNetsByRole[role][functionPin.SymbolPin]
				if netName == "" {
					continue
				}
				recordTopologyNet(t, topology.functionNets[role], functionPin.Function, netName, "component function "+role)
			}
		}
	}
	for role, pins := range topology.pinNetsByRole {
		for pin, netName := range pins {
			topology.orderedEvidence = append(topology.orderedEvidence, fmt.Sprintf("%s.pin.%s=%s", role, pin, netName))
		}
	}
	for role, symbol := range topology.symbolsByRole {
		topology.orderedEvidence = append(topology.orderedEvidence, fmt.Sprintf("%s.symbol=%s value=%s", role, symbol.LibraryID, symbol.Value))
	}
	for role, functions := range topology.functionNets {
		for function, netName := range functions {
			topology.orderedEvidence = append(topology.orderedEvidence, fmt.Sprintf("%s.function.%s=%s", role, function, netName))
		}
	}
	for port, netName := range topology.portNets {
		topology.orderedEvidence = append(topology.orderedEvidence, fmt.Sprintf("port.%s=%s", port, netName))
	}
	sort.Strings(topology.orderedEvidence)
	return topology
}

func symbolBindingMatches(catalogSymbolID, emittedSymbolID string) bool {
	if catalogSymbolID == emittedSymbolID {
		return true
	}
	catalogParts := strings.SplitN(catalogSymbolID, ":", 2)
	emittedParts := strings.SplitN(emittedSymbolID, ":", 2)
	return len(catalogParts) == 2 && len(emittedParts) == 2 && catalogParts[1] == emittedParts[1]
}

func symbolBindingFunctionSignature(binding components.SymbolBinding) string {
	pairs := make([]string, 0, len(binding.FunctionPins))
	for _, functionPin := range binding.FunctionPins {
		pairs = append(pairs, strings.ToUpper(functionPin.Function)+"="+functionPin.SymbolPin)
	}
	sort.Strings(pairs)
	return strings.Join(pairs, ",")
}

func recordTopologyNet(t *testing.T, values map[string]string, key, netName, subject string) {
	t.Helper()
	if key == "" || netName == "" {
		t.Fatalf("%s topology contains an empty key or net: %q=%q", subject, key, netName)
	}
	if prior := values[key]; prior != "" && prior != netName {
		t.Fatalf("%s %s is connected to conflicting nets %s and %s", subject, key, prior, netName)
	}
	values[key] = netName
}

func (topology projectedBlockTopology) requirePinNet(t *testing.T, role, pin, want string) {
	t.Helper()
	if got := topology.pinNetsByRole[role][pin]; got != want {
		t.Fatalf("%s pin %s net = %q, want %q; topology=%v", role, pin, got, want, topology.orderedEvidence)
	}
}

func (topology projectedBlockTopology) requireFunctionNet(t *testing.T, role, function, want string) {
	t.Helper()
	if got := topology.functionNets[role][function]; got != want {
		t.Fatalf("%s function %s net = %q, want %q; topology=%v", role, function, got, want, topology.orderedEvidence)
	}
}

func (topology projectedBlockTopology) requirePortNet(t *testing.T, port, want string) {
	t.Helper()
	if got := topology.portNets[port]; got != want {
		t.Fatalf("port %s net = %q, want %q; topology=%v", port, got, want, topology.orderedEvidence)
	}
}
