package pcbrules

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
)

type Role string

const (
	RolePower        Role = "power"
	RoleGround       Role = "ground"
	RoleSignal       Role = "signal"
	RoleClock        Role = "clock"
	RoleAnalog       Role = "analog"
	RoleHighCurrent  Role = "high_current"
	RoleDifferential Role = "differential"
	RoleUnknown      Role = "unknown"
)

type ZonePolicy string

const (
	ZoneIgnore      ZonePolicy = "ignore"
	ZoneObstacle    ZonePolicy = "obstacle"
	ZoneUnsupported ZonePolicy = "unsupported"
	ZoneSufficient  ZonePolicy = "sufficient"
)

type RuleSet struct {
	GridMM                 *float64          `json:"grid_mm,omitempty"`
	TraceWidthMM           *float64          `json:"trace_width_mm,omitempty"`
	ClearanceMM            *float64          `json:"clearance_mm,omitempty"`
	ViaDiameterMM          *float64          `json:"via_diameter_mm,omitempty"`
	ViaDrillMM             *float64          `json:"via_drill_mm,omitempty"`
	ViaClearanceMM         *float64          `json:"via_clearance_mm,omitempty"`
	EdgeClearanceMM        *float64          `json:"edge_clearance_mm,omitempty"`
	MaxSearchNodes         *int              `json:"max_search_nodes,omitempty"`
	MaxViasPerNet          *int              `json:"max_vias_per_net,omitempty"`
	AllowVias              *bool             `json:"allow_vias,omitempty"`
	AllowBackLayer         *bool             `json:"allow_back_layer,omitempty"`
	PreferLayer            string            `json:"prefer_layer,omitempty"`
	AllowedLayers          []string          `json:"allowed_layers,omitempty"`
	WarningLengthMM        *float64          `json:"warning_length_mm,omitempty"`
	MaxLengthMM            *float64          `json:"max_length_mm,omitempty"`
	MinTraceWidthMM        *float64          `json:"min_trace_width_mm,omitempty"`
	MinNeckdownWidthMM     *float64          `json:"min_neckdown_width_mm,omitempty"`
	NeckdownWidthMM        *float64          `json:"neckdown_width_mm,omitempty"`
	NeckdownLengthMM       *float64          `json:"neckdown_length_mm,omitempty"`
	TreatZonesAs           ZonePolicy        `json:"treat_zones_as,omitempty"`
	Classes                map[string]Class  `json:"classes,omitempty"`
	NetOverrides           map[string]Rule   `json:"net_overrides,omitempty"`
	ClearanceMatrix        ClearanceMatrix   `json:"clearance_matrix,omitempty"`
	DifferentialPairs      []CoupledNetGroup `json:"differential_pairs,omitempty"`
	RejectUnsupportedPairs bool              `json:"reject_unsupported_pairs,omitempty"`
}

type Class struct {
	TraceWidthMM       *float64 `json:"trace_width_mm,omitempty"`
	ClearanceMM        *float64 `json:"clearance_mm,omitempty"`
	ViaDiameterMM      *float64 `json:"via_diameter_mm,omitempty"`
	ViaDrillMM         *float64 `json:"via_drill_mm,omitempty"`
	ViaClearanceMM     *float64 `json:"via_clearance_mm,omitempty"`
	AllowedLayers      []string `json:"allowed_layers,omitempty"`
	PreferLayer        string   `json:"prefer_layer,omitempty"`
	MaxViasPerNet      *int     `json:"max_vias_per_net,omitempty"`
	WarningLengthMM    *float64 `json:"warning_length_mm,omitempty"`
	MaxLengthMM        *float64 `json:"max_length_mm,omitempty"`
	NeckdownWidthMM    *float64 `json:"neckdown_width_mm,omitempty"`
	NeckdownLengthMM   *float64 `json:"neckdown_length_mm,omitempty"`
	RequireExplicitUse bool     `json:"require_explicit_use,omitempty"`
}

type Rule struct {
	ClassName        string   `json:"class,omitempty"`
	Role             Role     `json:"role,omitempty"`
	TraceWidthMM     *float64 `json:"trace_width_mm,omitempty"`
	ClearanceMM      *float64 `json:"clearance_mm,omitempty"`
	ViaDiameterMM    *float64 `json:"via_diameter_mm,omitempty"`
	ViaDrillMM       *float64 `json:"via_drill_mm,omitempty"`
	ViaClearanceMM   *float64 `json:"via_clearance_mm,omitempty"`
	AllowedLayers    []string `json:"allowed_layers,omitempty"`
	PreferLayer      string   `json:"prefer_layer,omitempty"`
	MaxViasPerNet    *int     `json:"max_vias_per_net,omitempty"`
	WarningLengthMM  *float64 `json:"warning_length_mm,omitempty"`
	MaxLengthMM      *float64 `json:"max_length_mm,omitempty"`
	NeckdownWidthMM  *float64 `json:"neckdown_width_mm,omitempty"`
	NeckdownLengthMM *float64 `json:"neckdown_length_mm,omitempty"`
	PolicyTags       []string `json:"policy_tags,omitempty"`
}

type CoupledNetGroup struct {
	ID      string   `json:"id"`
	Mode    string   `json:"mode"`
	Members []string `json:"members"`
}

type NetDescriptor struct {
	Name  string
	Class string
	Role  Role
}

type EffectiveRule struct {
	NetName          string     `json:"net_name,omitempty"`
	ClassName        string     `json:"class,omitempty"`
	Role             Role       `json:"role,omitempty"`
	TraceWidthMM     float64    `json:"trace_width_mm"`
	ClearanceMM      float64    `json:"clearance_mm"`
	ViaDiameterMM    float64    `json:"via_diameter_mm"`
	ViaDrillMM       float64    `json:"via_drill_mm"`
	ViaClearanceMM   float64    `json:"via_clearance_mm"`
	AllowedLayers    []string   `json:"allowed_layers,omitempty"`
	PreferLayer      string     `json:"prefer_layer,omitempty"`
	MaxViasPerNet    int        `json:"max_vias_per_net"`
	WarningLengthMM  float64    `json:"warning_length_mm,omitempty"`
	MaxLengthMM      float64    `json:"max_length_mm,omitempty"`
	NeckdownWidthMM  float64    `json:"neckdown_width_mm,omitempty"`
	NeckdownLengthMM float64    `json:"neckdown_length_mm,omitempty"`
	TreatZonesAs     ZonePolicy `json:"treat_zones_as,omitempty"`
}

type Issue struct {
	Path     string
	Message  string
	Blocking bool
}

type ClearanceMatrix map[string]float64

const (
	DefaultTraceWidthMM   = 0.25
	DefaultClearanceMM    = 0.20
	DefaultViaDiameterMM  = 0.60
	DefaultViaDrillMM     = 0.30
	DefaultViaClearanceMM = 0.20
	DefaultMaxViasPerNet  = 4
	DifferentialPairMode  = "differential_pair"
)

type Resolver struct {
	set   RuleSet
	mu    sync.RWMutex
	cache map[resolveKey]resolveEntry
}

type resolveKey struct {
	name  string
	class string
	role  Role
}

type resolveEntry struct {
	rule   EffectiveRule
	issues []Issue
}

func NewResolver(set RuleSet) *Resolver {
	return &Resolver{set: set, cache: map[resolveKey]resolveEntry{}}
}

func PairKey(left string, right string) string {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	if left > right {
		left, right = right, left
	}
	return left + "|" + right
}

func Resolve(set RuleSet, net NetDescriptor) (EffectiveRule, []Issue) {
	return NewResolver(set).Resolve(net)
}

func (resolver *Resolver) Resolve(net NetDescriptor) (EffectiveRule, []Issue) {
	if resolver == nil {
		return Resolve(RuleSet{}, net)
	}
	key := resolveKey{name: strings.TrimSpace(net.Name), class: strings.TrimSpace(net.Class), role: net.Role}
	resolver.mu.RLock()
	if entry, ok := resolver.cache[key]; ok {
		resolver.mu.RUnlock()
		return entry.rule, append([]Issue(nil), entry.issues...)
	}
	resolver.mu.RUnlock()
	rule, issues := resolve(resolver.set, net)
	resolver.mu.Lock()
	resolver.cache[key] = resolveEntry{rule: rule, issues: append([]Issue(nil), issues...)}
	resolver.mu.Unlock()
	return rule, issues
}

func resolve(set RuleSet, net NetDescriptor) (EffectiveRule, []Issue) {
	issues := []Issue{}
	className := strings.TrimSpace(net.Class)
	override, hasOverride := set.NetOverrides[net.Name]
	if hasOverride && strings.TrimSpace(override.ClassName) != "" {
		className = strings.TrimSpace(override.ClassName)
	}
	class, hasClass := set.Classes[className]
	if className != "" && !hasClass {
		issues = append(issues, Issue{Path: "net.class", Message: fmt.Sprintf("unknown net class %q", className), Blocking: true})
	}
	role := net.Role
	if role == "" {
		role = RoleUnknown
	}
	roleRule := roleDefaults(role)
	effective := EffectiveRule{
		NetName:        net.Name,
		ClassName:      className,
		Role:           role,
		TraceWidthMM:   firstFloat(override.TraceWidthMM, class.TraceWidthMM, roleRule.TraceWidthMM, set.TraceWidthMM, floatPtr(DefaultTraceWidthMM)),
		ClearanceMM:    firstFloat(override.ClearanceMM, class.ClearanceMM, roleRule.ClearanceMM, set.ClearanceMM, floatPtr(DefaultClearanceMM)),
		ViaDiameterMM:  firstFloat(override.ViaDiameterMM, class.ViaDiameterMM, roleRule.ViaDiameterMM, set.ViaDiameterMM, floatPtr(DefaultViaDiameterMM)),
		ViaDrillMM:     firstFloat(override.ViaDrillMM, class.ViaDrillMM, roleRule.ViaDrillMM, set.ViaDrillMM, floatPtr(DefaultViaDrillMM)),
		ViaClearanceMM: firstFloat(override.ViaClearanceMM, class.ViaClearanceMM, roleRule.ViaClearanceMM, set.ViaClearanceMM, floatPtr(DefaultViaClearanceMM)),
		AllowedLayers:  firstStrings(override.AllowedLayers, class.AllowedLayers, roleRule.AllowedLayers, set.AllowedLayers),
		PreferLayer:    firstString(override.PreferLayer, class.PreferLayer, roleRule.PreferLayer, set.PreferLayer, "F.Cu"),
		MaxViasPerNet:  firstInt(override.MaxViasPerNet, class.MaxViasPerNet, roleRule.MaxViasPerNet, set.MaxViasPerNet, intPtr(DefaultMaxViasPerNet)),
		WarningLengthMM: firstFloat(
			override.WarningLengthMM,
			class.WarningLengthMM,
			roleRule.WarningLengthMM,
			set.WarningLengthMM,
		),
		MaxLengthMM: firstFloat(override.MaxLengthMM, class.MaxLengthMM, roleRule.MaxLengthMM, set.MaxLengthMM),
		NeckdownWidthMM: firstFloat(
			override.NeckdownWidthMM,
			class.NeckdownWidthMM,
			roleRule.NeckdownWidthMM,
			set.NeckdownWidthMM,
		),
		NeckdownLengthMM: firstFloat(
			override.NeckdownLengthMM,
			class.NeckdownLengthMM,
			roleRule.NeckdownLengthMM,
			set.NeckdownLengthMM,
		),
		TreatZonesAs: set.TreatZonesAs,
	}
	issues = append(issues, validateEffective(effective, set)...)
	return effective, issues
}

func Validate(set RuleSet) []Issue {
	issues := []Issue{}
	validatePositive := func(path string, value *float64) {
		if value != nil && (!finite(*value) || *value <= 0) {
			issues = append(issues, Issue{Path: path, Message: "must be positive and finite", Blocking: true})
		}
	}
	validateNonNegative := func(path string, value *float64) {
		if value != nil && (!finite(*value) || *value < 0) {
			issues = append(issues, Issue{Path: path, Message: "must be non-negative and finite", Blocking: true})
		}
	}
	validatePositive("trace_width_mm", set.TraceWidthMM)
	validateNonNegative("clearance_mm", set.ClearanceMM)
	validatePositive("via_diameter_mm", set.ViaDiameterMM)
	validatePositive("via_drill_mm", set.ViaDrillMM)
	validateNonNegative("via_clearance_mm", set.ViaClearanceMM)
	validateNonNegative("edge_clearance_mm", set.EdgeClearanceMM)
	validatePositive("neckdown_width_mm", set.NeckdownWidthMM)
	validateNonNegative("neckdown_length_mm", set.NeckdownLengthMM)
	validatePositive("min_trace_width_mm", set.MinTraceWidthMM)
	validatePositive("min_neckdown_width_mm", set.MinNeckdownWidthMM)
	if set.ViaDiameterMM != nil && set.ViaDrillMM != nil && *set.ViaDrillMM >= *set.ViaDiameterMM {
		issues = append(issues, Issue{Path: "via_drill_mm", Message: "via drill must be smaller than via diameter", Blocking: true})
	}
	for name, class := range set.Classes {
		prefix := fmt.Sprintf("classes[%s]", name)
		validatePositive(prefix+".trace_width_mm", class.TraceWidthMM)
		validateNonNegative(prefix+".clearance_mm", class.ClearanceMM)
		validatePositive(prefix+".via_diameter_mm", class.ViaDiameterMM)
		validatePositive(prefix+".via_drill_mm", class.ViaDrillMM)
		validateNonNegative(prefix+".via_clearance_mm", class.ViaClearanceMM)
		validatePositive(prefix+".neckdown_width_mm", class.NeckdownWidthMM)
		validateNonNegative(prefix+".neckdown_length_mm", class.NeckdownLengthMM)
		if class.ViaDiameterMM != nil && class.ViaDrillMM != nil && *class.ViaDrillMM >= *class.ViaDiameterMM {
			issues = append(issues, Issue{Path: prefix + ".via_drill_mm", Message: "net class via drill must be smaller than via diameter", Blocking: true})
		}
	}
	for key, clearance := range set.ClearanceMatrix {
		left, right, ok := strings.Cut(key, "|")
		if !ok || left == "" || right == "" || key != PairKey(left, right) {
			issues = append(issues, Issue{Path: "clearance_matrix[" + key + "]", Message: "clearance matrix keys must use normalized sorted class-pair keys", Blocking: true})
		}
		if !finite(clearance) || clearance < 0 {
			issues = append(issues, Issue{Path: "clearance_matrix[" + key + "]", Message: "clearance matrix clearance cannot be negative", Blocking: true})
		}
	}
	coupledMembers := map[string]string{}
	for index, group := range set.DifferentialPairs {
		path := fmt.Sprintf("differential_pairs[%d]", index)
		if strings.TrimSpace(group.ID) == "" {
			issues = append(issues, Issue{Path: path + ".id", Message: "coupled net group id is required", Blocking: true})
		}
		for _, member := range group.Members {
			member = strings.TrimSpace(member)
			if member == "" {
				continue
			}
			if existing := coupledMembers[member]; existing != "" && existing != group.ID {
				issues = append(issues, Issue{Path: path + ".members", Message: "net belongs to multiple coupled net groups", Blocking: true})
			}
			coupledMembers[member] = group.ID
		}
		switch strings.TrimSpace(group.Mode) {
		case "", DifferentialPairMode:
			if len(group.Members) != 2 {
				issues = append(issues, Issue{Path: path + ".members", Message: "differential pair requires exactly two members", Blocking: true})
			}
		default:
			if set.RejectUnsupportedPairs {
				issues = append(issues, Issue{Path: path + ".mode", Message: "unsupported coupled net group mode", Blocking: true})
			}
		}
	}
	return issues
}

func ClearanceBetween(set RuleSet, leftClass string, rightClass string, fallback float64) float64 {
	if set.ClearanceMatrix != nil {
		if value, ok := set.ClearanceMatrix[PairKey(leftClass, rightClass)]; ok {
			return value
		}
	}
	return fallback
}

func SortedClassNames(classes map[string]Class) []string {
	names := make([]string, 0, len(classes))
	for name := range classes {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func validateEffective(rule EffectiveRule, set RuleSet) []Issue {
	issues := []Issue{}
	if !finite(rule.TraceWidthMM) || rule.TraceWidthMM <= 0 {
		issues = append(issues, Issue{Path: "trace_width_mm", Message: "resolved trace width must be positive", Blocking: true})
	}
	if !finite(rule.ClearanceMM) || rule.ClearanceMM < 0 {
		issues = append(issues, Issue{Path: "clearance_mm", Message: "resolved clearance cannot be negative", Blocking: true})
	}
	if rule.ViaDrillMM >= rule.ViaDiameterMM {
		issues = append(issues, Issue{Path: "via_drill_mm", Message: "resolved via drill must be smaller than diameter", Blocking: true})
	}
	if set.MinTraceWidthMM != nil && rule.TraceWidthMM < *set.MinTraceWidthMM {
		issues = append(issues, Issue{Path: "trace_width_mm", Message: "resolved trace width is below manufacturing minimum", Blocking: true})
	}
	if set.MinNeckdownWidthMM != nil && rule.NeckdownWidthMM > 0 && rule.NeckdownWidthMM < *set.MinNeckdownWidthMM {
		issues = append(issues, Issue{Path: "neckdown_width_mm", Message: "resolved neckdown width is below manufacturing minimum", Blocking: true})
	}
	if rule.NeckdownWidthMM > rule.TraceWidthMM {
		issues = append(issues, Issue{Path: "neckdown_width_mm", Message: "neckdown width cannot exceed trace width", Blocking: true})
	}
	if rule.NeckdownLengthMM < 0 {
		issues = append(issues, Issue{Path: "neckdown_length_mm", Message: "neckdown length cannot be negative", Blocking: true})
	}
	return issues
}

func roleDefaults(role Role) Rule {
	switch role {
	case RolePower, RoleGround:
		return Rule{TraceWidthMM: floatPtr(0.35), MaxViasPerNet: intPtr(3)}
	case RoleHighCurrent:
		return Rule{TraceWidthMM: floatPtr(0.50), MaxViasPerNet: intPtr(2)}
	case RoleClock:
		return Rule{WarningLengthMM: floatPtr(75)}
	default:
		return Rule{}
	}
}

func firstFloat(values ...*float64) float64 {
	for _, value := range values {
		if value != nil {
			return *value
		}
	}
	return 0
}

func firstInt(values ...*int) int {
	for _, value := range values {
		if value != nil {
			return *value
		}
	}
	return 0
}

func firstString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func firstStrings(values ...[]string) []string {
	for _, value := range values {
		if len(value) > 0 {
			return append([]string(nil), value...)
		}
	}
	return nil
}

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func floatPtr(value float64) *float64 { return &value }
func intPtr(value int) *int           { return &value }
