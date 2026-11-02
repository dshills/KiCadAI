package boardfamily

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"kicadai/internal/aiprovider"
)

const SourceEligibleRequestRevision = "source-eligible-request-11"
const SourceEligibleMaxRequestBytes = 16000

// Same pinned model and conservative full-model rates; separate protocol
// identity prevents recycling the completed v7 evaluation's ledger or budget.
const SourceEligibleAccountingProfile = "source-eligible-v8-gpt-4.1-full-standard-2026-09-16"

type sourceEligibleInput struct {
	Source      ReferencedRequest `json:"source"`
	Eligibility sourceEligibility `json:"eligibility"`
}

func sourceEligibleLanguageContext() string {
	return strings.ReplaceAll(GroundedEvidenceLanguageContext(), GroundedEvidenceRequestRevision, SourceEligibleRequestRevision) + `
Feature labels require a source-compatible anchor. Copy anchor from that feature's feature_anchors and include it in evidence. An eligible mention is not proof that the feature is requested: determine state and scope from the whole request. Do not infer firmware from USB power, or a general peripheral ban from a ban on an external adapter. Preserve an unfamiliar requirement exactly as other rather than dropping it or forcing a feature label.
quantity_roles narrows dimensional fields when an explicit precision/tolerance phrase is attached to that exact quantity. Empty roles requires other with the precise requested meaning, or unclear for genuine ambiguity. For example, temperature accuracy within 0.2 C is a tolerance, not an operating range; operating at 15 to 30 C and accuracy within 0.2 C has two different quantities. Do not discard either.
`
}

func prepareSourceEligibleGenerateRequest(prompt string) (aiprovider.GenerateRequest, ReferencedRequest, error) {
	source, eligibility, err := eligibleSource(prompt)
	if err != nil {
		return aiprovider.GenerateRequest{}, source, err
	}
	schema, err := SourceEligibleEvidenceSchema(prompt)
	if err != nil {
		return aiprovider.GenerateRequest{}, source, err
	}
	encoded, err := json.Marshal(sourceEligibleInput{source, eligibility})
	if err != nil {
		return aiprovider.GenerateRequest{}, source, err
	}
	return aiprovider.GenerateRequest{Prompt: string(encoded), CapabilityContext: sourceEligibleLanguageContext(),
		DirectSourceJSON: true, OutputSchemaName: SourceEligibleSchemaName, OutputSchema: schema,
		SchemaVersion: aiprovider.EnvelopeSchemaV1, Attempt: 1, MaxOutputTokens: 1600}, source, nil
}

// InterpretSourceEligibleWithJournal is explicit opt-in only. A new journal and
// separate budget are required; their existence is not spending authorization.
func InterpretSourceEligibleWithJournal(ctx context.Context, prompt, ledgerPath string, policy LedgerPolicy, base http.RoundTripper, journalPath string) (ReferencedSelection, error) {
	return interpretProtocolJournal(ctx, prompt, ledgerPath, policy, base, journalPath, sourceEligibleProtocol)
}

// InspectSourceEligibleJournal replays only local bytes, without a key or network.
func InspectSourceEligibleJournal(root string) (ReferencedJournalAudit, error) {
	return inspectProtocolJournal(root, sourceEligibleProtocol)
}

// SourceEligibleEvidenceContract exports exactly the schema, instructions and
// direct input JSON used by preparation. Exporting it grants no dispatch right.
func SourceEligibleEvidenceContract(prompt string) (map[string]any, error) {
	request, source, err := prepareSourceEligibleGenerateRequest(prompt)
	if err != nil {
		return nil, err
	}
	var input sourceEligibleInput
	if err := json.Unmarshal([]byte(request.Prompt), &input); err != nil {
		return nil, err
	}
	return map[string]any{
		"admission_version": SourceEligibleVersion, "request_revision": SourceEligibleRequestRevision,
		"schema_name": request.OutputSchemaName, "schema": request.OutputSchema, "source": source,
		"eligibility": input.Eligibility, "input": input, "capability_context": request.CapabilityContext,
		"model": GroundedFullModel, "max_output_tokens": request.MaxOutputTokens,
		"accounting_profile": SourceEligibleAccountingProfile, "max_request_bytes": SourceEligibleMaxRequestBytes,
		"journal_version": sourceEligibleProtocol.journalVersion(), "experimental": true,
		"live_authorization_granted": false, "destination": "https://api.openai.com/v1/responses",
		"limitations":   "Lexical eligibility is a necessary guard, not a semantic entailment or completeness proof. Unknown wording uses other/unclear. Offline integration tests do not establish live semantic accuracy.",
		"other_payload": "Complete original request, application-owned clause/quantity tables and lexical eligibility tables, v8 extraction instructions and request-specific schema. Direct input JSON, attempt 1, no retries, diagnostics, gold answers, catalog, source code, geometry, environment values or credentials in the body. Requires new explicit payload/spending authorization, a separate source-eligible version-3 ledger and a new source-eligible-v8 evidence journal; this export grants none.",
	}, nil
}
