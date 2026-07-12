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
	CodeUnknown                   Code = "UNKNOWN"
	CodeInvalidArgument           Code = "INVALID_ARGUMENT"
	CodeMissingFile               Code = "MISSING_FILE"
	CodeUnsupportedOperation      Code = "UNSUPPORTED_OPERATION"
	CodeSkippedExternalTool       Code = "SKIPPED_EXTERNAL_TOOL"
	CodeOperationCanceled         Code = "OPERATION_CANCELED"
	CodeValidationFailed          Code = "VALIDATION_FAILED"
	CodeMissingFootprint          Code = "MISSING_FOOTPRINT"
	CodeDuplicateReference        Code = "DUPLICATE_REFERENCE"
	CodeDuplicateUUID             Code = "DUPLICATE_UUID"
	CodeUnknownSymbolLibrary      Code = "UNKNOWN_SYMBOL_LIBRARY"
	CodeUnknownFootprintLibrary   Code = "UNKNOWN_FOOTPRINT_LIBRARY"
	CodeMissingBoardOutline       Code = "MISSING_BOARD_OUTLINE"
	CodeDisconnectedPad           Code = "DISCONNECTED_PAD"
	CodeInvalidNetAssignment      Code = "INVALID_NET_ASSIGNMENT"
	CodeKiCadCLIFailed            Code = "KICAD_CLI_FAILED"
	CodeRoundTripDiff             Code = "ROUNDTRIP_DIFF"
	CodePreservationConflict      Code = "PRESERVATION_CONFLICT"
	CodeAmbiguousReference        Code = "AMBIGUOUS_REFERENCE"
	CodeUnsupportedImportedObject Code = "UNSUPPORTED_IMPORTED_OBJECT"
	CodeUnsafeRemove              Code = "UNSAFE_REMOVE"
	CodePinmapUnverified          Code = "PINMAP_UNVERIFIED"
	CodePlacementCollision        Code = "PLACEMENT_COLLISION"
	CodePlacementOutsideBoard     Code = "PLACEMENT_OUTSIDE_BOARD"
	CodeRouteContactMissingTarget Code = "ROUTE_CONTACT_MISSING_TARGET"
	CodeRouteContactNetMismatch   Code = "ROUTE_CONTACT_NET_MISMATCH"
	CodeRouteContactLayerMismatch Code = "ROUTE_CONTACT_LAYER_MISMATCH"
	CodeRouteContactMiss          Code = "ROUTE_CONTACT_MISS"
	CodeRouteContactAmbiguous     Code = "ROUTE_CONTACT_AMBIGUOUS"
	CodeRouteContactUnsupported   Code = "ROUTE_CONTACT_UNSUPPORTED_GEOMETRY"
	CodeRouteGraphIncomplete      Code = "ROUTE_GRAPH_INCOMPLETE"
	CodeRouteCompletionPartial    Code = "ROUTE_COMPLETION_PARTIAL"
	CodeFixedNetSkipped           Code = "FIXED_NET_SKIPPED"
	CodeMissingNetClass           Code = "MISSING_NET_CLASS"
	CodeAIProviderConfiguration   Code = "AI_PROVIDER_CONFIGURATION"
	CodeAIProviderTransport       Code = "AI_PROVIDER_TRANSPORT"
	CodeAIProviderAuthentication  Code = "AI_PROVIDER_AUTHENTICATION"
	CodeAIProviderRateLimit       Code = "AI_PROVIDER_RATE_LIMIT"
	CodeAIProviderTimeout         Code = "AI_PROVIDER_TIMEOUT"
	CodeAIProviderRefusal         Code = "AI_PROVIDER_REFUSAL"
	CodeAIProviderIncomplete      Code = "AI_PROVIDER_INCOMPLETE"
	CodeAIOutputInvalid           Code = "AI_OUTPUT_INVALID"
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
	// OperationID links this issue to a planned transaction operation when available.
	OperationID string `json:"operation_id,omitempty"`
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
