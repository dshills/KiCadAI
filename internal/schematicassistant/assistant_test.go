package schematicassistant

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAssistantMathAndBounds(t *testing.T) {
	divider, _ := Example("divider")
	m, err := Analyze(divider.Spec)
	if err != nil || m[0].Value != 2.5 {
		t.Fatalf("%+v %v", m, err)
	}
	divider.Spec.Load = Number(10000)
	m, err = Analyze(divider.Spec)
	if err != nil || math.Abs(m[0].Value-5.0/3) > 1e-12 {
		t.Fatalf("loaded divider: %+v %v", m, err)
	}
	filter, _ := Example("rc-filter")
	m, err = Analyze(filter.Spec)
	if err != nil || math.Abs(m[1].Value-159.15494309189535) > 1e-9 {
		t.Fatalf("%+v %v", m, err)
	}
	for _, input := range []float64{math.NaN(), math.Inf(1), 0, 25} {
		s := filter.Spec
		s.InputV = Number(input)
		if _, err := Confirm(s); err == nil {
			t.Fatalf("accepted invalid input %v", input)
		}
	}
	filter.Spec.InputV = Number(24)
	filter.Spec.R1 = Number(1000)
	if _, err = Confirm(filter.Spec); err == nil {
		t.Fatal("power budget not enforced")
	}
	for _, text := range []string{"NaN", "Inf", "1e10000", "1V", ""} {
		if _, err := ParseNumber(text); err == nil {
			t.Fatalf("accepted %q", text)
		}
	}
	for text, want := range map[string]float64{"1M": 1e6, "1m": 1e-3, "100n": 1e-7, "1e3": 1000} {
		got, err := ParseNumber(text)
		if err != nil || got != want {
			t.Fatalf("%s: %v %v", text, got, err)
		}
	}
}

func TestAssistantPendingInputCanBeCancelled(t *testing.T) {
	reader, writer := io.Pipe()
	defer func() { _ = reader.Close(); _ = writer.Close() }()
	ctx, cancel := context.WithCancel(context.Background())
	completed := make(chan error, 1)
	go func() { _, err := ReadLine(ctx, bufio.NewScanner(reader)); completed <- err }()
	cancel()
	select {
	case err := <-completed:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancel did not unblock prompt")
	}
}
func TestAssistantEnvironmentNeverFallsBackToCredentialInheritance(t *testing.T) {
	for _, source := range [][]string{nil, {"OPENAI_API_KEY=test-only", "ANTHROPIC_API_KEY=test-only", "SOME_TOKEN=test-only"}} {
		filtered := filteredEnvironment(source)
		if filtered == nil || len(filtered) != 0 {
			t.Fatalf("unsafe filtered environment: %#v", filtered)
		}
	}
	filtered := filteredEnvironment([]string{"PATH=/test/path", "HOME=/test/home", "API_KEY=test-only"})
	if len(filtered) != 2 {
		t.Fatalf("ordinary environment removed: %#v", filtered)
	}
}
func TestAssistantWizardResumesAndRequiresFreshConfirmation(t *testing.T) {
	var saved Session
	save := func(s Session) error { saved = s; return nil }
	var prompts bytes.Buffer
	initial := Session{Schema: Schema}
	partial, err := Wizard(context.Background(), strings.NewReader("my_filter\nrc-filter\n5\n"), &prompts, initial, save)
	if !errors.Is(err, ErrUnconfirmed) || saved.Spec.InputV == nil || partial.ConfirmationSHA256 != "" {
		t.Fatalf("%+v %v", saved, err)
	}
	complete, err := Wizard(context.Background(), strings.NewReader("10k\n0\n100n\nyes\n"), &prompts, saved, save)
	if err != nil || complete.ValidateConfirmation() != nil {
		t.Fatalf("%+v %v", complete, err)
	}
	_, err = Wizard(context.Background(), strings.NewReader("cancel\n"), &prompts, complete, save)
	if !errors.Is(err, ErrUnconfirmed) || saved.ConfirmationSHA256 != "" {
		t.Fatalf("cancel retained approval: %+v %v", saved, err)
	}
}
func TestAssistantGenerationPortableAndDeterministic(t *testing.T) {
	for _, family := range []string{"divider", "rc-filter"} {
		t.Run(family, func(t *testing.T) {
			session, _ := Example(family)
			root := t.TempDir()
			for _, name := range []string{"first", "second"} {
				out := filepath.Join(root, name)
				report, err := Generate(context.Background(), session, out, Options{})
				if err != nil {
					t.Fatal(err)
				}
				if report.Status != "candidate" {
					t.Fatalf("unexpected status %s", report.Status)
				}
				if err = Verify(out); err != nil {
					t.Fatal(err)
				}
				html, err := os.ReadFile(filepath.Join(out, "report.html"))
				if err != nil {
					t.Fatal(err)
				}
				for _, text := range []string{"not_checked", "Calculated behavior", "not hardware approval", "Ideal zero-impedance", "manifest.json", "Content-Security-Policy"} {
					if !strings.Contains(string(html), text) {
						t.Errorf("report lacks %s", text)
					}
				}
				if _, err = os.Stat(filepath.Join(out, session.Spec.Name+".kicad_pcb")); !os.IsNotExist(err) {
					t.Fatal("unexpected PCB")
				}
			}
			for _, extension := range []string{".kicad_sch", ".kicad_pro"} {
				a, _ := os.ReadFile(filepath.Join(root, "first", session.Spec.Name+extension))
				b, _ := os.ReadFile(filepath.Join(root, "second", session.Spec.Name+extension))
				if !bytes.Equal(a, b) {
					t.Fatalf("native output not deterministic: %s", extension)
				}
			}
			if _, err := Generate(context.Background(), session, filepath.Join(root, "first"), Options{}); err == nil {
				t.Fatal("overwrote output")
			}
			session.Spec.InputV = Number(7)
			target := filepath.Join(root, "tampered")
			if _, err := Generate(context.Background(), session, target, Options{}); err == nil {
				t.Fatal("accepted changed confirmed spec")
			}
			if _, err := os.Stat(target); !os.IsNotExist(err) {
				t.Fatal("wrote output for invalid spec")
			}
			if err := os.WriteFile(filepath.Join(root, "first", "report.html"), []byte("tampered"), 0644); err != nil {
				t.Fatal(err)
			}
			if Verify(filepath.Join(root, "first")) == nil {
				t.Fatal("tampered report accepted")
			}
		})
	}
}
func TestAssistantStrictInputAndNoWritesOnFailure(t *testing.T) {
	session, _ := Example("divider")
	raw, _ := json.Marshal(session)
	for _, input := range []string{string(raw) + "{}", "{\"schema\":\"wrong\"}", strings.Repeat(" ", MaxInputBytes+1), strings.Replace(string(raw), "\"spec\":", "\"unknown\":0,\"spec\":", 1)} {
		if _, err := Decode(strings.NewReader(input)); err == nil {
			t.Fatal("accepted invalid session")
		}
	}
	target := filepath.Join(t.TempDir(), "missing-native")
	if _, err := Generate(context.Background(), session, target, Options{KiCadCLI: "/definitely-missing-kicad", RequireERC: true}); err == nil {
		t.Fatal("missing required native checker accepted")
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatal("failed generation published output")
	}
}
func TestAssistantNativeKiCad(t *testing.T) {
	if os.Getenv("KICADAI_RUN_KICAD_CLI") != "1" {
		t.Skip("explicit native KiCad opt-in")
	}
	for _, family := range []string{"divider", "rc-filter"} {
		for _, load := range []float64{0, 10000} {
			session, _ := Example(family)
			session.Spec.Load = Number(load)
			session, err := Confirm(session.Spec)
			if err != nil {
				t.Fatal(err)
			}
			out := filepath.Join(t.TempDir(), family)
			report, err := Generate(context.Background(), session, out, Options{RequireERC: true})
			if err != nil {
				t.Fatal(err)
			}
			if report.Status != "erc_checked_candidate" || report.Diagram != session.Spec.Name+".svg" {
				t.Fatalf("native checks/preview incomplete: %+v", report)
			}
			if err = Verify(out); err != nil {
				t.Fatal(err)
			}
		}
	}
}
