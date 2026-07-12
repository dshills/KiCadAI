package aiprovider

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"kicadai/internal/intentplanner"
	"kicadai/internal/reports"
)

type envelope struct {
	Schema string          `json:"schema"`
	Intent json.RawMessage `json:"intent"`
}

func DecodeEnvelope(data []byte) (json.RawMessage, error) {
	if len(data) > MaxResponseBytes {
		return nil, newProviderError(ErrorMalformed, fmt.Sprintf("AI response exceeds %d-byte limit", MaxResponseBytes), nil)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var value envelope
	if err := decoder.Decode(&value); err != nil {
		return nil, newProviderError(ErrorMalformed, "decode AI intent envelope", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, newProviderError(ErrorMalformed, "AI intent envelope must contain exactly one JSON object", nil)
	}
	if value.Schema != EnvelopeSchemaV1 {
		return nil, newProviderError(ErrorSchema, "unsupported AI intent envelope schema "+value.Schema, nil)
	}
	trimmed := bytes.TrimSpace(value.Intent)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) || trimmed[0] != '{' {
		return nil, newProviderError(ErrorSchema, "AI intent envelope requires an intent object", nil)
	}
	return append(json.RawMessage(nil), trimmed...), nil
}

func DecodeIntent(data json.RawMessage) (intentplanner.Request, []reports.Issue) {
	request, issues := intentplanner.DecodeRequestStrict(bytes.NewReader(data))
	for index := range issues {
		path := strings.TrimSpace(issues[index].Path)
		if path == "" || path == "request" {
			issues[index].Path = "provider.intent"
		} else {
			issues[index].Path = "provider.intent." + path
		}
	}
	return request, issues
}
