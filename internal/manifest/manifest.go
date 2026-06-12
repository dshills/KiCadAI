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
	for _, artifact := range manifest.Artifacts {
		if strings.TrimSpace(artifact.Path) == "" {
			continue
		}
		absArtifact, err := filepath.Abs(artifact.Path)
		if err != nil {
			return reports.Artifact{}, err
		}
		rel, err := filepath.Rel(absRoot, absArtifact)
		if err != nil || strings.HasPrefix(rel, "..") {
			return reports.Artifact{}, fmt.Errorf("artifact is outside manifest root: %s", artifact.Path)
		}
		if filepath.ToSlash(rel) == RelativePath {
			continue
		}
		hash, err := fileHash(absArtifact)
		if err != nil {
			return reports.Artifact{}, err
		}
		manifest.FileHashes[filepath.ToSlash(rel)] = hash
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
	for rel, want := range manifest.FileHashes {
		got, err := fileHash(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			status.Stale = true
			status.Issues = append(status.Issues, rel+": "+err.Error())
			continue
		}
		if got != want {
			status.Stale = true
			status.Issues = append(status.Issues, rel+": hash mismatch")
		}
	}
	return manifest, status, nil
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
