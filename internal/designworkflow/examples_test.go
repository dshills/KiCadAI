package designworkflow

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDesignExamplesParseAndValidate(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("..", "..", "examples", "design", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Fatal("no design examples found")
	}
	for _, path := range paths {
		t.Run(filepath.Base(path), func(t *testing.T) {
			file, err := os.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()
			request, issues := DecodeRequestStrict(file)
			if len(issues) != 0 {
				t.Fatalf("decode issues: %s", issues[0].Message)
			}
			if validationIssues := ValidateRequest(request); len(validationIssues) != 0 {
				t.Fatalf("validation issues: %s", validationIssues[0].Message)
			}
		})
	}
}
