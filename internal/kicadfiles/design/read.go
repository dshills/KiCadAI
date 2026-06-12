package design

import (
	"errors"
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
