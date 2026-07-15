package aiprovider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const (
	EnvelopeSchemaV1   = "kicadai.ai.intent.v1"
	MaxPromptBytes     = 1 << 20
	MaxResponseBytes   = 2 << 20
	MaxDiagnostics     = 8
	MaxDiagnosticLen   = 512
	MaxCapabilityBytes = 64 << 10

	DefaultReferenceOutputTokens = 8192
	DefaultGenericOutputTokens   = 32768
	MinOutputTokenLimit          = 1024
	MaxOutputTokenLimit          = 65536
)

type Provider interface {
	Name() string
	GenerateIntent(context.Context, GenerateRequest) (GenerateResult, error)
}

type GenerateRequest struct {
	Prompt            string         `json:"-"`
	CapabilityContext string         `json:"-"`
	OutputSchemaName  string         `json:"-"`
	OutputSchema      map[string]any `json:"-"`
	SchemaVersion     string         `json:"schema_version"`
	Attempt           int            `json:"attempt"`
	Diagnostics       []Diagnostic   `json:"diagnostics,omitempty"`
	MaxOutputTokens   int            `json:"-"`
}

type Diagnostic struct {
	Code            string `json:"code"`
	Path            string `json:"path,omitempty"`
	Message         string `json:"message"`
	IssueID         string `json:"issue_id,omitempty"`
	RootCauseID     string `json:"root_cause_id,omitempty"`
	RetryScope      string `json:"retry_scope,omitempty"`
	SuggestedAction string `json:"suggested_action,omitempty"`
}

type GenerateResult struct {
	Provider        string          `json:"provider"`
	Model           string          `json:"model,omitempty"`
	ResponseID      string          `json:"response_id,omitempty"`
	IntentJSON      json.RawMessage `json:"intent"`
	Usage           Usage           `json:"usage,omitempty"`
	FinishReason    string          `json:"finish_reason,omitempty"`
	MaxOutputTokens int             `json:"max_output_tokens,omitempty"`
	Recorded        bool            `json:"recorded,omitempty"`
	Background      bool            `json:"background,omitempty"`
}

type Usage struct {
	InputTokens  int `json:"input_tokens,omitempty"`
	OutputTokens int `json:"output_tokens,omitempty"`
	TotalTokens  int `json:"total_tokens,omitempty"`
}

type ErrorCode string

const (
	ErrorConfiguration  ErrorCode = "ai_provider_configuration"
	ErrorTransport      ErrorCode = "ai_provider_transport"
	ErrorAuthentication ErrorCode = "ai_provider_authentication"
	ErrorRateLimit      ErrorCode = "ai_provider_rate_limit"
	ErrorTimeout        ErrorCode = "ai_provider_timeout"
	ErrorRefusal        ErrorCode = "ai_provider_refusal"
	ErrorIncomplete     ErrorCode = "ai_provider_incomplete"
	ErrorMalformed      ErrorCode = "ai_output_json_invalid"
	ErrorSchema         ErrorCode = "ai_output_schema_invalid"
)

type ProviderError struct {
	Code             ErrorCode
	Message          string
	Provider         string
	Model            string
	ResponseID       string
	IncompleteReason string
	MaxOutputTokens  int
	Usage            Usage
	RetryAllowed     bool
	Suggestion       string
	cause            error
}

func (err *ProviderError) Error() string {
	if err == nil {
		return ""
	}
	return strings.TrimSpace(err.Message)
}

func (err *ProviderError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.cause
}

func newProviderError(code ErrorCode, message string, cause error) error {
	return &ProviderError{Code: code, Message: message, cause: cause}
}

func ErrorCodeOf(err error) ErrorCode {
	var providerErr *ProviderError
	if errors.As(err, &providerErr) {
		return providerErr.Code
	}
	return ""
}

func ValidateGenerateRequest(request GenerateRequest) error {
	prompt := strings.TrimSpace(request.Prompt)
	if prompt == "" {
		return newProviderError(ErrorConfiguration, "AI prompt is required", nil)
	}
	if len(request.Prompt) > MaxPromptBytes {
		return newProviderError(ErrorConfiguration, fmt.Sprintf("AI prompt exceeds %d-byte limit", MaxPromptBytes), nil)
	}
	if len(request.CapabilityContext) > MaxCapabilityBytes {
		return newProviderError(ErrorConfiguration, fmt.Sprintf("AI capability context exceeds %d-byte limit", MaxCapabilityBytes), nil)
	}
	if request.SchemaVersion != EnvelopeSchemaV1 {
		return newProviderError(ErrorConfiguration, "unsupported AI intent schema "+request.SchemaVersion, nil)
	}
	if request.Attempt < 1 || request.Attempt > 2 {
		return newProviderError(ErrorConfiguration, "AI attempt must be 1 or 2", nil)
	}
	if request.MaxOutputTokens != 0 {
		if err := ValidateOutputTokenLimit(request.MaxOutputTokens); err != nil {
			return err
		}
	}
	if len(request.Diagnostics) > MaxDiagnostics {
		return newProviderError(ErrorConfiguration, fmt.Sprintf("AI correction accepts at most %d diagnostics", MaxDiagnostics), nil)
	}
	for index, diagnostic := range request.Diagnostics {
		if strings.TrimSpace(diagnostic.Code) == "" {
			return newProviderError(ErrorConfiguration, fmt.Sprintf("AI diagnostic %d code is required", index), nil)
		}
		if len(diagnostic.Code) > MaxDiagnosticLen || len(diagnostic.Path) > MaxDiagnosticLen || len(diagnostic.Message) > MaxDiagnosticLen {
			return newProviderError(ErrorConfiguration, fmt.Sprintf("AI diagnostic %d exceeds field size limit", index), nil)
		}
		if len(diagnostic.IssueID) > MaxDiagnosticLen || len(diagnostic.RootCauseID) > MaxDiagnosticLen || len(diagnostic.RetryScope) > MaxDiagnosticLen || len(diagnostic.SuggestedAction) > MaxDiagnosticLen {
			return newProviderError(ErrorConfiguration, fmt.Sprintf("AI diagnostic %d metadata exceeds field size limit", index), nil)
		}
	}
	return nil
}

func ValidateOutputTokenLimit(limit int) error {
	if limit < MinOutputTokenLimit || limit > MaxOutputTokenLimit {
		return newProviderError(
			ErrorConfiguration,
			fmt.Sprintf("AI max output tokens must be between %d and %d", MinOutputTokenLimit, MaxOutputTokenLimit),
			nil,
		)
	}
	return nil
}
