package profiles

import (
	"fmt"
	"slices"
	"strings"
	"sync"

	"kicadai/internal/reports"
)

const DefaultProfileID = "generic_assembly"

var builtinProfiles = []Profile{
	{
		Schema:  SchemaV1,
		ID:      DefaultProfileID,
		Name:    "Generic Assembly",
		Version: "2026-06",
		Source:  Source{Kind: SourceBuiltin},
		Units:   "mm",
		Stackup: Stackup{
			MinLayers:                    2,
			MaxLayers:                    4,
			AllowedLayerCounts:           []int{2, 4},
			MinBoardThicknessMM:          0.8,
			MaxBoardThicknessMM:          2.0,
			DefaultBoardThicknessMM:      1.6,
			RequiresTwoOuterCopperLayers: true,
		},
		Copper: Copper{
			MinTraceWidthMM:       0.15,
			MinSpacingMM:          0.15,
			MinCopperToEdgeMM:     0.25,
			MinCopperSliverMM:     0.127,
			MinZoneMinThicknessMM: 0.127,
		},
		Drill: Drill{
			MinDrillMM:           0.30,
			MinViaDrillMM:        0.30,
			MinPadAnnularRingMM:  0.15,
			MinViaAnnularRingMM:  0.10,
			MinHoleToEdgeMM:      0.50,
			MinFinishedHoleMM:    0.30,
			MinPlatedSlotWidthMM: 0.60,
		},
		SolderMask: SolderMask{
			MinSolderMaskWebMM:     0.10,
			DefaultMaskExpansionMM: 0.05,
			MinMaskClearanceMM:     0.05,
			PasteRequiredForSMD:    true,
		},
		Silkscreen: Silkscreen{
			MinLineWidthMM:        0.12,
			MinTextHeightMM:       0.80,
			MinSilkscreenToMaskMM: 0.15,
		},
		Assembly: Assembly{
			RequireCourtyards:           true,
			RequireReferenceDesignators: true,
		},
		EdgePlating: EdgePolicy{
			RequiresManualReview: true,
		},
		Impedance: Impedance{
			RequireStackupForImpedance:      true,
			RequireDiffPairWidthGapEvidence: true,
			RequireDiffPairSkewEvidence:     true,
		},
		Metadata: Metadata{
			RequireBoardFinish:      true,
			AllowedBoardFinishes:    []string{"ENIG", "HASL", "Immersion Silver"},
			RequireFabricationNotes: true,
		},
	},
	{
		Schema:  SchemaV1,
		ID:      "generic_2layer_economy",
		Name:    "Generic 2-Layer Economy",
		Version: "2026-06",
		Source:  Source{Kind: SourceBuiltin},
		Units:   "mm",
		Stackup: Stackup{
			MinLayers:                          2,
			MaxLayers:                          2,
			AllowedLayerCounts:                 []int{2},
			MinBoardThicknessMM:                1.0,
			MaxBoardThicknessMM:                1.6,
			DefaultBoardThicknessMM:            1.6,
			RequiresTwoOuterCopperLayers:       true,
			RequiresInternalPlanesForImpedance: false,
		},
		Copper: Copper{
			MinTraceWidthMM:       0.20,
			MinSpacingMM:          0.20,
			MinCopperToEdgeMM:     0.30,
			MinCopperSliverMM:     0.15,
			MinZoneMinThicknessMM: 0.15,
		},
		Drill: Drill{
			MinDrillMM:          0.35,
			MinViaDrillMM:       0.35,
			MinPadAnnularRingMM: 0.18,
			MinViaAnnularRingMM: 0.12,
			MinHoleToEdgeMM:     0.60,
		},
		SolderMask: SolderMask{MinSolderMaskWebMM: 0.12, DefaultMaskExpansionMM: 0.05, MinMaskClearanceMM: 0.05, PasteRequiredForSMD: true},
		Silkscreen: Silkscreen{MinLineWidthMM: 0.15, MinTextHeightMM: 1.0, MinSilkscreenToMaskMM: 0.15},
		Assembly:   Assembly{RequireCourtyards: true, RequireReferenceDesignators: true},
		EdgePlating: EdgePolicy{
			RequiresManualReview: true,
		},
		Metadata: Metadata{RequireBoardFinish: true, AllowedBoardFinishes: []string{"HASL", "ENIG"}},
	},
	{
		Schema:  SchemaV1,
		ID:      "generic_2layer_standard",
		Name:    "Generic 2-Layer Standard",
		Version: "2026-06",
		Source:  Source{Kind: SourceBuiltin},
		Units:   "mm",
		Stackup: Stackup{
			MinLayers:                    2,
			MaxLayers:                    2,
			AllowedLayerCounts:           []int{2},
			MinBoardThicknessMM:          0.8,
			MaxBoardThicknessMM:          2.0,
			DefaultBoardThicknessMM:      1.6,
			RequiresTwoOuterCopperLayers: true,
		},
		Copper:     Copper{MinTraceWidthMM: 0.15, MinSpacingMM: 0.15, MinCopperToEdgeMM: 0.25, MinCopperSliverMM: 0.127, MinZoneMinThicknessMM: 0.127},
		Drill:      Drill{MinDrillMM: 0.30, MinViaDrillMM: 0.30, MinPadAnnularRingMM: 0.15, MinViaAnnularRingMM: 0.10, MinHoleToEdgeMM: 0.50},
		SolderMask: SolderMask{MinSolderMaskWebMM: 0.10, DefaultMaskExpansionMM: 0.05, MinMaskClearanceMM: 0.05, PasteRequiredForSMD: true},
		Silkscreen: Silkscreen{MinLineWidthMM: 0.12, MinTextHeightMM: 0.80, MinSilkscreenToMaskMM: 0.15},
		Assembly:   Assembly{RequireCourtyards: true, RequireReferenceDesignators: true},
		EdgePlating: EdgePolicy{
			RequiresManualReview: true,
		},
		Metadata: Metadata{RequireBoardFinish: true, AllowedBoardFinishes: []string{"ENIG", "HASL", "Immersion Silver"}},
	},
	{
		Schema:  SchemaV1,
		ID:      "generic_4layer_standard",
		Name:    "Generic 4-Layer Standard",
		Version: "2026-06",
		Source:  Source{Kind: SourceBuiltin},
		Units:   "mm",
		Stackup: Stackup{
			MinLayers:                          4,
			MaxLayers:                          4,
			AllowedLayerCounts:                 []int{4},
			MinBoardThicknessMM:                1.0,
			MaxBoardThicknessMM:                1.6,
			DefaultBoardThicknessMM:            1.6,
			RequiresTwoOuterCopperLayers:       true,
			RequiresInternalPlanesForImpedance: true,
		},
		Copper:     Copper{MinTraceWidthMM: 0.125, MinSpacingMM: 0.125, MinCopperToEdgeMM: 0.25, MinCopperSliverMM: 0.10, MinZoneMinThicknessMM: 0.10},
		Drill:      Drill{MinDrillMM: 0.25, MinViaDrillMM: 0.25, MinPadAnnularRingMM: 0.13, MinViaAnnularRingMM: 0.10, MinHoleToEdgeMM: 0.50},
		SolderMask: SolderMask{MinSolderMaskWebMM: 0.10, DefaultMaskExpansionMM: 0.05, MinMaskClearanceMM: 0.05, PasteRequiredForSMD: true},
		Silkscreen: Silkscreen{MinLineWidthMM: 0.12, MinTextHeightMM: 0.80, MinSilkscreenToMaskMM: 0.15},
		Assembly:   Assembly{RequireCourtyards: true, RequireReferenceDesignators: true},
		Impedance:  Impedance{RequireStackupForImpedance: true, RequireDiffPairWidthGapEvidence: true, RequireDiffPairSkewEvidence: true},
		Metadata:   Metadata{RequireBoardFinish: true, AllowedBoardFinishes: []string{"ENIG"}},
	},
	{
		Schema:  SchemaV1,
		ID:      "generic_castellated_review",
		Name:    "Generic Castellated Review",
		Version: "2026-06",
		Source:  Source{Kind: SourceBuiltin},
		Units:   "mm",
		Stackup: Stackup{
			MinLayers:                    2,
			MaxLayers:                    4,
			AllowedLayerCounts:           []int{2, 4},
			MinBoardThicknessMM:          1.0,
			MaxBoardThicknessMM:          1.6,
			DefaultBoardThicknessMM:      1.6,
			RequiresTwoOuterCopperLayers: true,
		},
		Copper:     Copper{MinTraceWidthMM: 0.15, MinSpacingMM: 0.15, MinCopperToEdgeMM: 0.30, MinCopperSliverMM: 0.127, MinZoneMinThicknessMM: 0.127},
		Drill:      Drill{MinDrillMM: 0.30, MinViaDrillMM: 0.30, MinPadAnnularRingMM: 0.15, MinViaAnnularRingMM: 0.10, MinHoleToEdgeMM: 0.60},
		SolderMask: SolderMask{MinSolderMaskWebMM: 0.10, DefaultMaskExpansionMM: 0.05, MinMaskClearanceMM: 0.05, PasteRequiredForSMD: true},
		Silkscreen: Silkscreen{MinLineWidthMM: 0.12, MinTextHeightMM: 0.80, MinSilkscreenToMaskMM: 0.15},
		Assembly:   Assembly{RequireCourtyards: true, RequireReferenceDesignators: true},
		EdgePlating: EdgePolicy{
			AllowCastellations:       true,
			AllowEdgePlating:         true,
			MinCastellationDrillMM:   0.60,
			MinCastellationPitchMM:   1.20,
			RequiresEdgePlatingNotes: true,
			RequiresManualReview:     true,
		},
		Metadata: Metadata{RequireBoardFinish: true, AllowedBoardFinishes: []string{"ENIG"}, RequireFabricationNotes: true},
	},
}

var (
	builtinRegistryOnce sync.Once
	builtinRegistryData Registry
)

type Registry struct {
	profiles map[string]Profile
	order    []string
}

func Builtins() Registry {
	return builtinRegistry()
}

func builtinRegistry() Registry {
	builtinRegistryOnce.Do(func() {
		builtinRegistryData = buildBuiltinRegistry()
	})
	return builtinRegistryData
}

func buildBuiltinRegistry() Registry {
	registry := Registry{
		profiles: make(map[string]Profile, len(builtinProfiles)),
		order:    make([]string, 0, len(builtinProfiles)),
	}
	for _, profile := range builtinProfiles {
		if _, exists := registry.profiles[profile.ID]; exists {
			panic(fmt.Sprintf("duplicate builtin fabrication profile ID %q", profile.ID))
		}
		registry.profiles[profile.ID] = cloneProfile(profile)
		registry.order = append(registry.order, profile.ID)
	}
	slices.Sort(registry.order)
	return registry
}

func List() []Summary {
	return builtinRegistry().List()
}

func Resolve(id string) (Profile, []reports.Issue) {
	return builtinRegistry().Resolve(id)
}

func (registry Registry) List() []Summary {
	summaries := make([]Summary, 0, len(registry.order))
	for _, id := range registry.order {
		summaries = append(summaries, Summarize(registry.profiles[id]))
	}
	return summaries
}

func (registry Registry) Resolve(id string) (Profile, []reports.Issue) {
	normalizedID := strings.TrimSpace(id)
	if normalizedID == "" {
		normalizedID = DefaultProfileID
	}
	profile, ok := registry.profiles[normalizedID]
	if !ok {
		return Profile{}, []reports.Issue{{
			Code:       reports.CodeValidationFailed,
			Severity:   reports.SeverityError,
			Path:       "fabrication_profile.id",
			Message:    fmt.Sprintf("unknown fabrication profile %q", normalizedID),
			Suggestion: "run `kicadai fabrication profile list` to see available profiles",
		}}
	}
	return cloneProfile(profile), Validate(profile)
}

func cloneProfile(profile Profile) Profile {
	cloned := profile
	cloned.Stackup.AllowedLayerCounts = slices.Clone(profile.Stackup.AllowedLayerCounts)
	cloned.Metadata.AllowedBoardFinishes = slices.Clone(profile.Metadata.AllowedBoardFinishes)
	cloned.Metadata.WarningOnlyFields = slices.Clone(profile.Metadata.WarningOnlyFields)
	return cloned
}
