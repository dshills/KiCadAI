package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPromptFileBound(t *testing.T) {
	for _, size := range []int{0, 1, 2000, 2001, 10000} {
		path := filepath.Join(t.TempDir(), "prompt.txt")
		if err := os.WriteFile(path, []byte(strings.Repeat("x", size)), 0600); err != nil {
			t.Fatal(err)
		}
		b, err := readPrompt(path)
		valid := size > 0 && size <= 2000
		if (err == nil) != valid || (valid && len(b) != size) {
			t.Fatalf("size %d: %d bytes, %v", size, len(b), err)
		}
	}
}

func TestNativeChildrenHaveNoProviderCredentials(t *testing.T) {
	keys := []string{"OPENAI_API_KEY", "ANTHROPIC_API_KEY", "GEMINI_API_KEY", "GOOGLE_API_KEY"}
	for _, key := range keys {
		t.Setenv(key, "test-only-dummy")
	}
	if err := clearProviderCredentials(); err != nil {
		t.Fatal(err)
	}
	for _, key := range keys {
		if _, exists := os.LookupEnv(key); exists {
			t.Fatalf("%s would be inherited", key)
		}
	}
}
