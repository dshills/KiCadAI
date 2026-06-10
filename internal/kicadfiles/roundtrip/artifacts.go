package roundtrip

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

func NewArtifactWorkspace(fixtureName string, opts Options) (ArtifactWorkspace, func(), error) {
	base := strings.TrimSpace(opts.ArtifactDir)
	var root string
	var err error
	if base != "" {
		if err := os.MkdirAll(base, 0o755); err != nil {
			return ArtifactWorkspace{}, nil, fmt.Errorf("create artifact base: %w", err)
		}
		root, err = os.MkdirTemp(base, sanitizePathComponent(fixtureName)+"-*")
	} else {
		root, err = os.MkdirTemp("", "kicadai-"+sanitizePathComponent(fixtureName)+"-*")
	}
	if err != nil {
		return ArtifactWorkspace{}, nil, fmt.Errorf("create artifact workspace: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		_ = os.RemoveAll(root)
		return ArtifactWorkspace{}, nil, fmt.Errorf("resolve artifact workspace: %w", err)
	}
	absRoot, err := filepath.Abs(resolved)
	if err != nil {
		_ = os.RemoveAll(root)
		return ArtifactWorkspace{}, nil, fmt.Errorf("resolve absolute artifact workspace: %w", err)
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
	cleanParts := append([]string{w.Root}, parts...)
	target := filepath.Join(cleanParts...)
	if !pathWithinRoot(w.Root, target) {
		return "", fmt.Errorf("artifact path escapes workspace: %s", filepath.Join(parts...))
	}
	return target, nil
}

func (w ArtifactWorkspace) CopyInput(src string) (string, error) {
	name := sanitizePathComponent(filepath.Base(src))
	if name == "" {
		return "", fmt.Errorf("input path has empty base: %s", src)
	}
	in, err := os.Open(src)
	if err != nil {
		return "", fmt.Errorf("open input: %w", err)
	}
	defer in.Close()
	dst, out, err := w.createUniqueFile(name)
	if err != nil {
		return "", fmt.Errorf("create artifact copy: %w", err)
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		_ = os.Remove(dst)
		return "", fmt.Errorf("copy input: %w", err)
	}
	if err := out.Close(); err != nil {
		return "", fmt.Errorf("close artifact copy: %w", err)
	}
	return dst, nil
}

func (w ArtifactWorkspace) createUniqueFile(name string) (string, *os.File, error) {
	for i := 0; i < 1000; i++ {
		candidate := name
		if i > 0 {
			ext := filepath.Ext(name)
			base := strings.TrimSuffix(name, ext)
			candidate = fmt.Sprintf("%s-%d%s", base, i+1, ext)
		}
		path, err := w.Path(candidate)
		if err != nil {
			return "", nil, err
		}
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err == nil {
			return path, file, nil
		}
		if os.IsExist(err) {
			continue
		}
		return "", nil, err
	}
	return "", nil, fmt.Errorf("could not create unique artifact for %s after 1000 attempts", name)
}

func (w ArtifactWorkspace) WriteText(name string, contents string) (string, error) {
	target, err := w.Path(name)
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
	if rel == "." {
		return true
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
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
	return strings.Trim(out.String(), ".-")
}
