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

func TestReferencedJournalAuditRejectsTampering(t *testing.T) {
	for _, mode := range []string{"unchanged", "invented-decision", "invented-outcome", "coherent-request-rehash", "duplicate-json", "public-file", "extra-file", "missing-file", "symlink"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("OPENAI_API_KEY", "offline-referenced-placeholder")
			dir := t.TempDir()
			root := filepath.Join(dir, "journal")
			ledger := filepath.Join(dir, "ledger.json")
			s, err := InterpretReferencedWithJournal(context.Background(), journalTestPrompt, ledger, journalTestPolicy, roundTripperFunc(func(r *http.Request) (*http.Response, error) {
				checkReferencedProviderRequest(t, r, journalTestPrompt)
				return journalTestResponse(t), nil
			}), root)
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
			audit, err := InspectReferencedJournal(root)
			if (err == nil) != (mode == "unchanged") {
				t.Fatalf("wrong tamper result %s: %v", mode, err)
			}
			if mode == "unchanged" && (audit.Outcome != "decision" || len(audit.FilesSHA256) != 8 || audit.Selection.ResponseID != s.ResponseID) {
				t.Fatal("incomplete verified audit")
			}
		})
	}
}

func TestReferencedJournalAuditChecksPreviousResponseIDs(t *testing.T) {
	for _, duplicate := range []bool{false, true} {
		t.Run(map[bool]string{false: "unique", true: "duplicate"}[duplicate], func(t *testing.T) {
			t.Setenv("OPENAI_API_KEY", "offline-referenced-placeholder")
			dir := t.TempDir()
			ledger := filepath.Join(dir, "ledger.json")
			policy := LedgerPolicy{"offline-two-responses", 2, 100000}
			for i, rootName := range []string{"first", "second"} {
				id := rootName
				if duplicate {
					id = "repeated"
				}
				raw := syntheticReferencedIntent(t, referencedChoice("sensor", "BMP280", "required", 0), referencedChoice("profile", "standard", "required", 0))
				root := filepath.Join(dir, rootName)
				_, err := InterpretReferencedWithJournal(context.Background(), journalTestPrompt, ledger, policy, roundTripperFunc(func(r *http.Request) (*http.Response, error) {
					checkReferencedProviderRequest(t, r, journalTestPrompt)
					return referencedProviderResponse(t, raw, "", id), nil
				}), root)
				if err != nil {
					t.Fatal(err)
				}
				_, auditErr := InspectReferencedJournal(root)
				if (auditErr != nil) != (duplicate && i == 1) {
					t.Fatal("wrong prior response identity audit", auditErr)
				}
			}
		})
	}
}
