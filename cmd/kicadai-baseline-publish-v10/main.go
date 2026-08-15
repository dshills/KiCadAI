package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"kicadai/internal/capabilitybaselinepublicationv10"
	"kicadai/internal/capabilitybaselinev10"
)

const maximumInputBytes = 32 << 20
const frozenPublisherManifestSHA256 = "ac59be26ba83f8f8e8e3268ca732a3646c73d761446d0bd5c39bbba96029eab9"

type options struct{ repositoryRoot, reportPath, bindingPath, destination, publisherManifest string }

type summary struct {
	Schema         string                               `json:"schema"`
	Version        int                                  `json:"version"`
	Destination    string                               `json:"destination"`
	ManifestSHA256 string                               `json:"manifest_sha256"`
	ReportSHA256   string                               `json:"report_sha256"`
	CaseCount      int                                  `json:"case_count"`
	OutcomeCounts  []capabilitybaselinev10.OutcomeCount `json:"outcome_counts"`
}

func main() {
	if err := run(context.Background(), os.Args[1:], os.Stdout); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, arguments []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("kicadai-baseline-publish-v10", flag.ContinueOnError)
	var diagnostics bytes.Buffer
	flags.SetOutput(&diagnostics)
	var opts options
	flags.StringVar(&opts.repositoryRoot, "repository-root", ".", "clean committed repository root")
	flags.StringVar(&opts.reportPath, "report", "", "canonical V10 evaluator report")
	flags.StringVar(&opts.bindingPath, "binding", "", "canonical V10 baseline publication binding")
	flags.StringVar(&opts.destination, "destination", "", "fresh repository-child baseline destination")
	flags.StringVar(&opts.publisherManifest, "publisher-manifest", "", "frozen V10 baseline publisher manifest")
	if err := flags.Parse(arguments); err != nil {
		return fmt.Errorf("parse V10 baseline publisher arguments: %s", strings.TrimSpace(diagnostics.String()))
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("parse V10 baseline publisher arguments: unexpected positional arguments")
	}
	if err := normalize(&opts); err != nil {
		return err
	}
	if err := requireCleanRepository(ctx, opts.repositoryRoot); err != nil {
		return err
	}
	manifestBytes, err := readRegularFile(opts.publisherManifest)
	if err != nil || hashBytes(manifestBytes) != frozenPublisherManifestSHA256 {
		return fmt.Errorf("V10 baseline publisher manifest binding is invalid")
	}
	if err := verifyRepositorySourceManifest(opts.repositoryRoot, opts.publisherManifest); err != nil {
		return fmt.Errorf("verify frozen V10 baseline publisher: %w", err)
	}
	reportBytes, err := readRegularFile(opts.reportPath)
	if err != nil {
		return fmt.Errorf("read V10 evaluator report: %w", err)
	}
	var report capabilitybaselinev10.Report
	if err := decodeStrict(reportBytes, &report); err != nil {
		return fmt.Errorf("decode V10 evaluator report: %w", err)
	}
	wantReport, err := capabilitybaselinev10.MarshalJSONStable(report)
	wantReport = append(wantReport, '\n')
	if err != nil || !bytes.Equal(reportBytes, wantReport) {
		return fmt.Errorf("V10 evaluator report is not canonical")
	}
	bindingBytes, err := readRegularFile(opts.bindingPath)
	if err != nil {
		return fmt.Errorf("read V10 baseline binding: %w", err)
	}
	var binding capabilitybaselinepublicationv10.Binding
	if err := decodeCanonical(bindingBytes, &binding); err != nil {
		return fmt.Errorf("decode V10 baseline binding: %w", err)
	}
	result, err := capabilitybaselinepublicationv10.Publish(capabilitybaselinepublicationv10.Request{RepositoryRoot: opts.repositoryRoot, DestinationRoot: opts.destination, Binding: binding, Report: report})
	if err != nil {
		return err
	}
	if err := requireCleanRepositoryAllowing(ctx, opts.repositoryRoot, opts.destination); err != nil {
		return err
	}
	verified, err := capabilitybaselinepublicationv10.Verify(opts.destination)
	if err != nil || verified.ManifestSHA256 != result.ManifestSHA256 {
		return fmt.Errorf("published V10 baseline did not reauthenticate")
	}
	return json.NewEncoder(stdout).Encode(summary{Schema: "kicadai.closed-loop-open-set-baseline-publication-run.v10", Version: 10, Destination: opts.destination, ManifestSHA256: result.ManifestSHA256, ReportSHA256: result.Manifest.ReportSHA256, CaseCount: result.Manifest.CaseCount, OutcomeCounts: result.Manifest.OutcomeCounts})
}

func normalize(opts *options) error {
	root, err := filepath.Abs(opts.repositoryRoot)
	if err != nil {
		return err
	}
	opts.repositoryRoot = root
	if strings.TrimSpace(opts.publisherManifest) == "" {
		opts.publisherManifest = filepath.Join(root, "specs", "closed-loop-open-set-capability-expansion", "V10_BASELINE_PUBLISHER.sha256")
	}
	for _, value := range []*string{&opts.reportPath, &opts.bindingPath, &opts.destination, &opts.publisherManifest} {
		if strings.TrimSpace(*value) == "" {
			return fmt.Errorf("--report, --binding, and --destination are required")
		}
		absolute, err := filepath.Abs(*value)
		if err != nil {
			return err
		}
		*value = absolute
	}
	return nil
}

func hashBytes(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }

func decodeCanonical(data []byte, value any) error {
	if err := decodeStrict(data, value); err != nil {
		return err
	}
	want, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	want = append(want, '\n')
	if !bytes.Equal(data, want) {
		return fmt.Errorf("noncanonical JSON")
	}
	return nil
}

func decodeStrict(data []byte, value any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return fmt.Errorf("trailing JSON")
	}
	return nil
}

func readRegularFile(filename string) ([]byte, error) {
	info, err := os.Lstat(filename)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > maximumInputBytes {
		return nil, fmt.Errorf("input is not a bounded regular file")
	}
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || opened.Size() > maximumInputBytes || !os.SameFile(info, opened) {
		return nil, fmt.Errorf("input changed while opening")
	}
	data, err := io.ReadAll(io.LimitReader(file, maximumInputBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maximumInputBytes {
		return nil, fmt.Errorf("input exceeds size bound")
	}
	return data, nil
}

func verifyRepositorySourceManifest(repositoryRoot, manifestPath string) error {
	root, err := filepath.EvalSymlinks(repositoryRoot)
	if err != nil {
		return fmt.Errorf("resolve repository root: %w", err)
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("resolve repository root: %w", err)
	}
	manifest, err := readRegularFile(manifestPath)
	if err != nil {
		return fmt.Errorf("read checksum manifest: %w", err)
	}
	seen := make(map[string]struct{})
	entries := 0
	scanner := bufio.NewScanner(bytes.NewReader(manifest))
	scanner.Buffer(make([]byte, 64<<10), maximumInputBytes)
	for lineNumber := 1; scanner.Scan(); lineNumber++ {
		line := scanner.Text()
		if len(line) < sha256.Size*2+3 || line[sha256.Size*2:sha256.Size*2+2] != "  " {
			return fmt.Errorf("checksum manifest line %d is invalid", lineNumber)
		}
		digest, relative := line[:sha256.Size*2], line[sha256.Size*2+2:]
		decoded, decodeErr := hex.DecodeString(digest)
		if decodeErr != nil || len(decoded) != sha256.Size || digest != strings.ToLower(digest) {
			return fmt.Errorf("checksum manifest line %d has invalid digest", lineNumber)
		}
		if relative == "" || filepath.IsAbs(relative) || strings.ContainsAny(relative, "\\\x00") || filepath.ToSlash(relative) != relative {
			return fmt.Errorf("checksum manifest line %d has invalid path", lineNumber)
		}
		candidate := filepath.Clean(filepath.Join(filepath.Dir(manifestPath), filepath.FromSlash(relative)))
		candidateInfo, statErr := os.Lstat(candidate)
		if statErr != nil || !candidateInfo.Mode().IsRegular() {
			return fmt.Errorf("checksum manifest line %d does not name a regular file", lineNumber)
		}
		resolved, resolveErr := filepath.EvalSymlinks(candidate)
		if resolveErr != nil {
			return fmt.Errorf("resolve checksum manifest line %d: %w", lineNumber, resolveErr)
		}
		resolved, err = filepath.Abs(resolved)
		if err != nil {
			return fmt.Errorf("resolve checksum manifest line %d: %w", lineNumber, err)
		}
		withinRoot, relErr := filepath.Rel(root, resolved)
		if relErr != nil || withinRoot == ".." || strings.HasPrefix(withinRoot, ".."+string(filepath.Separator)) || filepath.IsAbs(withinRoot) {
			return fmt.Errorf("checksum manifest line %d escapes repository root", lineNumber)
		}
		if _, duplicate := seen[resolved]; duplicate {
			return fmt.Errorf("checksum manifest line %d duplicates a source file", lineNumber)
		}
		seen[resolved] = struct{}{}
		actualDigest, hashErr := hashRegularFile(resolved)
		if hashErr != nil {
			return fmt.Errorf("read checksum manifest line %d: %w", lineNumber, hashErr)
		}
		if actualDigest != digest {
			return fmt.Errorf("checksum manifest line %d does not match", lineNumber)
		}
		entries++
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scan checksum manifest: %w", err)
	}
	if entries == 0 {
		return fmt.Errorf("checksum manifest is empty")
	}
	return nil
}

func hashRegularFile(filename string) (string, error) {
	info, err := os.Lstat(filename)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() || info.Size() > maximumInputBytes {
		return "", fmt.Errorf("input is not a bounded regular file")
	}
	file, err := os.Open(filename)
	if err != nil {
		return "", err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || opened.Size() > maximumInputBytes || !os.SameFile(info, opened) {
		return "", fmt.Errorf("input changed while opening")
	}
	hasher := sha256.New()
	written, err := io.Copy(hasher, io.LimitReader(file, maximumInputBytes+1))
	if err != nil {
		return "", err
	}
	if written > maximumInputBytes {
		return "", fmt.Errorf("input exceeds size bound")
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func requireCleanRepository(ctx context.Context, root string) error {
	return requireCleanRepositoryAllowing(ctx, root, "")
}

func requireCleanRepositoryAllowing(ctx context.Context, root, allowed string) error {
	gitPath, err := exec.LookPath("git")
	if err != nil {
		return fmt.Errorf("V10 baseline publication requires git in PATH: %w", err)
	}
	top, err := exec.CommandContext(ctx, gitPath, "-C", root, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return fmt.Errorf("V10 baseline publication requires repository root")
	}
	rootInfo, rootErr := os.Stat(root)
	topInfo, topErr := os.Stat(strings.TrimSpace(string(top)))
	if rootErr != nil || topErr != nil || !os.SameFile(rootInfo, topInfo) {
		return fmt.Errorf("V10 baseline publication requires repository root")
	}
	status, err := exec.CommandContext(ctx, gitPath, "-C", root, "status", "--porcelain=v1", "-z", "--untracked-files=all").Output()
	if err != nil {
		return err
	}
	records := bytes.Split(bytes.TrimSuffix(status, []byte{0}), []byte{0})
	allowedRelative := ""
	if allowed != "" {
		allowedRelative, err = filepath.Rel(root, allowed)
		if err != nil {
			return err
		}
		allowedRelative = filepath.ToSlash(allowedRelative) + "/"
	}
	for _, record := range records {
		if len(record) == 0 {
			continue
		}
		if len(record) >= 4 && string(record[:2]) == "??" && record[2] == ' ' && allowedRelative != "" && strings.HasPrefix(string(record[3:]), allowedRelative) {
			continue
		}
		return fmt.Errorf("V10 baseline publication requires a clean committed repository")
	}
	return nil
}
