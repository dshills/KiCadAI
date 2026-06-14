package project

import (
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/text/unicode/norm"
	"kicadai/internal/kicadfiles"
)

var (
	namePattern     = regexp.MustCompile(`^[\p{L}\p{N}_]([\p{L}\p{N}._ -]*[\p{L}\p{N}_])?$`)
	variablePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	reservedPattern = regexp.MustCompile(`(?i)^(CON|PRN|AUX|NUL|COM[1-9]|LPT[1-9]|CLOCK\$|CONIN\$|CONOUT\$)(\..*)?$`)
)

type ProjectFile struct {
	Name              string
	DesignID          kicadfiles.UUID
	FormatVersion     kicadfiles.KiCadFormatVersion
	Generator         string
	PageSettings      PageSettings
	NetClasses        []NetClass
	Sheets            []Sheet
	TextVariables     map[string]string
	Preserved         map[string]json.RawMessage
	PreservedSections map[string]map[string]json.RawMessage
}

type PageSettings struct {
	Paper kicadfiles.Paper
}

type NetClass struct {
	Name        string
	Clearance   kicadfiles.IU
	TrackWidth  kicadfiles.IU
	ViaDiameter kicadfiles.IU
	ViaDrill    kicadfiles.IU
}

type Sheet struct {
	UUID string
	Name string
}

func Validate(project ProjectFile) error {
	var errs kicadfiles.ValidationErrors
	name := norm.NFC.String(strings.TrimSpace(project.Name))
	if name == "" {
		errs = append(errs, fieldError("name", "required"))
	} else {
		if len(name) > 128 {
			errs = append(errs, fieldError("name", "must be at most 128 UTF-8 bytes"))
		}
		if !namePattern.MatchString(name) {
			errs = append(errs, fieldError("name", "contains unsupported filename characters"))
		}
		if reservedPattern.MatchString(name) {
			errs = append(errs, fieldError("name", "reserved Windows device filename"))
		}
	}
	if !project.DesignID.Valid() {
		errs = append(errs, fieldError("design_id", "valid UUID required"))
	}
	if project.FormatVersion == "" {
		errs = append(errs, fieldError("format_version", "required"))
	} else if _, err := strconv.Atoi(string(project.FormatVersion)); err != nil {
		errs = append(errs, fieldError("format_version", "must be numeric"))
	}
	if strings.TrimSpace(project.Generator) == "" {
		errs = append(errs, fieldError("generator", "required"))
	}
	if strings.TrimSpace(project.PageSettings.Paper.Name) == "" {
		errs = append(errs, fieldError("page_settings.paper", "required"))
	}
	if project.PageSettings.Paper.Width < 0 {
		errs = append(errs, fieldError("page_settings.width", "must be non-negative"))
	}
	if project.PageSettings.Paper.Height < 0 {
		errs = append(errs, fieldError("page_settings.height", "must be non-negative"))
	}
	seen := map[string]struct{}{}
	hasDefault := false
	for i, class := range project.NetClasses {
		className := strings.TrimSpace(class.Name)
		if className == "" {
			errs = append(errs, fieldError(fmt.Sprintf("net_classes[%d].name", i), "required"))
			continue
		}
		if _, ok := seen[className]; ok {
			errs = append(errs, fieldError(fmt.Sprintf("net_classes[%d].name", i), "duplicate "+className))
		}
		seen[className] = struct{}{}
		if className == "Default" {
			hasDefault = true
		}
		if class.Clearance < 0 {
			errs = append(errs, fieldError(fmt.Sprintf("net_classes[%d].clearance", i), "must be non-negative"))
		}
		if class.TrackWidth <= 0 {
			errs = append(errs, fieldError(fmt.Sprintf("net_classes[%d].track_width", i), "must be positive"))
		}
		if class.ViaDiameter <= 0 {
			errs = append(errs, fieldError(fmt.Sprintf("net_classes[%d].via_diameter", i), "must be positive"))
		}
		if class.ViaDrill <= 0 {
			errs = append(errs, fieldError(fmt.Sprintf("net_classes[%d].via_drill", i), "must be positive"))
		}
		if class.ViaDiameter > 0 && class.ViaDrill >= class.ViaDiameter {
			errs = append(errs, fieldError(fmt.Sprintf("net_classes[%d].via_drill", i), "must be less than via diameter"))
		}
	}
	if !hasDefault {
		errs = append(errs, fieldError("net_classes", "Default net class required"))
	}
	for key := range project.TextVariables {
		if !variablePattern.MatchString(key) {
			errs = append(errs, fieldError("text_variables."+key, "invalid key"))
		}
	}
	errs = append(errs, validatePreservedProjectJSON(project)...)
	return errs.Err()
}

func Write(w io.Writer, project ProjectFile) error {
	if err := Validate(project); err != nil {
		return err
	}
	document := newDocument(project)
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(document)
}

func validatePreservedProjectJSON(project ProjectFile) kicadfiles.ValidationErrors {
	var errs kicadfiles.ValidationErrors
	for key, raw := range project.Preserved {
		trimmed := strings.TrimSpace(key)
		if trimmed == "" {
			errs = append(errs, fieldError("preserved", "top-level key required"))
			continue
		}
		if trimmed != key {
			errs = append(errs, fieldError("preserved."+key, "trimmed key required"))
		}
		if _, modeled := modeledProjectKeys()[key]; modeled {
			errs = append(errs, fieldError("preserved."+key, "must not replace modeled project key"))
		}
		if err := validateRawJSON(raw); err != nil {
			errs = append(errs, fieldError("preserved."+key, err.Error()))
		}
	}
	for sectionName, section := range project.PreservedSections {
		if _, ok := preservableProjectSections()[sectionName]; !ok {
			errs = append(errs, fieldError("preserved_sections."+sectionName, "unknown preservable section"))
			continue
		}
		for key, raw := range section {
			trimmed := strings.TrimSpace(key)
			if trimmed == "" {
				errs = append(errs, fieldError("preserved_sections."+sectionName, "section key required"))
				continue
			}
			if trimmed != key {
				errs = append(errs, fieldError("preserved_sections."+sectionName+"."+key, "trimmed key required"))
			}
			if _, modeled := modeledProjectSectionKeys(sectionName)[key]; modeled {
				errs = append(errs, fieldError("preserved_sections."+sectionName+"."+key, "must not replace modeled section key"))
			}
			if err := validateRawJSON(raw); err != nil {
				errs = append(errs, fieldError("preserved_sections."+sectionName+"."+key, err.Error()))
			}
		}
	}
	return errs
}

func validateRawJSON(raw json.RawMessage) error {
	if len(raw) == 0 {
		return fmt.Errorf("raw JSON required")
	}
	if !json.Valid(raw) {
		return fmt.Errorf("invalid raw JSON")
	}
	return nil
}

func modeledProjectKeys() map[string]struct{} {
	return map[string]struct{}{
		"board":                    {},
		"boards":                   {},
		"component_class_settings": {},
		"cvpcb":                    {},
		"erc":                      {},
		"libraries":                {},
		"meta":                     {},
		"net_settings":             {},
		"pcbnew":                   {},
		"schematic":                {},
		"sheets":                   {},
		"text_variables":           {},
		"time_domain_parameters":   {},
	}
}

func preservableProjectSections() map[string]struct{} {
	return map[string]struct{}{
		"board":                    {},
		"component_class_settings": {},
		"cvpcb":                    {},
		"erc":                      {},
		"libraries":                {},
		"net_settings":             {},
		"pcbnew":                   {},
		"schematic":                {},
		"time_domain_parameters":   {},
	}
}

func modeledProjectSectionKeys(section string) map[string]struct{} {
	switch section {
	case "board":
		return map[string]struct{}{"design_settings": {}}
	case "net_settings":
		return map[string]struct{}{"classes": {}}
	default:
		return map[string]struct{}{}
	}
}

type meta struct {
	Version int `json:"version"`
}

type pageSettings struct {
	Paper  string  `json:"paper"`
	Width  float64 `json:"width,omitempty"`
	Height float64 `json:"height,omitempty"`
}

type netSettings struct {
	Classes []netClass `json:"classes,omitempty"`
}

type netClass struct {
	Name        string  `json:"name"`
	Clearance   float64 `json:"clearance"`
	TrackWidth  float64 `json:"track_width"`
	ViaDiameter float64 `json:"via_diameter"`
	ViaDrill    float64 `json:"via_drill"`
}

type sheet []string

func newDocument(project ProjectFile) map[string]any {
	classes := make([]netClass, 0, len(project.NetClasses))
	for _, class := range project.NetClasses {
		classes = append(classes, netClass{
			Name:        strings.TrimSpace(class.Name),
			Clearance:   mmNumber(class.Clearance),
			TrackWidth:  mmNumber(class.TrackWidth),
			ViaDiameter: mmNumber(class.ViaDiameter),
			ViaDrill:    mmNumber(class.ViaDrill),
		})
	}

	document := map[string]any{}
	for key, raw := range project.Preserved {
		document[key] = raw
	}
	document["board"] = map[string]any{"design_settings": map[string]any{}}
	document["boards"] = []string{}
	document["component_class_settings"] = map[string]any{}
	document["cvpcb"] = map[string]any{}
	document["erc"] = map[string]any{}
	document["libraries"] = map[string]any{}
	document["meta"] = meta{Version: 1}
	document["net_settings"] = map[string]any{"classes": classes}
	document["pcbnew"] = map[string]any{}
	document["schematic"] = map[string]any{}
	document["sheets"] = renderSheets(project.Sheets)
	document["text_variables"] = textVariables(project.TextVariables)
	document["time_domain_parameters"] = map[string]any{}
	for sectionName, section := range project.PreservedSections {
		target, ok := document[sectionName].(map[string]any)
		if !ok {
			continue
		}
		for key, raw := range section {
			target[key] = raw
		}
	}
	return document
}

func renderSheets(projectSheets []Sheet) []sheet {
	sheets := make([]sheet, 0, len(projectSheets))
	for _, projectSheet := range projectSheets {
		sheets = append(sheets, sheet{projectSheet.UUID, projectSheet.Name})
	}
	return sheets
}

func textVariables(values map[string]string) map[string]string {
	if values == nil {
		return map[string]string{}
	}
	return values
}

func mmNumber(value kicadfiles.IU) float64 {
	return float64(value) / 1_000_000
}

func fieldError(field, message string) kicadfiles.ValidationError {
	return kicadfiles.ValidationError{Section: "project", Field: field, Message: message}
}
