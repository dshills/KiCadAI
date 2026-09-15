package boardfamily

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func TestOwnedJournalAuditRejectsTampering(t *testing.T) {
	testRequestEvidenceJournalTampering(t, ownedProtocol)
}

func testRequestEvidenceJournalTampering(t *testing.T, protocol extractionProtocol) {
	t.Helper()
	for _, mode := range []string{"unchanged", "invented-decision", "invented-outcome", "coherent-request-rehash", "duplicate-json", "public-file", "extra-file", "missing-file", "symlink"} {
		t.Run(mode, func(t *testing.T) {
			key, check, raw := "offline-owned-placeholder", checkOwnedProviderRequest, ownedRaw(t, ownedFact("sensor", "BMP280", "required", "c0"), ownedFact("profile", "standard", "required", "c0"))
			if protocol == connectionProtocol {
				key, check, raw = "offline-connection-placeholder", checkConnectionProviderRequest, connectionRaw(t, ownedFact("sensor", "BMP280", "required", "c0"), ownedFact("profile", "standard", "required", "c0"))
			}
			t.Setenv("OPENAI_API_KEY", key)
			dir := t.TempDir()
			root := filepath.Join(dir, "journal")
			ledger := filepath.Join(dir, "ledger.json")
			s, err := interpretProtocolJournal(context.Background(), journalTestPrompt, ledger, journalTestPolicy, roundTripperFunc(func(r *http.Request) (*http.Response, error) {
				check(t, r, journalTestPrompt)
				return referencedProviderResponse(t, raw, "complete", "offline-owned-audit"), nil
			}), root, protocol)
			if err != nil {
				t.Fatal(err)
			}
			write := func(name string, value any) {
				t.Helper()
				if err := writeReferencedJournalJSON(filepath.Join(root, name), value); err != nil {
					t.Fatal(err)
				}
			}
			receipt := func(name string, edit func(map[string]any)) {
				t.Helper()
				b, err := os.ReadFile(filepath.Join(root, name))
				var v map[string]any
				if err != nil || json.Unmarshal(b, &v) != nil {
					t.Fatal("cannot read fixture receipt", err)
				}
				edit(v)
				write(name, v)
			}
			switch mode {
			case "invented-decision":
				s.Decision.Configuration.Family = FamilySHT31
				write("selection/selection.json", s)
			case "invented-outcome":
				s.Outcome = "invalid_extraction"
				s.Decision.Configuration = nil
				write("selection/selection.json", s)
				receipt("selection/receipt.json", func(v map[string]any) { v["outcome"] = s.Outcome })
			case "coherent-request-rehash":
				e := s.ProviderEvidence
				e.RequestBody = append(e.RequestBody, ' ')
				e.RequestSHA256 = referencedDigest(e.RequestBody)
				if err := os.WriteFile(filepath.Join(root, "request/body.bin"), e.RequestBody, 0600); err != nil {
					t.Fatal(err)
				}
				write("selection/selection.json", s)
				receipt("request/receipt.json", func(v map[string]any) { v["bytes"] = len(e.RequestBody); v["sha256"] = e.RequestSHA256 })
				receipt("response/receipt.json", func(v map[string]any) { v["request_sha256"] = e.RequestSHA256 })
			case "duplicate-json":
				file := filepath.Join(root, "start.json")
				b, err := os.ReadFile(file)
				if err != nil {
					t.Fatal(err)
				}
				b = bytes.Replace(b, []byte(`"version":`), []byte(`"version":"other","version":`), 1)
				if err := os.WriteFile(file, b, 0600); err != nil {
					t.Fatal(err)
				}
			case "public-file":
				if err := os.Chmod(filepath.Join(root, "response.bin"), 0644); err != nil {
					t.Fatal(err)
				}
			case "extra-file":
				write("extra.json", map[string]bool{"fixture": true})
			case "missing-file":
				if err := os.Remove(filepath.Join(root, "request/receipt.json")); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				file := filepath.Join(root, "response.bin")
				if err := os.Rename(file, filepath.Join(dir, "response.bin")); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(filepath.Join(dir, "response.bin"), file); err != nil {
					t.Fatal(err)
				}
			}
			audit, err := inspectProtocolJournal(root, protocol)
			if (err == nil) != (mode == "unchanged") {
				t.Fatalf("wrong tamper result %s: %v", mode, err)
			}
			if mode == "unchanged" && (audit.Outcome != "decision" || len(audit.FilesSHA256) != 8 || audit.Selection.ResponseID != s.ResponseID) {
				t.Fatal("incomplete verified audit")
			}
			if mode == "unchanged" {
				for _, other := range []extractionProtocol{indexedProtocol, ownedProtocol, connectionProtocol} {
					if other != protocol {
						if _, err := inspectProtocolJournal(root, other); err == nil {
							t.Fatal("another protocol accepted this journal")
						}
					}
				}
			}
		})
	}
}
