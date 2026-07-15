package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"

	kicaddesign "kicadai/internal/kicadfiles/design"
	"kicadai/internal/reports"
)

const aiFailedAttemptRetention = 5

type aiOutputAttempt struct {
	target       string
	attemptsRoot string
	attemptRoot  string
	projectRoot  string
	overwrite    bool
}

func runAIDesignCreate(ctx context.Context, opts cliOptions, stdout io.Writer) error {
	attempt, issue := beginAIOutputAttempt(opts.output, opts.overwrite)
	if issue != nil {
		return writeDesignFailure(stdout, *issue)
	}
	stagedOpts := opts
	stagedOpts.output = attempt.projectRoot
	// Provider capture creates .kicadai before project writing; replacement is
	// safe inside the isolated attempt and the capture is restored afterward.
	stagedOpts.overwrite = true
	resultPath := filepath.Join(attempt.attemptRoot, ".command-result.tmp")
	resultFile, err := os.OpenFile(resultPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return writeDesignFailure(stdout, *aiOutputAttemptIssue(err))
	}
	runErr := runAIDesignCreateAttempt(ctx, stagedOpts, resultFile)
	closeErr := resultFile.Close()
	if runErr != nil {
		failurePath, preserveErr := attempt.preserveFailure(resultPath)
		writeErr := copyFileToWriter(failurePath, stdout)
		return errors.Join(runErr, closeErr, preserveErr, writeErr)
	}
	if closeErr != nil {
		failurePath, preserveErr := attempt.preserveFailure(resultPath)
		writeErr := copyFileToWriter(failurePath, stdout)
		return errors.Join(closeErr, preserveErr, writeErr)
	}
	if _, err := kicaddesign.CommitPreparedDirectory(attempt.target, attempt.projectRoot, attempt.overwrite); err != nil {
		_, preserveErr := attempt.preserveFailure(resultPath)
		return writeDesignFailure(stdout, reports.Issue{
			Code: reports.CodeValidationFailed, Severity: reports.SeverityError,
			Path: "output", Message: fmt.Sprintf("commit prepared AI project: %v", errors.Join(err, preserveErr)),
		})
	}
	resultFile, err = os.Open(resultPath)
	if err != nil {
		return err
	}
	rewriter := newByteReplacingWriter(stdout, []byte(attempt.projectRoot), []byte(attempt.target))
	_, copyErr := io.Copy(rewriter, resultFile)
	closeErr = resultFile.Close()
	flushErr := rewriter.Close()
	attempt.removeSuccessfulAttempt()
	return errors.Join(copyErr, closeErr, flushErr)
}

func beginAIOutputAttempt(output string, overwrite bool) (*aiOutputAttempt, *reports.Issue) {
	target := filepath.Clean(output)
	info, err := os.Stat(target)
	if err == nil {
		if !info.IsDir() {
			return nil, aiOutputAttemptIssue(fmt.Errorf("target exists and is not a directory: %s", target))
		}
		if !overwrite {
			return nil, aiOutputAttemptIssue(fmt.Errorf("target exists: %s", target))
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, aiOutputAttemptIssue(err)
	}
	parent := filepath.Dir(target)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return nil, aiOutputAttemptIssue(err)
	}
	attemptsRoot := filepath.Join(parent, "."+filepath.Base(target)+".kicadai-attempts")
	if err := os.MkdirAll(attemptsRoot, 0o700); err != nil {
		return nil, aiOutputAttemptIssue(err)
	}
	prefix := time.Now().UTC().Format("20060102T150405.000000000Z") + "-"
	attemptRoot, err := os.MkdirTemp(attemptsRoot, prefix)
	if err != nil {
		return nil, aiOutputAttemptIssue(err)
	}
	return &aiOutputAttempt{
		target: target, attemptsRoot: attemptsRoot, attemptRoot: attemptRoot,
		projectRoot: filepath.Join(attemptRoot, "project"), overwrite: overwrite,
	}, nil
}

func aiOutputAttemptIssue(err error) *reports.Issue {
	return &reports.Issue{
		Code: reports.CodeInvalidArgument, Severity: reports.SeverityError,
		Path: "output", Message: err.Error(),
	}
}

func (attempt *aiOutputAttempt) preserveFailure(resultPath string) (string, error) {
	if attempt == nil {
		return "", nil
	}
	final := filepath.Join(attempt.attemptRoot, "failure-result.json")
	if resultPath != "" {
		if err := os.Rename(resultPath, final); err != nil {
			return "", err
		}
	}
	return final, pruneAIFailedAttempts(attempt.attemptsRoot, aiFailedAttemptRetention, filepath.Base(attempt.attemptRoot))
}

func (attempt *aiOutputAttempt) removeSuccessfulAttempt() {
	if attempt == nil {
		return
	}
	_ = os.RemoveAll(attempt.attemptRoot)
	entries, err := os.ReadDir(attempt.attemptsRoot)
	if err == nil && len(entries) == 0 {
		_ = os.Remove(attempt.attemptsRoot)
	}
}

func pruneAIFailedAttempts(root string, limit int, preserve string) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	for len(names) > limit {
		removeIndex := 0
		if names[removeIndex] == preserve {
			removeIndex++
		}
		if removeIndex >= len(names) {
			break
		}
		if err := os.RemoveAll(filepath.Join(root, names[removeIndex])); err != nil {
			return err
		}
		names = append(names[:removeIndex], names[removeIndex+1:]...)
	}
	return nil
}

func copyFileToWriter(path string, writer io.Writer) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(writer, file)
	return errors.Join(copyErr, file.Close())
}

type byteReplacingWriter struct {
	destination io.Writer
	old         []byte
	new         []byte
	pending     []byte
	failed      error
}

func newByteReplacingWriter(destination io.Writer, old, new []byte) *byteReplacingWriter {
	return &byteReplacingWriter{destination: destination, old: append([]byte(nil), old...), new: append([]byte(nil), new...)}
}

func (writer *byteReplacingWriter) Write(data []byte) (int, error) {
	if writer.failed != nil {
		return 0, writer.failed
	}
	if len(writer.old) == 0 {
		if err := writeAll(writer.destination, data); err != nil {
			writer.failed = err
			return 0, err
		}
		return len(data), nil
	}
	inputLen := len(data)
	writer.pending = append(writer.pending, data...)
	for {
		index := bytes.Index(writer.pending, writer.old)
		if index < 0 {
			break
		}
		if err := writeAll(writer.destination, writer.pending[:index]); err != nil {
			return writer.fail(err)
		}
		if err := writeAll(writer.destination, writer.new); err != nil {
			return writer.fail(err)
		}
		writer.pending = writer.pending[index+len(writer.old):]
	}
	retain := len(writer.old) - 1
	if retain < 0 {
		retain = 0
	}
	if len(writer.pending) > retain {
		flush := len(writer.pending) - retain
		if err := writeAll(writer.destination, writer.pending[:flush]); err != nil {
			return writer.fail(err)
		}
		writer.pending = append(writer.pending[:0], writer.pending[flush:]...)
	}
	return inputLen, nil
}

func (writer *byteReplacingWriter) Close() error {
	if writer.failed != nil {
		return writer.failed
	}
	if len(writer.pending) == 0 {
		return nil
	}
	err := writeAll(writer.destination, writer.pending)
	writer.pending = nil
	return err
}

func (writer *byteReplacingWriter) fail(err error) (int, error) {
	writer.failed = err
	writer.pending = nil
	return 0, err
}

func writeAll(writer io.Writer, data []byte) error {
	written, err := writer.Write(data)
	if err != nil {
		return err
	}
	if written != len(data) {
		return io.ErrShortWrite
	}
	return nil
}
