package airequirementeval

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"kicadai/internal/aiprovider"
	"kicadai/internal/behavioralintent"
)

type captureTransport struct {
	body  []byte
	calls int
}

func (t *captureTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.calls++
	var err error
	t.body, err = io.ReadAll(io.LimitReader(req.Body, MaxRequestBytes+1))
	if closeErr := req.Body.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return nil, err
	}
	return nil, errors.New("offline request captured; no network transport exists")
}

type Preflight struct {
	Schema               string             `json:"schema"`
	CapabilitySHA256     string             `json:"capability_sha256"`
	ProviderSchemaSHA256 string             `json:"provider_schema_sha256"`
	InstructionSHA256    string             `json:"instruction_sha256"`
	CorpusSHA256         string             `json:"corpus_sha256"`
	LiveRequests         int                `json:"live_requests"`
	ResourcePreflight    map[string]any     `json:"resource_preflight"`
	Requests             []PreflightRequest `json:"requests"`
}
type PreflightRequest struct {
	CaseID string `json:"case_id"`
	Form   string `json:"form"`
	Bytes  int    `json:"bytes"`
	SHA256 string `json:"sha256"`
}

// Prepare uses a capturing fake transport: it cannot send an HTTP request.
func Prepare(ctx context.Context, repo, output string) error {
	var corpus Corpus
	corpusPath := filepath.Join(repo, SpecPath, "corpus.json")
	if err := readJSON(corpusPath, &corpus); err != nil {
		return err
	}
	if err := corpus.Validate(); err != nil {
		return err
	}
	capabilities, err := loadCapabilities(ctx)
	if err != nil {
		return err
	}
	if err := validateCapabilities(corpus, capabilities); err != nil {
		return err
	}
	if err := os.Mkdir(output, 0o700); err != nil {
		return err
	}
	if err := writeNew(filepath.Join(output, "installed-capabilities.json"), capabilities); err != nil {
		return err
	}
	schema, err := json.Marshal(aiprovider.BehavioralIntentProfile("").IntentEnvelopeSchema())
	if err != nil {
		return err
	}
	if err := writeNew(filepath.Join(output, "provider-schema.json"), schema); err != nil {
		return err
	}
	corpusBytes, err := os.ReadFile(corpusPath)
	if err != nil {
		return err
	}
	report := Preflight{Schema: "kicadai.interface-preflight.v1", CapabilitySHA256: sha(capabilities), ProviderSchemaSHA256: sha(schema), CorpusSHA256: sha(corpusBytes)}
	for _, item := range corpus.Cases {
		providerContext, err := behavioralintent.BuildProviderContext(item.Prompt, capabilities)
		if err != nil {
			return err
		}
		for _, form := range []string{"initial", "max_diagnostics"} {
			attempt := 1
			var diagnostics []aiprovider.Diagnostic
			if form == "max_diagnostics" {
				attempt = 2
				for range aiprovider.MaxDiagnostics {
					diagnostics = append(diagnostics, aiprovider.Diagnostic{Code: strings.Repeat("c", 512), Path: strings.Repeat("p", 512), Message: strings.Repeat("m", aiprovider.MaxDiagnosticLen)})
				}
			}
			request := requestFor(item.Prompt, providerContext, attempt, diagnostics)
			capture := &captureTransport{}
			provider, err := aiprovider.NewOpenAIProvider(aiprovider.OpenAIOptions{APIKey: "offline-preflight", Model: Model, MaxOutputTokens: MaxOutputTokens, HTTPClient: &http.Client{Transport: capture}})
			if err != nil {
				return err
			}
			_, providerErr := provider.GenerateIntent(ctx, request)
			if providerErr == nil || capture.calls != 1 {
				return fmt.Errorf("offline preflight did not capture exactly one attempted request")
			}
			var wire wireRequest
			if err := json.Unmarshal(capture.body, &wire); err != nil {
				return err
			}
			if report.InstructionSHA256 == "" {
				report.InstructionSHA256 = sha([]byte(wire.Instructions))
			}
			if err := validateWire(capture.body, request, report.InstructionSHA256); err != nil {
				return err
			}
			if err := writeNew(filepath.Join(output, item.ID+"."+form+".request.json"), capture.body); err != nil {
				return err
			}
			report.Requests = append(report.Requests, PreflightRequest{item.ID, form, len(capture.body), sha(capture.body)})
		}
	}
	rss, err := processTreeRSS(ctx)
	if err != nil {
		return fmt.Errorf("offline RSS preflight: %w", err)
	}
	disk, err := directoryBytes(output)
	if err != nil {
		return err
	}
	_, rootErr := os.Lstat(EvidenceRoot)
	if !os.IsNotExist(rootErr) {
		return fmt.Errorf("frozen live root must not exist during preparation")
	}
	report.ResourcePreflight = map[string]any{"process_tree_rss_bytes": rss, "snapshot_bytes_before_report": disk, "live_root_absent": true, "account_access_probed": false}
	return writeJSON(filepath.Join(output, "preflight.json"), report)
}
