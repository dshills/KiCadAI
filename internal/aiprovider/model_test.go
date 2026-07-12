package aiprovider

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateGenerateRequest(t *testing.T) {
	valid := GenerateRequest{Prompt: "build a sensor", SchemaVersion: EnvelopeSchemaV1, Attempt: 1}
	if err := ValidateGenerateRequest(valid); err != nil {
		t.Fatalf("valid request: %v", err)
	}
	tests := []struct {
		name    string
		request GenerateRequest
	}{
		{name: "prompt", request: GenerateRequest{SchemaVersion: EnvelopeSchemaV1, Attempt: 1}},
		{name: "prompt size", request: GenerateRequest{Prompt: strings.Repeat("x", MaxPromptBytes+1), SchemaVersion: EnvelopeSchemaV1, Attempt: 1}},
		{name: "schema", request: GenerateRequest{Prompt: "x", SchemaVersion: "other", Attempt: 1}},
		{name: "attempt", request: GenerateRequest{Prompt: "x", SchemaVersion: EnvelopeSchemaV1, Attempt: 3}},
		{name: "diagnostic count", request: GenerateRequest{Prompt: "x", SchemaVersion: EnvelopeSchemaV1, Attempt: 2, Diagnostics: make([]Diagnostic, MaxDiagnostics+1)}},
		{name: "diagnostic code", request: GenerateRequest{Prompt: "x", SchemaVersion: EnvelopeSchemaV1, Attempt: 2, Diagnostics: []Diagnostic{{Message: "bad"}}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := ValidateGenerateRequest(test.request); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestProviderErrorPreservesCodeWithoutLeakingCause(t *testing.T) {
	err := newProviderError(ErrorAuthentication, "provider authentication failed", errors.New("Authorization: Bearer secret"))
	if strings.Contains(err.Error(), "secret") {
		t.Fatalf("error leaked cause: %v", err)
	}
	var providerErr *ProviderError
	if !errors.As(err, &providerErr) || providerErr.Code != ErrorAuthentication {
		t.Fatalf("provider error = %#v", err)
	}
}
