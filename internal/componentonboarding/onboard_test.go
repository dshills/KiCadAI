package componentonboarding

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"kicadai/internal/aiprovider"
	"kicadai/internal/components"
	"kicadai/internal/libraryresolver"
	"kicadai/internal/modelprovenance"
	"kicadai/internal/simmodel"
)

type fixedExtractor struct {
	extraction Extraction
}

type repeatedByteReader struct {
	remaining int64
	value     byte
}

func (reader *repeatedByteReader) Read(buffer []byte) (int, error) {
	if reader.remaining == 0 {
		return 0, io.EOF
	}
	count := int64(len(buffer))
	if count > reader.remaining {
		count = reader.remaining
	}
	for index := int64(0); index < count; index++ {
		buffer[index] = reader.value
	}
	reader.remaining -= count
	return int(count), nil
}

type captureAIProvider struct {
	result  aiprovider.GenerateResult
	request aiprovider.GenerateRequest
}

func (provider *captureAIProvider) Name() string { return "capture" }

func (provider *captureAIProvider) GenerateIntent(_ context.Context, request aiprovider.GenerateRequest) (aiprovider.GenerateResult, error) {
	provider.request = request
	return provider.result, nil
}

func (extractor fixedExtractor) Extract(context.Context, BehavioralRequirement, []DocumentInput) (Extraction, error) {
	return extractor.extraction, nil
}

func TestOnboardQuarantinesRanksPromotesAndAppliesUnfamiliarComponent(t *testing.T) {
	base, requirement, documents, extraction, libraries := onboardingFixture(t)
	candidate, err := Onboard(context.Background(), requirement, documents, fixedExtractor{extraction}, base, libraries)
	if err != nil {
		t.Fatal(err)
	}
	if candidate.Status != StatusQuarantined || candidate.SelectedID != "opamp.example.ax101.sot23_5" {
		t.Fatalf("candidate status/selection = %q/%q", candidate.Status, candidate.SelectedID)
	}
	if slices.ContainsFunc(base.Records, func(record components.ComponentRecord) bool {
		return record.ID == candidate.SelectedID
	}) {
		t.Fatal("quarantined record mutated base catalog")
	}
	again, err := Onboard(context.Background(), requirement, documents, fixedExtractor{extraction}, base, libraries)
	if err != nil {
		t.Fatal(err)
	}
	if candidate.Hash != again.Hash {
		t.Fatalf("onboarding is nondeterministic: %s != %s", candidate.Hash, again.Hash)
	}

	gates := passingGates()
	approval := Approval{
		CandidateHash: candidate.Hash, Decision: "approve", Reviewer: "independent-reviewer",
		ReviewRef: "review://component-onboarding/ax101", ReviewSHA256: hashText("approved"),
	}
	promotion, overlay, err := Promote(candidate, documents, gates, approval, base, libraries)
	if err != nil {
		t.Fatal(err)
	}
	if promotion.Hash == "" || overlay.Status != StatusSupported || len(overlay.Records) != 1 {
		t.Fatalf("promotion/overlay incomplete: %#v %#v", promotion, overlay)
	}
	catalog, models, err := ApplyOverlay(base, modelprovenance.Registry{}, overlay, libraries)
	if err != nil {
		t.Fatal(err)
	}
	missingLibraries := libraries
	missingLibraries.Footprints = map[string]libraryresolver.FootprintRecord{}
	if _, _, err := ApplyOverlay(base, modelprovenance.Registry{}, overlay, missingLibraries); err == nil {
		t.Fatal("overlay was applied without its verified KiCad footprint")
	}
	selection, result := components.Select(context.Background(), catalog, components.SelectionRequest{
		Query:      components.Query{Text: candidate.SelectedID, Family: requirement.Family},
		Acceptance: components.AcceptanceERCDRC, RequiredRatings: requirement.RequiredRatings,
		RequiredTemperature: &requirement.RequiredTemperature, RequiredFunctions: requirement.RequiredFunctions,
		RequireConcrete: true,
	})
	if !result.OK || selection.Component.ID != candidate.SelectedID {
		t.Fatalf("promoted record is not selectable: %#v %#v", selection, result)
	}
	if _, found := modelprovenance.Lookup(models, candidate.SelectedID, extraction.Candidates[0].Model.ModelID); !found {
		t.Fatal("promoted model provenance is unavailable")
	}
	path := filepath.Join(t.TempDir(), "overlay.json")
	if err := WriteArtifact(path, overlay); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeOverlay(strings.NewReader(string(body)))
	if err != nil || decoded.Hash != overlay.Hash {
		t.Fatalf("overlay codec = %#v, %v", decoded, err)
	}
	decoded.Records[0].MPN = "tampered"
	if _, _, err := ApplyOverlay(base, modelprovenance.Registry{}, decoded, libraries); err == nil {
		t.Fatal("tampered overlay was applied")
	}
}

func TestOnboardFailsClosedForUntrustedEvidence(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*BehavioralRequirement, *[]DocumentInput, *Extraction, *libraryresolver.LibraryIndex)
	}{
		{name: "fabricated excerpt", mutate: func(_ *BehavioralRequirement, _ *[]DocumentInput, extraction *Extraction, _ *libraryresolver.LibraryIndex) {
			extraction.Claims[0].Excerpt = "not in source"
		}},
		{name: "value absent from excerpt", mutate: func(_ *BehavioralRequirement, _ *[]DocumentInput, extraction *Extraction, _ *libraryresolver.LibraryIndex) {
			extraction.Claims[0].Excerpt = extraction.Claims[1].Excerpt
		}},
		{name: "conflicting claim", mutate: func(_ *BehavioralRequirement, _ *[]DocumentInput, extraction *Extraction, _ *libraryresolver.LibraryIndex) {
			conflict := extraction.Claims[0]
			conflict.ID = "claim.conflict"
			conflict.Value = "other"
			extraction.Claims = append(extraction.Claims, conflict)
		}},
		{name: "missing category", mutate: func(_ *BehavioralRequirement, _ *[]DocumentInput, extraction *Extraction, _ *libraryresolver.LibraryIndex) {
			extraction.Candidates[0].Evidence = slices.DeleteFunc(extraction.Candidates[0].Evidence, func(binding EvidenceBinding) bool {
				return binding.Path == "derating"
			})
		}},
		{name: "unknown footprint pad", mutate: func(_ *BehavioralRequirement, _ *[]DocumentInput, extraction *Extraction, _ *libraryresolver.LibraryIndex) {
			extraction.Candidates[0].Record.Packages[0].PadFunctions[0].Pad = "99"
		}},
		{name: "missing derating", mutate: func(_ *BehavioralRequirement, _ *[]DocumentInput, extraction *Extraction, _ *libraryresolver.LibraryIndex) {
			extraction.Candidates[0].Record.DeratingRules = nil
		}},
		{name: "untrusted model hash", mutate: func(_ *BehavioralRequirement, _ *[]DocumentInput, extraction *Extraction, _ *libraryresolver.LibraryIndex) {
			extraction.Candidates[0].Model.Provenance.Provenance.SHA256 = strings.Repeat("0", 64)
		}},
		{name: "insufficient rating margin", mutate: func(requirement *BehavioralRequirement, _ *[]DocumentInput, _ *Extraction, _ *libraryresolver.LibraryIndex) {
			requirement.MinimumDerating = 2
		}},
		{name: "source digest mismatch", mutate: func(_ *BehavioralRequirement, documents *[]DocumentInput, _ *Extraction, _ *libraryresolver.LibraryIndex) {
			(*documents)[0].ExpectedSHA256 = strings.Repeat("f", 64)
		}},
		{name: "publisher mismatch", mutate: func(_ *BehavioralRequirement, documents *[]DocumentInput, _ *Extraction, _ *libraryresolver.LibraryIndex) {
			(*documents)[0].Publisher = "Different Manufacturer"
		}},
		{name: "model license missing", mutate: func(_ *BehavioralRequirement, documents *[]DocumentInput, _ *Extraction, _ *libraryresolver.LibraryIndex) {
			(*documents)[0].Kind = DocumentModel
			(*documents)[0].License = ""
		}},
		{name: "insufficient temperature range", mutate: func(requirement *BehavioralRequirement, _ *[]DocumentInput, _ *Extraction, _ *libraryresolver.LibraryIndex) {
			minimum := -55.0
			requirement.RequiredTemperature.MinimumC = &minimum
		}},
		{name: "missing library symbol", mutate: func(_ *BehavioralRequirement, _ *[]DocumentInput, _ *Extraction, libraries *libraryresolver.LibraryIndex) {
			libraries.Symbols = map[string]libraryresolver.SymbolRecord{}
		}},
		{name: "missing library symbol pin", mutate: func(_ *BehavioralRequirement, _ *[]DocumentInput, extraction *Extraction, _ *libraryresolver.LibraryIndex) {
			extraction.Candidates[0].Record.Symbols[0].FunctionPins[0].SymbolPin = "99"
		}},
		{name: "missing library footprint", mutate: func(_ *BehavioralRequirement, _ *[]DocumentInput, _ *Extraction, libraries *libraryresolver.LibraryIndex) {
			libraries.Footprints = map[string]libraryresolver.FootprintRecord{}
		}},
		{name: "unregistered model", mutate: func(_ *BehavioralRequirement, _ *[]DocumentInput, extraction *Extraction, _ *libraryresolver.LibraryIndex) {
			extraction.Candidates[0].Model.ModelID = "unregistered_model"
			extraction.Candidates[0].Model.Provenance.ModelID = "unregistered_model"
			extraction.Candidates[0].Record.SimulationModels[0].ModelID = "unregistered_model"
		}},
		{name: "analysis incompatible model", mutate: func(requirement *BehavioralRequirement, _ *[]DocumentInput, _ *Extraction, _ *libraryresolver.LibraryIndex) {
			requirement.RequiredAnalyses = []string{simmodel.AnalysisThermal}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			base, requirement, documents, extraction, libraries := onboardingFixture(t)
			test.mutate(&requirement, &documents, &extraction, &libraries)
			if _, err := Onboard(context.Background(), requirement, documents, fixedExtractor{extraction}, base, libraries); err == nil {
				t.Fatal("unsafe onboarding unexpectedly succeeded")
			}
		})
	}
}

func TestClaimValueAnchoringRejectsSubstringsAndReusedPinToken(t *testing.T) {
	tests := []struct {
		name  string
		claim Claim
		want  bool
	}{
		{name: "numeric substring", claim: Claim{Field: "rating.current.max", Value: "10", Excerpt: "maximum current: 110 mA"}},
		{name: "package suffix", claim: Claim{Field: "rating.current.max", Value: "23", Excerpt: "package: SOT-23"}},
		{name: "negative-value suffix", claim: Claim{Field: "temperature.max", Value: "40", Excerpt: "minimum temperature: -40 C"}},
		{name: "unit substring", claim: Claim{Field: "rating.voltage.max", Value: "5", Unit: "V", Excerpt: "maximum voltage: 5 volts"}},
		{name: "one token reused for equal pin and pad", claim: Claim{Field: "pin.OUT", Value: "3:3", Excerpt: "OUT is pin 3"}},
		{name: "distinct equal pin and pad tokens", claim: Claim{Field: "pin.OUT", Value: "3:3", Excerpt: "OUT symbol pin 3 maps to package pad 3"}, want: true},
		{name: "anchored engineering value", claim: Claim{Field: "rating.voltage.max", Value: "5.5", Unit: "V", Excerpt: "maximum supply: 5.5 V"}, want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := claimValueAnchored(test.claim); got != test.want {
				t.Fatalf("claimValueAnchored(%#v) = %t, want %t", test.claim, got, test.want)
			}
		})
	}
}

func TestDecodeRequirementStreamsAndRejectsOversizedArtifact(t *testing.T) {
	reader := io.MultiReader(
		strings.NewReader("{}"),
		&repeatedByteReader{remaining: maxArtifactBytes + 1, value: ' '},
	)
	if _, err := DecodeRequirement(reader); err == nil || !strings.Contains(err.Error(), "artifact exceeds") {
		t.Fatalf("oversized streamed artifact error = %v", err)
	}
}

func TestPromotionRequiresTwoDeterministicPhysicalRuns(t *testing.T) {
	base, requirement, documents, extraction, libraries := onboardingFixture(t)
	candidate, err := Onboard(context.Background(), requirement, documents, fixedExtractor{extraction}, base, libraries)
	if err != nil {
		t.Fatal(err)
	}
	approval := Approval{
		CandidateHash: candidate.Hash, Decision: "approve", Reviewer: "reviewer",
		ReviewRef: "review://test", ReviewSHA256: hashText("review"),
	}
	tests := []struct {
		name   string
		mutate func([]GateEvidence) []GateEvidence
	}{
		{name: "missing run", mutate: func(gates []GateEvidence) []GateEvidence { return gates[1:] }},
		{name: "failed gate", mutate: func(gates []GateEvidence) []GateEvidence {
			gates[0].Passed = false
			return gates
		}},
		{name: "nondeterministic gate", mutate: func(gates []GateEvidence) []GateEvidence {
			gates[1].EvidenceSHA256 = hashText("different")
			return gates
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, _, err := Promote(candidate, documents, test.mutate(passingGates()), approval, base, libraries); err == nil {
				t.Fatal("unsafe promotion unexpectedly succeeded")
			}
		})
	}
	badApproval := approval
	badApproval.CandidateHash = hashText("different-candidate")
	if _, _, err := Promote(candidate, documents, passingGates(), badApproval, base, libraries); err == nil {
		t.Fatal("candidate-hash-mismatched approval unexpectedly succeeded")
	}
}

func TestAIExtractorUsesStructuredProviderButDoesNotBypassValidation(t *testing.T) {
	base, requirement, documents, extraction, libraries := onboardingFixture(t)
	body, err := json.Marshal(extraction)
	if err != nil {
		t.Fatal(err)
	}
	provider := &captureAIProvider{result: aiprovider.GenerateResult{Provider: "capture", IntentJSON: body}}
	extractor := AIExtractor{
		Provider: provider,
		OutputSchema: map[string]any{
			"type": "object", "additionalProperties": false,
		},
	}
	candidate, err := Onboard(context.Background(), requirement, documents, extractor, base, libraries)
	if err != nil {
		t.Fatal(err)
	}
	if candidate.SelectedID == "" || provider.request.SchemaVersion != ExtractionSchema ||
		provider.request.OutputSchemaName != "component_onboarding_extraction" ||
		!strings.Contains(provider.request.Prompt, documents[0].ExpectedSHA256) {
		t.Fatalf("AI extraction request/candidate is incomplete: %#v %#v", provider.request, candidate)
	}
	provider.result.IntentJSON = []byte(`{"schema":"wrong","claims":[],"candidates":[]}`)
	if _, err := Onboard(context.Background(), requirement, documents, extractor, base, libraries); err == nil {
		t.Fatal("malformed AI extraction bypassed deterministic validation")
	}
}

func TestExtractionPromptRejectsOversizedAggregateBeforeMarshal(t *testing.T) {
	_, err := extractionPrompt(BehavioralRequirement{}, []DocumentInput{{
		ID: "oversized", Content: make([]byte, aiprovider.MaxPromptBytes+1),
	}})
	if err == nil || !strings.Contains(err.Error(), "pre-marshal bound") {
		t.Fatalf("oversized extraction input was not rejected safely: %v", err)
	}
}

func TestDocumentLocatorRejectsLocalFileReference(t *testing.T) {
	if err := validateLocator("file:///tmp/untrusted-datasheet.txt"); err == nil {
		t.Fatal("local file locator was accepted")
	}
	if err := validateLocator("https://manufacturer.example/datasheet.txt"); err != nil {
		t.Fatalf("publisher HTTPS locator was rejected: %v", err)
	}
	if err := validateLocator("https://manufacturer.example"); err != nil {
		t.Fatalf("publisher HTTPS root locator was rejected: %v", err)
	}
	if err := validateLocator("kicadai:document/sha256-example"); err != nil {
		t.Fatalf("internal content identity was rejected: %v", err)
	}
}

func TestHashWithoutFieldOmitsOnlySupportedTopLevelArtifactHash(t *testing.T) {
	candidate := Candidate{Schema: CandidateSchema, Hash: "candidate-one"}
	first, err := hashWithoutField(candidate)
	if err != nil {
		t.Fatal(err)
	}
	candidate.Hash = "candidate-two"
	second, err := hashWithoutField(candidate)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatal("candidate top-level hash was included in its canonical hash")
	}

	promotion := Promotion{Schema: PromotionSchema, Candidate: candidate, Hash: "promotion-one"}
	first, err = hashWithoutField(promotion)
	if err != nil {
		t.Fatal(err)
	}
	promotion.Hash = "promotion-two"
	second, err = hashWithoutField(promotion)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatal("promotion top-level hash was included in its canonical hash")
	}
	promotion.Candidate.Hash = "candidate-three"
	nested, err := hashWithoutField(promotion)
	if err != nil {
		t.Fatal(err)
	}
	if second == nested {
		t.Fatal("nested candidate hash was omitted from the promotion approval chain")
	}

	overlay := SupportedOverlay{Schema: OverlaySchema, Hash: "overlay-one"}
	first, err = hashWithoutField(overlay)
	if err != nil {
		t.Fatal(err)
	}
	overlay.Hash = "overlay-two"
	second, err = hashWithoutField(overlay)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatal("overlay top-level hash was included in its canonical hash")
	}
}

func TestValidateFunctionPinClaimsRequiresEveryRepeatedMapping(t *testing.T) {
	symbols := []components.SymbolBinding{{FunctionPins: []components.FunctionPin{
		{Function: "GND", SymbolPin: "1"},
		{Function: "GND", SymbolPin: "2"},
	}}}
	pads := []components.PadFunction{
		{Function: "GND", Pad: "A"},
		{Function: "GND", Pad: "B"},
	}
	claims := []Claim{
		{ID: "pin-1", Field: "pin.GND", Value: "1:A"},
		{ID: "pin-2", Field: "pin.GND", Value: "2:B"},
	}
	if err := validateFunctionPinClaims(symbols, pads, "GND", claims); err != nil {
		t.Fatalf("complete repeated-function map was rejected: %v", err)
	}
	if err := validateFunctionPinClaims(symbols, pads, "GND", claims[:1]); err == nil {
		t.Fatal("incomplete repeated-function map was accepted")
	}
	if err := validateFunctionPinClaims(symbols, pads[:1], "GND", []Claim{
		{ID: "many-to-one-1", Field: "pin.GND", Value: "1:A"},
		{ID: "many-to-one-2", Field: "pin.GND", Value: "2:A"},
	}); err != nil {
		t.Fatalf("valid many-to-one power mapping was rejected: %v", err)
	}
	onePinSymbols := []components.SymbolBinding{{FunctionPins: symbols[0].FunctionPins[:1]}}
	if err := validateFunctionPinClaims(onePinSymbols, pads, "GND", []Claim{
		{ID: "one-to-many-1", Field: "pin.GND", Value: "1:A"},
		{ID: "one-to-many-2", Field: "pin.GND", Value: "1:B"},
	}); err != nil {
		t.Fatalf("valid one-to-many thermal mapping was rejected: %v", err)
	}
	duplicateMapping := append([]Claim(nil), claims...)
	duplicateMapping[1].Value = "1:A"
	if err := validateFunctionPinClaims(symbols, pads, "GND", duplicateMapping); err == nil {
		t.Fatal("duplicate mapping edge was accepted")
	}
}

func onboardingFixture(t *testing.T) (
	*components.Catalog,
	BehavioralRequirement,
	[]DocumentInput,
	Extraction,
	libraryresolver.LibraryIndex,
) {
	t.Helper()
	base, err := components.LoadCatalog(context.Background(), components.LoadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	templateIndex := slices.IndexFunc(base.Records, func(record components.ComponentRecord) bool {
		return record.ID == "opamp.ti.lmv321.sot23_5"
	})
	if templateIndex < 0 {
		t.Fatal("op-amp template is absent")
	}
	record := base.Records[templateIndex]
	record.ID = "opamp.example.ax101.sot23_5"
	record.Name = "Example AX101 single op-amp"
	record.Manufacturer = "Example Analog"
	record.MPN = "AX101"
	record.Packages[0].MPN = record.MPN
	record.DeratingRules = []components.DeratingRule{{
		Kind: "supply_voltage", Expression: "operating_voltage <= rated_voltage / 1.2",
		Description: "Maintain at least twenty percent supply-voltage margin.",
	}}
	record.Verification = components.VerificationRecord{
		Confidence: components.ConfidenceVerified, ResolverChecked: true, PinMapChecked: true,
	}

	lines := []string{
		"Manufacturer: Example Analog",
		"Part number: AX101",
		"Family: opamp",
		"Supply voltage maximum: 5.5 V",
		"Operating temperature minimum: -40 C",
		"Operating temperature maximum: 125 C",
		"Footprint: Package_TO_SOT_SMD:SOT-23-5",
		"Pin IN_MINUS maps symbol 3 to package pad 3",
		"Pin IN_PLUS maps symbol 1 to package pad 1",
		"Pin OUT maps symbol 4 to package pad 4",
		"Pin V_MINUS maps symbol 2 to package pad 2",
		"Pin V_PLUS maps symbol 5 to package pad 5",
		"Derating: operating_voltage <= rated_voltage / 1.2",
		"Analytic model: mna_opamp_single_pole_v1",
		"Document revision: A",
	}
	content := []byte(strings.Join(lines, "\n"))
	document := DocumentInput{
		ID: "ax101.datasheet", Kind: DocumentDatasheet, Publisher: record.Manufacturer,
		Locator: "https://example.invalid/ax101-datasheet", Revision: "A",
		ExpectedSHA256: hashBytes(content), Content: content,
	}
	record.Verification.Sources = []string{"document:" + document.ID + "@sha256:" + document.ExpectedSHA256}

	claims := []Claim{
		claim("claim.family", document.ID, record.MPN, "identity.family", record.Family, "", lines[2]),
		claim("claim.manufacturer", document.ID, record.MPN, "identity.manufacturer", record.Manufacturer, "", lines[0]),
		claim("claim.mpn", document.ID, record.MPN, "identity.mpn", record.MPN, "", lines[1]),
		claim("claim.rating", document.ID, record.MPN, "rating.supply_voltage.max", "5.5", "V", lines[3]),
		claim("claim.temp.min", document.ID, record.MPN, "temperature.min", "-40", "C", lines[4]),
		claim("claim.temp.max", document.ID, record.MPN, "temperature.max", "125", "C", lines[5]),
		claim("claim.package", document.ID, record.MPN, "package.footprint", record.Packages[0].FootprintID, "", lines[6]),
		claim("claim.pin.in_minus", document.ID, record.MPN, "pin.IN_MINUS", "3:3", "", lines[7]),
		claim("claim.pin.in_plus", document.ID, record.MPN, "pin.IN_PLUS", "1:1", "", lines[8]),
		claim("claim.pin.out", document.ID, record.MPN, "pin.OUT", "4:4", "", lines[9]),
		claim("claim.pin.v_minus", document.ID, record.MPN, "pin.V_MINUS", "2:2", "", lines[10]),
		claim("claim.pin.v_plus", document.ID, record.MPN, "pin.V_PLUS", "5:5", "", lines[11]),
		claim("claim.derating", document.ID, record.MPN, "derating.supply_voltage", record.DeratingRules[0].Expression, "", lines[12]),
		claim("claim.model", document.ID, record.MPN, "model.id", record.SimulationModels[0].ModelID, "", lines[13]),
		claim("claim.provenance", document.ID, record.MPN, "provenance.revision", document.Revision, "", lines[14]),
	}
	modelHash, ok := simmodel.ModelContentHash(record.SimulationModels[0].ModelID)
	if !ok {
		t.Fatal("trusted op-amp model is absent")
	}
	minimum, maximum := -40.0, 125.0
	model := ModelProposal{
		Kind: ModelBounded, ModelID: record.SimulationModels[0].ModelID,
		Provenance: modelprovenance.Record{
			CatalogID: record.ID, Family: record.Family, ModelID: record.SimulationModels[0].ModelID,
			Provenance: simmodel.ModelProvenance{
				Source: "document:" + document.ID, Revision: document.Revision, SHA256: modelHash,
				ReviewStatus: "reviewed", AllowedAnalyses: []string{simmodel.AnalysisDCOperatingPoint},
				MinTemperatureC: &minimum, MaxTemperatureC: &maximum,
			},
		},
		ClaimIDs:           []string{"claim.model"},
		BoundedAssumptions: []string{"single-pole small-signal behavior inside documented supply and temperature bounds"},
	}
	extraction := Extraction{
		Schema: ExtractionSchema, Claims: claims,
		Candidates: []ComponentProposal{{
			Record: record,
			Evidence: []EvidenceBinding{
				{Path: "derating", ClaimIDs: []string{"claim.derating"}},
				{Path: "identity", ClaimIDs: []string{"claim.family", "claim.manufacturer", "claim.mpn"}},
				{Path: "model", ClaimIDs: []string{"claim.model"}},
				{Path: "package", ClaimIDs: []string{"claim.package"}},
				{Path: "pin_mapping", ClaimIDs: []string{"claim.pin.in_minus", "claim.pin.in_plus", "claim.pin.out", "claim.pin.v_minus", "claim.pin.v_plus"}},
				{Path: "provenance", ClaimIDs: []string{"claim.provenance"}},
				{Path: "ratings", ClaimIDs: []string{"claim.rating"}},
				{Path: "temperature", ClaimIDs: []string{"claim.temp.max", "claim.temp.min"}},
			},
			Model: model,
		}},
	}
	minTemp, maxTemp := -20.0, 85.0
	requirement := BehavioralRequirement{
		Schema: RequestSchema, ID: "low_voltage_buffer", Family: "opamp",
		RequiredFunctions:   []string{"IN_MINUS", "IN_PLUS", "OUT", "V_MINUS", "V_PLUS"},
		RequiredRatings:     []components.RequiredRating{{Kind: "supply_voltage", Value: "4", Unit: "V"}},
		RequiredTemperature: components.TemperatureRequirement{MinimumC: &minTemp, MaximumC: &maxTemp},
		RequiredAnalyses:    []string{simmodel.AnalysisDCOperatingPoint},
		AllowedPackages:     []string{"sot23_5"}, MinimumDerating: 1.2,
	}
	symbol := record.Symbols[0]
	librarySymbol := libraryresolver.SymbolRecord{LibraryID: symbol.SymbolID}
	for _, pin := range symbol.FunctionPins {
		librarySymbol.Pins = append(librarySymbol.Pins, libraryresolver.SymbolPin{Number: pin.SymbolPin, Name: pin.Function})
	}
	variant := record.Packages[0]
	footprint := libraryresolver.FootprintRecord{FootprintID: variant.FootprintID}
	for _, pad := range variant.PadFunctions {
		footprint.Pads = append(footprint.Pads, libraryresolver.FootprintPad{Name: pad.Pad, PinFunction: pad.Function})
	}
	libraries := libraryresolver.LibraryIndex{
		Symbols:    map[string]libraryresolver.SymbolRecord{symbol.SymbolID: librarySymbol},
		Footprints: map[string]libraryresolver.FootprintRecord{variant.FootprintID: footprint},
	}
	return base, requirement, []DocumentInput{document}, extraction, libraries
}

func claim(id, documentID, subject, field, value, unit, excerpt string) Claim {
	return Claim{
		ID: id, DocumentID: documentID, Subject: subject, Field: field,
		Value: value, Unit: unit, Excerpt: excerpt, Location: "section:test",
	}
}

func passingGates() []GateEvidence {
	var gates []GateEvidence
	for _, gate := range RequiredPromotionGates {
		hash := hashText("normalized-" + gate)
		for run := 1; run <= 2; run++ {
			gates = append(gates, GateEvidence{
				Gate: gate, Run: run, Passed: true,
				EvidencePath:   fmt.Sprintf("evidence/%s/run-%d.json", gate, run),
				EvidenceSHA256: hash,
			})
		}
	}
	return gates
}

func hashText(value string) string {
	return hashBytes([]byte(value))
}

func hashBytes(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}
