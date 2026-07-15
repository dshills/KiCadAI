package design

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"

	"golang.org/x/text/unicode/norm"
	"kicadai/internal/kicadfiles/library"
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
	commitResult, err := CommitPreparedDirectory(target, tempProjectDir, opts.Overwrite)
	if err != nil {
		return result, err
	}
	result.Warnings = append(result.Warnings, commitResult.Warnings...)
	result.WrittenFiles = finalWrittenFiles(target, writtenNames)
	cleanupTemp = false
	_ = os.RemoveAll(tempDir)
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
	projectFile := design.Project
	if len(projectFile.Sheets) == 0 {
		projectFile.Sheets = projectSheets(design)
	}
	files := []generatedFile{{
		Path: name + ".kicad_pro",
		Mode: 0o644,
		Write: func(w io.Writer) error {
			return project.Write(w, projectFile)
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
	for _, sheetFile := range design.SheetFiles {
		if sheetFile == nil {
			continue
		}
		file := sheetFile
		childPath, err := normalizeGeneratedPath(strings.TrimSpace(file.Filename))
		if err != nil {
			return nil, err
		}
		files = append(files, generatedFile{
			Path: childPath,
			Mode: 0o644,
			Write: func(w io.Writer) error {
				return schematic.Write(w, *file)
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
	if len(design.SymbolTables) > 0 {
		files = append(files, generatedFile{
			Path: "sym-lib-table",
			Mode: 0o644,
			Write: func(w io.Writer) error {
				return library.WriteSymbolLibraryTable(w, design.SymbolTables)
			},
		})
	}
	if len(design.FootprintTables) > 0 {
		files = append(files, generatedFile{
			Path: "fp-lib-table",
			Mode: 0o644,
			Write: func(w io.Writer) error {
				return library.WriteFootprintLibraryTable(w, design.FootprintTables)
			},
		})
	}
	artifactEntries, err := artifactFiles(design.RuleFiles, design.WorksheetFiles, design.AssetFiles)
	if err != nil {
		return nil, err
	}
	files = append(files, artifactEntries...)
	return validateGeneratedFiles(files)
}

func artifactFiles(groups ...[]TextArtifact) ([]generatedFile, error) {
	var files []generatedFile
	for _, artifacts := range groups {
		for _, artifact := range artifacts {
			cleaned, err := normalizeGeneratedPath(artifact.Path)
			if err != nil {
				return nil, err
			}
			contents := append([]byte(nil), artifact.Contents...)
			files = append(files, generatedFile{
				Path: cleaned,
				Mode: 0o644,
				Write: func(w io.Writer) error {
					_, err := io.Copy(w, bytes.NewReader(contents))
					return err
				},
			})
		}
	}
	return files, nil
}

func projectSheets(design Design) []project.Sheet {
	var sheets []project.Sheet
	if design.Schematic != nil {
		sheets = append(sheets, project.Sheet{UUID: string(design.Schematic.UUID), Name: "Root"})
	}
	for _, sheetFile := range design.SheetFiles {
		if sheetFile != nil {
			name := strings.TrimSuffix(path.Base(strings.TrimSpace(sheetFile.Filename)), ".kicad_sch")
			sheets = append(sheets, project.Sheet{UUID: string(sheetFile.UUID), Name: name})
		}
	}
	return sheets
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
