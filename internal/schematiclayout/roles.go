package schematiclayout

import "strings"

func normalizeRole(role string) string {
	role = strings.ToLower(strings.TrimSpace(role))
	role = strings.ReplaceAll(role, "-", "_")
	role = strings.ReplaceAll(role, " ", "_")
	return role
}

func containsRole(role string, tokens ...string) bool {
	return containsNormalizedRole(normalizeRole(role), tokens...)
}

func containsNormalizedRole(role string, tokens ...string) bool {
	for _, token := range tokens {
		normalizedToken := normalizeRole(token)
		if normalizedToken != "" && normalizedRoleHasToken(role, normalizedToken) {
			return true
		}
	}
	return false
}

func normalizedRoleHasToken(role, token string) bool {
	if role == token {
		return true
	}
	parts := strings.Split(role, "_")
	for _, part := range parts {
		if part == token {
			return true
		}
	}
	return strings.Contains("_"+role+"_", "_"+token+"_")
}
