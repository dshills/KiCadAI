package opentopologysynthesis

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strings"

	"kicadai/internal/components"
	"kicadai/internal/modelprovenance"
	"kicadai/internal/reports"
	"kicadai/internal/simmodel"
)

const (
	PrimitiveInventorySchema  = "kicadai.open-topology-primitive-inventory.v1"
	PrimitiveInventoryVersion = 1

	inventorySampleLimit = 16
)

const (
	inventoryRejectionNoPrimitiveModel = "no_primitive_model"
	inventoryRejectionFunctionalModel  = "functional_compact_model"
	inventoryRejectionUnreviewedModel  = "unreviewed_model"
	inventoryRejectionPhysicalEvidence = "physical_evidence_incomplete"
	inventoryRejectionTerminalContract = "terminal_contract_incomplete"
	inventoryRejectionValueDomain      = "value_domain_incomplete"
	inventoryRejectionLifecycle        = "lifecycle_unavailable"
)

type PrimitiveInventory struct {
	Schema            string               `json:"schema"`
	Version           int                  `json:"version"`
	CatalogHash       string               `json:"catalog_hash"`
	ModelRegistryHash string               `json:"model_registry_hash"`
	PrimitiveRegistry string               `json:"primitive_registry_hash"`
	Hash              string               `json:"hash"`
	Primitives        []PrimitiveCandidate `json:"primitives"`
	Rejections        []InventoryRejection `json:"rejections"`
	Stats             InventoryStats       `json:"stats"`
}

type InventoryStats struct {
	CatalogRecords       int `json:"catalog_records"`
	PhysicalVariants     int `json:"physical_variants"`
	PrimitiveCandidates  int `json:"primitive_candidates"`
	PrimitiveModelClaims int `json:"primitive_model_claims"`
}

type InventoryRejection struct {
	Code    string   `json:"code"`
	Count   int      `json:"count"`
	Samples []string `json:"samples"`
}

type PrimitiveCandidate struct {
	Key         string                   `json:"key"`
	CatalogID   string                   `json:"catalog_id"`
	VariantID   string                   `json:"variant_id"`
	UnitID      string                   `json:"unit_id,omitempty"`
	Kind        string                   `json:"kind"`
	Family      string                   `json:"family"`
	Generic     bool                     `json:"generic"`
	Evidence    string                   `json:"evidence"`
	SymbolIDs   []string                 `json:"symbol_ids"`
	FootprintID string                   `json:"footprint_id"`
	PackageType string                   `json:"package_type"`
	AreaMM2     float64                  `json:"area_mm2,omitempty"`
	Terminals   []PrimitiveTerminal      `json:"terminals"`
	Models      []PrimitiveModelContract `json:"models"`
	ValueDomain *PrimitiveValueDomain    `json:"value_domain,omitempty"`
	Ratings     []PrimitiveBound         `json:"ratings,omitempty"`
	Tolerances  []PrimitiveBound         `json:"tolerances,omitempty"`
}

type PrimitiveTerminal struct {
	Terminal   string   `json:"terminal"`
	Function   string   `json:"function"`
	Aliases    []string `json:"aliases,omitempty"`
	SymbolID   string   `json:"symbol_id"`
	UnitID     string   `json:"unit_id,omitempty"`
	SymbolPin  string   `json:"symbol_pin"`
	Pad        string   `json:"pad"`
	Electrical string   `json:"electrical,omitempty"`
	Polarity   string   `json:"polarity,omitempty"`
	Required   bool     `json:"required"`
	DefaultNet string   `json:"default_net,omitempty"`
}

type PrimitiveModelContract struct {
	ModelID              string                          `json:"model_id"`
	Family               string                          `json:"family"`
	RequiresValueSI      bool                            `json:"requires_value_si,omitempty"`
	Nonlinear            bool                            `json:"nonlinear,omitempty"`
	Transient            bool                            `json:"transient,omitempty"`
	ThermalRC            bool                            `json:"thermal_rc,omitempty"`
	SupportsTransientSOA bool                            `json:"supports_transient_soa,omitempty"`
	Parameters           []simmodel.NamedValue           `json:"parameters,omitempty"`
	Uncertainties        []simmodel.Uncertainty          `json:"uncertainties,omitempty"`
	ThermalModel         *simmodel.ThermalRCNetwork      `json:"thermal_model,omitempty"`
	TransientSOA         []simmodel.TransientSOAEnvelope `json:"transient_soa,omitempty"`
	AllowedAnalyses      []string                        `json:"allowed_analyses"`
	ProvenanceSource     string                          `json:"provenance_source"`
	ProvenanceRevision   string                          `json:"provenance_revision"`
	ProvenanceSHA256     string                          `json:"provenance_sha256"`
	MinTemperatureC      *float64                        `json:"min_temperature_c,omitempty"`
	MaxTemperatureC      *float64                        `json:"max_temperature_c,omitempty"`
}

type PrimitiveValueDomain struct {
	Kind    string   `json:"kind"`
	Unit    string   `json:"unit"`
	Minimum *float64 `json:"minimum,omitempty"`
	Nominal *float64 `json:"nominal,omitempty"`
	Maximum *float64 `json:"maximum,omitempty"`
}

type PrimitiveBound struct {
	Kind    string   `json:"kind"`
	Unit    string   `json:"unit"`
	Minimum *float64 `json:"minimum,omitempty"`
	Nominal *float64 `json:"nominal,omitempty"`
	Maximum *float64 `json:"maximum,omitempty"`
}

type inventoryRejections map[string][]string

func BuildPrimitiveInventory(catalog *components.Catalog, catalogHash string, registry modelprovenance.Registry) (PrimitiveInventory, []reports.Issue) {
	result := PrimitiveInventory{
		Schema:      PrimitiveInventorySchema,
		Version:     PrimitiveInventoryVersion,
		CatalogHash: strings.ToLower(strings.TrimSpace(catalogHash)),
	}
	if catalog == nil {
		return result, []reports.Issue{{
			Code:     CodePrimitiveUnavailable,
			Severity: reports.SeverityError,
			Stage:    "primitive_inventory",
			Path:     "catalog",
			Message:  "primitive inventory requires a loaded component catalog",
		}}
	}
	registry = modelprovenance.Normalize(registry)
	if diagnostics := modelprovenance.Validate(registry); len(diagnostics) != 0 {
		issues := make([]reports.Issue, 0, len(diagnostics))
		for _, diagnostic := range diagnostics {
			issues = append(issues, reports.Issue{
				Code:       CodeModelUnavailable,
				Severity:   reports.SeverityError,
				Stage:      "primitive_inventory",
				Path:       diagnostic.Path,
				Message:    diagnostic.Message,
				Suggestion: "repair the reviewed model-provenance registry",
			})
		}
		return result, reports.SortedIssues(issues)
	}
	modelHash, err := modelprovenance.Hash(registry)
	if err != nil {
		return result, []reports.Issue{{
			Code:     CodeModelUnavailable,
			Severity: reports.SeverityError,
			Stage:    "primitive_inventory",
			Path:     "model_registry",
			Message:  "hash model-provenance registry: " + err.Error(),
		}}
	}
	result.ModelRegistryHash = modelHash
	result.PrimitiveRegistry = primitiveRegistryHash()
	result.Stats.CatalogRecords = len(catalog.Records)

	descriptors := allowedPrimitiveDescriptors()
	rejections := inventoryRejections{}
	records := append([]components.ComponentRecord(nil), catalog.Records...)
	slices.SortFunc(records, func(left, right components.ComponentRecord) int {
		return cmp.Compare(left.ID, right.ID)
	})
	for _, record := range records {
		if lifecycleUnavailable(record.Lifecycle) {
			rejections.add(inventoryRejectionLifecycle, record.ID)
			continue
		}
		models, modelRejections := inventoryModels(record, registry, descriptors)
		for code := range modelRejections {
			rejections.add(code, record.ID)
		}
		if len(models) == 0 {
			if len(record.SimulationModels) == 0 {
				rejections.add(inventoryRejectionNoPrimitiveModel, record.ID)
			}
			continue
		}
		descriptor := descriptors[models[0].ModelID]
		valueDomain, valueOK := primitiveValueDomain(record, descriptor.RequiresValueSI)
		if !valueOK {
			rejections.add(inventoryRejectionValueDomain, record.ID)
			continue
		}
		variants := append([]components.PackageVariant(nil), record.Packages...)
		slices.SortFunc(variants, func(left, right components.PackageVariant) int {
			return cmp.Compare(left.ID, right.ID)
		})
		for _, variant := range variants {
			result.Stats.PhysicalVariants++
			if !physicalEvidenceAccepted(record, variant) {
				rejections.add(inventoryRejectionPhysicalEvidence, record.ID+"|"+variant.ID)
				continue
			}
			units := primitiveFunctionalUnits(record, descriptor)
			for _, unit := range units {
				terminals, symbolIDs, ok := bindPrimitiveTerminals(record, variant, unit, descriptor)
				if !ok {
					rejections.add(inventoryRejectionTerminalContract, primitiveCandidateKey(record.ID, variant.ID, unit))
					continue
				}
				requiredFunctions := make([]string, 0, len(terminals))
				for _, terminal := range terminals {
					requiredFunctions = append(requiredFunctions, terminal.Function)
				}
				slices.Sort(requiredFunctions)
				acceptanceResult := components.ValidateResolvedComponent(
					components.ResolvedComponent{
						Component: record,
						Variant:   variant,
					},
					components.SelectionRequest{
						Acceptance:        components.AcceptanceFabricationCandidate,
						RequiredFunctions: requiredFunctions,
						RequireConcrete:   true,
					},
				)
				if !acceptanceResult.OK {
					rejections.add(
						inventoryRejectionPhysicalEvidence,
						primitiveCandidateKey(record.ID, variant.ID, unit),
					)
					continue
				}
				result.Primitives = append(result.Primitives, PrimitiveCandidate{
					Key:         primitiveCandidateKey(record.ID, variant.ID, unit),
					CatalogID:   record.ID,
					VariantID:   variant.ID,
					UnitID:      unit,
					Kind:        openTopologyPrimitiveKind(models[0].ModelID),
					Family:      record.Family,
					Generic:     record.Generic,
					Evidence:    string(record.Verification.Confidence),
					SymbolIDs:   symbolIDs,
					FootprintID: variant.FootprintID,
					PackageType: variant.PackageType,
					AreaMM2:     packageArea(variant),
					Terminals:   terminals,
					Models:      models,
					ValueDomain: valueDomain,
					Ratings:     primitiveBounds(record.Ratings),
					Tolerances:  primitiveToleranceBounds(record.Tolerances),
				})
				result.Stats.PrimitiveModelClaims += len(models)
			}
		}
	}
	slices.SortFunc(result.Primitives, comparePrimitiveCandidates)
	result.Rejections = rejections.normalized()
	result.Stats.PrimitiveCandidates = len(result.Primitives)
	hash, err := primitiveInventoryHash(result)
	if err != nil {
		return result, []reports.Issue{{
			Code:     CodePrimitiveUnavailable,
			Severity: reports.SeverityError,
			Stage:    "primitive_inventory",
			Path:     "inventory",
			Message:  "hash primitive inventory: " + err.Error(),
		}}
	}
	result.Hash = hash
	if len(result.Primitives) == 0 {
		return result, []reports.Issue{{
			Code:       CodePrimitiveUnavailable,
			Severity:   reports.SeverityError,
			Stage:      "primitive_inventory",
			Path:       "inventory.primitives",
			Message:    "no reviewed primitive candidates satisfy the open-topology boundary",
			Suggestion: "onboard reviewed primitive model and physical evidence",
		}}
	}
	return result, nil
}

func allowedPrimitiveDescriptors() map[string]simmodel.PrimitiveDescriptor {
	allowed := map[string]bool{
		simmodel.PrimitiveResistorV1:                    true,
		simmodel.PrimitiveCapacitorV1:                   true,
		simmodel.PrimitiveCapacitorTransientV1:          true,
		simmodel.PrimitiveInductorTransientV1:           true,
		simmodel.PrimitiveOpAmpV1:                       true,
		simmodel.PrimitiveComparatorOpenCollectorV1:     true,
		simmodel.PrimitiveAdjustableLinearRegulatorV1:   true,
		simmodel.PrimitiveFixedLinearRegulatorV1:        true,
		simmodel.PrimitiveFloatingAdjustableRegulatorV1: true,
		simmodel.PrimitiveShuntVoltageReferenceV1:       true,
		simmodel.PrimitiveBidirectionalTVSV1:            true,
		simmodel.PrimitiveUnidirectionalZenerV1:         true,
		simmodel.PrimitiveDiodeShockleyV1:               true,
		simmodel.PrimitiveNMOSSwitchV1:                  true,
		simmodel.PrimitivePMOSSwitchV1:                  true,
		simmodel.PrimitiveBJTNPNV1:                      true,
		simmodel.PrimitiveBJTPNPV1:                      true,
	}
	result := map[string]simmodel.PrimitiveDescriptor{}
	for _, descriptor := range simmodel.PrimitiveDescriptors() {
		if allowed[descriptor.ID] {
			result[descriptor.ID] = descriptor
		}
	}
	return result
}

func inventoryModels(record components.ComponentRecord, registry modelprovenance.Registry, descriptors map[string]simmodel.PrimitiveDescriptor) ([]PrimitiveModelContract, map[string]bool) {
	result := []PrimitiveModelContract{}
	rejections := map[string]bool{}
	for _, claim := range record.SimulationModels {
		descriptor, allowed := descriptors[claim.ModelID]
		if !allowed {
			rejections[inventoryRejectionFunctionalModel] = true
			continue
		}
		provenanceRecord, found := modelprovenance.Lookup(registry, record.ID, claim.ModelID)
		if !found || provenanceRecord.Provenance.ReviewStatus != "reviewed" ||
			strings.TrimSpace(provenanceRecord.Provenance.Source) == "" ||
			strings.TrimSpace(provenanceRecord.Provenance.Revision) == "" ||
			len(provenanceRecord.Provenance.SHA256) != sha256.Size*2 ||
			len(provenanceRecord.Provenance.AllowedAnalyses) == 0 {
			rejections[inventoryRejectionUnreviewedModel] = true
			continue
		}
		claim = simmodel.CloneCatalogEvidence(claim)
		parameters := append([]simmodel.NamedValue(nil), claim.Parameters...)
		slices.SortFunc(parameters, func(left, right simmodel.NamedValue) int {
			return cmp.Or(cmp.Compare(left.Name, right.Name), cmp.Compare(left.Value, right.Value))
		})
		analyses := append([]string(nil), provenanceRecord.Provenance.AllowedAnalyses...)
		slices.Sort(analyses)
		analyses = slices.Compact(analyses)
		result = append(result, PrimitiveModelContract{
			ModelID:              claim.ModelID,
			Family:               descriptor.Family,
			RequiresValueSI:      descriptor.RequiresValueSI,
			Nonlinear:            descriptor.Nonlinear,
			Transient:            descriptor.Transient,
			ThermalRC:            descriptor.ThermalRC,
			SupportsTransientSOA: descriptor.TransientSOA,
			Parameters:           parameters,
			Uncertainties:        append([]simmodel.Uncertainty(nil), claim.Uncertainties...),
			ThermalModel:         claim.ThermalModel,
			TransientSOA:         append([]simmodel.TransientSOAEnvelope(nil), claim.TransientSOA...),
			AllowedAnalyses:      analyses,
			ProvenanceSource:     provenanceRecord.Provenance.Source,
			ProvenanceRevision:   provenanceRecord.Provenance.Revision,
			ProvenanceSHA256:     strings.ToLower(provenanceRecord.Provenance.SHA256),
			MinTemperatureC:      cloneInventoryFloat(provenanceRecord.Provenance.MinTemperatureC),
			MaxTemperatureC:      cloneInventoryFloat(provenanceRecord.Provenance.MaxTemperatureC),
		})
	}
	slices.SortFunc(result, func(left, right PrimitiveModelContract) int {
		return cmp.Compare(left.ModelID, right.ModelID)
	})
	return result, rejections
}

func primitiveFunctionalUnits(record components.ComponentRecord, descriptor simmodel.PrimitiveDescriptor) []string {
	if !descriptor.OpAmp && !descriptor.Comparator {
		return []string{""}
	}
	units := []string{}
	for _, symbol := range record.Symbols {
		if symbol.UnitType == components.SymbolUnitFunctional {
			unit := strings.TrimSpace(symbol.UnitID)
			if unit == "" {
				unit = fmt.Sprintf("%d", symbol.Unit)
			}
			units = append(units, unit)
		}
	}
	if len(units) == 0 {
		return []string{""}
	}
	slices.Sort(units)
	return slices.Compact(units)
}

func bindPrimitiveTerminals(record components.ComponentRecord, variant components.PackageVariant, unit string, descriptor simmodel.PrimitiveDescriptor) ([]PrimitiveTerminal, []string, bool) {
	terminals := make([]PrimitiveTerminal, 0, len(descriptor.Terminals))
	symbolIDs := []string{}
	for _, terminal := range descriptor.Terminals {
		symbol, functionPin, found := findPrimitiveFunction(record.Symbols, unit, terminal, descriptor)
		if !found {
			if defaultNet := descriptor.TerminalDefaults[terminal]; defaultNet != "" {
				terminals = append(terminals, PrimitiveTerminal{Terminal: terminal, DefaultNet: defaultNet})
				continue
			}
			return nil, nil, false
		}
		pad, found := findPrimitivePad(variant.PadFunctions, functionPin)
		if !found {
			return nil, nil, false
		}
		aliases := append([]string(nil), functionPin.Aliases...)
		slices.Sort(aliases)
		aliases = slices.Compact(aliases)
		unitID := strings.TrimSpace(symbol.UnitID)
		if unitID == "" && symbol.Unit != 0 {
			unitID = fmt.Sprintf("%d", symbol.Unit)
		}
		terminals = append(terminals, PrimitiveTerminal{
			Terminal:   terminal,
			Function:   functionPin.Function,
			Aliases:    aliases,
			SymbolID:   symbol.SymbolID,
			UnitID:     unitID,
			SymbolPin:  functionPin.SymbolPin,
			Pad:        pad.Pad,
			Electrical: functionPin.Electrical,
			Polarity:   functionPin.Polarity,
			Required:   functionPin.Required,
		})
		symbolIDs = append(symbolIDs, symbol.SymbolID)
	}
	slices.SortFunc(terminals, func(left, right PrimitiveTerminal) int {
		return cmp.Compare(left.Terminal, right.Terminal)
	})
	slices.Sort(symbolIDs)
	return terminals, slices.Compact(symbolIDs), true
}

func findPrimitiveFunction(symbols []components.SymbolBinding, unit, terminal string, descriptor simmodel.PrimitiveDescriptor) (components.SymbolBinding, components.FunctionPin, bool) {
	candidates := append([]string{terminal}, descriptor.TerminalAliases[terminal]...)
	search := func(symbol components.SymbolBinding) (components.FunctionPin, bool) {
		for _, pin := range symbol.FunctionPins {
			if stringMatchesAny(pin.Function, pin.Aliases, candidates) {
				return pin, true
			}
		}
		return components.FunctionPin{}, false
	}
	for _, symbol := range symbols {
		symbolUnit := strings.TrimSpace(symbol.UnitID)
		if symbolUnit == "" && symbol.Unit != 0 {
			symbolUnit = fmt.Sprintf("%d", symbol.Unit)
		}
		if unit != "" && symbolUnit != unit {
			continue
		}
		if pin, found := search(symbol); found {
			return symbol, pin, true
		}
	}
	for _, symbol := range symbols {
		if symbol.UnitType != components.SymbolUnitPower && !symbol.RequiredUnit {
			continue
		}
		if pin, found := search(symbol); found {
			return symbol, pin, true
		}
	}
	if unit == "" {
		for _, symbol := range symbols {
			if pin, found := search(symbol); found {
				return symbol, pin, true
			}
		}
	}
	return components.SymbolBinding{}, components.FunctionPin{}, false
}

func findPrimitivePad(pads []components.PadFunction, pin components.FunctionPin) (components.PadFunction, bool) {
	preferred := append([]string(nil), pin.Aliases...)
	preferred = append(preferred, pin.Function)
	for _, candidate := range preferred {
		for _, pad := range pads {
			if equalFold(candidate, pad.Function) || containsEqualFold(pad.Aliases, candidate) {
				return pad, true
			}
		}
	}
	return components.PadFunction{}, false
}

func stringMatchesAny(value string, aliases, candidates []string) bool {
	for _, candidate := range candidates {
		if equalFold(value, candidate) || containsEqualFold(aliases, candidate) {
			return true
		}
	}
	return false
}

func equalFold(left, right string) bool {
	return strings.EqualFold(strings.TrimSpace(left), strings.TrimSpace(right))
}

func containsEqualFold(values []string, target string) bool {
	for _, value := range values {
		if equalFold(value, target) {
			return true
		}
	}
	return false
}

func physicalEvidenceAccepted(record components.ComponentRecord, variant components.PackageVariant) bool {
	if !acceptedConfidence(record.Verification.Confidence) ||
		!acceptedConfidence(variant.Verification.Confidence) ||
		!record.Verification.PinMapChecked ||
		!variant.Verification.PinMapChecked ||
		strings.TrimSpace(variant.FootprintID) == "" ||
		len(variant.PadFunctions) == 0 ||
		len(record.Symbols) == 0 {
		return false
	}
	for _, symbol := range record.Symbols {
		if !acceptedConfidence(symbol.Verification.Confidence) ||
			!symbol.Verification.PinMapChecked ||
			strings.TrimSpace(symbol.SymbolID) == "" ||
			len(symbol.FunctionPins) == 0 {
			return false
		}
	}
	return true
}

func acceptedConfidence(confidence components.ConfidenceLevel) bool {
	return confidence == components.ConfidenceVerified ||
		confidence == components.ConfidenceLibraryDerived ||
		confidence == components.ConfidenceRuleInferred
}

func primitiveValueDomain(record components.ComponentRecord, required bool) (*PrimitiveValueDomain, bool) {
	if !required {
		return nil, true
	}
	expectedKind := map[string]string{
		"resistor":  "resistance",
		"capacitor": "capacitance",
		"inductor":  "inductance",
	}[record.Family]
	for _, value := range record.Values {
		if expectedKind != "" && value.Kind != expectedKind {
			continue
		}
		result := &PrimitiveValueDomain{Kind: value.Kind, Unit: canonicalUnit(value.Unit)}
		result.Minimum = engineeringPointer(value.Min)
		result.Nominal = engineeringPointer(value.Typ)
		result.Maximum = engineeringPointer(value.Max)
		if result.Minimum == nil && result.Nominal == nil && result.Maximum == nil {
			continue
		}
		return result, true
	}
	return nil, false
}

func primitiveBounds(values []components.RatingConstraint) []PrimitiveBound {
	result := make([]PrimitiveBound, 0, len(values))
	for _, value := range values {
		bound := PrimitiveBound{
			Kind:    value.Kind,
			Unit:    canonicalUnit(value.Unit),
			Minimum: engineeringPointer(value.Min),
			Nominal: engineeringPointer(value.Typ),
			Maximum: engineeringPointer(value.Max),
		}
		if bound.Minimum != nil || bound.Nominal != nil || bound.Maximum != nil {
			result = append(result, bound)
		}
	}
	slices.SortFunc(result, comparePrimitiveBounds)
	return result
}

func primitiveToleranceBounds(values []components.ToleranceConstraint) []PrimitiveBound {
	result := make([]PrimitiveBound, 0, len(values))
	for _, value := range values {
		bound := PrimitiveBound{
			Kind:    value.Kind,
			Unit:    canonicalUnit(value.Unit),
			Nominal: engineeringPointer(value.Typ),
			Maximum: engineeringPointer(value.Max),
		}
		if bound.Nominal != nil || bound.Maximum != nil {
			result = append(result, bound)
		}
	}
	slices.SortFunc(result, comparePrimitiveBounds)
	return result
}

func engineeringPointer(value string) *float64 {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parsed, ok := components.ParseEngineeringValue(value)
	if !ok || !finite(parsed) {
		return nil
	}
	return floatPointer(parsed)
}

func packageArea(variant components.PackageVariant) float64 {
	if variant.DimensionsMM == nil ||
		variant.DimensionsMM.Width <= 0 || variant.DimensionsMM.Height <= 0 ||
		!finite(variant.DimensionsMM.Width) || !finite(variant.DimensionsMM.Height) {
		return 0
	}
	return quantizeInventory(variant.DimensionsMM.Width * variant.DimensionsMM.Height)
}

func openTopologyPrimitiveKind(modelID string) string {
	switch modelID {
	case simmodel.PrimitiveResistorV1:
		return "resistor"
	case simmodel.PrimitiveCapacitorV1, simmodel.PrimitiveCapacitorTransientV1:
		return "capacitor"
	case simmodel.PrimitiveInductorTransientV1:
		return "inductor"
	case simmodel.PrimitiveOpAmpV1:
		return "opamp"
	case simmodel.PrimitiveComparatorOpenCollectorV1:
		return "comparator"
	case simmodel.PrimitiveFixedLinearRegulatorV1:
		return "fixed_voltage_regulator"
	case simmodel.PrimitiveAdjustableLinearRegulatorV1,
		simmodel.PrimitiveFloatingAdjustableRegulatorV1:
		return "adjustable_voltage_regulator"
	case simmodel.PrimitiveShuntVoltageReferenceV1, simmodel.PrimitiveUnidirectionalZenerV1:
		return "reference_diode"
	case simmodel.PrimitiveBidirectionalTVSV1:
		return "clamp_diode"
	case simmodel.PrimitiveDiodeShockleyV1:
		return "signal_diode"
	case simmodel.PrimitiveNMOSSwitchV1:
		return "n_channel_mosfet"
	case simmodel.PrimitivePMOSSwitchV1:
		return "p_channel_mosfet"
	case simmodel.PrimitiveBJTNPNV1:
		return "npn_bjt"
	case simmodel.PrimitiveBJTPNPV1:
		return "pnp_bjt"
	default:
		return ""
	}
}

func lifecycleUnavailable(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "obsolete", "discontinued", "not_recommended_for_new_design":
		return true
	default:
		return false
	}
}

func primitiveCandidateKey(catalogID, variantID, unitID string) string {
	key := catalogID + "|" + variantID
	if unitID != "" {
		key += "|" + unitID
	}
	return key
}

func comparePrimitiveCandidates(left, right PrimitiveCandidate) int {
	return cmp.Or(
		cmp.Compare(left.Kind, right.Kind),
		cmp.Compare(left.CatalogID, right.CatalogID),
		cmp.Compare(left.VariantID, right.VariantID),
		cmp.Compare(left.UnitID, right.UnitID),
	)
}

func comparePrimitiveBounds(left, right PrimitiveBound) int {
	return cmp.Or(cmp.Compare(left.Kind, right.Kind), cmp.Compare(left.Unit, right.Unit))
}

func primitiveRegistryHash() string {
	data, _ := json.Marshal(simmodel.PrimitiveDescriptors())
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func primitiveInventoryHash(inventory PrimitiveInventory) (string, error) {
	inventory.Hash = ""
	data, err := json.Marshal(inventory)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func (rejections inventoryRejections) add(code, sample string) {
	rejections[code] = append(rejections[code], sample)
}

func (rejections inventoryRejections) normalized() []InventoryRejection {
	codes := make([]string, 0, len(rejections))
	for code := range rejections {
		codes = append(codes, code)
	}
	slices.Sort(codes)
	result := make([]InventoryRejection, 0, len(codes))
	for _, code := range codes {
		samples := append([]string(nil), rejections[code]...)
		slices.Sort(samples)
		samples = slices.Compact(samples)
		count := len(samples)
		if len(samples) > inventorySampleLimit {
			samples = samples[:inventorySampleLimit]
		}
		result = append(result, InventoryRejection{Code: code, Count: count, Samples: samples})
	}
	return result
}

func floatPointer(value float64) *float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return nil
	}
	copy := value
	return &copy
}

func cloneInventoryFloat(value *float64) *float64 {
	if value == nil {
		return nil
	}
	return floatPointer(*value)
}

func quantizeInventory(value float64) float64 {
	return math.Round(value*1e12) / 1e12
}
