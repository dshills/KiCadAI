package aiprovider

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestGenerateJSONUsesIdenticalRequestAndPreservesLegacyEnvelopeGate(t *testing.T) {
	const decision = `{"disposition":"unsupported","configuration":null}`
	var requests []string
	client := clientWithRoundTrip(func(r *http.Request) (*http.Response, error) {
		b, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		requests = append(requests, string(b))
		event := "event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":" + openAIResponseJSON(t, decision) + "}\n\n"
		rsp := jsonHTTPResponse(http.StatusOK, event)
		rsp.Header.Set("Content-Type", "text/event-stream")
		return rsp, nil
	})
	p, err := NewOpenAIProvider(OpenAIOptions{APIKey: "test-only-key", HTTPClient: client})
	if err != nil {
		t.Fatal(err)
	}
	req := openAITestRequest("test")
	req.OutputSchemaName = "custom_object"
	req.OutputSchema = map[string]any{"type": "object"}
	r, err := p.GenerateJSON(context.Background(), req)
	if err != nil || string(r.IntentJSON) != decision || r.ResponseID != "resp_test" || r.Usage.TotalTokens != 15 {
		t.Fatalf("structured result: %+v %v", r, err)
	}
	_, err = p.GenerateIntent(context.Background(), req)
	var pe *ProviderError
	if !errors.As(err, &pe) || pe.ResponseID != "resp_test" || pe.Usage.TotalTokens != 15 {
		t.Fatalf("legacy envelope must fail with accounting metadata: %v", err)
	}
	if len(requests) != 2 || requests[0] != requests[1] {
		t.Fatal("decoding choice changed outgoing payload")
	}
}

func TestStructuredJSONObjectRejectsMalformedPayloads(t *testing.T) {
	for _, s := range []string{"", `null`, `[]`, `{"x":1} {}`, `{"x":`, strings.Repeat(" ", 1) + `{"x":"` + strings.Repeat("x", MaxResponseBytes) + `"}`} {
		if _, err := decodeJSONObject([]byte(s)); err == nil {
			t.Fatal("accepted malformed structured output")
		}
	}
	if b, err := decodeJSONObject([]byte(` {"x":1} `)); err != nil || !json.Valid(b) {
		t.Fatalf("valid object: %s %v", b, err)
	}
}

func TestStructuredJSONRetainsProviderFailureGates(t *testing.T) {
	for _, body := range []string{`{"status":"incomplete","usage":{}}`, `{"status":"completed","output":[{"type":"message","content":[{"type":"refusal","refusal":"no"}]}],"usage":{}}`, `{"status":"completed","output":[],"usage":{}}`, `{"status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":0,"total_tokens":9}}`} {
		if _, err := decodeOpenAIResponseWithDecoder([]byte(body), "test", 1600, decodeJSONObject); err == nil {
			t.Fatal("accepted incomplete/refused/malformed response")
		}
	}
}
