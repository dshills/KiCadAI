package boardfamily

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"kicadai/internal/aiprovider"
)

const ConnectionEvidenceSchemaName = "board_family_connection_requirements_v5"
const ConnectionEvidenceRequestRevision = "connection-request-06"
const ConnectionEvidenceMaxRequestBytes = 65536

// ConnectionEvidenceLanguageContext changes only the experimental extraction
// task. Shared instructions still preserve every source quantity, exclusion,
// unknown requirement and temporal condition; admission alone knows feasibility.
func ConnectionEvidenceLanguageContext() string {
	return strings.Replace(OwnedEvidenceLanguageContext(), OwnedEvidenceRequestRevision, ConnectionEvidenceRequestRevision, 1) + `
# Connection requirements
Use kind connection for communication mode, with value wired or wireless. Do not use a feature fact for radio. A requested wired or cable-connected data link is connection/wired/required, never connection/wireless/required. A required wireless, radio, Wi-Fi or Bluetooth link is connection/wireless/required. Do not invent either mode when none is specified.
Preserve the actual state: "wireless is unnecessary" is wireless/not_required; "do not enable radio" or "non-radio operation" is wireless/forbidden. Neither means wireless/required. A positive wired request alone does not forbid every additional mode. If wired and wireless are both required, preserve both. If a mode is forbidden initially but required later, preserve both temporal facts with the relevant source clauses. An unresolved wired/wireless choice remains uncertain.
Connection facts describe only communication mode. Keep any required connector, Ethernet/USB/protocol, range, power source, firmware or GPIO loading as separate faithful facts with their quantities; a wired fact does not satisfy or erase those requirements. Connection evidence uses cN clauses only and cannot consume a qN occurrence.
Return only the original-source-bound facts JSON. Before returning, distinguish words naming a connection from their negation or exclusion. Do not invent wireless from the word wired or discard a constraint to fit a supported configuration.
`
}

func prepareConnectionGenerateRequest(prompt string) (aiprovider.GenerateRequest, ReferencedRequest, error) {
	source, err := PrepareOwnedEvidenceRequest(prompt)
	if err != nil {
		return aiprovider.GenerateRequest{}, source.ReferencedRequest, err
	}
	encoded, err := json.Marshal(source.ReferencedRequest)
	if err != nil {
		return aiprovider.GenerateRequest{}, source.ReferencedRequest, err
	}
	schema, err := ConnectionEvidenceSchema(prompt)
	if err != nil {
		return aiprovider.GenerateRequest{}, source.ReferencedRequest, err
	}
	return aiprovider.GenerateRequest{Prompt: string(encoded), CapabilityContext: ConnectionEvidenceLanguageContext(),
		OutputSchemaName: ConnectionEvidenceSchemaName, OutputSchema: schema,
		SchemaVersion: aiprovider.EnvelopeSchemaV1, Attempt: 1, MaxOutputTokens: 1600}, source.ReferencedRequest, nil
}

// ConnectionEvidenceContract is key-free export, not spending authority.
func ConnectionEvidenceContract(prompt string) (map[string]any, error) {
	request, source, err := prepareConnectionGenerateRequest(prompt)
	if err != nil {
		return nil, err
	}
	return map[string]any{"admission_version": ConnectionEvidenceVersion, "request_revision": ConnectionEvidenceRequestRevision,
		"destination": "https://api.openai.com/v1/responses", "model": SelectionModel, "experimental": true,
		"capability_context": request.CapabilityContext, "source": source, "schema_name": request.OutputSchemaName, "schema": request.OutputSchema,
		"max_output_tokens": 1600, "max_request_bytes": ConnectionEvidenceMaxRequestBytes,
		"schema_scope":  "original request, cN source clauses, qN quantities with atomic field/owner bindings; separate connection facts cannot consume quantities",
		"other_payload": "Full original request, application-owned clauses and literal quantity table, extraction instructions and JSON schema. attempt=1, no diagnostics, no retries. No source files, geometry, catalog, gold answers, secrets or environment values enter model input. Credentials are used only for HTTPS Authorization. Separate fresh authority and immutable connection-v5 evidence are required; this export grants neither."}, nil
}

func InterpretConnectionWithJournal(ctx context.Context, prompt, ledgerPath string, policy LedgerPolicy, base http.RoundTripper, journalPath string) (ReferencedSelection, error) {
	return interpretProtocolJournal(ctx, prompt, ledgerPath, policy, base, journalPath, connectionProtocol)
}

func InspectConnectionJournal(root string) (ReferencedJournalAudit, error) {
	return inspectProtocolJournal(root, connectionProtocol)
}
