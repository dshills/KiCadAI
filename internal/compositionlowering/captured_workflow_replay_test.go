package compositionlowering

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"kicadai/internal/designworkflow"
	"kicadai/internal/libraryresolver"
)

func TestCapturedOpenSetWorkflowRequest(t *testing.T) {
	requestPath := os.Getenv("KICADAI_CAPTURED_OPEN_SET_REQUEST")
	if requestPath == "" {
		t.Skip("set KICADAI_CAPTURED_OPEN_SET_REQUEST to replay a captured resolved workflow request")
	}
	requestData, err := os.ReadFile(requestPath)
	if err != nil {
		t.Fatal(err)
	}
	var request designworkflow.Request
	if err := json.Unmarshal(requestData, &request); err != nil {
		t.Fatalf("decode captured workflow request: %v", err)
	}
	indexData, err := os.ReadFile(filepath.Join(filepath.Dir(requestPath), "library_index.json"))
	if err != nil {
		t.Fatal(err)
	}
	var index libraryresolver.LibraryIndex
	if err := json.Unmarshal(indexData, &index); err != nil {
		t.Fatalf("decode captured library index: %v", err)
	}
	runOpenSetWorkflow(t, request, index, "", t.TempDir())
}
