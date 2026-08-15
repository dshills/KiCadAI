package capabilityexecutorv10

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"kicadai/internal/canonicaljsonstream"
	"kicadai/internal/opentopologysynthesis"
)

const replaySpoolNameV11 = "SYNTHESIS_RUN.json"

func writeReplaySpoolV11(path string, run *opentopologysynthesis.SynthesisRun) (string, error) {
	if _, err := os.Lstat(path); err == nil {
		return "", fmt.Errorf("replay spool already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	parent := filepath.Dir(path)
	temporary, err := os.CreateTemp(parent, ".v11-replay-")
	if err != nil {
		return "", err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	digest := sha256.New()
	if err := canonicaljsonstream.Encode(io.MultiWriter(temporary, digest), run); err != nil {
		temporary.Close()
		return "", err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return "", err
	}
	writtenInfo, err := temporary.Stat()
	if err != nil {
		temporary.Close()
		return "", err
	}
	if err := temporary.Close(); err != nil {
		return "", err
	}
	if err := os.Chmod(temporaryPath, 0o444); err != nil {
		return "", err
	}
	sealedInfo, err := os.Lstat(temporaryPath)
	if err != nil || !sealedInfo.Mode().IsRegular() || sealedInfo.Mode().Perm() != 0o444 || !os.SameFile(writtenInfo, sealedInfo) {
		return "", fmt.Errorf("replay spool changed before installation")
	}
	if err := os.Link(temporaryPath, path); err != nil {
		return "", err
	}
	installedInfo, err := os.Lstat(path)
	if err != nil || !installedInfo.Mode().IsRegular() || installedInfo.Mode().Perm() != 0o444 || !os.SameFile(sealedInfo, installedInfo) {
		return "", fmt.Errorf("replay spool changed during installation")
	}
	directory, err := os.Open(parent)
	if err != nil {
		return "", err
	}
	defer directory.Close()
	if err := directory.Sync(); err != nil {
		return "", err
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

func removeReplaySpoolsV11(paths []string) error {
	for _, path := range paths {
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0o444 {
			return fmt.Errorf("remove replay spool %q: file binding is invalid", path)
		}
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("remove replay spool %q: %w", path, err)
		}
	}
	return nil
}
