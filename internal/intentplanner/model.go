package intentplanner

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"kicadai/internal/designworkflow"
	"kicadai/internal/reports"
)

const (
	RequestVersion  = "0.1.0"
	MaxRequestBytes = 1 << 20
)

type IntentKind string

const (
	IntentBreakout         IntentKind = "breakout"
	IntentPowerModule      IntentKind = "power_module"
	IntentSensorNode       IntentKind = "sensor_node"
	IntentMCUMinimal       IntentKind = "mcu_minimal"
	IntentAmplifier        IntentKind = "amplifier"
	IntentCustomStructured IntentKind = "custom_structured"
)

type Strength string

const (
	StrengthRequired  Strength = "required"
	StrengthPreferred Strength = "preferred"
	StrengthOptional  Strength = "optional"
	StrengthForbidden Strength = "forbidden"
)

type Request struct {
	Version       string                         `json:"version"`
	Name          string                         `json:"name"`
	Summary       string                         `json:"summary,omitempty"`
	Kind          IntentKind                     `json:"kind,omitempty"`
	Acceptance    designworkflow.AcceptanceLevel `json:"acceptance,omitempty"`
	Board         BoardIntent                    `json:"board,omitempty"`
	Power         PowerIntent                    `json:"power,omitempty"`
	Interfaces    []InterfaceIntent              `json:"interfaces,omitempty"`
	Functions     []FunctionIntent               `json:"functions,omitempty"`
	Protection    ProtectionIntent               `json:"protection,omitempty"`
	Manufacturing ManufacturingIntent            `json:"manufacturing,omitempty"`
	Constraints   ConstraintIntent               `json:"constraints,omitempty"`
}

type BoardIntent struct {
	WidthMM       float64  `json:"width_mm,omitempty"`
	HeightMM      float64  `json:"height_mm,omitempty"`
	Layers        int      `json:"layers,omitempty"`
	MountingHoles Strength `json:"mounting_holes,omitempty"`
}

type PowerIntent struct {
	Inputs []PowerInputIntent `json:"inputs,omitempty"`
	Rails  []PowerRailIntent  `json:"rails,omitempty"`
}

type PowerInputIntent struct {
	Kind      string   `json:"kind"`
	Voltage   string   `json:"voltage,omitempty"`
	CurrentMA float64  `json:"current_ma,omitempty"`
	Strength  Strength `json:"strength,omitempty"`
}

type PowerRailIntent struct {
	Name      string      `json:"name"`
	Voltage   string      `json:"voltage,omitempty"`
	CurrentMA float64     `json:"current_ma,omitempty"`
	Strength  Strength    `json:"strength,omitempty"`
	Supplies  []TargetRef `json:"supplies,omitempty"`
	Alias     string      `json:"alias,omitempty"`
}

type InterfaceIntent struct {
	Kind      string    `json:"kind"`
	Voltage   string    `json:"voltage,omitempty"`
	Connector string    `json:"connector,omitempty"`
	Quantity  int       `json:"quantity,omitempty"`
	Strength  Strength  `json:"strength,omitempty"`
	Target    TargetRef `json:"target,omitempty"`
	Bus       string    `json:"bus,omitempty"`
}

type FunctionIntent struct {
	Kind      string         `json:"kind"`
	Family    string         `json:"family,omitempty"`
	Quantity  int            `json:"quantity,omitempty"`
	Params    map[string]any `json:"params,omitempty"`
	Strength  Strength       `json:"strength,omitempty"`
	Target    TargetRef      `json:"target,omitempty"`
	Interface string         `json:"interface,omitempty"`
	Bus       string         `json:"bus,omitempty"`
	Supply    string         `json:"supply,omitempty"`
}

type TargetRef struct {
	ID   string `json:"id,omitempty"`
	Role string `json:"role,omitempty"`
}

type ProtectionIntent struct {
	ESD             Strength `json:"esd,omitempty"`
	ReversePolarity Strength `json:"reverse_polarity,omitempty"`
}

type ManufacturingIntent struct {
	Profile              string `json:"profile,omitempty"`
	FabricationCandidate bool   `json:"fabrication_candidate,omitempty"`
}

type ConstraintIntent struct {
	PreferSMD          bool              `json:"prefer_smd,omitempty"`
	AllowPlaceholders  bool              `json:"allow_placeholders,omitempty"`
	PackagePreferences map[string]string `json:"package_preferences,omitempty"`
	RouteWidthMM       float64           `json:"route_width_mm,omitempty"`
	ClearanceMM        float64           `json:"clearance_mm,omitempty"`
	SkipRouting        bool              `json:"skip_routing,omitempty"`
}

var projectNamePattern = regexp.MustCompile(`[^A-Za-z0-9_-]+`)

func DecodeRequestStrict(reader io.Reader) (Request, []reports.Issue) {
	var buffer bytes.Buffer
	limited := io.LimitReader(reader, MaxRequestBytes+1)
	if _, err := io.Copy(&buffer, limited); err != nil {
		return Request{}, []reports.Issue{issue("request", "read request: "+err.Error())}
	}
	if buffer.Len() > MaxRequestBytes {
		return Request{}, []reports.Issue{issue("request", "request exceeds maximum size")}
	}
	decoder := json.NewDecoder(bytes.NewReader(buffer.Bytes()))
	decoder.DisallowUnknownFields()
	var request Request
	if err := decoder.Decode(&request); err != nil {
		return Request{}, []reports.Issue{issue("request", "decode request: "+err.Error())}
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return Request{}, []reports.Issue{issue("request", "request must contain exactly one JSON object")}
	}
	return request, ValidateRequest(request)
}

func NormalizeRequest(request Request) Request {
	request.Version = strings.TrimSpace(request.Version)
	request.Name = normalizeProjectName(request.Name)
	request.Summary = strings.TrimSpace(request.Summary)
	request.Kind = IntentKind(strings.ToLower(strings.TrimSpace(string(request.Kind))))
	if request.Kind == "" {
		request.Kind = IntentCustomStructured
	}
	if request.Acceptance == "" {
		request.Acceptance = designworkflow.AcceptanceStructural
	}
	if request.Board.Layers == 0 {
		request.Board.Layers = 2
	}
	request.Board.MountingHoles = normalizeStrength(request.Board.MountingHoles, StrengthOptional)
	request.Manufacturing.Profile = strings.TrimSpace(request.Manufacturing.Profile)
	request.Power.Inputs = normalizePowerInputs(request.Power.Inputs)
	request.Power.Rails = normalizePowerRails(request.Power.Rails)
	request.Interfaces = normalizeInterfaces(request.Interfaces)
	request.Functions = normalizeFunctions(request.Functions)
	request.Protection.ESD = normalizeStrength(request.Protection.ESD, StrengthOptional)
	request.Protection.ReversePolarity = normalizeStrength(request.Protection.ReversePolarity, StrengthOptional)
	request.Constraints.PackagePreferences = normalizeStringMap(request.Constraints.PackagePreferences)
	return request
}

func normalizePowerInputs(values []PowerInputIntent) []PowerInputIntent {
	out := append([]PowerInputIntent(nil), values...)
	for i := range out {
		out[i].Kind = normalizeToken(out[i].Kind)
		out[i].Voltage = strings.TrimSpace(out[i].Voltage)
		out[i].Strength = normalizeStrength(out[i].Strength, StrengthRequired)
	}
	return out
}

func normalizePowerRails(values []PowerRailIntent) []PowerRailIntent {
	out := append([]PowerRailIntent(nil), values...)
	for i := range out {
		out[i].Name = strings.TrimSpace(out[i].Name)
		out[i].Voltage = strings.TrimSpace(out[i].Voltage)
		out[i].Strength = normalizeStrength(out[i].Strength, StrengthRequired)
		out[i].Alias = normalizeToken(out[i].Alias)
		out[i].Supplies = normalizeTargetRefs(out[i].Supplies)
	}
	return out
}

func normalizeInterfaces(values []InterfaceIntent) []InterfaceIntent {
	out := append([]InterfaceIntent(nil), values...)
	for i := range out {
		out[i].Kind = normalizeToken(out[i].Kind)
		out[i].Voltage = strings.TrimSpace(out[i].Voltage)
		out[i].Connector = normalizeToken(out[i].Connector)
		out[i].Target = normalizeTargetRef(out[i].Target)
		out[i].Bus = normalizeToken(out[i].Bus)
		if out[i].Quantity == 0 {
			out[i].Quantity = 1
		}
		out[i].Strength = normalizeStrength(out[i].Strength, StrengthRequired)
	}
	return out
}

func normalizeFunctions(values []FunctionIntent) []FunctionIntent {
	out := append([]FunctionIntent(nil), values...)
	for i := range out {
		out[i].Kind = normalizeToken(out[i].Kind)
		out[i].Family = normalizeToken(out[i].Family)
		out[i].Target = normalizeTargetRef(out[i].Target)
		out[i].Interface = normalizeToken(out[i].Interface)
		out[i].Bus = normalizeToken(out[i].Bus)
		out[i].Supply = normalizeToken(out[i].Supply)
		if out[i].Quantity == 0 {
			out[i].Quantity = 1
		}
		out[i].Strength = normalizeStrength(out[i].Strength, StrengthRequired)
		out[i].Params = cloneParams(out[i].Params)
	}
	return out
}

func ValidateRequest(request Request) []reports.Issue {
	request = NormalizeRequest(request)
	var issues []reports.Issue
	if request.Version == "" {
		issues = append(issues, issue("version", "intent request version is required"))
	} else if request.Version != RequestVersion {
		issues = append(issues, issue("version", "unsupported intent request version "+request.Version))
	}
	if !validIntentKind(request.Kind) {
		issues = append(issues, issue("kind", "unsupported intent kind "+string(request.Kind)))
	}
	if request.Board.WidthMM < 0 || math.IsNaN(request.Board.WidthMM) || math.IsInf(request.Board.WidthMM, 0) {
		issues = append(issues, issue("board.width_mm", "board width must be non-negative and finite"))
	}
	if request.Board.HeightMM < 0 || math.IsNaN(request.Board.HeightMM) || math.IsInf(request.Board.HeightMM, 0) {
		issues = append(issues, issue("board.height_mm", "board height must be non-negative and finite"))
	}
	if request.Board.Layers != 1 && request.Board.Layers != 2 {
		issues = append(issues, issue("board.layers", "board layers must be 1 or 2"))
	}
	if !validAcceptance(request.Acceptance) {
		issues = append(issues, issue("acceptance", "unsupported acceptance level "+string(request.Acceptance)))
	}
	if !validStrength(request.Board.MountingHoles) {
		issues = append(issues, issue("board.mounting_holes", "unsupported requirement strength "+string(request.Board.MountingHoles)))
	}
	issues = append(issues, validatePower(request.Power)...)
	issues = append(issues, validateInterfaces(request.Interfaces)...)
	issues = append(issues, validateFunctions(request.Functions)...)
	issues = append(issues, validateProtection(request.Protection)...)
	if request.Constraints.RouteWidthMM < 0 {
		issues = append(issues, issue("constraints.route_width_mm", "route width must be non-negative"))
	}
	if request.Constraints.ClearanceMM < 0 {
		issues = append(issues, issue("constraints.clearance_mm", "clearance must be non-negative"))
	}
	slices.SortFunc(issues, compareIssues)
	return issues
}

func validatePower(power PowerIntent) []reports.Issue {
	var issues []reports.Issue
	for index, input := range power.Inputs {
		path := fmt.Sprintf("power.inputs[%d]", index)
		if input.Kind == "" {
			issues = append(issues, issue(path+".kind", "power input kind is required"))
		} else if !validPowerInputKind(input.Kind) {
			issues = append(issues, issue(path+".kind", "unsupported power input kind "+input.Kind))
		}
		issues = append(issues, validateVoltage(path+".voltage", input.Voltage, input.Strength)...)
		if input.CurrentMA < 0 {
			issues = append(issues, issue(path+".current_ma", "power input current must be non-negative"))
		}
		if !validStrength(input.Strength) {
			issues = append(issues, issue(path+".strength", "unsupported requirement strength "+string(input.Strength)))
		}
	}
	for index, rail := range power.Rails {
		path := fmt.Sprintf("power.rails[%d]", index)
		if strings.TrimSpace(rail.Name) == "" {
			issues = append(issues, issue(path+".name", "power rail name is required"))
		}
		issues = append(issues, validateVoltage(path+".voltage", rail.Voltage, rail.Strength)...)
		if rail.CurrentMA < 0 {
			issues = append(issues, issue(path+".current_ma", "power rail current must be non-negative"))
		}
		if !validStrength(rail.Strength) {
			issues = append(issues, issue(path+".strength", "unsupported requirement strength "+string(rail.Strength)))
		}
		if rail.Alias != "" && !validSemanticToken(rail.Alias) {
			issues = append(issues, issue(path+".alias", "power rail alias must be a simple token"))
		}
		issues = append(issues, validateTargetRefs(path+".supplies", rail.Supplies)...)
	}
	return issues
}

func validateInterfaces(values []InterfaceIntent) []reports.Issue {
	var issues []reports.Issue
	for index, iface := range values {
		path := fmt.Sprintf("interfaces[%d]", index)
		if iface.Kind == "" {
			issues = append(issues, issue(path+".kind", "interface kind is required"))
		} else if !validInterfaceKind(iface.Kind) {
			issues = append(issues, issue(path+".kind", "unsupported interface kind "+iface.Kind))
		}
		if iface.Quantity < 1 {
			issues = append(issues, issue(path+".quantity", "interface quantity must be at least 1"))
		}
		issues = append(issues, validateVoltage(path+".voltage", iface.Voltage, iface.Strength)...)
		if !validStrength(iface.Strength) {
			issues = append(issues, issue(path+".strength", "unsupported requirement strength "+string(iface.Strength)))
		}
		if iface.Bus != "" && !validSemanticToken(iface.Bus) {
			issues = append(issues, issue(path+".bus", "interface bus must be a simple token"))
		}
		issues = append(issues, validateTargetRef(path+".target", iface.Target)...)
	}
	return issues
}

func validateFunctions(values []FunctionIntent) []reports.Issue {
	var issues []reports.Issue
	for index, function := range values {
		path := fmt.Sprintf("functions[%d]", index)
		if function.Kind == "" {
			issues = append(issues, issue(path+".kind", "function kind is required"))
		} else if !validFunctionKind(function.Kind) {
			issues = append(issues, issue(path+".kind", "unsupported function kind "+function.Kind))
		}
		if function.Quantity < 1 {
			issues = append(issues, issue(path+".quantity", "function quantity must be at least 1"))
		}
		if !validStrength(function.Strength) {
			issues = append(issues, issue(path+".strength", "unsupported requirement strength "+string(function.Strength)))
		}
		if function.Interface != "" && !validSemanticInterface(function.Interface) {
			issues = append(issues, issue(path+".interface", "unsupported semantic interface "+function.Interface))
		}
		if function.Bus != "" && !validSemanticToken(function.Bus) {
			issues = append(issues, issue(path+".bus", "function bus must be a simple token"))
		}
		if function.Supply != "" && !validSemanticToken(function.Supply) {
			issues = append(issues, issue(path+".supply", "function supply must be a simple token"))
		}
		issues = append(issues, validateTargetRef(path+".target", function.Target)...)
	}
	return issues
}

func validateProtection(protection ProtectionIntent) []reports.Issue {
	var issues []reports.Issue
	if !validStrength(protection.ESD) {
		issues = append(issues, issue("protection.esd", "unsupported requirement strength "+string(protection.ESD)))
	}
	if !validStrength(protection.ReversePolarity) {
		issues = append(issues, issue("protection.reverse_polarity", "unsupported requirement strength "+string(protection.ReversePolarity)))
	}
	return issues
}

func validateVoltage(path string, value string, strength Strength) []reports.Issue {
	value = strings.TrimSpace(value)
	if value == "" {
		if strength == StrengthRequired {
			return []reports.Issue{issue(path, "voltage is required")}
		}
		return nil
	}
	if _, ok := parseVoltage(value); !ok {
		return []reports.Issue{issue(path, "voltage must be a numeric value with optional V suffix")}
	}
	return nil
}

func parseVoltage(value string) (float64, bool) {
	trimmed := strings.TrimSpace(strings.TrimSuffix(strings.ToLower(value), "v"))
	if trimmed == "" {
		return 0, false
	}
	parsed, err := strconv.ParseFloat(trimmed, 64)
	return parsed, err == nil && !math.IsNaN(parsed) && !math.IsInf(parsed, 0)
}

func validIntentKind(kind IntentKind) bool {
	switch kind {
	case IntentBreakout, IntentPowerModule, IntentSensorNode, IntentMCUMinimal, IntentAmplifier, IntentCustomStructured:
		return true
	default:
		return false
	}
}

func validAcceptance(level designworkflow.AcceptanceLevel) bool {
	switch level {
	case designworkflow.AcceptanceDraft, designworkflow.AcceptanceStructural, designworkflow.AcceptanceConnectivity, designworkflow.AcceptanceERCDRC, designworkflow.AcceptanceFabricationCandidate:
		return true
	default:
		return false
	}
}

func validStrength(strength Strength) bool {
	switch strength {
	case StrengthRequired, StrengthPreferred, StrengthOptional, StrengthForbidden:
		return true
	default:
		return false
	}
}

func normalizeStrength(strength Strength, fallback Strength) Strength {
	strength = Strength(strings.ToLower(strings.TrimSpace(string(strength))))
	if strength == "" {
		return fallback
	}
	return strength
}

func validPowerInputKind(kind string) bool {
	switch kind {
	case "usb_c", "dc_jack", "header", "battery", "external":
		return true
	default:
		return false
	}
}

func validInterfaceKind(kind string) bool {
	switch kind {
	case "i2c", "spi", "uart", "gpio", "power", "connector", "usb_c", "analog":
		return true
	default:
		return false
	}
}

func validFunctionKind(kind string) bool {
	switch kind {
	case "indicator", "connector", "sensor", "mcu", "amplifier", "regulator", "power", "clock", "reset_programming", "protection":
		return true
	default:
		return false
	}
}

func validSemanticInterface(value string) bool {
	switch value {
	case "i2c", "gpio", "uart", "spi", "clock", "programming":
		return true
	default:
		return false
	}
}

func validSemanticToken(value string) bool {
	value = normalizeToken(value)
	return value == "" || strings.IndexFunc(value, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_' || r == '-')
	}) == -1
}

func normalizeProjectName(name string) string {
	name = strings.TrimSpace(name)
	name = projectNamePattern.ReplaceAllString(name, "_")
	name = strings.Trim(name, "_-")
	if name == "" {
		return "kicadai_intent"
	}
	return name
}

func normalizeToken(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizeTargetRef(value TargetRef) TargetRef {
	return TargetRef{ID: normalizeToken(value.ID), Role: normalizeToken(value.Role)}
}

func normalizeTargetRefs(values []TargetRef) []TargetRef {
	if len(values) == 0 {
		return nil
	}
	out := append([]TargetRef(nil), values...)
	for i := range out {
		out[i] = normalizeTargetRef(out[i])
	}
	return out
}

func validateTargetRef(path string, target TargetRef) []reports.Issue {
	var issues []reports.Issue
	if target.ID != "" && !validSemanticToken(target.ID) {
		issues = append(issues, issue(path+".id", "target id must be a simple token"))
	}
	if target.Role != "" && !validSemanticToken(target.Role) {
		issues = append(issues, issue(path+".role", "target role must be a simple token"))
	}
	return issues
}

func validateTargetRefs(path string, targets []TargetRef) []reports.Issue {
	var issues []reports.Issue
	for index, target := range targets {
		issues = append(issues, validateTargetRef(fmt.Sprintf("%s[%d]", path, index), target)...)
	}
	return issues
}

func normalizeStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]string, len(values))
	for key, value := range values {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key != "" && value != "" {
			out[key] = value
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func cloneParams(values map[string]any) map[string]any {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]any, len(values))
	for key, value := range values {
		out[key] = cloneAny(value)
	}
	return out
}

func cloneAny(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneParams(typed)
	case []any:
		out := make([]any, len(typed))
		for i := range typed {
			out[i] = cloneAny(typed[i])
		}
		return out
	case []string:
		return append([]string(nil), typed...)
	case []float64:
		return append([]float64(nil), typed...)
	case []int:
		return append([]int(nil), typed...)
	default:
		return value
	}
}

func issue(path string, message string) reports.Issue {
	return reports.Issue{
		Code:     reports.CodeInvalidArgument,
		Severity: reports.SeverityError,
		Path:     path,
		Message:  message,
	}
}

func compareIssues(a, b reports.Issue) int {
	if a.Path != b.Path {
		return strings.Compare(a.Path, b.Path)
	}
	if a.Message != b.Message {
		return strings.Compare(a.Message, b.Message)
	}
	return strings.Compare(string(a.Code), string(b.Code))
}
