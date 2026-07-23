package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

type externalReviewMatrix struct {
	SchemaVersion string                   `json:"schema_version"`
	Scenarios     []externalReviewScenario `json:"scenarios"`
	NegativeCases []externalReviewNegative `json:"negative_cases"`
}

const externalReviewMatrixScenarioCountV1 = 6

type externalReviewScenario struct {
	ID                 string              `json:"id"`
	ReviewEquivalent   string              `json:"review_equivalent"`
	Lane               string              `json:"lane"`
	Fixture            string              `json:"fixture"`
	Board              externalReviewBoard `json:"board"`
	ExpectedStatus     string              `json:"expected_status"`
	RequiredArtifacts  []string            `json:"required_artifacts"`
	InternalGates      []string            `json:"internal_gates"`
	OptionalKiCadGates []string            `json:"optional_kicad_gates"`
}

type externalReviewBoard struct {
	Mode     string  `json:"mode"`
	WidthMM  float64 `json:"width_mm,omitempty"`
	HeightMM float64 `json:"height_mm,omitempty"`
	Layers   int     `json:"layers"`
}

type externalReviewNegative struct {
	ID string `json:"id"`
}

func TestExternalReviewMatrixManifest(t *testing.T) {
	repoRoot := aiPromotionRepoRoot(t)
	path := filepath.Join(repoRoot, "testdata", "external-review-mitigation", "matrix.json")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var matrix externalReviewMatrix
	if err := decoder.Decode(&matrix); err != nil {
		t.Fatalf("decode matrix: %v", err)
	}
	if matrix.SchemaVersion != "kicadai.external-review-matrix.v1" {
		t.Fatalf("schema_version = %q", matrix.SchemaVersion)
	}
	// This milestone deliberately freezes the six requests in the independent
	// review. A later ladder is a versioned matrix change, not an accidental
	// append to this acceptance contract.
	if len(matrix.Scenarios) != externalReviewMatrixScenarioCountV1 {
		t.Fatalf("scenario count = %d, want 6", len(matrix.Scenarios))
	}

	wantArtifacts := []string{"design-promotion.json", "design-request.json", "manifest.json", "transaction.json", "validation-summary.json", "workflow-result.json"}
	wantInternal := []string{"connectivity", "deterministic_repeat", "route_completion", "round_trip", "routing", "writer_correctness"}
	wantKiCad := []string{"erc", "round_trip", "strict_drc", "writer_correctness"}
	seenIDs := make(map[string]bool, len(matrix.Scenarios))
	for _, scenario := range matrix.Scenarios {
		t.Run(scenario.ID, func(t *testing.T) {
			if scenario.ID == "" || seenIDs[scenario.ID] {
				t.Fatalf("missing or duplicate scenario id %q", scenario.ID)
			}
			seenIDs[scenario.ID] = true
			if scenario.ReviewEquivalent == "" || scenario.ExpectedStatus != "pass" {
				t.Fatalf("review equivalent/status = %q/%q", scenario.ReviewEquivalent, scenario.ExpectedStatus)
			}
			if !oneOf(scenario.Lane, "design", "intent", "circuit-explicit", "circuit-function") {
				t.Fatalf("unsupported lane %q", scenario.Lane)
			}
			if filepath.IsAbs(scenario.Fixture) || strings.Contains(filepath.ToSlash(scenario.Fixture), "../") {
				t.Fatalf("fixture must be repository-relative: %q", scenario.Fixture)
			}
			info, err := os.Stat(filepath.Join(repoRoot, filepath.FromSlash(scenario.Fixture)))
			if err != nil {
				t.Fatalf("fixture %q is not a file: %v", scenario.Fixture, err)
			}
			if info.IsDir() {
				t.Fatalf("fixture %q is a directory", scenario.Fixture)
			}
			fixtureBoard := readExternalReviewFixtureBoard(t, filepath.Join(repoRoot, filepath.FromSlash(scenario.Fixture)))
			if scenario.Board.Layers != 2 {
				t.Fatalf("layers = %d, want 2", scenario.Board.Layers)
			}
			switch scenario.Board.Mode {
			case "declared":
				if scenario.Board.WidthMM <= 0 || scenario.Board.HeightMM <= 0 {
					t.Fatalf("declared board lacks positive dimensions: %#v", scenario.Board)
				}
				if !externalReviewBoardsEqual(fixtureBoard, scenario.Board) {
					t.Fatalf("matrix board = %#v, fixture board = %#v", scenario.Board, fixtureBoard)
				}
			case "synthesized":
				if scenario.Board.WidthMM != 0 || scenario.Board.HeightMM != 0 {
					t.Fatalf("synthesized board must not freeze derived dimensions: %#v", scenario.Board)
				}
			default:
				t.Fatalf("unsupported board mode %q", scenario.Board.Mode)
			}
			assertContainsExactly(t, scenario.RequiredArtifacts, wantArtifacts, "required artifacts")
			assertContains(t, scenario.InternalGates, wantInternal, "internal gates")
			assertContains(t, scenario.OptionalKiCadGates, wantKiCad, "optional KiCad gates")
		})
	}

	wantNegatives := []string{"artifact_write_failure", "infeasible_rigid_group", "invalid_function_parameter", "referenced_malformed_library_object"}
	gotNegatives := make([]string, 0, len(matrix.NegativeCases))
	for _, negative := range matrix.NegativeCases {
		if negative.ID == "" {
			t.Fatalf("invalid negative case %#v", negative)
		}
		gotNegatives = append(gotNegatives, negative.ID)
	}
	sort.Strings(gotNegatives)
	if !reflect.DeepEqual(gotNegatives, wantNegatives) {
		t.Fatalf("negative cases = %v, want %v", gotNegatives, wantNegatives)
	}
}

func TestExternalReviewMatrixCLI(t *testing.T) {
	requireExternalReviewMatrixRun(t)
	t.Run("intent regulator planning", TestRunIntentPlanRegulatorEvidenceFixtures)
	t.Run("unreferenced library diagnostics", TestCircuitPreflightIgnoresUnreferencedLibraryDiagnostics)
	t.Run("referenced malformed library object", TestCircuitPreflightBlocksReferencedLibraryDefects)
	t.Run("deterministic RC creation", TestCircuitCreateRCGraphIsDeterministic)
	t.Run("shared core evidence", TestCircuitCreateWritesSharedCoreEvidence)
	t.Run("JSON success and failure", TestCircuitJSONSuccessAndFailureEachEmitOneDocument)
}

func requireExternalReviewMatrixRun(t *testing.T) {
	t.Helper()
	if os.Getenv("KICADAI_RUN_EXTERNAL_REVIEW_MATRIX") != "1" {
		t.Skip("run through make review-matrix")
	}
}

func externalReviewBoardsEqual(left, right externalReviewBoard) bool {
	const epsilon = 1e-9
	return left.Mode == right.Mode &&
		left.Layers == right.Layers &&
		math.Abs(left.WidthMM-right.WidthMM) <= epsilon &&
		math.Abs(left.HeightMM-right.HeightMM) <= epsilon
}

func readExternalReviewFixtureBoard(t *testing.T, path string) externalReviewBoard {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Board   *externalReviewBoard `json:"board"`
		Project struct {
			Board *externalReviewBoard `json:"board"`
		} `json:"project"`
	}
	if err := json.Unmarshal(body, &fixture); err != nil {
		t.Fatalf("decode fixture board: %v", err)
	}
	if fixture.Board != nil {
		fixture.Board.Mode = "declared"
		return *fixture.Board
	}
	if fixture.Project.Board != nil {
		fixture.Project.Board.Mode = "declared"
		return *fixture.Project.Board
	}
	return externalReviewBoard{Mode: "synthesized", Layers: 2}
}

func assertContainsExactly(t *testing.T, got, want []string, label string) {
	t.Helper()
	got = append([]string(nil), got...)
	sort.Strings(got)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%s = %v, want %v", label, got, want)
	}
}

func assertContains(t *testing.T, got, want []string, label string) {
	t.Helper()
	values := make(map[string]bool, len(got))
	for _, value := range got {
		if values[value] {
			t.Fatalf("%s contains duplicate %q", label, value)
		}
		values[value] = true
	}
	for _, value := range want {
		if !values[value] {
			t.Errorf("%s missing %q", label, value)
		}
	}
}

func oneOf(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

func Example_externalReviewMatrix() {
	fmt.Println("make review-matrix")
	// Output: make review-matrix
}
