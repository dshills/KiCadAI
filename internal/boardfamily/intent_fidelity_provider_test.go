package boardfamily

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func checkFidelityProviderRequest(t testing.TB, r *http.Request, prompt string) int {
	t.Helper()
	if r.Method != "POST" || r.URL.String() != "https://api.openai.com/v1/responses" || r.ContentLength <= 0 || r.ContentLength > FidelityEvidenceMaxRequestBytes {
		t.Fatal("coverage endpoint or size differs")
	}
	body, err := io.ReadAll(r.Body)
	if err = errors.Join(err, r.Body.Close()); err != nil {
		t.Fatal(err)
	}
	if int64(len(body)) != r.ContentLength || bytes.Contains(body, []byte("offline-boundary-placeholder")) {
		t.Fatal("length or credential separation differs")
	}
	var payload struct {
		Model, Input, Instructions string
		MaxOutputTokens            int `json:"max_output_tokens"`
		Store, Stream, Background  bool
		Text                       struct {
			Format struct {
				Type, Name string
				Strict     bool
				Schema     map[string]any
			}
		}
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatal(err)
	}
	want, source, err := prepareFidelityGenerateRequest(prompt)
	if err != nil {
		t.Fatal(err)
	}
	if payload.Model != GroundedFullModel || payload.MaxOutputTokens != 1600 || payload.Store || payload.Background || !payload.Stream ||
		!payload.Text.Format.Strict || payload.Text.Format.Type != "json_schema" || payload.Text.Format.Name != FidelityEvidenceSchemaName ||
		payload.Input != want.Prompt || !strings.HasSuffix(payload.Instructions, "\n\n"+want.CapabilityContext) || !sameReferencedJSON(payload.Text.Format.Schema, want.OutputSchema) {
		t.Fatal("coverage wire contract differs")
	}
	var input RequirementCoverageRequest
	if err := decodeReferencedAuditJSON([]byte(payload.Input), &input); err != nil || !sameReferencedJSON(input.Source, source) {
		t.Fatal("source inventory changed", err)
	}
	contract, err := FidelityEvidenceContract(prompt)
	if err != nil || !sameReferencedJSON(contract["input"], input) || !sameReferencedJSON(contract["source"], source) ||
		!sameReferencedJSON(contract["schema"], payload.Text.Format.Schema) || contract["capability_context"] != want.CapabilityContext ||
		contract["live_authorization_granted"] != false || contract["export_dispatches_request"] != false {
		t.Fatal("export differs from actual request", err)
	}
	return len(body)
}

func TestFidelityProviderCorpusWireAndJournal(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "offline-boundary-placeholder")
	t.Setenv("KICADAI_AI_MODEL", "must-not-override")
	b, err := os.ReadFile("../../specs/board-family-v2/typed-evaluation-02/cases-02.json")
	if err != nil {
		t.Fatal(err)
	}
	var corpus struct{ Cases []struct{ ID, Prompt string } }
	if err := json.Unmarshal(b, &corpus); err != nil || len(corpus.Cases) != 14 {
		t.Fatal("frozen corpus differs", err)
	}
	minimum, maximum := FidelityEvidenceMaxRequestBytes, 0
	for _, c := range corpus.Cases {
		t.Run(c.ID, func(t *testing.T) {
			root := t.TempDir()
			journal := filepath.Join(root, "journal")
			calls := 0
			s, err := InterpretFidelityWithJournal(context.Background(), c.Prompt, filepath.Join(root, "ledger.json"), LedgerPolicy{"offline-coverage-wire", 1, 50000}, roundTripperFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				n := checkFidelityProviderRequest(t, r, c.Prompt)
				minimum, maximum = min(minimum, n), max(maximum, n)
				return referencedProviderResponseForModel(t, []byte(`{}`), "refusal", "offline-coverage-"+c.ID, GroundedFullModel), nil
			}), journal)
			if err == nil || calls != 1 || s.Outcome != "provider_refusal" || s.Model != GroundedFullModel || s.AdmissionVersion != FidelityEvidenceVersion {
				t.Fatal("incorrect synthetic provider refusal", err, calls, s.Outcome)
			}
			a, err := InspectFidelityJournal(journal)
			if err != nil || a.Version != "requirement-fidelity-journal-audit-1" || a.Ledger.Version != 3 || a.Ledger.AccountingProfile != FidelityEvidenceAccountingProfile || a.Ledger.Entries[0].EstimatedMicroUSD != 1800 {
				t.Fatal("coverage accounting/audit differs", err)
			}
			for _, p := range []extractionProtocol{indexedProtocol, ownedProtocol, connectionProtocol, directProtocol, groundedProtocol, groundedFullProtocol, sourceEligibleProtocol, sourceAddressedProtocol, semanticBoundaryProtocol, coverageProtocol} {
				if _, err := inspectProtocolJournal(journal, p); err == nil {
					t.Fatal("historical inspector accepted coverage journal")
				}
			}
		})
	}
	t.Logf("14 SYNTHETIC refusal transports: %d-%d request bytes; no live semantic evaluation", minimum, maximum)
}

func TestFidelityProviderPreflightAndUnknownOutcome(t *testing.T) {
	testBoundedProviderPreflight(t, fidelityProtocol, checkFidelityProviderRequest)
}

func TestFidelityLedgerIsolationAndCaps(t *testing.T) {
	testBoundedLedgerIsolationAndCaps(t, fidelityProtocol)
}

func TestFidelityJournalTampering(t *testing.T) {
	testRequestEvidenceJournalTampering(t, fidelityProtocol)
}

func TestFidelityWireVersionIsolation(t *testing.T) {
	prompt := "Use SHT31."
	_, f := coverageFixture(t, prompt)
	offline := addressedJSON(t, f)
	f["version"] = FidelityEvidenceVersion
	wire := addressedJSON(t, f)
	before := bytes.Clone(wire)
	schema, err := FidelityEvidenceSchema(prompt)
	if err != nil || checkGroundedSchema(t, schema, wire) != nil {
		t.Fatal("wire fixture violates schema", err)
	}
	d, err := DecodeFidelityEvidenceIntent(prompt, wire)
	if err != nil || d.Disposition != "supported" || !bytes.Equal(wire, before) {
		t.Fatal("wire admission failed or raw changed", err)
	}
	if _, err := DecodeFidelityEvidenceIntent(prompt, offline); err == nil {
		t.Fatal("offline prototype bytes accepted as provider result")
	}
	if _, err := CompileRequirementCoverage(prompt, wire); err == nil {
		t.Fatal("wire bytes accepted by offline prototype")
	}
	if _, err := FidelityEvidenceContract(""); err == nil {
		t.Fatal("empty request contract accepted")
	}
}
