package atomicfile

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	JournalRelativePath      = ".kicadai/mutation-journal.json"
	MutationLockRelativePath = ".kicadai/mutation.lock"
)

type Transition string

const (
	TransitionStaging       Transition = "staging"
	TransitionFileSync      Transition = "file_sync"
	TransitionJournalSync   Transition = "journal_sync"
	TransitionReplacement   Transition = "replacement"
	TransitionDirectorySync Transition = "directory_sync"
	TransitionValidation    Transition = "validation"
	TransitionRollback      Transition = "rollback"
	TransitionCleanup       Transition = "cleanup"
)

// Mutation is one member of a recoverable file-group update.
type Mutation struct {
	Path   string
	Data   []byte
	Mode   os.FileMode
	Delete bool
}

// GroupOptions controls validation and deterministic fault injection.
type GroupOptions struct {
	Root            string
	ValidateStaged  func(stagedByTarget map[string]string) error
	Fault           func(Transition) error
	PreserveOnError bool
}

type journalState string

const (
	statePrepared    journalState = "prepared"
	stateApplying    journalState = "applying"
	stateApplied     journalState = "applied"
	stateCommitted   journalState = "committed"
	stateRollingBack journalState = "rolling_back"
	stateRolledBack  journalState = "rolled_back"
)

type journalEntry struct {
	Target      string `json:"target"`
	Stage       string `json:"stage,omitempty"`
	Backup      string `json:"backup,omitempty"`
	HadOriginal bool   `json:"had_original"`
	Delete      bool   `json:"delete,omitempty"`
	Mode        uint32 `json:"mode,omitempty"`
	PriorMode   uint32 `json:"prior_mode,omitempty"`
	PriorHash   string `json:"prior_hash,omitempty"`
	NewHash     string `json:"new_hash,omitempty"`
	Applied     bool   `json:"applied,omitempty"`
}

type groupJournal struct {
	Schema  string         `json:"schema"`
	Token   string         `json:"token"`
	State   journalState   `json:"state"`
	Entries []journalEntry `json:"entries"`
}

// Group retains rollback material until Commit confirms application-level
// validation, or Rollback restores the prior project.
type Group struct {
	root        string
	journalPath string
	journal     groupJournal
	options     GroupOptions
	lock        *Lock
	closed      bool
}

// BeginGroup stages, validates, journals, and atomically replaces every member.
// The caller must then call Commit or Rollback.
func BeginGroup(mutations []Mutation, options GroupOptions) (*Group, error) {
	if len(mutations) == 0 {
		return nil, nil
	}
	root, err := normalizedRoot(options.Root)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(root, ".kicadai"), 0o755); err != nil {
		return nil, err
	}
	lock, err := AcquireLock(filepath.Join(root, filepath.FromSlash(MutationLockRelativePath)))
	if err != nil {
		return nil, err
	}
	group := &Group{
		root:        root,
		journalPath: filepath.Join(root, filepath.FromSlash(JournalRelativePath)),
		options:     options,
		lock:        lock,
	}
	if err := group.recoverLocked(); err != nil {
		_ = lock.Release()
		return nil, fmt.Errorf("recover interrupted mutation: %w", err)
	}
	token, err := randomToken()
	if err != nil {
		_ = lock.Release()
		return nil, err
	}
	group.journal = groupJournal{Schema: "kicadai.atomicfile.group.v1", Token: token, State: statePrepared}
	normalized, err := group.normalizeMutations(mutations)
	if err != nil {
		_ = lock.Release()
		return nil, err
	}
	journalWritten := false
	if err := group.stage(normalized); err != nil {
		return nil, group.abortBegin(err, journalWritten)
	}
	if options.ValidateStaged != nil {
		staged := make(map[string]string, len(group.journal.Entries))
		for _, entry := range group.journal.Entries {
			if !entry.Delete {
				staged[filepath.Join(root, filepath.FromSlash(entry.Target))] = filepath.Join(root, filepath.FromSlash(entry.Stage))
			}
		}
		if err := options.ValidateStaged(staged); err != nil {
			return nil, group.abortBegin(err, journalWritten)
		}
	}
	if err := group.inject(TransitionValidation); err != nil {
		return nil, group.abortBegin(err, journalWritten)
	}
	if err := group.createBackups(); err != nil {
		return nil, group.abortBegin(err, journalWritten)
	}
	if err := group.writeJournal(); err != nil {
		_, statErr := os.Stat(group.journalPath)
		journalWritten = statErr == nil
		return nil, group.abortBegin(err, journalWritten)
	}
	journalWritten = true
	group.journal.State = stateApplying
	if err := group.writeJournal(); err != nil {
		return nil, group.abortBegin(err, journalWritten)
	}
	for index := range group.journal.Entries {
		if err := group.applyEntry(index); err != nil {
			return nil, group.abortBegin(err, journalWritten)
		}
		if err := group.inject(TransitionReplacement); err != nil {
			return nil, group.abortBegin(err, journalWritten)
		}
		group.journal.Entries[index].Applied = true
		if err := group.writeJournal(); err != nil {
			return nil, group.abortBegin(err, journalWritten)
		}
	}
	if err := group.syncTargetDirectories(); err != nil {
		return nil, group.abortBegin(err, journalWritten)
	}
	if err := group.inject(TransitionDirectorySync); err != nil {
		return nil, group.abortBegin(err, journalWritten)
	}
	if err := group.verifyNew(); err != nil {
		return nil, group.abortBegin(err, journalWritten)
	}
	if err := group.inject(TransitionValidation); err != nil {
		return nil, group.abortBegin(err, journalWritten)
	}
	group.journal.State = stateApplied
	if err := group.writeJournal(); err != nil {
		return nil, group.abortBegin(err, journalWritten)
	}
	return group, nil
}

// RecoverGroup resolves an interrupted group before a new writer starts.
func RecoverGroup(root string) error {
	normalized, err := normalizedRoot(root)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(normalized, ".kicadai"), 0o755); err != nil {
		return err
	}
	lock, err := AcquireLock(filepath.Join(normalized, filepath.FromSlash(MutationLockRelativePath)))
	if err != nil {
		return err
	}
	defer lock.Release()
	group := &Group{
		root:        normalized,
		journalPath: filepath.Join(normalized, filepath.FromSlash(JournalRelativePath)),
		lock:        lock,
	}
	return group.recoverLocked()
}

// Commit records the validated all-new state before deleting rollback data.
func (group *Group) Commit() error {
	if group == nil {
		return nil
	}
	if group.closed {
		return fmt.Errorf("mutation group is already closed")
	}
	defer group.close()
	if err := group.verifyNew(); err != nil {
		rollbackErr := group.rollbackLocked()
		return errors.Join(fmt.Errorf("validate committed mutation: %w", err), rollbackErr)
	}
	group.journal.State = stateCommitted
	if err := group.writeJournal(); err != nil {
		return err
	}
	return group.cleanupLocked()
}

// AdoptCurrent records a deliberate, still-locked post-processing result (for
// example a KiCad zone refill) as the replacement identity. Rollback identity
// is unchanged.
func (group *Group) AdoptCurrent() error {
	if group == nil {
		return nil
	}
	if group.closed {
		return fmt.Errorf("mutation group is already closed")
	}
	for index := range group.journal.Entries {
		entry := &group.journal.Entries[index]
		target := group.path(entry.Target)
		if entry.Delete {
			if _, err := os.Stat(target); !os.IsNotExist(err) {
				if err == nil {
					return fmt.Errorf("deleted target was recreated: %s", target)
				}
				return err
			}
			continue
		}
		hash, err := hashFile(target)
		if err != nil {
			return err
		}
		entry.NewHash = hash
	}
	group.journal.State = stateApplied
	return group.writeJournal()
}

// Rollback restores and verifies the all-old state. Any failure leaves the
// journal and recovery material in place.
func (group *Group) Rollback() error {
	if group == nil {
		return nil
	}
	if group.closed {
		return fmt.Errorf("mutation group is already closed")
	}
	defer group.close()
	return group.rollbackLocked()
}

func (group *Group) normalizeMutations(mutations []Mutation) ([]Mutation, error) {
	normalized := make([]Mutation, 0, len(mutations))
	seen := map[string]bool{}
	for _, mutation := range mutations {
		target, err := filepath.Abs(mutation.Path)
		if err != nil {
			return nil, err
		}
		rel, err := filepath.Rel(group.root, target)
		if err != nil {
			return nil, err
		}
		rel = filepath.Clean(rel)
		relSlash := filepath.ToSlash(rel)
		if rel == "." || rel == ".." || strings.HasPrefix(relSlash, "../") || filepath.IsAbs(rel) {
			return nil, fmt.Errorf("mutation target is outside root: %s", mutation.Path)
		}
		if strings.EqualFold(relSlash, JournalRelativePath) || strings.EqualFold(relSlash, MutationLockRelativePath) {
			return nil, fmt.Errorf("mutation target is reserved: %s", mutation.Path)
		}
		if seen[relSlash] {
			return nil, fmt.Errorf("duplicate mutation target: %s", mutation.Path)
		}
		seen[relSlash] = true
		mutation.Path = filepath.Join(group.root, rel)
		if mutation.Mode == 0 {
			mutation.Mode = 0o644
		}
		normalized = append(normalized, mutation)
	}
	sort.Slice(normalized, func(i, j int) bool {
		return normalized[i].Path < normalized[j].Path
	})
	return normalized, nil
}

func (group *Group) stage(mutations []Mutation) error {
	for _, mutation := range mutations {
		if err := os.MkdirAll(filepath.Dir(mutation.Path), 0o755); err != nil {
			return err
		}
		rel, _ := filepath.Rel(group.root, mutation.Path)
		entry := journalEntry{
			Target: filepath.ToSlash(rel),
			Delete: mutation.Delete,
			Mode:   uint32(mutation.Mode.Perm()),
		}
		if info, err := os.Stat(mutation.Path); err == nil {
			if !info.Mode().IsRegular() {
				return fmt.Errorf("mutation target is not a regular file: %s", mutation.Path)
			}
			entry.HadOriginal = true
			entry.PriorMode = uint32(info.Mode().Perm())
			entry.PriorHash, err = hashFile(mutation.Path)
			if err != nil {
				return err
			}
		} else if !os.IsNotExist(err) {
			return err
		}
		if !mutation.Delete {
			stage, err := os.CreateTemp(filepath.Dir(mutation.Path), "."+filepath.Base(mutation.Path)+".stage-*")
			if err != nil {
				return err
			}
			stagePath := stage.Name()
			if err := stage.Chmod(mutation.Mode.Perm()); err != nil {
				_ = stage.Close()
				_ = os.Remove(stagePath)
				return err
			}
			if _, err := stage.Write(mutation.Data); err != nil {
				_ = stage.Close()
				_ = os.Remove(stagePath)
				return err
			}
			if err := group.inject(TransitionStaging); err != nil {
				_ = stage.Close()
				_ = os.Remove(stagePath)
				return err
			}
			if err := stage.Sync(); err != nil {
				_ = stage.Close()
				_ = os.Remove(stagePath)
				return err
			}
			if err := group.inject(TransitionFileSync); err != nil {
				_ = stage.Close()
				_ = os.Remove(stagePath)
				return err
			}
			if err := stage.Close(); err != nil {
				_ = os.Remove(stagePath)
				return err
			}
			entry.Stage, _ = filepath.Rel(group.root, stagePath)
			entry.Stage = filepath.ToSlash(entry.Stage)
			entry.NewHash = hashBytes(mutation.Data)
		}
		group.journal.Entries = append(group.journal.Entries, entry)
	}
	return nil
}

func (group *Group) createBackups() error {
	for index := range group.journal.Entries {
		entry := &group.journal.Entries[index]
		if !entry.HadOriginal {
			continue
		}
		target := group.path(entry.Target)
		backup, err := copyToTemporary(target, filepath.Dir(target), "."+filepath.Base(target)+".backup-*", os.FileMode(entry.PriorMode))
		if err != nil {
			return err
		}
		entry.Backup, _ = filepath.Rel(group.root, backup)
		entry.Backup = filepath.ToSlash(entry.Backup)
		if err := group.inject(TransitionFileSync); err != nil {
			return err
		}
	}
	return nil
}

func (group *Group) applyEntry(index int) error {
	entry := group.journal.Entries[index]
	target := group.path(entry.Target)
	if entry.Delete {
		if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	return replace(group.path(entry.Stage), target)
}

func (group *Group) writeJournal() error {
	raw, err := json.MarshalIndent(group.journal, "", "  ")
	if err != nil {
		return err
	}
	if err := Write(group.journalPath, append(raw, '\n'), 0o600); err != nil {
		return err
	}
	return group.inject(TransitionJournalSync)
}

func (group *Group) verifyNew() error {
	for _, entry := range group.journal.Entries {
		target := group.path(entry.Target)
		if entry.Delete {
			if _, err := os.Stat(target); !os.IsNotExist(err) {
				if err == nil {
					return fmt.Errorf("deleted target still exists: %s", target)
				}
				return err
			}
			continue
		}
		hash, err := hashFile(target)
		if err != nil {
			return err
		}
		if hash != entry.NewHash {
			return fmt.Errorf("committed hash mismatch for %s", target)
		}
	}
	return nil
}

func (group *Group) verifyOld() error {
	for _, entry := range group.journal.Entries {
		target := group.path(entry.Target)
		if !entry.HadOriginal {
			if _, err := os.Stat(target); !os.IsNotExist(err) {
				if err == nil {
					return fmt.Errorf("new target remains after rollback: %s", target)
				}
				return err
			}
			continue
		}
		hash, err := hashFile(target)
		if err != nil {
			return err
		}
		if hash != entry.PriorHash {
			return fmt.Errorf("rollback hash mismatch for %s", target)
		}
	}
	return nil
}

func (group *Group) rollbackLocked() error {
	group.journal.State = stateRollingBack
	if err := group.writeJournal(); err != nil {
		return err
	}
	for index := len(group.journal.Entries) - 1; index >= 0; index-- {
		entry := group.journal.Entries[index]
		target := group.path(entry.Target)
		if entry.HadOriginal {
			backup := group.path(entry.Backup)
			hash, err := hashFile(backup)
			if err != nil {
				// A prior rollback attempt may already have restored the target.
				if current, currentErr := hashFile(target); currentErr == nil && current == entry.PriorHash {
					continue
				}
				return fmt.Errorf("rollback backup unavailable for %s: %w", target, err)
			}
			if hash != entry.PriorHash {
				return fmt.Errorf("rollback backup hash mismatch for %s", target)
			}
			restore, err := copyToTemporary(backup, filepath.Dir(target), "."+filepath.Base(target)+".rollback-*", os.FileMode(entry.PriorMode))
			if err != nil {
				return err
			}
			if err := replace(restore, target); err != nil {
				_ = os.Remove(restore)
				return err
			}
		} else if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
			return err
		}
		if err := group.inject(TransitionRollback); err != nil {
			return err
		}
	}
	if err := group.syncTargetDirectories(); err != nil {
		return err
	}
	if err := group.verifyOld(); err != nil {
		return err
	}
	group.journal.State = stateRolledBack
	if err := group.writeJournal(); err != nil {
		return err
	}
	return group.cleanupLocked()
}

func (group *Group) recoverLocked() error {
	raw, err := os.ReadFile(group.journalPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var journal groupJournal
	if err := json.Unmarshal(raw, &journal); err != nil {
		return fmt.Errorf("read mutation journal: %w", err)
	}
	if journal.Schema != "kicadai.atomicfile.group.v1" || journal.Token == "" {
		return fmt.Errorf("unsupported mutation journal")
	}
	if err := group.validateJournal(journal); err != nil {
		return err
	}
	group.journal = journal
	switch journal.State {
	case stateCommitted:
		if err := group.verifyNew(); err != nil {
			return err
		}
		return group.cleanupLocked()
	case stateRolledBack:
		if err := group.verifyOld(); err != nil {
			return err
		}
		return group.cleanupLocked()
	case statePrepared, stateApplying, stateApplied, stateRollingBack:
		return group.rollbackLocked()
	default:
		return fmt.Errorf("unsupported mutation journal state %q", journal.State)
	}
}

func (group *Group) validateJournal(journal groupJournal) error {
	if len(journal.Entries) == 0 {
		return fmt.Errorf("mutation journal has no entries")
	}
	seen := map[string]bool{}
	for _, entry := range journal.Entries {
		if !safeJournalPath(entry.Target) {
			return fmt.Errorf("unsafe mutation journal target %q", entry.Target)
		}
		target := filepath.ToSlash(filepath.Clean(filepath.FromSlash(entry.Target)))
		if strings.EqualFold(target, JournalRelativePath) || strings.EqualFold(target, MutationLockRelativePath) {
			return fmt.Errorf("reserved mutation journal target %q", entry.Target)
		}
		if seen[target] {
			return fmt.Errorf("duplicate mutation journal target %q", entry.Target)
		}
		seen[target] = true
		targetDir := filepath.Dir(filepath.FromSlash(target))
		for _, recoveryPath := range []string{entry.Stage, entry.Backup} {
			if recoveryPath == "" {
				continue
			}
			if !safeJournalPath(recoveryPath) {
				return fmt.Errorf("unsafe mutation recovery path %q", recoveryPath)
			}
			if filepath.Dir(filepath.FromSlash(recoveryPath)) != targetDir {
				return fmt.Errorf("mutation recovery path is not beside target: %q", recoveryPath)
			}
		}
		if entry.HadOriginal && (entry.Backup == "" || !validHash(entry.PriorHash)) {
			return fmt.Errorf("mutation journal lacks prior identity for %q", entry.Target)
		}
		if !entry.Delete && !validHash(entry.NewHash) {
			return fmt.Errorf("mutation journal lacks replacement identity for %q", entry.Target)
		}
	}
	return nil
}

func (group *Group) cleanupLocked() error {
	if err := group.inject(TransitionCleanup); err != nil {
		return err
	}
	for _, entry := range group.journal.Entries {
		for _, rel := range []string{entry.Stage, entry.Backup} {
			if rel == "" {
				continue
			}
			if err := os.Remove(group.path(rel)); err != nil && !os.IsNotExist(err) {
				return err
			}
		}
	}
	if err := os.Remove(group.journalPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return group.syncTargetDirectories()
}

func (group *Group) syncTargetDirectories() error {
	dirs := map[string]bool{filepath.Dir(group.journalPath): true}
	for _, entry := range group.journal.Entries {
		dirs[filepath.Dir(group.path(entry.Target))] = true
	}
	ordered := make([]string, 0, len(dirs))
	for dir := range dirs {
		ordered = append(ordered, dir)
	}
	sort.Strings(ordered)
	for _, dir := range ordered {
		if err := syncDirectory(dir); err != nil {
			return err
		}
	}
	return nil
}

func (group *Group) abortBegin(cause error, journalWritten bool) error {
	if !journalWritten {
		group.removeRecoveryFiles()
		group.close()
		return cause
	}
	if group.options.PreserveOnError {
		group.close()
		return cause
	}
	rollbackErr := group.rollbackLocked()
	group.close()
	if rollbackErr != nil {
		return errors.Join(cause, fmt.Errorf("rollback failed: %w", rollbackErr))
	}
	return cause
}

func (group *Group) removeRecoveryFiles() {
	for _, entry := range group.journal.Entries {
		for _, rel := range []string{entry.Stage, entry.Backup} {
			if rel != "" {
				_ = os.Remove(group.path(rel))
			}
		}
	}
}

func (group *Group) close() {
	if group.closed {
		return
	}
	group.closed = true
	if group.lock != nil {
		_ = group.lock.Release()
	}
}

func (group *Group) inject(transition Transition) error {
	if group.options.Fault == nil {
		return nil
	}
	if err := group.options.Fault(transition); err != nil {
		return fmt.Errorf("%s: %w", transition, err)
	}
	return nil
}

func (group *Group) path(rel string) string {
	return filepath.Join(group.root, filepath.FromSlash(rel))
}

func normalizedRoot(root string) (string, error) {
	if strings.TrimSpace(root) == "" {
		return "", fmt.Errorf("mutation root is required")
	}
	return filepath.Abs(root)
}

func safeJournalPath(value string) bool {
	if strings.TrimSpace(value) == "" {
		return false
	}
	native := filepath.Clean(filepath.FromSlash(value))
	slash := filepath.ToSlash(native)
	return native != "." && native != ".." && !filepath.IsAbs(native) && !strings.HasPrefix(slash, "../")
}

func validHash(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func copyToTemporary(source string, directory string, pattern string, mode os.FileMode) (string, error) {
	input, err := os.Open(source)
	if err != nil {
		return "", err
	}
	defer input.Close()
	output, err := os.CreateTemp(directory, pattern)
	if err != nil {
		return "", err
	}
	path := output.Name()
	cleanup := true
	closed := false
	defer func() {
		if !closed {
			_ = output.Close()
		}
		if cleanup {
			_ = os.Remove(path)
		}
	}()
	if err := output.Chmod(mode.Perm()); err != nil {
		return "", err
	}
	if _, err := io.Copy(output, input); err != nil {
		return "", err
	}
	// Staging and rollback copies are referenced by the recovery journal
	// across process or power loss, so Close alone is not a durability
	// boundary here.
	if err := output.Sync(); err != nil {
		return "", err
	}
	if err := output.Close(); err != nil {
		return "", err
	}
	closed = true
	cleanup = false
	return path, nil
}

func hashFile(path string) (string, error) {
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

func hashBytes(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}
