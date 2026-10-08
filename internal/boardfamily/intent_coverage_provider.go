package boardfamily

import (
	"context"
	"encoding/json"
	"net/http"

	"kicadai/internal/aiprovider"
)

// A separately selected integration of the offline prototype. Its wire version,
// journal and accounting identity are distinct; no old response is reinterpreted.
const CoverageEvidenceVersion = "11-requirement-coverage-experimental"
const CoverageEvidenceSchemaName = "board_family_requirement_coverage_v11"
const CoverageEvidenceRequestRevision = "requirement-coverage-request-14"
const CoverageEvidenceMaxRequestBytes = 16000

// Rechecked against the official GPT-4.1 model page on 2026-09-20. The
// established conservative $2/$8 per million uncached input/output rates remain.
const CoverageEvidenceAccountingProfile = "requirement-coverage-v11-gpt-4.1-full-standard-2026-09-20"

func CoverageEvidenceSchema(prompt string) (map[string]any, error) {
	return requirementCoverageSchema(prompt, CoverageEvidenceVersion)
}

func CompileCoverageEvidence(prompt string, raw []byte) ([]byte, error) {
	return compileRequirementCoverage(prompt, raw, CoverageEvidenceVersion)
}

func DecodeCoverageEvidenceIntent(prompt string, raw []byte) (Decision, error) {
	compiled, err := CompileCoverageEvidence(prompt, raw)
	if err != nil {
		return localDecision(prompt, "clarify", "The requirement-coverage extraction could not be validated; no board was generated.", nil), err
	}
	return decodeConnectionEvidenceWithLimit(prompt, compiled, groundedMaxAssertions)
}

func prepareCoverageGenerateRequest(prompt string) (aiprovider.GenerateRequest, ReferencedRequest, error) {
	input, err := PrepareRequirementCoverageRequest(prompt)
	if err != nil {
		return aiprovider.GenerateRequest{}, input.Source, err
	}
	schema, err := CoverageEvidenceSchema(prompt)
	if err != nil {
		return aiprovider.GenerateRequest{}, input.Source, err
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		return aiprovider.GenerateRequest{}, input.Source, err
	}
	return aiprovider.GenerateRequest{
		Prompt: string(encoded), CapabilityContext: "Request contract: " + CoverageEvidenceRequestRevision + ".\n" + RequirementCoverageInstructions,
		DirectSourceJSON: true, OutputSchemaName: CoverageEvidenceSchemaName, OutputSchema: schema,
		SchemaVersion: aiprovider.EnvelopeSchemaV1, Attempt: 1, MaxOutputTokens: 1600,
	}, input.Source, nil
}

// InterpretCoverageWithJournal uses the existing no-retry transport and requires
// a separate policy/ledger and a fresh journal. These do not grant authorization.
func InterpretCoverageWithJournal(ctx context.Context, prompt, ledgerPath string, policy LedgerPolicy, base http.RoundTripper, journalPath string) (ReferencedSelection, error) {
	return interpretProtocolJournal(ctx, prompt, ledgerPath, policy, base, journalPath, coverageProtocol)
}

func InspectCoverageJournal(root string) (ReferencedJournalAudit, error) {
	return inspectProtocolJournal(root, coverageProtocol)
}

// CoverageEvidenceContract describes the request without a key or network call.
func CoverageEvidenceContract(prompt string) (map[string]any, error) {
	request, source, err := prepareCoverageGenerateRequest(prompt)
	if err != nil {
		return nil, err
	}
	var input RequirementCoverageRequest
	if err := json.Unmarshal([]byte(request.Prompt), &input); err != nil {
		return nil, err
	}
	return map[string]any{
		"admission_version": CoverageEvidenceVersion, "request_revision": CoverageEvidenceRequestRevision,
		"schema_name": request.OutputSchemaName, "schema": request.OutputSchema, "source": source,
		"input": input, "capability_context": request.CapabilityContext,
		"model": GroundedFullModel, "max_output_tokens": request.MaxOutputTokens,
		"accounting_profile": CoverageEvidenceAccountingProfile, "max_request_bytes": CoverageEvidenceMaxRequestBytes,
		"journal_version": coverageProtocol.journalVersion(), "experimental": true,
		"stage": "experimental-integration-no-live-acceptance", "export_dispatches_request": false,
		"live_authorization_granted": false, "destination": "https://api.openai.com/v1/responses",
		"limitations":   "Coverage references prove local inventory, not semantic completeness. An incorrect represented/non_requirement classification can omit unfamiliar constraints. No live accuracy or speed improvement is established.",
		"other_payload": "Original request; source-owned clauses, quantities, mentions, controls, residuals, required states and local coverage references; standalone coverage instructions and request-specific schema. Direct input JSON, attempt 1, no retries. No gold answers, catalog, source code, geometry, environment values or credentials in the body. A separately approved payload/budget, isolated version-3 ledger and fresh requirement-coverage-v11 journal are required; this export grants no authorization.",
	}, nil
}
