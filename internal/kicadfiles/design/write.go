package design

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"

	"golang.org/x/text/unicode/norm"
	"kicadai/internal/kicadfiles/pcb"
	"kicadai/internal/kicadfiles/project"
	"kicadai/internal/kicadfiles/schematic"
)

type WriteOptions struct {
	Overwrite bool
}

type WriteResult struct {
	ProjectDir   string
	WrittenFiles []string
	Warnings     []string
	BackupDir    string
	JournalPath  string
}

type generatedFile struct {
	Path  string
	Mode  os.FileMode
	Write func(io.Writer) error
}

func WriteProjectDirectory(root string, design Design, opts WriteOptions) (WriteResult, error) {
	design.Name = norm.NFC.String(design.Name)
	if err := validateFileComponent(design.Name); err != nil {
		return WriteResult{}, err
	}
	if err := Validate(design); err != nil {
		return WriteResult{}, err
	}
	target := norm.NFC.String(filepath.Clean(root))
	parent := filepath.Dir(target)
	base := filepath.Base(target)
	if base == "." || base == string(filepath.Separator) || base == "" {
		return WriteResult{}, fmt.Errorf("target must name a project directory: %s", root)
	}
	if err := validateFileComponent(base); err != nil {
		return WriteResult{}, fmt.Errorf("target directory name: %w", err)
	}
	journalPath := filepath.Join(parent, "."+base+".kicadai-journal")
	if _, err := os.Stat(journalPath); err == nil {
		return WriteResult{}, fmt.Errorf("recovery journal exists: %s", journalPath)
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return WriteResult{}, err
	}
	targetInfo, targetErr := os.Stat(target)
	targetExists := targetErr == nil
	if targetExists && !targetInfo.IsDir() {
		return WriteResult{}, fmt.Errorf("target exists and is not a directory: %s", target)
	}
	if targetExists && !opts.Overwrite {
		return WriteResult{}, fmt.Errorf("target exists: %s", target)
	} else if targetErr != nil && !errors.Is(targetErr, os.ErrNotExist) {
		return WriteResult{}, targetErr
	}

	tempDir, err := os.MkdirTemp(parent, ".kicadai-tmp-*")
	if err != nil {
		return WriteResult{}, err
	}
	tempProjectDir := filepath.Join(tempDir, "project")
	cleanupTemp := true
	defer func() {
		if cleanupTemp {
			_ = os.RemoveAll(tempDir)
		}
	}()
	if err := os.Mkdir(tempProjectDir, 0o755); err != nil {
		return WriteResult{}, err
	}

	result := WriteResult{ProjectDir: target}
	writtenNames, err := writeDesignFiles(tempProjectDir, design)
	if err != nil {
		return result, err
	}

	if !targetExists {
		if err := os.Rename(tempProjectDir, target); err != nil {
			return result, err
		}
		if err := syncDir(parent); err != nil {
			return result, err
		}
		result.WrittenFiles = finalWrittenFiles(target, writtenNames)
		cleanupTemp = false
		_ = os.RemoveAll(tempDir)
		return result, nil
	}

	backupContainer, err := os.MkdirTemp(parent, ".kicadai-backup-*")
	if err != nil {
		return result, err
	}
	backupChild := filepath.Join(backupContainer, base)
	result.BackupDir = backupContainer
	result.JournalPath = journalPath
	if err := writeSyncedFile(journalPath, []byte("phase=move-existing\n"), 0o600, true); err != nil {
		_ = os.RemoveAll(backupContainer)
		return result, err
	}
	if err := os.Rename(target, backupChild); err != nil {
		_ = os.Remove(journalPath)
		_ = os.RemoveAll(backupContainer)
		return result, err
	}
	if err := syncDir(parent); err != nil {
		return result, err
	}
	if err := writeSyncedFile(journalPath, []byte("phase=move-new\n"), 0o600, false); err != nil {
		if rollbackErr := os.Rename(backupChild, target); rollbackErr != nil {
			return result, errors.Join(err, fmt.Errorf("rollback failed: %w", rollbackErr))
		}
		_ = syncDir(parent)
		_ = os.Remove(journalPath)
		_ = os.RemoveAll(backupContainer)
		return result, err
	}
	if err := os.Rename(tempProjectDir, target); err != nil {
		if rollbackErr := os.Rename(backupChild, target); rollbackErr != nil {
			return result, errors.Join(err, fmt.Errorf("rollback failed: %w", rollbackErr))
		}
		_ = os.Remove(journalPath)
		_ = os.RemoveAll(backupContainer)
		result.BackupDir = ""
		result.JournalPath = ""
		return result, err
	}
	if err := syncDir(parent); err != nil {
		return result, err
	}
	result.WrittenFiles = finalWrittenFiles(target, writtenNames)
	cleanupTemp = false
	_ = os.RemoveAll(tempDir)
	if err := os.RemoveAll(backupContainer); err != nil {
		result.Warnings = append(result.Warnings, err.Error())
	}
	result.BackupDir = ""
	if err := os.Remove(journalPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		result.Warnings = append(result.Warnings, err.Error())
		return result, nil
	}
	result.JournalPath = ""
	return result, nil
}

func writeDesignFiles(root string, design Design) ([]string, error) {
	name := norm.NFC.String(design.Name)
	if err := validateFileComponent(name); err != nil {
		return nil, err
	}
	files, err := designFiles(design)
	if err != nil {
		return nil, err
	}
	written := make([]string, 0, len(files))
	for _, file := range files {
		target := filepath.Join(root, filepath.FromSlash(file.Path))
		if err := writeFile(target, file.Mode, file.Write); err != nil {
			return nil, err
		}
		written = append(written, file.Path)
	}
	return written, nil
}

func designFiles(design Design) ([]generatedFile, error) {
	name := norm.NFC.String(design.Name)
	if err := validateFileComponent(name); err != nil {
		return nil, err
	}
	files := []generatedFile{{
		Path: name + ".kicad_pro",
		Mode: 0o644,
		Write: func(w io.Writer) error {
			return project.Write(w, design.Project)
		},
	}}
	if design.Schematic != nil {
		files = append(files, generatedFile{
			Path: name + ".kicad_sch",
			Mode: 0o644,
			Write: func(w io.Writer) error {
				return schematic.Write(w, *design.Schematic)
			},
		})
	}
	if design.PCB != nil {
		files = append(files, generatedFile{
			Path: name + ".kicad_pcb",
			Mode: 0o644,
			Write: func(w io.Writer) error {
				return pcb.Write(w, *design.PCB)
			},
		})
	}
	return validateGeneratedFiles(files)
}

func validateGeneratedFiles(files []generatedFile) ([]generatedFile, error) {
	normalized := make([]generatedFile, 0, len(files))
	seen := map[string]string{}
	seenFolded := map[string]string{}
	directories := map[string]string{}
	for _, file := range files {
		cleaned, err := normalizeGeneratedPath(file.Path)
		if err != nil {
			return nil, err
		}
		if prior, ok := seen[cleaned]; ok {
			return nil, fmt.Errorf("duplicate generated path %q also used by %q", cleaned, prior)
		}
		folded := strings.ToLower(cleaned)
		if prior, ok := seenFolded[folded]; ok {
			return nil, fmt.Errorf("case-insensitive generated path collision %q and %q", prior, cleaned)
		}
		for dir := path.Dir(cleaned); dir != "."; dir = path.Dir(dir) {
			if prior, ok := seen[dir]; ok {
				return nil, fmt.Errorf("generated path %q conflicts with directory needed by %q", prior, cleaned)
			}
			directories[dir] = cleaned
		}
		if child, ok := directories[cleaned]; ok {
			return nil, fmt.Errorf("generated path %q conflicts with directory needed by %q", cleaned, child)
		}
		file.Path = cleaned
		if file.Mode == 0 {
			file.Mode = 0o644
		}
		seen[cleaned] = cleaned
		seenFolded[folded] = cleaned
		normalized = append(normalized, file)
	}
	return normalized, nil
}

func normalizeGeneratedPath(raw string) (string, error) {
	if raw == "" {
		return "", fmt.Errorf("generated path must not be empty")
	}
	if strings.ContainsRune(raw, '\x00') {
		return "", fmt.Errorf("generated path contains null byte")
	}
	if filepath.IsAbs(raw) || path.IsAbs(raw) {
		return "", fmt.Errorf("generated path must be relative: %s", raw)
	}
	forward := strings.ReplaceAll(raw, "\\", "/")
	cleaned := path.Clean(forward)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("generated path escapes project directory: %s", raw)
	}
	for _, component := range strings.Split(cleaned, "/") {
		if err := validateFileComponent(component); err != nil {
			return "", fmt.Errorf("generated path component %q: %w", component, err)
		}
	}
	return cleaned, nil
}

func finalWrittenFiles(root string, names []string) []string {
	files := make([]string, 0, len(names))
	for _, name := range names {
		files = append(files, filepath.Join(root, name))
	}
	return files
}

func validateFileComponent(name string) error {
	normalized := norm.NFC.String(name)
	if normalized == "" || normalized == "." || normalized == ".." {
		return fmt.Errorf("design name must be a filename component")
	}
	if strings.HasSuffix(normalized, ".") || strings.HasSuffix(normalized, " ") {
		return fmt.Errorf("design name must not end with a space or period")
	}
	if filepath.Base(normalized) != normalized || strings.ContainsAny(normalized, `/\:*?"<>|`) {
		return fmt.Errorf("design name must not contain path separators")
	}
	if isWindowsReservedName(normalized) {
		return fmt.Errorf("design name must not be a reserved Windows filename")
	}
	return nil
}

func isWindowsReservedName(name string) bool {
	stem := name
	if dot := strings.IndexByte(stem, '.'); dot >= 0 {
		stem = stem[:dot]
	}
	upper := strings.ToUpper(stem)
	switch upper {
	case "CON", "PRN", "AUX", "NUL", "CLOCK$", "CONIN$", "CONOUT$":
		return true
	default:
		return len(upper) == 4 &&
			(upper[:3] == "COM" || upper[:3] == "LPT") &&
			upper[3] >= '1' && upper[3] <= '9'
	}
}

func syncDir(path string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

func writeFile(path string, mode os.FileMode, write func(io.Writer) error) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	if err := write(file); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		return errors.Join(err, file.Close())
	}
	return file.Close()
}

func writeSyncedFile(path string, data []byte, perm os.FileMode, exclusive bool) error {
	flags := os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	if exclusive {
		flags = os.O_WRONLY | os.O_CREATE | os.O_EXCL
	}
	file, err := os.OpenFile(path, flags, perm)
	if err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		return errors.Join(err, file.Close())
	}
	return file.Close()
}
