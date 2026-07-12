package aiprovider

import (
	"encoding/json"
	"strings"
	"testing"
)

const validEnvelope = `{
  "schema": "kicadai.ai.intent.v1",
  "intent": {
    "version": "0.1.0",
    "name": "usb_c_bmp280_breakout",
    "kind": "breakout"
  }
}`

func TestDecodeEnvelopeAndIntent(t *testing.T) {
	intentJSON, err := DecodeEnvelope([]byte(validEnvelope))
	if err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	request, issues := DecodeIntent(intentJSON)
	if len(issues) != 0 {
		t.Fatalf("decode intent issues = %#v", issues)
	}
	if request.Name != "usb_c_bmp280_breakout" {
		t.Fatalf("request name = %q", request.Name)
	}
}

func TestDecodeEnvelopeRejectsInvalidShapes(t *testing.T) {
	tests := []struct {
		name string
		data string
	}{
		{name: "malformed", data: `{`},
		{name: "unknown field", data: `{"schema":"kicadai.ai.intent.v1","intent":{},"extra":true}`},
		{name: "wrong schema", data: `{"schema":"other","intent":{}}`},
		{name: "missing intent", data: `{"schema":"kicadai.ai.intent.v1"}`},
		{name: "null intent", data: `{"schema":"kicadai.ai.intent.v1","intent":null}`},
		{name: "array intent", data: `{"schema":"kicadai.ai.intent.v1","intent":[]}`},
		{name: "trailing", data: validEnvelope + `{}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := DecodeEnvelope([]byte(test.data)); err == nil {
				t.Fatal("expected decode error")
			}
		})
	}
	if _, err := DecodeEnvelope([]byte(strings.Repeat(" ", MaxResponseBytes+1))); err == nil {
		t.Fatal("expected response-size error")
	}
}

func TestDecodeIntentPrefixesIssuePaths(t *testing.T) {
	_, issues := DecodeIntent(json.RawMessage(`{"version":"0.1.0","name":"bad","unknown":true}`))
	if len(issues) != 1 || issues[0].Path != "provider.intent" {
		t.Fatalf("issues = %#v", issues)
	}
}
