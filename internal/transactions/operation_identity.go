package transactions

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"strings"
	"unicode"

	"kicadai/internal/reports"
)

func annotatePlanIssueOperationIDs(plan *Plan) {
	if plan == nil || len(plan.Issues) == 0 || len(plan.Operations) == 0 {
		return
	}
	AnnotateIssueOperationIDs(plan.Issues, plan.Operations)
}

// AnnotateIssueOperationIDs mutates operation-scoped issues so paths such as
// operations[3].ref point at the matching planned operation ID.
func AnnotateIssueOperationIDs(issues []reports.Issue, operations []PlannedOperation) {
	if len(issues) == 0 || len(operations) == 0 {
		return
	}
	for i := range issues {
		if issues[i].OperationID != "" {
			continue
		}
		index, ok := operationIndexFromIssuePath(issues[i].Path)
		if !ok || index >= len(operations) {
			continue
		}
		if id := operations[index].ID; id != "" {
			issues[i].OperationID = id
		}
	}
}

func operationIndexFromIssuePath(path string) (index int, ok bool) {
	rest, found := strings.CutPrefix(path, "operations[")
	if !found {
		return 0, false
	}
	indexText, _, found := strings.Cut(rest, "]")
	if !found || indexText == "" {
		return 0, false
	}
	index, err := strconv.Atoi(indexText)
	if err != nil || index < 0 {
		return 0, false
	}
	return index, true
}

func plannedOperationID(planned PlannedOperation, op Operation) string {
	parts := []string{"op", sanitizeOperationIDPart(string(planned.Op))}
	if len(planned.Refs) > 0 {
		parts = append(parts, "ref", sanitizeOperationIDPart(planned.Refs[0]))
	} else if len(planned.Nets) > 0 {
		parts = append(parts, "net", sanitizeOperationIDPart(planned.Nets[0]))
	}
	parts = append(parts, operationContentHash(op))
	return strings.Join(nonEmptyIDParts(parts), "-")
}

func uniquePlannedOperationID(base string, seen map[string]struct{}, counts map[string]int) string {
	if _, exists := seen[base]; !exists {
		seen[base] = struct{}{}
		return base
	}
	for count := counts[base] + 1; ; count++ {
		candidate := base + "-n" + strconv.Itoa(count)
		if _, exists := seen[candidate]; exists {
			continue
		}
		seen[candidate] = struct{}{}
		counts[base] = count
		return candidate
	}
}

func operationContentHash(op Operation) string {
	data := canonicalOperationHashData(op)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])[:10]
}

func canonicalOperationHashData(op Operation) []byte {
	if len(op.Raw) > 0 {
		if canonical, ok := canonicalJSON(op.Raw); ok {
			return canonical
		}
		return append([]byte(nil), op.Raw...)
	}
	// Programmatic operations without Raw do not carry the full typed payload.
	// Hash the available metadata; constructors used by transaction flows
	// should populate Raw for content-stable IDs.
	data, err := json.Marshal(struct {
		Op  OperationKind `json:"op"`
		Ref string        `json:"ref,omitempty"`
	}{Op: op.Op, Ref: op.Ref})
	if err == nil {
		return data
	}
	return []byte(op.Op)
}

func canonicalJSON(data []byte) ([]byte, bool) {
	var value any
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return nil, false
	}
	canonical, err := json.Marshal(value)
	if err != nil {
		return nil, false
	}
	return canonical, true
}

func sanitizeOperationIDPart(value string) string {
	var out strings.Builder
	lastDash := false
	for _, r := range value {
		r = unicode.ToLower(r)
		keep := unicode.IsLetter(r) || unicode.IsDigit(r)
		if keep {
			out.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash && out.Len() > 0 {
			out.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(out.String(), "-")
}

func nonEmptyIDParts(parts []string) []string {
	filtered := make([]string, 0, len(parts))
	for _, part := range parts {
		if part != "" {
			filtered = append(filtered, part)
		}
	}
	return filtered
}
