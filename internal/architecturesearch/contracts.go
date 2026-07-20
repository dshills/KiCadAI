package architecturesearch

import (
	"cmp"
	"fmt"
	"math"
	"slices"
	"strings"

	"kicadai/internal/components"
	"kicadai/internal/reports"
)

type EvidenceConfidence string

const (
	EvidenceBlocked        EvidenceConfidence = "blocked"
	EvidenceUnknown        EvidenceConfidence = "unknown"
	EvidencePlaceholder    EvidenceConfidence = "placeholder"
	EvidenceRuleInferred   EvidenceConfidence = "rule_inferred"
	EvidenceLibraryDerived EvidenceConfidence = "library_derived"
	EvidenceVerified       EvidenceConfidence = "verified"
)

type ContractEvidence struct {
	Confidence EvidenceConfidence `json:"confidence"`
	Sources    []string           `json:"sources,omitempty"`
}

type NumericRange struct {
	Minimum *float64 `json:"minimum,omitempty"`
	Maximum *float64 `json:"maximum,omitempty"`
}

type LogicLevels struct {
	OutputLowMaximumV  *float64 `json:"output_low_maximum_v,omitempty"`
	OutputHighMinimumV *float64 `json:"output_high_minimum_v,omitempty"`
	InputLowMaximumV   *float64 `json:"input_low_maximum_v,omitempty"`
	InputHighMinimumV  *float64 `json:"input_high_minimum_v,omitempty"`
}

// PortContract is the common electrical boundary used by requirements,
// catalog-backed components, and reusable fragments. Capability and demand are
// separate so a source's drive can be checked against a sink's load without
// treating an unknown value as unlimited or zero.
type PortContract struct {
	ID                       string             `json:"id"`
	Kind                     string             `json:"kind"`
	Direction                string             `json:"direction"`
	Domain                   string             `json:"domain"`
	Voltage                  NumericRange       `json:"voltage"`
	CurrentCapacityA         *float64           `json:"current_capacity_a,omitempty"`
	CurrentDemandA           *float64           `json:"current_demand_a,omitempty"`
	RequiredCurrentCapacityA *float64           `json:"required_current_capacity_a,omitempty"`
	MaximumCurrentDemandA    *float64           `json:"maximum_current_demand_a,omitempty"`
	InputImpedanceMinOhm     *float64           `json:"input_impedance_min_ohm,omitempty"`
	FrequencyMaxHz           *float64           `json:"frequency_max_hz,omitempty"`
	Logic                    *LogicLevels       `json:"logic,omitempty"`
	Protocol                 *Protocol          `json:"protocol,omitempty"`
	DefaultState             string             `json:"default_state,omitempty"`
	Traits                   []string           `json:"traits,omitempty"`
	RequiredTraits           []string           `json:"required_traits,omitempty"`
	Evidence                 ContractEvidence   `json:"evidence"`
	MinimumEvidence          EvidenceConfidence `json:"minimum_evidence,omitempty"`
}

type ComponentContract struct {
	ComponentID string           `json:"component_id"`
	Family      string           `json:"family"`
	Ports       []PortContract   `json:"ports"`
	Evidence    ContractEvidence `json:"evidence"`
}

type ContractCheckStatus string

const (
	ContractCheckPass   ContractCheckStatus = "pass"
	ContractCheckReject ContractCheckStatus = "reject"
)

const (
	CodeContractInvalid          reports.Code = "ARCHITECTURE_CONTRACT_INVALID"
	CodePortKindMismatch         reports.Code = "ARCHITECTURE_PORT_KIND_MISMATCH"
	CodeDirectionMismatch        reports.Code = "ARCHITECTURE_DIRECTION_MISMATCH"
	CodeDomainMismatch           reports.Code = "ARCHITECTURE_DOMAIN_MISMATCH"
	CodeVoltageEvidenceMissing   reports.Code = "ARCHITECTURE_VOLTAGE_EVIDENCE_MISSING"
	CodeVoltageRangeMismatch     reports.Code = "ARCHITECTURE_VOLTAGE_RANGE_MISMATCH"
	CodeCurrentEvidenceMissing   reports.Code = "ARCHITECTURE_CURRENT_EVIDENCE_MISSING"
	CodeCurrentCapacity          reports.Code = "ARCHITECTURE_CURRENT_CAPACITY_INSUFFICIENT"
	CodeImpedanceEvidenceMissing reports.Code = "ARCHITECTURE_IMPEDANCE_EVIDENCE_MISSING"
	CodeImpedanceMismatch        reports.Code = "ARCHITECTURE_IMPEDANCE_MISMATCH"
	CodeFrequencyEvidenceMissing reports.Code = "ARCHITECTURE_FREQUENCY_EVIDENCE_MISSING"
	CodeFrequencyMismatch        reports.Code = "ARCHITECTURE_FREQUENCY_MISMATCH"
	CodeLogicEvidenceMissing     reports.Code = "ARCHITECTURE_LOGIC_EVIDENCE_MISSING"
	CodeLogicLevelMismatch       reports.Code = "ARCHITECTURE_LOGIC_LEVEL_MISMATCH"
	CodeProtocolMismatch         reports.Code = "ARCHITECTURE_PROTOCOL_MISMATCH"
	CodeDefaultStateMismatch     reports.Code = "ARCHITECTURE_DEFAULT_STATE_MISMATCH"
	CodeTraitMissing             reports.Code = "ARCHITECTURE_TRAIT_MISSING"
	CodeEvidenceInsufficient     reports.Code = "ARCHITECTURE_EVIDENCE_INSUFFICIENT"
)

type ContractCheck struct {
	Code     reports.Code        `json:"code"`
	Status   ContractCheckStatus `json:"status"`
	Path     string              `json:"path"`
	Message  string              `json:"message"`
	Required *float64            `json:"required,omitempty"`
	Offered  *float64            `json:"offered,omitempty"`
	Margin   *float64            `json:"margin,omitempty"`
}

type CompatibilityReport struct {
	Compatible bool            `json:"compatible"`
	Checks     []ContractCheck `json:"checks"`
}

func ContractFromRequirementPort(requirement Requirement, portID string, minimumEvidence EvidenceConfidence) (PortContract, []reports.Issue) {
	normalized := Normalize(requirement)
	if issues := Validate(normalized); len(issues) != 0 {
		return PortContract{}, issues
	}
	var port *Port
	for index := range normalized.Requirements.Ports {
		if normalized.Requirements.Ports[index].ID == canonicalIdentifier(portID) {
			port = &normalized.Requirements.Ports[index]
			break
		}
	}
	if port == nil {
		return PortContract{}, []reports.Issue{architectureIssue(CodeBindingUnresolved, "requirements.ports", "cannot build contract for unknown port "+portID)}
	}
	domain := requirementDomain(normalized, port.Domain)
	contract := PortContract{
		ID: port.ID, Kind: port.Kind, Direction: port.Direction, Domain: port.Domain,
		Voltage:         domainVoltageRange(domain),
		MinimumEvidence: minimumEvidence,
	}
	if port.Protocol != nil {
		protocol := *port.Protocol
		contract.Protocol = &protocol
	}
	if electrical := port.Electrical; electrical != nil {
		if electrical.MinVoltageV != nil {
			contract.Voltage.Minimum = float64Pointer(*electrical.MinVoltageV)
		}
		if electrical.MaxVoltageV != nil {
			contract.Voltage.Maximum = float64Pointer(*electrical.MaxVoltageV)
		}
		contract.InputImpedanceMinOhm = cloneFloat64(electrical.InputImpedanceMinOhm)
		contract.FrequencyMaxHz = cloneFloat64(electrical.FrequencyMaxHz)
		contract.DefaultState = electrical.DefaultState
		if electrical.MaxCurrentA != nil {
			if port.Direction == "source" || port.Kind == "switched_load" || port.Kind == "protected_output" {
				contract.RequiredCurrentCapacityA = cloneFloat64(electrical.MaxCurrentA)
			} else {
				contract.MaximumCurrentDemandA = cloneFloat64(electrical.MaxCurrentA)
			}
		}
		if electrical.MaxSourceCurrentMA != nil {
			amps := *electrical.MaxSourceCurrentMA / 1000
			if port.Direction == "source" {
				contract.RequiredCurrentCapacityA = maxPointer(contract.RequiredCurrentCapacityA, amps)
			} else {
				contract.MaximumCurrentDemandA = minPointer(contract.MaximumCurrentDemandA, amps)
			}
		}
	}
	if port.Electrical == nil || port.Electrical.MaxCurrentA == nil {
		if domain.MaxCurrentA != nil && port.Kind == "power" {
			if port.Direction == "source" {
				contract.RequiredCurrentCapacityA = cloneFloat64(domain.MaxCurrentA)
			} else {
				contract.MaximumCurrentDemandA = cloneFloat64(domain.MaxCurrentA)
			}
		}
	}
	return NormalizePortContract(contract), nil
}

// ContractFromRequirementSignal builds one directed endpoint contract for a
// behavior-level signal. The shared signal supplies electrical/protocol bounds;
// each binding supplies its endpoint direction.
func ContractFromRequirementSignal(requirement Requirement, signalID, direction string, minimumEvidence EvidenceConfidence) (PortContract, []reports.Issue) {
	normalized := Normalize(requirement)
	if issues := Validate(normalized); len(issues) != 0 {
		return PortContract{}, issues
	}
	signalID = canonicalIdentifier(signalID)
	direction = canonicalIdentifier(direction)
	var signal *Signal
	for index := range normalized.Requirements.Signals {
		if normalized.Requirements.Signals[index].ID == signalID {
			signal = &normalized.Requirements.Signals[index]
			break
		}
	}
	if signal == nil || !allowedDirection(direction) {
		return PortContract{}, []reports.Issue{architectureIssue(CodeBindingUnresolved, "requirements.signals", "cannot build directed contract for signal "+signalID)}
	}
	domain := requirementDomain(normalized, signal.Domain)
	contract := PortContract{
		ID: signal.ID, Kind: signal.Kind, Direction: direction, Domain: signal.Domain,
		Voltage: domainVoltageRange(domain), MinimumEvidence: minimumEvidence,
	}
	if signal.Protocol != nil {
		protocol := *signal.Protocol
		contract.Protocol = &protocol
		contract.FrequencyMaxHz = float64Pointer(protocol.MaxFrequencyHz)
	}
	if electrical := signal.Electrical; electrical != nil {
		if electrical.MinVoltageV != nil {
			contract.Voltage.Minimum = float64Pointer(*electrical.MinVoltageV)
		}
		if electrical.MaxVoltageV != nil {
			contract.Voltage.Maximum = float64Pointer(*electrical.MaxVoltageV)
		}
		contract.InputImpedanceMinOhm = cloneFloat64(electrical.InputImpedanceMinOhm)
		contract.FrequencyMaxHz = cloneFloat64(electrical.FrequencyMaxHz)
		contract.DefaultState = electrical.DefaultState
		if electrical.MaxCurrentA != nil {
			if direction == "source" {
				contract.RequiredCurrentCapacityA = cloneFloat64(electrical.MaxCurrentA)
			} else {
				contract.MaximumCurrentDemandA = cloneFloat64(electrical.MaxCurrentA)
			}
		}
		if electrical.MaxSourceCurrentMA != nil {
			amps := *electrical.MaxSourceCurrentMA / 1000
			if direction == "source" {
				contract.RequiredCurrentCapacityA = maxPointer(contract.RequiredCurrentCapacityA, amps)
			} else {
				contract.MaximumCurrentDemandA = minPointer(contract.MaximumCurrentDemandA, amps)
			}
		}
	}
	if (signal.Electrical == nil || signal.Electrical.MaxCurrentA == nil) && domain.MaxCurrentA != nil && signal.Kind == "power" {
		if direction == "source" {
			contract.RequiredCurrentCapacityA = cloneFloat64(domain.MaxCurrentA)
		} else {
			contract.MaximumCurrentDemandA = cloneFloat64(domain.MaxCurrentA)
		}
	}
	return NormalizePortContract(contract), nil
}

func ContractFromBinding(requirement Requirement, binding Binding, minimumEvidence EvidenceConfidence) (PortContract, []reports.Issue) {
	normalized := Normalize(requirement)
	if issues := Validate(normalized); len(issues) != 0 {
		return PortContract{}, issues
	}
	binding.Port = canonicalIdentifier(binding.Port)
	binding.Signal = canonicalIdentifier(binding.Signal)
	binding.Direction = canonicalIdentifier(binding.Direction)
	binding.Participant = canonicalIdentifier(binding.Participant)
	binding.ParticipantPort = canonicalIdentifier(binding.ParticipantPort)
	if binding.Port != "" && binding.Signal == "" && binding.Direction == "" && binding.Participant == "" && binding.ParticipantPort == "" {
		return ContractFromRequirementPort(normalized, binding.Port, minimumEvidence)
	}
	if binding.Signal != "" && binding.Direction != "" && binding.Port == "" && binding.Participant == "" && binding.ParticipantPort == "" {
		return ContractFromRequirementSignal(normalized, binding.Signal, binding.Direction, minimumEvidence)
	}
	if binding.Port != "" || binding.Signal != "" || binding.Direction != "" || binding.Participant == "" || binding.ParticipantPort == "" {
		return PortContract{}, []reports.Issue{architectureIssue(CodeBindingUnresolved, "binding", "binding must select exactly one external, participant, or directed signal endpoint")}
	}
	return contractFromParticipantPort(normalized, binding.Participant, binding.ParticipantPort, minimumEvidence, true)
}

func contractFromParticipantPort(requirement Requirement, participantID, portID string, minimumEvidence EvidenceConfidence, oppositeEndpoint bool) (PortContract, []reports.Issue) {
	for _, participant := range requirement.Requirements.Participants {
		if participant.ID != participantID {
			continue
		}
		for _, port := range participant.RequiredPorts {
			if port.ID != portID {
				continue
			}
			direction := port.Direction
			if oppositeEndpoint {
				direction = oppositeDirection(direction)
			}
			contract := PortContract{
				ID: participant.ID + "." + port.ID, Kind: port.Kind, Direction: direction,
				Domain: participant.Domain, Voltage: domainVoltageRange(requirementDomain(requirement, participant.Domain)),
				MinimumEvidence: minimumEvidence,
			}
			if port.Protocol != nil {
				protocol := *port.Protocol
				contract.Protocol = &protocol
				contract.FrequencyMaxHz = float64Pointer(protocol.MaxFrequencyHz)
			}
			return NormalizePortContract(contract), nil
		}
	}
	return PortContract{}, []reports.Issue{architectureIssue(CodeBindingUnresolved, "binding", "cannot build contract for unknown participant port")}
}

func oppositeDirection(direction string) string {
	switch direction {
	case "source":
		return "sink"
	case "sink":
		return "source"
	default:
		return direction
	}
}

func requirementDomain(requirement Requirement, domainID string) Domain {
	for _, domain := range requirement.Requirements.Domains {
		if domain.ID == domainID {
			return domain
		}
	}
	return Domain{}
}

func domainVoltageRange(domain Domain) NumericRange {
	minimum := domain.NominalVoltageV
	maximum := domain.NominalVoltageV
	if domain.MinVoltageV != nil {
		minimum = *domain.MinVoltageV
	}
	if domain.MaxVoltageV != nil {
		maximum = *domain.MaxVoltageV
	}
	return NumericRange{Minimum: float64Pointer(minimum), Maximum: float64Pointer(maximum)}
}

func NewComponentContract(record components.ComponentRecord, ports []PortContract) (ComponentContract, []reports.Issue) {
	evidence := ContractEvidence{Confidence: EvidenceConfidenceFromComponent(record.Verification.Confidence), Sources: append([]string(nil), record.Verification.Sources...)}
	contract := ComponentContract{ComponentID: record.ID, Family: record.Family, Ports: append([]PortContract(nil), ports...), Evidence: evidence}
	var issues []reports.Issue
	if contract.ComponentID == "" || contract.Family == "" {
		issues = append(issues, architectureIssue(CodeContractInvalid, "component", "catalog component contract requires component id and family"))
	}
	if evidence.Confidence == EvidenceUnknown || evidence.Confidence == EvidencePlaceholder || evidence.Confidence == EvidenceBlocked {
		issues = append(issues, architectureIssue(CodeEvidenceInsufficient, "component.evidence", "catalog component confidence is insufficient for electrical architecture selection"))
	}
	seen := map[string]bool{}
	for index := range contract.Ports {
		contract.Ports[index] = NormalizePortContract(contract.Ports[index])
		if contract.Ports[index].ID == "" || seen[contract.Ports[index].ID] {
			issues = append(issues, architectureIssue(CodeContractInvalid, fmt.Sprintf("component.ports[%d].id", index), "component port identity is empty or duplicated"))
		}
		seen[contract.Ports[index].ID] = true
		if contract.Ports[index].Evidence.Confidence == "" || contract.Ports[index].Evidence.Confidence == EvidenceUnknown {
			contract.Ports[index].Evidence = evidence
		}
		for _, check := range contractValidityChecks(fmt.Sprintf("component.ports[%d]", index), contract.Ports[index]) {
			if check.Status == ContractCheckReject {
				issues = append(issues, architectureIssue(check.Code, check.Path, check.Message))
			}
		}
	}
	slices.SortStableFunc(contract.Ports, func(left, right PortContract) int { return strings.Compare(left.ID, right.ID) })
	contract.Evidence = normalizeContractEvidence(contract.Evidence)
	return contract, issues
}

func EvidenceConfidenceFromComponent(confidence components.ConfidenceLevel) EvidenceConfidence {
	switch confidence {
	case components.ConfidenceVerified:
		return EvidenceVerified
	case components.ConfidenceLibraryDerived:
		return EvidenceLibraryDerived
	case components.ConfidenceRuleInferred:
		return EvidenceRuleInferred
	case components.ConfidencePlaceholder:
		return EvidencePlaceholder
	case components.ConfidenceBlocked:
		return EvidenceBlocked
	default:
		return EvidenceUnknown
	}
}

// SatisfiesPortRequirement proves that an offered component or fragment port
// conforms to a behavioral boundary requirement with the same direction.
func SatisfiesPortRequirement(required, offered PortContract) CompatibilityReport {
	required = NormalizePortContract(required)
	offered = NormalizePortContract(offered)
	checks := make([]ContractCheck, 0, 16)
	checks = append(checks, contractValidityChecks("required", required)...)
	checks = append(checks, contractValidityChecks("offered", offered)...)
	checks = append(checks, equalityCheck(CodePortKindMismatch, "kind", required.Kind, offered.Kind, "port kinds match"))
	checks = append(checks, directionConformanceCheck(required.Direction, offered.Direction))
	checks = append(checks, equalityCheck(CodeDomainMismatch, "domain", required.Domain, offered.Domain, "electrical domains match"))
	checks = append(checks, voltageConformanceChecks(required, offered)...)
	checks = append(checks, currentConformanceChecks(required, offered)...)
	checks = append(checks, lowerBoundCheck(CodeImpedanceEvidenceMissing, CodeImpedanceMismatch, "input_impedance_min_ohm", required.InputImpedanceMinOhm, offered.InputImpedanceMinOhm, "offered input impedance satisfies required minimum"))
	checks = append(checks, upperCapacityCheck(CodeFrequencyEvidenceMissing, CodeFrequencyMismatch, "frequency_max_hz", required.FrequencyMaxHz, offered.FrequencyMaxHz, "offered bandwidth satisfies required maximum frequency"))
	checks = append(checks, protocolCheck(required.Protocol, offered.Protocol)...)
	checks = append(checks, defaultStateCheck(required.DefaultState, offered.DefaultState))
	checks = append(checks, traitChecks(required.RequiredTraits, offered.Traits)...)
	checks = append(checks, evidenceCheck(required.MinimumEvidence, offered.Evidence.Confidence))
	if required.Logic != nil {
		checks = append(checks, logicConformanceChecks(*required.Logic, offered.Logic)...)
	}
	return finalizeCompatibility(checks)
}

// ConnectPorts proves that two realized ports can be connected directly. A
// converter fragment must expose one compatible port per domain; this function
// never silently translates domains or signaling modes.
func ConnectPorts(left, right PortContract) CompatibilityReport {
	left = NormalizePortContract(left)
	right = NormalizePortContract(right)
	checks := append(contractValidityChecks("left", left), contractValidityChecks("right", right)...)
	checks = append(checks,
		equalityCheck(CodePortKindMismatch, "kind", left.Kind, right.Kind, "connected port kinds match"),
		equalityCheck(CodeDomainMismatch, "domain", left.Domain, right.Domain, "connected electrical domains match"),
	)
	if left.Direction == "bidirectional" && right.Direction == "bidirectional" {
		checks = append(checks, passCheck(CodeDirectionMismatch, "direction", "bidirectional ports are directionally compatible"))
		checks = append(checks, overlappingVoltageCheck(left.Voltage, right.Voltage))
		checks = append(checks, connectionProtocolChecks(left.Protocol, right.Protocol)...)
		return finalizeCompatibility(checks)
	}

	var source, sink PortContract
	if left.Direction == "source" && right.Direction == "sink" {
		source, sink = left, right
	} else if right.Direction == "source" && left.Direction == "sink" {
		source, sink = right, left
	} else {
		checks = append(checks, rejectCheck(CodeDirectionMismatch, "direction", "direct connection requires one source and one sink, or two bidirectional ports"))
		return finalizeCompatibility(checks)
	}
	checks = append(checks, passCheck(CodeDirectionMismatch, "direction", "source and sink directions are compatible"))
	checks = append(checks, sourceWithinSinkVoltageCheck(source.Voltage, sink.Voltage))
	checks = append(checks, connectionCurrentCheck(source.CurrentCapacityA, sink.CurrentDemandA))
	checks = append(checks, connectionProtocolChecks(source.Protocol, sink.Protocol)...)
	if source.Logic != nil || sink.Logic != nil {
		checks = append(checks, logicConnectionChecks(source.Logic, sink.Logic)...)
	}
	return finalizeCompatibility(checks)
}

func NormalizePortContract(contract PortContract) PortContract {
	contract.ID = canonicalIdentifier(contract.ID)
	contract.Kind = canonicalIdentifier(contract.Kind)
	contract.Direction = canonicalIdentifier(contract.Direction)
	contract.Domain = canonicalIdentifier(contract.Domain)
	contract.DefaultState = canonicalIdentifier(contract.DefaultState)
	contract.Traits = normalizeStringSet(contract.Traits)
	contract.RequiredTraits = normalizeStringSet(contract.RequiredTraits)
	contract.Evidence = normalizeContractEvidence(contract.Evidence)
	if contract.Protocol != nil {
		normalizeProtocol(contract.Protocol)
	}
	return contract
}

func normalizeContractEvidence(evidence ContractEvidence) ContractEvidence {
	if evidence.Confidence == "" {
		evidence.Confidence = EvidenceUnknown
	}
	evidence.Sources = normalizeStringSet(evidence.Sources)
	return evidence
}

func normalizeStringSet(values []string) []string {
	normalized := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = canonicalIdentifier(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		normalized = append(normalized, value)
	}
	slices.Sort(normalized)
	return normalized
}

func contractValidityChecks(prefix string, contract PortContract) []ContractCheck {
	checks := []ContractCheck{}
	if !allowedPortKind(contract.Kind) {
		checks = append(checks, rejectCheck(CodeContractInvalid, prefix+".kind", "port contract has an unsupported kind"))
	}
	if !allowedDirection(contract.Direction) {
		checks = append(checks, rejectCheck(CodeContractInvalid, prefix+".direction", "port contract has an unsupported direction"))
	}
	if !validSemanticID(contract.Domain) {
		checks = append(checks, rejectCheck(CodeContractInvalid, prefix+".domain", "port contract has an invalid domain identity"))
	}
	if contract.Voltage.Minimum == nil || contract.Voltage.Maximum == nil {
		checks = append(checks, rejectCheck(CodeVoltageEvidenceMissing, prefix+".voltage", "port contract requires complete voltage evidence"))
	} else if !finiteInRange(*contract.Voltage.Minimum, -1000, 1000) || !finiteInRange(*contract.Voltage.Maximum, -1000, 1000) || *contract.Voltage.Minimum > *contract.Voltage.Maximum {
		checks = append(checks, rejectCheck(CodeContractInvalid, prefix+".voltage", "port contract voltage range is non-finite, reversed, or outside policy bounds"))
	}
	for _, numeric := range []struct {
		path  string
		value *float64
		max   float64
	}{
		{"current_capacity_a", contract.CurrentCapacityA, 10000},
		{"current_demand_a", contract.CurrentDemandA, 10000},
		{"required_current_capacity_a", contract.RequiredCurrentCapacityA, 10000},
		{"maximum_current_demand_a", contract.MaximumCurrentDemandA, 10000},
		{"input_impedance_min_ohm", contract.InputImpedanceMinOhm, 1e15},
		{"frequency_max_hz", contract.FrequencyMaxHz, 1e15},
	} {
		if numeric.value != nil && !finiteInRange(*numeric.value, 0, numeric.max) {
			checks = append(checks, rejectCheck(CodeContractInvalid, prefix+"."+numeric.path, "port contract numeric evidence is non-finite or outside policy bounds"))
		}
	}
	if contract.Protocol != nil && (!validSemanticID(contract.Protocol.Name) || (contract.Protocol.Mode != "open_drain" && contract.Protocol.Mode != "push_pull" && contract.Protocol.Mode != "differential" && contract.Protocol.Mode != "single_ended") || !finiteInRange(contract.Protocol.MaxFrequencyHz, 0.000001, 1e15)) {
		checks = append(checks, rejectCheck(CodeContractInvalid, prefix+".protocol", "port contract protocol evidence is invalid"))
	}
	if contract.Logic != nil {
		for _, level := range []struct {
			path  string
			value *float64
		}{
			{"output_low_maximum_v", contract.Logic.OutputLowMaximumV},
			{"output_high_minimum_v", contract.Logic.OutputHighMinimumV},
			{"input_low_maximum_v", contract.Logic.InputLowMaximumV},
			{"input_high_minimum_v", contract.Logic.InputHighMinimumV},
		} {
			if level.value != nil && !finiteInRange(*level.value, -1000, 1000) {
				checks = append(checks, rejectCheck(CodeContractInvalid, prefix+".logic."+level.path, "logic-level evidence is non-finite or outside policy bounds"))
			}
		}
	}
	if !validEvidenceConfidence(contract.Evidence.Confidence) || (contract.MinimumEvidence != "" && !validEvidenceConfidence(contract.MinimumEvidence)) {
		checks = append(checks, rejectCheck(CodeContractInvalid, prefix+".evidence", "port contract evidence confidence is invalid"))
	}
	return checks
}

func voltageConformanceChecks(required, offered PortContract) []ContractCheck {
	if required.Voltage.Minimum == nil && required.Voltage.Maximum == nil {
		return nil
	}
	if offered.Voltage.Minimum == nil || offered.Voltage.Maximum == nil {
		return []ContractCheck{rejectCheck(CodeVoltageEvidenceMissing, "voltage", "offered port lacks a complete voltage range")}
	}
	if required.Voltage.Minimum == nil || required.Voltage.Maximum == nil {
		return []ContractCheck{rejectCheck(CodeContractInvalid, "voltage", "required port has an incomplete voltage range")}
	}
	if required.Direction == "source" {
		return []ContractCheck{rangeContainmentCheck("voltage", required.Voltage, offered.Voltage, "offered source output remains within the required output window")}
	}
	return []ContractCheck{rangeContainmentCheck("voltage", offered.Voltage, required.Voltage, "offered sink accepts the complete required input range")}
}

func currentConformanceChecks(required, offered PortContract) []ContractCheck {
	return []ContractCheck{
		lowerBoundCheck(CodeCurrentEvidenceMissing, CodeCurrentCapacity, "required_current_capacity_a", required.RequiredCurrentCapacityA, offered.CurrentCapacityA, "offered current capacity satisfies requirement"),
		upperBoundCheck(CodeCurrentEvidenceMissing, CodeCurrentCapacity, "maximum_current_demand_a", required.MaximumCurrentDemandA, offered.CurrentDemandA, "offered current demand remains within requirement"),
	}
}

func directionConformanceCheck(required, offered string) ContractCheck {
	compatible := required == offered || (required != "bidirectional" && offered == "bidirectional")
	if compatible {
		return passCheck(CodeDirectionMismatch, "direction", "offered direction satisfies required direction")
	}
	return rejectCheck(CodeDirectionMismatch, "direction", fmt.Sprintf("required direction %q is not satisfied by %q", required, offered))
}

func protocolCheck(required, offered *Protocol) []ContractCheck {
	if required == nil && offered == nil {
		return nil
	}
	if required == nil {
		return []ContractCheck{passCheck(CodeProtocolMismatch, "protocol", "requirement does not constrain offered protocol")}
	}
	if offered == nil {
		return []ContractCheck{rejectCheck(CodeProtocolMismatch, "protocol", "offered port lacks required protocol evidence")}
	}
	checks := []ContractCheck{
		equalityCheck(CodeProtocolMismatch, "protocol.name", required.Name, offered.Name, "protocol names match"),
		equalityCheck(CodeProtocolMismatch, "protocol.mode", required.Mode, offered.Mode, "signaling modes match"),
	}
	requiredFrequency := required.MaxFrequencyHz
	offeredFrequency := offered.MaxFrequencyHz
	checks = append(checks, upperCapacityCheck(CodeFrequencyEvidenceMissing, CodeFrequencyMismatch, "protocol.max_frequency_hz", &requiredFrequency, &offeredFrequency, "offered protocol frequency satisfies requirement"))
	return checks
}

func connectionProtocolChecks(left, right *Protocol) []ContractCheck {
	if left == nil && right == nil {
		return nil
	}
	if left == nil || right == nil {
		return []ContractCheck{rejectCheck(CodeProtocolMismatch, "protocol", "direct connection requires protocol evidence on both constrained ports")}
	}
	checks := []ContractCheck{
		equalityCheck(CodeProtocolMismatch, "protocol.name", left.Name, right.Name, "connected protocol names match"),
		equalityCheck(CodeProtocolMismatch, "protocol.mode", left.Mode, right.Mode, "connected signaling modes match"),
	}
	if left.MaxFrequencyHz <= 0 || right.MaxFrequencyHz <= 0 {
		checks = append(checks, rejectCheck(CodeFrequencyEvidenceMissing, "protocol.max_frequency_hz", "direct connection requires positive frequency evidence on both ports"))
	} else {
		margin := math.Min(left.MaxFrequencyHz, right.MaxFrequencyHz)
		checks = append(checks, numericPass(CodeFrequencyMismatch, "protocol.max_frequency_hz", "connected protocols have a bounded shared frequency", margin, margin, margin))
	}
	return checks
}

func defaultStateCheck(required, offered string) ContractCheck {
	if required == "" {
		return passCheck(CodeDefaultStateMismatch, "default_state", "requirement does not constrain default state")
	}
	if offered == required {
		return passCheck(CodeDefaultStateMismatch, "default_state", "offered default state satisfies requirement")
	}
	return rejectCheck(CodeDefaultStateMismatch, "default_state", fmt.Sprintf("required default state %q is not provided by %q", required, offered))
}

func traitChecks(required, offered []string) []ContractCheck {
	offeredSet := map[string]bool{}
	for _, trait := range offered {
		offeredSet[trait] = true
	}
	checks := make([]ContractCheck, 0, len(required))
	for _, trait := range required {
		if offeredSet[trait] {
			checks = append(checks, passCheck(CodeTraitMissing, "traits."+trait, "required electrical trait is provided"))
		} else {
			checks = append(checks, rejectCheck(CodeTraitMissing, "traits."+trait, "required electrical trait is missing"))
		}
	}
	return checks
}

func evidenceCheck(required, offered EvidenceConfidence) ContractCheck {
	if required == "" || required == EvidenceUnknown {
		return passCheck(CodeEvidenceInsufficient, "evidence", "requirement does not impose a minimum evidence confidence")
	}
	if confidenceRank(offered) >= confidenceRank(required) {
		return passCheck(CodeEvidenceInsufficient, "evidence", "offered evidence confidence satisfies requirement")
	}
	return rejectCheck(CodeEvidenceInsufficient, "evidence", fmt.Sprintf("required evidence %q is not satisfied by %q", required, offered))
}

func logicConformanceChecks(required LogicLevels, offered *LogicLevels) []ContractCheck {
	if offered == nil {
		return []ContractCheck{rejectCheck(CodeLogicEvidenceMissing, "logic", "offered port lacks required logic-level evidence")}
	}
	checks := []ContractCheck{}
	checks = append(checks, upperBoundCheck(CodeLogicEvidenceMissing, CodeLogicLevelMismatch, "logic.output_low_maximum_v", required.OutputLowMaximumV, offered.OutputLowMaximumV, "offered low output is within required maximum"))
	checks = append(checks, lowerBoundCheck(CodeLogicEvidenceMissing, CodeLogicLevelMismatch, "logic.output_high_minimum_v", required.OutputHighMinimumV, offered.OutputHighMinimumV, "offered high output satisfies required minimum"))
	checks = append(checks, lowerBoundCheck(CodeLogicEvidenceMissing, CodeLogicLevelMismatch, "logic.input_low_maximum_v", required.InputLowMaximumV, offered.InputLowMaximumV, "offered low-input allowance satisfies requirement"))
	checks = append(checks, upperBoundCheck(CodeLogicEvidenceMissing, CodeLogicLevelMismatch, "logic.input_high_minimum_v", required.InputHighMinimumV, offered.InputHighMinimumV, "offered high-input threshold satisfies requirement"))
	return checks
}

func logicConnectionChecks(source, sink *LogicLevels) []ContractCheck {
	if source == nil || sink == nil {
		return []ContractCheck{rejectCheck(CodeLogicEvidenceMissing, "logic", "direct digital connection requires source output and sink input logic evidence")}
	}
	checks := []ContractCheck{}
	if source.OutputLowMaximumV == nil || sink.InputLowMaximumV == nil {
		checks = append(checks, rejectCheck(CodeLogicEvidenceMissing, "logic.low", "low-level compatibility evidence is incomplete"))
	} else {
		margin := *sink.InputLowMaximumV - *source.OutputLowMaximumV
		if margin >= 0 {
			checks = append(checks, numericPass(CodeLogicLevelMismatch, "logic.low", "source low output satisfies sink low threshold", *sink.InputLowMaximumV, *source.OutputLowMaximumV, margin))
		} else {
			checks = append(checks, numericReject(CodeLogicLevelMismatch, "logic.low", "source low output exceeds sink low threshold", *sink.InputLowMaximumV, *source.OutputLowMaximumV, margin))
		}
	}
	if source.OutputHighMinimumV == nil || sink.InputHighMinimumV == nil {
		checks = append(checks, rejectCheck(CodeLogicEvidenceMissing, "logic.high", "high-level compatibility evidence is incomplete"))
	} else {
		margin := *source.OutputHighMinimumV - *sink.InputHighMinimumV
		if margin >= 0 {
			checks = append(checks, numericPass(CodeLogicLevelMismatch, "logic.high", "source high output satisfies sink high threshold", *sink.InputHighMinimumV, *source.OutputHighMinimumV, margin))
		} else {
			checks = append(checks, numericReject(CodeLogicLevelMismatch, "logic.high", "source high output is below sink high threshold", *sink.InputHighMinimumV, *source.OutputHighMinimumV, margin))
		}
	}
	return checks
}

func sourceWithinSinkVoltageCheck(source, sink NumericRange) ContractCheck {
	if source.Minimum == nil || source.Maximum == nil || sink.Minimum == nil || sink.Maximum == nil {
		return rejectCheck(CodeVoltageEvidenceMissing, "voltage", "direct connection requires complete source and sink voltage ranges")
	}
	return rangeContainmentCheck("voltage", sink, source, "source voltage range remains within sink accepted range")
}

func overlappingVoltageCheck(left, right NumericRange) ContractCheck {
	if left.Minimum == nil || left.Maximum == nil || right.Minimum == nil || right.Maximum == nil {
		return rejectCheck(CodeVoltageEvidenceMissing, "voltage", "bidirectional connection requires complete voltage ranges")
	}
	minimum := math.Max(*left.Minimum, *right.Minimum)
	maximum := math.Min(*left.Maximum, *right.Maximum)
	margin := maximum - minimum
	if margin >= 0 {
		return numericPass(CodeVoltageRangeMismatch, "voltage", "bidirectional voltage ranges overlap", minimum, maximum, margin)
	}
	return numericReject(CodeVoltageRangeMismatch, "voltage", "bidirectional voltage ranges do not overlap", minimum, maximum, margin)
}

func connectionCurrentCheck(capacity, demand *float64) ContractCheck {
	if demand == nil {
		return passCheck(CodeCurrentCapacity, "current", "sink does not declare a current demand")
	}
	if capacity == nil {
		return rejectCheck(CodeCurrentEvidenceMissing, "current", "source lacks current-capacity evidence required by sink demand")
	}
	margin := *capacity - *demand
	if margin >= 0 {
		return numericPass(CodeCurrentCapacity, "current", "source current capacity satisfies sink demand", *demand, *capacity, margin)
	}
	return numericReject(CodeCurrentCapacity, "current", "source current capacity is below sink demand", *demand, *capacity, margin)
}

func rangeContainmentCheck(path string, outer, inner NumericRange, message string) ContractCheck {
	if outer.Minimum == nil || outer.Maximum == nil || inner.Minimum == nil || inner.Maximum == nil {
		return rejectCheck(CodeVoltageEvidenceMissing, path, "range-containment evidence is incomplete")
	}
	lowerMargin := *inner.Minimum - *outer.Minimum
	upperMargin := *outer.Maximum - *inner.Maximum
	margin := math.Min(lowerMargin, upperMargin)
	required := *outer.Maximum - *outer.Minimum
	offered := *inner.Maximum - *inner.Minimum
	if margin >= 0 {
		return numericPass(CodeVoltageRangeMismatch, path, message, required, offered, margin)
	}
	return numericReject(CodeVoltageRangeMismatch, path, message, required, offered, margin)
}

func lowerBoundCheck(missingCode, mismatchCode reports.Code, path string, required, offered *float64, message string) ContractCheck {
	if required == nil {
		return passCheck(mismatchCode, path, "requirement does not set a minimum")
	}
	if offered == nil {
		return rejectCheck(missingCode, path, "offered contract lacks required numeric evidence")
	}
	margin := *offered - *required
	if margin >= 0 {
		return numericPass(mismatchCode, path, message, *required, *offered, margin)
	}
	return numericReject(mismatchCode, path, message, *required, *offered, margin)
}

func upperCapacityCheck(missingCode, mismatchCode reports.Code, path string, required, offered *float64, message string) ContractCheck {
	return lowerBoundCheck(missingCode, mismatchCode, path, required, offered, message)
}

func upperBoundCheck(missingCode, mismatchCode reports.Code, path string, maximum, offered *float64, message string) ContractCheck {
	if maximum == nil {
		return passCheck(mismatchCode, path, "requirement does not set a maximum")
	}
	if offered == nil {
		return rejectCheck(missingCode, path, "offered contract lacks required numeric evidence")
	}
	margin := *maximum - *offered
	if margin >= 0 {
		return numericPass(mismatchCode, path, message, *maximum, *offered, margin)
	}
	return numericReject(mismatchCode, path, message, *maximum, *offered, margin)
}

func equalityCheck(code reports.Code, path, required, offered, message string) ContractCheck {
	if required == offered {
		return passCheck(code, path, message)
	}
	return rejectCheck(code, path, fmt.Sprintf("required %q does not match offered %q", required, offered))
}

func passCheck(code reports.Code, path, message string) ContractCheck {
	return ContractCheck{Code: code, Status: ContractCheckPass, Path: path, Message: message}
}

func rejectCheck(code reports.Code, path, message string) ContractCheck {
	return ContractCheck{Code: code, Status: ContractCheckReject, Path: path, Message: message}
}

func numericPass(code reports.Code, path, message string, required, offered, margin float64) ContractCheck {
	return ContractCheck{Code: code, Status: ContractCheckPass, Path: path, Message: message, Required: float64Pointer(required), Offered: float64Pointer(offered), Margin: float64Pointer(margin)}
}

func numericReject(code reports.Code, path, message string, required, offered, margin float64) ContractCheck {
	return ContractCheck{Code: code, Status: ContractCheckReject, Path: path, Message: message, Required: float64Pointer(required), Offered: float64Pointer(offered), Margin: float64Pointer(margin)}
}

func finalizeCompatibility(checks []ContractCheck) CompatibilityReport {
	filtered := make([]ContractCheck, 0, len(checks))
	compatible := true
	for _, check := range checks {
		if check.Path == "" {
			continue
		}
		filtered = append(filtered, check)
		if check.Status == ContractCheckReject {
			compatible = false
		}
	}
	slices.SortStableFunc(filtered, func(left, right ContractCheck) int {
		if order := strings.Compare(left.Path, right.Path); order != 0 {
			return order
		}
		if order := strings.Compare(string(left.Code), string(right.Code)); order != 0 {
			return order
		}
		return cmp.Compare(left.Status, right.Status)
	})
	return CompatibilityReport{Compatible: compatible, Checks: filtered}
}

func confidenceRank(confidence EvidenceConfidence) int {
	switch confidence {
	case EvidenceVerified:
		return 5
	case EvidenceLibraryDerived:
		return 4
	case EvidenceRuleInferred:
		return 3
	case EvidencePlaceholder:
		return 2
	case EvidenceUnknown:
		return 1
	default:
		return 0
	}
}

func validEvidenceConfidence(confidence EvidenceConfidence) bool {
	switch confidence {
	case EvidenceBlocked, EvidenceUnknown, EvidencePlaceholder, EvidenceRuleInferred, EvidenceLibraryDerived, EvidenceVerified:
		return true
	default:
		return false
	}
}

func cloneFloat64(value *float64) *float64 {
	if value == nil {
		return nil
	}
	return float64Pointer(*value)
}

func float64Pointer(value float64) *float64 {
	return &value
}

func minPointer(current *float64, candidate float64) *float64 {
	if current == nil || candidate < *current {
		return float64Pointer(candidate)
	}
	return current
}

func maxPointer(current *float64, candidate float64) *float64 {
	if current == nil || candidate > *current {
		return float64Pointer(candidate)
	}
	return current
}
