package capabilitybaselinepublicationv9

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"kicadai/internal/atomicdir"
	"kicadai/internal/capabilitybaselinev9"
)

func Publish(request Request) (Result, error) {
	destination, err := publicationDestination(request.RepositoryRoot, request.DestinationRoot)
	if err != nil {
		return Result{}, err
	}
	if err := capabilitybaselinev9.Validate(request.Report); err != nil {
		return Result{}, fmt.Errorf("validate V9 baseline report: %w", err)
	}
	if err := validateBinding(request.Binding); err != nil {
		return Result{}, fmt.Errorf("invalid V9 baseline publication binding: %w", err)
	}
	if request.Binding.CorpusManifestSHA256 != request.Report.CorpusManifestSHA256 {
		return Result{}, fmt.Errorf("V9 baseline corpus manifest binding does not match the report")
	}
	if request.Binding.EnvironmentSHA256 != request.Report.EnvironmentSHA256 {
		return Result{}, fmt.Errorf("V9 baseline environment binding does not match the report")
	}
	if request.Binding.EvaluatorManifestSHA256 != request.Report.EvaluatorManifestSHA256 {
		return Result{}, fmt.Errorf("V9 baseline evaluator manifest binding does not match the report")
	}

	reportBytes, err := capabilitybaselinev9.MarshalJSONStable(request.Report)
	if err != nil {
		return Result{}, err
	}
	reportBytes = withSingleTrailingLF(reportBytes)
	files := map[string][]byte{ReportFile: reportBytes}
	manifest := Manifest{Schema: ManifestSchema, Version: Version, Binding: request.Binding, ReportSHA256: hashBytes(reportBytes),
		CaseCount: len(request.Report.Cases), OutcomeCounts: slices.Clone(request.Report.OutcomeCounts)}
	for _, record := range request.Report.Cases {
		path := CaseDirectory + "/" + record.Case.ID + ".json"
		data, marshalErr := canonicalJSON(record)
		if marshalErr != nil {
			return Result{}, marshalErr
		}
		files[path] = data
		manifest.Cases = append(manifest.Cases, CaseReference{ID: record.Case.ID, Path: path, SHA256: hashBytes(data)})
	}
	manifest.Hash, err = manifestHash(manifest)
	if err != nil {
		return Result{}, err
	}
	files[ManifestFile], err = canonicalJSON(manifest)
	if err != nil {
		return Result{}, err
	}
	files[AuditFile] = auditBytes(manifest, hashBytes(files[ManifestFile]))
	files[ChecksumFile] = checksumBytes(files)

	if err := atomicdir.Publish(destination, func(stage string) error {
		if err := os.Mkdir(filepath.Join(stage, CaseDirectory), 0o755); err != nil {
			return err
		}
		for path, data := range files {
			if err := os.WriteFile(filepath.Join(stage, filepath.FromSlash(path)), data, 0o644); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return Result{}, fmt.Errorf("publish V9 discovery baseline: %w", err)
	}
	verified, err := Verify(destination)
	if err != nil {
		return Result{}, fmt.Errorf("verify published V9 discovery baseline: %w", err)
	}
	if verified.Manifest.Hash != manifest.Hash {
		return Result{}, fmt.Errorf("published V9 discovery baseline commitment drifted")
	}
	return Result{Manifest: manifest, ManifestSHA256: hashBytes(files[ManifestFile])}, nil
}

func publicationDestination(repositoryRoot, destinationRoot string) (string, error) {
	if strings.TrimSpace(repositoryRoot) == "" || strings.TrimSpace(destinationRoot) == "" {
		return "", fmt.Errorf("repository and destination roots are required")
	}
	repository, err := filepath.Abs(filepath.Clean(repositoryRoot))
	if err != nil {
		return "", err
	}
	repository, err = filepath.EvalSymlinks(repository)
	if err != nil {
		return "", fmt.Errorf("resolve repository root: %w", err)
	}
	info, err := os.Stat(repository)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("repository root is not a directory")
	}
	destination, err := filepath.Abs(filepath.Clean(destinationRoot))
	if err != nil {
		return "", err
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(destination))
	if err != nil {
		return "", fmt.Errorf("resolve publication parent: %w", err)
	}
	destination = filepath.Join(parent, filepath.Base(destination))
	relative, err := filepath.Rel(repository, destination)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("publication destination must be a new child of the repository root")
	}
	return destination, nil
}

func auditBytes(manifest Manifest, manifestFileSHA256 string) []byte {
	var counts []string
	for _, count := range manifest.OutcomeCounts {
		counts = append(counts, fmt.Sprintf("- %s: %d", count.Outcome, count.Count))
	}
	return []byte(fmt.Sprintf("# V9 Generation-Zero Public Baseline Audit\n\nThe public discovery baseline contains exactly %d validated cases. Every case was evaluated twice under one frozen environment; every pass, if any, carries two independent clean-root installed-KiCad promotions. No held-out source or key is accepted by this publisher.\n\n- corpus manifest: `%s`\n- evaluator manifest: `%s`\n- environment: `%s`\n- report: `%s`\n- semantic manifest commitment: `%s`\n- manifest file SHA-256: `%s`\n\n## Outcomes\n\n%s\n", manifest.CaseCount, manifest.Binding.CorpusManifestSHA256, manifest.Binding.EvaluatorManifestSHA256, manifest.Binding.EnvironmentSHA256, manifest.ReportSHA256, manifest.Hash, manifestFileSHA256, strings.Join(counts, "\n")))
}
