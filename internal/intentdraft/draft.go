package intentdraft

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"strings"
	"unicode"

	"kicadai/internal/designworkflow"
	"kicadai/internal/intentplanner"
	"kicadai/internal/reports"
)

const (
	SourceTypeText = "text"
	SourceTypeFile = "file"
)

type Options struct {
	SourceType         string                         `json:"source_type,omitempty"`
	SourceID           string                         `json:"source_id,omitempty"`
	AcceptanceOverride designworkflow.AcceptanceLevel `json:"acceptance,omitempty"`
	Strict             bool                           `json:"strict,omitempty"`
}

type Result struct {
	Request        intentplanner.Request `json:"request"`
	Extraction     ExtractionReport      `json:"extraction"`
	Clarifications []Clarification       `json:"clarifications,omitempty"`
	Issues         []reports.Issue       `json:"issues,omitempty"`
}

type ExtractionReport struct {
	SourceID    string            `json:"source_id"`
	SourceType  string            `json:"source_type"`
	SourceHash  string            `json:"source_hash"`
	Summary     string            `json:"summary,omitempty"`
	Fields      []ExtractedField  `json:"fields,omitempty"`
	Assumptions []DraftAssumption `json:"assumptions,omitempty"`
	Confidence  ConfidenceSummary `json:"confidence"`
}

type ExtractedField struct {
	Path       string   `json:"path"`
	Value      any      `json:"value,omitempty"`
	SourceText string   `json:"source_text,omitempty"`
	StartByte  int      `json:"start_byte"`
	EndByte    int      `json:"end_byte"`
	Confidence float64  `json:"confidence"`
	Method     string   `json:"method"`
	Notes      []string `json:"notes,omitempty"`
}

type DraftAssumption struct {
	ID         string  `json:"id"`
	Path       string  `json:"path,omitempty"`
	Message    string  `json:"message"`
	Confidence float64 `json:"confidence,omitempty"`
}

type ConfidenceSummary struct {
	Overall *float64 `json:"overall,omitempty"`
	Minimum *float64 `json:"minimum,omitempty"`
	Fields  int      `json:"fields"`
}

type Clarification struct {
	ID         string           `json:"id"`
	Path       string           `json:"path,omitempty"`
	Severity   string           `json:"severity"`
	Question   string           `json:"question"`
	Options    []string         `json:"options,omitempty"`
	Evidence   []ExtractedField `json:"evidence,omitempty"`
	Suggestion string           `json:"suggestion,omitempty"`
}

func Draft(text string, options Options) Result {
	sourceText := strings.TrimSpace(text)
	extraction := ExtractionReport{
		SourceID:   strings.TrimSpace(options.SourceID),
		SourceType: normalizeSourceType(options.SourceType),
		SourceHash: sourceHash(sourceText),
		Summary:    trimSummary(sourceText),
		Confidence: ConfidenceSummary{},
	}
	if extraction.SourceID == "" {
		extraction.SourceID = extraction.SourceType
	}
	request := intentplanner.Request{
		Version: intentplanner.RequestVersion,
		Name:    "natural_language_intent",
		Summary: extraction.Summary,
		Kind:    intentplanner.IntentCustomStructured,
	}
	if options.AcceptanceOverride != "" {
		request.Acceptance = options.AcceptanceOverride
	}
	var issues []reports.Issue
	if sourceText == "" {
		issues = append(issues, reports.Issue{
			Code:       reports.CodeInvalidArgument,
			Severity:   reports.SeverityError,
			Path:       "text",
			Message:    "intent text is required",
			Suggestion: "provide --text or --file with a natural-language board request",
		})
	}
	request = intentplanner.NormalizeRequest(request)
	issues = append(issues, intentplanner.ValidateRequest(request)...)
	return Result{
		Request:    request,
		Extraction: extraction,
		Issues:     issues,
	}
}

func normalizeSourceType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case SourceTypeFile:
		return SourceTypeFile
	default:
		return SourceTypeText
	}
}

func sourceHash(text string) string {
	hash := sha256.New()
	_, _ = io.WriteString(hash, text)
	return hex.EncodeToString(hash.Sum(nil))
}

func trimSummary(text string) string {
	const maxSummaryRunes = 240
	var builder strings.Builder
	builder.Grow(maxSummaryRunes * 4)
	spacePending := false
	runeCount := 0
	for _, r := range text {
		if unicode.IsSpace(r) {
			if builder.Len() > 0 {
				spacePending = true
			}
			continue
		}
		if spacePending {
			if runeCount+1 >= maxSummaryRunes {
				break
			}
			builder.WriteByte(' ')
			runeCount++
			spacePending = false
		}
		if runeCount >= maxSummaryRunes {
			break
		}
		builder.WriteRune(r)
		runeCount++
		if runeCount >= maxSummaryRunes {
			break
		}
	}
	return builder.String()
}
