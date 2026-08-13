package capabilityroundsv9

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
)

var tokenPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]{0,127}$`)
var digestPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

func AtomKey(category, scope, capability string) (string, error) {
	if !tokenPattern.MatchString(category) || !tokenPattern.MatchString(scope) || !tokenPattern.MatchString(capability) {
		return "", fmt.Errorf("%w: invalid atom identity", ErrInvalidInput)
	}
	return "atom:" + identityDigest(category, scope, capability), nil
}

func MemberKey(leaf Leaf) (string, error) {
	if !tokenPattern.MatchString(leaf.Stage) || !tokenPattern.MatchString(leaf.Category) || !tokenPattern.MatchString(leaf.Scope) || !tokenPattern.MatchString(leaf.Capability) || !tokenPattern.MatchString(leaf.Code) {
		return "", fmt.Errorf("%w: invalid member identity", ErrInvalidInput)
	}
	return "member:" + identityDigest(leaf.Stage, leaf.Category, leaf.Scope, leaf.Capability, leaf.Code), nil
}

func PathHash(gap Gap) (string, error) {
	if !digestPattern.MatchString(gap.ObligationAnchor) || len(gap.Path) == 0 {
		return "", fmt.Errorf("%w: invalid obligation path", ErrInvalidInput)
	}
	fields := []string{"path_v9", gap.ObligationAnchor, strconv.Itoa(len(gap.Path))}
	for _, leaf := range gap.Path {
		key, err := MemberKey(leaf)
		if err != nil {
			return "", err
		}
		if _, err := sortedUnique(leaf.RequiredEvidence); err != nil || len(leaf.RequiredEvidence) == 0 {
			return "", fmt.Errorf("%w: invalid path evidence", ErrInvalidInput)
		}
		fields = append(fields, "member", key, strconv.Itoa(len(leaf.RequiredEvidence)))
		fields = append(fields, leaf.RequiredEvidence...)
	}
	return "path:" + identityDigest(fields...), nil
}

// CaseHash commits the complete public case state while remaining independent
// of input slice order. It is used to prove non-exposed-case byte semantics did
// not drift across a round.
func CaseHash(value Case) (string, error) {
	if !tokenPattern.MatchString(value.ID) || !tokenPattern.MatchString(value.Role) || !tokenPattern.MatchString(value.ReportingDomain) ||
		!tokenPattern.MatchString(value.CircuitRole) || !tokenPattern.MatchString(value.SafetyImpact) || !tokenPattern.MatchString(value.Outcome) {
		return "", fmt.Errorf("%w: invalid case commitment identity", ErrInvalidInput)
	}
	pathHashes := make([]string, len(value.Frontier))
	for index, gap := range value.Frontier {
		hash, err := PathHash(gap)
		if err != nil {
			return "", err
		}
		if _, err := sortedUnique(gap.Diagnostics); err != nil || len(gap.Diagnostics) == 0 {
			return "", fmt.Errorf("%w: invalid case diagnostics", ErrInvalidInput)
		}
		fields := []string{"gap_state_v9", hash, strconv.Itoa(len(gap.Diagnostics))}
		fields = append(fields, gap.Diagnostics...)
		pathHashes[index] = identityDigest(fields...)
	}
	sort.Strings(pathHashes)
	if _, err := sortedUnique(pathHashes); err != nil && len(pathHashes) != 0 {
		return "", err
	}
	if _, err := sortedUnique(value.SatisfiedObligations); err != nil && len(value.SatisfiedObligations) != 0 {
		return "", err
	}
	fields := []string{"case_v9", value.ID, value.Role, value.ReportingDomain, value.CircuitRole, value.SafetyImpact, value.Outcome,
		strconv.Itoa(len(pathHashes))}
	fields = append(fields, pathHashes...)
	fields = append(fields, strconv.Itoa(len(value.SatisfiedObligations)))
	fields = append(fields, value.SatisfiedObligations...)
	return identityDigest(fields...), nil
}

func identityDigest(fields ...string) string {
	hash := sha256.New()
	var length [8]byte
	for _, field := range fields {
		binary.BigEndian.PutUint64(length[:], uint64(len(field)))
		_, _ = hash.Write(length[:])
		_, _ = hash.Write([]byte(field))
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func sortedUnique(values []string) ([]string, error) {
	result := append([]string(nil), values...)
	if !slices.IsSorted(result) {
		return nil, fmt.Errorf("%w: values are not sorted", ErrInvalidInput)
	}
	for index, value := range result {
		if value == "" || (index > 0 && result[index-1] == value) {
			return nil, fmt.Errorf("%w: values are empty or duplicated", ErrInvalidInput)
		}
	}
	return result, nil
}

func sortedSet(set map[string]bool) []string {
	result := make([]string, 0, len(set))
	for key := range set {
		result = append(result, key)
	}
	sort.Strings(result)
	return result
}

func subset(want []string, have map[string]bool) bool {
	for _, value := range want {
		if !have[value] {
			return false
		}
	}
	return true
}

func signature(atoms, members []string) string {
	return identityDigest(strings.Join(atoms, "\x00"), strings.Join(members, "\x00"))
}
