package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"kicadai/internal/reports"
)

func TestCircuitJSONSuccessAndFailureEachEmitOneDocument(t *testing.T) {
	tests := []struct {
		name   string
		write  func(*bytes.Buffer) error
		wantOK bool
	}{
		{
			name: "success",
			write: func(output *bytes.Buffer) error {
				return writeCircuitPreflightResult(output, circuitPreflightData{ReadyForWrite: true}, nil)
			},
			wantOK: true,
		},
		{
			name: "failure",
			write: func(output *bytes.Buffer) error {
				return writeReportFailure(output, "circuit.preflight", reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Message: "invalid request"})
			},
			wantOK: false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stdout bytes.Buffer
			_ = test.write(&stdout)
			result := decodeSingleResultDocument(t, stdout.Bytes())
			if result.OK != test.wantOK {
				t.Fatalf("OK = %t, want %t", result.OK, test.wantOK)
			}
		})
	}
}

func TestCircuitJSONStderrIsIndependent(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if _, err := io.WriteString(&stderr, "diagnostic log\n"); err != nil {
		t.Fatal(err)
	}
	if err := writeCircuitPreflightResult(&stdout, circuitPreflightData{ReadyForWrite: true}, nil); err != nil {
		t.Fatal(err)
	}
	result := decodeSingleResultDocument(t, stdout.Bytes())
	if !result.OK || stderr.String() != "diagnostic log\n" {
		t.Fatalf("result=%#v stderr=%q", result, stderr.String())
	}
}

func TestCircuitJSONRemainsIsolatedUnderConcurrentLogging(t *testing.T) {
	var stdout bytes.Buffer
	stderr := &lockedBuffer{}
	var wait sync.WaitGroup
	for worker := 0; worker < 32; worker++ {
		wait.Add(1)
		go func(worker int) {
			defer wait.Done()
			for line := 0; line < 100; line++ {
				_, _ = fmt.Fprintf(stderr, "worker=%d line=%d\n", worker, line)
			}
		}(worker)
	}
	if err := writeCircuitPreflightResult(&stdout, circuitPreflightData{ReadyForWrite: true}, nil); err != nil {
		t.Fatal(err)
	}
	wait.Wait()
	if result := decodeSingleResultDocument(t, stdout.Bytes()); !result.OK {
		t.Fatalf("result unexpectedly failed: %#v", result)
	}
	if stderr.Len() == 0 {
		t.Fatal("concurrent log stream is empty")
	}
}

func TestCircuitCancellationAndTimeoutEmitOneJSONDocument(t *testing.T) {
	request := filepath.Join("..", "..", "examples", "circuit-graph", "rc_filter.json")
	tests := []struct {
		name    string
		context func() (context.Context, context.CancelFunc)
	}{
		{
			name: "canceled",
			context: func() (context.Context, context.CancelFunc) {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx, func() {}
			},
		},
		{
			name: "timeout",
			context: func() (context.Context, context.CancelFunc) {
				return context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := test.context()
			defer cancel()
			var stdout bytes.Buffer
			opts := cliOptions{commandArgs: []string{"preflight", "--request", request}}
			_ = runCircuitPreflight(ctx, opts, &stdout)
			result := decodeSingleResultDocument(t, stdout.Bytes())
			if result.OK {
				t.Fatalf("canceled result unexpectedly OK: %#v", result)
			}
		})
	}
}

func TestCircuitCreatePersistsCompleteDiagnosticsWhenOutputExists(t *testing.T) {
	outputDir := t.TempDir()
	issues := make([]reports.Issue, reports.DefaultMaxEmittedIssues+17)
	for index := range issues {
		issues[index] = reports.Issue{
			Code:     reports.CodeUnknownSymbolLibrary,
			Severity: reports.SeverityError,
			Stage:    "library_context",
			Path:     "stock-library",
			Message:  fmt.Sprintf("missing symbol %03d", index),
		}
	}
	var stdout bytes.Buffer
	data := circuitCreateData{Preflight: circuitPreflightData{SchematicIssues: append([]reports.Issue(nil), issues...)}}
	if err := writeCircuitCreateResult(&stdout, data, issues, outputDir); err == nil {
		t.Fatal("writeCircuitCreateResult() error = nil, want blocking result")
	}
	result := decodeSingleResultDocument(t, stdout.Bytes())
	if len(result.Issues) != reports.DefaultMaxEmittedIssues || result.Diagnostics == nil || result.Diagnostics.OmittedCount != 17 {
		t.Fatalf("unexpected bounded result: issues=%d diagnostics=%#v", len(result.Issues), result.Diagnostics)
	}
	decodedData := result.Data.(map[string]any)
	preflight := decodedData["preflight"].(map[string]any)
	if got := len(preflight["schematic_issues"].([]any)); got != reports.DefaultMaxEmittedIssues {
		t.Fatalf("nested schematic issues = %d, want %d", got, reports.DefaultMaxEmittedIssues)
	}
	if len(result.Artifacts) != 1 || result.Artifacts[0].Kind != reports.ArtifactDiagnosticsReport {
		t.Fatalf("unexpected artifacts: %#v", result.Artifacts)
	}
	contents, err := os.ReadFile(filepath.Join(outputDir, ".kicadai", "diagnostics.json"))
	if err != nil {
		t.Fatal(err)
	}
	var complete circuitDiagnosticsArtifact
	if err := json.Unmarshal(contents, &complete); err != nil {
		t.Fatal(err)
	}
	if complete.TotalCount != len(issues) || len(complete.Issues) != len(issues) {
		t.Fatalf("complete diagnostics counts = %d/%d, want %d", complete.TotalCount, len(complete.Issues), len(issues))
	}
}

func TestCircuitCreateDoesNotCreateOutputForBlockedPreflight(t *testing.T) {
	outputDir := filepath.Join(t.TempDir(), "must-not-exist")
	issues := make([]reports.Issue, reports.DefaultMaxEmittedIssues+1)
	for index := range issues {
		issues[index] = reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Message: "blocked"}
	}
	var stdout bytes.Buffer
	_ = writeCircuitCreateResult(&stdout, circuitCreateData{}, issues, outputDir)
	if _, err := os.Stat(outputDir); !os.IsNotExist(err) {
		t.Fatalf("blocked result created output: %v", err)
	}
}

func decodeSingleResultDocument(t *testing.T, contents []byte) reports.Result {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(contents))
	var result reports.Result
	if err := decoder.Decode(&result); err != nil {
		t.Fatalf("first Decode() error = %v\n%s", err, contents)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		t.Fatalf("second Decode() error = %v, want io.EOF", err)
	}
	return result
}

type lockedBuffer struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (buffer *lockedBuffer) Write(contents []byte) (int, error) {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return buffer.buffer.Write(contents)
}

func (buffer *lockedBuffer) Len() int {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return buffer.buffer.Len()
}
