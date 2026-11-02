package boardfamily

import (
	"context"
	"encoding/json"
	"net/http"

	"kicadai/internal/aiprovider"
)

const SourceAddressedRequestRevision = "source-addressed-request-12"
const SourceAddressedMaxRequestBytes = 16000

// Separate identity, unchanged pinned model and conservative standard rates.
// $2/$8 per million input/output tokens, verified 2026-09-17 at
// https://developers.openai.com/api/docs/models/gpt-4.1 . Never reuse v8 funds.
const SourceAddressedAccountingProfile = "source-addressed-v9-gpt-4.1-full-standard-2026-09-17"

func prepareSourceAddressedGenerateRequest(prompt string) (aiprovider.GenerateRequest, ReferencedRequest, error) {
	input, err := PrepareSourceAddressedRequest(prompt)
	if err != nil {
		return aiprovider.GenerateRequest{}, input.Source, err
	}
	schema, err := SourceAddressedEvidenceSchema(prompt)
	if err != nil {
		return aiprovider.GenerateRequest{}, input.Source, err
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		return aiprovider.GenerateRequest{}, input.Source, err
	}
	return aiprovider.GenerateRequest{Prompt: string(encoded), CapabilityContext: sourceAddressedContext(),
		DirectSourceJSON: true, OutputSchemaName: SourceAddressedSchemaName, OutputSchema: schema,
		SchemaVersion: aiprovider.EnvelopeSchemaV1, Attempt: 1, MaxOutputTokens: 1600}, input.Source, nil
}

// InterpretSourceAddressedWithJournal requires explicit opt-in, a new journal,
// and a separate policy/ledger. None of these by itself grants spending authority.
func InterpretSourceAddressedWithJournal(ctx context.Context, prompt, ledgerPath string, policy LedgerPolicy, base http.RoundTripper, journalPath string) (ReferencedSelection, error) {
	return interpretProtocolJournal(ctx, prompt, ledgerPath, policy, base, journalPath, sourceAddressedProtocol)
}

// InspectSourceAddressedJournal replays local bytes without credentials/network.
func InspectSourceAddressedJournal(root string) (ReferencedJournalAudit, error) {
	return inspectProtocolJournal(root, sourceAddressedProtocol)
}

// SourceAddressedEvidenceContract describes the exact prepared request without
// dispatching it. Export is offline and never grants spending authority.
func SourceAddressedEvidenceContract(prompt string) (map[string]any, error) {
	request, source, err := prepareSourceAddressedGenerateRequest(prompt)
	if err != nil {
		return nil, err
	}
	var input SourceAddressedRequest
	if err := json.Unmarshal([]byte(request.Prompt), &input); err != nil {
		return nil, err
	}
	return map[string]any{
		"admission_version": SourceAddressedVersion, "request_revision": SourceAddressedRequestRevision,
		"schema_name": request.OutputSchemaName, "schema": request.OutputSchema, "source": source,
		"input": input, "capability_context": request.CapabilityContext,
		"model": GroundedFullModel, "max_output_tokens": request.MaxOutputTokens,
		"accounting_profile": SourceAddressedAccountingProfile, "max_request_bytes": SourceAddressedMaxRequestBytes,
		"journal_version": sourceAddressedProtocol.journalVersion(), "experimental": true,
		"stage": "experimental-integration-no-live-acceptance", "export_dispatches_request": false,
		"live_authorization_granted": false, "destination": "https://api.openai.com/v1/responses",
		"limitations":   "Source slots prevent invented identities/anchors, not wrong state, scope, context-only classification, or omitted additional requirements. Synthetic checks are not live acceptance.",
		"other_payload": "Complete original request, application-owned clause/quantity/mention tables, v9 extraction instructions and request-specific schema. Direct input JSON, attempt 1, no retries, diagnostics, gold answers, catalog, source code, geometry, environment values or credentials in the body. Requires new explicit payload/spending authorization, a separate source-addressed version-3 ledger and a new source-addressed-v9 evidence journal; this export grants none.",
	}, nil
}
