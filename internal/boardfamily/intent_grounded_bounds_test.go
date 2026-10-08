package boardfamily

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"kicadai/internal/aiprovider"
)

func TestGroundedMaximumInventoryAndProviderRequestBounds(t *testing.T) {
	inventory := "Use BMP280 or SHT31 with " + strings.Repeat("0 C, 0 C, 0 C, 0 C. ", 32)
	prompts := []string{inventory, inventory[:len(inventory)-2] + strings.Repeat("\x00", 2000-len(inventory)) + ". ", strings.Repeat("\x00", 2000), strings.Repeat("界", 666), strings.Repeat(`"\`, 1000), "Use BMP280 with 100 pF."}
	for i, prompt := range prompts {
		request, source, err := prepareGroundedGenerateRequest(prompt)
		if err != nil {
			t.Fatal(err)
		}
		if i < 2 && (len(source.Clauses) != 32 || len(source.Quantities) != 128) {
			t.Fatal("maximum source scope reduced")
		}
		enums, properties, characters := 0, 0, 0
		var walk func(any)
		walk = func(value any) {
			switch n := value.(type) {
			case map[string]any:
				if values, ok := n["enum"].([]string); ok {
					enums += len(values)
					for _, s := range values {
						characters += len(s)
					}
				}
				if props, ok := n["properties"].(map[string]any); ok {
					properties += len(props)
					if n["additionalProperties"] != false || len(n["required"].([]string)) != len(props) {
						t.Fatal("non-closed schema object")
					}
					for key := range props {
						characters += len(key)
					}
				}
				if defs, ok := n["$defs"].(map[string]any); ok {
					for key := range defs {
						characters += len(key)
					}
				}
				for _, child := range n {
					walk(child)
				}
			case []any:
				for _, child := range n {
					walk(child)
				}
			}
		}
		walk(request.OutputSchema)
		if enums > 1000 || properties > 5000 || characters > 120000 {
			t.Fatalf("schema bounds exceeded: %d enums, %d properties, %d chars", enums, properties, characters)
		}
		calls, bodyBytes := 0, 0
		p, err := aiprovider.NewOpenAIProvider(aiprovider.OpenAIOptions{APIKey: "offline-partitioned-placeholder", Model: SelectionModel, MaxOutputTokens: 1600,
			HTTPClient: &http.Client{Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				body, err := io.ReadAll(r.Body)
				if err = errors.Join(err, r.Body.Close()); err != nil {
					t.Fatal(err)
				}
				bodyBytes = len(body)
				if bodyBytes > 65536 || bytes.Contains(body, []byte("offline-partitioned-placeholder")) {
					t.Fatal("request bound or credential boundary changed", bodyBytes)
				}
				var wire struct {
					Model, Input, Instructions string
					MaxOutputTokens            int `json:"max_output_tokens"`
					Store, Stream, Background  bool
				}
				if err := json.Unmarshal(body, &wire); err != nil || wire.Model != SelectionModel || wire.Input != request.Prompt || wire.MaxOutputTokens != 1600 || wire.Store || wire.Background || !wire.Stream || !strings.HasSuffix(wire.Instructions, "\n\n"+request.CapabilityContext) {
					t.Fatal("provider envelope changed", err)
				}
				return nil, errors.New("intentional offline transport stop; no socket")
			})}})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := p.GenerateJSON(context.Background(), request); aiprovider.ErrorCodeOf(err) != aiprovider.ErrorTransport || calls != 1 {
			t.Fatal("offline request did not make exactly one in-memory attempt", err, calls)
		}
		t.Logf("fixture %d: %d clauses, %d quantities, %d body bytes, %d enum members, %d properties, %d schema string characters", i, len(source.Clauses), len(source.Quantities), bodyBytes, enums, properties, characters)
	}
}
