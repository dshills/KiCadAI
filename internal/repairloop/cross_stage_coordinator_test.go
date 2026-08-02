package repairloop

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

type crossStageTestState struct {
	Failures           []CrossStage       `json:"failures"`
	ScopeValues        map[string]string  `json:"scope_values"`
	Margins            map[string]float64 `json:"margins"`
	Variant            int                `json:"variant,omitempty"`
	OptionalGates      []CrossStage       `json:"optional_gates,omitempty"`
	UnprotectedMargins []string           `json:"unprotected_margins,omitempty"`
}

type crossStageTestTarget struct {
	state                      crossStageTestState
	cases                      map[CrossStage]crossStageRepairCorpusCase
	proposalStage              map[string]CrossStage
	reentries                  []CrossStage
	externalVariant            int
	regressScope               bool
	regressMargin              string
	introduceFailure           CrossStage
	nondeterministic           bool
	alternatives               bool
	addScope                   bool
	removeRequiredGate         CrossStage
	unprotectMargin            string
	applyCalls                 int
	failConfirmationCapture    bool
	failConfirmationDiagnose   bool
	confirmationCaptureFailed  bool
	confirmationDiagnoseFailed bool
	cancelOnApply              bool
	cancel                     context.CancelFunc
}

func newCrossStageTestTarget(cases ...crossStageRepairCorpusCase) *crossStageTestTarget {
	target := &crossStageTestTarget{
		cases:         map[CrossStage]crossStageRepairCorpusCase{},
		proposalStage: map[string]CrossStage{},
		state: crossStageTestState{
			ScopeValues: map[string]string{},
			Margins: map[string]float64{
				"electrical_corner": 1,
				"thermal":           1,
				"soa":               1,
				"physical":          1,
			},
		},
	}
	for _, frozen := range cases {
		stage := CrossStage(frozen.FailureStage)
		target.cases[stage] = frozen
		target.state.Failures = append(target.state.Failures, stage)
	}
	for _, stage := range crossStageOrder {
		target.state.ScopeValues[crossStageTestScope(stage)] = "original"
	}
	target.normalize()
	return target
}

func (target *crossStageTestTarget) Capture(context.Context) (CrossStageCheckpoint, error) {
	if target.failConfirmationCapture && target.applyCalls == 2 && !target.confirmationCaptureFailed {
		target.confirmationCaptureFailed = true
		return CrossStageCheckpoint{}, errors.New("injected confirmation capture failure")
	}
	target.normalize()
	payload, err := json.Marshal(target.state)
	if err != nil {
		return CrossStageCheckpoint{}, err
	}
	failures := map[CrossStage]struct{}{}
	for _, stage := range target.state.Failures {
		failures[stage] = struct{}{}
	}
	scopes := make([]CrossStageScopeHash, 0, len(target.state.ScopeValues))
	for scope, value := range target.state.ScopeValues {
		scopes = append(scopes, CrossStageScopeHash{Scope: scope, Hash: crossStageHash(value)})
	}
	gates := make([]CrossStageGate, 0, len(crossStageOrder))
	for _, stage := range crossStageOrder {
		status := CrossStageGatePassed
		if _, failed := failures[stage]; failed {
			status = CrossStageGateBlocked
		}
		gates = append(gates, CrossStageGate{
			ID: "gate:" + string(stage), Stage: stage, Status: status, Required: !slices.Contains(target.state.OptionalGates, stage),
			EvidenceHash: crossStageHash(struct {
				Stage  CrossStage
				Status CrossStageGateStatus
			}{stage, status}),
		})
	}
	margins := make([]CrossStageMargin, 0, len(target.state.Margins))
	for id, headroom := range target.state.Margins {
		margins = append(margins, CrossStageMargin{
			ID: id, Stage: CrossStageSimulation, Headroom: headroom, Protected: !slices.Contains(target.state.UnprotectedMargins, id),
			EvidenceHash: crossStageHash(struct {
				ID       string
				Headroom float64
			}{id, headroom}),
		})
	}
	return NewCrossStageCheckpoint(payload, scopes, gates, margins), nil
}

func (target *crossStageTestTarget) Restore(_ context.Context, checkpoint CrossStageCheckpoint) error {
	target.state = crossStageTestState{}
	return json.Unmarshal(checkpoint.Payload, &target.state)
}

func (target *crossStageTestTarget) Diagnose(context.Context) ([]CrossStageDiagnostic, error) {
	if target.failConfirmationDiagnose && target.applyCalls == 2 && !target.confirmationDiagnoseFailed {
		target.confirmationDiagnoseFailed = true
		return nil, errors.New("injected confirmation diagnosis failure")
	}
	diagnostics := make([]CrossStageDiagnostic, 0, len(target.state.Failures))
	for _, stage := range target.state.Failures {
		frozen, ok := target.cases[stage]
		if !ok {
			frozen = crossStageRepairCorpusCase{FailureStage: string(stage)}
			frozen.Fault.Code = "introduced_failure"
			frozen.Fault.Category = "regression"
		}
		diagnostics = append(diagnostics, NewCrossStageDiagnostic(
			stage, frozen.Fault.Code, frozen.Fault.Category, CrossStageSeverityBlocking,
			crossStageHash(struct {
				Stage CrossStage
				Code  string
			}{stage, frozen.Fault.Code}),
			[]string{crossStageTestScope(stage)},
		))
	}
	return diagnostics, nil
}

func (target *crossStageTestTarget) Propose(_ context.Context, diagnostic CrossStageDiagnostic) ([]CrossStageProposal, error) {
	frozen := target.cases[diagnostic.Stage]
	reenter := CrossStage(frozen.Expected.ReenterStage)
	proposal := NewCrossStageProposal(
		diagnostic, frozen.Expected.Operator, []CrossStage{reenter, diagnostic.Stage},
		[]string{"resolve " + diagnostic.Code}, []string{crossStageTestScope(diagnostic.Stage)}, 1, 0.1, 1, true, "",
	)
	target.proposalStage[proposal.ID] = diagnostic.Stage
	if target.alternatives {
		larger := NewCrossStageProposal(
			diagnostic, frozen.Expected.Operator+"_wide", []CrossStage{reenter, diagnostic.Stage},
			[]string{"resolve " + diagnostic.Code}, []string{crossStageTestScope(diagnostic.Stage)}, 2, 0.5, 100, true, "",
		)
		target.proposalStage[larger.ID] = diagnostic.Stage
		return []CrossStageProposal{larger, proposal}, nil
	}
	return []CrossStageProposal{proposal}, nil
}

func (target *crossStageTestTarget) Apply(ctx context.Context, proposal CrossStageProposal) error {
	target.applyCalls++
	if target.cancelOnApply && target.applyCalls == 1 {
		target.cancel()
		return ctx.Err()
	}
	stage := target.proposalStage[proposal.ID]
	remaining := target.state.Failures[:0]
	for _, failure := range target.state.Failures {
		if failure != stage {
			remaining = append(remaining, failure)
		}
	}
	target.state.Failures = remaining
	target.state.ScopeValues[crossStageTestScope(stage)] = "repaired"
	if target.regressScope {
		for _, candidate := range crossStageOrder {
			if candidate != stage {
				target.state.ScopeValues[crossStageTestScope(candidate)] = "unexpected-change"
				break
			}
		}
	}
	if target.addScope {
		target.state.ScopeValues["state:unexpected"] = "added"
	}
	if target.removeRequiredGate != "" {
		target.state.OptionalGates = append(target.state.OptionalGates, target.removeRequiredGate)
	}
	if target.unprotectMargin != "" {
		target.state.UnprotectedMargins = append(target.state.UnprotectedMargins, target.unprotectMargin)
	}
	if target.regressMargin != "" {
		target.state.Margins[target.regressMargin] = 0.5
	}
	if target.introduceFailure != "" && !slices.Contains(target.state.Failures, target.introduceFailure) {
		target.state.Failures = append(target.state.Failures, target.introduceFailure)
	}
	if target.nondeterministic {
		target.externalVariant++
		target.state.Variant = target.externalVariant
	}
	target.normalize()
	return nil
}

func (target *crossStageTestTarget) Reenter(_ context.Context, stage CrossStage) error {
	target.reentries = append(target.reentries, stage)
	return nil
}

func (target *crossStageTestTarget) normalize() {
	slices.SortFunc(target.state.Failures, func(left, right CrossStage) int { return CrossStageRank(left) - CrossStageRank(right) })
	target.state.Failures = slices.Compact(target.state.Failures)
	slices.Sort(target.state.OptionalGates)
	target.state.OptionalGates = slices.Compact(target.state.OptionalGates)
	slices.Sort(target.state.UnprotectedMargins)
	target.state.UnprotectedMargins = slices.Compact(target.state.UnprotectedMargins)
}

func crossStageTestScope(stage CrossStage) string {
	return "state:" + string(stage)
}

func TestCrossStageCoordinatorRecoversEveryFrozenStageDeterministically(t *testing.T) {
	for _, frozen := range loadCrossStageRepairCorpusCases(t) {
		frozen := frozen
		t.Run(frozen.FailureStage, func(t *testing.T) {
			firstTarget := newCrossStageTestTarget(frozen)
			first, err := RunCrossStageRepair(context.Background(), firstTarget, DefaultCrossStagePolicy())
			if err != nil {
				t.Fatal(err)
			}
			if err := ValidateCrossStageReport(first); err != nil {
				t.Fatalf("invalid first report: %v", err)
			}
			if first.Status != CrossStageStatusPassed || first.StopReason != CrossStageStopPassed || first.Consumption.CommittedRepairs != 1 || len(first.Trials) != 1 {
				t.Fatalf("stage %s report = %#v", frozen.FailureStage, first)
			}
			trial := first.Trials[0]
			if !trial.Accepted || !trial.Selected || !trial.Confirmed || !trial.Restored || trial.Proposal.ReenterStage != CrossStage(frozen.Expected.ReenterStage) {
				t.Fatalf("stage %s trial = %#v", frozen.FailureStage, trial)
			}

			secondTarget := newCrossStageTestTarget(frozen)
			second, err := RunCrossStageRepair(context.Background(), secondTarget, DefaultCrossStagePolicy())
			if err != nil {
				t.Fatal(err)
			}
			firstJSON, _ := json.Marshal(first)
			secondJSON, _ := json.Marshal(second)
			if !bytes.Equal(firstJSON, secondJSON) {
				t.Fatalf("stage %s replay differs", frozen.FailureStage)
			}
		})
	}
}

func TestCrossStageCoordinatorRepairsEarliestStageFirst(t *testing.T) {
	cases := loadCrossStageRepairCorpusCases(t)
	byStage := map[string]crossStageRepairCorpusCase{}
	for _, frozen := range cases {
		byStage[frozen.FailureStage] = frozen
	}
	target := newCrossStageTestTarget(byStage["drc"], byStage["simulation"], byStage["routing"])
	report, err := RunCrossStageRepair(context.Background(), target, DefaultCrossStagePolicy())
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateCrossStageReport(report); err != nil {
		t.Fatal(err)
	}
	if report.Status != CrossStageStatusPassed || report.Consumption.CommittedRepairs != 3 {
		t.Fatalf("mixed-stage report = %#v", report)
	}
	want := []CrossStage{CrossStageSizing, CrossStagePlacement, CrossStageRouting}
	got := []CrossStage{}
	for _, trial := range report.Trials {
		if trial.Confirmed {
			got = append(got, trial.Proposal.ReenterStage)
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("committed re-entry order = %#v, want %#v", got, want)
	}
}

func TestCrossStageCoordinatorSelectsSmallestEquivalentSafeRepair(t *testing.T) {
	target := newCrossStageTestTarget(loadCrossStageRepairCorpusCases(t)[0])
	target.alternatives = true
	report, err := RunCrossStageRepair(context.Background(), target, DefaultCrossStagePolicy())
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateCrossStageReport(report); err != nil {
		t.Fatal(err)
	}
	if report.Status != CrossStageStatusPassed || len(report.Trials) != 2 || report.Consumption.CommittedRepairs != 1 {
		t.Fatalf("alternative repair report = %#v", report)
	}
	for _, trial := range report.Trials {
		if trial.Selected && (trial.Proposal.ChangeCount != 1 || trial.Proposal.NormalizedChange != 0.1) {
			t.Fatalf("selected repair was not smallest: %#v", trial)
		}
	}
}

func TestCrossStageCoordinatorRollsBackUnsafeCandidates(t *testing.T) {
	frozen := loadCrossStageRepairCorpusCases(t)[0]
	tests := []struct {
		name   string
		modify func(*crossStageTestTarget)
		want   string
	}{
		{name: "unrelated scope", modify: func(target *crossStageTestTarget) { target.regressScope = true }, want: "unrelated_scope_changed:"},
		{name: "unrelated scope added", modify: func(target *crossStageTestTarget) { target.addScope = true }, want: "unrelated_scope_added:"},
		{name: "required gate made optional", modify: func(target *crossStageTestTarget) { target.removeRequiredGate = CrossStageWriter }, want: "required_gate_removed:"},
		{name: "protected margin made unprotected", modify: func(target *crossStageTestTarget) { target.unprotectMargin = "thermal" }, want: "protected_margin_unprotected:thermal"},
		{name: "electrical corner margin", modify: func(target *crossStageTestTarget) { target.regressMargin = "electrical_corner" }, want: "protected_margin_regressed:electrical_corner"},
		{name: "thermal margin", modify: func(target *crossStageTestTarget) { target.regressMargin = "thermal" }, want: "protected_margin_regressed:thermal"},
		{name: "soa margin", modify: func(target *crossStageTestTarget) { target.regressMargin = "soa" }, want: "protected_margin_regressed:soa"},
		{name: "physical margin", modify: func(target *crossStageTestTarget) { target.regressMargin = "physical" }, want: "protected_margin_regressed:physical"},
		{name: "new blocking gate", modify: func(target *crossStageTestTarget) { target.introduceFailure = CrossStageDRC }, want: "new_blocking_"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			target := newCrossStageTestTarget(frozen)
			test.modify(target)
			initial, _ := target.Capture(context.Background())
			report, err := RunCrossStageRepair(context.Background(), target, DefaultCrossStagePolicy())
			if err != nil {
				t.Fatal(err)
			}
			if err := ValidateCrossStageReport(report); err != nil {
				t.Fatal(err)
			}
			if report.Status != CrossStageStatusBlocked || report.StopReason != CrossStageStopNoSafeImprovement || report.Consumption.CommittedRepairs != 0 || report.Final.StateHash != initial.Snapshot.StateHash {
				t.Fatalf("unsafe report = %#v", report)
			}
			if len(report.Trials) != 1 || report.Trials[0].Accepted || !report.Trials[0].Restored || !strings.Contains(strings.Join(report.Trials[0].Rejections, ";"), test.want) {
				t.Fatalf("unsafe trial = %#v, want rejection containing %q", report.Trials, test.want)
			}
		})
	}
}

func TestCrossStageCoordinatorStopsDeterministicallyOnCancellation(t *testing.T) {
	target := newCrossStageTestTarget(loadCrossStageRepairCorpusCases(t)[0])
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	report, err := RunCrossStageRepair(ctx, target, DefaultCrossStagePolicy())
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateCrossStageReport(report); err != nil {
		t.Fatal(err)
	}
	if report.Status != CrossStageStatusBlocked || report.StopReason != CrossStageStopContextCanceled || report.Consumption.Trials != 0 {
		t.Fatalf("canceled report=%#v", report)
	}
}

func TestCrossStageCoordinatorRestoresCheckpointWhenCanceledDuringTrial(t *testing.T) {
	target := newCrossStageTestTarget(loadCrossStageRepairCorpusCases(t)[0])
	initial, _ := target.Capture(context.Background())
	ctx, cancel := context.WithCancel(context.Background())
	target.cancelOnApply = true
	target.cancel = cancel
	report, err := RunCrossStageRepair(ctx, target, DefaultCrossStagePolicy())
	if err != nil {
		t.Fatal(err)
	}
	if report.StopReason != CrossStageStopContextCanceled || report.Consumption.CommittedRepairs != 0 || report.Final.StateHash != initial.Snapshot.StateHash {
		t.Fatalf("mid-trial cancellation report = %#v", report)
	}
	if len(report.Trials) != 1 || !report.Trials[0].Restored {
		t.Fatalf("mid-trial cancellation did not restore checkpoint: %#v", report.Trials)
	}
}

func TestCrossStageCoordinatorRejectsNondeterministicConfirmation(t *testing.T) {
	target := newCrossStageTestTarget(loadCrossStageRepairCorpusCases(t)[0])
	target.nondeterministic = true
	initial, _ := target.Capture(context.Background())
	report, err := RunCrossStageRepair(context.Background(), target, DefaultCrossStagePolicy())
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateCrossStageReport(report); err != nil {
		t.Fatal(err)
	}
	if report.StopReason != CrossStageStopConfirmationMismatch || report.Consumption.CommittedRepairs != 0 || report.Final.StateHash != initial.Snapshot.StateHash {
		t.Fatalf("nondeterministic report = %#v", report)
	}
	if len(report.Trials) != 1 || !report.Trials[0].Selected || report.Trials[0].Confirmed {
		t.Fatalf("nondeterministic trial = %#v", report.Trials)
	}
}

func TestCrossStageCoordinatorRestoresCheckpointWhenConfirmationEvidenceFails(t *testing.T) {
	for _, test := range []struct {
		name   string
		modify func(*crossStageTestTarget)
	}{
		{name: "capture", modify: func(target *crossStageTestTarget) { target.failConfirmationCapture = true }},
		{name: "diagnose", modify: func(target *crossStageTestTarget) { target.failConfirmationDiagnose = true }},
	} {
		t.Run(test.name, func(t *testing.T) {
			target := newCrossStageTestTarget(loadCrossStageRepairCorpusCases(t)[0])
			test.modify(target)
			initial, _ := target.Capture(context.Background())
			report, err := RunCrossStageRepair(context.Background(), target, DefaultCrossStagePolicy())
			if err == nil {
				t.Fatal("confirmation evidence failure unexpectedly succeeded")
			}
			if report.StopReason != CrossStageStopInvalidTargetEvidence || report.Consumption.CommittedRepairs != 0 {
				t.Fatalf("confirmation failure report = %#v", report)
			}
			final, captureErr := target.Capture(context.Background())
			if captureErr != nil || final.Snapshot.Hash != initial.Snapshot.Hash {
				t.Fatalf("confirmation rollback final=%#v err=%v initial=%#v", final, captureErr, initial)
			}
		})
	}
}

func TestCrossStageCoordinatorEnforcesGlobalBudget(t *testing.T) {
	cases := loadCrossStageRepairCorpusCases(t)
	target := newCrossStageTestTarget(cases[0], cases[len(cases)-1])
	policy := DefaultCrossStagePolicy()
	policy.MaxTrials = 1
	report, err := RunCrossStageRepair(context.Background(), target, policy)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateCrossStageReport(report); err != nil {
		t.Fatal(err)
	}
	if report.StopReason != CrossStageStopBudgetExhausted || report.Consumption.Trials != 1 || report.Consumption.CommittedRepairs != 1 || len(report.FinalDiagnostics) != 1 {
		t.Fatalf("budget report = %#v", report)
	}
}

func loadCrossStageRepairCorpusCases(t *testing.T) []crossStageRepairCorpusCase {
	t.Helper()
	root := filepath.Join("testdata", "cross_stage_corpus")
	var manifest crossStageRepairCorpusManifest
	crossStageDecodeStrict(t, crossStageReadFile(t, filepath.Join(root, "manifest.json")), &manifest)
	result := make([]crossStageRepairCorpusCase, 0, len(manifest.Cases))
	for _, item := range manifest.Cases {
		var frozen crossStageRepairCorpusCase
		crossStageDecodeStrict(t, crossStageReadFile(t, filepath.Join(root, filepath.FromSlash(item.Path))), &frozen)
		result = append(result, frozen)
	}
	return result
}
