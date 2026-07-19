// Package compositionlowering converts a selected open-set architecture into
// the existing function-level circuit graph without introducing KiCad details.
package compositionlowering

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"unicode"

	"kicadai/internal/architecturesearch"
	"kicadai/internal/circuitgraph"
	"kicadai/internal/reports"
)

const (
	EvidenceSchema = "kicadai.composition-lowering-evidence.v1"
	PolicyVersion  = "composition-lowering-policy-v1"

	CodeLoweringInvalid reports.Code = "COMPOSITION_LOWERING_INVALID"
)

type Evidence struct {
	Schema               string   `json:"schema"`
	PolicyVersion        string   `json:"policy_version"`
	RequirementHash      string   `json:"requirement_hash"`
	RegistryHash         string   `json:"registry_hash"`
	CatalogHash          string   `json:"catalog_hash"`
	FormulaLibraryHash   string   `json:"formula_library_hash"`
	CandidateFingerprint string   `json:"candidate_fingerprint"`
	Selections           []string `json:"selections"`
}

type Result struct {
	Document circuitgraph.Document `json:"document"`
	Evidence Evidence              `json:"evidence"`
}

type nodeMetadata struct {
	role    circuitgraph.NetRole
	domain  string
	current float64
}

// Lower is deterministic and fail closed: every selected payload must decode,
// every role binding must resolve, and every semantic component port must join
// at least one other physical or external endpoint.
func Lower(requirement architecturesearch.Requirement, search architecturesearch.SearchResult) (Result, []reports.Issue) {
	if search.Status != architecturesearch.SearchSelected || search.Selected == nil {
		return Result{}, issues("search", "composition lowering requires one selected architecture")
	}
	requirement = architecturesearch.Normalize(requirement)
	if validation := architecturesearch.Validate(requirement); len(validation) != 0 {
		return Result{}, issues("requirement", "composition lowering requires a valid normalized requirement")
	}

	intent := circuitgraph.FunctionIntent{
		Constraints: circuitgraph.SynthesisConstraints{
			MaxWidthMM:                  requirement.Requirements.Constraints.MaxWidthMM,
			MaxHeightMM:                 requirement.Requirements.Constraints.MaxHeightMM,
			PreferredComponentSpacingMM: 1,
			Protection:                  protectionPolicy(search.Selected.Selections),
		},
	}
	for _, domain := range requirement.Requirements.Domains {
		intent.PowerDomains = append(intent.PowerDomains, lowerDomain(domain))
	}

	union := newDisjointSet()
	actual := map[string]circuitgraph.FunctionalEndpoint{}
	metadata := map[string]nodeMetadata{}
	interfaces, externalNodes := lowerInterfaces(requirement, union, actual, metadata)
	intent.Interfaces = interfaces

	selections := append([]architecturesearch.FragmentSelection(nil), search.Selected.Selections...)
	slices.SortStableFunc(selections, func(left, right architecturesearch.FragmentSelection) int {
		return strings.Compare(left.ObligationPath, right.ObligationPath)
	})
	selectionEvidence := make([]string, 0, len(selections))
	instanceIDs := map[string]bool{}
	for selectionIndex, selection := range selections {
		realization, err := architecturesearch.DecodeFragmentRealization(selection.Payload)
		if err != nil {
			return Result{}, issues(fmt.Sprintf("selections[%d].payload", selectionIndex), err.Error())
		}
		if realization.Capability != selection.Capability {
			return Result{}, issues(fmt.Sprintf("selections[%d].payload.capability", selectionIndex), "payload capability does not match its selected obligation")
		}
		prefix := safeID(selection.ObligationPath)
		selectionEvidence = append(selectionEvidence, selection.ObligationPath+"="+selection.ProviderID+"@"+selection.ProviderRevision+":"+selection.ExpansionID)
		localIDs := map[string]string{}
		for _, instance := range realization.Instances {
			id := safeID(prefix + "__" + instance.ID)
			if instanceIDs[id] {
				return Result{}, issues(fmt.Sprintf("selections[%d].instances", selectionIndex), "namespaced instance id is duplicated")
			}
			instanceIDs[id] = true
			localIDs[instance.ID] = id
			intent.Functions = append(intent.Functions, circuitgraph.FunctionRequirement{
				ID: id, Role: componentRole(instance.CatalogID, instance.Usage), ComponentID: instance.CatalogID,
				Value: instance.Value, RequiredFunctions: append([]string(nil), instance.RequiredFunctions...), Usage: instance.Usage,
			})
			for _, function := range instance.RequiredFunctions {
				node := functionNode(id, function)
				union.add(node)
				actual[node] = circuitgraph.FunctionalEndpoint{Function: id, Port: function}
			}
		}
		for _, connection := range realization.Connections {
			var first string
			for _, endpoint := range connection.Endpoints {
				id := localIDs[endpoint.Instance]
				node := functionNode(id, endpoint.Function)
				if first == "" {
					first = node
				} else {
					union.join(first, node)
				}
				mergeMetadata(metadata, node, nodeMetadata{role: lowerNetRole(connection.Role)})
			}
		}
		ports := map[string]architecturesearch.RoleContract{}
		for _, port := range selection.Ports {
			ports[port.Role] = port
		}
		for bindingIndex, binding := range realization.PortBindings {
			port, ok := ports[binding.Role]
			if !ok || port.Anchor == "" {
				return Result{}, issues(fmt.Sprintf("selections[%d].port_bindings[%d]", selectionIndex, bindingIndex), "binding role has no selected obligation anchor")
			}
			id := localIDs[binding.Instance]
			function := functionNode(id, binding.Function)
			anchor := anchorNode(port.Anchor, binding.Lane)
			union.join(function, anchor)
			mergeMetadata(metadata, anchor, nodeMetadata{role: contractNetRole(port.Contract), domain: port.Contract.Domain, current: contractCurrentMA(port.Contract)})
		}
	}

	for _, port := range requirement.Requirements.Ports {
		if port.Kind != "power" && port.Kind != "reference" {
			continue
		}
		external := anchorNode("external:"+port.ID, "")
		domain := anchorNode("domain:"+port.Domain, "")
		union.join(external, domain)
		if _, ok := externalNodes[port.ID]; !ok {
			return Result{}, issues("requirements.ports."+port.ID, "external power or reference port was not lowered")
		}
	}

	connections, connectionIssues := lowerConnections(union, actual, metadata)
	if len(connectionIssues) != 0 {
		return Result{}, connectionIssues
	}
	intent.Connections = connections
	slices.SortStableFunc(intent.Functions, func(left, right circuitgraph.FunctionRequirement) int { return strings.Compare(left.ID, right.ID) })

	document := circuitgraph.Document{
		Schema: circuitgraph.SchemaID, Version: circuitgraph.Version,
		Project:   circuitgraph.Project{Name: requirement.Project.Name, Title: requirement.Project.Title, Description: requirement.Project.Description, Acceptance: acceptance(requirement.Acceptance)},
		Synthesis: &intent,
		Policy: circuitgraph.Policy{
			AllowReferenceAssignment: boolPointer(true), AllowValueNormalization: boolPointer(true), AllowLayoutInference: boolPointer(true),
			AllowSpacingAdjustment: boolPointer(true), AllowLabelInsertion: boolPointer(true), AllowPlacementAdjustment: boolPointer(true), AllowRouteRetry: boolPointer(false),
		},
	}
	if validation := circuitgraph.Validate(document); len(validation) != 0 {
		return Result{}, validation
	}
	evidence := Evidence{
		Schema: EvidenceSchema, PolicyVersion: PolicyVersion,
		RequirementHash: search.RequirementHash, RegistryHash: search.RegistryHash, CatalogHash: search.CatalogHash,
		FormulaLibraryHash: search.FormulaLibraryHash, CandidateFingerprint: search.Selected.Score.Fingerprint,
		Selections: selectionEvidence,
	}
	return Result{Document: document, Evidence: evidence}, nil
}

func lowerDomain(domain architecturesearch.Domain) circuitgraph.PowerDomainIntent {
	role := circuitgraph.NetRolePower
	if domain.Kind == "reference" {
		role = circuitgraph.NetRoleGround
	}
	source := circuitgraph.PowerDomainExternal
	if domain.Source == "generated" {
		source = circuitgraph.PowerDomainGenerated
	}
	current := 0.0
	if domain.MaxCurrentA != nil {
		current = *domain.MaxCurrentA * 1000
	}
	return circuitgraph.PowerDomainIntent{Name: domain.ID, Role: role, VoltageV: domain.NominalVoltageV, MaxCurrentMA: current, Source: source}
}

func lowerInterfaces(requirement architecturesearch.Requirement, union *disjointSet, actual map[string]circuitgraph.FunctionalEndpoint, metadata map[string]nodeMetadata) ([]circuitgraph.InterfaceRequirement, map[string]string) {
	result := make([]circuitgraph.InterfaceRequirement, 0, len(requirement.Requirements.Ports))
	nodes := map[string]string{}
	referenceDomain := ""
	for _, domain := range requirement.Requirements.Domains {
		if domain.Kind == "reference" {
			referenceDomain = domain.ID
			break
		}
	}
	for _, port := range requirement.Requirements.Ports {
		primaryRole := portNetRole(port)
		candidate := circuitgraph.InterfaceRequirement{ID: port.ID, Role: interfaceRole(port)}
		lanes := []string{"", "return"}
		signals := []circuitgraph.InterfaceSignal{{Name: interfaceSignalName(port), Role: primaryRole}, {Name: "return", Role: circuitgraph.NetRoleGround}}
		if primaryRole == circuitgraph.NetRoleGround {
			lanes = []string{""}
			signals = signals[:1]
		}
		if port.Kind == "digital_bus" && port.Protocol != nil && port.Protocol.Name == "i2c" {
			lanes = []string{"sda", "scl"}
			signals = []circuitgraph.InterfaceSignal{{Name: "sda", Role: circuitgraph.NetRoleSignal}, {Name: "scl", Role: circuitgraph.NetRoleSignal}}
		}
		candidate.Signals = signals
		result = append(result, candidate)
		for index, lane := range lanes {
			node := interfaceNode(port.ID, signals[index].Name)
			anchor := anchorNode("external:"+port.ID, lane)
			domain := port.Domain
			role := primaryRole
			if lane == "return" {
				anchor = anchorNode("domain:"+referenceDomain, "")
				domain = referenceDomain
				role = circuitgraph.NetRoleGround
			}
			union.join(node, anchor)
			actual[node] = circuitgraph.FunctionalEndpoint{Interface: port.ID, Signal: signals[index].Name}
			mergeMetadata(metadata, node, nodeMetadata{role: role, domain: domain, current: portCurrentMA(port)})
			if lane == "" {
				nodes[port.ID] = node
			}
		}
	}
	return result, nodes
}

func lowerConnections(union *disjointSet, actual map[string]circuitgraph.FunctionalEndpoint, metadata map[string]nodeMetadata) ([]circuitgraph.FunctionConnection, []reports.Issue) {
	groups := map[string][]string{}
	for node := range actual {
		root := union.find(node)
		groups[root] = append(groups[root], node)
	}
	roots := make([]string, 0, len(groups))
	for root := range groups {
		roots = append(roots, root)
	}
	slices.Sort(roots)
	connections := make([]circuitgraph.FunctionConnection, 0, len(roots))
	for index, root := range roots {
		nodes := groups[root]
		slices.Sort(nodes)
		if len(nodes) < 2 {
			return nil, issues(nodes[0], "semantic endpoint is disconnected after composition")
		}
		combined := nodeMetadata{role: circuitgraph.NetRoleSignal}
		for node := range union.members(root) {
			combined = combineMetadata(combined, metadata[node])
		}
		connection := circuitgraph.FunctionConnection{Name: fmt.Sprintf("composition_net_%03d", index+1), Role: combined.role, VoltageDomain: combined.domain, CurrentMA: combined.current}
		for _, node := range nodes {
			connection.Endpoints = append(connection.Endpoints, actual[node])
		}
		connections = append(connections, connection)
	}
	return connections, nil
}

func interfaceRole(port architecturesearch.Port) circuitgraph.InterfaceRole {
	switch port.Kind {
	case "power", "reference":
		if port.Direction == "source" {
			return circuitgraph.InterfacePowerOutput
		}
		return circuitgraph.InterfacePowerInput
	case "analog_voltage":
		if port.Direction == "source" {
			return circuitgraph.InterfaceAnalogOut
		}
		return circuitgraph.InterfaceAnalogInput
	case "digital_bus":
		if port.Protocol != nil && port.Protocol.Name == "i2c" {
			return circuitgraph.InterfaceI2C
		}
		return circuitgraph.InterfaceGPIO
	default:
		if port.Direction == "source" {
			return circuitgraph.InterfaceDigitalOut
		}
		return circuitgraph.InterfaceDigitalIn
	}
}

func interfaceSignalName(port architecturesearch.Port) string {
	if port.Kind == "reference" {
		return "ground"
	}
	if port.Kind == "power" {
		return "power"
	}
	return "signal"
}

func portNetRole(port architecturesearch.Port) circuitgraph.NetRole {
	if port.Kind == "reference" {
		return circuitgraph.NetRoleGround
	}
	if port.Kind == "power" {
		return circuitgraph.NetRolePower
	}
	return circuitgraph.NetRoleSignal
}

func contractNetRole(contract architecturesearch.PortContract) circuitgraph.NetRole {
	if contract.Kind == "reference" {
		return circuitgraph.NetRoleGround
	}
	if contract.Kind == "power" || contract.Kind == "switched_load" {
		return circuitgraph.NetRolePower
	}
	return circuitgraph.NetRoleSignal
}

func lowerNetRole(role string) circuitgraph.NetRole {
	switch role {
	case "power", "switched_power":
		return circuitgraph.NetRolePower
	case "reference":
		return circuitgraph.NetRoleGround
	case "feedback":
		return circuitgraph.NetRoleFeedback
	default:
		return circuitgraph.NetRoleSignal
	}
}

func componentRole(catalogID, usage string) circuitgraph.ComponentRole {
	family := strings.SplitN(catalogID, ".", 2)[0]
	switch family {
	case "resistor":
		if strings.Contains(usage, "pullup") {
			return circuitgraph.RolePullup
		}
		return circuitgraph.RoleResistor
	case "capacitor":
		if strings.Contains(usage, "decoupl") {
			return circuitgraph.RoleDecouplingCapacitor
		}
		return circuitgraph.RoleCapacitor
	case "diode":
		return circuitgraph.RoleDiode
	case "mosfet":
		return circuitgraph.RoleMOSFET
	case "regulator":
		return circuitgraph.RoleRegulator
	case "sensor":
		return circuitgraph.RoleSensor
	case "mcu", "opamp", "comparator", "level_translator":
		return circuitgraph.RoleIC
	default:
		return circuitgraph.RoleGeneric
	}
}

func protectionPolicy(selections []architecturesearch.FragmentSelection) string {
	for _, selection := range selections {
		if strings.Contains(selection.Capability, "protected") || selection.Capability == "load_switch" {
			return "required"
		}
	}
	return "optional"
}

func acceptance(candidate architecturesearch.Acceptance) circuitgraph.AcceptanceLevel {
	if candidate.RequireERC || candidate.RequireStrictDRC {
		return circuitgraph.AcceptanceERCDRC
	}
	if candidate.RequireConnectivity || candidate.RequireCompleteRouting {
		return circuitgraph.AcceptanceConnectivity
	}
	return circuitgraph.AcceptanceStructural
}

func portCurrentMA(port architecturesearch.Port) float64 {
	if port.Electrical != nil && port.Electrical.MaxCurrentA != nil {
		return *port.Electrical.MaxCurrentA * 1000
	}
	return 0
}

func contractCurrentMA(contract architecturesearch.PortContract) float64 {
	if contract.MaximumCurrentDemandA != nil {
		return *contract.MaximumCurrentDemandA * 1000
	}
	if contract.RequiredCurrentCapacityA != nil {
		return *contract.RequiredCurrentCapacityA * 1000
	}
	return 0
}

func mergeMetadata(values map[string]nodeMetadata, node string, candidate nodeMetadata) {
	values[node] = combineMetadata(values[node], candidate)
}

func combineMetadata(left, right nodeMetadata) nodeMetadata {
	result := left
	if netRoleRank(right.role) > netRoleRank(result.role) {
		result.role = right.role
	}
	if result.domain == "" {
		result.domain = right.domain
	}
	if right.current > result.current {
		result.current = right.current
	}
	return result
}

func netRoleRank(role circuitgraph.NetRole) int {
	switch role {
	case circuitgraph.NetRoleGround:
		return 3
	case circuitgraph.NetRolePower:
		return 2
	case circuitgraph.NetRoleFeedback:
		return 1
	default:
		return 0
	}
}

func functionNode(instance, function string) string {
	return "function:" + instance + ":" + strings.ToUpper(strings.TrimSpace(function))
}
func interfaceNode(id, signal string) string { return "interface:" + id + ":" + signal }
func anchorNode(anchor, lane string) string {
	if lane != "" {
		return "anchor:" + anchor + ":" + lane
	}
	return "anchor:" + anchor
}

func safeID(value string) string {
	var builder strings.Builder
	for _, candidate := range strings.ToLower(strings.TrimSpace(value)) {
		if unicode.IsLetter(candidate) || unicode.IsDigit(candidate) || candidate == '_' {
			builder.WriteRune(candidate)
		} else {
			builder.WriteByte('_')
		}
	}
	return strings.Trim(builder.String(), "_")
}

func issues(path, message string) []reports.Issue {
	return []reports.Issue{{Code: CodeLoweringInvalid, Severity: reports.SeverityError, Path: path, Message: message}}
}

func boolPointer(value bool) *bool { return &value }

type disjointSet struct{ parent map[string]string }

func newDisjointSet() *disjointSet { return &disjointSet{parent: map[string]string{}} }
func (set *disjointSet) add(value string) {
	if _, ok := set.parent[value]; !ok {
		set.parent[value] = value
	}
}
func (set *disjointSet) find(value string) string {
	set.add(value)
	if set.parent[value] != value {
		set.parent[value] = set.find(set.parent[value])
	}
	return set.parent[value]
}
func (set *disjointSet) join(left, right string) {
	l, r := set.find(left), set.find(right)
	if l == r {
		return
	}
	if l < r {
		set.parent[r] = l
	} else {
		set.parent[l] = r
	}
}
func (set *disjointSet) members(root string) map[string]bool {
	result := map[string]bool{}
	for node := range set.parent {
		if set.find(node) == root {
			result[node] = true
		}
	}
	return result
}

func MarshalEvidence(evidence Evidence) (json.RawMessage, error) { return json.Marshal(evidence) }
