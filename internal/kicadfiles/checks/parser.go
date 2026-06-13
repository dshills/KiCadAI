package checks

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"
)

func ParseReportFile(kind CheckKind, path string) ([]CheckFinding, []ParserIssue, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, "", err
	}
	findings, issues, units := ParseReport(kind, data)
	return findings, issues, units, nil
}

func ParseReport(kind CheckKind, data []byte) ([]CheckFinding, []ParserIssue, string) {
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, []ParserIssue{{Message: err.Error(), Raw: string(data)}}, ""
	}
	units := stringValue(root["coordinate_units"])
	if units == "" {
		units = stringValue(root["units"])
	}
	if units == "" {
		units = "mm"
	}
	var findings []CheckFinding
	var issues []ParserIssue
	switch kind {
	case CheckKindERC:
		findings, issues = parseERCJSON(root, units)
	case CheckKindDRC:
		findings, issues = parseDRCJSON(root, units)
	default:
		issues = append(issues, ParserIssue{Message: "unsupported check kind: " + string(kind)})
	}
	findings = NormalizeFindings(kind, findings)
	return findings, issues, units
}

func parseERCJSON(root map[string]any, units string) ([]CheckFinding, []ParserIssue) {
	var findings []CheckFinding
	var issues []ParserIssue
	for _, sheet := range arrayValue(root["sheets"]) {
		sheetMap, ok := sheet.(map[string]any)
		if !ok {
			issues = append(issues, ParserIssue{Message: "ERC sheet is not an object"})
			continue
		}
		sheetPath := stringValue(sheetMap["path"])
		for _, violation := range arrayValue(sheetMap["violations"]) {
			finding, ok := parseViolationObject(CheckKindERC, violation, units)
			if !ok {
				issues = append(issues, ParserIssue{Message: "ERC violation is not an object", Raw: rawJSON(violation)})
				continue
			}
			finding.Sheet = sheetPath
			findings = append(findings, finding)
		}
	}
	if len(findings) == 0 {
		for _, violation := range arrayValue(root["violations"]) {
			finding, ok := parseViolationObject(CheckKindERC, violation, units)
			if !ok {
				issues = append(issues, ParserIssue{Message: "ERC violation is not an object", Raw: rawJSON(violation)})
				continue
			}
			findings = append(findings, finding)
		}
	}
	return findings, issues
}

func parseDRCJSON(root map[string]any, units string) ([]CheckFinding, []ParserIssue) {
	var findings []CheckFinding
	var issues []ParserIssue
	keys := []string{"violations", "drc_violations", "unconnected_items", "schematic_parity"}
	for _, key := range keys {
		for _, violation := range arrayValue(root[key]) {
			finding, ok := parseViolationObject(CheckKindDRC, violation, units)
			if !ok {
				issues = append(issues, ParserIssue{Message: "DRC violation is not an object", Raw: rawJSON(violation)})
				continue
			}
			findings = append(findings, finding)
		}
	}
	return findings, issues
}

func parseViolationObject(kind CheckKind, value any, units string) (CheckFinding, bool) {
	object, ok := value.(map[string]any)
	if !ok {
		return CheckFinding{}, false
	}
	finding := CheckFinding{
		Kind:           kind,
		Severity:       firstNonEmpty(stringValue(object["severity"]), "error"),
		Rule:           firstNonEmpty(stringValue(object["type"]), stringValue(object["rule"])),
		Code:           stringValue(object["code"]),
		Message:        firstNonEmpty(stringValue(object["description"]), stringValue(object["message"])),
		File:           firstNonEmpty(stringValue(object["file"]), stringValue(object["source"])),
		Layer:          stringValue(object["layer"]),
		Net:            stringValue(object["net"]),
		Raw:            rawJSON(value),
		RepairCategory: RepairUnknown,
	}
	if finding.Code == "" {
		finding.Code = finding.Rule
	}
	if finding.Message == "" {
		finding.Message = finding.Code
	}
	if nets := stringSlice(object["nets"]); len(nets) > 0 {
		finding.Nets = nets
	}
	for _, itemValue := range arrayValue(object["items"]) {
		item, ok := itemValue.(map[string]any)
		if !ok {
			continue
		}
		finding.Objects = append(finding.Objects, parseItemObject(item))
		if finding.Location == nil {
			if loc, ok := parseLocation(item["pos"], units); ok {
				finding.Location = &loc
			}
		}
		desc := stringValue(item["description"])
		if refs := referencesFromText(desc); len(refs) > 0 {
			finding.References = append(finding.References, refs...)
		}
		if pins := pinsFromText(desc); len(pins) > 0 {
			finding.Pins = append(finding.Pins, pins...)
		}
	}
	if loc, ok := parseLocation(object["pos"], units); ok && finding.Location == nil {
		finding.Location = &loc
	}
	if refs := stringSlice(object["references"]); len(refs) > 0 {
		finding.References = append(finding.References, refs...)
	}
	if refs := referencesFromText(finding.Message); len(refs) > 0 {
		finding.References = append(finding.References, refs...)
	}
	finding.References = uniqueStrings(finding.References)
	finding.Pins = uniqueStrings(finding.Pins)
	finding.Nets = uniqueStrings(finding.Nets)
	finding.RepairCategory = ClassifyRepairCategory(finding)
	return finding, true
}

func parseItemObject(item map[string]any) CheckObject {
	desc := stringValue(item["description"])
	object := CheckObject{
		Type:      objectTypeFromDescription(desc),
		ID:        stringValue(item["uuid"]),
		Reference: firstString(referencesFromText(desc)),
		Pad:       firstString(pinsFromText(desc)),
		Net:       stringValue(item["net"]),
		Layer:     stringValue(item["layer"]),
	}
	return object
}

func parseLocation(value any, units string) (CheckLocation, bool) {
	object, ok := value.(map[string]any)
	if !ok {
		return CheckLocation{}, false
	}
	x, okX := numberValue(object["x"])
	y, okY := numberValue(object["y"])
	if !okX || !okY {
		return CheckLocation{}, false
	}
	return CheckLocation{X: x, Y: y, Units: units}, true
}

var (
	referencePattern = regexp.MustCompile(`(?:^|[^A-Za-z0-9_#])(#?[A-Z]{1,5}[0-9]+[A-Za-z]?)\b`)
	pinPattern       = regexp.MustCompile(`\b[Pp]in\s+([A-Za-z0-9._+-]+)\b`)
)

func referencesFromText(text string) []string {
	matches := referencePattern.FindAllStringSubmatch(text, -1)
	var refs []string
	for _, match := range matches {
		if len(match) > 1 {
			refs = append(refs, match[1])
		}
	}
	return refs
}

func pinsFromText(text string) []string {
	matches := pinPattern.FindAllStringSubmatch(text, -1)
	var pins []string
	for _, match := range matches {
		if len(match) > 1 {
			pins = append(pins, match[1])
		}
	}
	return pins
}

func objectTypeFromDescription(desc string) string {
	desc = strings.TrimSpace(desc)
	if desc == "" {
		return ""
	}
	fields := strings.Fields(desc)
	if len(fields) == 0 {
		return ""
	}
	return strings.Trim(fields[0], ",:")
}

func rawJSON(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(data)
}

func arrayValue(value any) []any {
	if values, ok := value.([]any); ok {
		return values
	}
	return nil
}

func stringValue(value any) string {
	if s, ok := value.(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}

func stringSlice(value any) []string {
	values, ok := value.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if s := stringValue(value); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func numberValue(value any) (float64, bool) {
	n, ok := value.(float64)
	return n, ok
}

func firstString(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func uniqueStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := normalizeKey(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	return out
}
