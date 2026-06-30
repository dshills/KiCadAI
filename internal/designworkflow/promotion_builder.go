package designworkflow

import (
	"fmt"
	"hash"
	"hash/fnv"
	"sort"
	"strings"

	"kicadai/internal/kicadfiles/checks"
	"kicadai/internal/reports"
)

var promotionCodeReplacer = strings.NewReplacer("/", "_", "\\", "_", ".", "_", "-", "_", " ", "_")

const (
	promotionKiCadERCSummaryKey = "erc"
	promotionKiCadDRCSummaryKey = "drc"
)

type PromotionFixture struct {
	ID                string
	Request           string
	Tier              string
	DeclaredReadiness PromotionReadiness
	Acceptance        AcceptanceLevel
	RequireERC        bool
	RequireDRC        bool
	ExpectedArtifacts []string
	ExpectedStages    []StageName
	KnownGaps         []string
}

func BuildInternalPromotionReport(fixture PromotionFixture, result WorkflowResult) PromotionReport {
	builder := promotionReportBuilder{
		fixture: fixture,
		result:  result,
		issues:  map[string]PromotionIssue{},
		byStage: map[StageName][]string{},
		stages:  indexPromotionStages(result.Stages),
	}
	builder.collectWorkflowIssues()
	builder.addMetadataGate()
	builder.addStageGate()
	builder.addSchematicElectricalGate()
	builder.addWriterGate()
	builder.addConnectivityGate()
	builder.addKiCadGate()
	builder.addRouteGate()
	builder.addPhysicalGate()
	builder.addArtifactGate()
	return builder.report()
}

type promotionReportBuilder struct {
	fixture  PromotionFixture
	result   WorkflowResult
	gates    []PromotionGate
	issues   map[string]PromotionIssue
	byStage  map[StageName][]string
	stages   map[StageName]StageResult
	blocking []string
}

func (builder *promotionReportBuilder) collectWorkflowIssues() {
	if builder.byStage == nil {
		builder.byStage = map[StageName][]string{}
	}
	for _, stage := range builder.result.Stages {
		for _, issue := range stage.Issues {
			code := promotionIssueCode(stage.Name, issue)
			if _, exists := builder.issues[code]; exists {
				code = promotionUniqueIssueCode(builder.issues, code)
			}
			builder.byStage[stage.Name] = append(builder.byStage[stage.Name], code)
			if issue.Blocking() {
				builder.blocking = append(builder.blocking, code)
			}
			builder.issues[code] = PromotionIssue{
				Code:     code,
				Severity: issue.Severity,
				Stage:    stage.Name,
				Message:  issue.Message,
				Path:     issue.Path,
				Repair:   promotionRepairForWorkflowIssue(stage.Name, issue),
				Refs:     append([]string(nil), issue.Refs...),
				Nets:     append([]string(nil), issue.Nets...),
			}
		}
	}
}

func (builder *promotionReportBuilder) addMetadataGate() {
	status := PromotionGateStatusPass
	var issueCodes []string
	if strings.TrimSpace(builder.fixture.ID) == "" {
		status = PromotionGateStatusFailed
		issueCodes = append(issueCodes, builder.addSyntheticIssue("metadata_missing_id", reports.SeverityError, "", "fixture id is required"))
	}
	if strings.TrimSpace(builder.fixture.Request) == "" {
		status = PromotionGateStatusFailed
		issueCodes = append(issueCodes, builder.addSyntheticIssue("metadata_missing_request", reports.SeverityError, "", "fixture request is required"))
	}
	builder.gates = append(builder.gates, PromotionGate{
		ID:          "metadata",
		Status:      status,
		RequiredFor: []PromotionReadiness{PromotionReadinessCandidate, PromotionReadinessPass},
		IssueCodes:  issueCodes,
	})
}

func (builder *promotionReportBuilder) addStageGate() {
	reached := builder.reachedStages()
	status := PromotionGateStatusPass
	var issueCodes []string
	for _, expected := range builder.fixture.ExpectedStages {
		if !reached[expected] {
			status = PromotionGateStatusFailed
			issueCodes = append(issueCodes, builder.addSyntheticIssue("stage_missing_"+sanitizePromotionCode(string(expected)), reports.SeverityError, expected, "expected stage "+string(expected)+" was not reached"))
		}
	}
	if builder.hasBlockedStage() {
		status = PromotionGateStatusFailed
		issueCodes = append(issueCodes, builder.blockingIssueCodes()...)
	}
	builder.gates = append(builder.gates, PromotionGate{
		ID:          "stages",
		Status:      status,
		RequiredFor: []PromotionReadiness{PromotionReadinessCandidate, PromotionReadinessPass},
		IssueCodes:  issueCodes,
	})
}

func (builder *promotionReportBuilder) addWriterGate() {
	builder.addStageStatusGate("writer_correctness", StageWriterCorrect, []PromotionReadiness{PromotionReadinessCandidate, PromotionReadinessPass})
}

func (builder *promotionReportBuilder) addConnectivityGate() {
	builder.addStageStatusGate("connectivity", StageValidation, []PromotionReadiness{PromotionReadinessCandidate, PromotionReadinessPass})
}

func (builder *promotionReportBuilder) addSchematicElectricalGate() {
	builder.addStageStatusGate("schematic_electrical", StageSchematicElectrical, []PromotionReadiness{PromotionReadinessCandidate, PromotionReadinessPass})
}

func (builder *promotionReportBuilder) addKiCadGate() {
	required := builder.requiredForKiCadChecks()
	stage, ok := builder.stages[StageKiCadChecks]
	if !ok {
		builder.gates = append(builder.gates, PromotionGate{
			ID:          "kicad_checks",
			Status:      PromotionGateStatusSkipped,
			RequiredFor: required,
			IssueCodes:  builder.missingKiCadIssueCodes(required),
		})
		return
	}
	status := promotionGateStatusForKiCadStage(stage)
	issueCodes := builder.issueCodesForStage(StageKiCadChecks)
	if status != PromotionGateStatusSkipped {
		missingCodes := builder.missingRequiredKiCadCheckIssueCodes(stage)
		if len(missingCodes) != 0 {
			status = PromotionGateStatusFailed
			issueCodes = append(issueCodes, missingCodes...)
		}
	}
	builder.gates = append(builder.gates, PromotionGate{
		ID:          "kicad_checks",
		Status:      status,
		RequiredFor: required,
		IssueCodes:  issueCodes,
		Artifacts:   promotionStageArtifactPaths(stage),
	})
}

func (builder *promotionReportBuilder) addRouteGate() {
	stage, ok := builder.stages[StageRouting]
	if !ok {
		builder.gates = append(builder.gates, PromotionGate{
			ID:          "route_completion",
			Status:      PromotionGateStatusNotRun,
			RequiredFor: builder.requiredForStage(StageRouting),
		})
		return
	}
	status := promotionGateStatusForStage(stage)
	if status == PromotionGateStatusPass {
		if summary, ok := promotionRouteConnectivitySummary(stage); ok {
			if summary.RoutesAttempted > summary.EndpointContactsProven {
				status = PromotionGateStatusWarn
			}
		}
	}
	builder.gates = append(builder.gates, PromotionGate{
		ID:          "route_completion",
		Status:      status,
		RequiredFor: builder.requiredForStage(StageRouting),
		IssueCodes:  builder.issueCodesForStage(StageRouting),
	})
}

func (builder *promotionReportBuilder) addPhysicalGate() {
	status := PromotionGateStatusNotRun
	if stage, ok := builder.stages[StageFabricationReady]; ok {
		status = promotionGateStatusForStage(stage)
	}
	builder.gates = append(builder.gates, PromotionGate{
		ID:          "physical_rules",
		Status:      status,
		RequiredFor: builder.requiredForStage(StageFabricationReady),
		IssueCodes:  builder.issueCodesForStage(StageFabricationReady),
	})
}

func (builder *promotionReportBuilder) addArtifactGate() {
	status := PromotionGateStatusPass
	var issueCodes []string
	produced := builder.producedArtifactPaths()
	for _, rawPath := range builder.fixture.ExpectedArtifacts {
		path := strings.TrimSpace(rawPath)
		if path == "" {
			status = PromotionGateStatusFailed
			issueCodes = append(issueCodes, builder.addSyntheticIssue("artifact_missing_path", reports.SeverityError, "", "expected artifact path is empty"))
			continue
		}
		if !produced[path] {
			status = PromotionGateStatusFailed
			issueCodes = append(issueCodes, builder.addSyntheticIssue("artifact_not_produced_"+sanitizePromotionCode(path), reports.SeverityError, "", "expected artifact "+path+" was not produced"))
		}
	}
	builder.gates = append(builder.gates, PromotionGate{
		ID:          "artifacts",
		Status:      status,
		RequiredFor: []PromotionReadiness{PromotionReadinessCandidate, PromotionReadinessPass},
		IssueCodes:  issueCodes,
		Artifacts:   normalizedExpectedArtifacts(builder.fixture.ExpectedArtifacts),
	})
}

func (builder *promotionReportBuilder) addStageStatusGate(id string, stageName StageName, required []PromotionReadiness) {
	if !builder.stageRelevant(stageName) {
		required = nil
	}
	stage, ok := builder.stages[stageName]
	if !ok {
		builder.gates = append(builder.gates, PromotionGate{ID: id, Status: PromotionGateStatusNotRun, RequiredFor: required})
		return
	}
	builder.gates = append(builder.gates, PromotionGate{
		ID:          id,
		Status:      promotionGateStatusForStage(stage),
		RequiredFor: required,
		IssueCodes:  builder.issueCodesForStage(stageName),
	})
}

func (builder *promotionReportBuilder) report() PromotionReport {
	artifacts := builder.promotionArtifacts()
	issues := make([]PromotionIssue, 0, len(builder.issues))
	for _, issue := range builder.issues {
		issues = append(issues, issue)
	}
	sort.Slice(issues, func(i, j int) bool {
		return issues[i].Code < issues[j].Code
	})
	achieved := builder.achievedReadiness()
	status := builder.statusForAchieved(achieved)
	return NormalizePromotionReport(PromotionReport{
		ID:                 builder.fixture.ID,
		Request:            builder.fixture.Request,
		Tier:               builder.fixture.Tier,
		DeclaredReadiness:  builder.fixture.DeclaredReadiness,
		AchievedReadiness:  achieved,
		Acceptance:         builder.fixture.Acceptance,
		Status:             status,
		MatchesExpectation: achieved == builder.fixture.DeclaredReadiness,
		Summary:            promotionSummary(status, achieved),
		Gates:              builder.gates,
		Stages: PromotionStageReport{
			Expected:  append([]StageName(nil), builder.fixture.ExpectedStages...),
			Reached:   builder.reachedStageList(),
			StoppedAt: builder.stoppedStage(),
		},
		Issues:      issues,
		NextActions: builder.nextActions(),
		Artifacts:   artifacts,
	})
}

func (builder *promotionReportBuilder) nextActions() []PromotionNextAction {
	var actions []PromotionNextAction
	for _, gate := range builder.gates {
		if !promotionGateNeedsAction(gate) {
			continue
		}
		actions = append(actions, PromotionNextAction{
			Gate:       gate.ID,
			Severity:   promotionActionSeverity(gate.Status),
			Summary:    promotionActionSummary(gate),
			Action:     promotionActionText(gate),
			IssueCodes: append([]string(nil), gate.IssueCodes...),
			Artifacts:  append([]string(nil), gate.Artifacts...),
		})
	}
	return actions
}

func (builder *promotionReportBuilder) achievedReadiness() PromotionReadiness {
	if builder.fixture.DeclaredReadiness == PromotionReadinessBlocked {
		return PromotionReadinessBlocked
	}
	passAchieved := builder.gatesAllowReadiness(PromotionReadinessPass)
	candidateAchieved := builder.gatesAllowReadiness(PromotionReadinessCandidate)
	if !passAchieved && !candidateAchieved {
		if builder.fixture.DeclaredReadiness == PromotionReadinessExpectedFail {
			return PromotionReadinessExpectedFail
		}
		return PromotionReadinessBlocked
	}
	if passAchieved {
		return PromotionReadinessPass
	}
	return PromotionReadinessCandidate
}

func (builder *promotionReportBuilder) gatesAllowReadiness(readiness PromotionReadiness) bool {
	for _, gate := range builder.gates {
		if !gateRequiresReadiness(gate, readiness) {
			continue
		}
		switch readiness {
		case PromotionReadinessPass:
			if gate.Status != PromotionGateStatusPass {
				return false
			}
		case PromotionReadinessCandidate:
			if gate.Status != PromotionGateStatusPass && gate.Status != PromotionGateStatusWarn {
				return false
			}
		}
	}
	return true
}

func (builder *promotionReportBuilder) statusForAchieved(achieved PromotionReadiness) PromotionStatus {
	if builder.fixture.DeclaredReadiness == PromotionReadinessExpectedFail && (achieved == PromotionReadinessCandidate || achieved == PromotionReadinessPass) {
		return PromotionStatusUnexpectedPass
	}
	switch achieved {
	case PromotionReadinessPass:
		return PromotionStatusPass
	case PromotionReadinessCandidate:
		for _, gate := range builder.gates {
			if gate.Status == PromotionGateStatusWarn {
				return PromotionStatusWarn
			}
		}
		return PromotionStatusPass
	case PromotionReadinessExpectedFail:
		return PromotionStatusExpectedFail
	case PromotionReadinessBlocked:
		if builder.fixture.DeclaredReadiness == PromotionReadinessBlocked {
			return PromotionStatusBlocked
		}
		return PromotionStatusFailed
	default:
		return PromotionStatusError
	}
}

func (builder *promotionReportBuilder) reachedStages() map[StageName]bool {
	reached := map[StageName]bool{}
	for _, stage := range builder.result.Stages {
		reached[stage.Name] = true
	}
	return reached
}

func (builder *promotionReportBuilder) reachedStageList() []StageName {
	reached := make([]StageName, 0, len(builder.result.Stages))
	for _, stage := range builder.result.Stages {
		reached = append(reached, stage.Name)
	}
	return reached
}

func (builder *promotionReportBuilder) stoppedStage() StageName {
	for i := len(builder.result.Stages) - 1; i >= 0; i-- {
		stage := builder.result.Stages[i]
		if stage.Status == StageStatusBlocked || reports.HasBlockingIssue(stage.Issues) {
			return stage.Name
		}
	}
	if len(builder.result.Stages) != 0 {
		return builder.result.Stages[len(builder.result.Stages)-1].Name
	}
	return ""
}

func (builder *promotionReportBuilder) hasBlockedStage() bool {
	for _, stage := range builder.result.Stages {
		if stage.Status == StageStatusBlocked || reports.HasBlockingIssue(stage.Issues) {
			return true
		}
	}
	return false
}

func (builder *promotionReportBuilder) blockingIssueCodes() []string {
	return append([]string(nil), builder.blocking...)
}

func (builder *promotionReportBuilder) requiredForStage(stageName StageName) []PromotionReadiness {
	if !builder.stageRelevant(stageName) {
		return nil
	}
	return []PromotionReadiness{PromotionReadinessCandidate, PromotionReadinessPass}
}

func (builder *promotionReportBuilder) requiredForKiCadChecks() []PromotionReadiness {
	if builder.fixture.RequireERC || builder.fixture.RequireDRC || builder.stageRelevant(StageKiCadChecks) {
		return []PromotionReadiness{PromotionReadinessCandidate, PromotionReadinessPass}
	}
	return nil
}

func (builder *promotionReportBuilder) stageRelevant(stageName StageName) bool {
	for _, expected := range builder.fixture.ExpectedStages {
		if expected == stageName {
			return true
		}
	}
	_, reached := builder.stages[stageName]
	return reached
}

func (builder *promotionReportBuilder) issueCodesForStage(stageName StageName) []string {
	return append([]string(nil), builder.byStage[stageName]...)
}

func (builder *promotionReportBuilder) addSyntheticIssue(code string, severity reports.Severity, stage StageName, message string) string {
	if _, exists := builder.issues[code]; exists {
		code = promotionUniqueIssueCode(builder.issues, code)
	}
	builder.issues[code] = PromotionIssue{
		Code:     code,
		Severity: severity,
		Stage:    stage,
		Message:  message,
		Repair:   promotionRepairForSyntheticIssue(code, stage),
	}
	return code
}

func (builder *promotionReportBuilder) missingKiCadIssueCodes(required []PromotionReadiness) []string {
	if len(required) == 0 {
		return nil
	}
	return []string{builder.addSyntheticIssue("kicad_checks_missing", reports.SeverityBlocked, StageKiCadChecks, "required KiCad ERC/DRC evidence was not produced")}
}

func (builder *promotionReportBuilder) missingRequiredKiCadCheckIssueCodes(stage StageResult) []string {
	var issueCodes []string
	if builder.fixture.RequireERC && !kiCadCheckSummaryPresent(stage, promotionKiCadERCSummaryKey) {
		issueCodes = append(issueCodes, builder.addSyntheticIssue("kicad_erc_missing", reports.SeverityBlocked, StageKiCadChecks, "required KiCad ERC evidence was not produced"))
	}
	if builder.fixture.RequireDRC && !kiCadCheckSummaryPresent(stage, promotionKiCadDRCSummaryKey) {
		issueCodes = append(issueCodes, builder.addSyntheticIssue("kicad_drc_missing", reports.SeverityBlocked, StageKiCadChecks, "required KiCad DRC evidence was not produced"))
	}
	return issueCodes
}

func (builder *promotionReportBuilder) promotionArtifacts() []PromotionArtifact {
	expected := map[string]bool{}
	for _, rawPath := range builder.fixture.ExpectedArtifacts {
		path := strings.TrimSpace(rawPath)
		if path != "" {
			expected[path] = true
		}
	}
	seen := map[string]struct{}{}
	var artifacts []PromotionArtifact
	for _, stage := range builder.result.Stages {
		for _, artifact := range stage.Artifacts {
			path := strings.TrimSpace(artifact.Path)
			if path == "" {
				continue
			}
			if _, exists := seen[path]; exists {
				continue
			}
			seen[path] = struct{}{}
			artifacts = append(artifacts, PromotionArtifact{
				Path:        path,
				Kind:        artifact.Kind,
				Description: artifact.Description,
				Required:    expected[path],
			})
		}
	}
	for _, rawPath := range builder.fixture.ExpectedArtifacts {
		path := strings.TrimSpace(rawPath)
		if path == "" {
			continue
		}
		if _, exists := seen[path]; exists {
			continue
		}
		artifacts = append(artifacts, PromotionArtifact{Path: path, Required: true})
	}
	return artifacts
}

func (builder *promotionReportBuilder) producedArtifactPaths() map[string]bool {
	produced := map[string]bool{}
	for _, stage := range builder.result.Stages {
		for _, artifact := range stage.Artifacts {
			if path := strings.TrimSpace(artifact.Path); path != "" {
				produced[path] = true
			}
		}
	}
	return produced
}

func promotionGateStatusForStage(stage StageResult) PromotionGateStatus {
	switch stage.Status {
	case StageStatusOK:
		return PromotionGateStatusPass
	case StageStatusWarning:
		return PromotionGateStatusWarn
	case StageStatusBlocked:
		return PromotionGateStatusFailed
	case StageStatusSkipped:
		return PromotionGateStatusSkipped
	default:
		if reports.HasBlockingIssue(stage.Issues) {
			return PromotionGateStatusFailed
		}
		if len(stage.Issues) != 0 {
			return PromotionGateStatusWarn
		}
		return PromotionGateStatusNotRun
	}
}

func promotionGateNeedsAction(gate PromotionGate) bool {
	switch gate.Status {
	case PromotionGateStatusPass:
		return false
	case PromotionGateStatusSkipped, PromotionGateStatusNotRun:
		return len(gate.RequiredFor) != 0
	default:
		return true
	}
}

func promotionActionSeverity(status PromotionGateStatus) reports.Severity {
	switch status {
	case PromotionGateStatusFailed:
		return reports.SeverityError
	case PromotionGateStatusSkipped, PromotionGateStatusNotRun:
		return reports.SeverityError
	default:
		return reports.SeverityWarning
	}
}

func promotionActionSummary(gate PromotionGate) string {
	switch gate.Status {
	case PromotionGateStatusFailed:
		return "promotion gate " + gate.ID + " failed"
	case PromotionGateStatusWarn:
		return "promotion gate " + gate.ID + " needs review"
	case PromotionGateStatusSkipped:
		return "promotion gate " + gate.ID + " was skipped"
	case PromotionGateStatusNotRun:
		return "promotion gate " + gate.ID + " did not run"
	default:
		return "promotion gate " + gate.ID + " needs follow-up"
	}
}

func promotionActionText(gate PromotionGate) string {
	switch gate.ID {
	case "metadata":
		return "fix fixture metadata before rerunning promotion"
	case "stages":
		return "run the expected design workflow stages and resolve the stage blockers"
	case "writer_correctness":
		return "fix writer correctness blockers and regenerate the project"
	case "connectivity":
		return "repair board connectivity, net assignment, or route completion blockers"
	case "kicad_checks":
		return "run required KiCad ERC/DRC checks and resolve or document every finding"
	case "route_completion":
		return "complete same-net route endpoint contacts or adjust placement/routing constraints"
	case "physical_rules":
		return "satisfy fabrication, physical-rule, and package evidence gates"
	case "artifacts":
		return "produce the missing required promotion artifacts"
	default:
		return "inspect the gate issues and rerun promotion after repair"
	}
}

func promotionGateStatusForKiCadStage(stage StageResult) PromotionGateStatus {
	if stage.Status == StageStatusSkipped || stageHasIssueCode(stage, reports.CodeSkippedExternalTool) {
		return PromotionGateStatusSkipped
	}
	if status, ok := promotionKiCadSummaryStatus(stage); ok && status != PromotionGateStatusPass {
		return status
	}
	return promotionGateStatusForStage(stage)
}

func promotionKiCadSummaryStatus(stage StageResult) (PromotionGateStatus, bool) {
	if stage.Summary == nil {
		return "", false
	}
	found := false
	status := PromotionGateStatusPass
	for _, key := range []string{promotionKiCadERCSummaryKey, promotionKiCadDRCSummaryKey} {
		result, ok := stage.Summary[key]
		if !ok {
			continue
		}
		found = true
		next, ok := promotionCheckResultStatus(result)
		if !ok {
			status = promotionWorseGateStatus(status, PromotionGateStatusFailed)
			continue
		}
		status = promotionWorseGateStatus(status, next)
	}
	return status, found
}

func kiCadCheckSummaryPresent(stage StageResult, key string) bool {
	if stage.Summary == nil {
		return false
	}
	value, ok := stage.Summary[key]
	if !ok {
		return false
	}
	status, valid := promotionCheckResultStatus(value)
	return valid && status != PromotionGateStatusNotRun
}

func promotionWorseGateStatus(current PromotionGateStatus, next PromotionGateStatus) PromotionGateStatus {
	if promotionGateSeverityRank(next) > promotionGateSeverityRank(current) {
		return next
	}
	return current
}

func promotionGateSeverityRank(status PromotionGateStatus) int {
	switch status {
	case PromotionGateStatusFailed:
		return 4
	case PromotionGateStatusSkipped:
		return 3
	case PromotionGateStatusNotRun:
		return 2
	case PromotionGateStatusWarn:
		return 1
	default:
		return 0
	}
}

func promotionCheckResultStatus(result any) (PromotionGateStatus, bool) {
	switch value := result.(type) {
	case nil:
		return PromotionGateStatusNotRun, true
	case checks.CheckResult:
		return checkResultStatusToPromotionStatus(value.Status), true
	case *checks.CheckResult:
		if value == nil {
			return PromotionGateStatusNotRun, true
		}
		return checkResultStatusToPromotionStatus(value.Status), true
	case checks.CheckStatus:
		return checkResultStatusToPromotionStatus(value), true
	case string:
		status, ok := parsePromotionCheckStatus(value)
		if !ok {
			return PromotionGateStatusFailed, true
		}
		return checkResultStatusToPromotionStatus(status), true
	default:
		return "", false
	}
}

func parsePromotionCheckStatus(value string) (checks.CheckStatus, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case string(checks.CheckStatusPass):
		return checks.CheckStatusPass, true
	case string(checks.CheckStatusFail):
		return checks.CheckStatusFail, true
	case string(checks.CheckStatusSkipped):
		return checks.CheckStatusSkipped, true
	case string(checks.CheckStatusError):
		return checks.CheckStatusError, true
	default:
		return "", false
	}
}

func stageHasIssueCode(stage StageResult, code reports.Code) bool {
	for _, issue := range stage.Issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}

func promotionStageArtifactPaths(stage StageResult) []string {
	var paths []string
	for _, artifact := range stage.Artifacts {
		switch artifact.Kind {
		case reports.ArtifactERCReport, reports.ArtifactDRCReport:
			if path := strings.TrimSpace(artifact.Path); path != "" {
				paths = append(paths, path)
			}
		}
	}
	sort.Strings(paths)
	return paths
}

func promotionRouteConnectivitySummary(stage StageResult) (LocalRouteConnectivitySummary, bool) {
	if stage.Summary == nil {
		return LocalRouteConnectivitySummary{}, false
	}
	value, exists := stage.Summary["route_connectivity"]
	if !exists {
		return LocalRouteConnectivitySummary{}, false
	}
	switch summary := value.(type) {
	case LocalRouteConnectivitySummary:
		return summary, true
	case *LocalRouteConnectivitySummary:
		if summary == nil {
			return LocalRouteConnectivitySummary{}, false
		}
		return *summary, true
	default:
		return LocalRouteConnectivitySummary{}, false
	}
}

func promotionIssueCode(stage StageName, issue reports.Issue) string {
	code := strings.TrimSpace(string(issue.Code))
	if code == "" {
		code = "issue"
	}
	hash := fnv.New64a()
	_, _ = hash.Write([]byte(string(stage)))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write([]byte(code))
	_, _ = hash.Write([]byte{0})
	writePromotionHashStrings(hash, issue.Refs)
	_, _ = hash.Write([]byte{0})
	writePromotionHashStrings(hash, issue.Nets)
	return fmt.Sprintf("%s_%s_%016x", sanitizePromotionCode(string(stage)), sanitizePromotionCode(code), hash.Sum64())
}

func writePromotionHashStrings(hash hash.Hash64, values []string) {
	for _, value := range values {
		_, _ = hash.Write([]byte{0xff})
		_, _ = hash.Write([]byte(value))
	}
}

func promotionUniqueIssueCode(existing map[string]PromotionIssue, base string) string {
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s_%d", base, i)
		if _, exists := existing[candidate]; !exists {
			return candidate
		}
	}
}

func promotionRepairForWorkflowIssue(stage StageName, issue reports.Issue) string {
	return firstNonEmpty(issue.Suggestion, defaultRepairAction(stage, issue))
}

func promotionRepairForSyntheticIssue(code string, stage StageName) string {
	switch {
	case code == "metadata_missing_id" || code == "metadata_missing_request":
		return "repair the KiCad-backed fixture metadata and rerun promotion"
	case strings.HasPrefix(code, "stage_missing_"):
		return "run the workflow path that produces the expected stage or update fixture expected_stages"
	case strings.HasPrefix(code, "artifact_missing_path"):
		return "declare a non-empty expected artifact path in fixture metadata"
	case strings.HasPrefix(code, "artifact_not_produced_"):
		return "produce the expected artifact or remove it from fixture metadata until supported"
	case code == "kicad_checks_missing", code == "kicad_erc_missing", code == "kicad_drc_missing":
		return "configure kicad-cli and preserve the required ERC/DRC report evidence"
	}
	return defaultRepairAction(stage, reports.Issue{Code: reports.CodeValidationFailed})
}

func indexPromotionStages(stages []StageResult) map[StageName]StageResult {
	indexed := make(map[StageName]StageResult, len(stages))
	for _, stage := range stages {
		indexed[stage.Name] = stage
	}
	return indexed
}

func promotionSummary(status PromotionStatus, readiness PromotionReadiness) string {
	return "promotion " + string(status) + " with achieved readiness " + string(readiness)
}

func checkResultStatusToPromotionStatus(status checks.CheckStatus) PromotionGateStatus {
	switch status {
	case checks.CheckStatusPass:
		return PromotionGateStatusPass
	case checks.CheckStatusFail:
		return PromotionGateStatusFailed
	case checks.CheckStatusSkipped:
		return PromotionGateStatusSkipped
	case checks.CheckStatusError:
		return PromotionGateStatusFailed
	default:
		return PromotionGateStatusNotRun
	}
}

func sanitizePromotionCode(value string) string {
	return strings.ToLower(promotionCodeReplacer.Replace(value))
}

func normalizedExpectedArtifacts(paths []string) []string {
	var normalized []string
	for _, path := range paths {
		if trimmed := strings.TrimSpace(path); trimmed != "" {
			normalized = append(normalized, trimmed)
		}
	}
	return normalized
}
