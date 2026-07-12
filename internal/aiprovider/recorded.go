package aiprovider

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

type RecordedProvider struct {
	id     string
	intent []byte
}

func NewRecordedProvider(id string, envelopeData []byte) (*RecordedProvider, error) {
	intent, err := DecodeEnvelope(envelopeData)
	if err != nil {
		return nil, err
	}
	return &RecordedProvider{id: strings.TrimSpace(id), intent: intent}, nil
}

func (provider *RecordedProvider) Name() string {
	return "recorded"
}

func (provider *RecordedProvider) GenerateIntent(ctx context.Context, request GenerateRequest) (GenerateResult, error) {
	if err := ValidateGenerateRequest(request); err != nil {
		return GenerateResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return GenerateResult{}, newProviderError(ErrorTransport, "recorded AI request canceled", err)
	}
	hash := sha256.Sum256(provider.intent)
	responseID := "recorded-" + hex.EncodeToString(hash[:8])
	if provider.id != "" {
		responseID = provider.id + "-" + hex.EncodeToString(hash[:8])
	}
	return GenerateResult{
		Provider:     provider.Name(),
		Model:        "recorded",
		ResponseID:   responseID,
		IntentJSON:   append([]byte(nil), provider.intent...),
		FinishReason: "completed",
		Recorded:     true,
	}, nil
}
