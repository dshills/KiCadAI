package architecturesearch

import (
	"encoding/json"
	"math"
	"reflect"
	"slices"
	"testing"

	"kicadai/internal/components"
	"kicadai/internal/reports"
)

func TestContractFromRequirementPortPreservesElectricalMeaning(t *testing.T) {
	requirement := validRequirement()
	contract, issues := ContractFromRequirementPort(requirement, "power", EvidenceVerified)
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	if contract.ID != "power" || contract.Kind != "power" || contract.Direction != "sink" || contract.Domain != "vcc" {
		t.Fatalf("contract identity = %#v", contract)
	}
	if contract.Voltage.Minimum == nil || *contract.Voltage.Minimum != 4.75 || contract.Voltage.Maximum == nil || *contract.Voltage.Maximum != 5.25 {
		t.Fatalf("contract voltage = %#v", contract.Voltage)
	}
	if contract.MaximumCurrentDemandA == nil || *contract.MaximumCurrentDemandA != 0.02 || contract.RequiredCurrentCapacityA != nil {
		t.Fatalf("contract current = %#v", contract)
	}
	if contract.MinimumEvidence != EvidenceVerified || contract.Evidence.Confidence != EvidenceUnknown {
		t.Fatalf("contract evidence = %#v minimum=%s", contract.Evidence, contract.MinimumEvidence)
	}

	requirement.Requirements.Ports = append(requirement.Requirements.Ports, Port{
		ID: "drive", Kind: "digital_logic", Direction: "source", Domain: "vcc",
		Electrical: &Electrical{MinVoltageV: floatPointer(0), MaxVoltageV: floatPointer(5), MaxSourceCurrentMA: floatPointer(2)},
	})
	requirement.Requirements.Objectives[0].Bindings = append(requirement.Requirements.Objectives[0].Bindings, Binding{Role: "drive", Port: "drive"})
	drive, issues := ContractFromRequirementPort(requirement, "drive", EvidenceRuleInferred)
	if len(issues) != 0 {
		t.Fatalf("drive issues = %#v", issues)
	}
	if drive.RequiredCurrentCapacityA == nil || *drive.RequiredCurrentCapacityA != 0.002 || drive.MaximumCurrentDemandA != nil {
		t.Fatalf("drive current = %#v", drive)
	}
}

func TestContractFromBindingSupportsAbstractParticipantPorts(t *testing.T) {
	requirement := validRequirement()
	requirement.Requirements.Domains = append(requirement.Requirements.Domains, Domain{ID: "logic_1v8", Kind: "supply", MinVoltageV: floatPointer(1.71), NominalVoltageV: 1.8, MaxVoltageV: floatPointer(1.89), Source: "external"})
	requirement.Requirements.Participants = []Participant{{
		ID: "sensor", Capability: "environment_sensor", Domain: "logic_1v8",
		RequiredPorts: []ParticipantPort{{ID: "bus", Kind: "digital_bus", Direction: "bidirectional", Protocol: &Protocol{Name: "i2c", Mode: "open_drain", MaxFrequencyHz: 400000}}},
		Constraints:   []Constraint{{Name: "measurement", Relation: "equal", Value: json.RawMessage(`"temperature"`)}},
	}}
	requirement.Requirements.Objectives[0].Bindings = append(requirement.Requirements.Objectives[0].Bindings, Binding{Role: "sensor_bus", Participant: "sensor", ParticipantPort: "bus"})
	contract, issues := ContractFromBinding(requirement, Binding{Role: "sensor_bus", Participant: "sensor", ParticipantPort: "bus"}, EvidenceVerified)
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	if contract.ID != "sensor.bus" || contract.Kind != "digital_bus" || contract.Direction != "bidirectional" || contract.Domain != "logic_1v8" {
		t.Fatalf("participant contract = %#v", contract)
	}
	if contract.Voltage.Minimum == nil || *contract.Voltage.Minimum != 1.71 || contract.Voltage.Maximum == nil || *contract.Voltage.Maximum != 1.89 {
		t.Fatalf("participant voltage = %#v", contract.Voltage)
	}
	if contract.Protocol == nil || contract.Protocol.Name != "i2c" || contract.Protocol.Mode != "open_drain" || contract.FrequencyMaxHz == nil || *contract.FrequencyMaxHz != 400000 {
		t.Fatalf("participant protocol = %#v", contract)
	}
}

func TestSatisfiesPortRequirementProvesSinkAndSourceContracts(t *testing.T) {
	requiredSink := PortContract{
		ID: "sense", Kind: "analog_voltage", Direction: "sink", Domain: "logic_3v3",
		Voltage:               NumericRange{Minimum: float64Pointer(0), Maximum: float64Pointer(3.3)},
		MaximumCurrentDemandA: float64Pointer(0.004), InputImpedanceMinOhm: float64Pointer(100000),
		FrequencyMaxHz: float64Pointer(100000), DefaultState: "inactive",
		RequiredTraits: []string{"input_protected"}, MinimumEvidence: EvidenceVerified,
	}
	offeredSink := PortContract{
		ID: "input", Kind: "analog_voltage", Direction: "sink", Domain: "logic_3v3",
		Voltage:        NumericRange{Minimum: float64Pointer(-0.3), Maximum: float64Pointer(5.5)},
		CurrentDemandA: float64Pointer(0.000001), InputImpedanceMinOhm: float64Pointer(1000000),
		FrequencyMaxHz: float64Pointer(1000000), DefaultState: "inactive", Traits: []string{"input_protected", "esd"},
		Evidence: ContractEvidence{Confidence: EvidenceVerified, Sources: []string{"datasheet"}},
	}
	if report := SatisfiesPortRequirement(requiredSink, offeredSink); !report.Compatible || hasRejectedCheck(report, "") {
		t.Fatalf("valid sink report = %#v", report)
	}

	requiredSource := PortContract{
		ID: "output", Kind: "power", Direction: "source", Domain: "regulated_5v",
		Voltage:                  NumericRange{Minimum: float64Pointer(4.9), Maximum: float64Pointer(5.1)},
		RequiredCurrentCapacityA: float64Pointer(0.25), MinimumEvidence: EvidenceRuleInferred,
	}
	offeredSource := PortContract{
		ID: "out", Kind: "power", Direction: "source", Domain: "regulated_5v",
		Voltage:          NumericRange{Minimum: float64Pointer(4.95), Maximum: float64Pointer(5.05)},
		CurrentCapacityA: float64Pointer(0.5), Evidence: ContractEvidence{Confidence: EvidenceVerified},
	}
	if report := SatisfiesPortRequirement(requiredSource, offeredSource); !report.Compatible {
		t.Fatalf("valid source report = %#v", report)
	}
	offeredSource.Voltage = NumericRange{Minimum: float64Pointer(4.8), Maximum: float64Pointer(5.2)}
	if report := SatisfiesPortRequirement(requiredSource, offeredSource); report.Compatible || !hasRejectedCheck(report, CodeVoltageRangeMismatch) {
		t.Fatalf("out-of-window source report = %#v", report)
	}
}

func TestSatisfiesPortRequirementFailsClosedOnMissingOrInsufficientEvidence(t *testing.T) {
	required := PortContract{
		Kind: "digital_logic", Direction: "sink", Domain: "logic_3v3",
		Voltage:               NumericRange{Minimum: float64Pointer(0), Maximum: float64Pointer(3.3)},
		MaximumCurrentDemandA: float64Pointer(0.004), InputImpedanceMinOhm: float64Pointer(100000),
		FrequencyMaxHz: float64Pointer(400000), MinimumEvidence: EvidenceVerified,
		Logic: &LogicLevels{InputLowMaximumV: float64Pointer(0.8), InputHighMinimumV: float64Pointer(2)},
	}
	offered := PortContract{
		Kind: "digital_logic", Direction: "sink", Domain: "logic_3v3",
		Voltage: NumericRange{}, CurrentDemandA: float64Pointer(0.01),
		Evidence: ContractEvidence{Confidence: EvidencePlaceholder},
	}
	report := SatisfiesPortRequirement(required, offered)
	for _, code := range []reports.Code{CodeVoltageEvidenceMissing, CodeCurrentCapacity, CodeImpedanceEvidenceMissing, CodeFrequencyEvidenceMissing, CodeLogicEvidenceMissing, CodeEvidenceInsufficient} {
		if !hasRejectedCheck(report, code) {
			t.Fatalf("report lacks rejection %s: %#v", code, report)
		}
	}
	if report.Compatible {
		t.Fatal("incomplete safety evidence was accepted")
	}
}

func TestConnectPortsChecksVoltageCurrentAndLogicLevels(t *testing.T) {
	source := PortContract{
		ID: "gpio_out", Kind: "digital_logic", Direction: "source", Domain: "logic_3v3",
		Voltage: NumericRange{Minimum: float64Pointer(0), Maximum: float64Pointer(3.3)}, CurrentCapacityA: float64Pointer(0.004),
		Logic: &LogicLevels{OutputLowMaximumV: float64Pointer(0.4), OutputHighMinimumV: float64Pointer(2.9)},
	}
	sink := PortContract{
		ID: "logic_in", Kind: "digital_logic", Direction: "sink", Domain: "logic_3v3",
		Voltage: NumericRange{Minimum: float64Pointer(-0.3), Maximum: float64Pointer(3.6)}, CurrentDemandA: float64Pointer(0.001),
		Logic: &LogicLevels{InputLowMaximumV: float64Pointer(0.8), InputHighMinimumV: float64Pointer(2)},
	}
	if report := ConnectPorts(source, sink); !report.Compatible {
		t.Fatalf("valid logic connection = %#v", report)
	}

	sink.Domain = "logic_1v8"
	sink.Voltage.Maximum = float64Pointer(1.89)
	sink.Logic.InputHighMinimumV = float64Pointer(1.2)
	report := ConnectPorts(source, sink)
	if report.Compatible || !hasRejectedCheck(report, CodeDomainMismatch) || !hasRejectedCheck(report, CodeVoltageRangeMismatch) {
		t.Fatalf("cross-domain connection = %#v", report)
	}

	sink.Domain = "logic_3v3"
	sink.Voltage.Maximum = float64Pointer(3.6)
	sink.Logic = nil
	report = ConnectPorts(source, sink)
	if report.Compatible || !hasRejectedCheck(report, CodeLogicEvidenceMissing) {
		t.Fatalf("missing logic evidence = %#v", report)
	}

	sink.Logic = &LogicLevels{InputLowMaximumV: float64Pointer(0.2), InputHighMinimumV: float64Pointer(3.1)}
	report = ConnectPorts(source, sink)
	if report.Compatible || !hasRejectedCheck(report, CodeLogicLevelMismatch) {
		t.Fatalf("insufficient logic margin = %#v", report)
	}
}

func TestConnectBidirectionalPortsRequiresSameDomainAndProtocol(t *testing.T) {
	left := PortContract{
		ID: "bus_a", Kind: "digital_bus", Direction: "bidirectional", Domain: "logic_3v3",
		Voltage:  NumericRange{Minimum: float64Pointer(0), Maximum: float64Pointer(3.6)},
		Protocol: &Protocol{Name: "i2c", Mode: "open_drain", MaxFrequencyHz: 400000},
	}
	right := PortContract{
		ID: "bus_b", Kind: "digital_bus", Direction: "bidirectional", Domain: "logic_3v3",
		Voltage:  NumericRange{Minimum: float64Pointer(0), Maximum: float64Pointer(3.6)},
		Protocol: &Protocol{Name: "i2c", Mode: "open_drain", MaxFrequencyHz: 1000000},
	}
	if report := ConnectPorts(left, right); !report.Compatible {
		t.Fatalf("valid open-drain bus = %#v", report)
	}
	right.Domain = "logic_1v8"
	if report := ConnectPorts(left, right); report.Compatible || !hasRejectedCheck(report, CodeDomainMismatch) {
		t.Fatalf("cross-domain bus = %#v", report)
	}
	right.Domain = "logic_3v3"
	right.Protocol = nil
	if report := ConnectPorts(left, right); report.Compatible || !hasRejectedCheck(report, CodeProtocolMismatch) {
		t.Fatalf("missing protocol = %#v", report)
	}
}

func TestCompatibilityReportAndContractNormalizationAreOrderNeutral(t *testing.T) {
	required := PortContract{
		Kind: "digital_logic", Direction: "source", Domain: "logic_3v3",
		Voltage:        NumericRange{Minimum: float64Pointer(0), Maximum: float64Pointer(3.3)},
		RequiredTraits: []string{"default_off", "protected", "default_off"}, MinimumEvidence: EvidenceRuleInferred,
	}
	offered := PortContract{
		Kind: "digital_logic", Direction: "source", Domain: "logic_3v3",
		Voltage:  NumericRange{Minimum: float64Pointer(0), Maximum: float64Pointer(3.3)},
		Traits:   []string{"protected", "default_off"},
		Evidence: ContractEvidence{Confidence: EvidenceVerified, Sources: []string{"z_source", "a_source", "z_source"}},
	}
	first := SatisfiesPortRequirement(required, offered)
	slices.Reverse(required.RequiredTraits)
	slices.Reverse(offered.Traits)
	slices.Reverse(offered.Evidence.Sources)
	second := SatisfiesPortRequirement(required, offered)
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("order changed report\nfirst=%#v\nsecond=%#v", first, second)
	}
	if normalized := NormalizePortContract(offered); !slices.IsSorted(normalized.Traits) || !slices.IsSorted(normalized.Evidence.Sources) || len(normalized.Evidence.Sources) != 2 {
		t.Fatalf("normalized contract = %#v", normalized)
	}
}

func TestNewComponentContractBindsCatalogEvidenceAndFailsClosed(t *testing.T) {
	record := components.ComponentRecord{
		ID: "verified_part", Family: "logic",
		Verification: components.VerificationRecord{Confidence: components.ConfidenceVerified, Sources: []string{"datasheet", "pinmap"}},
	}
	contract, issues := NewComponentContract(record, []PortContract{{ID: "out", Kind: "digital_logic", Direction: "source", Domain: "logic_3v3", Voltage: NumericRange{Minimum: float64Pointer(0), Maximum: float64Pointer(3.3)}}})
	if len(issues) != 0 {
		t.Fatalf("verified issues = %#v", issues)
	}
	if contract.Evidence.Confidence != EvidenceVerified || contract.Ports[0].Evidence.Confidence != EvidenceVerified {
		t.Fatalf("catalog evidence not propagated: %#v", contract)
	}
	encoded, err := json.Marshal(contract)
	if err != nil || len(encoded) == 0 {
		t.Fatalf("marshal contract: %v", err)
	}

	record.Verification.Confidence = components.ConfidencePlaceholder
	if _, issues := NewComponentContract(record, nil); !containsIssue(issues, CodeEvidenceInsufficient, "component.evidence") {
		t.Fatalf("placeholder issues = %#v", issues)
	}
	record.Verification.Confidence = components.ConfidenceBlocked
	if _, issues := NewComponentContract(record, nil); !containsIssue(issues, CodeEvidenceInsufficient, "component.evidence") {
		t.Fatalf("blocked issues = %#v", issues)
	}
	record.Verification.Confidence = components.ConfidenceVerified
	if _, issues := NewComponentContract(record, []PortContract{{ID: "bad", Kind: "digital_logic", Direction: "source", Domain: "logic_3v3"}}); !containsIssue(issues, CodeVoltageEvidenceMissing, ".voltage") {
		t.Fatalf("missing port voltage issues = %#v", issues)
	}
}

func TestCompatibilityRejectsInvalidContractEvenWhenBothSidesMatch(t *testing.T) {
	invalid := PortContract{
		Kind: "invented", Direction: "source", Domain: "logic_3v3",
		Voltage:  NumericRange{Minimum: float64Pointer(math.NaN()), Maximum: float64Pointer(3.3)},
		Evidence: ContractEvidence{Confidence: EvidenceVerified},
	}
	report := SatisfiesPortRequirement(invalid, invalid)
	if report.Compatible || !hasRejectedCheck(report, CodeContractInvalid) {
		t.Fatalf("invalid matching contracts were accepted: %#v", report)
	}
}

func hasRejectedCheck(report CompatibilityReport, code reports.Code) bool {
	for _, check := range report.Checks {
		if check.Status == ContractCheckReject && (code == "" || check.Code == code) {
			return true
		}
	}
	return false
}
