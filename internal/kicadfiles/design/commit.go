package design

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/text/unicode/norm"
)

const directoryCommitJournalVersion = 1

const (
	directoryCommitMoveExisting = "move-existing"
	directoryCommitMoveNew      = "move-new"
)

type directoryCommitJournal struct {
	Version  int    `json:"version"`
	Phase    string `json:"phase"`
	Target   string `json:"target"`
	Prepared string `json:"prepared"`
	Backup   string `json:"backup"`
}

// CommitPreparedDirectory replaces target with a fully prepared sibling
// directory. Existing targets are protected by a recoverable move-aside
// journal until the replacement is durable.
func CommitPreparedDirectory(root, prepared string, overwrite bool) (WriteResult, error) {
	target, parent, base, err := commitPaths(root)
	if err != nil {
		return WriteResult{}, err
	}
	prepared = norm.NFC.String(filepath.Clean(prepared))
	relPrepared, err := filepath.Rel(parent, prepared)
	if err != nil || relPrepared == ".." || filepath.IsAbs(relPrepared) || len(relPrepared) >= 3 && relPrepared[:3] == ".."+string(filepath.Separator) {
		return WriteResult{}, fmt.Errorf("prepared directory must be on the target filesystem: %s", prepared)
	}
	preparedInfo, err := os.Stat(prepared)
	if err != nil {
		return WriteResult{}, fmt.Errorf("prepared directory: %w", err)
	}
	if !preparedInfo.IsDir() {
		return WriteResult{}, fmt.Errorf("prepared path is not a directory: %s", prepared)
	}
	journalPath := filepath.Join(parent, "."+base+".kicadai-journal")
	backupPath := filepath.Join(parent, "."+base+".kicadai-backup")
	lock, err := acquireDirectoryCommitLock(filepath.Join(parent, "."+base+".kicadai-lock"))
	if err != nil {
		return WriteResult{}, fmt.Errorf("acquire project commit lock: %w", err)
	}
	defer lock.Close()
	if err := recoverDirectoryCommit(target, journalPath, backupPath); err != nil {
		return WriteResult{}, err
	}

	targetInfo, targetErr := os.Stat(target)
	targetExists := targetErr == nil
	if targetExists && !targetInfo.IsDir() {
		return WriteResult{}, fmt.Errorf("target exists and is not a directory: %s", target)
	}
	if targetExists && !overwrite {
		return WriteResult{}, fmt.Errorf("target exists: %s", target)
	}
	if targetErr != nil && !errors.Is(targetErr, os.ErrNotExist) {
		return WriteResult{}, targetErr
	}
	if !targetExists {
		if err := os.Rename(prepared, target); err != nil {
			return WriteResult{}, err
		}
		if err := syncDir(parent); err != nil {
			return WriteResult{}, err
		}
		return WriteResult{ProjectDir: target}, nil
	}
	if _, err := os.Stat(backupPath); err == nil {
		return WriteResult{}, fmt.Errorf("orphaned commit backup exists: %s", backupPath)
	} else if !errors.Is(err, os.ErrNotExist) {
		return WriteResult{}, err
	}

	journal := directoryCommitJournal{
		Version: directoryCommitJournalVersion, Phase: directoryCommitMoveExisting,
		Target: target, Prepared: prepared, Backup: backupPath,
	}
	if err := writeDirectoryCommitJournal(journalPath, journal, true); err != nil {
		return WriteResult{}, err
	}
	if err := os.Rename(target, backupPath); err != nil {
		_ = os.Remove(journalPath)
		return WriteResult{}, err
	}
	if err := syncDir(parent); err != nil {
		return WriteResult{}, rollbackDirectoryCommit(target, backupPath, journalPath, err)
	}
	journal.Phase = directoryCommitMoveNew
	if err := writeDirectoryCommitJournal(journalPath, journal, false); err != nil {
		return WriteResult{}, rollbackDirectoryCommit(target, backupPath, journalPath, err)
	}
	if err := os.Rename(prepared, target); err != nil {
		return WriteResult{}, rollbackDirectoryCommit(target, backupPath, journalPath, err)
	}
	if err := syncDir(parent); err != nil {
		return WriteResult{}, rollbackDirectoryCommit(target, backupPath, journalPath, err)
	}

	result := WriteResult{ProjectDir: target}
	if err := os.RemoveAll(backupPath); err != nil {
		result.Warnings = append(result.Warnings, err.Error())
		return result, nil
	}
	if err := os.Remove(journalPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		result.Warnings = append(result.Warnings, err.Error())
		return result, nil
	}
	if err := syncDir(parent); err != nil {
		result.Warnings = append(result.Warnings, err.Error())
	}
	return result, nil
}

func commitPaths(root string) (target, parent, base string, err error) {
	target = norm.NFC.String(filepath.Clean(root))
	parent = filepath.Dir(target)
	base = filepath.Base(target)
	if base == "." || base == string(filepath.Separator) || base == "" {
		return "", "", "", fmt.Errorf("target must name a project directory: %s", root)
	}
	if err := validateFileComponent(base); err != nil {
		return "", "", "", fmt.Errorf("target directory name: %w", err)
	}
	return target, parent, base, nil
}

func writeDirectoryCommitJournal(path string, journal directoryCommitJournal, exclusive bool) error {
	data, err := json.Marshal(journal)
	if err != nil {
		return err
	}
	return writeSyncedFile(path, append(data, '\n'), 0o600, exclusive)
}

func rollbackDirectoryCommit(target, backup, journal string, cause error) error {
	if removeErr := os.RemoveAll(target); removeErr != nil {
		return errors.Join(cause, fmt.Errorf("rollback remove replacement: %w", removeErr))
	}
	if err := os.Rename(backup, target); err != nil {
		return errors.Join(cause, fmt.Errorf("rollback restore existing project: %w", err))
	}
	_ = syncDir(filepath.Dir(target))
	_ = os.Remove(journal)
	return cause
}

func recoverDirectoryCommit(target, journalPath, expectedBackup string) error {
	data, err := os.ReadFile(journalPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var journal directoryCommitJournal
	if err := json.Unmarshal(data, &journal); err != nil {
		return fmt.Errorf("invalid recovery journal %s: %w", journalPath, err)
	}
	if journal.Version != directoryCommitJournalVersion || journal.Target != target || journal.Backup != expectedBackup {
		return fmt.Errorf("recovery journal does not match target: %s", journalPath)
	}
	targetExists, err := directoryExists(target)
	if err != nil {
		return err
	}
	backupExists, err := directoryExists(journal.Backup)
	if err != nil {
		return err
	}
	switch journal.Phase {
	case directoryCommitMoveExisting:
		if !targetExists && backupExists {
			if err := os.Rename(journal.Backup, target); err != nil {
				return fmt.Errorf("recover existing project: %w", err)
			}
		} else if targetExists && backupExists {
			return fmt.Errorf("ambiguous move-existing recovery state: %s", journalPath)
		} else if !targetExists {
			return fmt.Errorf("recovery journal has no target or backup: %s", journalPath)
		}
	case directoryCommitMoveNew:
		if targetExists && backupExists {
			if err := os.RemoveAll(journal.Backup); err != nil {
				return fmt.Errorf("complete replacement cleanup: %w", err)
			}
		} else if !targetExists && backupExists {
			if err := os.Rename(journal.Backup, target); err != nil {
				return fmt.Errorf("roll back interrupted replacement: %w", err)
			}
		} else if !targetExists {
			return fmt.Errorf("recovery journal has no target or backup: %s", journalPath)
		}
	default:
		return fmt.Errorf("unknown recovery journal phase %q", journal.Phase)
	}
	parent := filepath.Dir(target)
	if err := syncDir(parent); err != nil {
		return err
	}
	if err := os.Remove(journalPath); err != nil {
		return err
	}
	return syncDir(parent)
}

func directoryExists(path string) (bool, error) {
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !info.IsDir() {
		return false, fmt.Errorf("commit path is not a directory: %s", path)
	}
	return true, nil
}
