package manifest

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"kicadai/internal/reports"
)

const RelativePath = ".kicadai/manifest.json"

type Manifest struct {
	ProjectName      string             `json:"project_name"`
	GeneratorVersion string             `json:"generator_version"`
	Operations       []OperationSummary `json:"operations"`
	Artifacts        []reports.Artifact `json:"artifacts"`
	FileHashes       map[string]string  `json:"file_hashes"`
	Provenance       *ProvenanceRef     `json:"provenance,omitempty"`
}

type ProvenanceRef struct {
	TransactionPath string `json:"transaction_path,omitempty"`
	Schema          string `json:"schema,omitempty"`
	OperationCount  int    `json:"operation_count,omitempty"`
	Hash            string `json:"hash,omitempty"`
}

type OperationSummary struct {
	Index int    `json:"index"`
	Op    string `json:"op"`
}

type Status struct {
	Present bool     `json:"present"`
	Path    string   `json:"path,omitempty"`
	Stale   bool     `json:"stale"`
	Issues  []string `json:"issues,omitempty"`
}

func Write(root string, manifest Manifest) (reports.Artifact, error) {
	if strings.TrimSpace(root) == "" {
		return reports.Artifact{}, fmt.Errorf("manifest root required")
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return reports.Artifact{}, err
	}
	if manifest.FileHashes == nil {
		manifest.FileHashes = map[string]string{}
	}
	if manifest.Provenance != nil {
		if err := hydrateProvenanceRef(absRoot, manifest.Provenance, manifest.FileHashes); err != nil {
			return reports.Artifact{}, err
		}
	}
	for _, artifact := range manifest.Artifacts {
		if strings.TrimSpace(artifact.Path) == "" {
			continue
		}
		artifactPath := artifact.Path
		if filepath.IsAbs(artifactPath) {
			rel, err := filepath.Rel(absRoot, artifactPath)
			if err != nil {
				return reports.Artifact{}, fmt.Errorf("artifact is outside manifest root: %s", artifact.Path)
			}
			artifactPath = rel
		}
		rel, absArtifact, err := cleanRelativePath(absRoot, artifactPath)
		if err != nil {
			return reports.Artifact{}, fmt.Errorf("artifact is outside manifest root: %s", artifact.Path)
		}
		if rel == RelativePath {
			continue
		}
		hash, err := fileHash(absArtifact)
		if err != nil {
			return reports.Artifact{}, err
		}
		manifest.FileHashes[rel] = hash
	}
	path := filepath.Join(root, RelativePath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return reports.Artifact{}, err
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return reports.Artifact{}, err
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		return reports.Artifact{}, err
	}
	return reports.Artifact{Kind: reports.ArtifactValidationReport, Path: filepath.ToSlash(path), Description: "KiCadAI generated-project manifest"}, nil
}

func Read(root string) (Manifest, Status, error) {
	path := filepath.Join(root, RelativePath)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Manifest{}, Status{Present: false}, nil
	}
	if err != nil {
		return Manifest{}, Status{Present: true, Path: filepath.ToSlash(path), Stale: true, Issues: []string{err.Error()}}, err
	}
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return Manifest{}, Status{Present: true, Path: filepath.ToSlash(path), Stale: true, Issues: []string{err.Error()}}, err
	}
	status := Status{Present: true, Path: filepath.ToSlash(path)}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return manifest, Status{Present: true, Path: filepath.ToSlash(path), Stale: true, Issues: []string{err.Error()}}, err
	}
	checkProvenanceRef(absRoot, manifest, &status)
	for rel, want := range manifest.FileHashes {
		if manifest.Provenance != nil && rel == manifest.Provenance.TransactionPath {
			continue
		}
		cleanRel, absPath, err := cleanRelativePath(absRoot, rel)
		if err != nil {
			status.Stale = true
			status.Issues = append(status.Issues, rel+": "+err.Error())
			continue
		}
		got, err := fileHash(absPath)
		if err != nil {
			status.Stale = true
			status.Issues = append(status.Issues, cleanRel+": "+err.Error())
			continue
		}
		if got != want {
			status.Stale = true
			status.Issues = append(status.Issues, cleanRel+": hash mismatch")
		}
	}
	return manifest, status, nil
}

func hydrateProvenanceRef(absRoot string, ref *ProvenanceRef, hashes map[string]string) error {
	if ref == nil {
		return nil
	}
	rel := strings.TrimSpace(ref.TransactionPath)
	if rel == "" {
		return fmt.Errorf("provenance transaction_path is required")
	}
	cleanRel, absPath, err := cleanRelativePath(absRoot, rel)
	if err != nil {
		return fmt.Errorf("invalid provenance transaction_path: %w", err)
	}
	hash, err := fileHash(absPath)
	if err != nil {
		return fmt.Errorf("hash provenance transaction: %w", err)
	}
	ref.TransactionPath = cleanRel
	ref.Hash = hash
	hashes[cleanRel] = hash
	return nil
}

func checkProvenanceRef(absRoot string, manifest Manifest, status *Status) {
	if manifest.Provenance == nil {
		return
	}
	ref := manifest.Provenance
	if strings.TrimSpace(ref.TransactionPath) == "" {
		markStatusStale(status, "provenance.transaction_path: required")
		return
	}
	cleanRel, absPath, err := cleanRelativePath(absRoot, ref.TransactionPath)
	if err != nil {
		markStatusStale(status, "provenance.transaction_path: "+err.Error())
		return
	}
	got, err := fileHash(absPath)
	if err != nil {
		markStatusStale(status, cleanRel+": "+err.Error())
		return
	}
	if ref.Hash != "" && got != ref.Hash {
		markStatusStale(status, cleanRel+": provenance hash mismatch")
	}
	if want, ok := manifest.FileHashes[cleanRel]; !ok {
		markStatusStale(status, cleanRel+": missing file hash")
	} else if got != want {
		markStatusStale(status, cleanRel+": hash mismatch")
	}
}

func cleanRelativePath(absRoot string, rel string) (string, string, error) {
	if filepath.IsAbs(rel) {
		return "", "", fmt.Errorf("path must be relative")
	}
	cleanRel := filepath.Clean(filepath.FromSlash(rel))
	if cleanRel == "." {
		return "", "", fmt.Errorf("path is required")
	}
	for _, part := range strings.Split(filepath.ToSlash(cleanRel), "/") {
		if part == ".." {
			return "", "", fmt.Errorf("path must not contain parent traversal")
		}
	}
	absPath := filepath.Join(absRoot, cleanRel)
	relToRoot, err := filepath.Rel(absRoot, absPath)
	if err != nil || relToRoot == ".." || strings.HasPrefix(relToRoot, ".."+string(filepath.Separator)) {
		return "", "", fmt.Errorf("path must be inside manifest root")
	}
	return filepath.ToSlash(cleanRel), absPath, nil
}

func markStatusStale(status *Status, issue string) {
	status.Stale = true
	status.Issues = append(status.Issues, issue)
}

func fileHash(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
