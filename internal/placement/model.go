package placement

import (
	"fmt"
	"math"
	"slices"
	"strings"

	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

const maxCandidatesPerPartLimit = 100000

type Status string

const (
	StatusPlaced  Status = "placed"
	StatusPartial Status = "partial"
	StatusBlocked Status = "blocked"
)

type BoundsSource string

const (
	BoundsLibraryCourtyard BoundsSource = "library_courtyard"
	BoundsLibraryPads      BoundsSource = "library_pads"
	BoundsGeneratedPads    BoundsSource = "generated_pads"
	BoundsExplicit         BoundsSource = "explicit"
	BoundsEstimated        BoundsSource = "estimated"
)

type SideConstraint string

const (
	SideAny    SideConstraint = "any"
	SideTop    SideConstraint = "top"
	SideBottom SideConstraint = "bottom"
)

type EdgeConstraint string

const (
	EdgeNone   EdgeConstraint = ""
	EdgeAny    EdgeConstraint = "any"
	EdgeLeft   EdgeConstraint = "left"
	EdgeRight  EdgeConstraint = "right"
	EdgeTop    EdgeConstraint = "top"
	EdgeBottom EdgeConstraint = "bottom"
)

type NetRole string

const (
	NetPower        NetRole = "power"
	NetGround       NetRole = "ground"
	NetSignal       NetRole = "signal"
	NetClock        NetRole = "clock"
	NetAnalog       NetRole = "analog"
	NetDifferential NetRole = "differential"
	NetUnknown      NetRole = "unknown"
)

type IntentRole string

const (
	IntentDecoupling  IntentRole = "decoupling"
	IntentClock       IntentRole = "clock"
	IntentFeedback    IntentRole = "feedback"
	IntentPowerPath   IntentRole = "power_path"
	IntentPullup      IntentRole = "pullup"
	IntentConnector   IntentRole = "connector"
	IntentThermal     IntentRole = "thermal"
	IntentReset       IntentRole = "reset"
	IntentProgramming IntentRole = "programming"
)

type Request struct {
	Board          BoardPlacementArea
	Components     []Component
	Nets           []Net
	Groups         []Group
	Keepouts       []Keepout
	Mechanical     []MechanicalConstraint
	ProximityRules []ProximityRule
	RegionRules    []RegionRule
	AdvancedRules  AdvancedPlacementRules
	Rules          Rules
	Existing       ExistingPlacementPolicy
	Seed           string
}

type BoardPlacementArea struct {
	WidthMM  float64
	HeightMM float64
	Origin   Point
	MarginMM float64
}

type Component struct {
	Ref                           string
	Value                         string
	FootprintID                   string
	Role                          string
	Bounds                        Bounds
	Pads                          []PadSummary
	AllowUnmatchedUnconnectedPads bool
	Fixed                         bool
	Position                      *Placement
	Side                          SideConstraint
	Rotation                      RotationConstraint
	Edge                          EdgeConstraint
	GroupID                       string
	Priority                      int
	Hints                         []Hint
	Mobility                      MobilityPolicy
}

type Bounds struct {
	WidthMM      float64
	HeightMM     float64
	CourtyardMM  float64
	AnchorOffset Point
	Source       BoundsSource
}

type PadSummary struct {
	Name     string
	Net      string
	XMM      float64
	YMM      float64
	WidthMM  float64
	HeightMM float64
	Type     string
	DrillMM  float64
	Layers   []string
}

type Net struct {
	Name       string
	Endpoints  []Endpoint
	Role       NetRole
	Weight     int
	WidthClass string
}

type Endpoint struct {
	Ref string
	Pin string
}

type Group struct {
	ID           string
	Role         string
	Components   []string
	Anchor       GroupAnchor
	KeepTogether bool
	MaxSpreadMM  float64
	Priority     int
}

type GroupAnchor struct {
	Ref string
	At  *Point
}

type Keepout struct {
	ID         string
	Bounds     Rect
	Layers     []string
	Reason     string
	Optional   bool
	Mechanical bool `json:"Mechanical,omitempty"`
}

type MechanicalConstraint struct {
	ID       string   `json:"id"`
	Kind     string   `json:"kind"`
	Bounds   Rect     `json:"bounds"`
	Layers   []string `json:"layers,omitempty"`
	Optional bool     `json:"optional,omitempty"`
	Reason   string   `json:"reason,omitempty"`
}

type Rules struct {
	GridMM                   float64
	ComponentSpacingMM       float64
	BoardEdgeClearanceMM     float64
	GroupSpacingMM           float64
	PreferTopLayer           bool
	AllowBackLayer           bool
	ConnectorEdgeClearanceMM float64
	MaxCandidatesPerPart     int
	CandidateScoring         CandidateScoringRules
}

type CandidateScoringRules struct {
	Enabled                     bool
	Policy                      string
	MaxAlternativesPerComponent int
	MaxEvidencePerDimension     int
	Weights                     CandidateScoreWeights
}

type CandidateScoreWeights struct {
	HardConstraints     float64
	SemanticRole        float64
	GroupCohesion       float64
	ElectricalProximity float64
	RouteLength         float64
	Congestion          float64
	Fanout              float64
	Edge                float64
	Region              float64
	Mobility            float64
	Thermal             float64
	HighCurrent         float64
	CreepageClearance   float64
	DifferentialPair    float64
	ControlledImpedance float64
	TimingSensitive     float64
}

type ProximityRule struct {
	ID            string
	Source        string
	Role          IntentRole
	AnchorRef     string
	TargetRefs    []string
	AnchorPins    []string
	TargetPins    []string
	MaxDistanceMM float64
	Weight        int
	Required      bool
}

type RegionRule struct {
	ID        string
	Source    string
	Region    string
	Refs      []string
	NetRoles  []NetRole
	Preferred Rect
	Weight    int
	Required  bool
}

type AdvancedPlacementRules struct {
	Thermal             []ThermalPlacementRule             `json:"thermal,omitempty"`
	HighCurrent         []HighCurrentPlacementRule         `json:"high_current,omitempty"`
	CreepageClearance   []CreepageClearancePlacementRule   `json:"creepage_clearance,omitempty"`
	DifferentialPair    []DifferentialPairPlacementRule    `json:"differential_pair,omitempty"`
	ControlledImpedance []ControlledImpedancePlacementRule `json:"controlled_impedance,omitempty"`
}

type AdvancedRuleSeverity string

const (
	AdvancedRuleSeverityInfo    AdvancedRuleSeverity = "info"
	AdvancedRuleSeverityWarning AdvancedRuleSeverity = "warning"
	AdvancedRuleSeverityError   AdvancedRuleSeverity = "error"
)

type AdvancedRuleEnforcement string

const (
	AdvancedRuleSoft AdvancedRuleEnforcement = "soft"
	AdvancedRuleHard AdvancedRuleEnforcement = "hard"
)

type ThermalRole string

const (
	ThermalRoleHeatSource       ThermalRole = "heat_source"
	ThermalRoleThermalSensitive ThermalRole = "thermal_sensitive"
	ThermalRoleHeatSink         ThermalRole = "heat_sink"
	ThermalRoleRegulator        ThermalRole = "regulator"
	ThermalRolePowerSwitch      ThermalRole = "power_switch"
	ThermalRoleConnector        ThermalRole = "connector"
)

type ThermalPlacementRule struct {
	ID              string                  `json:"id"`
	Source          string                  `json:"source,omitempty"`
	Refs            []string                `json:"refs,omitempty"`
	Roles           []string                `json:"roles,omitempty"`
	ThermalRole     ThermalRole             `json:"thermal_role,omitempty"`
	PreferredEdge   EdgeConstraint          `json:"preferred_edge,omitempty"`
	PreferredRegion string                  `json:"preferred_region,omitempty"`
	KeepAwayRefs    []string                `json:"keep_away_refs,omitempty"`
	KeepAwayRoles   []string                `json:"keep_away_roles,omitempty"`
	MinDistanceMM   float64                 `json:"min_distance_mm,omitempty"`
	PreferCopper    bool                    `json:"prefer_copper,omitempty"`
	Severity        AdvancedRuleSeverity    `json:"severity,omitempty"`
	Enforcement     AdvancedRuleEnforcement `json:"enforcement,omitempty"`
}

type HighCurrentPlacementRule struct {
	ID                   string                  `json:"id"`
	Source               string                  `json:"source,omitempty"`
	Nets                 []string                `json:"nets,omitempty"`
	NetRoles             []NetRole               `json:"net_roles,omitempty"`
	CurrentClass         string                  `json:"current_class,omitempty"`
	CurrentEstimateA     float64                 `json:"current_estimate_a,omitempty"`
	SourceRefs           []string                `json:"source_refs,omitempty"`
	SinkRefs             []string                `json:"sink_refs,omitempty"`
	SourcePads           []string                `json:"source_pads,omitempty"`
	SinkPads             []string                `json:"sink_pads,omitempty"`
	MaxPreferredLengthMM float64                 `json:"max_preferred_length_mm,omitempty"`
	PreferredLayer       string                  `json:"preferred_layer,omitempty"`
	KeepAwayRefs         []string                `json:"keep_away_refs,omitempty"`
	KeepAwayRoles        []string                `json:"keep_away_roles,omitempty"`
	Severity             AdvancedRuleSeverity    `json:"severity,omitempty"`
	Enforcement          AdvancedRuleEnforcement `json:"enforcement,omitempty"`
}

type CreepageClearancePlacementRule struct {
	ID              string                  `json:"id"`
	Source          string                  `json:"source,omitempty"`
	DomainA         PlacementRuleDomain     `json:"domain_a"`
	DomainB         PlacementRuleDomain     `json:"domain_b"`
	MinClearanceMM  float64                 `json:"min_clearance_mm,omitempty"`
	MinCreepageMM   float64                 `json:"min_creepage_mm,omitempty"`
	Voltage         string                  `json:"voltage,omitempty"`
	InsulationClass string                  `json:"insulation_class,omitempty"`
	BoardSides      []string                `json:"board_sides,omitempty"`
	Severity        AdvancedRuleSeverity    `json:"severity,omitempty"`
	Enforcement     AdvancedRuleEnforcement `json:"enforcement,omitempty"`
}

type DifferentialPairPlacementRule struct {
	ID                   string                  `json:"id"`
	Source               string                  `json:"source,omitempty"`
	PositiveNet          string                  `json:"positive_net"`
	NegativeNet          string                  `json:"negative_net"`
	SourceRefs           []string                `json:"source_refs,omitempty"`
	SinkRefs             []string                `json:"sink_refs,omitempty"`
	SourcePads           []string                `json:"source_pads,omitempty"`
	SinkPads             []string                `json:"sink_pads,omitempty"`
	PreferredOrientation string                  `json:"preferred_orientation,omitempty"`
	MaxSkewMM            float64                 `json:"max_skew_mm,omitempty"`
	LengthToleranceMM    float64                 `json:"length_tolerance_mm,omitempty"`
	PairGroupID          string                  `json:"pair_group_id,omitempty"`
	Severity             AdvancedRuleSeverity    `json:"severity,omitempty"`
	Enforcement          AdvancedRuleEnforcement `json:"enforcement,omitempty"`
}

type ControlledImpedancePlacementRule struct {
	ID                    string                  `json:"id"`
	Source                string                  `json:"source,omitempty"`
	Nets                  []string                `json:"nets,omitempty"`
	NetRoles              []NetRole               `json:"net_roles,omitempty"`
	PreferredLayers       []string                `json:"preferred_layers,omitempty"`
	MinCorridorWidthMM    float64                 `json:"min_corridor_width_mm,omitempty"`
	SourceRefs            []string                `json:"source_refs,omitempty"`
	SinkRefs              []string                `json:"sink_refs,omitempty"`
	SourcePads            []string                `json:"source_pads,omitempty"`
	SinkPads              []string                `json:"sink_pads,omitempty"`
	MaxViaCountPreference int                     `json:"max_via_count_preference,omitempty"`
	ReferencePlane        string                  `json:"reference_plane,omitempty"`
	Severity              AdvancedRuleSeverity    `json:"severity,omitempty"`
	Enforcement           AdvancedRuleEnforcement `json:"enforcement,omitempty"`
}

type PlacementRuleDomain struct {
	Refs       []string  `json:"refs,omitempty"`
	Nets       []string  `json:"nets,omitempty"`
	NetClasses []string  `json:"net_classes,omitempty"`
	Roles      []string  `json:"roles,omitempty"`
	NetRoles   []NetRole `json:"net_roles,omitempty"`
}

type ExistingPlacementPolicy struct {
	PreserveFixed bool
}

type RotationConstraint struct {
	FixedDeg   *float64
	AllowedDeg []float64
}

type Hint struct {
	Kind  string
	Value string
}

type Point struct {
	XMM float64 `json:"x_mm"`
	YMM float64 `json:"y_mm"`
}

type Rect struct {
	Min Point `json:"min"`
	Max Point `json:"max"`
}

type Placement struct {
	XMM         float64
	YMM         float64
	RotationDeg float64
	Layer       string
}

type Result struct {
	Status           Status                   `json:"status"`
	Placements       []PlacementResult        `json:"placements"`
	Issues           []reports.Issue          `json:"issues"`
	Metrics          Metrics                  `json:"metrics"`
	Operations       []transactions.Operation `json:"operations,omitempty"`
	Quality          *QualityReport           `json:"quality,omitempty"`
	CandidateScoring *CandidateScoringReport  `json:"candidate_scoring,omitempty"`
}

type PlacementResult struct {
	Ref         string
	FootprintID string
	Position    Placement
	Bounds      Rect
	Fixed       bool
	GroupID     string
	Mobility    MobilityPolicy
	Reason      string
}

type Metrics struct {
	ComponentCount       int
	PlacedCount          int
	FixedCount           int
	UnplacedCount        int
	CollisionCount       int
	OutsideOutlineCount  int
	EstimatedBoundsCount int
	HPWLMM               float64
}

type MobilityClass string

const (
	MobilityFixed          MobilityClass = "fixed"
	MobilityGroupTransform MobilityClass = "group_transform"
	MobilityLocalRebuild   MobilityClass = "local_rebuild"
	MobilitySoftPreferred  MobilityClass = "soft_preferred"
	MobilityUnowned        MobilityClass = "unowned"
)

type RouteHandlingPolicy string

const (
	RouteHandlingTransformWithGroup RouteHandlingPolicy = "transform_with_group"
	RouteHandlingInvalidateRebuild  RouteHandlingPolicy = "invalidate_and_rebuild"
	RouteHandlingPreserveFixed      RouteHandlingPolicy = "preserve_fixed"
	RouteHandlingUnsupported        RouteHandlingPolicy = "unsupported"
)

type MobilityPolicy struct {
	Class         MobilityClass       `json:"class,omitempty"`
	Reason        string              `json:"reason,omitempty"`
	OwnerScope    string              `json:"owner_scope,omitempty"`
	GroupID       string              `json:"group_id,omitempty"`
	Transforms    []string            `json:"transforms,omitempty"`
	RouteHandling RouteHandlingPolicy `json:"route_handling,omitempty"`
	Constraints   []string            `json:"constraints,omitempty"`
}

type MobilitySummary struct {
	Total                 int            `json:"total"`
	ByClass               map[string]int `json:"by_class,omitempty"`
	EligibleCount         int            `json:"eligible_count"`
	FixedCount            int            `json:"fixed_count"`
	UnownedCount          int            `json:"unowned_count"`
	GroupTransformCount   int            `json:"group_transform_count"`
	LocalRebuildCount     int            `json:"local_rebuild_count"`
	SoftPreferredCount    int            `json:"soft_preferred_count"`
	TransformableRouteCnt int            `json:"transformable_route_count"`
	RebuildableRouteCnt   int            `json:"rebuildable_route_count"`
	PreservedRouteCnt     int            `json:"preserved_route_count"`
	UnsupportedRouteCnt   int            `json:"unsupported_route_count"`
}

type ScoreReport struct {
	Total      float64          `json:"total"`
	Dimensions []ScoreDimension `json:"dimensions,omitempty"`
}

type ScoreDimension struct {
	Name       string   `json:"name"`
	Score      float64  `json:"score"`
	Weight     float64  `json:"weight"`
	Status     string   `json:"status"`
	Refs       []string `json:"refs,omitempty"`
	Groups     []string `json:"groups,omitempty"`
	Nets       []string `json:"nets,omitempty"`
	Message    string   `json:"message,omitempty"`
	Suggestion string   `json:"suggestion,omitempty"`
}

type CandidateScoreDimensionName string

const (
	CandidateScoreHardConstraints     CandidateScoreDimensionName = "hard_constraints"
	CandidateScoreSemanticRole        CandidateScoreDimensionName = "semantic_role"
	CandidateScoreGroupCohesion       CandidateScoreDimensionName = "group_cohesion"
	CandidateScoreElectricalProximity CandidateScoreDimensionName = "electrical_proximity"
	CandidateScoreRouteLength         CandidateScoreDimensionName = "route_length"
	CandidateScoreCongestion          CandidateScoreDimensionName = "congestion"
	CandidateScoreFanout              CandidateScoreDimensionName = "fanout"
	CandidateScoreEdge                CandidateScoreDimensionName = "edge"
	CandidateScoreRegion              CandidateScoreDimensionName = "region"
	CandidateScoreMobility            CandidateScoreDimensionName = "mobility"
	CandidateScoreThermal             CandidateScoreDimensionName = "thermal"
	CandidateScoreHighCurrent         CandidateScoreDimensionName = "high_current"
	CandidateScoreCreepageClearance   CandidateScoreDimensionName = "creepage_clearance"
	CandidateScoreDifferentialPair    CandidateScoreDimensionName = "differential_pair"
	CandidateScoreControlledImpedance CandidateScoreDimensionName = "controlled_impedance"
	CandidateScoreTimingSensitive     CandidateScoreDimensionName = "timing_sensitive"
)

type CandidateRejectionReasonName string

const (
	CandidateRejectOutsideBoard      CandidateRejectionReasonName = "outside_board"
	CandidateRejectCollision         CandidateRejectionReasonName = "collision"
	CandidateRejectKeepout           CandidateRejectionReasonName = "keepout"
	CandidateRejectMobility          CandidateRejectionReasonName = "mobility"
	CandidateRejectEdge              CandidateRejectionReasonName = "edge"
	CandidateRejectSide              CandidateRejectionReasonName = "side"
	CandidateRejectRotation          CandidateRejectionReasonName = "rotation"
	CandidateRejectGroupConstraint   CandidateRejectionReasonName = "group_constraint"
	CandidateRejectMissingGeometry   CandidateRejectionReasonName = "missing_geometry"
	CandidateRejectUnsupportedPolicy CandidateRejectionReasonName = "unsupported_policy"
	CandidateRejectAdvancedRule      CandidateRejectionReasonName = "advanced_rule"
)

type CandidateScoringReport struct {
	Enabled               bool                      `json:"enabled"`
	Policy                string                    `json:"policy,omitempty"`
	ScoreVersion          string                    `json:"score_version,omitempty"`
	AverageWinningScore   float64                   `json:"average_winning_score,omitempty"`
	LowestWinningScore    float64                   `json:"lowest_winning_score,omitempty"`
	RejectedCount         int                       `json:"rejected_count,omitempty"`
	RejectedByReason      map[string]int            `json:"rejected_by_reason,omitempty"`
	AggregateDimensions   []CandidateScoreDimension `json:"aggregate_dimensions,omitempty"`
	TopPenalties          []CandidateScoreDimension `json:"top_penalties,omitempty"`
	WinningCandidates     []CandidateScore          `json:"winning_candidates,omitempty"`
	AlternativeCandidates []CandidateScore          `json:"alternative_candidates,omitempty"`
	rejectedSamplesByRef  map[string]int
}

type CandidateScore struct {
	Ref        string                     `json:"ref"`
	Role       string                     `json:"role,omitempty"`
	Index      int                        `json:"index"`
	Placement  Placement                  `json:"placement"`
	Total      float64                    `json:"total"`
	Dimensions []CandidateScoreDimension  `json:"dimensions,omitempty"`
	Rejected   bool                       `json:"rejected,omitempty"`
	Reasons    []CandidateRejectionReason `json:"reasons,omitempty"`
	Evidence   []string                   `json:"evidence,omitempty"`
}

type CandidateScoreDimension struct {
	Name     CandidateScoreDimensionName `json:"name"`
	Score    float64                     `json:"score"`
	Weight   float64                     `json:"weight"`
	Evidence []string                    `json:"evidence,omitempty"`
}

type CandidateRejectionReason struct {
	Name     CandidateRejectionReasonName `json:"name"`
	Severity reports.Severity             `json:"severity,omitempty"`
	Message  string                       `json:"message,omitempty"`
	Refs     []string                     `json:"refs,omitempty"`
}

func DefaultRules() Rules {
	return Rules{
		GridMM:                   0.5,
		ComponentSpacingMM:       0.5,
		BoardEdgeClearanceMM:     1.0,
		GroupSpacingMM:           1.0,
		PreferTopLayer:           true,
		AllowBackLayer:           false,
		ConnectorEdgeClearanceMM: 1.0,
		MaxCandidatesPerPart:     5000,
		CandidateScoring: CandidateScoringRules{
			Policy:                      defaultCandidateScoringPolicy,
			MaxAlternativesPerComponent: defaultCandidateAlternativesPerComponent,
			MaxEvidencePerDimension:     defaultCandidateScoreEvidencePerDimension,
			Weights: CandidateScoreWeights{
				HardConstraints:     1,
				SemanticRole:        1,
				GroupCohesion:       1,
				ElectricalProximity: 1,
				RouteLength:         1,
				Congestion:          1,
				Fanout:              1,
				Edge:                1,
				Region:              1,
				Mobility:            1,
				Thermal:             1,
				HighCurrent:         1,
				CreepageClearance:   1,
				DifferentialPair:    1,
				ControlledImpedance: 1,
				TimingSensitive:     1,
			},
		},
	}
}

func NormalizeRequest(request Request) Request {
	request.Components = slices.Clone(request.Components)
	request.Nets = slices.Clone(request.Nets)
	request.Groups = slices.Clone(request.Groups)
	request.Keepouts = slices.Clone(request.Keepouts)
	request.Mechanical = slices.Clone(request.Mechanical)
	request.ProximityRules = slices.Clone(request.ProximityRules)
	request.RegionRules = slices.Clone(request.RegionRules)
	request.AdvancedRules = normalizeAdvancedPlacementRules(request.AdvancedRules)
	request.Rules = normalizeRules(request.Rules)
	for i := range request.Components {
		request.Components[i].Pads = slices.Clone(request.Components[i].Pads)
		request.Components[i].Hints = slices.Clone(request.Components[i].Hints)
		request.Components[i].Rotation.AllowedDeg = slices.Clone(request.Components[i].Rotation.AllowedDeg)
		for j := range request.Components[i].Pads {
			request.Components[i].Pads[j].Name = strings.TrimSpace(request.Components[i].Pads[j].Name)
			request.Components[i].Pads[j].Net = strings.TrimSpace(request.Components[i].Pads[j].Net)
		}
		if request.Components[i].Rotation.FixedDeg != nil {
			rotation := *request.Components[i].Rotation.FixedDeg
			request.Components[i].Rotation.FixedDeg = &rotation
		}
		if request.Components[i].Position != nil {
			position := *request.Components[i].Position
			request.Components[i].Position = &position
		}
		if request.Existing.PreserveFixed && request.Components[i].Position != nil {
			request.Components[i].Fixed = true
		}
		request.Components[i].Ref = strings.TrimSpace(request.Components[i].Ref)
		request.Components[i].FootprintID = strings.TrimSpace(request.Components[i].FootprintID)
		request.Components[i].GroupID = strings.TrimSpace(request.Components[i].GroupID)
		request.Components[i].Mobility = normalizeMobilityPolicy(request.Components[i])
		if request.Components[i].Side == "" {
			request.Components[i].Side = SideTop
		}
		if request.Components[i].Side == SideAny && !request.Rules.AllowBackLayer {
			request.Components[i].Side = SideTop
		}
		if len(request.Components[i].Rotation.AllowedDeg) == 0 && request.Components[i].Rotation.FixedDeg == nil {
			request.Components[i].Rotation.AllowedDeg = []float64{0, 90, 180, 270}
		}
	}
	for i := range request.Nets {
		request.Nets[i].Endpoints = slices.Clone(request.Nets[i].Endpoints)
		request.Nets[i].Name = strings.TrimSpace(request.Nets[i].Name)
		for j := range request.Nets[i].Endpoints {
			request.Nets[i].Endpoints[j].Ref = strings.TrimSpace(request.Nets[i].Endpoints[j].Ref)
			request.Nets[i].Endpoints[j].Pin = strings.TrimSpace(request.Nets[i].Endpoints[j].Pin)
		}
		if request.Nets[i].Role == "" {
			request.Nets[i].Role = NetUnknown
		}
	}
	for i := range request.Groups {
		request.Groups[i].Components = slices.Clone(request.Groups[i].Components)
		if request.Groups[i].Anchor.At != nil {
			at := *request.Groups[i].Anchor.At
			request.Groups[i].Anchor.At = &at
		}
		request.Groups[i].ID = strings.TrimSpace(request.Groups[i].ID)
		request.Groups[i].Anchor.Ref = strings.TrimSpace(request.Groups[i].Anchor.Ref)
		for j := range request.Groups[i].Components {
			request.Groups[i].Components[j] = strings.TrimSpace(request.Groups[i].Components[j])
		}
		slices.Sort(request.Groups[i].Components)
	}
	keepouts := make([]Keepout, 0, len(request.Keepouts)+len(request.Mechanical))
	for i := range request.Keepouts {
		if request.Keepouts[i].Mechanical {
			continue
		}
		request.Keepouts[i].Layers = slices.Clone(request.Keepouts[i].Layers)
		request.Keepouts[i].ID = strings.TrimSpace(request.Keepouts[i].ID)
		keepouts = append(keepouts, request.Keepouts[i])
	}
	request.Keepouts = keepouts
	request.Keepouts = slices.Grow(request.Keepouts, len(request.Mechanical))
	for i := range request.Mechanical {
		request.Mechanical[i].Layers = slices.Clone(request.Mechanical[i].Layers)
		request.Mechanical[i].ID = stableRuleID(request.Mechanical[i].ID, "mechanical", i)
		request.Mechanical[i].Kind = strings.TrimSpace(request.Mechanical[i].Kind)
		request.Mechanical[i].Reason = strings.TrimSpace(request.Mechanical[i].Reason)
		request.Keepouts = append(request.Keepouts, Keepout{
			ID:         request.Mechanical[i].ID,
			Bounds:     request.Mechanical[i].Bounds,
			Layers:     request.Mechanical[i].Layers,
			Reason:     firstNonEmpty(request.Mechanical[i].Reason, request.Mechanical[i].Kind),
			Optional:   request.Mechanical[i].Optional,
			Mechanical: true,
		})
	}
	for i := range request.ProximityRules {
		request.ProximityRules[i].ID = stableRuleID(request.ProximityRules[i].ID, "proximity", i)
		request.ProximityRules[i].Source = strings.TrimSpace(request.ProximityRules[i].Source)
		request.ProximityRules[i].AnchorRef = normalizeRef(request.ProximityRules[i].AnchorRef)
		if request.ProximityRules[i].Weight <= 0 {
			request.ProximityRules[i].Weight = 1
		}
		request.ProximityRules[i].TargetRefs = uniqueSortedRefs(request.ProximityRules[i].TargetRefs)
		request.ProximityRules[i].AnchorPins = uniqueSortedStrings(request.ProximityRules[i].AnchorPins)
		request.ProximityRules[i].TargetPins = uniqueSortedStrings(request.ProximityRules[i].TargetPins)
	}
	for i := range request.RegionRules {
		request.RegionRules[i].NetRoles = slices.Clone(request.RegionRules[i].NetRoles)
		request.RegionRules[i].ID = stableRuleID(request.RegionRules[i].ID, "region", i)
		request.RegionRules[i].Source = strings.TrimSpace(request.RegionRules[i].Source)
		request.RegionRules[i].Region = strings.TrimSpace(request.RegionRules[i].Region)
		if request.RegionRules[i].Weight <= 0 {
			request.RegionRules[i].Weight = 1
		}
		request.RegionRules[i].Refs = uniqueSortedStrings(request.RegionRules[i].Refs)
	}
	return request
}

func normalizeAdvancedPlacementRules(rules AdvancedPlacementRules) AdvancedPlacementRules {
	rules.Thermal = slices.Clone(rules.Thermal)
	for i := range rules.Thermal {
		rules.Thermal[i].ID = strings.TrimSpace(rules.Thermal[i].ID)
		rules.Thermal[i].Source = strings.TrimSpace(rules.Thermal[i].Source)
		rules.Thermal[i].ThermalRole = ThermalRole(strings.ToLower(strings.TrimSpace(string(rules.Thermal[i].ThermalRole))))
		rules.Thermal[i].PreferredRegion = strings.TrimSpace(rules.Thermal[i].PreferredRegion)
		rules.Thermal[i].Refs = uniqueSortedStrings(rules.Thermal[i].Refs)
		rules.Thermal[i].Roles = uniqueSortedStrings(rules.Thermal[i].Roles)
		rules.Thermal[i].KeepAwayRefs = uniqueSortedStrings(rules.Thermal[i].KeepAwayRefs)
		rules.Thermal[i].KeepAwayRoles = uniqueSortedStrings(rules.Thermal[i].KeepAwayRoles)
		rules.Thermal[i].Enforcement = normalizeAdvancedRuleEnforcement(rules.Thermal[i].Enforcement)
		rules.Thermal[i].Severity = normalizeAdvancedRuleSeverity(rules.Thermal[i].Severity, rules.Thermal[i].Enforcement)
	}
	slices.SortStableFunc(rules.Thermal, func(left, right ThermalPlacementRule) int {
		return compareOrdered(thermalRuleSortKey(left), thermalRuleSortKey(right))
	})
	for i := range rules.Thermal {
		rules.Thermal[i].ID = stableRuleID(rules.Thermal[i].ID, "thermal", i)
	}
	rules.HighCurrent = slices.Clone(rules.HighCurrent)
	for i := range rules.HighCurrent {
		rules.HighCurrent[i].ID = strings.TrimSpace(rules.HighCurrent[i].ID)
		rules.HighCurrent[i].Source = strings.TrimSpace(rules.HighCurrent[i].Source)
		rules.HighCurrent[i].CurrentClass = strings.TrimSpace(rules.HighCurrent[i].CurrentClass)
		rules.HighCurrent[i].PreferredLayer = strings.TrimSpace(rules.HighCurrent[i].PreferredLayer)
		rules.HighCurrent[i].Nets = uniqueSortedStrings(rules.HighCurrent[i].Nets)
		rules.HighCurrent[i].NetRoles = normalizeNetRoles(rules.HighCurrent[i].NetRoles)
		rules.HighCurrent[i].SourceRefs = uniqueSortedStrings(rules.HighCurrent[i].SourceRefs)
		rules.HighCurrent[i].SinkRefs = uniqueSortedStrings(rules.HighCurrent[i].SinkRefs)
		rules.HighCurrent[i].SourcePads = uniqueSortedStrings(rules.HighCurrent[i].SourcePads)
		rules.HighCurrent[i].SinkPads = uniqueSortedStrings(rules.HighCurrent[i].SinkPads)
		rules.HighCurrent[i].KeepAwayRefs = uniqueSortedStrings(rules.HighCurrent[i].KeepAwayRefs)
		rules.HighCurrent[i].KeepAwayRoles = uniqueSortedStrings(rules.HighCurrent[i].KeepAwayRoles)
		rules.HighCurrent[i].Enforcement = normalizeAdvancedRuleEnforcement(rules.HighCurrent[i].Enforcement)
		rules.HighCurrent[i].Severity = normalizeAdvancedRuleSeverity(rules.HighCurrent[i].Severity, rules.HighCurrent[i].Enforcement)
	}
	slices.SortStableFunc(rules.HighCurrent, func(left, right HighCurrentPlacementRule) int {
		return compareOrdered(highCurrentRuleSortKey(left), highCurrentRuleSortKey(right))
	})
	for i := range rules.HighCurrent {
		rules.HighCurrent[i].ID = stableRuleID(rules.HighCurrent[i].ID, "high-current", i)
	}
	rules.CreepageClearance = slices.Clone(rules.CreepageClearance)
	for i := range rules.CreepageClearance {
		rules.CreepageClearance[i].ID = strings.TrimSpace(rules.CreepageClearance[i].ID)
		rules.CreepageClearance[i].Source = strings.TrimSpace(rules.CreepageClearance[i].Source)
		rules.CreepageClearance[i].Voltage = strings.TrimSpace(rules.CreepageClearance[i].Voltage)
		rules.CreepageClearance[i].InsulationClass = strings.TrimSpace(rules.CreepageClearance[i].InsulationClass)
		rules.CreepageClearance[i].BoardSides = uniqueSortedStrings(rules.CreepageClearance[i].BoardSides)
		rules.CreepageClearance[i].DomainA = normalizePlacementRuleDomain(rules.CreepageClearance[i].DomainA)
		rules.CreepageClearance[i].DomainB = normalizePlacementRuleDomain(rules.CreepageClearance[i].DomainB)
		rules.CreepageClearance[i].Enforcement = normalizeAdvancedRuleEnforcement(rules.CreepageClearance[i].Enforcement)
		rules.CreepageClearance[i].Severity = normalizeAdvancedRuleSeverity(rules.CreepageClearance[i].Severity, rules.CreepageClearance[i].Enforcement)
	}
	slices.SortStableFunc(rules.CreepageClearance, func(left, right CreepageClearancePlacementRule) int {
		return compareOrdered(creepageClearanceRuleSortKey(left), creepageClearanceRuleSortKey(right))
	})
	for i := range rules.CreepageClearance {
		rules.CreepageClearance[i].ID = stableRuleID(rules.CreepageClearance[i].ID, "creepage-clearance", i)
	}
	rules.DifferentialPair = slices.Clone(rules.DifferentialPair)
	for i := range rules.DifferentialPair {
		rules.DifferentialPair[i].ID = strings.TrimSpace(rules.DifferentialPair[i].ID)
		rules.DifferentialPair[i].Source = strings.TrimSpace(rules.DifferentialPair[i].Source)
		rules.DifferentialPair[i].PositiveNet = strings.TrimSpace(rules.DifferentialPair[i].PositiveNet)
		rules.DifferentialPair[i].NegativeNet = strings.TrimSpace(rules.DifferentialPair[i].NegativeNet)
		rules.DifferentialPair[i].PreferredOrientation = strings.TrimSpace(rules.DifferentialPair[i].PreferredOrientation)
		rules.DifferentialPair[i].PairGroupID = strings.TrimSpace(rules.DifferentialPair[i].PairGroupID)
		rules.DifferentialPair[i].SourceRefs = uniqueSortedStrings(rules.DifferentialPair[i].SourceRefs)
		rules.DifferentialPair[i].SinkRefs = uniqueSortedStrings(rules.DifferentialPair[i].SinkRefs)
		rules.DifferentialPair[i].SourcePads = uniqueSortedStrings(rules.DifferentialPair[i].SourcePads)
		rules.DifferentialPair[i].SinkPads = uniqueSortedStrings(rules.DifferentialPair[i].SinkPads)
		rules.DifferentialPair[i].Enforcement = normalizeAdvancedRuleEnforcement(rules.DifferentialPair[i].Enforcement)
		rules.DifferentialPair[i].Severity = normalizeAdvancedRuleSeverity(rules.DifferentialPair[i].Severity, rules.DifferentialPair[i].Enforcement)
	}
	slices.SortStableFunc(rules.DifferentialPair, func(left, right DifferentialPairPlacementRule) int {
		return compareOrdered(differentialPairRuleSortKey(left), differentialPairRuleSortKey(right))
	})
	for i := range rules.DifferentialPair {
		rules.DifferentialPair[i].ID = stableRuleID(rules.DifferentialPair[i].ID, "differential-pair", i)
	}
	rules.ControlledImpedance = slices.Clone(rules.ControlledImpedance)
	for i := range rules.ControlledImpedance {
		rules.ControlledImpedance[i].ID = strings.TrimSpace(rules.ControlledImpedance[i].ID)
		rules.ControlledImpedance[i].Source = strings.TrimSpace(rules.ControlledImpedance[i].Source)
		rules.ControlledImpedance[i].ReferencePlane = strings.TrimSpace(rules.ControlledImpedance[i].ReferencePlane)
		rules.ControlledImpedance[i].Nets = uniqueSortedStrings(rules.ControlledImpedance[i].Nets)
		rules.ControlledImpedance[i].NetRoles = normalizeNetRoles(rules.ControlledImpedance[i].NetRoles)
		rules.ControlledImpedance[i].PreferredLayers = uniqueSortedStrings(rules.ControlledImpedance[i].PreferredLayers)
		rules.ControlledImpedance[i].SourceRefs = uniqueSortedStrings(rules.ControlledImpedance[i].SourceRefs)
		rules.ControlledImpedance[i].SinkRefs = uniqueSortedStrings(rules.ControlledImpedance[i].SinkRefs)
		rules.ControlledImpedance[i].SourcePads = uniqueSortedStrings(rules.ControlledImpedance[i].SourcePads)
		rules.ControlledImpedance[i].SinkPads = uniqueSortedStrings(rules.ControlledImpedance[i].SinkPads)
		rules.ControlledImpedance[i].Enforcement = normalizeAdvancedRuleEnforcement(rules.ControlledImpedance[i].Enforcement)
		rules.ControlledImpedance[i].Severity = normalizeAdvancedRuleSeverity(rules.ControlledImpedance[i].Severity, rules.ControlledImpedance[i].Enforcement)
	}
	slices.SortStableFunc(rules.ControlledImpedance, func(left, right ControlledImpedancePlacementRule) int {
		return compareOrdered(controlledImpedanceRuleSortKey(left), controlledImpedanceRuleSortKey(right))
	})
	for i := range rules.ControlledImpedance {
		rules.ControlledImpedance[i].ID = stableRuleID(rules.ControlledImpedance[i].ID, "controlled-impedance", i)
	}
	return rules
}

func thermalRuleSortKey(rule ThermalPlacementRule) string {
	return strings.Join([]string{
		rule.ID,
		rule.Source,
		strings.Join(rule.Refs, ","),
		strings.Join(rule.Roles, ","),
		string(rule.ThermalRole),
		string(rule.PreferredEdge),
		rule.PreferredRegion,
		strings.Join(rule.KeepAwayRefs, ","),
		strings.Join(rule.KeepAwayRoles, ","),
		fmt.Sprintf("%.6f", rule.MinDistanceMM),
		fmt.Sprintf("%t", rule.PreferCopper),
		string(rule.Severity),
		string(rule.Enforcement),
	}, "|")
}

func highCurrentRuleSortKey(rule HighCurrentPlacementRule) string {
	return strings.Join([]string{
		rule.ID,
		rule.Source,
		strings.Join(rule.Nets, ","),
		netRolesSortKey(rule.NetRoles),
		rule.CurrentClass,
		fmt.Sprintf("%.6f", rule.CurrentEstimateA),
		strings.Join(rule.SourceRefs, ","),
		strings.Join(rule.SinkRefs, ","),
		strings.Join(rule.SourcePads, ","),
		strings.Join(rule.SinkPads, ","),
		fmt.Sprintf("%.6f", rule.MaxPreferredLengthMM),
		rule.PreferredLayer,
		strings.Join(rule.KeepAwayRefs, ","),
		strings.Join(rule.KeepAwayRoles, ","),
		string(rule.Severity),
		string(rule.Enforcement),
	}, "|")
}

func creepageClearanceRuleSortKey(rule CreepageClearancePlacementRule) string {
	return strings.Join([]string{
		rule.ID,
		rule.Source,
		placementRuleDomainSortKey(rule.DomainA),
		placementRuleDomainSortKey(rule.DomainB),
		fmt.Sprintf("%.6f", rule.MinClearanceMM),
		fmt.Sprintf("%.6f", rule.MinCreepageMM),
		rule.Voltage,
		rule.InsulationClass,
		strings.Join(rule.BoardSides, ","),
		string(rule.Severity),
		string(rule.Enforcement),
	}, "|")
}

func differentialPairRuleSortKey(rule DifferentialPairPlacementRule) string {
	return strings.Join([]string{
		rule.ID,
		rule.Source,
		rule.PositiveNet,
		rule.NegativeNet,
		strings.Join(rule.SourceRefs, ","),
		strings.Join(rule.SinkRefs, ","),
		strings.Join(rule.SourcePads, ","),
		strings.Join(rule.SinkPads, ","),
		rule.PreferredOrientation,
		fmt.Sprintf("%.6f", rule.MaxSkewMM),
		fmt.Sprintf("%.6f", rule.LengthToleranceMM),
		rule.PairGroupID,
		string(rule.Severity),
		string(rule.Enforcement),
	}, "|")
}

func controlledImpedanceRuleSortKey(rule ControlledImpedancePlacementRule) string {
	return strings.Join([]string{
		rule.ID,
		rule.Source,
		strings.Join(rule.Nets, ","),
		netRolesSortKey(rule.NetRoles),
		strings.Join(rule.PreferredLayers, ","),
		fmt.Sprintf("%.6f", rule.MinCorridorWidthMM),
		strings.Join(rule.SourceRefs, ","),
		strings.Join(rule.SinkRefs, ","),
		strings.Join(rule.SourcePads, ","),
		strings.Join(rule.SinkPads, ","),
		fmt.Sprintf("%d", rule.MaxViaCountPreference),
		rule.ReferencePlane,
		string(rule.Severity),
		string(rule.Enforcement),
	}, "|")
}

func placementRuleDomainSortKey(domain PlacementRuleDomain) string {
	return strings.Join([]string{
		strings.Join(domain.Refs, ","),
		strings.Join(domain.Nets, ","),
		strings.Join(domain.NetClasses, ","),
		strings.Join(domain.Roles, ","),
		netRolesSortKey(domain.NetRoles),
	}, "/")
}

func netRolesSortKey(roles []NetRole) string {
	values := make([]string, 0, len(roles))
	for _, role := range roles {
		values = append(values, string(role))
	}
	return strings.Join(values, ",")
}

func normalizePlacementRuleDomain(domain PlacementRuleDomain) PlacementRuleDomain {
	domain.Refs = uniqueSortedStrings(domain.Refs)
	domain.Nets = uniqueSortedStrings(domain.Nets)
	domain.NetClasses = uniqueSortedStrings(domain.NetClasses)
	domain.Roles = uniqueSortedStrings(domain.Roles)
	domain.NetRoles = normalizeNetRoles(domain.NetRoles)
	return domain
}

func normalizeNetRoles(roles []NetRole) []NetRole {
	if len(roles) == 0 {
		return nil
	}
	seen := map[NetRole]struct{}{}
	out := make([]NetRole, 0, len(roles))
	for _, role := range roles {
		if role == "" {
			continue
		}
		if _, ok := seen[role]; ok {
			continue
		}
		seen[role] = struct{}{}
		out = append(out, role)
	}
	slices.SortFunc(out, func(left, right NetRole) int {
		return compareOrdered(left, right)
	})
	return out
}

func normalizeAdvancedRuleEnforcement(enforcement AdvancedRuleEnforcement) AdvancedRuleEnforcement {
	switch strings.ToLower(strings.TrimSpace(string(enforcement))) {
	case string(AdvancedRuleHard):
		return AdvancedRuleHard
	default:
		return AdvancedRuleSoft
	}
}

func normalizeAdvancedRuleSeverity(severity AdvancedRuleSeverity, enforcement AdvancedRuleEnforcement) AdvancedRuleSeverity {
	switch strings.ToLower(strings.TrimSpace(string(severity))) {
	case string(AdvancedRuleSeverityInfo), string(AdvancedRuleSeverityWarning), string(AdvancedRuleSeverityError):
		return AdvancedRuleSeverity(strings.ToLower(strings.TrimSpace(string(severity))))
	default:
		if enforcement == AdvancedRuleHard {
			return AdvancedRuleSeverityError
		}
		return AdvancedRuleSeverityWarning
	}
}

func normalizeRules(rules Rules) Rules {
	defaults := DefaultRules()
	if rules.GridMM <= 0 {
		rules.GridMM = defaults.GridMM
	}
	if rules.ComponentSpacingMM <= 0 {
		rules.ComponentSpacingMM = defaults.ComponentSpacingMM
	}
	if rules.BoardEdgeClearanceMM <= 0 {
		rules.BoardEdgeClearanceMM = defaults.BoardEdgeClearanceMM
	}
	if rules.GroupSpacingMM <= 0 {
		rules.GroupSpacingMM = defaults.GroupSpacingMM
	}
	if rules.ConnectorEdgeClearanceMM <= 0 {
		rules.ConnectorEdgeClearanceMM = defaults.ConnectorEdgeClearanceMM
	}
	if rules.MaxCandidatesPerPart <= 0 {
		rules.MaxCandidatesPerPart = defaults.MaxCandidatesPerPart
	} else if rules.MaxCandidatesPerPart > maxCandidatesPerPartLimit {
		rules.MaxCandidatesPerPart = maxCandidatesPerPartLimit
	}
	rules.CandidateScoring = normalizeCandidateScoringRules(rules.CandidateScoring)
	return rules
}

func Validate(request Request) []reports.Issue {
	request.Rules = normalizeRules(request.Rules)
	request.AdvancedRules = normalizeAdvancedPlacementRules(request.AdvancedRules)
	var issues []reports.Issue
	if request.Board.WidthMM <= 0 {
		issues = append(issues, issue("board.width_mm", "board width must be positive"))
	}
	if request.Board.HeightMM <= 0 {
		issues = append(issues, issue("board.height_mm", "board height must be positive"))
	}
	if request.Board.MarginMM < 0 {
		issues = append(issues, issue("board.margin_mm", "board margin must be non-negative"))
	}
	if request.Board.WidthMM > 0 && request.Board.HeightMM > 0 && request.Board.MarginMM*2 >= min(request.Board.WidthMM, request.Board.HeightMM) {
		issues = append(issues, issue("board.margin_mm", "board margin leaves no usable placement area"))
	}
	refs := map[string]Component{}
	for i, component := range request.Components {
		path := fmt.Sprintf("components[%d]", i)
		ref := strings.TrimSpace(component.Ref)
		if ref == "" {
			issues = append(issues, issue(path+".ref", "component reference required"))
			continue
		}
		key := strings.ToUpper(ref)
		if existing, ok := refs[key]; ok {
			issues = append(issues, issue(path+".ref", "duplicate component reference "+ref+" collides with "+strings.TrimSpace(existing.Ref)))
		}
		refs[key] = component
		hasFootprint := strings.TrimSpace(component.FootprintID) != ""
		hasBounds := component.Bounds.WidthMM > 0 && component.Bounds.HeightMM > 0
		hasPartialBounds := component.Bounds.WidthMM > 0 || component.Bounds.HeightMM > 0
		if !hasFootprint && !hasBounds {
			issues = append(issues, issue(path+".footprint_id", "footprint id or explicit bounds required"))
		}
		if hasPartialBounds && !hasBounds {
			issues = append(issues, issue(path+".bounds", "component bounds must be positive when provided"))
		}
		if component.Fixed && component.Position == nil {
			issues = append(issues, issue(path+".position", "fixed component requires position"))
		}
		if component.Side == SideBottom && !request.Rules.AllowBackLayer {
			issues = append(issues, issue(path+".side", "bottom placement requires AllowBackLayer"))
		}
		if !validSide(component.Side) {
			issues = append(issues, issue(path+".side", "invalid side constraint "+string(component.Side)))
		}
		if !validEdge(component.Edge) {
			issues = append(issues, issue(path+".edge", "invalid edge constraint "+string(component.Edge)))
		}
		if !validBoundsSource(component.Bounds.Source) {
			issues = append(issues, issue(path+".bounds.source", "invalid bounds source "+string(component.Bounds.Source)))
		}
		if err := validateRotation(component.Rotation); err != nil {
			issues = append(issues, issue(path+".rotation", err.Error()))
		}
	}
	groupIDs := map[string]int{}
	for i, group := range request.Groups {
		path := fmt.Sprintf("groups[%d]", i)
		id := strings.TrimSpace(group.ID)
		if id == "" {
			issues = append(issues, issue(path+".id", "group id required"))
		} else {
			key := strings.ToUpper(id)
			if previous, ok := groupIDs[key]; ok {
				issues = append(issues, issue(path+".id", fmt.Sprintf("duplicate group ID %s already defined at index %d", id, previous)))
			}
			groupIDs[key] = i
		}
		for _, ref := range group.Components {
			trimmedRef := strings.TrimSpace(ref)
			component, ok := refs[strings.ToUpper(trimmedRef)]
			if !ok {
				issues = append(issues, issue(path+".components", "group references unknown component "+trimmedRef))
				continue
			}
			componentGroup := strings.TrimSpace(component.GroupID)
			if componentGroup != "" && !strings.EqualFold(componentGroup, id) {
				issues = append(issues, issue(path+".components", fmt.Sprintf("component %s has group ID %s but is listed in group %s", trimmedRef, componentGroup, id)))
			}
		}
		if group.Anchor.Ref != "" {
			trimmedRef := strings.TrimSpace(group.Anchor.Ref)
			if _, ok := refs[strings.ToUpper(trimmedRef)]; !ok {
				issues = append(issues, issue(path+".anchor.ref", "group anchor references unknown component "+trimmedRef))
			}
		}
	}
	netNames := map[string]int{}
	netClasses := map[string]int{}
	for i, net := range request.Nets {
		path := fmt.Sprintf("nets[%d]", i)
		name := strings.TrimSpace(net.Name)
		if name == "" {
			issues = append(issues, issue(path+".name", "net name required"))
		} else {
			key := strings.ToUpper(name)
			if previous, ok := netNames[key]; ok {
				issues = append(issues, issue(path+".name", fmt.Sprintf("duplicate net name %s already defined at index %d", name, previous)))
			}
			netNames[key] = i
		}
		if !validNetRole(net.Role) {
			issues = append(issues, issue(path+".role", "invalid net role "+string(net.Role)))
		}
		widthClass := strings.TrimSpace(net.WidthClass)
		if widthClass != "" {
			netClasses[strings.ToUpper(widthClass)]++
		}
		for endpointIndex, endpoint := range net.Endpoints {
			endpointRef := strings.TrimSpace(endpoint.Ref)
			component, ok := refs[strings.ToUpper(endpointRef)]
			if !ok {
				issues = append(issues, issue(path+".endpoints", "net endpoint references unknown component "+endpointRef))
			}
			pin := strings.TrimSpace(endpoint.Pin)
			if pin == "" {
				issues = append(issues, issue(path+".endpoints", "net endpoint pin required"))
			} else if ok && len(component.Pads) > 0 && !componentHasPad(component, pin) {
				issues = append(issues, issue(fmt.Sprintf("%s.endpoints[%d].pin", path, endpointIndex), "pin "+pin+" not found in component "+endpointRef))
			}
		}
	}
	for i, keepout := range request.Keepouts {
		if keepout.Mechanical {
			continue
		}
		path := fmt.Sprintf("keepouts[%d].bounds", i)
		if keepout.Bounds.Min.XMM > keepout.Bounds.Max.XMM || keepout.Bounds.Min.YMM > keepout.Bounds.Max.YMM {
			issues = append(issues, issue(path, "keepout bounds min must not exceed max"))
		}
	}
	for i, mechanical := range request.Mechanical {
		path := fmt.Sprintf("mechanical[%d]", i)
		if strings.TrimSpace(mechanical.Kind) == "" {
			issues = append(issues, issue(path+".kind", "mechanical constraint kind required"))
		}
		if mechanical.Bounds.Min.XMM > mechanical.Bounds.Max.XMM || mechanical.Bounds.Min.YMM > mechanical.Bounds.Max.YMM {
			issues = append(issues, issue(path+".bounds", "mechanical constraint bounds min must not exceed max"))
		}
	}
	issues = append(issues, validateProximityRules(request.ProximityRules, refs)...)
	issues = append(issues, validateRegionRules(request.RegionRules, refs)...)
	issues = append(issues, validateAdvancedPlacementRules(request.AdvancedRules, refs, netNames, netClasses, request.RegionRules)...)
	return issues
}

func validateAdvancedPlacementRules(rules AdvancedPlacementRules, refs map[string]Component, netNames map[string]int, netClasses map[string]int, regionRules []RegionRule) []reports.Issue {
	var issues []reports.Issue
	ids := map[string]int{}
	regions := placementRegionNames(regionRules)
	for i, rule := range rules.Thermal {
		path := fmt.Sprintf("advanced_rules.thermal[%d]", i)
		issues = append(issues, validateAdvancedRuleID(ids, path, rule.ID)...)
		if !validThermalRole(rule.ThermalRole) {
			issues = append(issues, advancedRuleIssue(rule, path+".thermal_role", "invalid thermal role "+string(rule.ThermalRole)))
		}
		if !validEdge(rule.PreferredEdge) {
			issues = append(issues, advancedRuleIssue(rule, path+".preferred_edge", "invalid preferred edge "+string(rule.PreferredEdge)))
		}
		if len(rule.Refs) == 0 && len(rule.Roles) == 0 {
			issues = append(issues, advancedRuleIssue(rule, path+".refs", "thermal rule requires refs or roles"))
		}
		if rule.PreferredRegion != "" {
			if _, ok := regions[strings.ToUpper(rule.PreferredRegion)]; !ok {
				issues = append(issues, advancedRuleIssue(rule, path+".preferred_region", "thermal preferred region references unknown region "+rule.PreferredRegion))
			}
		}
		issues = append(issues, validateKnownRefs(rule, path+".refs", rule.Refs, refs)...)
		issues = append(issues, validateKnownRefs(rule, path+".keep_away_refs", rule.KeepAwayRefs, refs)...)
		if rule.MinDistanceMM < 0 {
			issues = append(issues, advancedRuleIssue(rule, path+".min_distance_mm", "thermal min distance must be non-negative"))
		}
	}
	for i, rule := range rules.HighCurrent {
		path := fmt.Sprintf("advanced_rules.high_current[%d]", i)
		issues = append(issues, validateAdvancedRuleID(ids, path, rule.ID)...)
		if len(rule.Nets) == 0 && len(rule.NetRoles) == 0 {
			issues = append(issues, advancedRuleIssue(rule, path+".nets", "high-current rule requires nets or net roles"))
		}
		issues = append(issues, validateKnownNets(rule, path+".nets", rule.Nets, netNames)...)
		issues = append(issues, validateAdvancedNetRoles(rule, path+".net_roles", rule.NetRoles)...)
		issues = append(issues, validateKnownRefs(rule, path+".source_refs", rule.SourceRefs, refs)...)
		issues = append(issues, validateKnownRefs(rule, path+".sink_refs", rule.SinkRefs, refs)...)
		issues = append(issues, validateKnownRefs(rule, path+".keep_away_refs", rule.KeepAwayRefs, refs)...)
		issues = append(issues, validatePadSelectors(rule, path+".source_pads", rule.SourcePads, refs)...)
		issues = append(issues, validatePadSelectors(rule, path+".sink_pads", rule.SinkPads, refs)...)
		if rule.CurrentEstimateA < 0 {
			issues = append(issues, advancedRuleIssue(rule, path+".current_estimate_a", "high-current estimate must be non-negative"))
		}
		if rule.MaxPreferredLengthMM < 0 {
			issues = append(issues, advancedRuleIssue(rule, path+".max_preferred_length_mm", "high-current preferred length must be non-negative"))
		}
	}
	for i, rule := range rules.CreepageClearance {
		path := fmt.Sprintf("advanced_rules.creepage_clearance[%d]", i)
		issues = append(issues, validateAdvancedRuleID(ids, path, rule.ID)...)
		if placementRuleDomainEmpty(rule.DomainA) {
			issues = append(issues, advancedRuleIssue(rule, path+".domain_a", "clearance rule domain A required"))
		}
		if placementRuleDomainEmpty(rule.DomainB) {
			issues = append(issues, advancedRuleIssue(rule, path+".domain_b", "clearance rule domain B required"))
		}
		issues = append(issues, validatePlacementRuleDomain(rule, path+".domain_a", rule.DomainA, refs, netNames, netClasses)...)
		issues = append(issues, validatePlacementRuleDomain(rule, path+".domain_b", rule.DomainB, refs, netNames, netClasses)...)
		if rule.MinClearanceMM < 0 {
			issues = append(issues, advancedRuleIssue(rule, path+".min_clearance_mm", "minimum clearance must be non-negative"))
		}
		if rule.MinCreepageMM < 0 {
			issues = append(issues, advancedRuleIssue(rule, path+".min_creepage_mm", "minimum creepage must be non-negative"))
		}
		if rule.MinClearanceMM == 0 && rule.MinCreepageMM == 0 {
			issues = append(issues, advancedRuleIssue(rule, path+".min_clearance_mm", "clearance rule requires clearance or creepage distance"))
		}
	}
	for i, rule := range rules.DifferentialPair {
		path := fmt.Sprintf("advanced_rules.differential_pair[%d]", i)
		issues = append(issues, validateAdvancedRuleID(ids, path, rule.ID)...)
		if strings.TrimSpace(rule.PositiveNet) == "" {
			issues = append(issues, advancedRuleIssue(rule, path+".positive_net", "differential-pair positive net required"))
		} else if _, ok := netNames[strings.ToUpper(rule.PositiveNet)]; !ok {
			issues = append(issues, advancedRuleIssue(rule, path+".positive_net", "differential-pair positive net references unknown net "+rule.PositiveNet))
		}
		if strings.TrimSpace(rule.NegativeNet) == "" {
			issues = append(issues, advancedRuleIssue(rule, path+".negative_net", "differential-pair negative net required"))
		} else if _, ok := netNames[strings.ToUpper(rule.NegativeNet)]; !ok {
			issues = append(issues, advancedRuleIssue(rule, path+".negative_net", "differential-pair negative net references unknown net "+rule.NegativeNet))
		}
		if strings.EqualFold(rule.PositiveNet, rule.NegativeNet) && strings.TrimSpace(rule.PositiveNet) != "" {
			issues = append(issues, advancedRuleIssue(rule, path+".negative_net", "differential-pair nets must be distinct"))
		}
		issues = append(issues, validateKnownRefs(rule, path+".source_refs", rule.SourceRefs, refs)...)
		issues = append(issues, validateKnownRefs(rule, path+".sink_refs", rule.SinkRefs, refs)...)
		issues = append(issues, validatePadSelectors(rule, path+".source_pads", rule.SourcePads, refs)...)
		issues = append(issues, validatePadSelectors(rule, path+".sink_pads", rule.SinkPads, refs)...)
		if rule.MaxSkewMM < 0 {
			issues = append(issues, advancedRuleIssue(rule, path+".max_skew_mm", "differential-pair skew must be non-negative"))
		}
		if rule.LengthToleranceMM < 0 {
			issues = append(issues, advancedRuleIssue(rule, path+".length_tolerance_mm", "differential-pair length tolerance must be non-negative"))
		}
	}
	for i, rule := range rules.ControlledImpedance {
		path := fmt.Sprintf("advanced_rules.controlled_impedance[%d]", i)
		issues = append(issues, validateAdvancedRuleID(ids, path, rule.ID)...)
		if len(rule.Nets) == 0 && len(rule.NetRoles) == 0 {
			issues = append(issues, advancedRuleIssue(rule, path+".nets", "controlled-impedance rule requires nets or net roles"))
		}
		issues = append(issues, validateKnownNets(rule, path+".nets", rule.Nets, netNames)...)
		issues = append(issues, validateAdvancedNetRoles(rule, path+".net_roles", rule.NetRoles)...)
		issues = append(issues, validateKnownRefs(rule, path+".source_refs", rule.SourceRefs, refs)...)
		issues = append(issues, validateKnownRefs(rule, path+".sink_refs", rule.SinkRefs, refs)...)
		issues = append(issues, validatePadSelectors(rule, path+".source_pads", rule.SourcePads, refs)...)
		issues = append(issues, validatePadSelectors(rule, path+".sink_pads", rule.SinkPads, refs)...)
		if rule.MinCorridorWidthMM < 0 {
			issues = append(issues, advancedRuleIssue(rule, path+".min_corridor_width_mm", "controlled-impedance corridor width must be non-negative"))
		}
		if rule.MaxViaCountPreference < 0 {
			issues = append(issues, advancedRuleIssue(rule, path+".max_via_count_preference", "controlled-impedance via preference must be non-negative"))
		}
	}
	return issues
}

type advancedPlacementRule interface {
	advancedPlacementRuleSeverity() AdvancedRuleSeverity
	advancedPlacementRuleEnforcement() AdvancedRuleEnforcement
}

func (rule ThermalPlacementRule) advancedPlacementRuleSeverity() AdvancedRuleSeverity {
	return rule.Severity
}

func (rule ThermalPlacementRule) advancedPlacementRuleEnforcement() AdvancedRuleEnforcement {
	return rule.Enforcement
}

func (rule HighCurrentPlacementRule) advancedPlacementRuleSeverity() AdvancedRuleSeverity {
	return rule.Severity
}

func (rule HighCurrentPlacementRule) advancedPlacementRuleEnforcement() AdvancedRuleEnforcement {
	return rule.Enforcement
}

func (rule CreepageClearancePlacementRule) advancedPlacementRuleSeverity() AdvancedRuleSeverity {
	return rule.Severity
}

func (rule CreepageClearancePlacementRule) advancedPlacementRuleEnforcement() AdvancedRuleEnforcement {
	return rule.Enforcement
}

func (rule DifferentialPairPlacementRule) advancedPlacementRuleSeverity() AdvancedRuleSeverity {
	return rule.Severity
}

func (rule DifferentialPairPlacementRule) advancedPlacementRuleEnforcement() AdvancedRuleEnforcement {
	return rule.Enforcement
}

func (rule ControlledImpedancePlacementRule) advancedPlacementRuleSeverity() AdvancedRuleSeverity {
	return rule.Severity
}

func (rule ControlledImpedancePlacementRule) advancedPlacementRuleEnforcement() AdvancedRuleEnforcement {
	return rule.Enforcement
}

func validateAdvancedRuleID(ids map[string]int, path string, id string) []reports.Issue {
	id = strings.TrimSpace(id)
	if id == "" {
		return []reports.Issue{issue(path+".id", "advanced placement rule id required")}
	}
	key := strings.ToUpper(id)
	if previous, ok := ids[key]; ok {
		return []reports.Issue{issue(path+".id", fmt.Sprintf("duplicate advanced placement rule ID %s already defined at rule index %d", id, previous))}
	}
	ids[key] = len(ids)
	return nil
}

func advancedRuleIssue(rule advancedPlacementRule, path string, message string) reports.Issue {
	out := issue(path, message)
	if rule.advancedPlacementRuleEnforcement() != AdvancedRuleHard {
		switch rule.advancedPlacementRuleSeverity() {
		case AdvancedRuleSeverityInfo:
			out.Severity = reports.SeverityInfo
		case AdvancedRuleSeverityError:
			out.Severity = reports.SeverityError
		default:
			out.Severity = reports.SeverityWarning
		}
	}
	return out
}

func validateKnownRefs(rule advancedPlacementRule, path string, refsToCheck []string, refs map[string]Component) []reports.Issue {
	var issues []reports.Issue
	for _, ref := range refsToCheck {
		if _, ok := refs[strings.ToUpper(strings.TrimSpace(ref))]; !ok {
			issues = append(issues, advancedRuleIssue(rule, path, "advanced placement rule references unknown component "+strings.TrimSpace(ref)))
		}
	}
	return issues
}

func validateKnownNets(rule advancedPlacementRule, path string, nets []string, netNames map[string]int) []reports.Issue {
	var issues []reports.Issue
	for _, net := range nets {
		if _, ok := netNames[strings.ToUpper(strings.TrimSpace(net))]; !ok {
			issues = append(issues, advancedRuleIssue(rule, path, "advanced placement rule references unknown net "+strings.TrimSpace(net)))
		}
	}
	return issues
}

func validatePadSelectors(rule advancedPlacementRule, path string, selectors []string, refs map[string]Component) []reports.Issue {
	var issues []reports.Issue
	for _, selector := range selectors {
		selector = strings.TrimSpace(selector)
		if selector == "" {
			continue
		}
		ref, pad, ok := splitPadSelector(selector)
		if !ok {
			issues = append(issues, advancedRuleIssue(rule, path, "advanced placement rule pad selector must use Ref.Pad format: "+selector))
			continue
		}
		component, exists := refs[strings.ToUpper(ref)]
		if !exists {
			issues = append(issues, advancedRuleIssue(rule, path, "advanced placement rule pad selector references unknown component "+ref))
			continue
		}
		if len(component.Pads) > 0 && !componentHasPad(component, pad) {
			issues = append(issues, advancedRuleIssue(rule, path, "advanced placement rule pad selector references unknown pad "+selector))
		}
	}
	return issues
}

func splitPadSelector(selector string) (ref string, pad string, ok bool) {
	index := strings.LastIndex(selector, ".")
	if index <= 0 || index == len(selector)-1 {
		return "", "", false
	}
	ref = strings.TrimSpace(selector[:index])
	pad = strings.TrimSpace(selector[index+1:])
	return ref, pad, ref != "" && pad != ""
}

func validateAdvancedNetRoles(rule advancedPlacementRule, path string, roles []NetRole) []reports.Issue {
	var issues []reports.Issue
	for _, role := range roles {
		if !validNetRole(role) {
			issues = append(issues, advancedRuleIssue(rule, path, "invalid net role "+string(role)))
		}
	}
	return issues
}

func placementRegionNames(regionRules []RegionRule) map[string]struct{} {
	regions := make(map[string]struct{}, len(regionRules))
	for _, rule := range regionRules {
		region := strings.TrimSpace(rule.Region)
		if region == "" {
			continue
		}
		regions[strings.ToUpper(region)] = struct{}{}
	}
	return regions
}

func validatePlacementRuleDomain(rule advancedPlacementRule, path string, domain PlacementRuleDomain, refs map[string]Component, netNames map[string]int, netClasses map[string]int) []reports.Issue {
	var issues []reports.Issue
	issues = append(issues, validateKnownRefs(rule, path+".refs", domain.Refs, refs)...)
	issues = append(issues, validateKnownNets(rule, path+".nets", domain.Nets, netNames)...)
	issues = append(issues, validateKnownNetClasses(rule, path+".net_classes", domain.NetClasses, netClasses)...)
	issues = append(issues, validateAdvancedNetRoles(rule, path+".net_roles", domain.NetRoles)...)
	return issues
}

func validateKnownNetClasses(rule advancedPlacementRule, path string, classes []string, netClasses map[string]int) []reports.Issue {
	if len(classes) == 0 {
		return nil
	}
	var issues []reports.Issue
	for _, class := range classes {
		if _, ok := netClasses[strings.ToUpper(strings.TrimSpace(class))]; !ok {
			issues = append(issues, advancedRuleIssue(rule, path, "advanced placement rule references unknown net class "+strings.TrimSpace(class)))
		}
	}
	return issues
}

func placementRuleDomainEmpty(domain PlacementRuleDomain) bool {
	return len(domain.Refs) == 0 && len(domain.Nets) == 0 && len(domain.NetClasses) == 0 && len(domain.Roles) == 0 && len(domain.NetRoles) == 0
}

func validateProximityRules(rules []ProximityRule, refs map[string]Component) []reports.Issue {
	var issues []reports.Issue
	ids := map[string]int{}
	for i, rule := range rules {
		path := fmt.Sprintf("proximity_rules[%d]", i)
		id := strings.TrimSpace(rule.ID)
		if id == "" {
			issues = append(issues, issue(path+".id", "proximity rule id required"))
		} else {
			key := strings.ToUpper(id)
			if previous, ok := ids[key]; ok {
				issues = append(issues, issue(path+".id", fmt.Sprintf("duplicate proximity rule ID %s already defined at index %d", id, previous)))
			}
			ids[key] = i
		}
		if !validIntentRole(rule.Role) {
			issues = append(issues, intentRuleIssue(rule.Required, path+".role", "invalid intent role "+string(rule.Role)))
		}
		anchorRef := strings.TrimSpace(rule.AnchorRef)
		anchor, hasAnchor := refs[strings.ToUpper(anchorRef)]
		if anchorRef == "" {
			issues = append(issues, intentRuleIssue(rule.Required, path+".anchor_ref", "proximity rule anchor ref required"))
		} else if !hasAnchor {
			issues = append(issues, intentRuleIssue(rule.Required, path+".anchor_ref", "proximity rule anchor references unknown component "+anchorRef))
		} else {
			for _, pin := range rule.AnchorPins {
				if len(anchor.Pads) > 0 && !componentHasPad(anchor, pin) {
					issues = append(issues, intentRuleIssue(rule.Required, path+".anchor_pins", "proximity rule anchor pin "+pin+" not found in component "+anchorRef))
				}
			}
		}
		if len(rule.TargetRefs) == 0 {
			issues = append(issues, intentRuleIssue(rule.Required, path+".target_refs", "proximity rule target refs required"))
		}
		for _, target := range rule.TargetRefs {
			targetRef := strings.TrimSpace(target)
			targetComponent, hasTarget := refs[strings.ToUpper(targetRef)]
			if targetRef == "" {
				issues = append(issues, intentRuleIssue(rule.Required, path+".target_refs", "proximity rule target ref required"))
				continue
			}
			if !hasTarget {
				issues = append(issues, intentRuleIssue(rule.Required, path+".target_refs", "proximity rule target references unknown component "+targetRef))
				continue
			}
			for _, pin := range rule.TargetPins {
				if len(targetComponent.Pads) > 0 && !componentHasPad(targetComponent, pin) {
					issues = append(issues, intentRuleIssue(rule.Required, path+".target_pins", "proximity rule target pin "+pin+" not found in component "+targetRef))
				}
			}
		}
		if rule.MaxDistanceMM < 0 {
			issues = append(issues, intentRuleIssue(rule.Required, path+".max_distance_mm", "proximity rule max distance must be non-negative"))
		}
	}
	return issues
}

func validateRegionRules(rules []RegionRule, refs map[string]Component) []reports.Issue {
	var issues []reports.Issue
	ids := map[string]int{}
	for i, rule := range rules {
		path := fmt.Sprintf("region_rules[%d]", i)
		id := strings.TrimSpace(rule.ID)
		if id == "" {
			issues = append(issues, issue(path+".id", "region rule id required"))
		} else {
			key := strings.ToUpper(id)
			if previous, ok := ids[key]; ok {
				issues = append(issues, issue(path+".id", fmt.Sprintf("duplicate region rule ID %s already defined at index %d", id, previous)))
			}
			ids[key] = i
		}
		if strings.TrimSpace(rule.Region) == "" {
			issues = append(issues, intentRuleIssue(rule.Required, path+".region", "region rule region required"))
		}
		for _, ref := range rule.Refs {
			if strings.TrimSpace(ref) == "" {
				issues = append(issues, intentRuleIssue(rule.Required, path+".refs", "region rule ref required"))
				continue
			}
			if _, ok := refs[strings.ToUpper(strings.TrimSpace(ref))]; !ok {
				issues = append(issues, intentRuleIssue(rule.Required, path+".refs", "region rule references unknown component "+strings.TrimSpace(ref)))
			}
		}
		for _, role := range rule.NetRoles {
			if !validNetRole(role) {
				issues = append(issues, intentRuleIssue(rule.Required, path+".net_roles", "invalid net role "+string(role)))
			}
		}
		if rule.Preferred.Min.XMM > rule.Preferred.Max.XMM || rule.Preferred.Min.YMM > rule.Preferred.Max.YMM {
			issues = append(issues, intentRuleIssue(rule.Required, path+".preferred", "region preferred bounds min must not exceed max"))
		}
	}
	return issues
}

func intentRuleIssue(required bool, path string, message string) reports.Issue {
	out := issue(path, message)
	if !required {
		out.Severity = reports.SeverityWarning
	}
	return out
}

func stableRuleID(id string, prefix string, index int) string {
	id = strings.TrimSpace(id)
	if id != "" {
		return id
	}
	return fmt.Sprintf("%s-%03d", prefix, index+1)
}

func uniqueSortedStrings(values []string) []string {
	seen := map[string]string{}
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		key := strings.ToUpper(trimmed)
		if _, ok := seen[key]; !ok {
			seen[key] = trimmed
		}
	}
	out := make([]string, 0, len(seen))
	for _, value := range seen {
		out = append(out, value)
	}
	slices.Sort(out)
	return out
}

func uniqueSortedRefs(values []string) []string {
	seen := map[string]struct{}{}
	for _, value := range values {
		ref := normalizeRef(value)
		if ref == "" {
			continue
		}
		seen[ref] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for value := range seen {
		out = append(out, value)
	}
	slices.Sort(out)
	return out
}

func validIntentRole(role IntentRole) bool {
	switch role {
	case "", IntentDecoupling, IntentClock, IntentFeedback, IntentPowerPath, IntentPullup, IntentConnector, IntentThermal, IntentReset, IntentProgramming:
		return true
	default:
		return false
	}
}

func validThermalRole(role ThermalRole) bool {
	switch role {
	case "", ThermalRoleHeatSource, ThermalRoleThermalSensitive, ThermalRoleHeatSink, ThermalRoleRegulator, ThermalRolePowerSwitch, ThermalRoleConnector:
		return true
	default:
		return false
	}
}

func validSide(side SideConstraint) bool {
	switch side {
	case "", SideAny, SideTop, SideBottom:
		return true
	default:
		return false
	}
}

func validEdge(edge EdgeConstraint) bool {
	switch edge {
	case EdgeNone, EdgeAny, EdgeLeft, EdgeRight, EdgeTop, EdgeBottom:
		return true
	default:
		return false
	}
}

func validNetRole(role NetRole) bool {
	switch role {
	case "", NetPower, NetGround, NetSignal, NetClock, NetAnalog, NetDifferential, NetUnknown:
		return true
	default:
		return false
	}
}

func validBoundsSource(source BoundsSource) bool {
	switch source {
	case "", BoundsLibraryCourtyard, BoundsLibraryPads, BoundsGeneratedPads, BoundsExplicit, BoundsEstimated:
		return true
	default:
		return false
	}
}

func componentHasPad(component Component, pin string) bool {
	for _, pad := range component.Pads {
		if strings.EqualFold(strings.TrimSpace(pad.Name), pin) {
			return true
		}
	}
	return false
}

func validateRotation(rotation RotationConstraint) error {
	if rotation.FixedDeg != nil {
		if !validRotation(*rotation.FixedDeg) {
			return fmt.Errorf("fixed rotation must be one of 0, 90, 180, 270")
		}
	}
	for _, value := range rotation.AllowedDeg {
		if !validRotation(value) {
			return fmt.Errorf("allowed rotation must be one of 0, 90, 180, 270")
		}
	}
	return nil
}

func validRotation(value float64) bool {
	normalized := math.Mod(value, 360)
	if normalized < 0 {
		normalized += 360
	}
	const epsilon = 1e-9
	return math.Abs(normalized-0) < epsilon ||
		math.Abs(normalized-90) < epsilon ||
		math.Abs(normalized-180) < epsilon ||
		math.Abs(normalized-270) < epsilon ||
		math.Abs(normalized-360) < epsilon
}

func issue(path string, message string) reports.Issue {
	return reports.Issue{
		Code:     reports.CodeValidationFailed,
		Severity: reports.SeverityError,
		Path:     path,
		Message:  message,
	}
}
