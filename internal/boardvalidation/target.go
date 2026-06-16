package boardvalidation

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const (
	kicadPCBExt     = ".kicad_pcb"
	kicadProjectExt = ".kicad_pro"
)

type Target struct {
	InputPath   string `json:"input_path"`
	BoardPath   string `json:"board_path"`
	ProjectPath string `json:"project_path,omitempty"`
	ProjectDir  string `json:"project_dir,omitempty"`
}

func ResolveTarget(path string) (Target, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return Target{}, targetError("target", errors.New("board validation target is required"))
	}
	absolute, err := filepath.Abs(trimmed)
	if err != nil {
		return Target{}, targetError(trimmed, err)
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return Target{}, targetError(absolute, err)
	}
	if !info.IsDir() {
		switch strings.ToLower(filepath.Ext(absolute)) {
		case kicadPCBExt:
			return Target{
				InputPath:  filepath.ToSlash(absolute),
				BoardPath:  filepath.ToSlash(absolute),
				ProjectDir: filepath.ToSlash(filepath.Dir(absolute)),
			}, nil
		case kicadProjectExt:
			return resolveProjectFile(absolute)
		default:
			return Target{}, targetError(absolute, errors.New("target must be a .kicad_pcb file, .kicad_pro file, or KiCad project directory"))
		}
	}
	return resolveProjectDirectory(absolute)
}

func resolveProjectFile(projectPath string) (Target, error) {
	dir := filepath.Dir(projectPath)
	boardFiles, err := filesWithExtension(dir, kicadPCBExt)
	if err != nil {
		return Target{}, err
	}
	boardPath, err := selectProjectBoard(projectPath, boardFiles)
	if err != nil {
		return Target{}, err
	}
	return Target{
		InputPath:   filepath.ToSlash(projectPath),
		BoardPath:   filepath.ToSlash(boardPath),
		ProjectPath: filepath.ToSlash(projectPath),
		ProjectDir:  filepath.ToSlash(dir),
	}, nil
}

func resolveProjectDirectory(dir string) (Target, error) {
	projectFiles, err := filesWithExtension(dir, kicadProjectExt)
	if err != nil {
		return Target{}, err
	}
	boardFiles, err := filesWithExtension(dir, kicadPCBExt)
	if err != nil {
		return Target{}, err
	}
	if len(projectFiles) == 0 {
		return Target{}, targetError(dir, errors.New("no .kicad_pro file found"))
	}
	if len(projectFiles) > 1 {
		return Target{}, targetError(dir, fmt.Errorf("multiple .kicad_pro files found: %s", strings.Join(baseNames(projectFiles), ", ")))
	}
	boardPath, err := selectProjectBoard(projectFiles[0], boardFiles)
	if err != nil {
		return Target{}, err
	}
	return Target{
		InputPath:   filepath.ToSlash(dir),
		BoardPath:   filepath.ToSlash(boardPath),
		ProjectPath: filepath.ToSlash(projectFiles[0]),
		ProjectDir:  filepath.ToSlash(dir),
	}, nil
}

func filesWithExtension(dir string, ext string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, targetError(dir, err)
	}
	var matches []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.ToLower(filepath.Ext(entry.Name())) == ext {
			matches = append(matches, filepath.Join(dir, entry.Name()))
		}
	}
	slices.Sort(matches)
	return matches, nil
}

func selectProjectBoard(projectPath string, boardFiles []string) (string, error) {
	if len(boardFiles) == 0 {
		return "", targetError(filepath.Dir(projectPath), errors.New("no .kicad_pcb file found"))
	}
	projectBase := strings.TrimSuffix(filepath.Base(projectPath), filepath.Ext(projectPath))
	for _, boardPath := range boardFiles {
		boardBase := strings.TrimSuffix(filepath.Base(boardPath), filepath.Ext(boardPath))
		if strings.EqualFold(boardBase, projectBase) {
			return boardPath, nil
		}
	}
	if len(boardFiles) == 1 {
		return boardFiles[0], nil
	}
	return "", targetError(filepath.Dir(projectPath), fmt.Errorf("ambiguous .kicad_pcb files: %s", strings.Join(baseNames(boardFiles), ", ")))
}

func baseNames(paths []string) []string {
	names := make([]string, len(paths))
	for index, path := range paths {
		names[index] = filepath.Base(path)
	}
	return names
}

func targetError(path string, err error) error {
	return fmt.Errorf("%s: %w", filepath.ToSlash(path), err)
}
