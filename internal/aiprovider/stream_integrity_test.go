package aiprovider

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func syntheticStreamEvent(t *testing.T, kind string, sequence int, extra map[string]any) string {
	t.Helper()
	event := map[string]any{"type": kind, "sequence_number": sequence}
	for name, value := range extra {
		event[name] = value
	}
	encoded, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	return "event: " + kind + "\ndata: " + string(encoded) + "\n\n"
}

func syntheticCompleteStream(t *testing.T) string {
	t.Helper()
	return syntheticStreamEvent(t, "response.created", 0, nil) +
		syntheticStreamEvent(t, "response.output_text.delta", 1, map[string]any{"delta": validEnvelope}) +
		syntheticStreamEvent(t, "response.completed", 2, map[string]any{"response": json.RawMessage(openAIResponseJSON(t, validEnvelope))})
}

func TestOpenAIStreamIntegrityFailsClosed(t *testing.T) {
	valid := syntheticCompleteStream(t)
	terminal := syntheticStreamEvent(t, "response.completed", 3, map[string]any{"response": json.RawMessage(openAIResponseJSON(t, validEnvelope))})
	for _, test := range []struct {
		name, body string
		code       ErrorCode
	}{
		{"duplicate terminal", valid + terminal, ErrorMalformed},
		{"post terminal data", valid + syntheticStreamEvent(t, "response.output_text.delta", 3, map[string]any{"delta": "extra"}), ErrorMalformed},
		{"gap", strings.Replace(valid, `"sequence_number":1`, `"sequence_number":4`, 1), ErrorMalformed},
		{"duplicate sequence", strings.Replace(valid, `"sequence_number":1`, `"sequence_number":0`, 1), ErrorMalformed},
		{"missing sequence", strings.Replace(valid, `"sequence_number":1,`, "", 1), ErrorMalformed},
		{"event type mismatch", strings.Replace(valid, "event: response.completed", "event: response.failed", 1), ErrorMalformed},
		{"status mismatch", syntheticStreamEvent(t, "response.failed", 0, map[string]any{"response": json.RawMessage(openAIResponseJSON(t, validEnvelope))}), ErrorMalformed},
		{"different final text", syntheticStreamEvent(t, "response.output_text.delta", 0, map[string]any{"delta": "different"}) + syntheticStreamEvent(t, "response.completed", 1, map[string]any{"response": json.RawMessage(openAIResponseJSON(t, validEnvelope))}), ErrorMalformed},
		{"malformed intermediate JSON", "event: response.created\ndata: {broken\n\n" + terminal, ErrorMalformed},
		{"missing delta", syntheticStreamEvent(t, "response.output_text.delta", 0, nil) + terminal, ErrorMalformed},
		{"missing terminal", syntheticStreamEvent(t, "response.created", 0, nil), ErrorIncomplete},
		{"stream error", syntheticStreamEvent(t, "error", 0, map[string]any{"message": "synthetic_sensitive_detail"}) + terminal, ErrorTransport},
	} {
		t.Run(test.name, func(t *testing.T) {
			requests := 0
			client := clientWithRoundTrip(func(_ *http.Request) (*http.Response, error) {
				requests++
				r := jsonHTTPResponse(http.StatusOK, test.body)
				r.Header.Set("Content-Type", "text/event-stream")
				return r, nil
			})
			provider, err := newOpenAIProvider(OpenAIOptions{APIKey: "synthetic-test-key", HTTPClient: client}, openAIResponsesEndpoint)
			if err != nil {
				t.Fatal(err)
			}
			result, err := provider.GenerateIntent(context.Background(), openAITestRequest("offline synthetic stream"))
			if ErrorCodeOf(err) != test.code || len(result.IntentJSON) != 0 || result.Usage.TotalTokens != 0 || requests != 1 {
				t.Fatalf("code=%s want=%s result=%#v requests=%d", ErrorCodeOf(err), test.code, result, requests)
			}
			if strings.Contains(err.Error(), "synthetic_sensitive_detail") {
				t.Fatal("raw stream error leaked")
			}
		})
	}
}

func TestOpenAIStreamFramesAndUsageArePreserved(t *testing.T) {
	valid := syntheticCompleteStream(t)
	var terminal map[string]any
	if err := json.Unmarshal([]byte(openAIResponseJSON(t, validEnvelope)), &terminal); err != nil {
		t.Fatal(err)
	}
	wrapped, err := json.MarshalIndent(map[string]any{"type": "response.completed", "sequence_number": 0, "response": terminal}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	multiline := "event: response.completed\ndata: " + strings.ReplaceAll(string(wrapped), "\n", "\ndata: ") + "\n\n"
	for name, body := range map[string]string{"numbered": valid, "comments and done": valid + ": keepalive\n\ndata: [DONE]\n\n", "crlf": strings.ReplaceAll(valid, "\n", "\r\n"), "multiline": multiline, "no final separator": strings.TrimSuffix(valid, "\n\n")} {
		t.Run(name, func(t *testing.T) {
			payload, err := finalOpenAISSEPayload([]byte(body))
			if err != nil {
				t.Fatal(err)
			}
			result, err := decodeOpenAIResponse(payload, "unused", DefaultReferenceOutputTokens)
			if err != nil || result.ResponseID != "resp_test" || result.Usage.InputTokens != 10 || result.Usage.OutputTokens != 5 || result.Usage.TotalTokens != 15 || len(result.IntentJSON) == 0 {
				t.Fatalf("result=%#v err=%v", result, err)
			}
		})
	}
}

func TestOpenAIOpaqueMetadataDoesNotBypassUnchangedLimit(t *testing.T) {
	if limit := openAIResponseByteLimit(16384, true); limit != 2097152 {
		t.Fatalf("frozen 16384-token stream limit changed: %d", limit)
	}
	for _, size := range []int{10000, 2097152} {
		var response map[string]any
		if err := json.Unmarshal([]byte(openAIResponseJSON(t, validEnvelope)), &response); err != nil {
			t.Fatal(err)
		}
		response["output"] = append(response["output"].([]any), map[string]any{"type": "reasoning", "encrypted_content": strings.Repeat("x", size)})
		body := syntheticStreamEvent(t, "response.completed", 0, map[string]any{"response": response})
		requests := 0
		client := clientWithRoundTrip(func(_ *http.Request) (*http.Response, error) {
			requests++
			r := jsonHTTPResponse(http.StatusOK, body)
			r.Header.Set("Content-Type", "text/event-stream")
			return r, nil
		})
		provider, err := newOpenAIProvider(OpenAIOptions{APIKey: "synthetic-test-key", HTTPClient: client}, openAIResponsesEndpoint)
		if err != nil {
			t.Fatal(err)
		}
		request := openAITestRequest("offline opaque metadata")
		request.MaxOutputTokens = 16384
		result, err := provider.GenerateIntent(context.Background(), request)
		if size == 10000 {
			if err != nil || result.Usage.TotalTokens != 15 || len(result.IntentJSON) == 0 {
				t.Fatalf("valid metadata rejected: %v", err)
			}
		} else if ErrorCodeOf(err) != ErrorMalformed || len(result.IntentJSON) != 0 || result.Usage.TotalTokens != 0 {
			t.Fatal("oversized metadata bypassed cap or leaked accepted output/usage")
		}
		if requests != 1 {
			t.Fatalf("unexpected retry count %d", requests)
		}
	}
}

func TestOpenAIRejectsInconsistentUsage(t *testing.T) {
	for _, usage := range []map[string]int{
		{"input_tokens": -1, "output_tokens": 5, "total_tokens": 4},
		{"input_tokens": 10, "output_tokens": -5, "total_tokens": 5},
		{"input_tokens": 10, "output_tokens": 5, "total_tokens": 99},
		{"input_tokens": 10, "output_tokens": 16385, "total_tokens": 16395},
	} {
		var response map[string]any
		if err := json.Unmarshal([]byte(openAIResponseJSON(t, validEnvelope)), &response); err != nil {
			t.Fatal(err)
		}
		response["usage"] = usage
		encoded, err := json.Marshal(response)
		if err != nil {
			t.Fatal(err)
		}
		result, err := decodeOpenAIResponse(encoded, "unused", 16384)
		if ErrorCodeOf(err) != ErrorMalformed || len(result.IntentJSON) != 0 || result.Usage.TotalTokens != 0 {
			t.Fatalf("invalid usage accepted: %v", usage)
		}
	}
}
