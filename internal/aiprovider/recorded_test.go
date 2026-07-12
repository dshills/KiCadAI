package aiprovider

import (
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

func TestRecordedProviderRejectsInvalidEnvelope(t *testing.T) {
	if _, err := NewRecordedProvider("bad", []byte(`{"schema":"other","intent":{}}`)); err == nil {
		t.Fatal("expected invalid envelope error")
	}
}
