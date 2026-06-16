package design

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"kicadai/internal/kicadfiles/pcb"
	"kicadai/internal/kicadfiles/project"
	"kicadai/internal/kicadfiles/schematic"
)

func ReadProjectDirectory(root string) (Design, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return Design{}, err
	}
	var projectPath string
	var projectBase string
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".kicad_pro" {
			if projectPath != "" {
				return Design{}, errors.New("project directory contains multiple .kicad_pro files")
			}
			projectPath = filepath.Join(root, entry.Name())
			projectBase = strings.TrimSuffix(entry.Name(), ".kicad_pro")
		}
	}
	if projectPath == "" {
		return Design{}, errors.New("project directory does not contain a .kicad_pro file")
	}
	projectFile, err := project.ReadFile(projectPath)
	if err != nil {
		return Design{}, err
	}
	if projectFile.Name == "" {
		projectFile.Name = projectBase
	}
	design := Design{Name: projectFile.Name, Project: projectFile}
	schematicPath := filepath.Join(root, projectBase+".kicad_sch")
	schematicFile, err := schematic.ReadFile(schematicPath)
	if err == nil {
		design.Schematic = &schematicFile
		sheetFiles, err := readSheetFiles(root, &schematicFile)
		if err != nil {
			return Design{}, err
		}
		design.SheetFiles = sheetFiles
	} else if !errors.Is(err, os.ErrNotExist) {
		return Design{}, err
	}
	pcbPath := filepath.Join(root, projectBase+".kicad_pcb")
	pcbFile, err := pcb.ReadFile(pcbPath)
	if err == nil {
		design.PCB = &pcbFile
	} else if !errors.Is(err, os.ErrNotExist) {
		return Design{}, err
	}
	return design, nil
}

func readSheetFiles(root string, rootSchematic *schematic.SchematicFile) ([]*schematic.SchematicFile, error) {
	var files []*schematic.SchematicFile
	rootName := projectRelativeSchematicPath(root, rootSchematic.Filename)
	if rootName == "." || rootName == "" {
		rootName = filepath.ToSlash(filepath.Clean(filepath.Base(rootSchematic.Filename)))
	}
	seen := map[string]struct{}{rootName: {}}
	var visit func(parentPath string, file *schematic.SchematicFile) error
	visit = func(parentPath string, file *schematic.SchematicFile) error {
		for _, sheet := range file.Sheets {
			name := strings.TrimSpace(sheet.Filename)
			if name == "" {
				continue
			}
			childPath, err := ResolveSheetPath(parentPath, name)
			if err != nil {
				return fmt.Errorf("sheet file path must be project-relative: %s: %w", name, err)
			}
			if _, ok := seen[childPath]; ok {
				continue
			}
			seen[childPath] = struct{}{}
			child, err := schematic.ReadFile(filepath.Join(root, filepath.FromSlash(childPath)))
			if err != nil {
				if errors.Is(err, os.ErrNotExist) {
					return fmt.Errorf("sheet file not found: %s", childPath)
				}
				return err
			}
			child.Filename = childPath
			files = append(files, &child)
			if err := visit(childPath, &child); err != nil {
				return err
			}
		}
		return nil
	}
	if err := visit(rootName, rootSchematic); err != nil {
		return nil, err
	}
	return files, nil
}

func projectRelativeSchematicPath(root string, filename string) string {
	if filepath.IsAbs(filename) {
		if rel, err := filepath.Rel(root, filename); err == nil {
			return filepath.ToSlash(filepath.Clean(rel))
		}
	}
	return filepath.ToSlash(filepath.Clean(filename))
}

func ResolveSheetPath(parentPath string, sheetFilename string) (string, error) {
	parentClean := filepath.Clean(filepath.FromSlash(parentPath))
	parentSlash := filepath.ToSlash(parentClean)
	if filepath.IsAbs(parentClean) || parentSlash == ".." || strings.HasPrefix(parentSlash, "../") {
		return "", errors.New("parent sheet path must be project-relative")
	}
	cleanName := filepath.Clean(filepath.FromSlash(sheetFilename))
	if filepath.IsAbs(cleanName) {
		return "", errors.New("absolute sheet path")
	}
	parentDir := filepath.Dir(parentClean)
	if parentDir == "." {
		parentDir = ""
	}
	clean := filepath.Clean(filepath.Join(parentDir, cleanName))
	slashClean := filepath.ToSlash(clean)
	if slashClean == "." || slashClean == ".." || strings.HasPrefix(slashClean, "../") {
		return "", errors.New("sheet path escapes project")
	}
	return slashClean, nil
}
