package promotionrunner

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

var identifierPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,127}$`)

const (
	MatrixSchema = "kicadai.external-review-matrix.v1"
	LaneSchema   = "kicadai.promotion-lanes.v1"
)

type Board struct {
	Mode     string  `json:"mode"`
	WidthMM  float64 `json:"width_mm,omitempty"`
	HeightMM float64 `json:"height_mm,omitempty"`
	Layers   int     `json:"layers"`
}

type Scenario struct {
	ID                 string   `json:"id"`
	ReviewEquivalent   string   `json:"review_equivalent"`
	Lane               string   `json:"lane"`
	Fixture            string   `json:"fixture"`
	Board              Board    `json:"board"`
	ExpectedStatus     string   `json:"expected_status"`
	RequiredArtifacts  []string `json:"required_artifacts"`
	InternalGates      []string `json:"internal_gates"`
	OptionalKiCadGates []string `json:"optional_kicad_gates"`
}

type NegativeCase struct {
	ID string `json:"id"`
}

type Matrix struct {
	SchemaVersion string         `json:"schema_version"`
	Scenarios     []Scenario     `json:"scenarios"`
	NegativeCases []NegativeCase `json:"negative_cases"`
}

type MatrixDocument struct {
	Matrix Matrix
	Path   string
	SHA256 string
}

type Lane struct {
	Name    string   `json:"name"`
	Command []string `json:"command"`
}

var lanes = []Lane{
	{Name: "circuit-explicit", Command: []string{"circuit", "create"}},
	{Name: "circuit-function", Command: []string{"circuit", "create"}},
	{Name: "design", Command: []string{"design", "create"}},
	{Name: "intent", Command: []string{"intent", "create"}},
	{Name: "requirement", Command: []string{"requirement", "create"}},
}

var strictGateContract = struct {
	Internal  []string
	KiCad     []string
	Promotion []string
}{
	Internal:  []string{"routing", "connectivity", "route_completion", "writer_correctness", "round_trip", "deterministic_repeat"},
	KiCad:     []string{"erc", "strict_drc", "writer_correctness", "round_trip"},
	Promotion: []string{"connectivity", "kicad_checks", "route_completion", "writer_correctness"},
}

func LoadMatrix(path, repositoryRoot string) (MatrixDocument, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return MatrixDocument{}, fmt.Errorf("read promotion matrix: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var matrix Matrix
	if err := decoder.Decode(&matrix); err != nil {
		return MatrixDocument{}, fmt.Errorf("decode promotion matrix: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return MatrixDocument{}, errors.New("promotion matrix must contain one JSON value")
	}
	if err := validateMatrix(matrix, repositoryRoot); err != nil {
		return MatrixDocument{}, err
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return MatrixDocument{}, err
	}
	sum := sha256.Sum256(data)
	return MatrixDocument{Matrix: matrix, Path: absolute, SHA256: hex.EncodeToString(sum[:])}, nil
}

func LaneFor(name string) (Lane, error) {
	for _, lane := range lanes {
		if lane.Name == name {
			return lane, nil
		}
	}
	return Lane{}, fmt.Errorf("unsupported promotion lane %q", name)
}

func LaneRegistrySHA256() string {
	data, err := json.Marshal(struct {
		Schema string `json:"schema"`
		Lanes  []Lane `json:"lanes"`
	}{Schema: LaneSchema, Lanes: lanes})
	if err != nil {
		panic(fmt.Sprintf("marshal static promotion lane registry: %v", err))
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func validateMatrix(matrix Matrix, repositoryRoot string) error {
	if matrix.SchemaVersion != MatrixSchema || len(matrix.Scenarios) == 0 {
		return fmt.Errorf("unsupported or empty promotion matrix %q", matrix.SchemaVersion)
	}
	seen := map[string]bool{}
	for index, scenario := range matrix.Scenarios {
		if !identifierPattern.MatchString(scenario.ID) || seen[scenario.ID] {
			return fmt.Errorf("scenarios[%d] has an invalid or duplicate id %q", index, scenario.ID)
		}
		seen[scenario.ID] = true
		if _, err := LaneFor(scenario.Lane); err != nil {
			return fmt.Errorf("scenarios[%d]: %w", index, err)
		}
		if !safeRepositoryPath(scenario.Fixture) {
			return fmt.Errorf("scenarios[%d] has unsafe fixture path %q", index, scenario.Fixture)
		}
		if info, err := os.Stat(filepath.Join(repositoryRoot, filepath.FromSlash(scenario.Fixture))); err != nil || !info.Mode().IsRegular() {
			return fmt.Errorf("scenarios[%d] fixture is not a regular repository file", index)
		}
		if scenario.ExpectedStatus != "pass" {
			return fmt.Errorf("scenarios[%d] positive promotion status must be pass", index)
		}
		if scenario.Board.Layers <= 0 || scenario.Board.Mode != "declared" && scenario.Board.Mode != "synthesized" {
			return fmt.Errorf("scenarios[%d] has invalid board contract", index)
		}
		if scenario.Board.Mode == "declared" && (scenario.Board.WidthMM <= 0 || scenario.Board.HeightMM <= 0) {
			return fmt.Errorf("scenarios[%d] declared board dimensions must be positive", index)
		}
		for field, values := range map[string][]string{
			"required_artifacts":   scenario.RequiredArtifacts,
			"internal_gates":       scenario.InternalGates,
			"optional_kicad_gates": scenario.OptionalKiCadGates,
		} {
			if len(values) == 0 || hasEmptyOrDuplicate(values) {
				return fmt.Errorf("scenarios[%d].%s must be non-empty and unique", index, field)
			}
		}
		for _, artifact := range scenario.RequiredArtifacts {
			if filepath.Base(artifact) != artifact || artifact == "." || artifact == ".." {
				return fmt.Errorf("scenarios[%d] has unsafe required artifact %q", index, artifact)
			}
		}
		for field, contract := range map[string]struct {
			actual, required []string
		}{
			"required_artifacts":   {scenario.RequiredArtifacts, []string{"design-request.json", "transaction.json", "workflow-result.json", "validation-summary.json", "design-promotion.json", "manifest.json"}},
			"internal_gates":       {scenario.InternalGates, strictGateContract.Internal},
			"optional_kicad_gates": {scenario.OptionalKiCadGates, strictGateContract.KiCad},
		} {
			if missing := missingValues(contract.actual, contract.required); len(missing) != 0 {
				return fmt.Errorf("scenarios[%d].%s is missing %s", index, field, strings.Join(missing, ", "))
			}
		}
	}
	for index, negative := range matrix.NegativeCases {
		if !identifierPattern.MatchString(negative.ID) || seen[negative.ID] {
			return fmt.Errorf("negative_cases[%d] has an invalid or duplicate id %q", index, negative.ID)
		}
		seen[negative.ID] = true
	}
	return nil
}

func safeRepositoryPath(value string) bool {
	clean := filepath.Clean(filepath.FromSlash(value))
	return value != "" && !filepath.IsAbs(clean) && clean != "." && clean != ".." &&
		!strings.HasPrefix(clean, ".."+string(filepath.Separator))
}

func hasEmptyOrDuplicate(values []string) bool {
	sorted := slices.Clone(values)
	slices.Sort(sorted)
	for index, value := range sorted {
		if strings.TrimSpace(value) == "" || index > 0 && value == sorted[index-1] {
			return true
		}
	}
	return false
}

func missingValues(actual, required []string) []string {
	present := make(map[string]bool, len(actual))
	for _, value := range actual {
		present[value] = true
	}
	var missing []string
	for _, value := range required {
		if !present[value] {
			missing = append(missing, value)
		}
	}
	return missing
}
