package blocks

import (
	"context"
	"fmt"

	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

const compositionInstanceSpacingMM = 90.0

type CompositionRequest struct {
	ProjectName string                  `json:"project_name,omitempty"`
	Instances   []CompositionInstance   `json:"instances"`
	Connections []CompositionConnection `json:"connections,omitempty"`
	NetAliases  map[string]string       `json:"net_aliases,omitempty"`
}

type CompositionInstance struct {
	ID      string         `json:"id"`
	BlockID string         `json:"block_id"`
	Params  map[string]any `json:"params,omitempty"`
}

type CompositionConnection struct {
	From     PortRef `json:"from"`
	To       PortRef `json:"to"`
	NetAlias string  `json:"net_alias,omitempty"`
}

type PortRef struct {
	InstanceID string `json:"instance_id"`
	Port       string `json:"port"`
}

type CompositionOutput struct {
	ProjectName string                   `json:"project_name,omitempty"`
	Instances   []BlockInstance          `json:"instances,omitempty"`
	Operations  []transactions.Operation `json:"operations,omitempty"`
	Issues      []reports.Issue          `json:"issues,omitempty"`
}

func ComposeBlocks(ctx context.Context, registry Registry, request CompositionRequest) CompositionOutput {
	output := CompositionOutput{
		ProjectName: request.ProjectName,
		Instances:   make([]BlockInstance, 0, len(request.Instances)),
	}
	seenInstances := map[string]struct{}{}
	portsByInstance := map[string]map[string]BlockPort{}
	for instanceIndex, instance := range request.Instances {
		if contextIssue := compositionContextIssue(ctx); contextIssue != nil {
			output.Issues = append(output.Issues, *contextIssue)
			return output
		}
		if instance.ID == "" {
			output.Issues = append(output.Issues, blockIssue("instances", "instance ID is required"))
			continue
		}
		if _, exists := seenInstances[instance.ID]; exists {
			output.Issues = append(output.Issues, blockIssue("instances."+instance.ID, "duplicate instance ID "+instance.ID))
			continue
		}
		seenInstances[instance.ID] = struct{}{}
		blockOutput, issues := registry.Instantiate(ctx, BlockRequest{BlockID: instance.BlockID, InstanceID: instance.ID, Params: instance.Params})
		output.Issues = append(output.Issues, issues...)
		if hasBlockingIssues(issues) {
			continue
		}
		instancePorts, portIssues := portMap(instance.ID, blockOutput.Instance.Ports)
		output.Issues = append(output.Issues, portIssues...)
		if len(portIssues) != 0 {
			continue
		}
		offsetOperations, offsetIssues := offsetCompositionOperations(blockOutput.Operations, float64(instanceIndex)*compositionInstanceSpacingMM, 0)
		output.Issues = append(output.Issues, offsetIssues...)
		if len(offsetIssues) != 0 {
			continue
		}
		output.Instances = append(output.Instances, blockOutput.Instance)
		output.Operations = append(output.Operations, offsetOperations...)
		portsByInstance[instance.ID] = instancePorts
	}
	netGroups := newPortDisjointSet()
	for _, connection := range request.Connections {
		if contextIssue := compositionContextIssue(ctx); contextIssue != nil {
			output.Issues = append(output.Issues, *contextIssue)
			return output
		}
		netGroups.union(connection.From, connection.To)
	}
	aliasesByRoot := map[PortRef]string{}
	voltageByRoot := map[PortRef]string{}
	for index, connection := range request.Connections {
		if contextIssue := compositionContextIssue(ctx); contextIssue != nil {
			output.Issues = append(output.Issues, *contextIssue)
			return output
		}
		root := netGroups.find(connection.From)
		if connection.NetAlias != "" {
			alias := connection.NetAlias
			if mapped, ok := request.NetAliases[alias]; ok {
				alias = mapped
			}
			alias = sanitizeNetPart(alias)
			if existing := aliasesByRoot[root]; existing != "" && existing != alias {
				output.Issues = append(output.Issues, blockIssue(fmt.Sprintf("connections[%d].net_alias", index), "conflicting net alias for connected ports"))
			} else {
				aliasesByRoot[root] = alias
			}
		}
		for _, ref := range []PortRef{connection.From, connection.To} {
			port, ok := lookupPort(portsByInstance, ref)
			if !ok || port.Voltage == "" {
				continue
			}
			root := netGroups.find(ref)
			if existing := voltageByRoot[root]; existing != "" && existing != port.Voltage {
				output.Issues = append(output.Issues, blockIssue(fmt.Sprintf("connections[%d]", index), "conflicting voltage domains"))
				continue
			}
			voltageByRoot[root] = port.Voltage
		}
	}
	for index, connection := range request.Connections {
		if contextIssue := compositionContextIssue(ctx); contextIssue != nil {
			output.Issues = append(output.Issues, *contextIssue)
			return output
		}
		fromPort, fromOK := lookupPort(portsByInstance, connection.From)
		toPort, toOK := lookupPort(portsByInstance, connection.To)
		if !fromOK {
			output.Issues = append(output.Issues, blockIssue(fmt.Sprintf("connections[%d].from", index), "unknown port "+connection.From.InstanceID+"."+connection.From.Port))
		}
		if !toOK {
			output.Issues = append(output.Issues, blockIssue(fmt.Sprintf("connections[%d].to", index), "unknown port "+connection.To.InstanceID+"."+connection.To.Port))
		}
		if !fromOK || !toOK {
			continue
		}
		if !compatiblePortDirections(fromPort.Direction, toPort.Direction) {
			output.Issues = append(output.Issues, blockIssue(fmt.Sprintf("connections[%d]", index), "incompatible port directions"))
			continue
		}
		root := netGroups.find(connection.From)
		netName := aliasesByRoot[root]
		if netName == "" {
			netName = netGroups.netName(connection.From)
		}
		operation, issues := ConnectOperation(connection.From.InstanceID, connection.From.Port, connection.To.InstanceID, connection.To.Port, netName)
		output.Issues = append(output.Issues, issues...)
		if len(issues) == 0 {
			output.Operations = append(output.Operations, operation)
		}
	}
	output.Issues = append(output.Issues, validateI2CAddressCollisions(output.Instances, netGroups)...)
	return output
}

func offsetCompositionOperations(operations []transactions.Operation, xOffsetMM float64, yOffsetMM float64) ([]transactions.Operation, []reports.Issue) {
	if xOffsetMM == 0 && yOffsetMM == 0 {
		return append([]transactions.Operation(nil), operations...), nil
	}
	offset := append([]transactions.Operation(nil), operations...)
	var issues []reports.Issue
	for index, operation := range offset {
		switch operation.Op {
		case transactions.OpAddSymbol:
			var payload transactions.AddSymbolOperation
			if err := decodeBlockOperation(operation, &payload); err != nil {
				issues = append(issues, blockIssue(fmt.Sprintf("operations[%d]", index), "decode add_symbol for composition offset: "+err.Error()))
				continue
			}
			payload.At.XMM += xOffsetMM
			payload.At.YMM += yOffsetMM
			updated, err := wrapOperation(transactions.OpAddSymbol, payload)
			if err != nil {
				issues = append(issues, blockIssue(fmt.Sprintf("operations[%d]", index), "encode add_symbol for composition offset: "+err.Error()))
				continue
			}
			offset[index] = updated
		case transactions.OpPlaceFootprint:
			var payload transactions.PlaceFootprintOperation
			if err := decodeBlockOperation(operation, &payload); err != nil {
				issues = append(issues, blockIssue(fmt.Sprintf("operations[%d]", index), "decode place_footprint for composition offset: "+err.Error()))
				continue
			}
			payload.At.XMM += xOffsetMM
			payload.At.YMM += yOffsetMM
			updated, err := wrapOperation(transactions.OpPlaceFootprint, payload)
			if err != nil {
				issues = append(issues, blockIssue(fmt.Sprintf("operations[%d]", index), "encode place_footprint for composition offset: "+err.Error()))
				continue
			}
			offset[index] = updated
		case transactions.OpRoute:
			var payload transactions.RouteOperation
			if err := decodeBlockOperation(operation, &payload); err != nil {
				issues = append(issues, blockIssue(fmt.Sprintf("operations[%d]", index), "decode route for composition offset: "+err.Error()))
				continue
			}
			offsetPoints(payload.Points, xOffsetMM, yOffsetMM)
			updated, err := wrapOperation(transactions.OpRoute, payload)
			if err != nil {
				issues = append(issues, blockIssue(fmt.Sprintf("operations[%d]", index), "encode route for composition offset: "+err.Error()))
				continue
			}
			offset[index] = updated
		case transactions.OpAddZone:
			var payload transactions.AddZoneOperation
			if err := decodeBlockOperation(operation, &payload); err != nil {
				issues = append(issues, blockIssue(fmt.Sprintf("operations[%d]", index), "decode add_zone for composition offset: "+err.Error()))
				continue
			}
			offsetPoints(payload.Polygon, xOffsetMM, yOffsetMM)
			updated, err := wrapOperation(transactions.OpAddZone, payload)
			if err != nil {
				issues = append(issues, blockIssue(fmt.Sprintf("operations[%d]", index), "encode add_zone for composition offset: "+err.Error()))
				continue
			}
			offset[index] = updated
		case transactions.OpConnect:
			// Connect operations are logical ref/pin endpoints. The schematic
			// writer derives wire geometry from the already-offset symbol anchors.
		}
	}
	return offset, issues
}

func offsetPoints(points []transactions.Point, xOffsetMM float64, yOffsetMM float64) {
	for i := range points {
		points[i].XMM += xOffsetMM
		points[i].YMM += yOffsetMM
	}
}

func compositionContextIssue(ctx context.Context) *reports.Issue {
	if ctx == nil {
		issue := reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Path: "composition", Message: "context is required"}
		return &issue
	}
	if err := ctx.Err(); err != nil {
		issue := reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Path: "composition", Message: err.Error()}
		return &issue
	}
	return nil
}

type portDisjointSet struct {
	parent map[PortRef]PortRef
}

func newPortDisjointSet() portDisjointSet {
	return portDisjointSet{parent: map[PortRef]PortRef{}}
}

func (set portDisjointSet) union(a PortRef, b PortRef) {
	rootA := set.find(a)
	rootB := set.find(b)
	if rootA == rootB {
		return
	}
	if portRefLess(rootB, rootA) {
		rootA, rootB = rootB, rootA
	}
	set.parent[rootB] = rootA
}

func (set portDisjointSet) find(value PortRef) PortRef {
	parent, ok := set.parent[value]
	if !ok {
		set.parent[value] = value
		return value
	}
	root := value
	for {
		parent = set.parent[root]
		if parent == root {
			break
		}
		root = parent
	}
	for value != root {
		next := set.parent[value]
		set.parent[value] = root
		value = next
	}
	return root
}

func (set portDisjointSet) netName(ref PortRef) string {
	root := set.find(ref)
	return sanitizeNetPart(root.InstanceID + "_" + root.Port)
}

func portRefLess(a PortRef, b PortRef) bool {
	if a.InstanceID != b.InstanceID {
		return a.InstanceID < b.InstanceID
	}
	return a.Port < b.Port
}

func portMap(instanceID string, ports []BlockPort) (map[string]BlockPort, []reports.Issue) {
	result := map[string]BlockPort{}
	var issues []reports.Issue
	for _, port := range ports {
		if _, exists := result[port.Name]; exists {
			issues = append(issues, blockIssue("instances."+instanceID+".ports."+port.Name, "duplicate port name "+port.Name))
			continue
		}
		result[port.Name] = port
	}
	return result, issues
}

func lookupPort(portsByInstance map[string]map[string]BlockPort, ref PortRef) (BlockPort, bool) {
	ports, ok := portsByInstance[ref.InstanceID]
	if !ok {
		return BlockPort{}, false
	}
	port, ok := ports[ref.Port]
	return port, ok
}

func compatiblePortDirections(a PortDirection, b PortDirection) bool {
	if a == "" || b == "" || a == PortPassive || b == PortPassive || a == PortBidirectional || b == PortBidirectional {
		return true
	}
	if a == PortPower && b == PortPower {
		return true
	}
	if (a == PortInput && b == PortInput) || (a == PortPower && b == PortInput) || (a == PortInput && b == PortPower) {
		return true
	}
	return (a == PortInput && b == PortOutput) || (a == PortOutput && b == PortInput)
}

func validateI2CAddressCollisions(instances []BlockInstance, netGroups portDisjointSet) []reports.Issue {
	type addressedInstance struct {
		instance BlockInstance
		address  string
	}
	var sensors []addressedInstance
	for _, instance := range instances {
		address := i2cAddressKey(instance.Params["i2c_address"])
		if address != "" && hasI2CBusPorts(instance) {
			sensors = append(sensors, addressedInstance{instance: instance, address: address})
		}
	}
	var issues []reports.Issue
	for left := 0; left < len(sensors); left++ {
		for right := left + 1; right < len(sensors); right++ {
			if sensors[left].address != sensors[right].address {
				continue
			}
			if sameI2CBus(sensors[left].instance, sensors[right].instance, netGroups) {
				issues = append(issues, i2cAddressCollisionIssue(sensors[left].instance, sensors[right].instance, sensors[left].address))
			}
		}
	}
	return issues
}

func hasI2CBusPorts(instance BlockInstance) bool {
	hasSDA := false
	hasSCL := false
	for _, port := range instance.Ports {
		hasSDA = hasSDA || port.Name == "SDA"
		hasSCL = hasSCL || port.Name == "SCL"
	}
	return hasSDA && hasSCL
}
