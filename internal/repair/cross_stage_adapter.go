package repair

import (
	"bytes"
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"kicadai/internal/repairloop"
	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

type CrossStageTransactionStage struct {
	Stage  repairloop.CrossStage `json:"stage"`
	Issues []reports.Issue       `json:"issues"`
}

type CrossStageTransactionEvidence struct {
	Stages  []CrossStageTransactionStage  `json:"stages"`
	Margins []repairloop.CrossStageMargin `json:"margins,omitempty"`
}

type CrossStageTransactionValidateFunc func(context.Context, repairloop.CrossStage, *transactions.Transaction) (CrossStageTransactionEvidence, error)

type CrossStageTransactionReenterFunc func(context.Context, repairloop.CrossStage, *transactions.Transaction) error

type CrossStageTransactionTargetOptions struct {
	Repair         Options
	Execution      ExecutionContext
	RequiredStages []repairloop.CrossStage
	Validate       CrossStageTransactionValidateFunc
	Reenter        CrossStageTransactionReenterFunc
}

type CrossStageTransactionTarget struct {
	opts              CrossStageTransactionTargetOptions
	executor          *Executor
	evidence          CrossStageTransactionEvidence
	lastReenter       repairloop.CrossStage
	diagnosticIssues  map[string]crossStageTransactionIssue
	proposalAttempts  map[string]Attempt
	evidenceAvailable bool
}

type crossStageTransactionIssue struct {
	Stage repairloop.CrossStage
	Issue reports.Issue
}

type crossStageTransactionState struct {
	Transaction transactions.Transaction `json:"transaction"`
	LastReenter repairloop.CrossStage    `json:"last_reenter,omitempty"`
}

func NewCrossStageTransactionTarget(opts CrossStageTransactionTargetOptions) (*CrossStageTransactionTarget, error) {
	if opts.Execution.Transaction == nil {
		return nil, errors.New("cross-stage transaction target requires a transaction")
	}
	if opts.Validate == nil {
		return nil, errors.New("cross-stage transaction target requires structured validation")
	}
	planner := NewPlanner(opts.Repair)
	opts.Repair = planner.Options
	opts.Repair.Enabled = true
	opts.Repair.Apply = true
	opts.RequiredStages = normalizeTransactionCrossStages(opts.RequiredStages)
	target := &CrossStageTransactionTarget{opts: opts}
	target.rebuildExecutor()
	return target, nil
}

func (target *CrossStageTransactionTarget) Capture(ctx context.Context) (repairloop.CrossStageCheckpoint, error) {
	if err := target.refreshEvidence(ctx); err != nil {
		return repairloop.CrossStageCheckpoint{}, err
	}
	state := crossStageTransactionState{Transaction: target.opts.Execution.Transaction.Clone(), LastReenter: target.lastReenter}
	payload, err := json.Marshal(state)
	if err != nil {
		return repairloop.CrossStageCheckpoint{}, err
	}
	return repairloop.NewCrossStageCheckpoint(
		payload,
		crossStageTransactionScopeHashes(state.Transaction),
		crossStageTransactionGates(target.opts.RequiredStages, target.evidence.Stages),
		target.evidence.Margins,
	), nil
}

func (target *CrossStageTransactionTarget) Restore(ctx context.Context, checkpoint repairloop.CrossStageCheckpoint) error {
	var state crossStageTransactionState
	decoder := json.NewDecoder(bytes.NewReader(checkpoint.Payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&state); err != nil {
		return err
	}
	*target.opts.Execution.Transaction = state.Transaction.Clone()
	target.lastReenter = state.LastReenter
	target.evidenceAvailable = false
	target.rebuildExecutor()
	return target.refreshEvidence(ctx)
}

func (target *CrossStageTransactionTarget) Diagnose(ctx context.Context) ([]repairloop.CrossStageDiagnostic, error) {
	if err := target.refreshEvidence(ctx); err != nil {
		return nil, err
	}
	target.diagnosticIssues = map[string]crossStageTransactionIssue{}
	diagnostics := []repairloop.CrossStageDiagnostic{}
	for _, stage := range target.evidence.Stages {
		if repairloop.CrossStageRank(stage.Stage) < 0 {
			return nil, fmt.Errorf("transaction validator returned unknown stage %q", stage.Stage)
		}
		for _, issue := range stage.Issues {
			severity := crossStageSeverityForIssue(issue)
			if severity == repairloop.CrossStageSeverityInfo {
				continue
			}
			classification := Classify(issue)
			diagnostic := repairloop.NewCrossStageDiagnostic(
				stage.Stage, string(issue.Code), string(classification.Category), severity,
				transactionIssueEvidenceHash(stage.Stage, issue), transactionIssueScopes(stage.Stage, issue),
			)
			target.diagnosticIssues[diagnostic.Hash] = crossStageTransactionIssue{Stage: stage.Stage, Issue: issue}
			diagnostics = append(diagnostics, diagnostic)
		}
	}
	return diagnostics, nil
}

func (target *CrossStageTransactionTarget) Propose(_ context.Context, diagnostic repairloop.CrossStageDiagnostic) ([]repairloop.CrossStageProposal, error) {
	entry, ok := target.diagnosticIssues[diagnostic.Hash]
	if !ok {
		return nil, errors.New("cross-stage transaction diagnostic is not current")
	}
	classification := Classify(entry.Issue)
	action, status, message := NewPlanner(target.opts.Repair).actionFor(classification)
	affected := transactionActionAffectedStages(action, entry.Stage)
	authorized := status == StatusPlanned
	if authorized {
		message = ""
	} else if strings.TrimSpace(message) == "" {
		message = firstNonEmpty(classification.Reason, "transaction repair is not authorized")
	}
	changeCount := transactionActionChangeCount(action, target.opts.Execution)
	scope := transactionActionScopes(action, diagnostic.Scope, target.opts.Execution)
	proposal := repairloop.NewCrossStageProposal(
		diagnostic, string(action), affected, []string{transactionActionExpectedEffect(action)}, scope,
		changeCount, float64(changeCount), 1, authorized, message,
	)
	target.proposalAttempts[proposal.ID] = Attempt{
		Stage: string(entry.Stage), Issue: entry.Issue, Category: classification.Category,
		Action: action, Status: status, DryRun: false, Message: message,
	}
	return []repairloop.CrossStageProposal{proposal}, nil
}

func (target *CrossStageTransactionTarget) Apply(_ context.Context, proposal repairloop.CrossStageProposal) error {
	attempt, ok := target.proposalAttempts[proposal.ID]
	if !ok {
		return errors.New("cross-stage transaction proposal is not current")
	}
	executed := target.executor.Execute(attempt)
	if executed.Status != StatusRepaired {
		return fmt.Errorf("transaction repair %s did not apply", proposal.Operator)
	}
	target.evidenceAvailable = false
	return nil
}

func (target *CrossStageTransactionTarget) Reenter(ctx context.Context, stage repairloop.CrossStage) error {
	if target.opts.Reenter != nil {
		if err := target.opts.Reenter(ctx, stage, target.opts.Execution.Transaction); err != nil {
			return err
		}
	}
	target.lastReenter = stage
	target.evidenceAvailable = false
	target.rebuildExecutor()
	return target.refreshEvidence(ctx)
}

func (target *CrossStageTransactionTarget) refreshEvidence(ctx context.Context) error {
	if target.evidenceAvailable {
		return nil
	}
	evidence, err := target.opts.Validate(ctx, target.lastReenter, target.opts.Execution.Transaction)
	if err != nil {
		return err
	}
	evidence.Stages = normalizeCrossStageTransactionStages(evidence.Stages)
	evidence.Margins = normalizeTransactionCrossStageMargins(evidence.Margins)
	target.evidence = evidence
	target.evidenceAvailable = true
	return nil
}

func (target *CrossStageTransactionTarget) rebuildExecutor() {
	target.executor = NewExecutor(target.opts.Execution)
	if target.proposalAttempts == nil {
		target.proposalAttempts = map[string]Attempt{}
	}
}

func normalizeCrossStageTransactionStages(source []CrossStageTransactionStage) []CrossStageTransactionStage {
	result := make([]CrossStageTransactionStage, 0, len(source))
	for _, stage := range source {
		stage.Issues = append([]reports.Issue(nil), stage.Issues...)
		slices.SortFunc(stage.Issues, compareTransactionCrossStageIssue)
		result = append(result, stage)
	}
	slices.SortFunc(result, func(left, right CrossStageTransactionStage) int {
		return cmp.Compare(repairloop.CrossStageRank(left.Stage), repairloop.CrossStageRank(right.Stage))
	})
	return result
}

func compareTransactionCrossStageIssue(left, right reports.Issue) int {
	return cmp.Or(
		cmp.Compare(left.Code, right.Code),
		cmp.Compare(left.Path, right.Path),
		cmp.Compare(strings.Join(transactionIssueScopes("", left), "\x00"), strings.Join(transactionIssueScopes("", right), "\x00")),
	)
}

func normalizeTransactionCrossStages(source []repairloop.CrossStage) []repairloop.CrossStage {
	result := slices.Clone(source)
	slices.SortFunc(result, func(left, right repairloop.CrossStage) int {
		return cmp.Compare(repairloop.CrossStageRank(left), repairloop.CrossStageRank(right))
	})
	return slices.Compact(result)
}

func normalizeTransactionCrossStageMargins(source []repairloop.CrossStageMargin) []repairloop.CrossStageMargin {
	result := slices.Clone(source)
	slices.SortFunc(result, func(left, right repairloop.CrossStageMargin) int { return cmp.Compare(left.ID, right.ID) })
	return result
}

func crossStageSeverityForIssue(issue reports.Issue) repairloop.CrossStageSeverity {
	switch issue.Severity {
	case reports.SeverityError, reports.SeverityBlocked:
		return repairloop.CrossStageSeverityBlocking
	case reports.SeverityWarning:
		return repairloop.CrossStageSeverityWarning
	default:
		return repairloop.CrossStageSeverityInfo
	}
}

func transactionIssueEvidenceHash(stage repairloop.CrossStage, issue reports.Issue) string {
	return transactionCrossStageHash(struct {
		Stage       repairloop.CrossStage
		Code        reports.Code
		Severity    reports.Severity
		IssueID     string
		RootCauseID string
		IssueStage  string
		RetryScope  string
		Path        string
		UUIDs       []string
		Refs        []string
		Nets        []string
		OperationID string
	}{
		Stage: stage, Code: issue.Code, Severity: issue.Severity,
		IssueID: issue.IssueID, RootCauseID: issue.RootCauseID, IssueStage: issue.Stage,
		RetryScope: issue.RetryScope, Path: issue.Path, UUIDs: transactionSortedStrings(issue.UUIDs),
		Refs: transactionSortedStrings(issue.Refs), Nets: transactionSortedStrings(issue.Nets), OperationID: issue.OperationID,
	})
}

func transactionIssueScopes(stage repairloop.CrossStage, issue reports.Issue) []string {
	scopes := []string{}
	for _, ref := range issue.Refs {
		if ref = strings.ToUpper(strings.TrimSpace(ref)); ref != "" {
			scopes = append(scopes, "ref:"+ref)
		}
	}
	for _, netName := range issue.Nets {
		if netName = strings.ToLower(strings.TrimSpace(netName)); netName != "" {
			scopes = append(scopes, "net:"+netName)
		}
	}
	if issue.OperationID != "" {
		scopes = append(scopes, "operation:"+strings.TrimSpace(issue.OperationID))
	}
	if len(scopes) == 0 {
		scopes = append(scopes, "stage:"+string(stage))
	}
	return transactionSortedStrings(scopes)
}

func crossStageTransactionScopeHashes(transaction transactions.Transaction) []repairloop.CrossStageScopeHash {
	buckets := map[string][]transactions.Operation{}
	for _, operation := range transaction.Operations {
		scopes := []string{}
		if ref := strings.ToUpper(strings.TrimSpace(operation.Ref)); ref != "" {
			scopes = append(scopes, "ref:"+ref)
		}
		if netName := strings.ToLower(strings.TrimSpace(operation.Net)); netName != "" {
			scopes = append(scopes, "net:"+netName)
		}
		if len(scopes) == 0 {
			switch operation.Op {
			case transactions.OpSetBoardOutline:
				scopes = []string{"board:outline"}
			case transactions.OpWriteProject:
				scopes = []string{"project:writer"}
			default:
				scopes = []string{"operation-kind:" + string(operation.Op)}
			}
		}
		for _, scope := range scopes {
			buckets[scope] = append(buckets[scope], operation.Clone())
		}
	}
	buckets["project:identity"] = nil
	result := make([]repairloop.CrossStageScopeHash, 0, len(buckets))
	for scope, operations := range buckets {
		value := any(operations)
		if scope == "project:identity" {
			value = struct {
				Name    string
				Project string
			}{transaction.Name, transaction.Project}
		}
		result = append(result, repairloop.CrossStageScopeHash{Scope: scope, Hash: transactionCrossStageHash(value)})
	}
	slices.SortFunc(result, func(left, right repairloop.CrossStageScopeHash) int { return cmp.Compare(left.Scope, right.Scope) })
	return result
}

func crossStageTransactionGates(required []repairloop.CrossStage, evidence []CrossStageTransactionStage) []repairloop.CrossStageGate {
	stageIssues := map[repairloop.CrossStage][]reports.Issue{}
	for _, stage := range evidence {
		stageIssues[stage.Stage] = append(stageIssues[stage.Stage], stage.Issues...)
	}
	if len(required) == 0 {
		for stage := range stageIssues {
			required = append(required, stage)
		}
		required = normalizeTransactionCrossStages(required)
	}
	gates := make([]repairloop.CrossStageGate, 0, len(required))
	for _, stage := range required {
		issues, available := stageIssues[stage]
		status := repairloop.CrossStageGateMissing
		if available {
			status = repairloop.CrossStageGatePassed
			for _, issue := range issues {
				if issue.Blocking() {
					status = repairloop.CrossStageGateBlocked
					break
				}
				if issue.Severity == reports.SeverityWarning {
					status = repairloop.CrossStageGateWarning
				}
			}
		}
		gates = append(gates, repairloop.CrossStageGate{
			ID: "transaction:" + string(stage), Stage: stage, Status: status, Required: true,
			EvidenceHash: transactionCrossStageHash(struct {
				Stage  repairloop.CrossStage
				Issues []reports.Issue
			}{stage, structuredTransactionIssues(issues)}),
		})
	}
	return gates
}

func structuredTransactionIssues(source []reports.Issue) []reports.Issue {
	result := append([]reports.Issue(nil), source...)
	for index := range result {
		result[index].Message = ""
		result[index].Suggestion = ""
	}
	slices.SortFunc(result, compareTransactionCrossStageIssue)
	return result
}

func transactionActionAffectedStages(action Action, diagnosticStage repairloop.CrossStage) []repairloop.CrossStage {
	var reenter repairloop.CrossStage
	switch action {
	case ActionAssignFootprint:
		reenter = repairloop.CrossStageSynthesis
	case ActionRegeneratePadNets:
		reenter = repairloop.CrossStageSchematic
	case ActionRetryPlacement, ActionGenerateOutline:
		reenter = repairloop.CrossStagePlacement
	case ActionRerouteNet:
		reenter = repairloop.CrossStageRouting
	case ActionRequireKiCadRefill, ActionRepairZoneNet:
		reenter = repairloop.CrossStageWriter
	default:
		reenter = diagnosticStage
	}
	return normalizeTransactionCrossStages([]repairloop.CrossStage{reenter, diagnosticStage})
}

func transactionActionExpectedEffect(action Action) string {
	switch action {
	case ActionAssignFootprint:
		return "restore verified footprint assignment"
	case ActionRegeneratePadNets:
		return "restore authoritative pad-to-net assignments"
	case ActionRetryPlacement:
		return "replace only affected generated placement operations"
	case ActionRerouteNet:
		return "replace only affected generated route operations"
	case ActionGenerateOutline:
		return "restore the generated board boundary"
	case ActionRequireKiCadRefill:
		return "refresh generated zone fill with the configured KiCad toolchain"
	case ActionRepairZoneNet:
		return "restore the authoritative generated zone net"
	default:
		return "no deterministic transaction effect is registered"
	}
}

func transactionActionChangeCount(action Action, execution ExecutionContext) int {
	switch action {
	case ActionRetryPlacement:
		return max(1, len(execution.PlacementOps))
	case ActionRerouteNet:
		return max(1, len(execution.RouteOps))
	default:
		return 1
	}
}

func transactionActionScopes(action Action, diagnosticScopes []string, execution ExecutionContext) []string {
	result := append([]string(nil), diagnosticScopes...)
	operations := []transactions.Operation{}
	switch action {
	case ActionGenerateOutline:
		result = append(result, "board:outline")
	case ActionRetryPlacement:
		operations = execution.PlacementOps
	case ActionRerouteNet:
		operations = execution.RouteOps
	}
	for _, operation := range operations {
		if ref := strings.ToUpper(strings.TrimSpace(operation.Ref)); ref != "" {
			result = append(result, "ref:"+ref)
		}
		if netName := strings.ToLower(strings.TrimSpace(operation.Net)); netName != "" {
			result = append(result, "net:"+netName)
		}
	}
	return transactionSortedStrings(result)
}

func transactionSortedStrings(source []string) []string {
	result := make([]string, 0, len(source))
	for _, value := range source {
		if value = strings.TrimSpace(value); value != "" {
			result = append(result, value)
		}
	}
	slices.Sort(result)
	return slices.Compact(result)
}

func transactionCrossStageHash(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		panic("transaction cross-stage hash invariant: " + err.Error())
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
