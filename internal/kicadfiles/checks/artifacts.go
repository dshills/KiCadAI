package checks

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type ArtifactWorkspace struct {
	Root string
}

func NewArtifactWorkspace(name string, opts Options) (ArtifactWorkspace, func(), error) {
	base := strings.TrimSpace(opts.ArtifactDir)
	var (
		root string
		err  error
	)
	prefix := "kicadai-check-" + sanitizePathComponent(name) + "-"
	if base != "" {
		if err := os.MkdirAll(base, 0o755); err != nil {
			return ArtifactWorkspace{}, nil, fmt.Errorf("create artifact base: %w", err)
		}
		root, err = os.MkdirTemp(base, prefix)
	} else {
		root, err = os.MkdirTemp("", prefix)
	}
	if err != nil {
		return ArtifactWorkspace{}, nil, fmt.Errorf("create artifact workspace: %w", err)
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		_ = os.RemoveAll(root)
		return ArtifactWorkspace{}, nil, fmt.Errorf("resolve artifact workspace: %w", err)
	}
	workspace := ArtifactWorkspace{Root: absRoot}
	cleanup := func() {
		if !opts.KeepArtifacts {
			_ = os.RemoveAll(workspace.Root)
		}
	}
	return workspace, cleanup, nil
}

func (w ArtifactWorkspace) Path(parts ...string) (string, error) {
	if strings.TrimSpace(w.Root) == "" {
		return "", fmt.Errorf("artifact workspace root is empty")
	}
	for _, part := range parts {
		if filepath.IsAbs(part) {
			return "", fmt.Errorf("artifact path must be relative: %s", part)
		}
	}
	target := filepath.Join(append([]string{w.Root}, parts...)...)
	if !pathWithinRoot(w.Root, target) {
		return "", fmt.Errorf("artifact path escapes workspace: %s", filepath.Join(parts...))
	}
	return target, nil
}

func (w ArtifactWorkspace) CopyFile(src string, rel string) (string, error) {
	if strings.TrimSpace(rel) == "" {
		rel = filepath.Base(src)
	}
	dst, err := w.Path(rel)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return "", fmt.Errorf("create artifact parent: %w", err)
	}
	in, err := os.Open(src)
	if err != nil {
		return "", fmt.Errorf("open input: %w", err)
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return "", fmt.Errorf("create artifact copy: %w", err)
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return "", fmt.Errorf("copy input: %w", err)
	}
	if err := out.Close(); err != nil {
		return "", fmt.Errorf("close artifact copy: %w", err)
	}
	return dst, nil
}

func (w ArtifactWorkspace) WriteText(rel string, contents string) (string, error) {
	target, err := w.Path(rel)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return "", fmt.Errorf("create artifact parent: %w", err)
	}
	if err := os.WriteFile(target, []byte(contents), 0o644); err != nil {
		return "", fmt.Errorf("write artifact: %w", err)
	}
	return target, nil
}

func pathWithinRoot(root, target string) bool {
	root = filepath.Clean(root)
	target = filepath.Clean(target)
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

func sanitizePathComponent(value string) string {
	value = strings.TrimSpace(value)
	var out strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			out.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			out.WriteRune(r)
		case r >= '0' && r <= '9':
			out.WriteRune(r)
		case r == '-' || r == '_' || r == '.':
			out.WriteRune(r)
		default:
			out.WriteRune('-')
		}
	}
	cleaned := strings.Trim(out.String(), ".-")
	if cleaned == "" {
		return "check"
	}
	return cleaned
}
