package componentonboarding

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"kicadai/internal/components"
	"kicadai/internal/libraryresolver"
	"kicadai/internal/modelprovenance"
	"kicadai/internal/simmodel"
)

type heldOutManifest struct {
	Schema string        `json:"schema"`
	Cases  []heldOutCase `json:"cases"`
}

type heldOutCase struct {
	ID           string   `json:"id"`
	Category     string   `json:"category"`
	TemplateID   string   `json:"template_id"`
	CandidateID  string   `json:"candidate_id"`
	Manufacturer string   `json:"manufacturer"`
	MPN          string   `json:"mpn"`
	RatingKind   string   `json:"rating_kind"`
	RatingValue  string   `json:"rating_value"`
	RatingUnit   string   `json:"rating_unit"`
	Analysis     string   `json:"analysis"`
	Functions    []string `json:"functions"`
}

func TestHeldOutCorpusOnboardsSevenUnfamiliarFamilies(t *testing.T) {
	manifest := loadHeldOutManifest(t)
	expectedCategories := []string{"converter", "interface", "logic", "opamp", "regulator", "sensor", "transistor"}
	actualCategories := make([]string, 0, len(manifest.Cases))
	for _, testCase := range manifest.Cases {
		actualCategories = append(actualCategories, testCase.Category)
		t.Run(testCase.ID, func(t *testing.T) {
			base, requirement, documents, extraction, libraries := heldOutFixture(t, testCase)
			candidate, err := Onboard(context.Background(), requirement, documents, fixedExtractor{extraction}, base, libraries)
			if err != nil {
				t.Fatal(err)
			}
			reordered := extraction
			reordered.Claims = append([]Claim(nil), extraction.Claims...)
			slices.Reverse(reordered.Claims)
			again, err := Onboard(context.Background(), requirement, documents, fixedExtractor{reordered}, base, libraries)
			if err != nil {
				t.Fatal(err)
			}
			if candidate.Hash != again.Hash || candidate.SelectedID != testCase.CandidateID {
				t.Fatalf("held-out replay mismatch: %s/%s selected %q", candidate.Hash, again.Hash, candidate.SelectedID)
			}
			approval := Approval{
				CandidateHash: candidate.Hash, Decision: "approve", Reviewer: "corpus-reviewer",
				ReviewRef: "review://held-out/" + testCase.ID, ReviewSHA256: hashText("review-" + testCase.ID),
			}
			_, overlay, err := Promote(candidate, documents, passingGates(), approval, base, libraries)
			if err != nil {
				t.Fatal(err)
			}
			catalog, models, err := ApplyOverlay(base, modelprovenance.Registry{}, overlay, libraries)
			if err != nil {
				t.Fatal(err)
			}
			selection, result := components.Select(context.Background(), catalog, components.SelectionRequest{
				Query:      components.Query{Text: testCase.CandidateID, Family: requirement.Family},
				Acceptance: components.AcceptanceERCDRC, RequiredRatings: requirement.RequiredRatings,
				RequiredTemperature: &requirement.RequiredTemperature, RequiredFunctions: requirement.RequiredFunctions,
				RequireConcrete: true,
			})
			if !result.OK || selection.Component.ID != testCase.CandidateID {
				t.Fatalf("promoted held-out candidate is not selectable: %#v %#v", selection, result)
			}
			if _, found := modelprovenance.Lookup(models, testCase.CandidateID, extraction.Candidates[0].Model.ModelID); !found {
				t.Fatal("promoted held-out model is not selectable")
			}
		})
	}
	slices.Sort(actualCategories)
	if !slices.Equal(actualCategories, expectedCategories) {
		t.Fatalf("held-out categories = %#v", actualCategories)
	}
}

func TestHeldOutIdentitiesDoNotAppearInProductionGo(t *testing.T) {
	manifest := loadHeldOutManifest(t)
	root := filepath.Clean(filepath.Join("..", ".."))
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == ".git" || entry.Name() == ".tmp" || entry.Name() == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, testCase := range manifest.Cases {
			if strings.Contains(string(body), testCase.CandidateID) || strings.Contains(string(body), testCase.MPN) {
				t.Fatalf("held-out identity %q leaked into production file %s", testCase.MPN, path)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func loadHeldOutManifest(t *testing.T) heldOutManifest {
	t.Helper()
	path := filepath.Join("testdata", "held_out_corpus", "manifest.json")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	expectedBody, err := os.ReadFile(filepath.Join("testdata", "held_out_corpus", "manifest.sha256"))
	if err != nil {
		t.Fatal(err)
	}
	if actual, expected := hashBytes(body), strings.TrimSpace(string(expectedBody)); actual != expected {
		t.Fatalf("held-out manifest hash = %s, want %s", actual, expected)
	}
	var manifest heldOutManifest
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Schema != "kicadai.component-onboarding-held-out-corpus.v1" || len(manifest.Cases) != 7 {
		t.Fatalf("held-out manifest identity/count = %q/%d", manifest.Schema, len(manifest.Cases))
	}
	ids := make([]string, 0, len(manifest.Cases))
	for _, testCase := range manifest.Cases {
		ids = append(ids, testCase.ID)
		if !slices.IsSorted(testCase.Functions) {
			t.Fatalf("%s functions are not canonical", testCase.ID)
		}
	}
	if !slices.IsSorted(ids) {
		t.Fatal("held-out cases are not canonically ordered")
	}
	return manifest
}

func heldOutFixture(t *testing.T, testCase heldOutCase) (
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
		return record.ID == testCase.TemplateID
	})
	if templateIndex < 0 {
		t.Fatalf("template %s is absent", testCase.TemplateID)
	}
	record := base.Records[templateIndex]
	record.ID = testCase.CandidateID
	record.Name = testCase.Manufacturer + " " + testCase.MPN
	record.Manufacturer = testCase.Manufacturer
	record.MPN = testCase.MPN
	record.Equivalence = nil
	if len(record.Packages) == 0 || len(record.Symbols) == 0 || len(record.SimulationModels) == 0 {
		t.Fatalf("template %s lacks physical or model evidence", testCase.TemplateID)
	}
	record.Packages[0].MPN = record.MPN
	if record.Temperature == nil {
		record.Temperature = &components.TemperatureRange{Min: "-40", Max: "85", Unit: "C"}
	}
	record.DeratingRules = []components.DeratingRule{{
		Kind: "operating_limit", Expression: "applied_rating <= documented_rating / 1.2",
		Description: "Maintain at least twenty percent margin from the documented absolute rating.",
	}}
	record.Verification = components.VerificationRecord{
		Confidence: components.ConfidenceVerified, ResolverChecked: true, PinMapChecked: true,
	}
	ratingIndex := slices.IndexFunc(record.Ratings, func(rating components.RatingConstraint) bool {
		return rating.Kind == testCase.RatingKind && rating.Unit == testCase.RatingUnit
	})
	if ratingIndex < 0 {
		t.Fatalf("template %s lacks rating %s", testCase.TemplateID, testCase.RatingKind)
	}
	rating := record.Ratings[ratingIndex]
	ratingEvidenceValue := rating.Max
	ratingSuffix := "max"
	if ratingEvidenceValue == "" {
		ratingEvidenceValue = rating.Typ
		ratingSuffix = "typ"
	}
	if ratingEvidenceValue == "" {
		ratingEvidenceValue = rating.Min
		ratingSuffix = "min"
	}

	lines := []string{
		"Manufacturer: " + record.Manufacturer,
		"Part number: " + record.MPN,
		"Family: " + record.Family,
		"Rating " + rating.Kind + " " + ratingSuffix + ": " + ratingEvidenceValue + " " + rating.Unit,
		"Operating temperature minimum: " + record.Temperature.Min + " " + record.Temperature.Unit,
		"Operating temperature maximum: " + record.Temperature.Max + " " + record.Temperature.Unit,
		"Footprint: " + record.Packages[0].FootprintID,
		"Derating: " + record.DeratingRules[0].Expression,
		"Analytic model: " + record.SimulationModels[0].ModelID,
		"Document revision: A",
	}
	pinLines := map[string][]string{}
	for _, function := range testCase.Functions {
		symbolPins := functionSymbolPins(record.Symbols, function)
		pads := functionPads(record.Packages[0].PadFunctions, function)
		if len(symbolPins) == 0 || len(symbolPins) != len(pads) {
			t.Fatalf("template %s lacks %s pin mapping", testCase.TemplateID, function)
		}
		for index, symbolPin := range symbolPins {
			line := "Pin " + function + " maps symbol " + symbolPin + " to package pad " + pads[index]
			pinLines[function] = append(pinLines[function], line)
			lines = append(lines, line)
		}
	}
	content := []byte(strings.Join(lines, "\n"))
	document := DocumentInput{
		ID: testCase.ID + ".datasheet", Kind: DocumentDatasheet,
		Publisher: record.Manufacturer, Locator: "https://corpus.invalid/" + testCase.MPN,
		Revision: "A", ExpectedSHA256: hashBytes(content), Content: content,
	}
	record.Verification.Sources = []string{"document:" + document.ID + "@sha256:" + document.ExpectedSHA256}
	claims := []Claim{
		claim(testCase.ID+".claim.family", document.ID, record.MPN, "identity.family", record.Family, "", lines[2]),
		claim(testCase.ID+".claim.manufacturer", document.ID, record.MPN, "identity.manufacturer", record.Manufacturer, "", lines[0]),
		claim(testCase.ID+".claim.mpn", document.ID, record.MPN, "identity.mpn", record.MPN, "", lines[1]),
		claim(testCase.ID+".claim.rating", document.ID, record.MPN, "rating."+rating.Kind+"."+ratingSuffix, ratingEvidenceValue, rating.Unit, lines[3]),
		claim(testCase.ID+".claim.temp.min", document.ID, record.MPN, "temperature.min", record.Temperature.Min, record.Temperature.Unit, lines[4]),
		claim(testCase.ID+".claim.temp.max", document.ID, record.MPN, "temperature.max", record.Temperature.Max, record.Temperature.Unit, lines[5]),
		claim(testCase.ID+".claim.package", document.ID, record.MPN, "package.footprint", record.Packages[0].FootprintID, "", lines[6]),
		claim(testCase.ID+".claim.derating", document.ID, record.MPN, "derating."+record.DeratingRules[0].Kind, record.DeratingRules[0].Expression, "", lines[7]),
		claim(testCase.ID+".claim.model", document.ID, record.MPN, "model.id", record.SimulationModels[0].ModelID, "", lines[8]),
		claim(testCase.ID+".claim.provenance", document.ID, record.MPN, "provenance.revision", document.Revision, "", lines[9]),
	}
	pinClaimIDs := make([]string, 0, len(testCase.Functions))
	for _, function := range testCase.Functions {
		symbolPins := functionSymbolPins(record.Symbols, function)
		pads := functionPads(record.Packages[0].PadFunctions, function)
		for index, symbolPin := range symbolPins {
			id := fmt.Sprintf("%s.claim.pin.%s.%d", testCase.ID, strings.ToLower(function), index+1)
			claims = append(claims, claim(
				id,
				document.ID,
				record.MPN,
				"pin."+function,
				symbolPin+":"+pads[index],
				"",
				pinLines[function][index],
			))
			pinClaimIDs = append(pinClaimIDs, id)
		}
	}
	slices.SortStableFunc(claims, func(left, right Claim) int { return strings.Compare(left.ID, right.ID) })
	slices.Sort(pinClaimIDs)

	modelHash, ok := simmodel.ModelContentHash(record.SimulationModels[0].ModelID)
	if !ok {
		t.Fatalf("model %s is not registered", record.SimulationModels[0].ModelID)
	}
	minimum, _ := components.ParseEngineeringValue(record.Temperature.Min)
	maximum, _ := components.ParseEngineeringValue(record.Temperature.Max)
	model := ModelProposal{
		Kind: ModelBounded, ModelID: record.SimulationModels[0].ModelID,
		Provenance: modelprovenance.Record{
			CatalogID: record.ID, Family: record.Family, ModelID: record.SimulationModels[0].ModelID,
			Provenance: simmodel.ModelProvenance{
				Source: "document:" + document.ID, Revision: document.Revision, SHA256: modelHash,
				ReviewStatus: "reviewed", AllowedAnalyses: []string{testCase.Analysis},
				MinTemperatureC: &minimum, MaxTemperatureC: &maximum,
			},
		},
		ClaimIDs:           []string{testCase.ID + ".claim.model"},
		BoundedAssumptions: []string{"catalog primitive is limited to documented ratings and temperature bounds"},
	}
	extraction := Extraction{
		Schema: ExtractionSchema, Claims: claims,
		Candidates: []ComponentProposal{{
			Record: record,
			Evidence: []EvidenceBinding{
				{Path: "derating", ClaimIDs: []string{testCase.ID + ".claim.derating"}},
				{Path: "identity", ClaimIDs: []string{testCase.ID + ".claim.family", testCase.ID + ".claim.manufacturer", testCase.ID + ".claim.mpn"}},
				{Path: "model", ClaimIDs: []string{testCase.ID + ".claim.model"}},
				{Path: "package", ClaimIDs: []string{testCase.ID + ".claim.package"}},
				{Path: "pin_mapping", ClaimIDs: pinClaimIDs},
				{Path: "provenance", ClaimIDs: []string{testCase.ID + ".claim.provenance"}},
				{Path: "ratings", ClaimIDs: []string{testCase.ID + ".claim.rating"}},
				{Path: "temperature", ClaimIDs: []string{testCase.ID + ".claim.temp.max", testCase.ID + ".claim.temp.min"}},
			},
			Model: model,
		}},
	}
	requiredMin, requiredMax := -20.0, 70.0
	requirement := BehavioralRequirement{
		Schema: RequestSchema, ID: testCase.ID, Family: record.Family,
		RequiredFunctions: append([]string(nil), testCase.Functions...),
		RequiredRatings: []components.RequiredRating{{
			Kind: testCase.RatingKind, Value: testCase.RatingValue, Unit: testCase.RatingUnit,
		}},
		RequiredTemperature: components.TemperatureRequirement{MinimumC: &requiredMin, MaximumC: &requiredMax},
		RequiredAnalyses:    []string{testCase.Analysis},
		AllowedPackages:     []string{record.Packages[0].PackageType}, MinimumDerating: 1.2,
	}
	libraries := libraryresolver.LibraryIndex{
		Symbols:    map[string]libraryresolver.SymbolRecord{},
		Footprints: map[string]libraryresolver.FootprintRecord{},
	}
	for _, symbol := range record.Symbols {
		librarySymbol := libraryresolver.SymbolRecord{LibraryID: symbol.SymbolID}
		for _, pin := range symbol.FunctionPins {
			librarySymbol.Pins = append(librarySymbol.Pins, libraryresolver.SymbolPin{Number: pin.SymbolPin, Name: pin.Function})
		}
		libraries.Symbols[symbol.SymbolID] = librarySymbol
	}
	for _, variant := range record.Packages {
		footprint := libraryresolver.FootprintRecord{FootprintID: variant.FootprintID}
		for _, pad := range variant.PadFunctions {
			footprint.Pads = append(footprint.Pads, libraryresolver.FootprintPad{Name: pad.Pad, PinFunction: pad.Function})
		}
		libraries.Footprints[variant.FootprintID] = footprint
	}
	return base, requirement, []DocumentInput{document}, extraction, libraries
}
