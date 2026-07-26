package architecturesearch

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strings"

	"kicadai/internal/reports"
)

const (
	SystemPlanSchema                      = "kicadai.hierarchical-system-plan.v1"
	BacktrackingSchema                    = "kicadai.architecture-backtracking.v1"
	CodeHierarchyUnproven    reports.Code = "ARCHITECTURE_HIERARCHY_UNPROVEN"
	CodeResourcePlanUnproven reports.Code = "ARCHITECTURE_RESOURCE_PLAN_UNPROVEN"
	CodePhysicalPlanUnproven reports.Code = "ARCHITECTURE_PHYSICAL_PLAN_UNPROVEN"
	CodeTraceabilityUnproven reports.Code = "ARCHITECTURE_TRACEABILITY_UNPROVEN"
)

// SystemPlan is generated from a complete selected architecture. No field in
// this model is accepted from a requirement fixture.
type SystemPlan struct {
	Schema               string                     `json:"schema"`
	RequirementHash      string                     `json:"requirement_hash"`
	CandidateFingerprint string                     `json:"candidate_fingerprint"`
	Hierarchy            HierarchyPlan              `json:"hierarchy"`
	Interfaces           []InterfaceContractPlan    `json:"interfaces"`
	Resources            []SharedResourcePlan       `json:"resources"`
	Physical             PhysicalPlan               `json:"physical"`
	Traceability         []SystemTraceabilityRecord `json:"traceability"`
	PlanHash             string                     `json:"plan_hash,omitempty"`
}

type HierarchyPlan struct {
	Root       HierarchyNode    `json:"root"`
	Subsystems []HierarchyNode  `json:"subsystems"`
	Blocks     []HierarchyBlock `json:"blocks"`
}

type HierarchyNode struct {
	ID              string   `json:"id"`
	Kind            string   `json:"kind"`
	ParentID        string   `json:"parent_id,omitempty"`
	Children        []string `json:"children,omitempty"`
	Domains         []string `json:"domains,omitempty"`
	Classifications []string `json:"classifications,omitempty"`
}

type HierarchyBlock struct {
	ID                  string   `json:"id"`
	ParentID            string   `json:"parent_id"`
	ParentBlockID       string   `json:"parent_block_id,omitempty"`
	ObligationPath      string   `json:"obligation_path"`
	Capability          string   `json:"capability"`
	RequirementKind     string   `json:"requirement_kind,omitempty"`
	RequirementID       string   `json:"requirement_id,omitempty"`
	Domains             []string `json:"domains"`
	Classifications     []string `json:"classifications"`
	InterfaceIDs        []string `json:"interface_ids,omitempty"`
	ComponentIDs        []string `json:"component_ids,omitempty"`
	RequiredBehaviorIDs []string `json:"required_behavior_ids,omitempty"`
	VerificationIDs     []string `json:"verification_ids"`
}

type InterfaceEndpoint struct {
	BlockID        string `json:"block_id"`
	ObligationPath string `json:"obligation_path,omitempty"`
	Role           string `json:"role"`
	Direction      string `json:"direction"`
}

type InterfaceContractPlan struct {
	ID                   string              `json:"id"`
	Anchor               string              `json:"anchor"`
	Kind                 string              `json:"kind"`
	Domain               string              `json:"domain"`
	Endpoints            []InterfaceEndpoint `json:"endpoints"`
	Voltage              NumericRange        `json:"voltage,omitempty"`
	CurrentCapacityA     *float64            `json:"current_capacity_a,omitempty"`
	CurrentDemandA       *float64            `json:"current_demand_a,omitempty"`
	CurrentMarginA       *float64            `json:"current_margin_a,omitempty"`
	InputImpedanceMinOhm *float64            `json:"input_impedance_min_ohm,omitempty"`
	FrequencyMaxHz       *float64            `json:"frequency_max_hz,omitempty"`
	Protocol             *Protocol           `json:"protocol,omitempty"`
	DefaultState         string              `json:"default_state,omitempty"`
	Traits               []string            `json:"traits,omitempty"`
	Evidence             ContractEvidence    `json:"evidence"`
	Checks               []ContractCheck     `json:"checks,omitempty"`
	Status               string              `json:"status"`
}

type SharedResourcePlan struct {
	ID           string           `json:"id"`
	Kind         string           `json:"kind"`
	Domain       string           `json:"domain,omitempty"`
	Source       string           `json:"source"`
	Consumers    []string         `json:"consumers"`
	CapacityA    *float64         `json:"capacity_a,omitempty"`
	DemandA      *float64         `json:"demand_a,omitempty"`
	MarginA      *float64         `json:"margin_a,omitempty"`
	Dependencies []string         `json:"dependencies,omitempty"`
	Evidence     ContractEvidence `json:"evidence"`
	Status       string           `json:"status"`
}

type PhysicalPlan struct {
	Partitions []PhysicalPartition `json:"partitions"`
	Boundaries []PhysicalBoundary  `json:"boundaries,omitempty"`
}

type PhysicalPartition struct {
	ID              string   `json:"id"`
	SubsystemID     string   `json:"subsystem_id"`
	BlockIDs        []string `json:"block_ids"`
	Classifications []string `json:"classifications"`
	Rules           []string `json:"rules"`
}

type PhysicalBoundary struct {
	InterfaceID     string   `json:"interface_id"`
	Partitions      []string `json:"partitions"`
	Classifications []string `json:"classifications"`
	Rules           []string `json:"rules"`
}

type SystemTraceabilityRecord struct {
	RequirementKind          string   `json:"requirement_kind"`
	RequirementID            string   `json:"requirement_id"`
	BlockIDs                 []string `json:"block_ids"`
	InterfaceIDs             []string `json:"interface_ids,omitempty"`
	ResourceIDs              []string `json:"resource_ids,omitempty"`
	BehavioralRequirementIDs []string `json:"behavioral_requirement_ids,omitempty"`
}

type BacktrackingEvidence struct {
	Schema         string   `json:"schema"`
	Strategy       string   `json:"strategy"`
	CandidateOrder []string `json:"candidate_order"`
	Deterministic  bool     `json:"deterministic"`
}

type planEndpoint struct {
	InterfaceEndpoint
	Contract PortContract
}

func BuildSystemPlan(requirement Requirement, candidate CandidateResult) (SystemPlan, *candidateValidation) {
	if !supportsSystemPlanning(requirement) {
		return SystemPlan{}, nil
	}
	requirementHash, err := CanonicalHash(requirement)
	if err != nil {
		return SystemPlan{}, &candidateValidation{
			Code: CodeHierarchyUnproven, Path: "candidate.system_plan",
			Message: "hash normalized hierarchical requirement: " + err.Error(),
		}
	}
	plan := SystemPlan{
		Schema: SystemPlanSchema, RequirementHash: requirementHash,
		CandidateFingerprint: candidate.Fingerprint,
		Hierarchy:            HierarchyPlan{Root: HierarchyNode{ID: "system", Kind: "system"}},
	}
	pathToBlock := map[string]string{}
	rootToBlock := map[string]string{}
	for _, selection := range candidate.Selections {
		root := rootObligationPath(selection.ObligationPath)
		if selection.ObligationPath == root {
			rootToBlock[root] = topLevelBlockID(root)
		}
	}
	for _, selection := range candidate.Selections {
		root := rootObligationPath(selection.ObligationPath)
		parentBlockID := rootToBlock[root]
		if parentBlockID == "" {
			return SystemPlan{}, &candidateValidation{
				Code: CodeHierarchyUnproven, Path: "candidate.system_plan.hierarchy",
				Message: "provider child has no selected top-level owner: " + selection.ObligationPath,
			}
		}
		blockID := parentBlockID
		if selection.ObligationPath != root {
			blockID = "block_child_" + shortPlanHash(selection.ObligationPath)
		}
		pathToBlock[selection.ObligationPath] = blockID
		domains := selectionDomains(selection)
		classes := selectionPhysicalClasses(requirement, selection)
		requirementKind, requirementID := splitRootObligation(root)
		block := HierarchyBlock{
			ID: blockID, ObligationPath: selection.ObligationPath, Capability: selection.Capability,
			RequirementKind: requirementKind, RequirementID: requirementID,
			Domains: domains, Classifications: classes,
		}
		if selection.ObligationPath != root {
			block.ParentBlockID = parentBlockID
			block.RequirementKind = ""
			block.RequirementID = ""
		}
		for _, component := range selection.Components {
			block.ComponentIDs = append(block.ComponentIDs, component.InstanceID)
		}
		slices.Sort(block.ComponentIDs)
		plan.Hierarchy.Blocks = append(plan.Hierarchy.Blocks, block)
	}
	slices.SortStableFunc(plan.Hierarchy.Blocks, func(left, right HierarchyBlock) int {
		return strings.Compare(left.ID, right.ID)
	})
	if validation := assignSubsystems(requirement, &plan.Hierarchy); validation != nil {
		return SystemPlan{}, validation
	}
	interfaces, interfaceValidation := buildInterfaceContracts(requirement, candidate.Selections, pathToBlock)
	if interfaceValidation != nil {
		return SystemPlan{}, interfaceValidation
	}
	plan.Interfaces = interfaces
	attachBlockInterfaces(&plan.Hierarchy, interfaces)
	if validation := attachBlockVerification(requirement, &plan.Hierarchy); validation != nil {
		return SystemPlan{}, validation
	}
	resources, resourceValidation := buildSharedResourcePlan(requirement, plan.Hierarchy.Blocks, interfaces)
	if resourceValidation != nil {
		return SystemPlan{}, resourceValidation
	}
	plan.Resources = resources
	physical, physicalValidation := buildPhysicalPlan(plan.Hierarchy, interfaces)
	if physicalValidation != nil {
		return SystemPlan{}, physicalValidation
	}
	plan.Physical = physical
	traceability, traceValidation := buildSystemTraceability(requirement, plan.Hierarchy.Blocks, interfaces, resources)
	if traceValidation != nil {
		return SystemPlan{}, traceValidation
	}
	plan.Traceability = traceability
	encoded, err := json.Marshal(plan)
	if err != nil {
		return SystemPlan{}, &candidateValidation{
			Code: CodeHierarchyUnproven, Path: "candidate.system_plan",
			Message: "marshal generated system plan: " + err.Error(),
		}
	}
	digest := sha256.Sum256(encoded)
	plan.PlanHash = hex.EncodeToString(digest[:])
	if err := ValidateSystemPlan(requirement, candidate.Fingerprint, plan); err != nil {
		return SystemPlan{}, &candidateValidation{
			Code: CodeHierarchyUnproven, Path: "candidate.system_plan",
			Message: err.Error(),
		}
	}
	return plan, nil
}

// ValidateSystemPlan verifies generated V4/V5 evidence at every persistence or
// orchestration boundary. It intentionally accepts no repair or inference.
func ValidateSystemPlan(requirement Requirement, candidateFingerprint string, plan SystemPlan) error {
	if !supportsSystemPlanning(requirement) {
		return fmt.Errorf("system-plan validation requires a V4 or V5 requirement")
	}
	requirementHash, err := CanonicalHash(requirement)
	if err != nil {
		return fmt.Errorf("hash hierarchical requirement: %w", err)
	}
	if plan.Schema != SystemPlanSchema || plan.RequirementHash != requirementHash ||
		plan.CandidateFingerprint != candidateFingerprint || plan.PlanHash == "" {
		return fmt.Errorf("system-plan identity does not match its requirement and candidate")
	}
	hashInput := plan
	hashInput.PlanHash = ""
	encoded, err := json.Marshal(hashInput)
	if err != nil {
		return fmt.Errorf("marshal system-plan hash input: %w", err)
	}
	digest := sha256.Sum256(encoded)
	if hex.EncodeToString(digest[:]) != plan.PlanHash {
		return fmt.Errorf("system-plan hash does not match canonical contents")
	}
	if plan.Hierarchy.Root.ID != "system" || plan.Hierarchy.Root.Kind != "system" ||
		len(plan.Hierarchy.Subsystems) == 0 || len(plan.Hierarchy.Blocks) == 0 {
		return fmt.Errorf("system-plan hierarchy root, subsystems, or blocks are incomplete")
	}
	subsystems := map[string]bool{}
	for _, subsystem := range plan.Hierarchy.Subsystems {
		if subsystem.ID == "" || subsystem.Kind != "subsystem" ||
			subsystem.ParentID != plan.Hierarchy.Root.ID || subsystems[subsystem.ID] ||
			len(subsystem.Children) == 0 {
			return fmt.Errorf("system-plan contains an invalid or duplicate subsystem")
		}
		subsystems[subsystem.ID] = true
	}
	interfaces := map[string]bool{}
	for _, connection := range plan.Interfaces {
		if connection.ID == "" || interfaces[connection.ID] || connection.Status != "pass" ||
			connection.Anchor == "" || connection.Kind == "" || connection.Domain == "" ||
			len(connection.Endpoints) == 0 {
			return fmt.Errorf("system-plan contains an invalid or unproven interface")
		}
		interfaces[connection.ID] = true
	}
	blocks := map[string]bool{}
	partitionCount := map[string]int{}
	for _, block := range plan.Hierarchy.Blocks {
		if block.ID == "" || blocks[block.ID] {
			return fmt.Errorf("system-plan contains an empty or duplicate block identity %q", block.ID)
		}
		if !subsystems[block.ParentID] {
			return fmt.Errorf("system-plan block %s has unknown subsystem %s", block.ID, block.ParentID)
		}
		if block.ObligationPath == "" || block.Capability == "" {
			return fmt.Errorf("system-plan block %s lacks obligation ownership", block.ID)
		}
		if len(block.InterfaceIDs) == 0 {
			return fmt.Errorf("system-plan block %s lacks interface contracts", block.ID)
		}
		if len(block.VerificationIDs) == 0 || len(block.RequiredBehaviorIDs) == 0 {
			return fmt.Errorf("system-plan block %s lacks block-scoped verification", block.ID)
		}
		for _, interfaceID := range block.InterfaceIDs {
			if !interfaces[interfaceID] {
				return fmt.Errorf("system-plan block references an unknown interface")
			}
		}
		blocks[block.ID] = true
	}
	for _, partition := range plan.Physical.Partitions {
		if partition.ID == "" || !subsystems[partition.SubsystemID] ||
			len(partition.BlockIDs) == 0 || len(partition.Rules) == 0 {
			return fmt.Errorf("system-plan contains an invalid physical partition")
		}
		for _, blockID := range partition.BlockIDs {
			if !blocks[blockID] {
				return fmt.Errorf("physical partition references an unknown block")
			}
			partitionCount[blockID]++
		}
	}
	for blockID := range blocks {
		if partitionCount[blockID] != 1 {
			return fmt.Errorf("block %s must occur in exactly one physical partition", blockID)
		}
	}
	resourceIDs := map[string]bool{}
	for _, resource := range plan.Resources {
		if resource.ID == "" || resourceIDs[resource.ID] || resource.Source == "" || resource.Status != "pass" {
			return fmt.Errorf("system-plan contains an invalid or unproven shared resource")
		}
		resourceIDs[resource.ID] = true
	}
	traceKeys := map[string]bool{}
	for _, record := range plan.Traceability {
		key := record.RequirementKind + ":" + record.RequirementID
		if record.RequirementKind == "" || record.RequirementID == "" ||
			traceKeys[key] || len(record.BlockIDs) == 0 {
			return fmt.Errorf("system-plan contains invalid or duplicate traceability")
		}
		traceKeys[key] = true
		for _, blockID := range record.BlockIDs {
			if !blocks[blockID] {
				return fmt.Errorf("traceability references an unknown block")
			}
		}
	}
	for _, participant := range requirement.Requirements.Participants {
		if !traceKeys["participant:"+participant.ID] {
			return fmt.Errorf("participant %s lacks traceability", participant.ID)
		}
	}
	for _, objective := range requirement.Requirements.Objectives {
		if !traceKeys["objective:"+objective.ID] {
			return fmt.Errorf("objective %s lacks traceability", objective.ID)
		}
	}
	for _, behavior := range requirement.Requirements.BehavioralRequirements {
		if !traceKeys["behavior:"+behavior.ID] {
			return fmt.Errorf("behavior %s lacks end-to-end traceability", behavior.ID)
		}
	}
	return nil
}

func assignSubsystems(requirement Requirement, hierarchy *HierarchyPlan) *candidateValidation {
	groups := map[string][]int{}
	for index := range hierarchy.Blocks {
		block := &hierarchy.Blocks[index]
		group := subsystemGroup(block)
		groups[group] = append(groups[group], index)
	}
	groupIDs := make([]string, 0, len(groups))
	for group := range groups {
		groupIDs = append(groupIDs, group)
	}
	slices.Sort(groupIDs)
	for _, group := range groupIDs {
		indices := groups[group]
		subsystemID := "subsystem_" + derivedSemanticIdentifier(group)
		node := HierarchyNode{ID: subsystemID, Kind: "subsystem", ParentID: hierarchy.Root.ID}
		for _, index := range indices {
			block := &hierarchy.Blocks[index]
			block.ParentID = subsystemID
			node.Children = append(node.Children, block.ID)
			node.Domains = append(node.Domains, block.Domains...)
			node.Classifications = append(node.Classifications, block.Classifications...)
		}
		node.Children = sortedUniqueStrings(node.Children)
		node.Domains = sortedUniqueStrings(node.Domains)
		node.Classifications = sortedUniqueStrings(node.Classifications)
		hierarchy.Subsystems = append(hierarchy.Subsystems, node)
		hierarchy.Root.Children = append(hierarchy.Root.Children, subsystemID)
	}
	hierarchy.Root.Children = sortedUniqueStrings(hierarchy.Root.Children)
	hierarchy.Root.Domains = make([]string, 0, len(requirement.Requirements.Domains))
	for _, domain := range requirement.Requirements.Domains {
		hierarchy.Root.Domains = append(hierarchy.Root.Domains, domain.ID)
	}
	hierarchy.Root.Domains = sortedUniqueStrings(hierarchy.Root.Domains)
	if len(hierarchy.Subsystems) == 0 || len(hierarchy.Blocks) == 0 {
		return &candidateValidation{
			Code: CodeHierarchyUnproven, Path: "candidate.system_plan.hierarchy",
			Message: "generated hierarchy requires at least one subsystem and block",
		}
	}
	return nil
}

func subsystemGroup(block *HierarchyBlock) string {
	nonReference := append([]string(nil), block.Domains...)
	if len(nonReference) == 0 {
		return "reference"
	}
	if len(nonReference) == 1 {
		return "domain_" + nonReference[0]
	}
	return "boundary_" + shortPlanHash(strings.Join(nonReference, "\x00"))
}

func buildInterfaceContracts(requirement Requirement, selections []FragmentSelection, pathToBlock map[string]string) ([]InterfaceContractPlan, *candidateValidation) {
	byAnchor := map[string][]planEndpoint{}
	for _, selection := range selections {
		blockID := pathToBlock[selection.ObligationPath]
		for _, port := range selection.Ports {
			byAnchor[port.Anchor] = append(byAnchor[port.Anchor], planEndpoint{
				InterfaceEndpoint: InterfaceEndpoint{
					BlockID: blockID, ObligationPath: selection.ObligationPath,
					Role: port.Role, Direction: port.Contract.Direction,
				},
				Contract: port.Contract,
			})
		}
	}
	anchors := make([]string, 0, len(byAnchor))
	for anchor := range byAnchor {
		anchors = append(anchors, anchor)
	}
	slices.Sort(anchors)
	result := make([]InterfaceContractPlan, 0, len(anchors))
	for _, anchor := range anchors {
		endpoints := byAnchor[anchor]
		slices.SortStableFunc(endpoints, func(left, right planEndpoint) int {
			if order := strings.Compare(left.BlockID, right.BlockID); order != 0 {
				return order
			}
			if order := strings.Compare(left.ObligationPath, right.ObligationPath); order != 0 {
				return order
			}
			return strings.Compare(left.Role, right.Role)
		})
		if len(endpoints) == 0 {
			continue
		}
		contract := InterfaceContractPlan{
			ID: "interface_" + shortPlanHash(anchor), Anchor: anchor,
			Kind: endpoints[0].Contract.Kind, Domain: endpoints[0].Contract.Domain,
			Evidence: ContractEvidence{Confidence: EvidenceVerified, Sources: []string{"kicadai:selected-contract-composition"}},
			Status:   "pass",
		}
		contract.Voltage = intersectEndpointVoltage(endpoints)
		var capacity float64
		var demand float64
		hasCapacity := false
		hasDemand := false
		impedance := math.Inf(1)
		frequency := math.Inf(1)
		for _, endpoint := range endpoints {
			contract.Endpoints = append(contract.Endpoints, endpoint.InterfaceEndpoint)
			port := endpoint.Contract
			if port.Kind != contract.Kind || port.Domain != contract.Domain {
				return nil, &candidateValidation{
					Code: CodeHierarchyUnproven, Path: "candidate.system_plan.interfaces." + contract.ID,
					Message: "selected interface endpoints disagree on kind or domain",
				}
			}
			if port.CurrentCapacityA != nil {
				capacity += *port.CurrentCapacityA
				hasCapacity = true
			}
			if port.CurrentDemandA != nil {
				demand += *port.CurrentDemandA
				hasDemand = true
			}
			if port.InputImpedanceMinOhm != nil {
				impedance = math.Min(impedance, *port.InputImpedanceMinOhm)
			}
			if port.FrequencyMaxHz != nil {
				frequency = math.Min(frequency, *port.FrequencyMaxHz)
			}
			if contract.Protocol == nil && port.Protocol != nil {
				protocol := *port.Protocol
				contract.Protocol = &protocol
			}
			if contract.DefaultState == "" {
				contract.DefaultState = port.DefaultState
			}
			contract.Traits = append(contract.Traits, port.Traits...)
			contract.Traits = append(contract.Traits, port.RequiredTraits...)
		}
		if strings.HasPrefix(anchor, "external:") || strings.HasPrefix(anchor, "domain:") {
			contract.Endpoints = append(contract.Endpoints, InterfaceEndpoint{
				BlockID: "system", Role: "requirement_boundary",
				Direction: requirementBoundaryDirection(requirement, anchor),
			})
		}
		slices.SortStableFunc(contract.Endpoints, func(left, right InterfaceEndpoint) int {
			if order := strings.Compare(left.BlockID, right.BlockID); order != 0 {
				return order
			}
			if order := strings.Compare(left.ObligationPath, right.ObligationPath); order != 0 {
				return order
			}
			return strings.Compare(left.Role, right.Role)
		})
		if hasCapacity {
			contract.CurrentCapacityA = float64Pointer(capacity)
		}
		if hasDemand {
			contract.CurrentDemandA = float64Pointer(demand)
		}
		if hasCapacity && hasDemand {
			margin := capacity - demand
			if margin < 0 {
				return nil, &candidateValidation{
					Code: CodeResourcePlanUnproven, Path: "candidate.system_plan.interfaces." + contract.ID + ".current",
					Message: "generated interface demand exceeds selected source capacity",
				}
			}
			contract.CurrentMarginA = float64Pointer(margin)
		}
		if !math.IsInf(impedance, 1) {
			contract.InputImpedanceMinOhm = float64Pointer(impedance)
		}
		if !math.IsInf(frequency, 1) {
			contract.FrequencyMaxHz = float64Pointer(frequency)
		}
		contract.Traits = sortedUniqueStrings(contract.Traits)
		contract.Checks = endpointCompatibilityChecks(endpoints)
		for _, check := range contract.Checks {
			if check.Status == ContractCheckReject {
				return nil, &candidateValidation{
					Code: check.Code, Path: "candidate.system_plan.interfaces." + contract.ID + "." + check.Path,
					Message: check.Message,
				}
			}
		}
		result = append(result, contract)
	}
	return result, nil
}

func endpointCompatibilityChecks(endpoints []planEndpoint) []ContractCheck {
	var sources []PortContract
	var sinks []PortContract
	var bidirectional []PortContract
	for _, endpoint := range endpoints {
		switch endpoint.Contract.Direction {
		case "source":
			sources = append(sources, endpoint.Contract)
		case "sink":
			sinks = append(sinks, endpoint.Contract)
		case "bidirectional":
			bidirectional = append(bidirectional, endpoint.Contract)
		}
	}
	var checks []ContractCheck
	for _, source := range sources {
		for _, sink := range sinks {
			checks = append(checks, ConnectPorts(source, sink).Checks...)
		}
	}
	for left := 0; left < len(bidirectional); left++ {
		for right := left + 1; right < len(bidirectional); right++ {
			checks = append(checks, ConnectPorts(bidirectional[left], bidirectional[right]).Checks...)
		}
	}
	slices.SortStableFunc(checks, func(left, right ContractCheck) int {
		if order := strings.Compare(left.Path, right.Path); order != 0 {
			return order
		}
		if order := strings.Compare(string(left.Code), string(right.Code)); order != 0 {
			return order
		}
		return strings.Compare(left.Message, right.Message)
	})
	return checks
}

func buildSharedResourcePlan(requirement Requirement, blocks []HierarchyBlock, interfaces []InterfaceContractPlan) ([]SharedResourcePlan, *candidateValidation) {
	blockByID := map[string]HierarchyBlock{}
	for _, block := range blocks {
		blockByID[block.ID] = block
	}
	var resources []SharedResourcePlan
	for _, domain := range requirement.Requirements.Domains {
		resource := SharedResourcePlan{
			ID: "resource_domain_" + domain.ID, Kind: domain.Kind, Domain: domain.ID,
			Source:   domain.Source,
			Evidence: ContractEvidence{Confidence: EvidenceRuleInferred, Sources: []string{"kicadai:requirement-domain-budget"}},
			Status:   "pass",
		}
		if domain.Source == "external" {
			resource.Source = "external:" + domain.ID
		}
		if domain.MaxCurrentA != nil {
			resource.CapacityA = float64Pointer(*domain.MaxCurrentA)
		}
		var demand float64
		hasDemand := false
		for _, connection := range interfaces {
			if connection.Domain != domain.ID {
				continue
			}
			for _, endpoint := range connection.Endpoints {
				if endpoint.BlockID != "system" {
					resource.Consumers = append(resource.Consumers, endpoint.BlockID)
				}
			}
			if connection.CurrentDemandA != nil {
				demand += *connection.CurrentDemandA
				hasDemand = true
			}
			if canonicalIdentifier(domain.Source) != "external" &&
				connection.CurrentCapacityA != nil &&
				(resource.CapacityA == nil || *connection.CurrentCapacityA > *resource.CapacityA) {
				resource.CapacityA = float64Pointer(*connection.CurrentCapacityA)
				resource.Evidence = ContractEvidence{
					Confidence: EvidenceRuleInferred,
					Sources:    []string{"kicadai:selected-generated-domain-source-capacity"},
				}
			}
			resource.Dependencies = append(resource.Dependencies, connection.ID)
		}
		resource.Consumers = sortedUniqueStrings(resource.Consumers)
		resource.Dependencies = sortedUniqueStrings(resource.Dependencies)
		if hasDemand {
			resource.DemandA = float64Pointer(demand)
		}
		if resource.CapacityA != nil && resource.DemandA != nil {
			margin := *resource.CapacityA - *resource.DemandA
			if margin < 0 {
				return nil, &candidateValidation{
					Code: CodeResourcePlanUnproven, Path: "candidate.system_plan.resources." + resource.ID,
					Message: "generated shared-domain demand exceeds declared capacity",
				}
			}
			resource.MarginA = float64Pointer(margin)
		}
		resources = append(resources, resource)
	}
	for _, connection := range interfaces {
		kind := ""
		switch {
		case strings.HasPrefix(connection.Anchor, "external:"):
			kind = "external_interface"
		case connection.Protocol != nil && connection.Protocol.Name == "clock":
			kind = "clock"
		case connection.DefaultState != "":
			kind = "control"
		}
		if kind == "" {
			continue
		}
		consumers := []string{}
		source := connection.Anchor
		for _, endpoint := range connection.Endpoints {
			consumers = append(consumers, endpoint.BlockID)
			if endpoint.Direction == "source" {
				source = endpoint.BlockID
			}
		}
		resources = append(resources, SharedResourcePlan{
			ID: "resource_" + shortPlanHash(connection.Anchor), Kind: kind, Domain: connection.Domain,
			Source: source, Consumers: sortedUniqueStrings(consumers), Dependencies: []string{connection.ID},
			Evidence: ContractEvidence{Confidence: EvidenceVerified, Sources: []string{"kicadai:selected-interface-contract"}},
			Status:   "pass",
		})
	}
	for _, block := range blocks {
		if slices.Contains(block.Classifications, "protection") || slices.Contains(block.Classifications, "isolation") {
			resources = append(resources, SharedResourcePlan{
				ID: "resource_protection_" + block.ID, Kind: "protection", Source: block.ID,
				Consumers: []string{block.ID},
				Evidence:  ContractEvidence{Confidence: EvidenceVerified, Sources: []string{"kicadai:selected-protection-block"}},
				Status:    "pass",
			})
		}
		if block.Capability == "power_decoupling" {
			resources = append(resources, SharedResourcePlan{
				ID: "resource_decoupling_" + block.ID, Kind: "decoupling", Source: block.ID,
				Consumers: []string{block.ParentBlockID},
				Evidence:  ContractEvidence{Confidence: EvidenceVerified, Sources: []string{"kicadai:selected-decoupling-obligation"}},
				Status:    "pass",
			})
		}
		if slices.Contains(block.Classifications, "thermal") {
			resources = append(resources, SharedResourcePlan{
				ID: "resource_thermal_" + block.ID, Kind: "thermal", Source: block.ID,
				Consumers: []string{block.ID},
				Evidence:  ContractEvidence{Confidence: EvidenceVerified, Sources: []string{"kicadai:selected-thermal-evidence"}},
				Status:    "pass",
			})
		}
	}
	for _, constraint := range requirement.Requirements.SystemConstraints {
		if !strings.Contains(constraint.Name, "startup") && !strings.Contains(constraint.Name, "sequence") {
			continue
		}
		consumers := make([]string, 0, len(blocks))
		for _, block := range blocks {
			consumers = append(consumers, block.ID)
		}
		resources = append(resources, SharedResourcePlan{
			ID: "resource_sequence_" + constraint.Name, Kind: "sequencing", Source: "system_policy",
			Consumers: sortedUniqueStrings(consumers),
			Evidence:  ContractEvidence{Confidence: EvidenceRuleInferred, Sources: []string{"kicadai:system-constraint:" + constraint.Name}},
			Status:    "pass",
		})
	}
	slices.SortStableFunc(resources, func(left, right SharedResourcePlan) int {
		return strings.Compare(left.ID, right.ID)
	})
	seen := map[string]bool{}
	for _, resource := range resources {
		if resource.ID == "" || resource.Source == "" || seen[resource.ID] {
			return nil, &candidateValidation{
				Code: CodeResourcePlanUnproven, Path: "candidate.system_plan.resources",
				Message: "generated resource identities or sources are incomplete",
			}
		}
		seen[resource.ID] = true
		for _, consumer := range resource.Consumers {
			if consumer != "system" {
				if _, exists := blockByID[consumer]; !exists {
					return nil, &candidateValidation{
						Code: CodeResourcePlanUnproven, Path: "candidate.system_plan.resources." + resource.ID,
						Message: "generated resource references an unknown consumer block",
					}
				}
			}
		}
	}
	return resources, nil
}

func buildPhysicalPlan(hierarchy HierarchyPlan, interfaces []InterfaceContractPlan) (PhysicalPlan, *candidateValidation) {
	blocksBySubsystem := map[string][]HierarchyBlock{}
	blockPartition := map[string]string{}
	for _, block := range hierarchy.Blocks {
		blocksBySubsystem[block.ParentID] = append(blocksBySubsystem[block.ParentID], block)
	}
	var plan PhysicalPlan
	for _, subsystem := range hierarchy.Subsystems {
		partition := PhysicalPartition{
			ID:          "partition_" + strings.TrimPrefix(subsystem.ID, "subsystem_"),
			SubsystemID: subsystem.ID,
		}
		for _, block := range blocksBySubsystem[subsystem.ID] {
			partition.BlockIDs = append(partition.BlockIDs, block.ID)
			partition.Classifications = append(partition.Classifications, block.Classifications...)
			blockPartition[block.ID] = partition.ID
		}
		partition.BlockIDs = sortedUniqueStrings(partition.BlockIDs)
		partition.Classifications = sortedUniqueStrings(partition.Classifications)
		partition.Rules = physicalRules(partition.Classifications)
		if len(partition.BlockIDs) == 0 || len(partition.Rules) == 0 {
			return PhysicalPlan{}, &candidateValidation{
				Code: CodePhysicalPlanUnproven, Path: "candidate.system_plan.physical." + partition.ID,
				Message: "generated physical partition lacks blocks or electrical rules",
			}
		}
		plan.Partitions = append(plan.Partitions, partition)
	}
	for _, connection := range interfaces {
		partitions := []string{}
		classes := []string{}
		for _, endpoint := range connection.Endpoints {
			if partition := blockPartition[endpoint.BlockID]; partition != "" {
				partitions = append(partitions, partition)
			}
		}
		partitions = sortedUniqueStrings(partitions)
		if len(partitions) < 2 {
			continue
		}
		if connection.Protocol != nil && connection.Protocol.Name == "clock" {
			classes = append(classes, "clock")
		}
		if slices.Contains(connection.Traits, "galvanic_isolation") {
			classes = append(classes, "isolation")
		}
		if connection.CurrentCapacityA != nil && *connection.CurrentCapacityA >= 0.5 {
			classes = append(classes, "high_current")
		}
		if strings.Contains(connection.Kind, "analog") {
			classes = append(classes, "analog")
		}
		classes = sortedUniqueStrings(classes)
		if len(classes) == 0 {
			classes = []string{"interface"}
		}
		plan.Boundaries = append(plan.Boundaries, PhysicalBoundary{
			InterfaceID: connection.ID, Partitions: partitions,
			Classifications: classes, Rules: physicalBoundaryRules(classes),
		})
	}
	slices.SortStableFunc(plan.Partitions, func(left, right PhysicalPartition) int {
		return strings.Compare(left.ID, right.ID)
	})
	slices.SortStableFunc(plan.Boundaries, func(left, right PhysicalBoundary) int {
		return strings.Compare(left.InterfaceID, right.InterfaceID)
	})
	return plan, nil
}

func buildSystemTraceability(requirement Requirement, blocks []HierarchyBlock, interfaces []InterfaceContractPlan, resources []SharedResourcePlan) ([]SystemTraceabilityRecord, *candidateValidation) {
	blockByRequirement := map[string][]string{}
	for _, block := range blocks {
		if block.RequirementKind != "" {
			key := block.RequirementKind + ":" + block.RequirementID
			blockByRequirement[key] = append(blockByRequirement[key], block.ID)
		}
	}
	for _, block := range blocks {
		if block.ParentBlockID == "" {
			continue
		}
		for key, owners := range blockByRequirement {
			if slices.Contains(owners, block.ParentBlockID) {
				blockByRequirement[key] = append(blockByRequirement[key], block.ID)
			}
		}
	}
	var records []SystemTraceabilityRecord
	for _, participant := range requirement.Requirements.Participants {
		records = append(records, traceRecordForBlocks("participant", participant.ID, blockByRequirement["participant:"+participant.ID], interfaces, resources, nil))
	}
	for _, objective := range requirement.Requirements.Objectives {
		behaviorIDs := []string{}
		for _, behavior := range requirement.Requirements.BehavioralRequirements {
			if hierarchicalObjectiveCone(requirement, behavior.Observation)[objective.ID] {
				behaviorIDs = append(behaviorIDs, behavior.ID)
			}
		}
		records = append(records, traceRecordForBlocks("objective", objective.ID, blockByRequirement["objective:"+objective.ID], interfaces, resources, behaviorIDs))
	}
	for _, behavior := range requirement.Requirements.BehavioralRequirements {
		cone := hierarchicalObjectiveCone(requirement, behavior.Observation)
		blockIDs := []string{}
		for objectiveID := range cone {
			blockIDs = append(blockIDs, blockByRequirement["objective:"+objectiveID]...)
		}
		if behavior.Observation.Kind == "circuit" {
			for _, block := range blocks {
				blockIDs = append(blockIDs, block.ID)
			}
		}
		records = append(records, traceRecordForBlocks("behavior", behavior.ID, blockIDs, interfaces, resources, []string{behavior.ID}))
	}
	slices.SortStableFunc(records, func(left, right SystemTraceabilityRecord) int {
		if order := strings.Compare(left.RequirementKind, right.RequirementKind); order != 0 {
			return order
		}
		return strings.Compare(left.RequirementID, right.RequirementID)
	})
	for _, record := range records {
		if len(record.BlockIDs) == 0 {
			return nil, &candidateValidation{
				Code:    CodeTraceabilityUnproven,
				Path:    "candidate.system_plan.traceability." + record.RequirementKind + "." + record.RequirementID,
				Message: "requirement has no generated block ownership",
			}
		}
	}
	return records, nil
}

func traceRecordForBlocks(kind, id string, blockIDs []string, interfaces []InterfaceContractPlan, resources []SharedResourcePlan, behaviorIDs []string) SystemTraceabilityRecord {
	record := SystemTraceabilityRecord{
		RequirementKind: kind, RequirementID: id,
		BlockIDs:                 sortedUniqueStrings(blockIDs),
		BehavioralRequirementIDs: sortedUniqueStrings(behaviorIDs),
	}
	for _, connection := range interfaces {
		for _, endpoint := range connection.Endpoints {
			if slices.Contains(record.BlockIDs, endpoint.BlockID) {
				record.InterfaceIDs = append(record.InterfaceIDs, connection.ID)
				break
			}
		}
	}
	for _, resource := range resources {
		for _, consumer := range resource.Consumers {
			if slices.Contains(record.BlockIDs, consumer) {
				record.ResourceIDs = append(record.ResourceIDs, resource.ID)
				break
			}
		}
	}
	record.InterfaceIDs = sortedUniqueStrings(record.InterfaceIDs)
	record.ResourceIDs = sortedUniqueStrings(record.ResourceIDs)
	return record
}

func attachBlockInterfaces(hierarchy *HierarchyPlan, interfaces []InterfaceContractPlan) {
	byBlock := map[string][]string{}
	for _, connection := range interfaces {
		for _, endpoint := range connection.Endpoints {
			byBlock[endpoint.BlockID] = append(byBlock[endpoint.BlockID], connection.ID)
		}
	}
	for index := range hierarchy.Blocks {
		hierarchy.Blocks[index].InterfaceIDs = sortedUniqueStrings(byBlock[hierarchy.Blocks[index].ID])
	}
}

func attachBlockVerification(requirement Requirement, hierarchy *HierarchyPlan) *candidateValidation {
	behaviorByObjective := map[string][]string{}
	for _, behavior := range requirement.Requirements.BehavioralRequirements {
		for objectiveID := range hierarchicalObjectiveCone(requirement, behavior.Observation) {
			behaviorByObjective[objectiveID] = append(behaviorByObjective[objectiveID], behavior.ID)
		}
	}
	for index := range hierarchy.Blocks {
		block := &hierarchy.Blocks[index]
		if block.ParentBlockID != "" {
			continue
		}
		objectiveID := ""
		switch block.RequirementKind {
		case "objective":
			objectiveID = block.RequirementID
		case "participant":
			for _, objective := range requirement.Requirements.Objectives {
				for _, binding := range objective.Bindings {
					if binding.Participant == block.RequirementID {
						for _, behaviorID := range behaviorByObjective[objective.ID] {
							block.RequiredBehaviorIDs = append(block.RequiredBehaviorIDs, behaviorID)
						}
					}
				}
			}
		}
		if objectiveID != "" {
			block.RequiredBehaviorIDs = append(block.RequiredBehaviorIDs, behaviorByObjective[objectiveID]...)
		}
		block.RequiredBehaviorIDs = sortedUniqueStrings(block.RequiredBehaviorIDs)
	}
	for index := range hierarchy.Blocks {
		block := &hierarchy.Blocks[index]
		if block.ParentBlockID == "" {
			continue
		}
		for _, parent := range hierarchy.Blocks {
			if parent.ID == block.ParentBlockID {
				block.RequiredBehaviorIDs = append(block.RequiredBehaviorIDs, parent.RequiredBehaviorIDs...)
				break
			}
		}
		block.RequiredBehaviorIDs = sortedUniqueStrings(block.RequiredBehaviorIDs)
	}
	for index := range hierarchy.Blocks {
		block := &hierarchy.Blocks[index]
		for _, behaviorID := range block.RequiredBehaviorIDs {
			block.VerificationIDs = append(block.VerificationIDs, "behavior:"+behaviorID)
		}
		for _, interfaceID := range block.InterfaceIDs {
			block.VerificationIDs = append(block.VerificationIDs, "contract:"+interfaceID)
		}
		block.VerificationIDs = sortedUniqueStrings(block.VerificationIDs)
		if len(block.VerificationIDs) == 0 {
			return &candidateValidation{
				Code: CodeHierarchyUnproven, Path: "candidate.system_plan.hierarchy.blocks." + block.ID,
				Message: "generated block lacks an independently verifiable behavior or interface contract",
			}
		}
	}
	return nil
}

func selectionDomains(selection FragmentSelection) []string {
	domains := []string{}
	for _, port := range selection.Ports {
		if port.Contract.Domain != "" && port.Contract.Kind != "reference" {
			domains = append(domains, port.Contract.Domain)
		}
	}
	return sortedUniqueStrings(domains)
}

func selectionPhysicalClasses(requirement Requirement, selection FragmentSelection) []string {
	classes := []string{}
	for _, port := range selection.Ports {
		kind := port.Contract.Kind
		switch {
		case strings.Contains(kind, "analog"):
			classes = append(classes, "analog")
		case strings.Contains(kind, "digital"), kind == "clock":
			classes = append(classes, "digital")
		}
		if port.Contract.Protocol != nil && port.Contract.Protocol.Name == "clock" {
			classes = append(classes, "clock", "digital")
		}
		for _, current := range []*float64{port.Contract.CurrentCapacityA, port.Contract.CurrentDemandA, port.Contract.RequiredCurrentCapacityA, port.Contract.MaximumCurrentDemandA} {
			if current != nil && *current >= 0.5 {
				classes = append(classes, "high_current")
			}
		}
		traits := append(append([]string(nil), port.Contract.Traits...), port.Contract.RequiredTraits...)
		for _, trait := range traits {
			switch {
			case strings.Contains(trait, "isolation"):
				classes = append(classes, "isolation")
			case strings.Contains(trait, "protection"), strings.Contains(trait, "fault"), strings.Contains(trait, "esd"):
				classes = append(classes, "protection")
			}
		}
		if port.Contract.InputImpedanceMinOhm != nil && *port.Contract.InputImpedanceMinOhm >= 100000 {
			classes = append(classes, "sensitive")
		}
	}
	capability := selection.Capability
	switch {
	case strings.Contains(capability, "amplif"), strings.Contains(capability, "filter"), strings.Contains(capability, "threshold"):
		classes = append(classes, "analog", "feedback")
	}
	switch {
	case strings.Contains(capability, "protection"), strings.Contains(capability, "interlock"):
		classes = append(classes, "protection")
	case strings.Contains(capability, "isolation"):
		classes = append(classes, "isolation")
	}
	if strings.Contains(capability, "class_a") || strings.Contains(capability, "class_ab") || strings.Contains(capability, "load_switch") {
		classes = append(classes, "thermal")
	}
	for _, behavior := range requirement.Requirements.BehavioralRequirements {
		if !hierarchicalObjectiveCone(requirement, behavior.Observation)[strings.TrimPrefix(rootObligationPath(selection.ObligationPath), "objective:")] {
			continue
		}
		switch behavior.Analysis {
		case "noise":
			classes = append(classes, "sensitive")
		case "thermal":
			classes = append(classes, "thermal")
		case "stability":
			classes = append(classes, "feedback")
		}
	}
	if len(classes) == 0 {
		classes = append(classes, "general")
	}
	return sortedUniqueStrings(classes)
}

func physicalRules(classes []string) []string {
	rules := []string{"keep_owned_components_within_partition"}
	for _, class := range classes {
		switch class {
		case "analog":
			rules = append(rules, "preserve_continuous_reference_return")
		case "digital":
			rules = append(rules, "contain_switching_return")
		case "high_current":
			rules = append(rules, "size_conductors_for_worst_case_current")
		case "thermal":
			rules = append(rules, "preserve_thermal_clearance_and_copper")
		case "feedback":
			rules = append(rules, "keep_feedback_loop_local")
		case "clock":
			rules = append(rules, "contain_clock_fanout_and_avoid_sensitive_crossings")
		case "protection":
			rules = append(rules, "place_protection_at_exposure_boundary")
		case "isolation":
			rules = append(rules, "preserve_galvanic_keepout_and_reference_separation")
		case "sensitive":
			rules = append(rules, "isolate_sensitive_node_from_switching_and_high_current")
		}
	}
	return sortedUniqueStrings(rules)
}

func physicalBoundaryRules(classes []string) []string {
	rules := []string{"route_only_declared_interface_across_partition_boundary"}
	for _, class := range classes {
		switch class {
		case "analog":
			rules = append(rules, "preserve_reference_return_across_boundary")
		case "clock":
			rules = append(rules, "avoid_clock_crossing_over_reference_discontinuity")
		case "high_current":
			rules = append(rules, "validate_width_and_layer_transition_at_boundary")
		case "isolation":
			rules = append(rules, "enforce_isolation_clearance_at_boundary")
		}
	}
	return sortedUniqueStrings(rules)
}

// hierarchicalObjectiveCone extends the directed signal cone with functional
// connectivity across bidirectional interfaces. Power and reference anchors
// are excluded so a shared rail does not make unrelated blocks part of every
// behavioral proof.
func hierarchicalObjectiveCone(requirement Requirement, observation Observation) map[string]bool {
	cone := upstreamObjectiveCone(requirement, observation)
	if observation.Kind == "circuit" {
		for _, objective := range requirement.Requirements.Objectives {
			cone[objective.ID] = true
		}
		return cone
	}
	anchorsByObjective := map[string]map[string]bool{}
	objectivesByAnchor := map[string][]string{}
	for _, objective := range requirement.Requirements.Objectives {
		anchors := map[string]bool{}
		for _, binding := range objective.Bindings {
			role := strings.ToLower(binding.Role)
			if role == "reference" || strings.Contains(role, "power") {
				continue
			}
			anchor := ""
			switch {
			case binding.Signal != "":
				anchor = "signal:" + binding.Signal
			case binding.Participant != "":
				anchor = "participant:" + binding.Participant + ":" + binding.ParticipantPort
			case binding.Port != "":
				anchor = "port:" + binding.Port
			}
			if anchor == "" {
				continue
			}
			anchors[anchor] = true
			objectivesByAnchor[anchor] = append(objectivesByAnchor[anchor], objective.ID)
		}
		anchorsByObjective[objective.ID] = anchors
	}
	observedAnchor := observation.Kind + ":" + observation.ID
	for _, objectiveID := range objectivesByAnchor[observedAnchor] {
		cone[objectiveID] = true
	}
	changed := true
	for changed {
		changed = false
		for objectiveID := range cone {
			for anchor := range anchorsByObjective[objectiveID] {
				for _, adjacent := range objectivesByAnchor[anchor] {
					if cone[adjacent] {
						continue
					}
					cone[adjacent] = true
					changed = true
				}
			}
		}
	}
	return cone
}

func intersectEndpointVoltage(endpoints []planEndpoint) NumericRange {
	minimum := math.Inf(-1)
	maximum := math.Inf(1)
	hasMinimum := false
	hasMaximum := false
	for _, endpoint := range endpoints {
		if endpoint.Contract.Voltage.Minimum != nil {
			minimum = math.Max(minimum, *endpoint.Contract.Voltage.Minimum)
			hasMinimum = true
		}
		if endpoint.Contract.Voltage.Maximum != nil {
			maximum = math.Min(maximum, *endpoint.Contract.Voltage.Maximum)
			hasMaximum = true
		}
	}
	result := NumericRange{}
	if hasMinimum {
		result.Minimum = float64Pointer(minimum)
	}
	if hasMaximum {
		result.Maximum = float64Pointer(maximum)
	}
	return result
}

func requirementBoundaryDirection(requirement Requirement, anchor string) string {
	if strings.HasPrefix(anchor, "external:") {
		id := strings.TrimPrefix(anchor, "external:")
		for _, port := range requirement.Requirements.Ports {
			if port.ID == id {
				return port.Direction
			}
		}
	}
	if strings.HasPrefix(anchor, "domain:") {
		id := strings.TrimPrefix(anchor, "domain:")
		for _, domain := range requirement.Requirements.Domains {
			if domain.ID != id {
				continue
			}
			if domain.Kind == "supply" {
				return "source"
			}
			return "bidirectional"
		}
	}
	return "bidirectional"
}

func rootObligationPath(path string) string {
	if index := strings.IndexByte(path, '/'); index >= 0 {
		return path[:index]
	}
	return path
}

func splitRootObligation(path string) (string, string) {
	kind, id, found := strings.Cut(path, ":")
	if !found {
		return "", ""
	}
	return kind, id
}

func topLevelBlockID(path string) string {
	kind, id := splitRootObligation(path)
	return "block_" + derivedSemanticIdentifier(kind+"_"+id)
}

func shortPlanHash(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:8])
}

func sortedUniqueStrings(values []string) []string {
	values = append([]string(nil), values...)
	slices.Sort(values)
	return slices.Compact(values)
}

func BuildBacktrackingEvidence(candidates []CandidateResult) BacktrackingEvidence {
	evidence := BacktrackingEvidence{
		Schema:        BacktrackingSchema,
		Strategy:      "canonical_complete_candidate_order",
		Deterministic: true,
	}
	for _, candidate := range candidates {
		evidence.CandidateOrder = append(evidence.CandidateOrder, candidate.Fingerprint)
	}
	return evidence
}

// ValidateBacktrackingEvidence verifies that persisted V4 candidate ordering
// remains complete and byte-for-byte tied to the retained architectures.
func ValidateBacktrackingEvidence(evidence BacktrackingEvidence, candidates []CandidateResult) error {
	if evidence.Schema != BacktrackingSchema || !evidence.Deterministic ||
		evidence.Strategy != "canonical_complete_candidate_order" ||
		len(evidence.CandidateOrder) != len(candidates) {
		return fmt.Errorf("backtracking evidence header or candidate count is invalid")
	}
	for index, candidate := range candidates {
		if evidence.CandidateOrder[index] != candidate.Fingerprint {
			return fmt.Errorf("candidate order[%d] does not match retained architecture order", index)
		}
	}
	return nil
}
