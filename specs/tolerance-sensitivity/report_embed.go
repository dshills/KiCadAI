package tolerancesensitivity

import _ "embed"

// CapabilityReport contains the exact published tolerance capability evidence.
//
//go:embed CAPABILITY_REPORT.json
var CapabilityReport []byte
