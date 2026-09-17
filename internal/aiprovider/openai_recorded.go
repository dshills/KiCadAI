package aiprovider

import (
	"encoding/json"
	"errors"
	"strings"
)

// RecordedOpenAITerminal extracts a terminal response from locally recorded
// bytes using the same stream validation as the provider. It performs no I/O.
// Callers must independently establish EOF, transport success, HTTP status,
// metadata integrity and usage. This is not proof of provider authenticity.
func RecordedOpenAITerminal(data []byte, contentType string, maxOutputTokens int) (json.RawMessage, error) {
	streaming := strings.Contains(strings.ToLower(contentType), "text/event-stream")
	if maxOutputTokens < MinOutputTokenLimit || maxOutputTokens > MaxOutputTokenLimit || len(data) > openAIResponseByteLimit(maxOutputTokens, streaming) {
		return nil, errors.New("recorded OpenAI response exceeds its contract bounds")
	}
	if streaming {
		return finalOpenAISSEPayload(data)
	}
	if !json.Valid(data) {
		return nil, errors.New("recorded OpenAI response is not valid JSON")
	}
	return append(json.RawMessage(nil), data...), nil
}
