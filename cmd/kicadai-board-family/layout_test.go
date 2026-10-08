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

func prepareLayoutConfirmation(t *testing.T, c boardfamily.Config, size string) string {
	t.Helper()
	dir := t.TempDir()
	config := filepath.Join(dir, "config.json")
	layout := filepath.Join(dir, "layout.json")
	for name, value := range map[string]any{config: c, layout: boardfamily.LayoutSpec{Version: "board-layout-1", Size: size, Power: "west", UART: "south", GPIO: "east"}} {
		if err := writeNewSpecification(name, value); err != nil {
			t.Fatal(err)
		}
	}
	draft := filepath.Join(dir, "draft.json")
	if err := runSpecification([]string{"new", "--config", config, "--layout", layout, "--output", draft}, commandPipeline{}, io.Discard); err != nil {
		t.Fatal(err)
	}
	s, err := readSpecification(draft)
	if err != nil {
		t.Fatal(err)
	}
	r, err := boardfamily.ReviewSpecification(s)
	if err != nil || r.Layout == nil || !r.Ready {
		t.Fatal(err, r)
	}
	confirmed := filepath.Join(dir, "confirmed.json")
	if err = runSpecification([]string{"confirm", "--input", draft, "--accept-sha256", r.SHA256, "--output", confirmed}, commandPipeline{}, io.Discard); err != nil {
		t.Fatal(err)
	}
	return confirmed
}

func TestLayoutCLIFailureDoesNotPublishBundle(t *testing.T) {
	confirmed := prepareLayoutConfirmation(t, boardfamily.Catalog()[0].DefaultConfiguration, "120x70")
	for _, mode := range []string{"routing", "validation"} {
		t.Run(mode, func(t *testing.T) {
			out := filepath.Join(t.TempDir(), "board")
			called := false
			p := commandPipeline{generate: func(boardfamily.Config, string) (boardfamily.Electrical, error) {
				t.Fatal("fell back to fixed reference")
				return boardfamily.Electrical{}, nil
			},
				generateLayout: func(c boardfamily.Config, s *boardfamily.LayoutSpec, dir string) (boardfamily.Electrical, error) {
					called = true
					if mode == "routing" {
						return boardfamily.Electrical{}, errors.New("injected routing limit")
					}
					return boardfamily.GenerateWithLayout(c, s, dir)
				},
				validate: func(context.Context, string, string) (boardfamily.Validation, error) {
					return boardfamily.Validation{}, errors.New("injected physical check failure")
				}}
			if err := runSpecification([]string{"build", "--input", confirmed, "--output", out}, p, io.Discard); err == nil || !called {
				t.Fatal("failure accepted")
			}
			if _, err := os.Stat(filepath.Join(out, "bundle-manifest.json")); !os.IsNotExist(err) {
				t.Fatal("failure published successful bundle")
			}
		})
	}
	var options bytes.Buffer
	if err := runSpecification([]string{"layouts"}, commandPipeline{}, &options); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(options.Bytes(), []byte("140x80")) {
		t.Fatal("missing layout choices")
	}
}

func TestLayoutBundleNative(t *testing.T) {
	cli := os.Getenv("KICADAI_OFFLINE_NATIVE_CLI")
	if cli == "" {
		t.Skip("requires installed KiCad")
	}
	for i, family := range boardfamily.Catalog() {
		t.Run(family.ID, func(t *testing.T) {
			size := []string{"120x70", "140x80"}[i]
			confirmed := prepareLayoutConfirmation(t, family.DefaultConfiguration, size)
			base := t.TempDir()
			if value := os.Getenv("KICADAI_LAYOUT_BUNDLE_ROOT"); value != "" {
				base = filepath.Join(value, family.ID)
			}
			var previous map[string]string
			for _, name := range []string{"first", "second"} {
				out := filepath.Join(base, name)
				if err := runSpecification([]string{"build", "--input", confirmed, "--output", out, "--kicad-cli", cli}, defaultCommandPipeline(), io.Discard); err != nil {
					t.Fatal(err)
				}
				b, err := os.ReadFile(filepath.Join(out, "bundle-manifest.json"))
				if err != nil {
					t.Fatal(err)
				}
				var manifest struct {
					Passed bool              `json:"passed"`
					Gates  int               `json:"validation_gates"`
					Files  map[string]string `json:"files_sha256"`
				}
				if err = json.Unmarshal(b, &manifest); err != nil {
					t.Fatal(err)
				}
				actual, err := specificationBundleFiles(out)
				delete(actual, "bundle-manifest.json")
				if err != nil || !manifest.Passed || manifest.Gates != 14 || !reflect.DeepEqual(actual, manifest.Files) {
					t.Fatal("bundle inventory mismatch", err)
				}
				native := map[string]string{}
				for _, file := range []string{"board.kicad_pcb", "board.kicad_sch", "board.kicad_pro", "bom.json", "layout.json", "layout-report.json"} {
					native[file] = actual[file]
				}
				if previous != nil && !reflect.DeepEqual(native, previous) {
					t.Fatal("layout bundle native outputs did not reproduce")
				}
				previous = native
			}
		})
	}
}
