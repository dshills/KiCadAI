package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"kicadai/internal/boardfamily"
)

func prepareConfirmedForTest(t *testing.T, c boardfamily.Config) (string, string) {
	t.Helper()
	dir := t.TempDir()
	config := filepath.Join(dir, "config.json")
	if err := writeNewSpecification(config, c); err != nil {
		t.Fatal(err)
	}
	draft := filepath.Join(dir, "draft.json")
	if err := runSpecification([]string{"new", "--config", config, "--output", draft}, commandPipeline{}, io.Discard); err != nil {
		t.Fatal(err)
	}
	var review bytes.Buffer
	if err := runSpecification([]string{"review", "--input", draft}, commandPipeline{}, &review); err != nil {
		t.Fatal(err)
	}
	var r boardfamily.SpecificationReview
	if err := json.Unmarshal(review.Bytes(), &r); err != nil {
		t.Fatal(err)
	}
	confirmed := filepath.Join(dir, "confirmed.json")
	if err := runSpecification([]string{"confirm", "--input", draft, "--accept-sha256", r.SHA256, "--output", confirmed}, commandPipeline{}, io.Discard); err != nil {
		t.Fatal(err)
	}
	return draft, confirmed
}

func TestSpecificationBuildRequiresConfirmation(t *testing.T) {
	draft, confirmed := prepareConfirmedForTest(t, boardfamily.Catalog()[0].DefaultConfiguration)
	calls := 0
	p := commandPipeline{generate: func(boardfamily.Config, string) (boardfamily.Electrical, error) {
		calls++
		return boardfamily.Electrical{}, errors.New("synthetic stop")
	}, validate: func(context.Context, string, string) (boardfamily.Validation, error) {
		t.Fatal("validate called after generation failure")
		return boardfamily.Validation{}, nil
	}}
	out := filepath.Join(t.TempDir(), "board")
	if err := runSpecification([]string{"build", "--input", draft, "--output", out}, p, io.Discard); err == nil || calls != 0 {
		t.Fatal("draft generated a board")
	}
	if err := runSpecification([]string{"build", "--input", confirmed, "--output", out}, p, io.Discard); err == nil || calls != 1 {
		t.Fatal("confirmed input did not reach generator")
	}
}

func TestSpecificationValidationFailureHasNoSuccessManifest(t *testing.T) {
	for _, mode := range []string{"error", "false", "incomplete", "failed-gate"} {
		t.Run(mode, func(t *testing.T) {
			_, confirmed := prepareConfirmedForTest(t, boardfamily.Catalog()[0].DefaultConfiguration)
			out := filepath.Join(t.TempDir(), "board")
			p := commandPipeline{generate: func(_ boardfamily.Config, dir string) (boardfamily.Electrical, error) {
				return boardfamily.Electrical{}, os.Mkdir(dir, 0700)
			}, validate: func(context.Context, string, string) (boardfamily.Validation, error) {
				v := boardfamily.Validation{Passed: true, Checks: make([]boardfamily.CheckResult, 14)}
				for i := range v.Checks {
					v.Checks[i].Passed = true
				}
				switch mode {
				case "error":
					return v, errors.New("native failure")
				case "false":
					v.Passed = false
				case "incomplete":
					v.Checks = v.Checks[:13]
				case "failed-gate":
					v.Checks[0].Passed = false
				}
				return v, nil
			}}
			if err := runSpecification([]string{"build", "--input", confirmed, "--output", out}, p, io.Discard); err == nil {
				t.Fatal("validation failure returned success")
			}
			if _, err := os.Stat(filepath.Join(out, "bundle-manifest.json")); !os.IsNotExist(err) {
				t.Fatal("failed build has success manifest")
			}
		})
	}
}

func TestSpecificationCLIAndOutputGuards(t *testing.T) {
	for _, action := range []string{"new", "draft", "review", "confirm", "build"} {
		if err := runSpecification([]string{action, "--help"}, commandPipeline{}, io.Discard); err != nil {
			t.Fatal(err)
		}
	}
	if err := runForTest(t, "spec", "help"); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{}, {"unknown"}, {"new"}, {"review"}, {"confirm"}, {"build"}, {"draft"}, {"review", "--unexpected"}} {
		if err := runSpecification(args, commandPipeline{}, io.Discard); err == nil {
			t.Fatalf("accepted bad arguments %v", args)
		}
	}
	dir := t.TempDir()
	file := filepath.Join(dir, "keep.json")
	if err := writeNewSpecification(file, map[string]bool{"keep": true}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if err = writeNewSpecification(file, map[string]bool{"replace": true}); err == nil {
		t.Fatal("overwrote file")
	}
	after, err := os.ReadFile(file)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("existing output changed")
	}
	if err = os.Symlink(file, filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	if _, err = specificationBundleFiles(dir); err == nil {
		t.Fatal("bundle accepted symlink")
	}
}

func TestSpecificationDraftIsOptionalAndNeverGenerates(t *testing.T) {
	for _, mode := range []string{"supported", "clarify", "error"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			budget := filepath.Join(dir, "budget.json")
			if err := writeNewSpecification(budget, boardfamily.LedgerPolicy{Goal: "confirmed-spec-test", MaxRequests: 1, MaxMicroUSD: 50000}); err != nil {
				t.Fatal(err)
			}
			out := filepath.Join(dir, "draft.json")
			calls := 0
			p := commandPipeline{interpret: func(_ context.Context, prompt, ledger string, policy boardfamily.LedgerPolicy) (boardfamily.Selection, any, error) {
				calls++
				if prompt != "Original request with no substitutions." || ledger != filepath.Join(dir, "ledger.json") || policy.MaxRequests != 1 {
					t.Fatal("draft inputs changed")
				}
				c := boardfamily.Catalog()[1].DefaultConfiguration
				s := boardfamily.Selection{OriginalRequest: prompt, Decision: boardfamily.Decision{Disposition: mode, Configuration: &c}}
				if mode == "error" {
					return s, nil, errors.New("synthetic provider failure")
				}
				return s, nil, nil
			}, generate: func(boardfamily.Config, string) (boardfamily.Electrical, error) {
				t.Fatal("AI draft attempted generation")
				return boardfamily.Electrical{}, nil
			}}
			args := []string{"draft", "--prompt", "Original request with no substitutions.", "--ledger", filepath.Join(dir, "ledger.json"), "--live-budget", budget, "--output", out}
			err := runSpecification(args, p, io.Discard)
			if (err != nil) != (mode == "error") {
				t.Fatal(err)
			}
			if calls != 1 {
				t.Fatal("draft retried")
			}
			s, err := readSpecification(out)
			if err != nil {
				t.Fatal(err)
			}
			if s.Origin != "ai" || s.OriginalRequest != "Original request with no substitutions." {
				t.Fatal("lost original request")
			}
			if mode == "supported" {
				if s.Configuration == nil {
					t.Fatal("missing proposal")
				}
			} else if len(s.Unresolved) == 0 || s.Configuration != nil {
				t.Fatal("failed draft was confirmable")
			}
			if err = runSpecification(args, p, io.Discard); err == nil || calls != 1 {
				t.Fatal("overwrote draft or spent before output check")
			}
		})
	}
}

func TestConfirmedSpecificationNative(t *testing.T) {
	cli := os.Getenv("KICADAI_OFFLINE_NATIVE_CLI")
	if cli == "" {
		t.Skip("set KICADAI_OFFLINE_NATIVE_CLI for real KiCad validation")
	}
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range boardfamily.Catalog() {
		for _, profile := range f.Profiles {
			t.Run(f.ID+"/"+profile.ID, func(t *testing.T) {
				c := f.DefaultConfiguration
				c.Profile = profile.ID
				c.TotalBusCapacitancePF = profile.MaxBusPF
				_, confirmed := prepareConfirmedForTest(t, c)
				out := filepath.Join(t.TempDir(), "board")
				if err := runSpecification([]string{"build", "--input", confirmed, "--output", out, "--kicad-cli", cli}, defaultCommandPipeline(), io.Discard); err != nil {
					t.Fatal(err)
				}
				b, err := os.ReadFile(filepath.Join(out, "validation.json"))
				if err != nil {
					t.Fatal(err)
				}
				var v boardfamily.Validation
				if err = json.Unmarshal(b, &v); err != nil {
					t.Fatal(err)
				}
				family := "bmp280"
				if f.ID == boardfamily.FamilySHT31 {
					family = "sht31"
				}
				b, err = os.ReadFile(filepath.Join(root, "examples/board-family-v2", family+"-"+profile.ID, "validation.json"))
				if err != nil {
					t.Fatal(err)
				}
				var reviewed boardfamily.Validation
				if err = json.Unmarshal(b, &reviewed); err != nil {
					t.Fatal(err)
				}
				if !v.Passed || len(v.Checks) != 14 || !reflect.DeepEqual(v.NativeSHA256, reviewed.NativeSHA256) {
					t.Fatal("native qualification changed")
				}
				b, err = os.ReadFile(filepath.Join(out, "bundle-manifest.json"))
				if err != nil {
					t.Fatal(err)
				}
				var manifest struct {
					Files map[string]string `json:"files_sha256"`
				}
				if err = json.Unmarshal(b, &manifest); err != nil {
					t.Fatal(err)
				}
				actual, err := specificationBundleFiles(out)
				if err != nil {
					t.Fatal(err)
				}
				delete(actual, "bundle-manifest.json")
				if !reflect.DeepEqual(actual, manifest.Files) {
					t.Fatal("bundle manifest mismatch")
				}
				for _, file := range []string{"board.kicad_pro", "board.kicad_sch", "board.kicad_pcb", "bom.json", "bom.csv", "validation.json", "manufacturing/manifest.json", "confirmed-specification.json"} {
					if manifest.Files[file] == "" {
						t.Fatalf("missing %s", file)
					}
				}
				if err = runSpecification([]string{"build", "--input", confirmed, "--output", out, "--kicad-cli", cli}, defaultCommandPipeline(), io.Discard); err == nil {
					t.Fatal("overwrote existing output")
				}
			})
		}
	}
}
