package repair

import (
	"encoding/json"
	"regexp"
	"strings"

	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

type FootprintEvidence struct {
	Ref         string `json:"ref"`
	FootprintID string `json:"footprint_id"`
	Role        string `json:"role,omitempty"`
	Verified    bool   `json:"verified,omitempty"`
}

type PadNetHint struct {
	Ref string `json:"ref"`
	Pad string `json:"pad"`
	Net string `json:"net"`
}

type ExecutionContext struct {
	Transaction      *transactions.Transaction
	Footprints       map[string]FootprintEvidence
	PadNets          []PadNetHint
	AllowUnknownRefs bool
	DeferValidation  bool
	Revalidate       Revalidator
}

type Revalidator interface {
	Validate() []reports.Issue
}

// Executor mutates its transaction context sequentially. Callers that share one
// transaction across goroutines must serialize calls above this layer.
type Executor struct {
	Context     ExecutionContext
	footprints  map[string]FootprintEvidence
	assignIndex map[string][]int
	placeIndex  map[string][]int
	padNetIndex map[string]map[string]string
}

func NewExecutor(context ExecutionContext) *Executor {
	executor := &Executor{Context: context}
	executor.rebuildIndexes()
	return executor
}

func (executor *Executor) Execute(attempt Attempt) Attempt {
	if attempt.DryRun {
		attempt.Operations = executor.previewOperations(attempt)
		return attempt
	}
	switch attempt.Action {
	case ActionAssignFootprint:
		return executor.assignFootprint(attempt)
	case ActionRegeneratePadNets:
		return executor.regeneratePadNets(attempt)
	default:
		attempt.Status = StatusBlocked
		attempt.Message = "repair action is not executable by this executor"
		return attempt
	}
}

func (executor *Executor) previewOperations(attempt Attempt) []string {
	switch attempt.Action {
	case ActionAssignFootprint:
		if evidence, ok := executor.footprintEvidence(attempt.Issue); ok {
			return []string{"assign_footprint " + evidence.Ref + " " + evidence.FootprintID}
		}
	case ActionRegeneratePadNets:
		refs := executor.issueRefs(attempt.Issue)
		if len(refs) == 0 {
			return []string{"regenerate_pad_net_hints"}
		}
		operations := make([]string, 0, len(refs))
		for _, ref := range refs {
			operations = append(operations, "regenerate_pad_net_hints "+ref)
		}
		return operations
	}
	return nil
}

func (executor *Executor) assignFootprint(attempt Attempt) Attempt {
	if executor.Context.Transaction == nil {
		return blockedAttempt(attempt, "transaction is required for footprint assignment repair")
	}
	evidence, ok := executor.footprintEvidence(attempt.Issue)
	if !ok {
		return blockedAttempt(attempt, "no verified footprint evidence is available")
	}
	payload := transactions.AssignFootprintOperation{
		Op:          transactions.OpAssignFootprint,
		Ref:         evidence.Ref,
		Role:        evidence.Role,
		FootprintID: evidence.FootprintID,
	}
	operation, err := repairOperation(transactions.OpAssignFootprint, payload, evidence.Ref)
	if err != nil {
		return blockedAttempt(attempt, "encode assign_footprint repair: "+err.Error())
	}
	refKey := normalizeRef(evidence.Ref)
	updated := 0
	for _, index := range executor.assignIndex[refKey] {
		executor.Context.Transaction.Operations[index] = operation
		updated++
	}
	if updated > 0 {
		attempt.Status = StatusRepaired
		attempt.Message = "updated verified footprint assignment"
		attempt.Operations = []string{"assign_footprint " + evidence.Ref + " " + evidence.FootprintID}
		return executor.revalidate(attempt)
	}
	executor.Context.Transaction.Operations = append(executor.Context.Transaction.Operations, operation)
	executor.assignIndex[refKey] = append(executor.assignIndex[refKey], len(executor.Context.Transaction.Operations)-1)
	attempt.Status = StatusRepaired
	attempt.Message = "assigned verified footprint"
	attempt.Operations = []string{"assign_footprint " + evidence.Ref + " " + evidence.FootprintID}
	return executor.revalidate(attempt)
}

func (executor *Executor) regeneratePadNets(attempt Attempt) Attempt {
	if executor.Context.Transaction == nil {
		return blockedAttempt(attempt, "transaction is required for pad net repair")
	}
	hintsByRef := executor.padNetHintsByRef(attempt.Issue)
	if len(hintsByRef) == 0 {
		return blockedAttempt(attempt, "no generated pad net evidence is available")
	}
	changed := 0
	matched := 0
	decodeFailures := []string{}
	for _, index := range executor.placeIndexesForHints(hintsByRef) {
		operation := executor.Context.Transaction.Operations[index]
		var payload transactions.PlaceFootprintOperation
		if err := json.Unmarshal(operation.Raw, &payload); err != nil {
			decodeFailures = append(decodeFailures, err.Error())
			continue
		}
		refKey := normalizeRef(payload.Ref)
		hints, ok := hintsByRef[refKey]
		if !ok {
			continue
		}
		matched++
		if applyPadHints(&payload, hints) {
			updated, err := repairOperation(transactions.OpPlaceFootprint, payload, payload.Ref)
			if err != nil {
				return blockedAttempt(attempt, "encode place_footprint repair: "+err.Error())
			}
			executor.Context.Transaction.Operations[index] = updated
			changed++
		}
	}
	if changed == 0 {
		if matched > 0 {
			attempt.Status = StatusRepaired
			attempt.Message = "pad net hints already matched"
			attempt.Operations = []string{"regenerate_pad_net_hints"}
			return attempt
		}
		return blockedAttempt(attempt, "no generated place_footprint operation matched pad net evidence")
	}
	attempt.Status = StatusRepaired
	attempt.Message = "regenerated pad net hints"
	if len(decodeFailures) > 0 {
		attempt.Message += "; skipped malformed place_footprint operation"
	}
	attempt.Operations = []string{"regenerate_pad_net_hints"}
	return executor.revalidate(attempt)
}

func (executor *Executor) rebuildIndexes() {
	executor.assignIndex = map[string][]int{}
	executor.placeIndex = map[string][]int{}
	executor.padNetIndex = map[string]map[string]string{}
	executor.footprints = map[string]FootprintEvidence{}
	for ref, evidence := range executor.Context.Footprints {
		key := normalizeRef(ref)
		if key == "" {
			key = normalizeRef(evidence.Ref)
		}
		if key != "" {
			if existing, exists := executor.footprints[key]; exists && (existing.Verified || !evidence.Verified) {
				continue
			}
			executor.footprints[key] = evidence
		}
	}
	for _, hint := range executor.Context.PadNets {
		ref := normalizeRef(hint.Ref)
		pad := strings.TrimSpace(hint.Pad)
		net := strings.TrimSpace(hint.Net)
		if ref == "" || pad == "" || net == "" {
			continue
		}
		if executor.padNetIndex[ref] == nil {
			executor.padNetIndex[ref] = map[string]string{}
		}
		executor.padNetIndex[ref][pad] = net
	}
	if executor.Context.Transaction == nil {
		return
	}
	for index, operation := range executor.Context.Transaction.Operations {
		switch operation.Op {
		case transactions.OpAssignFootprint:
			if ref := operationRef(operation); ref != "" {
				executor.assignIndex[normalizeRef(ref)] = append(executor.assignIndex[normalizeRef(ref)], index)
			}
		case transactions.OpPlaceFootprint:
			if ref := operationRef(operation); ref != "" {
				executor.placeIndex[normalizeRef(ref)] = append(executor.placeIndex[normalizeRef(ref)], index)
			}
		}
	}
}

func (executor *Executor) placeIndexesForHints(hints map[string]map[string]string) []int {
	if len(hints) == 0 {
		return nil
	}
	indexes := []int{}
	for ref := range hints {
		indexes = append(indexes, executor.placeIndex[ref]...)
	}
	return indexes
}

func (executor *Executor) revalidate(attempt Attempt) Attempt {
	if executor.Context.Revalidate == nil || executor.Context.DeferValidation {
		return attempt
	}
	issues := executor.Context.Revalidate.Validate()
	attempt.AfterIssues = len(issues)
	attempt.Issues = append([]reports.Issue(nil), issues...)
	if len(issues) > 0 {
		attempt.Status = StatusPartial
	}
	return attempt
}

func (executor *Executor) footprintEvidence(issue reports.Issue) (FootprintEvidence, bool) {
	for _, ref := range executor.issueRefs(issue) {
		if evidence, ok := executor.footprints[normalizeRef(ref)]; ok && evidence.Verified && strings.TrimSpace(evidence.FootprintID) != "" {
			if strings.TrimSpace(evidence.Ref) == "" {
				evidence.Ref = ref
			}
			return evidence, true
		}
	}
	return FootprintEvidence{}, false
}

func (executor *Executor) padNetHintsByRef(issue reports.Issue) map[string]map[string]string {
	allowedRefs := map[string]struct{}{}
	for _, ref := range executor.issueRefs(issue) {
		allowedRefs[normalizeRef(ref)] = struct{}{}
	}
	out := map[string]map[string]string{}
	if len(allowedRefs) > 0 {
		for ref := range allowedRefs {
			if hints, ok := executor.padNetIndex[ref]; ok {
				out[ref] = copyPadHints(hints)
			}
		}
		return out
	}
	for ref, hints := range executor.padNetIndex {
		if !executor.Context.AllowUnknownRefs {
			continue
		}
		out[ref] = copyPadHints(hints)
	}
	return out
}

func (executor *Executor) issueRefs(issue reports.Issue) []string {
	if len(issue.Refs) > 0 {
		return append([]string(nil), issue.Refs...)
	}
	if issue.Path == "" {
		return nil
	}
	fields := strings.FieldsFunc(issue.Path, func(r rune) bool {
		return r == '[' || r == ']' || r == '"' || r == '\'' || r == '/'
	})
	refs := []string{}
	for _, field := range fields {
		if looksLikeReference(field) {
			refs = append(refs, field)
		}
	}
	return refs
}

func applyPadHints(payload *transactions.PlaceFootprintOperation, hints map[string]string) bool {
	changed := false
	for index := range payload.Pads {
		pad := strings.TrimSpace(payload.Pads[index].Name)
		net, ok := hints[pad]
		if !ok {
			continue
		}
		if payload.Pads[index].Net != nil && strings.TrimSpace(*payload.Pads[index].Net) == net {
			continue
		}
		netCopy := net
		payload.Pads[index].Net = &netCopy
		changed = true
	}
	return changed
}

func repairOperation(kind transactions.OperationKind, payload any, ref string) (transactions.Operation, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return transactions.Operation{}, err
	}
	return transactions.NewOperationWithRef(kind, data, ref), nil
}

func operationRef(operation transactions.Operation) string {
	return strings.TrimSpace(operation.Ref)
}

func copyPadHints(hints map[string]string) map[string]string {
	out := make(map[string]string, len(hints))
	for pad, net := range hints {
		out[pad] = net
	}
	return out
}

func blockedAttempt(attempt Attempt, message string) Attempt {
	attempt.Status = StatusBlocked
	attempt.Message = message
	return attempt
}

func normalizeRef(ref string) string {
	return strings.ToUpper(strings.TrimSpace(ref))
}

func looksLikeReference(value string) bool {
	return referencePattern.MatchString(value)
}

var referencePattern = regexp.MustCompile(`^[A-Za-z]+[\w.-]*[0-9]+[A-Za-z0-9._-]*$`)
