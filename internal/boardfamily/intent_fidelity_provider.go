package boardfamily

import (
	"context"
	"encoding/json"
	"net/http"

	"kicadai/internal/aiprovider"
)

// FidelityInstructions adds general scope distinctions without corpus IDs,
// expected answers, text substitutions, or decoder-side repairs.
const FidelityInstructions = RequirementCoverageInstructions + `
Scope distinctions:
- Accepting a specific named choice (for example, "the fast profile is acceptable") selects it: requested. unnecessary_but_allowed requires an actual statement that the item is not needed. Mere politeness or acceptance is not exclusion.
- Explicit restrictions on omission, substitution, adapters, calibration or changes are requirements, not task framing or background. If no application control or local slot represents the restriction itself, retain it as a constraints/other fact with requested state and the complete original residual as its source. Positive sensor mentions do not by themselves represent a no-substitution restriction.
- A clear restriction is not unresolved_choice merely because it is unsupported or inconvenient. Use unclear only for genuine missing choices; detail must ask the targeted question. Keep the scope over every referenced requirement, including earlier clauses; do not silently narrow a shared restriction to its nearest noun.
`

// A separately selected integration of the offline prototype. Its wire version,
// journal and accounting identity are distinct; no old response is reinterpreted.
const FidelityEvidenceVersion = "12-requirement-fidelity-experimental"
const FidelityEvidenceSchemaName = "board_family_requirement_fidelity_v12"
const FidelityEvidenceRequestRevision = "requirement-fidelity-request-15"
const FidelityEvidenceMaxRequestBytes = 16000

// Rechecked against the official GPT-4.1 model page on 2026-09-20. The
// established conservative $2/$8 per million uncached input/output rates remain.
const FidelityEvidenceAccountingProfile = "requirement-fidelity-v12-gpt-4.1-full-standard-2026-09-20"

func FidelityEvidenceSchema(prompt string) (map[string]any, error) {
	return requirementCoverageSchema(prompt, FidelityEvidenceVersion)
}

func CompileFidelityEvidence(prompt string, raw []byte) ([]byte, error) {
	return compileRequirementCoverage(prompt, raw, FidelityEvidenceVersion)
}

func DecodeFidelityEvidenceIntent(prompt string, raw []byte) (Decision, error) {
	compiled, err := CompileFidelityEvidence(prompt, raw)
	if err != nil {
		return localDecision(prompt, "clarify", "The requirement-coverage extraction could not be validated; no board was generated.", nil), err
	}
	return decodeConnectionEvidenceWithExplanation(prompt, compiled, groundedMaxAssertions, noSubstitutionExplanation)
}

func prepareFidelityGenerateRequest(prompt string) (aiprovider.GenerateRequest, ReferencedRequest, error) {
	input, err := PrepareRequirementCoverageRequest(prompt)
	if err != nil {
		return aiprovider.GenerateRequest{}, input.Source, err
	}
	schema, err := FidelityEvidenceSchema(prompt)
	if err != nil {
		return aiprovider.GenerateRequest{}, input.Source, err
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		return aiprovider.GenerateRequest{}, input.Source, err
	}
	return aiprovider.GenerateRequest{
		Prompt: string(encoded), CapabilityContext: "Request contract: " + FidelityEvidenceRequestRevision + ".\n" + FidelityInstructions,
		DirectSourceJSON: true, OutputSchemaName: FidelityEvidenceSchemaName, OutputSchema: schema,
		SchemaVersion: aiprovider.EnvelopeSchemaV1, Attempt: 1, MaxOutputTokens: 1600,
	}, input.Source, nil
}

// InterpretFidelityWithJournal uses the existing no-retry transport and requires
// a separate policy/ledger and a fresh journal. These do not grant authorization.
func InterpretFidelityWithJournal(ctx context.Context, prompt, ledgerPath string, policy LedgerPolicy, base http.RoundTripper, journalPath string) (ReferencedSelection, error) {
	return interpretProtocolJournal(ctx, prompt, ledgerPath, policy, base, journalPath, fidelityProtocol)
}

func InspectFidelityJournal(root string) (ReferencedJournalAudit, error) {
	return inspectProtocolJournal(root, fidelityProtocol)
}

// FidelityEvidenceContract describes the request without a key or network call.
func FidelityEvidenceContract(prompt string) (map[string]any, error) {
	request, source, err := prepareFidelityGenerateRequest(prompt)
	if err != nil {
		return nil, err
	}
	var input RequirementCoverageRequest
	if err := json.Unmarshal([]byte(request.Prompt), &input); err != nil {
		return nil, err
	}
	return map[string]any{
		"admission_version": FidelityEvidenceVersion, "request_revision": FidelityEvidenceRequestRevision,
		"schema_name": request.OutputSchemaName, "schema": request.OutputSchema, "source": source,
		"input": input, "capability_context": request.CapabilityContext,
		"model": GroundedFullModel, "max_output_tokens": request.MaxOutputTokens,
		"accounting_profile": FidelityEvidenceAccountingProfile, "max_request_bytes": FidelityEvidenceMaxRequestBytes,
		"journal_version": fidelityProtocol.journalVersion(), "experimental": true,
		"stage": "experimental-integration-no-live-acceptance", "export_dispatches_request": false,
		"live_authorization_granted": false, "destination": "https://api.openai.com/v1/responses",
		"limitations":   "Coverage references prove local inventory, not semantic completeness. An incorrect represented/non_requirement classification can omit unfamiliar constraints. No live accuracy or speed improvement is established.",
		"other_payload": "Original request; source-owned clauses, quantities, mentions, controls, residuals, required states and local coverage references; standalone coverage instructions and request-specific schema. Direct input JSON, attempt 1, no retries. No gold answers, catalog, source code, geometry, environment values or credentials in the body. A separately approved payload/budget, isolated version-3 ledger and fresh requirement-fidelity-v12 journal are required; this export grants no authorization.",
	}, nil
}
