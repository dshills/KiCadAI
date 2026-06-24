package verification

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"

	"kicadai/internal/blocks"
	"kicadai/internal/components"
	"kicadai/internal/reports"
)

type EvidenceLevel string

const (
	EvidenceDefinitionOnly    EvidenceLevel = "definition_only"
	EvidenceSchematicVerified EvidenceLevel = "schematic_verified"
	EvidenceTransferVerified  EvidenceLevel = "transfer_verified"
	EvidencePCBVerified       EvidenceLevel = "pcb_verified"
	EvidenceERCDRCVerified    EvidenceLevel = "erc_drc_verified"
	EvidenceReferenceVerified EvidenceLevel = "reference_verified"
)

type Manifest struct {
	ID          string      `json:"id"`
	BlockID     string      `json:"block_id"`
	Description string      `json:"description,omitempty"`
	Acceptance  string      `json:"acceptance,omitempty"`
	Request     RequestSpec `json:"request"`
	Expected    Expected    `json:"expected"`
}

type RequestSpec struct {
	InstanceID string         `json:"instance_id,omitempty"`
	Params     map[string]any `json:"params,omitempty"`
}

type Expected struct {
	EvidenceLevel EvidenceLevel       `json:"evidence_level"`
	References    []string            `json:"references,omitempty"`
	Components    []ExpectedComponent `json:"components,omitempty"`
	Ports         []ExpectedPort      `json:"ports,omitempty"`
	Nets          []ExpectedNet       `json:"nets,omitempty"`
	PCB           ExpectedPCB         `json:"pcb,omitempty"`
	Writer        ExpectedWriter      `json:"writer,omitempty"`
	ERCDRC        ExpectedERCDRC      `json:"erc_drc,omitempty"`
	Strict        bool                `json:"strict,omitempty"`
}

type ExpectedComponent struct {
	Role        string `json:"role"`
	Ref         string `json:"ref,omitempty"`
	RefPrefix   string `json:"ref_prefix,omitempty"`
	SymbolID    string `json:"symbol_id,omitempty"`
	FootprintID string `json:"footprint_id,omitempty"`
	Value       string `json:"value,omitempty"`
}

type ExpectedPort struct {
	Name      string `json:"name"`
	Direction string `json:"direction,omitempty"`
	Net       string `json:"net,omitempty"`
	Optional  bool   `json:"optional,omitempty"`
}

type ExpectedNet struct {
	Name       string        `json:"name"`
	Visibility string        `json:"visibility,omitempty"`
	Pins       []ExpectedPin `json:"pins,omitempty"`
}

type ExpectedPin struct {
	Role string `json:"role,omitempty"`
	Ref  string `json:"ref,omitempty"`
	Pin  string `json:"pin"`
}

type ExpectedPCB struct {
	Placements             []ExpectedPlacement     `json:"placements,omitempty"`
	RequiredRoutes         []string                `json:"required_routes,omitempty"`
	RequiredLocalRoutes    []string                `json:"required_local_routes,omitempty"`
	RequiredZones          []string                `json:"required_zones,omitempty"`
	TimingFixtures         []ExpectedTimingFixture `json:"timing_fixtures,omitempty"`
	PadNets                []ExpectedPadNet        `json:"pad_nets,omitempty"`
	AllowUnrouted          bool                    `json:"allow_unrouted,omitempty"`
	RequireRoutes          bool                    `json:"require_routes,omitempty"`
	RequireZones           bool                    `json:"require_zones,omitempty"`
	RequireRealization     bool                    `json:"require_realization,omitempty"`
	RequireBoardValidation bool                    `json:"require_board_validation,omitempty"`
}

type ExpectedPlacement struct {
	Ref          string   `json:"ref,omitempty"`
	Role         string   `json:"role,omitempty"`
	FootprintID  string   `json:"footprint_id,omitempty"`
	XMM          *float64 `json:"x_mm,omitempty"`
	YMM          *float64 `json:"y_mm,omitempty"`
	RotationDeg  *float64 `json:"rotation_deg,omitempty"`
	ToleranceMM  *float64 `json:"tolerance_mm,omitempty"`
	ToleranceDeg *float64 `json:"tolerance_deg,omitempty"`
}

type ExpectedPadNet struct {
	Ref string `json:"ref"`
	Pad string `json:"pad"`
	Net string `json:"net"`
}

type ExpectedTimingFixture struct {
	ID                string   `json:"id"`
	Satisfied         *bool    `json:"satisfied,omitempty"`
	RequiredFindings  []string `json:"required_findings,omitempty"`
	ForbiddenFindings []string `json:"forbidden_findings,omitempty"`
}

type ExpectedWriter struct {
	Required         bool `json:"required,omitempty"`
	OK               bool `json:"ok,omitempty"`
	AllowUnrouted    bool `json:"allow_unrouted,omitempty"`
	RequireRoundTrip bool `json:"require_round_trip,omitempty"`
}

type ExpectedERCDRC struct {
	Required       bool     `json:"required,omitempty"`
	RequireERC     bool     `json:"require_erc,omitempty"`
	RequireDRC     bool     `json:"require_drc,omitempty"`
	AllowedCodes   []string `json:"allowed_codes,omitempty"`
	ExpectedIssues []string `json:"expected_issues,omitempty"`
}

var manifestIDPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

func LoadManifest(path string) (Manifest, []reports.Issue) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, []reports.Issue{issue(reports.CodeMissingFile, reports.SeverityError, "verification.manifest", err.Error())}
	}
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return Manifest{}, []reports.Issue{issue(reports.CodeValidationFailed, reports.SeverityError, "verification.manifest", err.Error())}
	}
	return manifest, nil
}

func ValidateManifest(manifest Manifest, registry blocks.Registry) []reports.Issue {
	var issues []reports.Issue
	basePath := "verification." + pathID(manifest.ID)
	if registry == nil {
		return []reports.Issue{issue(reports.CodeInvalidArgument, reports.SeverityError, basePath+".registry", "block registry is required")}
	}
	if strings.TrimSpace(manifest.ID) == "" {
		issues = append(issues, issue(reports.CodeValidationFailed, reports.SeverityError, basePath+".id", "verification case ID is required"))
	} else if strings.TrimSpace(manifest.ID) != manifest.ID || !manifestIDPattern.MatchString(manifest.ID) {
		issues = append(issues, issue(reports.CodeValidationFailed, reports.SeverityError, basePath+".id", "verification case ID must start with a lowercase letter and contain only lowercase letters, digits, and underscores"))
	}
	if acceptance := components.AcceptanceLevel(strings.TrimSpace(manifest.Acceptance)); acceptance != "" && !components.ValidAcceptance(acceptance) {
		issues = append(issues, issue(reports.CodeValidationFailed, reports.SeverityError, basePath+".acceptance", "unsupported verification acceptance "+manifest.Acceptance))
	}
	if strings.TrimSpace(manifest.BlockID) == "" {
		issues = append(issues, issue(reports.CodeValidationFailed, reports.SeverityError, basePath+".block_id", "block ID is required"))
	} else {
		if _, ok := registry.GetBlock(manifest.BlockID); !ok {
			issues = append(issues, issue(reports.CodeMissingFile, reports.SeverityError, basePath+".block_id", "unknown block ID "+manifest.BlockID))
		} else {
			request := blocks.BlockRequest{BlockID: manifest.BlockID, InstanceID: requestInstanceID(manifest), Params: manifest.Request.Params}
			for _, requestIssue := range registry.ValidateRequest(request) {
				requestIssue.Path = basePath + ".request." + strings.TrimPrefix(requestIssue.Path, "block.")
				issues = append(issues, requestIssue)
			}
		}
	}
	issues = append(issues, validateEvidenceLevel(basePath+".expected.evidence_level", manifest.Expected.EvidenceLevel)...)
	issues = append(issues, validateUniqueStrings(basePath+".expected.references", "duplicate expected reference", manifest.Expected.References)...)
	componentRefs, componentRoles, componentRefPrefixes, roleToRefs, componentIssues := validateExpectedComponents(basePath+".expected.components", manifest.Expected.Components, manifest.Expected.EvidenceLevel)
	issues = append(issues, componentIssues...)
	expectedNetNames := expectedNetNameSet(manifest.Expected.Nets)
	issues = append(issues, validateExpectedReferences(basePath+".expected.references", manifest.Expected.References, componentRefs, componentRefPrefixes)...)
	issues = append(issues, validateExpectedPorts(basePath+".expected.ports", manifest.Expected.Ports, expectedNetNames)...)
	issues = append(issues, validateExpectedNets(basePath+".expected.nets", manifest.Expected.Nets, componentRefs, componentRoles)...)
	issues = append(issues, validateExpectedPCB(basePath+".expected.pcb", manifest.Expected.PCB, componentRefs, componentRoles, roleToRefs, expectedNetNames)...)
	issues = append(issues, validateExpectedWriter(basePath+".expected.writer", manifest.Expected.Writer)...)
	issues = append(issues, validateExpectedERCDRC(basePath+".expected.erc_drc", manifest.Expected.ERCDRC)...)
	return issues
}

func requestInstanceID(manifest Manifest) string {
	if strings.TrimSpace(manifest.Request.InstanceID) != "" {
		return manifest.Request.InstanceID
	}
	return manifest.ID
}

func validateEvidenceLevel(path string, level EvidenceLevel) []reports.Issue {
	switch level {
	case EvidenceDefinitionOnly, EvidenceSchematicVerified, EvidenceTransferVerified, EvidencePCBVerified, EvidenceERCDRCVerified, EvidenceReferenceVerified:
		return nil
	case "":
		return []reports.Issue{issue(reports.CodeValidationFailed, reports.SeverityError, path, "expected evidence level is required")}
	default:
		return []reports.Issue{issue(reports.CodeValidationFailed, reports.SeverityError, path, "unsupported evidence level "+string(level))}
	}
}

func validateExpectedComponents(path string, components []ExpectedComponent, evidenceLevel EvidenceLevel) (map[string]struct{}, map[string]struct{}, map[string]struct{}, map[string][]string, []reports.Issue) {
	var issues []reports.Issue
	roles := map[string]struct{}{}
	refs := map[string]struct{}{}
	refPrefixes := map[string]struct{}{}
	roleToRefs := map[string][]string{}
	for index, component := range components {
		componentPath := fmt.Sprintf("%s.%d", path, index)
		role := strings.TrimSpace(component.Role)
		if role == "" {
			issues = append(issues, issue(reports.CodeValidationFailed, reports.SeverityError, componentPath+".role", "expected component role is required"))
		}
		if role != "" {
			roles[role] = struct{}{}
		}
		ref := strings.TrimSpace(component.Ref)
		if ref != "" {
			if _, exists := refs[ref]; exists {
				issues = append(issues, issue(reports.CodeValidationFailed, reports.SeverityError, componentPath+".ref", "duplicate expected component ref "+ref))
			}
			refs[ref] = struct{}{}
			if role != "" {
				roleToRefs[role] = append(roleToRefs[role], ref)
			}
		}
		if ref == "" && strings.TrimSpace(component.RefPrefix) == "" {
			issues = append(issues, issue(reports.CodeValidationFailed, reports.SeverityError, componentPath+".ref", "expected component requires ref or ref_prefix"))
		}
		if prefix := strings.TrimSpace(component.RefPrefix); prefix != "" {
			refPrefixes[prefix] = struct{}{}
		}
		if strings.TrimSpace(component.SymbolID) == "" {
			issues = append(issues, issue(reports.CodeValidationFailed, reports.SeverityError, componentPath+".symbol_id", "expected component symbol ID is required"))
		}
		if evidenceRequiresFootprint(evidenceLevel) && strings.TrimSpace(component.FootprintID) == "" {
			issues = append(issues, issue(reports.CodeValidationFailed, reports.SeverityError, componentPath+".footprint_id", "expected component footprint ID is required for "+string(evidenceLevel)))
		}
	}
	return refs, roles, refPrefixes, roleToRefs, issues
}

func validateExpectedReferences(path string, references []string, componentRefs map[string]struct{}, componentRefPrefixes map[string]struct{}) []reports.Issue {
	if len(references) == 0 {
		return nil
	}
	var issues []reports.Issue
	for _, ref := range references {
		trimmed := strings.TrimSpace(ref)
		if trimmed == "" {
			continue
		}
		if _, ok := componentRefs[trimmed]; ok {
			continue
		}
		if referenceMatchesPrefix(trimmed, componentRefPrefixes) {
			continue
		}
		issues = append(issues, issue(reports.CodeValidationFailed, reports.SeverityError, path+"."+pathSegment(trimmed), "expected reference has no matching component expectation "+trimmed))
	}
	return issues
}

func validateExpectedPorts(path string, ports []ExpectedPort, expectedNetNames map[string]struct{}) []reports.Issue {
	var issues []reports.Issue
	names := map[string]struct{}{}
	for index, port := range ports {
		portPath := fmt.Sprintf("%s.%d", path, index)
		name := strings.TrimSpace(port.Name)
		if name == "" {
			issues = append(issues, issue(reports.CodeValidationFailed, reports.SeverityError, portPath+".name", "expected port name is required"))
		} else if _, exists := names[name]; exists {
			issues = append(issues, issue(reports.CodeValidationFailed, reports.SeverityError, portPath+".name", "duplicate expected port "+name))
		}
		names[name] = struct{}{}
		switch strings.TrimSpace(port.Direction) {
		case "", string(blocks.PortInput), string(blocks.PortOutput), string(blocks.PortBidirectional), string(blocks.PortPassive), string(blocks.PortPower):
		default:
			issues = append(issues, issue(reports.CodeValidationFailed, reports.SeverityError, portPath+".direction", "unsupported expected port direction "+port.Direction))
		}
		net := strings.TrimSpace(port.Net)
		if net != "" {
			if _, ok := expectedNetNames[net]; !ok {
				issues = append(issues, issue(reports.CodeValidationFailed, reports.SeverityError, portPath+".net", "expected port references unknown net "+net))
			}
		}
	}
	return issues
}

func validateExpectedNets(path string, nets []ExpectedNet, componentRefs map[string]struct{}, componentRoles map[string]struct{}) []reports.Issue {
	var issues []reports.Issue
	names := map[string]struct{}{}
	for index, net := range nets {
		netPath := fmt.Sprintf("%s.%d", path, index)
		name := strings.TrimSpace(net.Name)
		if name == "" {
			issues = append(issues, issue(reports.CodeValidationFailed, reports.SeverityError, netPath+".name", "expected net name is required"))
		} else if _, exists := names[name]; exists {
			issues = append(issues, issue(reports.CodeValidationFailed, reports.SeverityError, netPath+".name", "duplicate expected net "+name))
		}
		names[name] = struct{}{}
		visibility := strings.TrimSpace(net.Visibility)
		if len(net.Pins) < 2 && visibility != "exported" && visibility != "global" && visibility != "power" {
			issues = append(issues, issue(reports.CodeValidationFailed, reports.SeverityError, netPath+".pins", "expected net requires at least two pins unless exported"))
		}
		switch visibility {
		case "", "local", "exported", "global", "power":
		default:
			issues = append(issues, issue(reports.CodeValidationFailed, reports.SeverityError, netPath+".visibility", "unsupported expected net visibility "+net.Visibility))
		}
		for pinIndex, pin := range net.Pins {
			pinPath := fmt.Sprintf("%s.pins.%d", netPath, pinIndex)
			if strings.TrimSpace(pin.Pin) == "" {
				issues = append(issues, issue(reports.CodeValidationFailed, reports.SeverityError, pinPath+".pin", "expected net pin number is required"))
			}
			role := strings.TrimSpace(pin.Role)
			ref := strings.TrimSpace(pin.Ref)
			if role == "" && ref == "" {
				issues = append(issues, issue(reports.CodeValidationFailed, reports.SeverityError, pinPath+".role", "expected net pin requires role or ref"))
			}
			if role != "" {
				if _, ok := componentRoles[role]; !ok {
					issues = append(issues, issue(reports.CodeValidationFailed, reports.SeverityError, pinPath+".role", "expected net pin references unknown component role "+role))
				}
			}
			if ref != "" {
				if _, ok := componentRefs[ref]; !ok {
					issues = append(issues, issue(reports.CodeValidationFailed, reports.SeverityError, pinPath+".ref", "expected net pin references unknown component ref "+ref))
				}
			}
		}
	}
	return issues
}

func validateExpectedPCB(path string, pcb ExpectedPCB, componentRefs map[string]struct{}, componentRoles map[string]struct{}, roleToRefs map[string][]string, expectedNetNames map[string]struct{}) []reports.Issue {
	var issues []reports.Issue
	placements := map[string]struct{}{}
	for index, placement := range pcb.Placements {
		placementPath := fmt.Sprintf("%s.placements.%d", path, index)
		role := strings.TrimSpace(placement.Role)
		ref := strings.TrimSpace(placement.Ref)
		if ref == "" && role == "" {
			issues = append(issues, issue(reports.CodeValidationFailed, reports.SeverityError, placementPath+".role", "expected placement requires ref or role"))
		}
		if role != "" {
			if _, ok := componentRoles[role]; !ok {
				issues = append(issues, issue(reports.CodeValidationFailed, reports.SeverityError, placementPath+".role", "expected placement references unknown component role "+role))
			}
		}
		if ref != "" {
			if _, ok := componentRefs[ref]; !ok {
				issues = append(issues, issue(reports.CodeValidationFailed, reports.SeverityError, placementPath+".ref", "expected placement references unknown component ref "+ref))
			}
		}
		key := placementKey(ref, role, roleToRefs)
		if key != "" {
			if _, exists := placements[key]; exists {
				field := ".role"
				if ref != "" {
					field = ".ref"
				}
				issues = append(issues, issue(reports.CodeValidationFailed, reports.SeverityError, placementPath+field, "duplicate expected placement for "+key))
			}
			placements[key] = struct{}{}
		}
		if placement.ToleranceMM != nil && *placement.ToleranceMM < 0 {
			issues = append(issues, issue(reports.CodeValidationFailed, reports.SeverityError, placementPath+".tolerance_mm", "expected placement tolerance must be non-negative"))
		}
		if placement.ToleranceDeg != nil && *placement.ToleranceDeg < 0 {
			issues = append(issues, issue(reports.CodeValidationFailed, reports.SeverityError, placementPath+".tolerance_deg", "expected placement rotation tolerance must be non-negative"))
		}
	}
	for index, padNet := range pcb.PadNets {
		padPath := fmt.Sprintf("%s.pad_nets.%d", path, index)
		if strings.TrimSpace(padNet.Ref) == "" {
			issues = append(issues, issue(reports.CodeValidationFailed, reports.SeverityError, padPath+".ref", "expected pad net ref is required"))
		}
		if strings.TrimSpace(padNet.Pad) == "" {
			issues = append(issues, issue(reports.CodeValidationFailed, reports.SeverityError, padPath+".pad", "expected pad net pad is required"))
		}
		if strings.TrimSpace(padNet.Net) == "" {
			issues = append(issues, issue(reports.CodeValidationFailed, reports.SeverityError, padPath+".net", "expected pad net net is required"))
		}
		ref := strings.TrimSpace(padNet.Ref)
		if ref != "" {
			if _, ok := componentRefs[ref]; !ok {
				issues = append(issues, issue(reports.CodeValidationFailed, reports.SeverityError, padPath+".ref", "expected pad net references unknown component ref "+ref))
			}
		}
		net := strings.TrimSpace(padNet.Net)
		if net != "" {
			if _, ok := expectedNetNames[net]; !ok {
				issues = append(issues, issue(reports.CodeValidationFailed, reports.SeverityError, padPath+".net", "expected pad net references unknown net "+net))
			}
		}
	}
	issues = append(issues, validateUniqueStrings(path+".required_routes", "duplicate required route", pcb.RequiredRoutes)...)
	issues = append(issues, validateUniqueStrings(path+".required_local_routes", "duplicate required local route", pcb.RequiredLocalRoutes)...)
	issues = append(issues, validateIdentifierStrings(path+".required_local_routes", "required local route", pcb.RequiredLocalRoutes)...)
	issues = append(issues, validateUniqueStrings(path+".required_zones", "duplicate required zone", pcb.RequiredZones)...)
	issues = append(issues, validateExpectedTimingFixtures(path+".timing_fixtures", pcb.TimingFixtures)...)
	return issues
}

func validateExpectedTimingFixtures(path string, fixtures []ExpectedTimingFixture) []reports.Issue {
	var issues []reports.Issue
	seen := map[string]struct{}{}
	for index, fixture := range fixtures {
		fixturePath := fmt.Sprintf("%s.%d", path, index)
		id := strings.TrimSpace(fixture.ID)
		if id == "" {
			issues = append(issues, issue(reports.CodeValidationFailed, reports.SeverityError, fixturePath+".id", "expected timing fixture ID is required"))
		} else if id != fixture.ID {
			issues = append(issues, issue(reports.CodeValidationFailed, reports.SeverityError, fixturePath+".id", "expected timing fixture ID must not contain leading or trailing whitespace"))
		} else if strings.ContainsAny(fixture.ID, " \t\n\r") {
			issues = append(issues, issue(reports.CodeValidationFailed, reports.SeverityError, fixturePath+".id", "expected timing fixture ID must not contain whitespace"))
		} else if _, exists := seen[id]; exists {
			issues = append(issues, issue(reports.CodeValidationFailed, reports.SeverityError, fixturePath+".id", "duplicate expected timing fixture "+id))
		} else {
			seen[id] = struct{}{}
		}
		issues = append(issues, validateUniqueStrings(fixturePath+".required_findings", "duplicate required timing finding", fixture.RequiredFindings)...)
		issues = append(issues, validateUniqueStrings(fixturePath+".forbidden_findings", "duplicate forbidden timing finding", fixture.ForbiddenFindings)...)
		issues = append(issues, validateIdentifierStrings(fixturePath+".required_findings", "required timing finding", fixture.RequiredFindings)...)
		issues = append(issues, validateIdentifierStrings(fixturePath+".forbidden_findings", "forbidden timing finding", fixture.ForbiddenFindings)...)
		issues = append(issues, validateDisjointTimingFindings(fixturePath, fixture.RequiredFindings, fixture.ForbiddenFindings)...)
	}
	return issues
}

func validateIdentifierStrings(path string, label string, values []string) []reports.Issue {
	var issues []reports.Issue
	for index, value := range values {
		valuePath := fmt.Sprintf("%s.%d", path, index)
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			issues = append(issues, issue(reports.CodeValidationFailed, reports.SeverityError, valuePath, label+" ID is required"))
		} else if trimmed != value {
			issues = append(issues, issue(reports.CodeValidationFailed, reports.SeverityError, valuePath, label+" ID must not contain leading or trailing whitespace"))
		} else if strings.ContainsAny(value, " \t\n\r") {
			issues = append(issues, issue(reports.CodeValidationFailed, reports.SeverityError, valuePath, label+" ID must not contain whitespace"))
		}
	}
	return issues
}

func validateDisjointTimingFindings(path string, required []string, forbidden []string) []reports.Issue {
	requiredSet := map[string]struct{}{}
	for _, finding := range required {
		if trimmed := strings.TrimSpace(finding); trimmed != "" {
			requiredSet[trimmed] = struct{}{}
		}
	}
	var issues []reports.Issue
	reported := map[string]struct{}{}
	for index, finding := range forbidden {
		trimmed := strings.TrimSpace(finding)
		if trimmed == "" {
			continue
		}
		if _, duplicate := reported[trimmed]; duplicate {
			continue
		}
		if _, exists := requiredSet[trimmed]; exists {
			issues = append(issues, issue(reports.CodeValidationFailed, reports.SeverityError, fmt.Sprintf("%s.forbidden_findings.%d", path, index), "timing finding cannot be both required and forbidden "+trimmed))
			reported[trimmed] = struct{}{}
		}
	}
	return issues
}

func validateExpectedWriter(path string, writer ExpectedWriter) []reports.Issue {
	var issues []reports.Issue
	if writer.RequireRoundTrip && !writer.Required {
		issues = append(issues, issue(reports.CodeValidationFailed, reports.SeverityError, path+".require_round_trip", "writer round-trip evidence requires writer validation"))
	}
	return issues
}

func validateExpectedERCDRC(path string, ercDRC ExpectedERCDRC) []reports.Issue {
	var issues []reports.Issue
	if (ercDRC.RequireERC || ercDRC.RequireDRC) && !ercDRC.Required {
		issues = append(issues, issue(reports.CodeValidationFailed, reports.SeverityError, path+".required", "ERC or DRC requirements require erc_drc.required"))
	}
	issues = append(issues, validateUniqueStrings(path+".allowed_codes", "duplicate allowed ERC/DRC code", ercDRC.AllowedCodes)...)
	issues = append(issues, validateUniqueStrings(path+".expected_issues", "duplicate expected ERC/DRC issue", ercDRC.ExpectedIssues)...)
	return issues
}

func expectedNetNameSet(nets []ExpectedNet) map[string]struct{} {
	names := map[string]struct{}{}
	for _, net := range nets {
		name := strings.TrimSpace(net.Name)
		if name != "" {
			names[name] = struct{}{}
		}
	}
	return names
}

func referenceMatchesPrefix(ref string, prefixes map[string]struct{}) bool {
	for prefix := range prefixes {
		if strings.HasPrefix(ref, prefix) && numericSuffix(strings.TrimPrefix(ref, prefix)) {
			return true
		}
	}
	return false
}

func numericSuffix(suffix string) bool {
	if suffix == "" {
		return false
	}
	for _, r := range suffix {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func placementKey(ref string, role string, roleToRefs map[string][]string) string {
	if ref != "" {
		return "ref:" + ref
	}
	if role != "" {
		if refs := roleToRefs[role]; len(refs) == 1 {
			return "ref:" + refs[0]
		}
		return "role:" + role
	}
	return ""
}

func evidenceRequiresFootprint(level EvidenceLevel) bool {
	switch level {
	case EvidenceTransferVerified, EvidencePCBVerified, EvidenceERCDRCVerified, EvidenceReferenceVerified:
		return true
	default:
		return false
	}
}

func validateUniqueStrings(path string, message string, values []string) []reports.Issue {
	var issues []reports.Issue
	seen := map[string]struct{}{}
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, exists := seen[trimmed]; exists {
			issues = append(issues, issue(reports.CodeValidationFailed, reports.SeverityError, path+"."+pathSegment(trimmed), message+" "+trimmed))
		}
		seen[trimmed] = struct{}{}
	}
	return issues
}

func SortIssues(issues []reports.Issue) {
	slices.SortFunc(issues, func(a, b reports.Issue) int {
		if a.Path != b.Path {
			return strings.Compare(a.Path, b.Path)
		}
		return strings.Compare(a.Message, b.Message)
	})
}

func issue(code reports.Code, severity reports.Severity, path string, message string) reports.Issue {
	return reports.Issue{Code: code, Severity: severity, Path: path, Message: message}
}

func pathID(id string) string {
	if strings.TrimSpace(id) == "" {
		return "unknown"
	}
	return pathSegment(id)
}

func pathSegment(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	var builder strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			builder.WriteRune(r)
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
		case r == '_' || r == '-':
			builder.WriteRune(r)
		default:
			builder.WriteRune('_')
		}
	}
	return builder.String()
}
