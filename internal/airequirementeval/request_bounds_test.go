package airequirementeval

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"kicadai/internal/aiprovider"
	"kicadai/internal/behavioralintent"
)

// This captures serialization only: the transport has no network delegate.
// The immutable corpus is read, never refrozen or scored as a new live run.
func TestOfflineRepairRequestFormsRemainWithinExistingWireBudget(t *testing.T) {
	var corpus Corpus
	if err := readJSON(filepath.Join("..", "..", SpecPath, "corpus.json"), &corpus); err != nil {
		t.Fatal(err)
	}
	capabilities, err := loadCapabilities(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	forms, largest := 0, 0
	for _, item := range corpus.Cases {
		providerContext, err := behavioralintent.BuildProviderContext(item.Prompt, capabilities)
		if err != nil {
			t.Fatal(err)
		}
		for _, correction := range []bool{false, true} {
			attempt := 1
			var diagnostics []aiprovider.Diagnostic
			if correction {
				attempt = 2
				for range aiprovider.MaxDiagnostics {
					diagnostics = append(diagnostics, aiprovider.Diagnostic{Code: strings.Repeat("c", 512), Path: strings.Repeat("p", 512), Message: strings.Repeat("m", aiprovider.MaxDiagnosticLen)})
				}
			}
			request := requestFor(item.Prompt, providerContext, attempt, diagnostics)
			capture := &captureTransport{}
			provider, err := aiprovider.NewOpenAIProvider(aiprovider.OpenAIOptions{APIKey: "offline-test", Model: Model, MaxOutputTokens: MaxOutputTokens, HTTPClient: &http.Client{Transport: capture}})
			if err != nil {
				t.Fatal(err)
			}
			_, err = provider.GenerateIntent(context.Background(), request)
			if err == nil || capture.calls != 1 {
				t.Fatal("offline capture did not fail closed after exactly one request")
			}
			var wire wireRequest
			if err := json.Unmarshal(capture.body, &wire); err != nil {
				t.Fatal(err)
			}
			if err := validateWire(capture.body, request, sha([]byte(wire.Instructions))); err != nil {
				t.Fatalf("%s correction=%v: %v", item.ID, correction, err)
			}
			forms++
			largest = max(largest, len(capture.body))
		}
	}
	if forms != 16 {
		t.Fatalf("unexpected request-form count: %d", forms)
	}
	t.Logf("captured %d initial/correction forms; largest=%d bytes; unchanged limit=%d bytes; live requests=0", forms, largest, MaxRequestBytes)
}
