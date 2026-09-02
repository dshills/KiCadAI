package simmodel

import "strings"

// IsPrimitiveModel reports whether modelID identifies a compiled trusted
// component primitive rather than a circuit-level workflow model. Admission
// uses this distinction to prevent a functional compact model from silently
// competing with the one primitive selected for graph MNA.
func IsPrimitiveModel(modelID string) bool {
	_, exists := primitiveByID(strings.TrimSpace(modelID))
	return exists
}
