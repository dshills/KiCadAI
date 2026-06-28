package blocks

import (
	"encoding/json"
	"fmt"
	"hash/crc32"
	"regexp"
	"strings"
	"sync"

	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

var netNameSanitizer = regexp.MustCompile(`[^A-Za-z0-9_]+`)
var refPrefixSanitizer = regexp.MustCompile(`[^A-Za-z]+`)

type InstantiateOptions struct {
	OriginXMM float64 `json:"origin_x_mm,omitempty"`
	OriginYMM float64 `json:"origin_y_mm,omitempty"`
	SpacingMM float64 `json:"spacing_mm,omitempty"`
}

type ReferenceAllocator struct {
	state *referenceAllocatorState
}

type referenceAllocatorState struct {
	token string
	mu    sync.Mutex
	next  map[string]int
}

func NewReferenceAllocator() *ReferenceAllocator {
	return &ReferenceAllocator{state: &referenceAllocatorState{next: map[string]int{}}}
}

func NewInstanceReferenceAllocator(instanceID string) *ReferenceAllocator {
	token := fmt.Sprintf("%08x", crc32.ChecksumIEEE([]byte(instanceID)))
	return &ReferenceAllocator{state: &referenceAllocatorState{token: token, next: map[string]int{}}}
}

func (allocator *ReferenceAllocator) Next(prefix string) string {
	if allocator == nil || allocator.state == nil {
		panic("blocks.ReferenceAllocator must be created with NewReferenceAllocator")
	}
	allocator.state.mu.Lock()
	defer allocator.state.mu.Unlock()
	prefix = normalizeRefPrefix(prefix)
	if prefix == "" {
		prefix = "U"
	}
	if allocator.state.next == nil {
		allocator.state.next = map[string]int{}
	}
	allocator.state.next[prefix]++
	if allocator.state.token != "" {
		return fmt.Sprintf("%s%s%03d", prefix, allocator.state.token, allocator.state.next[prefix])
	}
	return fmt.Sprintf("%s%d", prefix, allocator.state.next[prefix])
}

func normalizeRefPrefix(prefix string) string {
	prefix = strings.ToUpper(strings.TrimSpace(prefix))
	prefix = refPrefixSanitizer.ReplaceAllString(prefix, "")
	return prefix
}

func InstanceNetName(instanceID string, role string) string {
	instanceID = sanitizeNetPart(instanceID)
	role = sanitizeNetPart(role)
	if role == "" {
		return instanceID
	}
	if instanceID == "" {
		return role
	}
	return instanceID + "_" + role
}

func ComponentOperations(component BlockComponent, ref string, at transactions.Point) ([]transactions.Operation, []reports.Issue) {
	var issues []reports.Issue
	if ref == "" {
		issues = append(issues, blockIssue("component."+component.Role+".ref", "component reference is required"))
	}
	if component.SymbolID == "" {
		issues = append(issues, blockIssue("component."+component.Role+".symbol_id", "component symbol ID is required"))
	}
	if len(issues) != 0 {
		return nil, issues
	}
	addSymbol, err := wrapOperation(transactions.OpAddSymbol, transactions.AddSymbolOperation{
		Op:        transactions.OpAddSymbol,
		Ref:       ref,
		Role:      component.Role,
		Value:     component.Value,
		LibraryID: component.SymbolID,
		At:        at,
		Pins:      append([]transactions.PinSpec(nil), component.Pins...),
	})
	if err != nil {
		return nil, []reports.Issue{blockIssue("component."+component.Role, err.Error())}
	}
	operations := []transactions.Operation{addSymbol}
	if component.FootprintID != "" {
		assign, err := wrapOperation(transactions.OpAssignFootprint, transactions.AssignFootprintOperation{
			Op:          transactions.OpAssignFootprint,
			Ref:         ref,
			Role:        component.Role,
			FootprintID: component.FootprintID,
		})
		if err != nil {
			return nil, []reports.Issue{blockIssue("component."+component.Role, err.Error())}
		}
		operations = append(operations, assign)
		place, err := wrapOperation(transactions.OpPlaceFootprint, transactions.PlaceFootprintOperation{
			Op:          transactions.OpPlaceFootprint,
			Ref:         ref,
			Role:        component.Role,
			FootprintID: component.FootprintID,
			Value:       component.Value,
			At:          at,
		})
		if err != nil {
			return nil, []reports.Issue{blockIssue("component."+component.Role, err.Error())}
		}
		operations = append(operations, place)
	}
	return operations, nil
}

func ConnectOperation(fromRef string, fromPin string, toRef string, toPin string, netName string) (transactions.Operation, []reports.Issue) {
	if fromRef == "" || fromPin == "" || toRef == "" || toPin == "" || netName == "" {
		return transactions.Operation{}, []reports.Issue{blockIssue("connect", "connect operation requires from ref/pin, to ref/pin, and net name")}
	}
	operation, err := wrapOperation(transactions.OpConnect, transactions.ConnectOperation{
		Op:      transactions.OpConnect,
		From:    transactions.Endpoint{Ref: fromRef, Pin: fromPin},
		To:      transactions.Endpoint{Ref: toRef, Pin: toPin},
		NetName: netName,
	})
	if err != nil {
		return transactions.Operation{}, []reports.Issue{blockIssue("connect", err.Error())}
	}
	return operation, nil
}

func NoConnectOperation(ref string, pin string) (transactions.Operation, []reports.Issue) {
	if ref == "" || pin == "" {
		return transactions.Operation{}, []reports.Issue{blockIssue("no_connect", "no-connect operation requires ref and pin")}
	}
	operation, err := wrapOperation(transactions.OpAddNoConnect, transactions.AddNoConnectOperation{
		Op:       transactions.OpAddNoConnect,
		Endpoint: transactions.Endpoint{Ref: ref, Pin: pin},
	})
	if err != nil {
		return transactions.Operation{}, []reports.Issue{blockIssue("no_connect", err.Error())}
	}
	return operation, nil
}

func dryRunBlockOutput(definition BlockDefinition, request BlockRequest, operations []transactions.Operation, issues []reports.Issue) BlockOutput {
	params := ApplyParameterDefaults(definition, request.Params)
	return BlockOutput{
		Definition: Summary(definition),
		Instance: BlockInstance{
			BlockID:    definition.ID,
			InstanceID: request.InstanceID,
			Params:     params,
			Ports:      resolvePortVoltages(definition.Ports, params),
		},
		Operations: operations,
		Issues:     issues,
	}
}

func resolvePortVoltages(ports []BlockPort, params map[string]any) []BlockPort {
	resolved := append([]BlockPort(nil), ports...)
	for i := range resolved {
		if isVoltageLiteral(resolved[i].Voltage) {
			continue
		}
		raw, exists := params[resolved[i].Voltage]
		if !exists {
			continue
		}
		value := portVoltageValue(raw)
		if value != "" {
			resolved[i].Voltage = value
		}
	}
	return resolved
}

func portVoltageValue(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case float64, float32, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return fmt.Sprint(typed)
	default:
		return ""
	}
}

func wrapOperation(kind transactions.OperationKind, payload any) (transactions.Operation, error) {
	if kind == "" {
		return transactions.Operation{}, fmt.Errorf("operation kind is required")
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return transactions.Operation{}, err
	}
	return transactions.NewOperationWithMetadata(kind, data, operationRef(payload), operationNetName(payload)), nil
}

func operationRef(payload any) string {
	switch value := payload.(type) {
	case transactions.AddSymbolOperation:
		return value.Ref
	case transactions.AssignFootprintOperation:
		return value.Ref
	case transactions.PlaceFootprintOperation:
		return value.Ref
	case transactions.AddNoConnectOperation:
		return value.Endpoint.Ref
	default:
		return ""
	}
}

func operationNetName(payload any) string {
	switch value := payload.(type) {
	case transactions.ConnectOperation:
		return value.NetName
	case *transactions.ConnectOperation:
		if value != nil {
			return value.NetName
		}
	case transactions.RouteOperation:
		return value.NetName
	case *transactions.RouteOperation:
		if value != nil {
			return value.NetName
		}
	case transactions.AddZoneOperation:
		if value.NetName != nil {
			return *value.NetName
		}
	case *transactions.AddZoneOperation:
		if value != nil && value.NetName != nil {
			return *value.NetName
		}
	}
	return ""
}

func sanitizeNetPart(value string) string {
	value = strings.TrimSpace(value)
	value = netNameSanitizer.ReplaceAllString(value, "_")
	value = strings.Trim(value, "_")
	return value
}
