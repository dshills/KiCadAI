package reproduction

import (
	"context"
	"encoding/json"
	"kicadai/internal/evidencebundle"
	"kicadai/internal/schematicassistant"
	"os"
	"path/filepath"
	"testing"
)

func TestReproductionCaptureReplayAndStages(t *testing.T) {
	session, err := schematicassistant.Example("divider")
	if err != nil {
		t.Fatal(err)
	}
	valid, err := json.Marshal(session)
	if err != nil {
		t.Fatal(err)
	}
	unconfirmed := session
	unconfirmed.ConfirmationSHA256 = ""
	draft, _ := json.Marshal(unconfirmed)
	for _, test := range []struct {
		name  string
		raw   []byte
		stage string
	}{{"valid", valid, ""}, {"decode", []byte("{bad"), "session_decode"}, {"draft", draft, "confirmation"}} {
		t.Run(test.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "repro")
			record, err := CaptureSchematic(context.Background(), test.raw, root, schematicassistant.Options{})
			if err != nil {
				t.Fatal(err)
			}
			if record.FirstFailingStage != test.stage {
				t.Fatalf("record %+v", record)
			}
			raw, prior, err := Load(root)
			if err != nil || string(raw) != string(test.raw) || prior.InputSHA256 != record.InputSHA256 {
				t.Fatalf("load %v %+v", err, prior)
			}
			replayed, err := CaptureSchematic(context.Background(), raw, filepath.Join(t.TempDir(), "replayed"), schematicassistant.Options{})
			if err != nil || replayed.FirstFailingStage != record.FirstFailingStage {
				t.Fatalf("replay %v %+v", err, replayed)
			}
			if _, err = CaptureSchematic(context.Background(), raw, root, schematicassistant.Options{}); err == nil {
				t.Fatal("overwrite accepted")
			}
			if err = os.WriteFile(filepath.Join(root, "input.json"), []byte("{}"), 0600); err != nil {
				t.Fatal(err)
			}
			if _, _, err = Load(root); err == nil {
				t.Fatal("tampered input accepted")
			}
		})
	}
	root := filepath.Join(t.TempDir(), "native")
	record, err := CaptureSchematic(context.Background(), valid, root, schematicassistant.Options{RequireERC: true, KiCadCLI: "/nonexistent/kicad-cli"})
	if err != nil || record.FirstFailingStage != "native_kicad" || !record.NativeRequested {
		t.Fatalf("native %v %+v", err, record)
	}
	if err = evidencebundle.Verify(root); err != nil {
		t.Fatal(err)
	}
	record.RegistrySHA256 = "historical-model-registry"
	encoded, err := evidencebundle.JSON(record)
	if err != nil {
		t.Fatal(err)
	}
	historical := filepath.Join(t.TempDir(), "historical")
	if err = evidencebundle.Publish(historical, map[string][]byte{"input.json": valid, "reproduction.json": encoded}); err != nil {
		t.Fatal(err)
	}
	if _, _, err = Inspect(historical); err != nil {
		t.Fatalf("historical integrity rejected: %v", err)
	}
	if _, _, err = Load(historical); err == nil {
		t.Fatal("historical model replay accepted")
	}
}
