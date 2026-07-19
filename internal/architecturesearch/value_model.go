package architecturesearch

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strings"

	"kicadai/internal/reports"
)

const (
	FormulaDividerAttenuator = "divider_attenuator"
	FormulaFeedbackDivider   = "feedback_divider"
	FormulaRCPole            = "rc_pole"
	FormulaSallenKeyLowPass  = "sallen_key_low_pass"
	FormulaHysteresis        = "comparator_hysteresis"
	FormulaGateDrive         = "gate_drive"
	FormulaCurrentSense      = "current_sense_transfer"
	FormulaRatingMargin      = "rating_margin"
	FormulaRevision          = "1.0.0"

	DefaultMaxValueCandidates   = 16
	DefaultMaxCornerEvaluations = 10000
)

const (
	CodeValueInputInvalid reports.Code = "ARCHITECTURE_VALUE_INPUT_INVALID"
	CodeValueUnsolved     reports.Code = "ARCHITECTURE_VALUE_UNSOLVED"
	CodeToleranceFailed   reports.Code = "ARCHITECTURE_TOLERANCE_FAILED"
	CodeRatingFailed      reports.Code = "ARCHITECTURE_RATING_FAILED"
	CodeCornerLimit       reports.Code = "ARCHITECTURE_CORNER_LIMIT"
)

type PreferredSeries string

const (
	SeriesE6  PreferredSeries = "E6"
	SeriesE12 PreferredSeries = "E12"
	SeriesE24 PreferredSeries = "E24"
	SeriesE48 PreferredSeries = "E48"
	SeriesE96 PreferredSeries = "E96"
)

type FormulaDescriptor struct {
	ID       string `json:"id"`
	Revision string `json:"revision"`
}

type NamedQuantity struct {
	Name  string  `json:"name"`
	Value float64 `json:"value"`
	Unit  string  `json:"unit"`
}

type SelectedValueEvidence struct {
	Name             string          `json:"name"`
	Ideal            float64         `json:"ideal"`
	Selected         float64         `json:"selected"`
	Unit             string          `json:"unit"`
	Series           PreferredSeries `json:"series"`
	TolerancePercent float64         `json:"tolerance_percent"`
	RelativeError    float64         `json:"relative_error"`
}

type CornerEvidence struct {
	ID      string          `json:"id"`
	Inputs  []NamedQuantity `json:"inputs"`
	Outputs []NamedQuantity `json:"outputs"`
}

type CalculationBound struct {
	Name          string  `json:"name"`
	Relation      string  `json:"relation"`
	Required      float64 `json:"required"`
	ObservedWorst float64 `json:"observed_worst"`
	Margin        float64 `json:"margin"`
	Unit          string  `json:"unit"`
	Pass          bool    `json:"pass"`
}

type ValueCandidateRejection struct {
	Value   float64 `json:"value"`
	Unit    string  `json:"unit"`
	Code    string  `json:"code"`
	Message string  `json:"message"`
	Margin  float64 `json:"margin"`
}

type CalculationEvidence struct {
	ID                 string                    `json:"id"`
	FormulaID          string                    `json:"formula_id"`
	FormulaRevision    string                    `json:"formula_revision"`
	FormulaLibraryHash string                    `json:"formula_library_hash"`
	Inputs             []NamedQuantity           `json:"inputs"`
	SelectedValues     []SelectedValueEvidence   `json:"selected_values,omitempty"`
	NominalOutputs     []NamedQuantity           `json:"nominal_outputs"`
	Corners            []CornerEvidence          `json:"corners,omitempty"`
	Bounds             []CalculationBound        `json:"bounds"`
	RejectedCandidates []ValueCandidateRejection `json:"rejected_candidates,omitempty"`
	CornerEvaluations  int                       `json:"corner_evaluations"`
	WorstMargin        float64                   `json:"worst_margin"`
	Pass               bool                      `json:"pass"`
	Hash               string                    `json:"hash"`
}

func FormulaLibraryHash() string {
	descriptors := []FormulaDescriptor{
		{ID: FormulaDividerAttenuator, Revision: FormulaRevision},
		{ID: FormulaFeedbackDivider, Revision: FormulaRevision},
		{ID: FormulaGateDrive, Revision: FormulaRevision},
		{ID: FormulaCurrentSense, Revision: FormulaRevision},
		{ID: FormulaHysteresis, Revision: FormulaRevision},
		{ID: FormulaRCPole, Revision: FormulaRevision},
		{ID: FormulaRatingMargin, Revision: FormulaRevision},
		{ID: FormulaSallenKeyLowPass, Revision: FormulaRevision},
	}
	encoded, _ := json.Marshal(descriptors)
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}

func FinalizeCalculation(evidence CalculationEvidence) (CalculationEvidence, error) {
	evidence.ID = canonicalIdentifier(evidence.ID)
	evidence.FormulaID = canonicalIdentifier(evidence.FormulaID)
	evidence.FormulaRevision = strings.TrimSpace(evidence.FormulaRevision)
	evidence.FormulaLibraryHash = FormulaLibraryHash()
	normalizeNamedQuantities(evidence.Inputs)
	normalizeNamedQuantities(evidence.NominalOutputs)
	slices.SortStableFunc(evidence.SelectedValues, func(left, right SelectedValueEvidence) int {
		return strings.Compare(left.Name, right.Name)
	})
	for index := range evidence.Corners {
		evidence.Corners[index].ID = canonicalIdentifier(evidence.Corners[index].ID)
		normalizeNamedQuantities(evidence.Corners[index].Inputs)
		normalizeNamedQuantities(evidence.Corners[index].Outputs)
	}
	slices.SortStableFunc(evidence.Corners, func(left, right CornerEvidence) int { return strings.Compare(left.ID, right.ID) })
	slices.SortStableFunc(evidence.Bounds, func(left, right CalculationBound) int {
		if order := strings.Compare(left.Name, right.Name); order != 0 {
			return order
		}
		return strings.Compare(left.Relation, right.Relation)
	})
	slices.SortStableFunc(evidence.RejectedCandidates, func(left, right ValueCandidateRejection) int {
		if left.Value < right.Value {
			return -1
		}
		if left.Value > right.Value {
			return 1
		}
		return strings.Compare(left.Code, right.Code)
	})
	evidence.Hash = ""
	encoded, err := json.Marshal(evidence)
	if err != nil {
		return evidence, err
	}
	sum := sha256.Sum256(encoded)
	evidence.Hash = hex.EncodeToString(sum[:])
	return evidence, nil
}

func ValidateCalculation(evidence CalculationEvidence) []reports.Issue {
	var issues []reports.Issue
	if !validSemanticID(evidence.ID) || !validSemanticID(evidence.FormulaID) || !validRevision(evidence.FormulaRevision) || evidence.FormulaLibraryHash != FormulaLibraryHash() {
		issues = append(issues, architectureIssue(CodeValueInputInvalid, "calculation", "calculation identity or formula-library evidence is invalid"))
	}
	if evidence.CornerEvaluations < 0 || evidence.CornerEvaluations > DefaultMaxCornerEvaluations || evidence.CornerEvaluations != len(evidence.Corners) {
		issues = append(issues, architectureIssue(CodeCornerLimit, "calculation.corners", "corner count is inconsistent or exceeds the deterministic limit"))
	}
	for index, bound := range evidence.Bounds {
		if !finiteNumbers(bound.Required, bound.ObservedWorst, bound.Margin) || bound.Pass != (bound.Margin >= 0) {
			issues = append(issues, architectureIssue(CodeToleranceFailed, fmt.Sprintf("calculation.bounds[%d]", index), "calculation bound evidence is inconsistent"))
		}
	}
	if !finiteNumbers(evidence.WorstMargin) || evidence.Pass != (evidence.WorstMargin >= 0) {
		issues = append(issues, architectureIssue(CodeToleranceFailed, "calculation.worst_margin", "calculation pass state does not match worst margin"))
	}
	finalized, err := FinalizeCalculation(evidence)
	if err != nil || finalized.Hash != evidence.Hash {
		issues = append(issues, architectureIssue(CodeValueInputInvalid, "calculation.hash", "calculation evidence hash is missing or invalid"))
	}
	return issues
}

// ObservedCalculation records a deterministic provider-derived scalar so a
// later composition pass can compare it with a system-level constraint. The
// zero floor makes negative or non-finite observations fail at the provider
// boundary instead of becoming untrusted global evidence.
func ObservedCalculation(id string, outputs ...NamedQuantity) (CalculationEvidence, error) {
	if len(outputs) == 0 {
		return CalculationEvidence{}, fmt.Errorf("observed calculation requires at least one output")
	}
	bounds := make([]CalculationBound, 0, len(outputs))
	worst := math.Inf(1)
	for _, output := range outputs {
		if !finiteNumbers(output.Value) || output.Value < 0 || !validSemanticID(canonicalIdentifier(output.Name)) {
			return CalculationEvidence{}, fmt.Errorf("observed calculation output is invalid")
		}
		bound := minimumBound(canonicalIdentifier(output.Name)+"_evidence", 0, output.Value, output.Unit)
		bounds = append(bounds, bound)
		worst = math.Min(worst, normalizedMargin(bound.Margin, 1))
	}
	evidence := CalculationEvidence{
		ID: id, FormulaID: FormulaRatingMargin, FormulaRevision: FormulaRevision,
		NominalOutputs: outputs, Bounds: bounds, WorstMargin: quantize(worst), Pass: true,
	}
	return FinalizeCalculation(evidence)
}

func normalizeNamedQuantities(values []NamedQuantity) {
	for index := range values {
		values[index].Name = canonicalIdentifier(values[index].Name)
		values[index].Unit = canonicalUnit(values[index].Unit)
		values[index].Value = quantize(values[index].Value)
	}
	slices.SortStableFunc(values, func(left, right NamedQuantity) int { return strings.Compare(left.Name, right.Name) })
}

func calculationIssue(code reports.Code, path, message string) []reports.Issue {
	return []reports.Issue{architectureIssue(code, path, message)}
}
