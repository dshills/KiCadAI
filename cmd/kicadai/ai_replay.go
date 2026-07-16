package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"kicadai/internal/aiprovider"
	"kicadai/internal/reports"
)

const aiReplayArtifactRelativePath = ".kicadai/ai-provider-replay.json"

type aiProviderCaptureFunc func(aiprovider.GenerateResult) error

type aiReplayCapture struct {
	opts     cliOptions
	profile  string
	artifact reports.Artifact
	command  string
	argv     []string
	data     []byte
	result   aiprovider.GenerateResult
	captured bool
}

func newAIReplayCapture(opts cliOptions, profile string) *aiReplayCapture {
	return &aiReplayCapture{opts: opts, profile: strings.TrimSpace(profile)}
}

func (capture *aiReplayCapture) Capture(result aiprovider.GenerateResult) error {
	artifact, err := aiprovider.NewReplayArtifact(capture.profile, result)
	if err != nil {
		return err
	}
	data, err := aiprovider.MarshalReplayArtifact(artifact)
	if err != nil {
		return err
	}
	path := filepath.Join(capture.opts.output, filepath.FromSlash(aiReplayArtifactRelativePath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return &aiprovider.ProviderError{Code: aiprovider.ErrorConfiguration, Message: "create AI replay artifact directory: " + err.Error()}
	}
	if issue := writeLocalArtifact(path, data, true); issue != nil {
		return &aiprovider.ProviderError{Code: aiprovider.ErrorConfiguration, Message: "write sanitized AI replay artifact: " + issue.Message}
	}
	capture.artifact = reports.Artifact{
		Kind: reports.ArtifactValidationReport, Path: aiReplayArtifactRelativePath,
		Description: "sanitized provider envelope for deterministic offline replay",
	}
	capture.command, capture.argv = aiReplayCommand(capture.opts, capture.profile, path)
	capture.data = append([]byte(nil), data...)
	capture.result = result
	capture.captured = true
	return nil
}

func (capture *aiReplayCapture) Restore() error {
	// Project generation atomically replaces the output directory after capture.
	// Restore the identical sanitized bytes before manifests enumerate artifacts.
	if capture == nil || !capture.captured {
		return nil
	}
	path := filepath.Join(capture.opts.output, filepath.FromSlash(aiReplayArtifactRelativePath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return &aiprovider.ProviderError{Code: aiprovider.ErrorConfiguration, Message: "restore AI replay artifact directory: " + err.Error()}
	}
	if issue := writeLocalArtifact(path, capture.data, true); issue != nil {
		return &aiprovider.ProviderError{Code: aiprovider.ErrorConfiguration, Message: "restore sanitized AI replay artifact: " + issue.Message}
	}
	return nil
}

func (capture *aiReplayCapture) Artifacts() []reports.Artifact {
	if capture == nil || !capture.captured {
		return nil
	}
	return []reports.Artifact{capture.artifact}
}

func (capture *aiReplayCapture) ProviderSummary(result aiprovider.GenerateResult) aiProviderSummary {
	if capture != nil && result.Provider == "" {
		result = capture.result
	}
	summary := aiProviderSummary{Name: result.Provider, Model: result.Model, ResponseID: result.ResponseID, Recorded: result.Recorded}
	if capture != nil && capture.captured {
		summary.ReplayArtifact = capture.artifact.Path
		summary.ReplayCommand = capture.command
		summary.ReplayArgv = append([]string(nil), capture.argv...)
	}
	return summary
}

func runAIProviderCaptures(captures []aiProviderCaptureFunc, result aiprovider.GenerateResult) error {
	for _, capture := range captures {
		if capture == nil {
			continue
		}
		if err := capture(result); err != nil {
			return err
		}
	}
	return nil
}

func writeAIProviderFailure(stdout io.Writer, issue reports.Issue, capture *aiReplayCapture) error {
	data := map[string]any{"provider": capture.ProviderSummary(aiprovider.GenerateResult{})}
	result := reports.ResultWithIssues("design", data, []reports.Issue{issue}, capture.Artifacts())
	if err := writeReportJSON(stdout, result); err != nil {
		return err
	}
	return errors.New(issue.Message)
}

func aiReplayCommand(opts cliOptions, profile, replayPath string) (string, []string) {
	args := []string{"kicadai", "--provider", "recorded", "--provider-record", replayPath, "--ai-profile", profile}
	appendStringFlag := func(flag, value string) {
		if strings.TrimSpace(value) != "" {
			args = append(args, flag, value)
		}
	}
	appendStringFlag("--catalog-dir", opts.catalogDir)
	appendStringFlag("--symbols-root", opts.symbolsRoot)
	appendStringFlag("--footprints-root", opts.footprintsRoot)
	appendStringFlag("--library-cache", opts.libraryCache)
	appendStringFlag("--kicad-cli", opts.kicadCLI)
	appendStringFlag("--promotion-readiness", opts.promotionReadiness)
	if opts.requireERC {
		args = append(args, "--require-erc")
	}
	if opts.requireDRC {
		args = append(args, "--require-drc")
	}
	if opts.requireKiCadRoundTrip {
		args = append(args, "--require-kicad-roundtrip")
	}
	if opts.strictDiffs {
		args = append(args, "--strict-diffs")
	}
	if opts.strictUnrouted {
		args = append(args, "--strict-unrouted")
	}
	args = append(args, "--output", aiReplayOutputPath(opts.output), "--overwrite", "design", "create")
	quoted := make([]string, len(args))
	for index, arg := range args {
		quoted[index] = shellQuoteArgument(arg)
	}
	return strings.Join(quoted, " "), append([]string(nil), args...)
}

func aiReplayOutputPath(output string) string {
	clean := filepath.Clean(strings.TrimSpace(output))
	switch clean {
	case ".":
		return "replay"
	case string(filepath.Separator):
		return filepath.Join(clean, "replay")
	default:
		return clean + "-replay"
	}
}

func shellQuoteArgument(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func applyRecordedReplayMetadata(opts cliOptions) (cliOptions, *reports.Issue) {
	if !strings.EqualFold(strings.TrimSpace(opts.aiProvider), "recorded") || strings.TrimSpace(opts.aiProviderRecord) == "" {
		return opts, nil
	}
	data, err := readBoundedFile(opts.aiProviderRecord, aiprovider.MaxResponseBytes)
	if err != nil {
		code := reports.CodeAIProviderConfiguration
		if os.IsNotExist(err) {
			code = reports.CodeMissingFile
		}
		return opts, &reports.Issue{Code: code, Severity: reports.SeverityError, Path: "provider_record", Message: err.Error()}
	}
	artifact, replay, err := aiprovider.DecodeReplayArtifact(data)
	if err != nil {
		return opts, &reports.Issue{Code: reports.CodeAIOutputInvalid, Severity: reports.SeverityError, Path: "provider_record", Message: err.Error()}
	}
	if !replay {
		return opts, nil
	}
	if explicit := strings.TrimSpace(opts.aiProfile); explicit != "" && explicit != artifact.Profile {
		return opts, &reports.Issue{
			Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "ai_profile",
			Message: fmt.Sprintf("--ai-profile %q conflicts with replay profile %q", explicit, artifact.Profile),
		}
	}
	opts.aiProfile = artifact.Profile
	if !hasAIPromptSource(opts) {
		opts.aiPrompt = "replay captured provider envelope"
	}
	return opts, nil
}
