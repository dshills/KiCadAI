package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"kicadai/internal/capabilityexpansion"
	"kicadai/internal/capabilitygate"
	"kicadai/internal/reports"
)

const (
	candidateBuildRequestSchema = "kicadai.capability-candidate-build-request.v1"
	bundleBuildRequestSchema    = "kicadai.capability-bundle-build-request.v1"
	maxCapabilityRequestBytes   = 16 << 20
	maxCapabilitySourceBytes    = 8 << 20
)

type candidateBuildRequest struct {
	Schema      string                                          `json:"schema"`
	Plan        capabilityexpansion.ExpansionPlan               `json:"plan"`
	Sources     []candidateSourceFile                           `json:"sources"`
	Artifacts   []capabilityexpansion.CapabilityArtifact        `json:"artifacts"`
	Providers   []capabilityexpansion.DeclarativeProviderRecord `json:"providers,omitempty"`
	Assumptions []string                                        `json:"assumptions,omitempty"`
	Risks       []string                                        `json:"risks,omitempty"`
}

type candidateSourceFile struct {
	ID             string                         `json:"id"`
	Kind           capabilityexpansion.SourceKind `json:"kind"`
	Publisher      string                         `json:"publisher"`
	Locator        string                         `json:"locator"`
	License        string                         `json:"license,omitempty"`
	Claims         []string                       `json:"claims"`
	Path           string                         `json:"path"`
	ExpectedSHA256 string                         `json:"expected_sha256"`
}

type bundleBuildRequest struct {
	Schema         string                                `json:"schema"`
	Candidate      capabilityexpansion.CandidateRegistry `json:"candidate"`
	Results        []capabilityexpansion.GateResult      `json:"results"`
	RemainingRisks []string                              `json:"remaining_risks,omitempty"`
}

func runCapabilityExpansion(ctx context.Context, opts cliOptions, stdout io.Writer) error {
	if len(opts.commandArgs) != 2 {
		return capabilityExpansionFailure(stdout, "command", "capability expansion requires exactly one subcommand: plan, candidate, bundle, or promote")
	}
	switch strings.TrimSpace(opts.commandArgs[1]) {
	case "plan":
		return runCapabilityExpansionPlan(opts, stdout)
	case "candidate":
		return runCapabilityExpansionCandidate(ctx, opts, stdout)
	case "bundle":
		return runCapabilityExpansionBundle(opts, stdout)
	case "promote":
		return runCapabilityExpansionPromote(opts, stdout)
	default:
		return capabilityExpansionFailure(stdout, "command", "unknown capability expansion subcommand")
	}
}

func runCapabilityExpansionPlan(opts cliOptions, stdout io.Writer) error {
	if strings.TrimSpace(opts.requestPath) == "" {
		return capabilityExpansionFailure(stdout, "request", "capability expansion plan requires --request")
	}
	assessment, err := decodeCapabilityFile[capabilitygate.Assessment](opts.requestPath)
	if err != nil {
		return capabilityExpansionFailure(stdout, opts.requestPath, err.Error())
	}
	if err := capabilitygate.Validate(assessment); err != nil {
		return capabilityExpansionFailure(stdout, opts.requestPath, err.Error())
	}
	plan, err := capabilityexpansion.Plan(assessment)
	if err != nil {
		return capabilityExpansionFailure(stdout, "capability.expansion.plan", err.Error())
	}
	if err := writeCapabilityArtifact(opts.output, plan); err != nil {
		return capabilityExpansionFailure(stdout, opts.output, err.Error())
	}
	return writeReportJSON(stdout, reports.OKResult("capability.expansion.plan", plan, nil))
}

func runCapabilityExpansionCandidate(ctx context.Context, opts cliOptions, stdout io.Writer) error {
	if strings.TrimSpace(opts.requestPath) == "" {
		return capabilityExpansionFailure(stdout, "request", "capability expansion candidate requires --request")
	}
	request, err := decodeCapabilityFile[candidateBuildRequest](opts.requestPath)
	if err != nil {
		return capabilityExpansionFailure(stdout, opts.requestPath, err.Error())
	}
	if request.Schema != candidateBuildRequestSchema {
		return capabilityExpansionFailure(stdout, "request.schema", "unsupported candidate build request schema")
	}
	if len(request.Sources) > capabilityexpansion.MaxCandidateSources {
		return capabilityExpansionFailure(stdout, "sources", fmt.Sprintf(
			"candidate exceeds %d-source limit", capabilityexpansion.MaxCandidateSources,
		))
	}
	sources := make([]capabilityexpansion.SourceInput, 0, len(request.Sources))
	baseDir := filepath.Dir(opts.requestPath)
	totalSourceBytes := 0
	for _, source := range request.Sources {
		if err := ctx.Err(); err != nil {
			return capabilityExpansionFailure(stdout, "context", err.Error())
		}
		sourcePath := strings.TrimSpace(source.Path)
		if sourcePath == "" {
			return capabilityExpansionFailure(stdout, "sources.path", "candidate source path is required")
		}
		sourcePath, err = resolveCapabilitySourcePath(baseDir, sourcePath)
		if err != nil {
			return capabilityExpansionFailure(stdout, "sources.path", err.Error())
		}
		content, err := readCapabilitySource(sourcePath)
		if err != nil {
			return capabilityExpansionFailure(stdout, sourcePath, err.Error())
		}
		if len(content) > capabilityexpansion.MaxCandidateSourceBytes-totalSourceBytes {
			return capabilityExpansionFailure(stdout, "sources", fmt.Sprintf(
				"candidate sources exceed %d-byte aggregate limit",
				capabilityexpansion.MaxCandidateSourceBytes,
			))
		}
		totalSourceBytes += len(content)
		sources = append(sources, capabilityexpansion.SourceInput{
			ID: source.ID, Kind: source.Kind, Publisher: source.Publisher,
			Locator: source.Locator, License: source.License, Claims: source.Claims,
			Content: content, ExpectedSHA256: source.ExpectedSHA256,
		})
	}
	candidate, err := capabilityexpansion.BuildCandidate(
		request.Plan, sources, request.Artifacts, request.Providers,
		request.Assumptions, request.Risks,
	)
	if err != nil {
		return capabilityExpansionFailure(stdout, "capability.expansion.candidate", err.Error())
	}
	if err := writeCapabilityArtifact(opts.output, candidate); err != nil {
		return capabilityExpansionFailure(stdout, opts.output, err.Error())
	}
	return writeReportJSON(stdout, reports.OKResult("capability.expansion.candidate", candidate, nil))
}

func runCapabilityExpansionBundle(opts cliOptions, stdout io.Writer) error {
	if strings.TrimSpace(opts.requestPath) == "" {
		return capabilityExpansionFailure(stdout, "request", "capability expansion bundle requires --request")
	}
	request, err := decodeCapabilityFile[bundleBuildRequest](opts.requestPath)
	if err != nil {
		return capabilityExpansionFailure(stdout, opts.requestPath, err.Error())
	}
	if request.Schema != bundleBuildRequestSchema {
		return capabilityExpansionFailure(stdout, "request.schema", "unsupported bundle build request schema")
	}
	bundle, err := capabilityexpansion.BuildBundle(request.Candidate, request.Results, request.RemainingRisks)
	if err != nil {
		return capabilityExpansionFailure(stdout, "capability.expansion.bundle", err.Error())
	}
	if err := writeCapabilityArtifact(opts.output, bundle); err != nil {
		return capabilityExpansionFailure(stdout, opts.output, err.Error())
	}
	return writeReportJSON(stdout, reports.OKResult("capability.expansion.bundle", bundle, nil))
}

func runCapabilityExpansionPromote(opts cliOptions, stdout io.Writer) error {
	if strings.TrimSpace(opts.requestPath) == "" || strings.TrimSpace(opts.intentFile) == "" ||
		strings.TrimSpace(opts.output) == "" {
		return capabilityExpansionFailure(stdout, "promotion", "capability expansion promote requires --request bundle, --file approval, and --output registry")
	}
	bundle, err := decodeCapabilityArtifactFile(opts.requestPath, capabilityexpansion.DecodeBundle)
	if err != nil {
		return capabilityExpansionFailure(stdout, opts.requestPath, err.Error())
	}
	approval, err := decodeCapabilityArtifactFile(opts.intentFile, capabilityexpansion.DecodeApproval)
	if err != nil {
		return capabilityExpansionFailure(stdout, opts.intentFile, err.Error())
	}
	registry := capabilityexpansion.EmptySupportedRegistry()
	if strings.TrimSpace(opts.target) != "" {
		registry, err = decodeCapabilityArtifactFile(opts.target, capabilityexpansion.DecodeSupportedRegistry)
		if err != nil {
			return capabilityExpansionFailure(stdout, opts.target, err.Error())
		}
	}
	registry, err = capabilityexpansion.Promote(registry, bundle, approval, opts.execute)
	if err != nil {
		return capabilityExpansionFailure(stdout, "capability.expansion.promote", err.Error())
	}
	if err := capabilityexpansion.WriteArtifact(opts.output, registry); err != nil {
		return capabilityExpansionFailure(stdout, opts.output, err.Error())
	}
	return writeReportJSON(stdout, reports.OKResult("capability.expansion.promote", registry, nil))
}

func decodeCapabilityArtifactFile[T any](path string, decode func(io.Reader) (T, error)) (T, error) {
	var value T
	file, err := os.Open(path)
	if err != nil {
		return value, err
	}
	defer func() { _ = file.Close() }()
	return decode(file)
}

func decodeCapabilityFile[T any](path string) (T, error) {
	var value T
	file, err := os.Open(path)
	if err != nil {
		return value, err
	}
	defer func() { _ = file.Close() }()
	content, err := io.ReadAll(io.LimitReader(file, maxCapabilityRequestBytes+1))
	if err != nil {
		return value, err
	}
	if len(content) > maxCapabilityRequestBytes {
		return value, fmt.Errorf("request exceeds %d-byte limit", maxCapabilityRequestBytes)
	}
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return value, fmt.Errorf("decode strict JSON: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return value, fmt.Errorf("decode strict JSON: trailing content")
	}
	return value, nil
}

func readCapabilitySource(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	content, err := io.ReadAll(io.LimitReader(file, maxCapabilitySourceBytes+1))
	if err != nil {
		return nil, err
	}
	if len(content) > maxCapabilitySourceBytes {
		return nil, fmt.Errorf("source exceeds %d-byte limit", maxCapabilitySourceBytes)
	}
	return content, nil
}

func resolveCapabilitySourcePath(baseDir, sourcePath string) (string, error) {
	basePath, err := filepath.Abs(baseDir)
	if err != nil {
		return "", fmt.Errorf("resolve candidate manifest directory: %w", err)
	}
	basePath, err = filepath.EvalSymlinks(basePath)
	if err != nil {
		return "", fmt.Errorf("resolve candidate manifest directory: %w", err)
	}
	if !filepath.IsAbs(sourcePath) {
		sourcePath = filepath.Join(basePath, sourcePath)
	}
	sourcePath, err = filepath.Abs(sourcePath)
	if err != nil {
		return "", fmt.Errorf("resolve candidate source: %w", err)
	}
	sourcePath, err = filepath.EvalSymlinks(sourcePath)
	if err != nil {
		return "", fmt.Errorf("resolve candidate source: %w", err)
	}
	relative, err := filepath.Rel(basePath, sourcePath)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) ||
		filepath.IsAbs(relative) {
		return "", fmt.Errorf("candidate source must resolve inside the manifest directory")
	}
	return sourcePath, nil
}

func writeCapabilityArtifact(path string, value any) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	return capabilityexpansion.WriteArtifact(path, value)
}

func capabilityExpansionFailure(stdout io.Writer, path, message string) error {
	return writeReportFailure(stdout, "capability.expansion", reports.Issue{
		Code: reports.CodeValidationFailed, Severity: reports.SeverityError,
		Path: path, Message: message,
	})
}
