package boardfamily

import (
	"context"
	"encoding/json"
	"net/http"

	"kicadai/internal/aiprovider"
)

const SemanticBoundarySchemaName = "board_family_semantic_boundaries_v10"
const SemanticBoundaryRequestRevision = "semantic-boundary-request-13"
const SemanticBoundaryMaxRequestBytes = 16000

// Isolated ledger identity; the pinned full model and conservative standard
// input/output rates are unchanged from v9. No historical funds are reused.
const SemanticBoundaryAccountingProfile = "semantic-boundary-v10-gpt-4.1-full-standard-2026-09-17"

func prepareSemanticBoundaryGenerateRequest(prompt string) (aiprovider.GenerateRequest, ReferencedRequest, error) {
	input, err := PrepareSemanticBoundaryRequest(prompt)
	if err != nil {
		return aiprovider.GenerateRequest{}, input.Source, err
	}
	schema, err := SemanticBoundarySchema(prompt)
	if err != nil {
		return aiprovider.GenerateRequest{}, input.Source, err
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		return aiprovider.GenerateRequest{}, input.Source, err
	}
	return aiprovider.GenerateRequest{Prompt: string(encoded), CapabilityContext: semanticBoundaryContext(),
		DirectSourceJSON: true, OutputSchemaName: SemanticBoundarySchemaName, OutputSchema: schema,
		SchemaVersion: aiprovider.EnvelopeSchemaV1, Attempt: 1, MaxOutputTokens: 1600}, input.Source, nil
}

// InterpretSemanticBoundaryWithJournal requires explicit opt-in, a new journal
// and a separate policy/ledger. None of these grants spending authorization.
func InterpretSemanticBoundaryWithJournal(ctx context.Context, prompt, ledgerPath string, policy LedgerPolicy, base http.RoundTripper, journalPath string) (ReferencedSelection, error) {
	return interpretProtocolJournal(ctx, prompt, ledgerPath, policy, base, journalPath, semanticBoundaryProtocol)
}

// InspectSemanticBoundaryJournal replays local bytes without credentials/network.
func InspectSemanticBoundaryJournal(root string) (ReferencedJournalAudit, error) {
	return inspectProtocolJournal(root, semanticBoundaryProtocol)
}

// SemanticBoundaryEvidenceContract exports the exact request components
// without dispatching a request or granting spending authorization.
func SemanticBoundaryEvidenceContract(prompt string) (map[string]any, error) {
	request, source, err := prepareSemanticBoundaryGenerateRequest(prompt)
	if err != nil {
		return nil, err
	}
	var input SemanticBoundaryRequest
	if err := json.Unmarshal([]byte(request.Prompt), &input); err != nil {
		return nil, err
	}
	return map[string]any{
		"admission_version": SemanticBoundaryVersion, "request_revision": SemanticBoundaryRequestRevision,
		"schema_name": request.OutputSchemaName, "schema": request.OutputSchema, "source": source,
		"input": input, "capability_context": request.CapabilityContext,
		"model": GroundedFullModel, "max_output_tokens": request.MaxOutputTokens,
		"accounting_profile": SemanticBoundaryAccountingProfile, "max_request_bytes": SemanticBoundaryMaxRequestBytes,
		"journal_version": semanticBoundaryProtocol.journalVersion(), "experimental": true,
		"stage": "experimental-integration-no-live-acceptance", "export_dispatches_request": false,
		"live_authorization_granted": false, "destination": "https://api.openai.com/v1/responses",
		"limitations":   "Source-owned text and narrow grammatical guards prevent specific transformations, not every incorrect state, scope, omission or context-only classification. Synthetic checks are not live acceptance.",
		"other_payload": "Complete original request, application-owned clauses, quantities, mentions, control spans, residual spans and required states; v10 extraction instructions and request-specific schema. Direct input JSON, attempt 1, no retries, diagnostics, gold answers, catalog, source code, geometry, environment values or credentials in the body. Requires new explicit payload/spending authorization, a separate semantic-boundary version-3 ledger and a new semantic-boundaries-v10 evidence journal; this export grants none.",
	}, nil
}
