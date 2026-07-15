package aiprovider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestOpenAIProviderSendsStrictResponsesRequest(t *testing.T) {
	client := clientWithRoundTrip(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodPost || request.URL.Path != "/v1/responses" {
			t.Errorf("request = %s %s", request.Method, request.URL.Path)
		}
		if request.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("authorization header missing")
		}
		var body openAIRequest
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if body.Model != "test-model" || body.Text.Format.Type != "json_schema" || !body.Text.Format.Strict || body.Text.Format.Name != "kicadai_bmp280_intent_v1" {
			t.Errorf("request body = %#v", body)
		}
		if body.Store || !body.Stream || body.MaxOutputTokens != DefaultReferenceOutputTokens || strings.Contains(body.Instructions, "sensor.bosch.bmp280.lga8") {
			t.Errorf("request policy = %#v", body)
		}
		var input openAIInput
		if err := json.Unmarshal([]byte(body.Input), &input); err != nil || input.Prompt != "build bmp280" || input.Attempt != 1 || !strings.Contains(input.CapabilityContext, "sensor.bosch.bmp280.lga8") {
			t.Errorf("input = %#v err=%v", input, err)
		}
		return jsonHTTPResponse(http.StatusOK, openAIResponseJSON(t, validEnvelope)), nil
	})

	provider, err := newOpenAIProvider(OpenAIOptions{APIKey: "test-key", Model: "test-model", HTTPClient: client}, openAIResponsesEndpoint)
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}
	result, err := provider.GenerateIntent(context.Background(), openAITestRequest("build bmp280"))
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if result.Provider != "openai" || result.Model != "test-model-resolved" || result.ResponseID != "resp_test" || result.Recorded {
		t.Fatalf("result metadata = %#v", result)
	}
	if result.Usage.TotalTokens != 15 {
		t.Fatalf("usage = %#v", result.Usage)
	}
	if result.MaxOutputTokens != DefaultReferenceOutputTokens {
		t.Fatalf("max output tokens = %d", result.MaxOutputTokens)
	}
}

func TestOpenAIProviderUsesBoundedOutputTokenOverride(t *testing.T) {
	client := clientWithRoundTrip(func(request *http.Request) (*http.Response, error) {
		var body openAIRequest
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.MaxOutputTokens != 24000 {
			t.Fatalf("max output tokens = %d", body.MaxOutputTokens)
		}
		return jsonHTTPResponse(http.StatusOK, openAIResponseJSON(t, validEnvelope)), nil
	})
	provider, err := newOpenAIProvider(OpenAIOptions{APIKey: "test-key", HTTPClient: client, MaxOutputTokens: 24000}, openAIResponsesEndpoint)
	if err != nil {
		t.Fatal(err)
	}
	result, err := provider.GenerateIntent(context.Background(), openAITestRequest("x"))
	if err != nil || result.MaxOutputTokens != 24000 {
		t.Fatalf("result=%#v err=%v", result, err)
	}
}

func TestOpenAIProviderReadsTerminalStreamingResponse(t *testing.T) {
	client := clientWithRoundTrip(func(_ *http.Request) (*http.Response, error) {
		body := openAIResponseJSON(t, validEnvelope)
		event := "event: response.created\ndata: {\"type\":\"response.created\"}\n\n" +
			"event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":" + body + "}\n\n"
		response := jsonHTTPResponse(http.StatusOK, event)
		response.Header.Set("Content-Type", "text/event-stream")
		return response, nil
	})
	provider, _ := newOpenAIProvider(OpenAIOptions{APIKey: "test-key", HTTPClient: client}, openAIResponsesEndpoint)
	result, err := provider.GenerateIntent(context.Background(), openAITestRequest("x"))
	if err != nil {
		t.Fatalf("generate from stream: %v", err)
	}
	if result.ResponseID != "resp_test" || len(result.IntentJSON) == 0 {
		t.Fatalf("result = %#v", result)
	}
}

func TestOpenAIProviderPollsBackgroundResponse(t *testing.T) {
	requests := 0
	client := clientWithRoundTrip(func(request *http.Request) (*http.Response, error) {
		requests++
		switch requests {
		case 1:
			if request.Method != http.MethodPost {
				t.Fatalf("initial method = %s", request.Method)
			}
			var body openAIRequest
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Fatalf("decode initial request: %v", err)
			}
			if !body.Background || !body.Store || body.Stream {
				t.Fatalf("background request policy = %#v", body)
			}
			return jsonHTTPResponse(http.StatusOK, `{"id":"resp_background","status":"queued"}`), nil
		case 2:
			if request.Method != http.MethodGet || request.URL.Path != "/v1/responses/resp_background" {
				t.Fatalf("poll request = %s %s", request.Method, request.URL.Path)
			}
			return jsonHTTPResponse(http.StatusOK, openAIResponseJSON(t, validEnvelope)), nil
		default:
			t.Fatalf("unexpected request %d", requests)
			return nil, nil
		}
	})
	provider, _ := newOpenAIProvider(OpenAIOptions{APIKey: "test-key", HTTPClient: client, Background: true}, openAIResponsesEndpoint)
	result, err := provider.GenerateIntent(context.Background(), openAITestRequest("x"))
	if err != nil {
		t.Fatalf("generate in background: %v", err)
	}
	if result.ResponseID != "resp_test" || requests != 2 {
		t.Fatalf("result=%#v requests=%d", result, requests)
	}
}

func TestOpenAIProviderRejectsStreamWithoutTerminalResponse(t *testing.T) {
	client := clientWithRoundTrip(func(_ *http.Request) (*http.Response, error) {
		response := jsonHTTPResponse(http.StatusOK, "event: response.created\ndata: {}\n\n")
		response.Header.Set("Content-Type", "text/event-stream")
		return response, nil
	})
	provider, _ := newOpenAIProvider(OpenAIOptions{APIKey: "test-key", HTTPClient: client}, openAIResponsesEndpoint)
	_, err := provider.GenerateIntent(context.Background(), openAITestRequest("x"))
	if ErrorCodeOf(err) != ErrorIncomplete {
		t.Fatalf("error = %v code=%q", err, ErrorCodeOf(err))
	}
}

func TestOpenAIProviderClassifiesHTTPFailuresWithoutLeakingBody(t *testing.T) {
	tests := []struct {
		name   string
		status int
		code   ErrorCode
	}{
		{name: "authentication", status: http.StatusUnauthorized, code: ErrorAuthentication},
		{name: "rate limit", status: http.StatusTooManyRequests, code: ErrorRateLimit},
		{name: "server", status: http.StatusInternalServerError, code: ErrorTransport},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := clientWithRoundTrip(func(_ *http.Request) (*http.Response, error) {
				return jsonHTTPResponse(test.status, `{"error":{"code":"test_error","message":"Authorization: Bearer leaked-secret"}}`), nil
			})
			provider, _ := newOpenAIProvider(OpenAIOptions{APIKey: "test-key", HTTPClient: client}, openAIResponsesEndpoint)
			_, err := provider.GenerateIntent(context.Background(), openAITestRequest("x"))
			if ErrorCodeOf(err) != test.code {
				t.Fatalf("error = %v code=%q", err, ErrorCodeOf(err))
			}
			if strings.Contains(err.Error(), "leaked-secret") {
				t.Fatalf("error leaked response body: %v", err)
			}
		})
	}
}

func TestOpenAIProviderRejectsRefusalIncompleteAndMultipleOutput(t *testing.T) {
	tests := []struct {
		name string
		body string
		code ErrorCode
	}{
		{name: "refusal", code: ErrorRefusal, body: `{"id":"r","status":"completed","model":"m","error":null,"incomplete_details":null,"output":[{"type":"message","status":"completed","content":[{"type":"refusal","refusal":"no"}]}],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`},
		{name: "incomplete", code: ErrorIncomplete, body: `{"id":"r","status":"incomplete","model":"m","error":null,"incomplete_details":{"reason":"max_output_tokens"},"output":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`},
		{name: "multiple", code: ErrorMalformed, body: fmt.Sprintf(`{"id":"r","status":"completed","model":"m","error":null,"incomplete_details":null,"output":[{"type":"message","status":"completed","content":[{"type":"output_text","text":%q},{"type":"output_text","text":%q}]}],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`, validEnvelope, validEnvelope)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := clientWithRoundTrip(func(_ *http.Request) (*http.Response, error) {
				return jsonHTTPResponse(http.StatusOK, test.body), nil
			})
			provider, _ := newOpenAIProvider(OpenAIOptions{APIKey: "test-key", HTTPClient: client}, openAIResponsesEndpoint)
			_, err := provider.GenerateIntent(context.Background(), openAITestRequest("x"))
			if ErrorCodeOf(err) != test.code {
				t.Fatalf("error = %v code=%q, want %q", err, ErrorCodeOf(err), test.code)
			}
		})
	}
}

func TestOpenAIProviderReportsOutputTokenExhaustion(t *testing.T) {
	for _, background := range []bool{false, true} {
		t.Run(fmt.Sprintf("background_%t", background), func(t *testing.T) {
			body := `{"id":"resp_limit","status":"incomplete","model":"test-model","error":null,"incomplete_details":{"reason":"max_output_tokens"},"output":[],"usage":{"input_tokens":120,"output_tokens":32768,"total_tokens":32888}}`
			client := clientWithRoundTrip(func(_ *http.Request) (*http.Response, error) {
				return jsonHTTPResponse(http.StatusOK, body), nil
			})
			provider, err := newOpenAIProvider(OpenAIOptions{APIKey: "test-key", HTTPClient: client, Background: background}, openAIResponsesEndpoint)
			if err != nil {
				t.Fatal(err)
			}
			request := openAITestRequest("x")
			request.MaxOutputTokens = DefaultGenericOutputTokens
			_, err = provider.GenerateIntent(context.Background(), request)
			var providerErr *ProviderError
			if !errors.As(err, &providerErr) {
				t.Fatalf("error = %v", err)
			}
			if providerErr.Code != ErrorIncomplete || providerErr.IncompleteReason != "max_output_tokens" || providerErr.MaxOutputTokens != DefaultGenericOutputTokens || providerErr.Usage.OutputTokens != DefaultGenericOutputTokens || !providerErr.RetryAllowed {
				t.Fatalf("provider error = %#v", providerErr)
			}
			if !strings.Contains(providerErr.Suggestion, "--ai-max-output-tokens") || !strings.Contains(providerErr.Error(), "limit=32768") {
				t.Fatalf("provider guidance = %#v", providerErr)
			}
		})
	}
}

func TestOpenAIProviderEnforcesResponseLimitAndTimeout(t *testing.T) {
	t.Run("response limit", func(t *testing.T) {
		client := clientWithRoundTrip(func(_ *http.Request) (*http.Response, error) {
			return jsonHTTPResponse(http.StatusOK, strings.Repeat("x", MaxResponseBytes+1)), nil
		})
		provider, _ := newOpenAIProvider(OpenAIOptions{APIKey: "test-key", HTTPClient: client}, openAIResponsesEndpoint)
		_, err := provider.GenerateIntent(context.Background(), openAITestRequest("x"))
		if ErrorCodeOf(err) != ErrorMalformed {
			t.Fatalf("error = %v code=%q", err, ErrorCodeOf(err))
		}
	})

	t.Run("timeout", func(t *testing.T) {
		client := clientWithRoundTrip(func(_ *http.Request) (*http.Response, error) {
			return nil, context.DeadlineExceeded
		})
		provider, _ := newOpenAIProvider(OpenAIOptions{APIKey: "test-key", HTTPClient: client}, openAIResponsesEndpoint)
		_, err := provider.GenerateIntent(context.Background(), openAITestRequest("x"))
		if ErrorCodeOf(err) != ErrorTimeout {
			t.Fatalf("error = %v code=%q", err, ErrorCodeOf(err))
		}
	})
}

func TestNewOpenAIProviderRequiresKey(t *testing.T) {
	if _, err := NewOpenAIProvider(OpenAIOptions{}); ErrorCodeOf(err) != ErrorConfiguration {
		t.Fatalf("error = %v code=%q", err, ErrorCodeOf(err))
	}
}

func TestOpenAIOptionsFromEnvironmentParsesOutputTokenLimit(t *testing.T) {
	t.Setenv(EnvAIMaxOutputTokens, "24000")
	if got := OpenAIOptionsFromEnvironment().MaxOutputTokens; got != 24000 {
		t.Fatalf("max output tokens = %d", got)
	}
	t.Setenv(EnvAIMaxOutputTokens, "not-a-number")
	if _, err := NewOpenAIProvider(OpenAIOptions{APIKey: "test-key", MaxOutputTokens: OpenAIOptionsFromEnvironment().MaxOutputTokens}); ErrorCodeOf(err) != ErrorConfiguration {
		t.Fatalf("invalid environment limit error = %v", err)
	}
}

func TestOpenAIProviderRequiresCapabilityContextAndSchema(t *testing.T) {
	provider, _ := newOpenAIProvider(OpenAIOptions{APIKey: "test-key", HTTPClient: clientWithRoundTrip(func(_ *http.Request) (*http.Response, error) {
		t.Fatal("provider should fail before HTTP")
		return nil, nil
	})}, openAIResponsesEndpoint)
	tests := []struct {
		name   string
		mutate func(*GenerateRequest)
	}{
		{name: "capability", mutate: func(request *GenerateRequest) { request.CapabilityContext = "" }},
		{name: "schema", mutate: func(request *GenerateRequest) { request.OutputSchema = nil }},
		{name: "schema name", mutate: func(request *GenerateRequest) { request.OutputSchemaName = "bad schema" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := openAITestRequest("x")
			test.mutate(&request)
			_, err := provider.GenerateIntent(context.Background(), request)
			if ErrorCodeOf(err) != ErrorConfiguration {
				t.Fatalf("error = %v code=%q", err, ErrorCodeOf(err))
			}
		})
	}
}

func openAITestRequest(prompt string) GenerateRequest {
	return GenerateRequest{
		Prompt:            prompt,
		CapabilityContext: BMP280ReferenceCapabilityContext,
		OutputSchemaName:  "kicadai_bmp280_intent_v1",
		OutputSchema:      BMP280ReferenceIntentEnvelopeSchema(),
		SchemaVersion:     EnvelopeSchemaV1,
		Attempt:           1,
		MaxOutputTokens:   DefaultReferenceOutputTokens,
	}
}

func openAIResponseJSON(t *testing.T, output string) string {
	t.Helper()
	response := map[string]any{
		"id":                 "resp_test",
		"object":             "response",
		"created_at":         1,
		"status":             "completed",
		"model":              "test-model-resolved",
		"error":              nil,
		"incomplete_details": nil,
		"output": []any{map[string]any{
			"type":   "message",
			"status": "completed",
			"content": []any{map[string]any{
				"type": "output_text",
				"text": output,
			}},
		}},
		"usage": map[string]any{"input_tokens": 10, "output_tokens": 5, "total_tokens": 15},
	}
	data, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("encode response: %v", err)
	}
	return string(data)
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func clientWithRoundTrip(function roundTripFunc) *http.Client {
	return &http.Client{Transport: function}
}

func jsonHTTPResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}
