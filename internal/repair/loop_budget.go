package repair

import "sync"

const (
	defaultLoopMaxCycles               = 2
	defaultLoopPerCategory             = 1
	defaultLoopRoutePerCategory        = 2
	defaultLoopConnectivityPerCategory = 2
)

type LoopBudgetOptions struct {
	MaxCycles              *int           `json:"max_cycles,omitempty"`
	MaxRepairs             *int           `json:"max_repairs,omitempty"`
	MaxPerCategory         map[string]int `json:"max_per_category,omitempty"`
	StopOnNoImprovement    *bool          `json:"stop_on_no_improvement,omitempty"`
	StopOnRepeatedEvidence *bool          `json:"stop_on_repeated_evidence,omitempty"`
}

type LoopBudgetSummary struct {
	MaxCycles              int                   `json:"max_cycles"`
	MaxRepairs             int                   `json:"max_repairs"`
	MaxPerCategory         map[string]int        `json:"max_per_category,omitempty"`
	CyclesUsed             int                   `json:"cycles_used"`
	RepairsUsed            int                   `json:"repairs_used"`
	CategoryAttempts       map[string]int        `json:"category_attempts,omitempty"`
	RemainingCycles        int                   `json:"remaining_cycles"`
	RemainingRepairs       int                   `json:"remaining_repairs"`
	Exhausted              bool                  `json:"exhausted"`
	ExhaustedCategory      string                `json:"exhausted_category,omitempty"`
	RepeatedEvidenceKey    string                `json:"repeated_evidence_key,omitempty"`
	StopReason             ConvergenceStopReason `json:"stop_reason,omitempty"`
	StopOnNoImprovement    bool                  `json:"stop_on_no_improvement"`
	StopOnRepeatedEvidence bool                  `json:"stop_on_repeated_evidence"`
}

type LoopBudgetLedger struct {
	mu                  sync.Mutex
	options             LoopBudgetOptions
	cyclesUsed          int
	repairsUsed         int
	categoryAttempts    map[string]int
	exhaustedCategory   string
	repeatedEvidenceKey string
	stopReason          ConvergenceStopReason
}

func NormalizeLoopBudgetOptions(opts LoopBudgetOptions) LoopBudgetOptions {
	if opts.MaxCycles == nil {
		opts.MaxCycles = intPointer(defaultLoopMaxCycles)
	}
	if opts.MaxRepairs == nil {
		opts.MaxRepairs = intPointer(intOptionValue(opts.MaxCycles))
	}
	opts.MaxPerCategory = normalizeLoopCategoryBudgets(opts.MaxPerCategory)
	if opts.StopOnNoImprovement == nil {
		opts.StopOnNoImprovement = boolPointer(true)
	}
	if opts.StopOnRepeatedEvidence == nil {
		opts.StopOnRepeatedEvidence = boolPointer(true)
	}
	return opts
}

func NewLoopBudgetLedger(opts LoopBudgetOptions) *LoopBudgetLedger {
	return &LoopBudgetLedger{
		options:          NormalizeLoopBudgetOptions(opts),
		categoryAttempts: map[string]int{},
	}
}

// RecordCycle reserves one validation/repair cycle. It returns a stop reason
// when starting another cycle would exceed the configured budget.
func (ledger *LoopBudgetLedger) RecordCycle(delta NormalizedDeltaSummary) ConvergenceStopReason {
	if ledger == nil {
		return StopReasonTotalBudgetExhausted
	}
	repeatedKey := ""
	if delta.RepeatedBlockingCount > 0 && !delta.Improved {
		repeatedKey = firstFindingKey(delta.Repeated)
	}
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	if ledger.stopReason != "" {
		return ledger.stopReason
	}
	if intOptionValue(ledger.options.MaxCycles) <= 0 {
		ledger.stopReason = StopReasonTotalBudgetExhausted
		return ledger.stopReason
	}
	if ledger.cyclesUsed >= intOptionValue(ledger.options.MaxCycles) {
		ledger.stopReason = StopReasonTotalBudgetExhausted
		return ledger.stopReason
	}
	ledger.cyclesUsed++
	if boolOptionValue(ledger.options.StopOnRepeatedEvidence) && repeatedKey != "" {
		ledger.repeatedEvidenceKey = repeatedKey
		ledger.stopReason = StopReasonRepeatedEvidence
		return ledger.stopReason
	}
	if boolOptionValue(ledger.options.StopOnNoImprovement) && !delta.Improved && delta.After.BlockingCount > 0 {
		ledger.stopReason = StopReasonNoImprovement
		return ledger.stopReason
	}
	return ""
}

// RecordRepair reserves one repair attempt for a category. It returns a stop
// reason when starting that repair would exceed total or per-category budget.
func (ledger *LoopBudgetLedger) RecordRepair(category FindingCategory) ConvergenceStopReason {
	if ledger == nil {
		return StopReasonTotalBudgetExhausted
	}
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	if ledger.stopReason != "" {
		return ledger.stopReason
	}
	if ledger.repairsUsed >= intOptionValue(ledger.options.MaxRepairs) {
		ledger.stopReason = StopReasonTotalBudgetExhausted
		return ledger.stopReason
	}
	categoryKey := normalizedCategoryKey(category)
	if ledger.categoryAttempts == nil {
		ledger.categoryAttempts = map[string]int{}
	}
	maxForCategory := ledger.maxForCategory(categoryKey)
	if ledger.categoryAttempts[categoryKey] >= maxForCategory {
		ledger.exhaustedCategory = categoryKey
		ledger.stopReason = StopReasonCategoryBudgetExhausted
		return ledger.stopReason
	}
	ledger.repairsUsed++
	ledger.categoryAttempts[categoryKey]++
	return ""
}

func (ledger *LoopBudgetLedger) Summary() LoopBudgetSummary {
	if ledger == nil {
		return LoopBudgetSummary{Exhausted: true, StopReason: StopReasonTotalBudgetExhausted}
	}
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	summary := LoopBudgetSummary{
		MaxCycles:              intOptionValue(ledger.options.MaxCycles),
		MaxRepairs:             intOptionValue(ledger.options.MaxRepairs),
		MaxPerCategory:         copyStringIntMap(ledger.options.MaxPerCategory),
		CyclesUsed:             ledger.cyclesUsed,
		RepairsUsed:            ledger.repairsUsed,
		CategoryAttempts:       copyStringIntMap(ledger.categoryAttempts),
		RemainingCycles:        maxInt(0, intOptionValue(ledger.options.MaxCycles)-ledger.cyclesUsed),
		RemainingRepairs:       maxInt(0, intOptionValue(ledger.options.MaxRepairs)-ledger.repairsUsed),
		ExhaustedCategory:      ledger.exhaustedCategory,
		RepeatedEvidenceKey:    ledger.repeatedEvidenceKey,
		StopReason:             ledger.stopReason,
		StopOnNoImprovement:    boolOptionValue(ledger.options.StopOnNoImprovement),
		StopOnRepeatedEvidence: boolOptionValue(ledger.options.StopOnRepeatedEvidence),
	}
	summary.Exhausted = summary.StopReason == StopReasonTotalBudgetExhausted ||
		summary.StopReason == StopReasonCategoryBudgetExhausted ||
		summary.StopReason == StopReasonNoImprovement ||
		summary.StopReason == StopReasonRepeatedEvidence
	return summary
}

func (ledger *LoopBudgetLedger) maxForCategory(category string) int {
	if ledger == nil {
		return defaultLoopPerCategory
	}
	if max, ok := ledger.options.MaxPerCategory[category]; ok {
		return max
	}
	return defaultLoopPerCategory
}

func normalizeLoopCategoryBudgets(input map[string]int) map[string]int {
	out := map[string]int{
		string(FindingCategoryRoute):        defaultLoopRoutePerCategory,
		string(FindingCategoryConnectivity): defaultLoopConnectivityPerCategory,
	}
	for key, value := range input {
		key = normalizedCategoryKey(FindingCategory(key))
		if key == "" || value < 0 {
			continue
		}
		out[key] = value
	}
	return out
}

func firstFindingKey(findings []NormalizedFinding) string {
	minKey := ""
	for _, finding := range findings {
		key := finding.Key
		if key != "" && (minKey == "" || key < minKey) {
			minKey = key
		}
	}
	return minKey
}

func copyStringIntMap(input map[string]int) map[string]int {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]int, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func maxInt(left int, right int) int {
	if left > right {
		return left
	}
	return right
}

func boolPointer(value bool) *bool {
	return &value
}

func boolOptionValue(value *bool) bool {
	return value != nil && *value
}

func intPointer(value int) *int {
	return &value
}

func intOptionValue(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}
