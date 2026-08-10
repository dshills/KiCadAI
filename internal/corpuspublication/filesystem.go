package corpuspublication

import (
	"bufio"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func validatePaths(repository, destination, key string) (string, string, string, error) {
	if strings.TrimSpace(repository) == "" || strings.TrimSpace(destination) == "" || strings.TrimSpace(key) == "" {
		return "", "", "", fmt.Errorf("repository, destination, and key paths are required")
	}
	repository, err := filepath.Abs(filepath.Clean(repository))
	if err != nil {
		return "", "", "", fmt.Errorf("resolve repository root: %w", err)
	}
	repository, err = filepath.EvalSymlinks(repository)
	if err != nil {
		return "", "", "", fmt.Errorf("resolve real repository root: %w", err)
	}
	destination, err = canonicalFuturePath(destination)
	if err != nil {
		return "", "", "", fmt.Errorf("resolve destination: %w", err)
	}
	key, err = canonicalFuturePath(key)
	if err != nil {
		return "", "", "", fmt.Errorf("resolve external key path: %w", err)
	}
	if !pathWithin(repository, destination) || pathWithin(repository, key) {
		return "", "", "", fmt.Errorf("destination must be inside and key must be outside the repository")
	}
	return repository, destination, key, nil
}

func canonicalFuturePath(path string) (string, error) {
	absolute, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", err
	}
	ancestor := absolute
	for {
		if _, err := os.Lstat(ancestor); err == nil {
			break
		} else if !os.IsNotExist(err) {
			return "", err
		}
		parent := filepath.Dir(ancestor)
		if parent == ancestor {
			return "", fmt.Errorf("no existing path ancestor")
		}
		ancestor = parent
	}
	realAncestor, err := filepath.EvalSymlinks(ancestor)
	if err != nil {
		return "", err
	}
	remainder, err := filepath.Rel(ancestor, absolute)
	if err != nil {
		return "", err
	}
	return filepath.Join(realAncestor, remainder), nil
}

func createExclusiveKey(repositoryRoot, path string, random io.Reader) ([]byte, error) {
	parent := filepath.Dir(path)
	if pathWithin(repositoryRoot, parent) {
		return nil, fmt.Errorf("external key directory is inside the repository")
	}
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return nil, fmt.Errorf("create external key directory: %w", err)
	}
	realParent, err := filepath.EvalSymlinks(parent)
	if err != nil {
		return nil, fmt.Errorf("resolve external key directory: %w", err)
	}
	if pathWithin(repositoryRoot, realParent) {
		return nil, fmt.Errorf("external key directory resolves inside the repository")
	}
	key := make([]byte, 32)
	if random == nil {
		random = rand.Reader
	}
	if _, err := io.ReadFull(random, key); err != nil {
		return nil, fmt.Errorf("generate held-out source key: %w", err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, fmt.Errorf("create fresh external key: %w", err)
	}
	remove := true
	defer func() {
		_ = file.Close()
		if remove {
			_ = os.Remove(path)
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
	remove = false
	return key, nil
}

func writeExclusive(path string, data []byte, mode fs.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create publication directory: %w", err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return fmt.Errorf("create publication artifact: %w", err)
	}
	remove := true
	defer func() {
		_ = file.Close()
		if remove {
			_ = os.Remove(path)
		}
	}()
	if _, err := file.Write(data); err != nil {
		return fmt.Errorf("write publication artifact: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync publication artifact: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close publication artifact: %w", err)
	}
	remove = false
	return nil
}

func writeChecksums(root string) error {
	files := []string{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("publication stage contains a symlink")
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("publication stage contains a special file")
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if relative != ChecksumFile {
			files = append(files, relative)
		}
		return nil
	})
	if err != nil {
		return err
	}
	sort.Strings(files)
	var output strings.Builder
	for _, relative := range files {
		digest, err := hashRegularFile(filepath.Join(root, filepath.FromSlash(relative)), 32<<20)
		if err != nil {
			return fmt.Errorf("hash publication artifact: %w", err)
		}
		fmt.Fprintf(&output, "%s  %s\n", digest, relative)
	}
	return writeExclusive(filepath.Join(root, ChecksumFile), []byte(output.String()), 0o644)
}

func hashRegularFile(path string, maximum int64) (string, error) {
	before, err := os.Lstat(path)
	if err != nil {
		return "", fmt.Errorf("inspect checksum source: %w", err)
	}
	if before.Mode()&os.ModeSymlink != 0 || !before.Mode().IsRegular() || before.Size() < 0 || before.Size() > maximum {
		return "", fmt.Errorf("checksum source is not a bounded regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open checksum source: %w", err)
	}
	hash := sha256.New()
	written, copyErr := io.Copy(hash, file)
	after, statErr := file.Stat()
	closeErr := file.Close()
	if copyErr != nil || statErr != nil || closeErr != nil || !os.SameFile(before, after) || written != before.Size() || after.Size() != before.Size() || after.ModTime() != before.ModTime() {
		return "", fmt.Errorf("checksum source changed while hashing")
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func syncTree(root string) error {
	directories := []string{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			directories = append(directories, path)
		}
		return nil
	})
	if err != nil {
		return err
	}
	sort.Slice(directories, func(i, j int) bool { return len(directories[i]) > len(directories[j]) })
	for _, directory := range directories {
		if err := syncDirectory(directory); err != nil {
			return err
		}
	}
	return nil
}

func pathWithin(root, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	return relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)))
}

func verifyChecksums(root string) error {
	data, err := readRegularFile(filepath.Join(root, ChecksumFile))
	if err != nil {
		return err
	}
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		parts := strings.SplitN(scanner.Text(), "  ", 2)
		if len(parts) != 2 || !validSHA256(parts[0]) || !validRelativeArtifactPath(parts[1]) {
			return fmt.Errorf("invalid publication checksum entry")
		}
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(parts[1])))
		if err != nil || hashBytes(data) != parts[0] {
			return fmt.Errorf("publication checksum mismatch")
		}
	}
	return scanner.Err()
}

func validRelativeArtifactPath(path string) bool {
	return path != "" && path == filepath.ToSlash(filepath.Clean(filepath.FromSlash(path))) && path != "." && !strings.HasPrefix(path, "../") && !filepath.IsAbs(filepath.FromSlash(path))
}
