package transactions

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"kicadai/internal/libraryresolver"
)

// This diagnostic only replays non-writing operations and reads recorded
// metadata. It must never invoke Apply, WriteProject, KiCad, or a provider.
func TestNativeLocalWiringFrozenCandidateDiagnostic(t *testing.T) {
	root := os.Getenv("KICADAI_LOCAL_WIRING_DIAGNOSTIC_INPUT")
	if root == "" {
		t.Skip("requires explicit immutable recorded input")
	}
	read := func(name string, value any) {
		t.Helper()
		f, err := os.Open(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		h := sha256.New()
		r := io.TeeReader(f, h)
		if err := json.NewDecoder(r).Decode(value); err != nil {
			t.Fatal(err)
		}
		if _, err := io.Copy(io.Discard, r); err != nil {
			t.Fatal(err)
		}
		t.Logf("immutable input %s sha256=%x", name, h.Sum(nil))
	}
	var tx Transaction
	var index libraryresolver.LibraryIndex
	derived := os.Getenv("KICADAI_LOCAL_WIRING_DIAGNOSTIC_TRANSACTION")
	if derived == "" {
		read("controller_adc_100ma/schematic_transaction.json", &tx)
	} else {
		data, err := os.ReadFile(derived)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(data, &tx); err != nil {
			t.Fatal(err)
		}
		t.Logf("derived drawing-only diagnostic %s sha256=%x", derived, sha256.Sum256(data))
	}
	read("library_index.json", &index)
	opts := ApplyOptions{LibraryIndex: &index}
	builder, err := builderFromTransaction(tx, opts)
	if err != nil {
		t.Fatal(err)
	}
	for _, op := range tx.Operations {
		switch op.Op {
		case OpCreateProject:
			continue
		case OpAddSymbol, OpAssignFootprint, OpAddNoConnect, OpConnect:
		default:
			t.Fatalf("diagnostic rejects non-whitelisted operation %q", op.Op)
		}
		if op.Op == OpAddSymbol {
			// Match designworkflow.explicitSchematicTransaction's resolver
			// hydration on a local operation copy, never the frozen input.
			var payload AddSymbolOperation
			if err := json.Unmarshal(op.Raw, &payload); err != nil {
				t.Fatal(err)
			}
			payload.PreferResolverSymbol = true
			op.Raw, err = json.Marshal(payload)
			if err != nil {
				t.Fatal(err)
			}
		}
		artifacts, err := applyOperation(builder, op, opts)
		if err != nil {
			t.Fatalf("operation %s: %v", op.Op, err)
		}
		if len(artifacts) != 0 {
			t.Fatal("unexpected generated artifact")
		}
	}
	labels, placementErr := builder.NativeAnnotationDiagnostics()
	t.Logf("labels=%d placement_error=%v", len(labels), placementErr)
	for i, label := range labels {
		t.Logf("candidate_order=%d uuid=%s text=%q count=%d origin=%v", i, label.UUID, label.Text, len(label.Candidates), label.Origin)
		if i < 8 {
			encoded, err := json.Marshal(label)
			if err != nil {
				t.Fatal(err)
			}
			t.Log(string(encoded))
		}
	}
	if derived != "" && placementErr != nil {
		t.Error(placementErr)
	}
	again, againErr := builder.NativeAnnotationDiagnostics()
	a, _ := json.Marshal(labels)
	b, _ := json.Marshal(again)
	if string(a) != string(b) || fmt.Sprint(placementErr) != fmt.Sprint(againErr) {
		t.Fatal("diagnostic mutated subsequent candidate arrangement")
	}
}
