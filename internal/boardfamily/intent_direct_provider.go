package boardfamily

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"kicadai/internal/aiprovider"
)

// Direct source is a separate candidate, not a reinterpretation of any v5 run.
// It changes message framing only. Fact semantics, admission and CAD are reused.
const DirectEvidenceVersion = "6-direct-source-experimental"
const DirectEvidenceSchemaName = "board_family_direct_requirements_v6"
const DirectEvidenceRequestRevision = "direct-request-07"
const DirectEvidenceMaxRequestBytes = ConnectionEvidenceMaxRequestBytes

func prepareDirectGenerateRequest(prompt string) (aiprovider.GenerateRequest, ReferencedRequest, error) {
	request, source, err := prepareConnectionGenerateRequest(prompt)
	if err != nil {
		return request, source, err
	}
	request.DirectSourceJSON = true
	request.CapabilityContext = strings.Replace(request.CapabilityContext, ConnectionEvidenceRequestRevision, DirectEvidenceRequestRevision, 1)
	request.OutputSchemaName = DirectEvidenceSchemaName
	request.OutputSchema["properties"].(map[string]any)["version"] = enumSchema(DirectEvidenceVersion)
	return request, source, nil
}

// DirectEvidenceContract exports the exact key-free candidate input. It grants
// no authority to send it or to spend money. All original constraints remain.
func DirectEvidenceContract(prompt string) (map[string]any, error) {
	request, source, err := prepareDirectGenerateRequest(prompt)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"admission_version": DirectEvidenceVersion, "request_revision": DirectEvidenceRequestRevision,
		"destination": "https://api.openai.com/v1/responses", "model": SelectionModel, "experimental": true,
		"capability_context": request.CapabilityContext, "source": source,
		"schema_name": request.OutputSchemaName, "schema": request.OutputSchema,
		"max_output_tokens": request.MaxOutputTokens, "max_request_bytes": DirectEvidenceMaxRequestBytes,
		"message_layout": "direct-source-json-with-application-instructions",
		"other_payload":  "Complete original request and application-owned clause/quantity tables are one user-input JSON object. Static extraction instructions are appended to the provider instructions. Facts retain connection-v5 semantics under a distinct version. attempt=1, no diagnostics, retries, catalog, gold answers, source code, geometry or credentials in model input. Separate fresh budget approval and a new immutable direct-v6 journal are required.",
	}, nil
}

// DecodeDirectEvidenceIntent validates the distinct envelope before a pure,
// internal version lowering. It never edits provider bytes or historical data.
func DecodeDirectEvidenceIntent(prompt string, raw []byte) (Decision, error) {
	failure := localDecision(prompt, "clarify", "The direct-source extraction could not be validated; no board was generated.", nil)
	if len(raw) > 65536 {
		return failure, errors.New("direct evidence exceeds 65536 bytes")
	}
	if err := validateIntentJSON(raw); err != nil {
		return failure, err
	}
	var envelope struct {
		Version string            `json:"version"`
		Facts   []json.RawMessage `json:"facts"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return failure, err
	}
	if !exactFields(raw, "version", "facts") || envelope.Version != DirectEvidenceVersion || envelope.Facts == nil || len(envelope.Facts) > 64 {
		return failure, errors.New("invalid direct evidence version or fact inventory")
	}
	envelope.Version = ConnectionEvidenceVersion
	lowered, err := json.Marshal(envelope)
	if err != nil {
		return failure, err
	}
	return DecodeConnectionEvidenceIntent(prompt, lowered)
}

func InterpretDirectWithJournal(ctx context.Context, prompt, ledgerPath string, policy LedgerPolicy, base http.RoundTripper, journalPath string) (ReferencedSelection, error) {
	return interpretProtocolJournal(ctx, prompt, ledgerPath, policy, base, journalPath, directProtocol)
}

func InspectDirectJournal(root string) (ReferencedJournalAudit, error) {
	return inspectProtocolJournal(root, directProtocol)
}
