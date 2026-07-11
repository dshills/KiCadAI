package designworkflow

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"kicadai/internal/blocks"
	"kicadai/internal/components"
	"kicadai/internal/transactions"
)

type componentRoutingHintResult struct {
	Evidence []ComponentHintEvidence
}

type componentRoutingOperationIndex struct {
	Connects   []transactions.ConnectOperation
	NoConnects []transactions.AddNoConnectOperation
}

func componentRoutingHints(selections []ComponentSelectionEntry, fragments PCBFragmentResult) componentRoutingHintResult {
	hints := NormalizeComponentHints(selections)
	if len(hints) == 0 {
		return componentRoutingHintResult{}
	}
	fragmentByInstance := map[string]*BlockFragment{}
	for index := range fragments.Fragments {
		fragment := &fragments.Fragments[index]
		fragmentByInstance[fragment.InstanceID] = fragment
	}
	operationsByInstance := map[string]componentRoutingOperationIndex{}
	for _, fragment := range fragments.Fragments {
		operations := fragment.SourceOperations
		if len(operations) == 0 {
			operations = fragment.Realization.Operations
		}
		operationsByInstance[fragment.InstanceID] = componentRoutingOperationIndexFromOperations(operations)
	}
	selectionByHint := map[string]ComponentSelectionEntry{}
	for _, selection := range selections {
		selectionByHint[componentHintSelectionKey(selection.InstanceID, selection.BlockID, selection.Role, selection.ComponentID)] = selection
	}
	result := componentRoutingHintResult{Evidence: make([]ComponentHintEvidence, 0, len(hints))}
	for _, hint := range hints {
		if hint.Type != ComponentHintRouting {
			result.Evidence = append(result.Evidence, hint)
			continue
		}
		evidence := hint
		if hint.Status == ComponentHintUnsupported {
			result.Evidence = append(result.Evidence, evidence)
			continue
		}
		fragment := fragmentByInstance[hint.InstanceID]
		if fragment == nil {
			evidence.Status = ComponentHintSkipped
			evidence.Message = "routing hint skipped because block instance was not realized"
			result.Evidence = append(result.Evidence, evidence)
			continue
		}
		selection := selectionByHint[componentHintSelectionKey(hint.InstanceID, hint.BlockID, hint.Role, hint.ComponentID)]
		operationIndex := operationsByInstance[hint.InstanceID]
		switch hint.Kind {
		case "net_class":
			evidence = evaluateNetClassRoutingHint(evidence, fragment, selection)
		case "tie":
			evidence = evaluateTieRoutingHint(evidence, fragment, selection, operationIndex)
		case "no_connect":
			evidence = evaluateNoConnectRoutingHint(evidence, fragment, selection, operationIndex)
		default:
			evidence.Status = ComponentHintUnsupported
			evidence.Message = "unsupported routing hint kind"
		}
		result.Evidence = append(result.Evidence, evidence)
	}
	return result
}

func componentRoutingOperationIndexFromOperations(operations []transactions.Operation) componentRoutingOperationIndex {
	var index componentRoutingOperationIndex
	for _, operation := range operations {
		switch operation.Op {
		case transactions.OpConnect:
			var payload transactions.ConnectOperation
			if err := json.Unmarshal(operation.Raw, &payload); err == nil {
				index.Connects = append(index.Connects, payload)
			}
		case transactions.OpAddNoConnect:
			var payload transactions.AddNoConnectOperation
			if err := json.Unmarshal(operation.Raw, &payload); err == nil {
				index.NoConnects = append(index.NoConnects, payload)
			}
		}
	}
	return index
}

func evaluateNetClassRoutingHint(hint ComponentHintEvidence, fragment *BlockFragment, selection ComponentSelectionEntry) ComponentHintEvidence {
	requested, ok, message := componentHintWidthMM(hint)
	if !ok {
		hint.Status = ComponentHintSkipped
		hint.Message = message
		return hint
	}
	var observed float64
	matched := false
	for _, route := range fragment.Realization.LocalRoutes {
		if !componentHintRouteMatchesNetRole(hint, fragment, selection, route) {
			continue
		}
		if !matched || route.WidthMM < observed {
			observed = route.WidthMM
		}
		matched = true
	}
	if !matched {
		hint.Status = ComponentHintSkipped
		hint.Message = "routing hint skipped because net role did not map to realized local routes"
		return hint
	}
	if observed >= requested {
		hint.Status = ComponentHintEnforced
		hint.Message = fmt.Sprintf("routing hint enforced by local routes with %.3f mm width", observed)
		return hint
	}
	hint.Status = ComponentHintFailed
	hint.Message = fmt.Sprintf("routing hint failed because narrowest matching local route is %.3f mm", observed)
	return hint
}

func evaluateTieRoutingHint(hint ComponentHintEvidence, fragment *BlockFragment, selection ComponentSelectionEntry, operationIndex componentRoutingOperationIndex) ComponentHintEvidence {
	ref := strings.TrimSpace(fragment.Realization.RoleRefs[hint.Role])
	if ref == "" {
		hint.Status = ComponentHintSkipped
		hint.Message = "routing hint skipped because source role ref was missing"
		return hint
	}
	expectedPin, ok := componentHintExpectedPin(hint, selection)
	if !ok {
		hint.Status = ComponentHintSkipped
		hint.Message = "routing hint skipped because pin mapping is not available"
		return hint
	}
	for _, payload := range operationIndex.Connects {
		if connectOperationTouchesPin(payload, ref, expectedPin) && componentHintTieNetMatches(hint.NetRole, payload.NetName) {
			hint.Status = ComponentHintSatisfiedByBlock
			hint.Message = "routing hint satisfied by block-local tie operation"
			return hint
		}
	}
	hint.Status = ComponentHintSkipped
	hint.Message = "routing hint skipped because no block-local tie operation was found"
	return hint
}

func evaluateNoConnectRoutingHint(hint ComponentHintEvidence, fragment *BlockFragment, selection ComponentSelectionEntry, operationIndex componentRoutingOperationIndex) ComponentHintEvidence {
	ref := strings.TrimSpace(fragment.Realization.RoleRefs[hint.Role])
	if ref == "" {
		hint.Status = ComponentHintSkipped
		hint.Message = "routing hint skipped because source role ref was missing"
		return hint
	}
	expectedPin, ok := componentHintExpectedPin(hint, selection)
	if !ok {
		hint.Status = ComponentHintSkipped
		hint.Message = "routing hint skipped because pin mapping is not available"
		return hint
	}
	for _, payload := range operationIndex.NoConnects {
		if strings.EqualFold(payload.Endpoint.Ref, ref) && strings.EqualFold(payload.Endpoint.Pin, expectedPin) {
			hint.Status = ComponentHintSatisfiedByBlock
			hint.Message = "routing hint satisfied by block-local no-connect operation"
			return hint
		}
	}
	hint.Status = ComponentHintSkipped
	hint.Message = "routing hint skipped because no block-local no-connect operation was found"
	return hint
}

func connectOperationTouchesPin(payload transactions.ConnectOperation, ref string, pin string) bool {
	return (strings.EqualFold(payload.From.Ref, ref) && strings.EqualFold(payload.From.Pin, pin)) ||
		(strings.EqualFold(payload.To.Ref, ref) && strings.EqualFold(payload.To.Pin, pin))
}

func componentHintWidthMM(hint ComponentHintEvidence) (float64, bool, string) {
	valueText := strings.TrimSpace(hint.Value)
	unit := strings.ToLower(strings.TrimSpace(hint.Unit))
	if valueText == "" {
		return 0, false, "routing hint skipped because width value is missing"
	}
	value, err := strconv.ParseFloat(valueText, 64)
	if err != nil {
		return 0, false, "routing hint skipped because width value is not numeric"
	}
	if value < 0 {
		return 0, false, "routing hint skipped because width value is negative"
	}
	switch unit {
	case "", "mm":
	case "mil", "mils", "th", "thou":
		value *= 0.0254
	default:
		return 0, false, "routing hint skipped because width unit is not supported"
	}
	return value, true, ""
}

func componentHintExpectedPin(hint ComponentHintEvidence, selection ComponentSelectionEntry) (string, bool) {
	target := componentHintPinFunction(hint)
	if target == "" {
		return "", false
	}
	for _, functionPin := range selection.FunctionPins {
		if componentHintFunctionMatches(functionPin, target) {
			pin := strings.TrimSpace(functionPin.SymbolPin)
			return pin, pin != ""
		}
	}
	return "", false
}

func componentHintPinFunction(hint ComponentHintEvidence) string {
	switch normalizeRoleName(hint.NetRole) {
	case "enable":
		return "enable"
	case "nc", "no_connect":
		return "nc"
	default:
		return normalizeRoleName(hint.NetRole)
	}
}

func componentHintFunctionMatches(functionPin components.FunctionPin, target string) bool {
	function := normalizeRoleName(functionPin.Function)
	if function == target || componentHintEquivalentFunction(function, target) {
		return true
	}
	for _, alias := range functionPin.Aliases {
		alias = normalizeRoleName(alias)
		if alias == target || componentHintEquivalentFunction(alias, target) {
			return true
		}
	}
	return false
}

func componentHintEquivalentFunction(function string, target string) bool {
	return target == "enable" && function == "en"
}

func componentHintRouteMatchesNetRole(hint ComponentHintEvidence, fragment *BlockFragment, selection ComponentSelectionEntry, route blocks.RealizedPCBLocalRoute) bool {
	if componentHintNetRoleMatches(hint.NetRole, route.NetName) {
		return true
	}
	ref := strings.TrimSpace(fragment.Realization.RoleRefs[hint.Role])
	if ref == "" {
		return false
	}
	pins := componentHintPinsForNetRole(hint.NetRole, selection)
	if len(pins) == 0 {
		return false
	}
	return endpointMatchesAnyPin(route.From, ref, pins) || endpointMatchesAnyPin(route.To, ref, pins)
}

func componentHintPinsForNetRole(role string, selection ComponentSelectionEntry) map[string]bool {
	role = normalizeRoleName(role)
	pins := map[string]bool{}
	for _, functionPin := range selection.FunctionPins {
		if componentHintFunctionInNetRole(functionPin, role) {
			pin := strings.TrimSpace(functionPin.SymbolPin)
			if pin != "" {
				pins[strings.ToLower(pin)] = true
			}
		}
	}
	return pins
}

func componentHintFunctionInNetRole(functionPin components.FunctionPin, role string) bool {
	function := normalizeRoleName(functionPin.Function)
	electrical := normalizeRoleName(functionPin.Electrical)
	switch role {
	case "power":
		return (strings.Contains(electrical, "power") && function != "gnd") ||
			function == "vin" ||
			function == "vout" ||
			function == "vcc" ||
			function == "vdd" ||
			function == "vbus"
	case "ground", "gnd":
		return function == "gnd"
	default:
		return componentHintFunctionMatches(functionPin, role)
	}
}

func endpointMatchesAnyPin(endpoint transactions.Endpoint, ref string, pins map[string]bool) bool {
	return strings.EqualFold(endpoint.Ref, ref) && pins[strings.ToLower(strings.TrimSpace(endpoint.Pin))]
}

func componentHintNetRoleMatches(role string, netName string) bool {
	role = normalizeRoleName(role)
	netName = normalizeRoleName(netName)
	if role == "" || netName == "" {
		return false
	}
	switch role {
	case "power":
		return containsToken(netName, "vin") ||
			containsToken(netName, "vout") ||
			containsToken(netName, "vcc") ||
			containsToken(netName, "vdd") ||
			containsToken(netName, "vbus") ||
			containsToken(netName, "power") ||
			containsToken(netName, "supply")
	case "ground", "gnd":
		return containsToken(netName, "gnd") || containsToken(netName, "ground")
	case "enable":
		return containsToken(netName, "en") || containsToken(netName, "enable")
	case "clock":
		return containsToken(netName, "clk") || containsToken(netName, "clock") || containsToken(netName, "xtal")
	default:
		return containsToken(netName, role)
	}
}

func componentHintTieNetMatches(role string, netName string) bool {
	if componentHintNetRoleMatches(role, netName) {
		return true
	}
	return normalizeRoleName(role) == "enable" && componentHintNetRoleMatches("power", netName)
}

func componentHintSelectionKey(instanceID string, blockID string, role string, componentID string) string {
	return strings.Join([]string{
		strings.TrimSpace(instanceID),
		strings.TrimSpace(blockID),
		strings.TrimSpace(role),
		strings.TrimSpace(componentID),
	}, "\x00")
}
