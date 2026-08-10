// Package externalkey manages fixed-size secret keys that must remain outside
// a repository tree.
package externalkey

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const Size = 32

func Create(repositoryRoot, path string, random io.Reader) ([]byte, error) {
	if random == nil {
		return nil, fmt.Errorf("external key randomness is required")
	}
	resolved, err := resolve(repositoryRoot, path)
	if err != nil {
		return nil, err
	}
	key := make([]byte, Size)
	if _, err := io.ReadFull(random, key); err != nil {
		return nil, fmt.Errorf("read external key randomness: %w", err)
	}
	file, err := os.OpenFile(resolved, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, fmt.Errorf("create exclusive external key: %w", err)
	}
	committed := false
	defer func() {
		_ = file.Close()
		if !committed {
			_ = os.Remove(resolved)
		}
	}()
	if _, err := file.Write(key); err != nil {
		return nil, fmt.Errorf("write external key: %w", err)
	}
	if err := file.Sync(); err != nil {
		return nil, fmt.Errorf("sync external key: %w", err)
	}
	if err := file.Close(); err != nil {
		return nil, fmt.Errorf("close external key: %w", err)
	}
	info, err := os.Lstat(resolved)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 || info.Size() != Size {
		return nil, fmt.Errorf("external key permissions or size are invalid")
	}
	committed = true
	return key, nil
}

func Read(repositoryRoot, path string) ([]byte, error) {
	resolved, err := resolve(repositoryRoot, path)
	if err != nil {
		return nil, err
	}
	file, err := os.Open(resolved)
	if err != nil {
		return nil, fmt.Errorf("open external key: %w", err)
	}
	defer func() { _ = file.Close() }()
	openedInfo, err := file.Stat()
	if err != nil || !openedInfo.Mode().IsRegular() || openedInfo.Mode().Perm() != 0o600 || openedInfo.Size() != Size {
		return nil, fmt.Errorf("external key must be a 0600 regular %d-byte file", Size)
	}
	pathInfo, err := os.Lstat(resolved)
	if err != nil || !pathInfo.Mode().IsRegular() || !os.SameFile(openedInfo, pathInfo) {
		return nil, fmt.Errorf("external key path changed or is not regular")
	}
	key := make([]byte, Size)
	if _, err := io.ReadFull(file, key); err != nil {
		return nil, fmt.Errorf("read external key: %w", err)
	}
	var extra [1]byte
	if count, err := file.Read(extra[:]); count != 0 || err != io.EOF {
		return nil, fmt.Errorf("external key changed while reading")
	}
	return key, nil
}

func Distinct(repositoryRoot string, paths ...string) error {
	seen := map[string]bool{}
	for _, path := range paths {
		resolved, err := resolve(repositoryRoot, path)
		if err != nil {
			return err
		}
		if seen[resolved] {
			return fmt.Errorf("external key paths must be distinct")
		}
		seen[resolved] = true
	}
	return nil
}

func Remove(repositoryRoot, path string) error {
	resolved, err := resolve(repositoryRoot, path)
	if err != nil {
		return err
	}
	if err := os.Remove(resolved); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func resolve(repositoryRoot, path string) (string, error) {
	repositoryRoot, err := filepath.Abs(repositoryRoot)
	if err != nil {
		return "", fmt.Errorf("resolve repository root: %w", err)
	}
	repositoryRoot, err = filepath.EvalSymlinks(repositoryRoot)
	if err != nil {
		return "", fmt.Errorf("resolve real repository root: %w", err)
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve external key path: %w", err)
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(path))
	if err != nil {
		return "", fmt.Errorf("resolve external key parent: %w", err)
	}
	path = filepath.Join(parent, filepath.Base(path))
	relative, err := filepath.Rel(repositoryRoot, path)
	inside := err == nil && !filepath.IsAbs(relative) && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
	if err != nil || inside {
		return "", fmt.Errorf("external key path must be outside the repository")
	}
	return path, nil
}
