package reports

type Severity string

const (
	SeverityInfo    Severity = "info"
	SeverityWarning Severity = "warning"
	SeverityError   Severity = "error"
	SeverityBlocked Severity = "blocked"
)

type Code string

const (
	CodeUnknown                 Code = "UNKNOWN"
	CodeInvalidArgument         Code = "INVALID_ARGUMENT"
	CodeMissingFile             Code = "MISSING_FILE"
	CodeUnsupportedOperation    Code = "UNSUPPORTED_OPERATION"
	CodeSkippedExternalTool     Code = "SKIPPED_EXTERNAL_TOOL"
	CodeValidationFailed        Code = "VALIDATION_FAILED"
	CodeMissingFootprint        Code = "MISSING_FOOTPRINT"
	CodeDuplicateReference      Code = "DUPLICATE_REFERENCE"
	CodeDuplicateUUID           Code = "DUPLICATE_UUID"
	CodeUnknownSymbolLibrary    Code = "UNKNOWN_SYMBOL_LIBRARY"
	CodeUnknownFootprintLibrary Code = "UNKNOWN_FOOTPRINT_LIBRARY"
	CodeMissingBoardOutline     Code = "MISSING_BOARD_OUTLINE"
	CodeDisconnectedPad         Code = "DISCONNECTED_PAD"
	CodeInvalidNetAssignment    Code = "INVALID_NET_ASSIGNMENT"
	CodeKiCadCLIFailed          Code = "KICAD_CLI_FAILED"
	CodeRoundTripDiff           Code = "ROUNDTRIP_DIFF"
	CodePreservationConflict    Code = "PRESERVATION_CONFLICT"
)

type Issue struct {
	Code       Code     `json:"code"`
	Severity   Severity `json:"severity"`
	Path       string   `json:"path,omitempty"`
	Message    string   `json:"message"`
	UUIDs      []string `json:"uuids,omitempty"`
	Refs       []string `json:"refs,omitempty"`
	Nets       []string `json:"nets,omitempty"`
	Suggestion string   `json:"suggestion,omitempty"`
}

func (issue Issue) Blocking() bool {
	return issue.Severity == SeverityError || issue.Severity == SeverityBlocked
}

func IssueFromError(err error) (Issue, bool) {
	if err == nil {
		return Issue{}, false
	}
	return Issue{
		Code:     CodeValidationFailed,
		Severity: SeverityError,
		Message:  err.Error(),
	}, true
}
