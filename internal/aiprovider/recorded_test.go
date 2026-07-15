package aiprovider

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"testing"
)

func TestRecordedProviderDeterministicReplay(t *testing.T) {
	provider, err := NewRecordedProvider("bmp280", []byte(validEnvelope))
	if err != nil {
		t.Fatalf("new recorded provider: %v", err)
	}
	request := GenerateRequest{Prompt: "create a BMP280 breakout", SchemaVersion: EnvelopeSchemaV1, Attempt: 1}
	first, err := provider.GenerateIntent(context.Background(), request)
	if err != nil {
		t.Fatalf("first generation: %v", err)
	}
	second, err := provider.GenerateIntent(context.Background(), request)
	if err != nil {
		t.Fatalf("second generation: %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("recorded results differ:\nfirst=%#v\nsecond=%#v", first, second)
	}
	if !first.Recorded || first.Provider != "recorded" || first.ResponseID == "" {
		t.Fatalf("recorded metadata = %#v", first)
	}
	var intent map[string]any
	if err := json.Unmarshal(first.IntentJSON, &intent); err != nil || intent["name"] != "usb_c_bmp280_breakout" {
		t.Fatalf("intent = %#v err=%v", intent, err)
	}
}

func TestReplayArtifactRoundTripIsDeterministicAndSanitized(t *testing.T) {
	result := GenerateResult{
		Provider: "openai", Model: "test-model", ResponseID: "resp_test",
		IntentJSON:   json.RawMessage(`{"z":2,"name":"captured","a":1}`),
		Usage:        Usage{InputTokens: 10, OutputTokens: 20, TotalTokens: 30},
		FinishReason: "completed", MaxOutputTokens: 32768, Background: true,
	}
	artifact, err := NewReplayArtifact("generic-circuit-v1", result)
	if err != nil {
		t.Fatal(err)
	}
	first, err := MarshalReplayArtifact(artifact)
	if err != nil {
		t.Fatal(err)
	}
	decoded, replay, err := DecodeReplayArtifact(first)
	if err != nil || !replay {
		t.Fatalf("decode replay=%t err=%v", replay, err)
	}
	second, err := MarshalReplayArtifact(decoded)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatalf("replay serialization changed\nfirst=%s\nsecond=%s", first, second)
	}
	for _, secret := range []string{"raw-user-prompt", "Bearer", "OPENAI_API_KEY"} {
		if bytes.Contains(first, []byte(secret)) {
			t.Fatalf("replay artifact leaked %q: %s", secret, first)
		}
	}
	provider, err := NewRecordedProvider("capture", first)
	if err != nil {
		t.Fatal(err)
	}
	if provider.ReplayProfile() != "generic-circuit-v1" {
		t.Fatalf("replay profile = %q", provider.ReplayProfile())
	}
	replayed, err := provider.GenerateIntent(context.Background(), GenerateRequest{
		Prompt: "offline replay", SchemaVersion: EnvelopeSchemaV1, Attempt: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(replayed.IntentJSON, artifact.Envelope.Intent) {
		t.Fatalf("replayed intent differs\nreplayed=%s\ncaptured=%s", replayed.IntentJSON, artifact.Envelope.Intent)
	}
}

func TestReplayArtifactRejectsTamperingAndUnknownFields(t *testing.T) {
	artifact, err := NewReplayArtifact("generic-circuit-v1", GenerateResult{Provider: "openai", IntentJSON: json.RawMessage(`{"name":"invalid downstream graph"}`)})
	if err != nil {
		t.Fatal(err)
	}
	data, err := MarshalReplayArtifact(artifact)
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatal(err)
	}
	value["envelope_hash"] = "tampered"
	tampered, _ := json.Marshal(value)
	if _, replay, err := DecodeReplayArtifact(tampered); !replay || ErrorCodeOf(err) != ErrorSchema {
		t.Fatalf("tampered replay=%t err=%v", replay, err)
	}
	value["envelope_hash"] = artifact.EnvelopeHash
	value["raw_prompt"] = "must not be accepted"
	unknown, _ := json.Marshal(value)
	if _, replay, err := DecodeReplayArtifact(unknown); !replay || ErrorCodeOf(err) != ErrorMalformed {
		t.Fatalf("unknown replay=%t err=%v", replay, err)
	}
}

func TestRecordedProviderRejectsInvalidEnvelope(t *testing.T) {
	if _, err := NewRecordedProvider("bad", []byte(`{"schema":"other","intent":{}}`)); err == nil {
		t.Fatal("expected invalid envelope error")
	}
}
