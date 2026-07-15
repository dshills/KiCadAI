package aiprovider

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

const ReplaySchemaV1 = "kicadai.ai.replay.v1"

type ReplayArtifact struct {
	Schema       string                 `json:"schema"`
	Profile      string                 `json:"profile"`
	Provider     ReplayProviderEvidence `json:"provider"`
	Envelope     ReplayEnvelope         `json:"envelope"`
	EnvelopeHash string                 `json:"envelope_hash"`
}

type ReplayProviderEvidence struct {
	Name            string `json:"name"`
	Model           string `json:"model,omitempty"`
	ResponseID      string `json:"response_id,omitempty"`
	Usage           Usage  `json:"usage,omitempty"`
	FinishReason    string `json:"finish_reason,omitempty"`
	MaxOutputTokens int    `json:"max_output_tokens,omitempty"`
	Background      bool   `json:"background,omitempty"`
}

type ReplayEnvelope struct {
	Schema string          `json:"schema"`
	Intent json.RawMessage `json:"intent"`
}

type RecordedProvider struct {
	id      string
	intent  []byte
	profile string
}

func NewRecordedProvider(id string, envelopeData []byte) (*RecordedProvider, error) {
	artifact, replay, err := DecodeReplayArtifact(envelopeData)
	if err != nil {
		return nil, err
	}
	profile := ""
	data := envelopeData
	if replay {
		profile = artifact.Profile
		data, err = json.Marshal(artifact.Envelope)
		if err != nil {
			return nil, newProviderError(ErrorMalformed, "encode recorded replay envelope", err)
		}
	}
	intent, err := DecodeEnvelope(data)
	if err != nil {
		return nil, err
	}
	return &RecordedProvider{id: strings.TrimSpace(id), intent: intent, profile: profile}, nil
}

func NewReplayArtifact(profile string, result GenerateResult) (ReplayArtifact, error) {
	profile = strings.TrimSpace(profile)
	if profile == "" {
		return ReplayArtifact{}, newProviderError(ErrorConfiguration, "AI replay profile is required", nil)
	}
	intent, err := canonicalJSONObject(result.IntentJSON)
	if err != nil {
		return ReplayArtifact{}, err
	}
	envelope := ReplayEnvelope{Schema: EnvelopeSchemaV1, Intent: intent}
	hash, err := replayEnvelopeHash(envelope)
	if err != nil {
		return ReplayArtifact{}, err
	}
	return ReplayArtifact{
		Schema: ReplaySchemaV1, Profile: profile,
		Provider: ReplayProviderEvidence{
			Name: result.Provider, Model: result.Model, ResponseID: result.ResponseID,
			Usage: result.Usage, FinishReason: result.FinishReason,
			MaxOutputTokens: result.MaxOutputTokens, Background: result.Background,
		},
		Envelope: envelope, EnvelopeHash: hash,
	}, nil
}

func MarshalReplayArtifact(artifact ReplayArtifact) ([]byte, error) {
	data, err := json.MarshalIndent(artifact, "", "  ")
	if err != nil {
		return nil, newProviderError(ErrorMalformed, "encode AI replay artifact", err)
	}
	return append(data, '\n'), nil
}

func DecodeReplayArtifact(data []byte) (ReplayArtifact, bool, error) {
	if len(data) > MaxResponseBytes {
		return ReplayArtifact{}, false, newProviderError(ErrorMalformed, fmt.Sprintf("AI replay artifact exceeds %d-byte limit", MaxResponseBytes), nil)
	}
	var header struct {
		Schema string `json:"schema"`
	}
	if err := json.Unmarshal(data, &header); err != nil || header.Schema != ReplaySchemaV1 {
		return ReplayArtifact{}, false, nil
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var artifact ReplayArtifact
	if err := decoder.Decode(&artifact); err != nil {
		return ReplayArtifact{}, true, newProviderError(ErrorMalformed, "decode AI replay artifact", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return ReplayArtifact{}, true, newProviderError(ErrorMalformed, "AI replay artifact must contain exactly one JSON object", nil)
	}
	if strings.TrimSpace(artifact.Profile) == "" {
		return ReplayArtifact{}, true, newProviderError(ErrorSchema, "AI replay artifact profile is required", nil)
	}
	if artifact.Envelope.Schema != EnvelopeSchemaV1 {
		return ReplayArtifact{}, true, newProviderError(ErrorSchema, "unsupported AI replay envelope schema "+artifact.Envelope.Schema, nil)
	}
	intent, err := canonicalJSONObject(artifact.Envelope.Intent)
	if err != nil {
		return ReplayArtifact{}, true, err
	}
	artifact.Envelope.Intent = intent
	wantHash, err := replayEnvelopeHash(artifact.Envelope)
	if err != nil {
		return ReplayArtifact{}, true, err
	}
	if strings.TrimSpace(artifact.EnvelopeHash) != wantHash {
		return ReplayArtifact{}, true, newProviderError(ErrorSchema, "AI replay artifact envelope hash does not match", nil)
	}
	return artifact, true, nil
}

func canonicalJSONObject(data []byte) (json.RawMessage, error) {
	// Provider envelopes require an intent object; replay must not widen that contract.
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var value map[string]any
	if err := decoder.Decode(&value); err != nil {
		return nil, newProviderError(ErrorMalformed, "decode AI replay intent", err)
	}
	if value == nil {
		return nil, newProviderError(ErrorSchema, "AI replay intent requires an object", nil)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, newProviderError(ErrorMalformed, "AI replay intent must contain exactly one JSON object", nil)
	}
	canonical, err := json.Marshal(value)
	if err != nil {
		return nil, newProviderError(ErrorMalformed, "encode AI replay intent", err)
	}
	return canonical, nil
}

func replayEnvelopeHash(envelope ReplayEnvelope) (string, error) {
	data, err := json.Marshal(envelope)
	if err != nil {
		return "", newProviderError(ErrorMalformed, "encode AI replay envelope hash", err)
	}
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}

func (provider *RecordedProvider) Name() string {
	return "recorded"
}

func (provider *RecordedProvider) ReplayProfile() string {
	return provider.profile
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
