package atomicfile

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type lockRecord struct {
	Token        string `json:"token"`
	PID          int    `json:"pid"`
	ProcessStart string `json:"process_start,omitempty"`
}

var ErrLockHeld = errors.New("lock already held")

// Lock is an ownership-checked interprocess lock. The token is the portable
// ownership contract; advisory locking narrows stale-takeover races on
// platforms that support it.
type Lock struct {
	path   string
	token  string
	file   *os.File
	closed bool
}

// AcquireLock creates or safely takes over a stale lock.
func AcquireLock(path string) (*Lock, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	for attempt := 0; attempt < 4; attempt++ {
		token, err := randomToken()
		if err != nil {
			return nil, err
		}
		record := lockRecord{
			Token:        token,
			PID:          os.Getpid(),
			ProcessStart: processStartIdentity(os.Getpid()),
		}
		raw, err := json.Marshal(record)
		if err != nil {
			return nil, err
		}
		raw = append(raw, '\n')
		file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
		if err == nil {
			if err := advisoryLock(file); err != nil {
				_ = removeOwnedLockPath(file, path)
				_ = file.Close()
				return nil, fmt.Errorf("lock %s: %w", path, err)
			}
			if _, err := file.Write(raw); err != nil {
				_ = advisoryUnlock(file)
				_ = removeOwnedLockPath(file, path)
				_ = file.Close()
				return nil, err
			}
			if err := file.Sync(); err != nil {
				_ = advisoryUnlock(file)
				_ = removeOwnedLockPath(file, path)
				_ = file.Close()
				return nil, err
			}
			return &Lock{path: path, token: token, file: file}, nil
		}
		if !os.IsExist(err) {
			return nil, err
		}
		takenOver, takeoverErr := takeOverStaleLock(path)
		if takeoverErr != nil {
			return nil, takeoverErr
		}
		if !takenOver {
			return nil, fmt.Errorf("%w: %s", ErrLockHeld, path)
		}
	}
	return nil, fmt.Errorf("%w: ownership changed repeatedly: %s", ErrLockHeld, path)
}

func takeOverStaleLock(path string) (bool, error) {
	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		if os.IsNotExist(err) {
			return true, nil
		}
		return false, err
	}
	if err := advisoryLock(file); err != nil {
		_ = file.Close()
		if advisoryLockBusy(err) {
			return false, nil
		}
		return false, err
	}
	defer func() {
		_ = advisoryUnlock(file)
		_ = file.Close()
	}()
	inspected, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return true, nil
		}
		return false, err
	}
	record, ok := parseLockRecord(inspected)
	if !ok || lockRecordAlive(record) {
		return false, nil
	}
	current, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return true, nil
		}
		return false, err
	}
	if !bytes.Equal(inspected, current) {
		return false, nil
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return false, err
	}
	return true, nil
}

func parseLockRecord(raw []byte) (lockRecord, bool) {
	var record lockRecord
	if json.Unmarshal(raw, &record) == nil && record.PID > 0 {
		return record, record.Token != ""
	}
	for _, line := range strings.Split(string(raw), "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok || key != "pid" {
			continue
		}
		pid, err := strconv.Atoi(value)
		if err == nil && pid > 0 {
			return lockRecord{PID: pid}, true
		}
	}
	return lockRecord{}, false
}

func lockRecordAlive(record lockRecord) bool {
	if !processAlive(record.PID) {
		return false
	}
	if record.ProcessStart == "" {
		return true
	}
	current := processStartIdentity(record.PID)
	return current == "" || current == record.ProcessStart
}

// Release removes the lock only if its on-disk token still belongs to this
// holder. Replacing the lock path cannot make an old holder delete a new lock.
func (lock *Lock) Release() error {
	if lock == nil || lock.closed {
		return nil
	}
	lock.closed = true
	var releaseErr error
	raw, err := os.ReadFile(lock.path)
	if err == nil {
		record, ok := parseLockRecord(raw)
		if ok && record.Token == lock.token {
			if err := removeOwnedLockPath(lock.file, lock.path); err != nil && !os.IsNotExist(err) {
				releaseErr = err
			}
		}
	} else if !os.IsNotExist(err) {
		releaseErr = err
	}
	if err := advisoryUnlock(lock.file); releaseErr == nil && err != nil {
		releaseErr = err
	}
	if err := lock.file.Close(); releaseErr == nil && err != nil {
		releaseErr = err
	}
	return releaseErr
}

func randomToken() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}

func removeOwnedLockPath(file *os.File, path string) error {
	heldInfo, err := file.Stat()
	if err != nil {
		return err
	}
	pathInfo, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if !os.SameFile(heldInfo, pathInfo) {
		return nil
	}
	return os.Remove(path)
}

var errAdvisoryLockBusy = errors.New("advisory lock busy")
