package project

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"kicadai/internal/kicadfiles"
)

func ReadFile(path string) (ProjectFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ProjectFile{}, err
	}
	project, err := Read(data)
	if err != nil {
		return ProjectFile{}, fmt.Errorf("%s: %w", path, err)
	}
	if project.Name == "" {
		project.Name = trimProjectExtension(filepath.Base(path))
	}
	return project, nil
}

func Read(data []byte) (ProjectFile, error) {
	var document map[string]json.RawMessage
	if err := json.Unmarshal(data, &document); err != nil {
		return ProjectFile{}, err
	}
	project := ProjectFile{
		Generator:         "kicadai",
		PageSettings:      PageSettings{Paper: kicadfiles.Paper{Name: "A4"}},
		TextVariables:     map[string]string{},
		Preserved:         map[string]json.RawMessage{},
		PreservedSections: map[string]map[string]json.RawMessage{},
	}
	if raw, ok := document["sheets"]; ok {
		var sheets []sheet
		if err := json.Unmarshal(raw, &sheets); err != nil {
			return ProjectFile{}, fmt.Errorf("sheets: %w", err)
		}
		for _, value := range sheets {
			if len(value) >= 2 {
				project.Sheets = append(project.Sheets, Sheet{UUID: value[0], Name: value[1]})
			}
		}
	}
	if raw, ok := document["text_variables"]; ok {
		if err := json.Unmarshal(raw, &project.TextVariables); err != nil {
			return ProjectFile{}, fmt.Errorf("text_variables: %w", err)
		}
	}
	if raw, ok := document["net_settings"]; ok {
		var settings netSettings
		if err := json.Unmarshal(raw, &settings); err != nil {
			return ProjectFile{}, fmt.Errorf("net_settings: %w", err)
		}
		for _, class := range settings.Classes {
			project.NetClasses = append(project.NetClasses, NetClass{
				Name:        class.Name,
				Clearance:   kicadfiles.MM(class.Clearance),
				TrackWidth:  kicadfiles.MM(class.TrackWidth),
				ViaDiameter: kicadfiles.MM(class.ViaDiameter),
				ViaDrill:    kicadfiles.MM(class.ViaDrill),
			})
		}
	}
	for key, raw := range document {
		if _, modeled := modeledProjectKeys()[key]; !modeled {
			project.Preserved[key] = append(json.RawMessage(nil), raw...)
		}
	}
	return project, nil
}

func trimProjectExtension(name string) string {
	if filepath.Ext(name) == ".kicad_pro" {
		return name[:len(name)-len(".kicad_pro")]
	}
	return name
}
