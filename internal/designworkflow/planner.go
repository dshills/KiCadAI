package designworkflow

import (
	"context"
	"strconv"
	"strings"

	"kicadai/internal/amplifiers"
	"kicadai/internal/blocks"
	"kicadai/internal/reports"
)

const defaultHeadphoneDCCapacitance = "220uF"

type BlockPlanResult struct {
	Request     Request                   `json:"request"`
	Composition blocks.CompositionRequest `json:"composition"`
	Output      blocks.CompositionOutput  `json:"output"`
	Stage       StageResult               `json:"stage"`
}

func PlanBlocks(ctx context.Context, registry blocks.Registry, request Request) BlockPlanResult {
	normalized := NormalizeRequest(request)
	var issues []reports.Issue
	issues = append(issues, ValidateRequest(normalized)...)
	if registry == nil {
		issues = append(issues, reports.Issue{
			Code:     reports.CodeInvalidArgument,
			Severity: reports.SeverityError,
			Path:     "registry",
			Message:  "block registry is required",
		})
	}
	composition, compositionIssues := ToCompositionRequest(normalized)
	issues = append(issues, compositionIssues...)
	if !reports.HasBlockingIssue(issues) && registry != nil {
		issues = append(issues, validateBlocksAgainstRegistry(registry, normalized)...)
	}
	if reports.HasBlockingIssue(issues) {
		stage := NewStageResult(StageBlockPlanning, issues)
		populateBlockPlanningSummaries(&stage, normalized, issues)
		return BlockPlanResult{
			Request:     normalized,
			Composition: composition,
			Stage:       stage,
		}
	}
	output := blocks.ComposeBlocks(ctx, registry, composition)
	issues = append(issues, output.Issues...)
	evidence, evidenceIssues := blockEvidenceForRequest(ctx, registry, normalized)
	issues = append(issues, evidenceIssues...)
	stage := NewStageResult(StageBlockPlanning, issues)
	stage.Summary = map[string]any{
		"block_count":      len(normalized.Blocks),
		"connection_count": len(normalized.Connections),
		"operation_count":  len(output.Operations),
		"block_evidence":   evidence,
	}
	populateBlockPlanningSummaries(&stage, normalized, issues)
	return BlockPlanResult{
		Request:     normalized,
		Composition: composition,
		Output:      output,
		Stage:       stage,
	}
}

func populateBlockPlanningSummaries(stage *StageResult, request Request, issues []reports.Issue) {
	if stage.Summary == nil {
		stage.Summary = map[string]any{}
	}
	if amplifierSummaries := amplifierOutputStageSummaries(request, issues); len(amplifierSummaries) != 0 {
		stage.Summary["amplifier_output_stages"] = amplifierSummaries
		if len(amplifierSummaries) == 1 {
			stage.Summary["amplifier_output_stage"] = amplifierSummaries[0]
		}
	}
	if protectionSummaries := headphoneOutputProtectionSummaries(request, issues); len(protectionSummaries) != 0 {
		stage.Summary["headphone_output_protections"] = protectionSummaries
		if len(protectionSummaries) == 1 {
			stage.Summary["headphone_output_protection"] = protectionSummaries[0]
		}
	}
}

type AmplifierOutputStageSummary struct {
	InstanceID        string                      `json:"instance_id"`
	BlockID           string                      `json:"block_id"`
	Topology          string                      `json:"topology,omitempty"`
	SupplyVoltage     string                      `json:"supply_voltage,omitempty"`
	LoadImpedance     string                      `json:"load_impedance,omitempty"`
	OutputDevices     []string                    `json:"output_devices,omitempty"`
	DCBlockingPresent bool                        `json:"dc_blocking_present"`
	SimulationStatus  amplifiers.SimulationStatus `json:"simulation_status"`
	Readiness         string                      `json:"readiness"`
	Notes             []string                    `json:"notes,omitempty"`
	Blockers          []string                    `json:"blockers,omitempty"`
}

type HeadphoneOutputProtectionSummary struct {
	InstanceID              string   `json:"instance_id"`
	BlockID                 string   `json:"block_id"`
	LoadKind                string   `json:"load_kind"`
	NominalLoadOhms         string   `json:"nominal_load_ohms"`
	ACOutputCouplingPresent bool     `json:"ac_output_coupling_present"`
	DCBlockingCapacitance   string   `json:"dc_blocking_capacitance,omitempty"`
	BleedPolicyStatus       string   `json:"bleed_policy_status"`
	SeriesResistorStatus    string   `json:"series_resistor_status"`
	ConnectorReturnStatus   string   `json:"connector_return_status"`
	FaultProtectionStatus   string   `json:"fault_protection_status"`
	Readiness               string   `json:"readiness"`
	Notes                   []string `json:"notes,omitempty"`
	Blockers                []string `json:"blockers,omitempty"`
}

func amplifierOutputStageSummaries(request Request, issues []reports.Issue) []AmplifierOutputStageSummary {
	var summaries []AmplifierOutputStageSummary
	blocksByInstance := blocksByInstanceID(request)
	for index, instance := range request.Blocks {
		switch instance.BlockID {
		case "class_ab_output_stage", "class_ab_output_pair":
			summaries = append(summaries, amplifierOutputStageSummary(request, issues, instance, index, blocksByInstance))
		}
	}
	return summaries
}

func headphoneOutputProtectionSummaries(request Request, issues []reports.Issue) []HeadphoneOutputProtectionSummary {
	var summaries []HeadphoneOutputProtectionSummary
	var connectedPorts map[string]map[string]bool
	for index, instance := range request.Blocks {
		if instance.BlockID != "headphone_output_protection" {
			continue
		}
		if connectedPorts == nil {
			connectedPorts = connectedPortsByInstance(request)
		}
		summaries = append(summaries, headphoneOutputProtectionSummary(issues, instance, index, connectedPorts))
	}
	return summaries
}

func headphoneOutputProtectionSummary(issues []reports.Issue, instance BlockInstanceSpec, blockIndex int, connectedPorts map[string]map[string]bool) HeadphoneOutputProtectionSummary {
	summary := HeadphoneOutputProtectionSummary{
		InstanceID:              instance.ID,
		BlockID:                 instance.BlockID,
		LoadKind:                stringParamSummaryDefault(instance.Params, "load_kind", "headphone"),
		NominalLoadOhms:         stringParamSummaryDefault(instance.Params, "nominal_load_ohms", "32Ω"),
		DCBlockingCapacitance:   stringParamSummaryDefault(instance.Params, "dc_blocking_capacitance", defaultHeadphoneDCCapacitance),
		ACOutputCouplingPresent: headphoneProtectionHasDCBlocking(instance),
		BleedPolicyStatus:       bleedPolicyStatus(instance),
		SeriesResistorStatus:    seriesResistorStatus(instance),
		ConnectorReturnStatus:   connectorReturnStatus(connectedPorts, instance),
		FaultProtectionStatus:   stringParamSummaryDefault(instance.Params, "fault_protection_status", "placeholder_blocked"),
		Readiness:               "connectivity",
	}
	if summary.ACOutputCouplingPresent {
		summary.Notes = append(summary.Notes, "AC output coupling is present for single-supply headphone connectivity.")
	} else if headphoneProtectionCouplingMode(instance) == "dual_rail_direct_review_required" {
		summary.Blockers = append(summary.Blockers, "dual-rail direct-coupled headphone output requires verified offset and load-safety review")
	} else {
		summary.Blockers = append(summary.Blockers, "positive DC-blocking capacitance is required for single-supply headphone output")
	}
	if summary.LoadKind != "headphone" {
		summary.Blockers = append(summary.Blockers, "only headphone loads are supported; speaker, bridge, and unknown outputs remain blocked")
	}
	if summary.BleedPolicyStatus != "present" && summary.BleedPolicyStatus != "not_required" {
		if summary.BleedPolicyStatus == "shorted" {
			summary.Blockers = append(summary.Blockers, "required bleed/reference resistor must be positive; 0Ω would short the headphone output reference")
		} else {
			summary.Blockers = append(summary.Blockers, "required bleed/reference policy is missing or invalid")
		}
	}
	if summary.ConnectorReturnStatus != "load_return_and_reference_connected" {
		summary.Blockers = append(summary.Blockers, "headphone connector return and reference must both be connected")
	}
	if summary.FaultProtectionStatus != "connectivity" {
		summary.Notes = append(summary.Notes, "Fault protection is not KiCad-verified yet and remains a higher-readiness warning.")
	}
	for _, issue := range issues {
		if issue.Message == "" || !issueBelongsToBlock(issue, blockIndex) {
			continue
		}
		if issue.Blocking() {
			summary.Blockers = append(summary.Blockers, issue.Message)
			continue
		}
		summary.Notes = append(summary.Notes, issue.Message)
	}
	if len(summary.Blockers) != 0 {
		summary.Readiness = "blocked"
	}
	return summary
}

func amplifierOutputStageSummary(request Request, issues []reports.Issue, stage BlockInstanceSpec, stageIndex int, blocksByInstance map[string]BlockInstanceSpec) AmplifierOutputStageSummary {
	dcBlockingPresent := classABHasOutputCoupling(request, stage, blocksByInstance)
	summary := AmplifierOutputStageSummary{
		InstanceID:    stage.ID,
		BlockID:       stage.BlockID,
		Topology:      stringParamSummary(stage.Params, "topology"),
		SupplyVoltage: stringParamSummary(stage.Params, "supply_voltage"),
		LoadImpedance: stringParamSummary(stage.Params, "load_impedance"),
		OutputDevices: []string{
			stringParamSummaryDefault(stage.Params, "upper_output_component_id", "bjt.onsemi.mmbt3904.sot23"),
			stringParamSummaryDefault(stage.Params, "lower_output_component_id", "bjt.onsemi.mmbt3906.sot23"),
		},
		DCBlockingPresent: dcBlockingPresent,
		SimulationStatus:  amplifiers.SimulationStatusNotRun,
		Readiness:         "headphone_connectivity",
	}
	summary.Notes = []string{
		"Class AB output stage is limited to headphone-class loads within the derated current envelope of the selected output devices: " + strings.Join(summary.OutputDevices, ", ") + ".",
		"Thermal design, quiescent-current trimming, VBE multiplier support, and speaker/power-amplifier use remain blocked.",
	}
	if classABRequiresDCBlocking(request, stage) && !dcBlockingPresent {
		summary.Readiness = "blocked"
		summary.Blockers = append(summary.Blockers, "single-supply headphone outputs require a DC blocking capacitor before the load")
	}
	for _, issue := range issues {
		if issue.Message == "" || !issueBelongsToBlock(issue, stageIndex) {
			continue
		}
		if issue.Blocking() {
			summary.Blockers = append(summary.Blockers, issue.Message)
			continue
		}
		summary.Notes = append(summary.Notes, issue.Message)
	}
	if len(summary.Blockers) != 0 {
		summary.Readiness = "blocked"
	}
	return summary
}

func classABRequiresDCBlocking(request Request, stage BlockInstanceSpec) bool {
	veeEndpoint := stage.ID + ".VEE"
	for _, connection := range request.Connections {
		if !strings.EqualFold(connection.From, veeEndpoint) && !strings.EqualFold(connection.To, veeEndpoint) {
			continue
		}
		from, fromOK := ParseEndpoint(connection.From)
		to, toOK := ParseEndpoint(connection.To)
		if fromOK && toOK {
			other := from
			if other.InstanceID == stage.ID && strings.EqualFold(other.Port, "VEE") {
				other = to
			}
			if other.InstanceID != stage.ID && isNegativeRailAlias(other.Port) {
				return false
			}
		}
		alias := strings.ToUpper(strings.TrimSpace(connection.NetAlias))
		if isNegativeRailAlias(alias) {
			return false
		}
	}
	return true
}

func classABHasOutputCoupling(request Request, stage BlockInstanceSpec, blocksByInstance map[string]BlockInstanceSpec) bool {
	outputAliases := map[string]struct{}{}
	dcBlockAliases := map[string]struct{}{}
	for _, connection := range request.Connections {
		from, fromOK := ParseEndpoint(connection.From)
		to, toOK := ParseEndpoint(connection.To)
		fromIsOutput := fromOK && endpointIsClassABOutput(from, stage)
		toIsOutput := toOK && endpointIsClassABOutput(to, stage)
		fromIsDCBlock := fromOK && endpointIsDCBlock(from, blocksByInstance)
		toIsDCBlock := toOK && endpointIsDCBlock(to, blocksByInstance)
		if fromIsOutput && toIsDCBlock {
			return true
		}
		if toIsOutput && fromIsDCBlock {
			return true
		}
		if fromIsOutput || toIsOutput {
			if alias := strings.TrimSpace(connection.NetAlias); alias != "" {
				outputAliases[alias] = struct{}{}
			}
		}
		if fromIsDCBlock || toIsDCBlock {
			if alias := strings.TrimSpace(connection.NetAlias); alias != "" {
				dcBlockAliases[alias] = struct{}{}
			}
		}
	}
	for alias := range outputAliases {
		if _, ok := dcBlockAliases[alias]; ok {
			return true
		}
	}
	return false
}

func blocksByInstanceID(request Request) map[string]BlockInstanceSpec {
	instances := make(map[string]BlockInstanceSpec, len(request.Blocks))
	for _, instance := range request.Blocks {
		instances[instance.ID] = instance
	}
	return instances
}

func endpointIsClassABOutput(endpoint blocks.PortRef, stage BlockInstanceSpec) bool {
	return endpoint.InstanceID == stage.ID && strings.EqualFold(endpoint.Port, "AMP_OUT")
}

func endpointIsDCBlock(endpoint blocks.PortRef, blocksByInstance map[string]BlockInstanceSpec) bool {
	instance, ok := blocksByInstance[endpoint.InstanceID]
	if !ok {
		return false
	}
	switch instance.BlockID {
	case "dc_blocking_capacitor":
		return hasPositiveCapacitance(instance, "capacitance", defaultHeadphoneDCCapacitance)
	case "headphone_output_protection":
		return strings.EqualFold(endpoint.Port, "AMP_OUT") && headphoneProtectionHasDCBlocking(instance)
	default:
		return false
	}
}

func headphoneProtectionHasDCBlocking(instance BlockInstanceSpec) bool {
	if headphoneProtectionCouplingMode(instance) == "dual_rail_direct_review_required" {
		return false
	}
	return hasPositiveCapacitance(instance, "dc_blocking_capacitance", defaultHeadphoneDCCapacitance)
}

func headphoneProtectionCouplingMode(instance BlockInstanceSpec) string {
	return strings.TrimSpace(stringParamSummaryDefault(instance.Params, "coupling", "ac_coupled_required"))
}

func hasPositiveCapacitance(instance BlockInstanceSpec, key string, defaultValue string) bool {
	capacitance := strings.TrimSpace(stringParamSummaryDefault(instance.Params, key, defaultValue))
	if capacitance == "" {
		return false
	}
	return hasPositiveNumericPrefix(capacitance)
}

func hasPositiveNumericPrefix(value string) bool {
	number, ok := numericPrefix(value)
	return ok && number > 0
}

func numericPrefix(value string) (float64, bool) {
	value = strings.TrimSpace(value)
	end := 0
	if end < len(value) && (value[end] == '+' || value[end] == '-') {
		end++
	}
	mantissaStart := end
	dotSeen := false
	for end < len(value) {
		char := value[end]
		if char >= '0' && char <= '9' {
			end++
			continue
		}
		if char == '.' && !dotSeen {
			dotSeen = true
			end++
			continue
		}
		break
	}
	if end == mantissaStart || value[mantissaStart:end] == "." {
		return 0, false
	}
	numberEnd := end
	if end < len(value) && (value[end] == 'e' || value[end] == 'E') {
		exponentEnd := end + 1
		if exponentEnd < len(value) && (value[exponentEnd] == '+' || value[exponentEnd] == '-') {
			exponentEnd++
		}
		exponentDigitsStart := exponentEnd
		for exponentEnd < len(value) {
			char := value[exponentEnd]
			if char < '0' || char > '9' {
				break
			}
			exponentEnd++
		}
		if exponentEnd > exponentDigitsStart {
			numberEnd = exponentEnd
		}
	}
	number, err := strconv.ParseFloat(value[:numberEnd], 64)
	return number, err == nil
}

func bleedPolicyStatus(instance BlockInstanceSpec) string {
	required := boolParamSummaryDefault(instance.Params, "bleed_required", true)
	if !required {
		return "not_required"
	}
	value := stringParamSummaryDefault(instance.Params, "bleed_resistor_ohms", "100kΩ")
	resistance, ok := numericPrefix(value)
	if !ok {
		return "blocked"
	}
	if resistance == 0 {
		return "shorted"
	}
	if resistance < 0 {
		return "blocked"
	}
	return "present"
}

func seriesResistorStatus(instance BlockInstanceSpec) string {
	value := stringParamSummaryDefault(instance.Params, "series_resistor_ohms", "0Ω")
	resistance, ok := numericPrefix(value)
	if !ok || resistance < 0 {
		return "blocked"
	}
	if resistance == 0 {
		return "omitted"
	}
	return "present"
}

func connectorReturnStatus(connectedPorts map[string]map[string]bool, instance BlockInstanceSpec) string {
	ports := connectedPorts[instance.ID]
	loadRetConnected := ports["LOAD_RET"]
	loadRefConnected := ports["LOAD_REF"]
	switch {
	case loadRetConnected && loadRefConnected:
		return "load_return_and_reference_connected"
	case loadRetConnected:
		return "missing_reference_connection"
	case loadRefConnected:
		return "missing_return_connection"
	default:
		return "missing_return_and_reference_connections"
	}
}

func connectedPortsByInstance(request Request) map[string]map[string]bool {
	connected := map[string]map[string]bool{}
	for _, connection := range request.Connections {
		recordConnectedPort(connected, connection.From)
		recordConnectedPort(connected, connection.To)
	}
	return connected
}

func recordConnectedPort(connected map[string]map[string]bool, endpointText string) {
	endpoint, ok := ParseEndpoint(endpointText)
	if !ok {
		return
	}
	ports := connected[endpoint.InstanceID]
	if ports == nil {
		ports = map[string]bool{}
		connected[endpoint.InstanceID] = ports
	}
	ports[strings.ToUpper(endpoint.Port)] = true
}

func issueBelongsToBlock(issue reports.Issue, blockIndex int) bool {
	if blockIndex < 0 {
		return false
	}
	prefix := "blocks[" + strconv.Itoa(blockIndex) + "]"
	return issue.Path == prefix || strings.HasPrefix(issue.Path, prefix+".")
}

func isNegativeRailAlias(alias string) bool {
	alias = strings.ToUpper(strings.TrimSpace(alias))
	return alias == "VEE" ||
		alias == "VSS" ||
		alias == "VNEG" ||
		alias == "V-" ||
		isNegativeVoltageAlias(alias)
}

func isNegativeVoltageAlias(alias string) bool {
	if len(alias) < 3 || alias[0] != '-' || !strings.HasSuffix(alias, "V") {
		return false
	}
	return alias[1] >= '0' && alias[1] <= '9'
}

func stringParamSummary(params map[string]any, key string) string {
	if params == nil {
		return ""
	}
	switch value := params[key].(type) {
	case string:
		return value
	default:
		return ""
	}
}

func stringParamSummaryDefault(params map[string]any, key string, fallback string) string {
	if value := stringParamSummary(params, key); value != "" {
		return value
	}
	return fallback
}

func boolParamSummaryDefault(params map[string]any, key string, fallback bool) bool {
	if params == nil {
		return fallback
	}
	switch value := params[key].(type) {
	case bool:
		return value
	default:
		return fallback
	}
}

func validateBlocksAgainstRegistry(registry blocks.Registry, request Request) []reports.Issue {
	var issues []reports.Issue
	for index, instance := range request.Blocks {
		definition, ok := registry.GetBlock(instance.BlockID)
		if !ok {
			issues = append(issues, reports.Issue{
				Code:     reports.CodeMissingFile,
				Severity: reports.SeverityError,
				Path:     "blocks[" + strconv.Itoa(index) + "].block_id",
				Message:  "block not found: " + instance.BlockID,
			})
			continue
		}
		blockIssues := registry.ValidateRequest(blocks.BlockRequest{
			BlockID:    definition.ID,
			InstanceID: instance.ID,
			Params:     instance.Params,
		})
		for _, issue := range blockIssues {
			if issue.Path != "" {
				issue.Path = "blocks[" + strconv.Itoa(index) + "]." + issue.Path
			}
			issues = append(issues, issue)
		}
	}
	return issues
}
