package workflows

import (
	"fmt"
	"strings"

	"kicadai/internal/placement"
	"kicadai/internal/reports"
)

type FeedbackStatus string

const (
	FeedbackStatusReady         FeedbackStatus = "ready"
	FeedbackStatusNeedsRevision FeedbackStatus = "needs_revision"
	FeedbackStatusBlocked       FeedbackStatus = "blocked"
)

type DesignFeedback struct {
	Stage           string                          `json:"stage"`
	Status          FeedbackStatus                  `json:"status"`
	Summary         string                          `json:"summary"`
	Diagnostics     []placement.PlacementDiagnostic `json:"diagnostics,omitempty"`
	Recommendations []string                        `json:"recommendations,omitempty"`
	Placement       *PlacementFeedback              `json:"placement,omitempty"`
}

type PlacementFeedback struct {
	Status  placement.Status        `json:"status"`
	Metrics placement.Metrics       `json:"metrics"`
	Quality placement.QualityReport `json:"quality"`
}

func EvaluatePlacement(request placement.Request, result placement.Result) DesignFeedback {
	quality := placement.BuildQualityReport(request, result)
	diagnostics := quality.Diagnostics
	status := feedbackStatus(quality, diagnostics)
	feedback := DesignFeedback{
		Stage:           "placement",
		Status:          status,
		Summary:         placementFeedbackSummary(status, quality, diagnostics),
		Diagnostics:     diagnostics,
		Recommendations: placementRecommendations(diagnostics),
		Placement: &PlacementFeedback{
			Status:  result.Status,
			Metrics: result.Metrics,
			Quality: quality,
		},
	}
	return feedback
}

func feedbackStatus(quality placement.QualityReport, diagnostics []placement.PlacementDiagnostic) FeedbackStatus {
	if len(quality.UnplacedRefs) > 0 {
		return FeedbackStatusBlocked
	}
	hasWarning := false
	for _, diagnostic := range diagnostics {
		if diagnostic.Severity == reports.SeverityBlocked || diagnostic.Severity == reports.SeverityError {
			return FeedbackStatusBlocked
		}
		if diagnostic.Severity == reports.SeverityWarning {
			hasWarning = true
		}
	}
	if !quality.Ready || hasWarning {
		return FeedbackStatusNeedsRevision
	}
	return FeedbackStatusReady
}

func placementFeedbackSummary(status FeedbackStatus, quality placement.QualityReport, diagnostics []placement.PlacementDiagnostic) string {
	switch status {
	case FeedbackStatusReady:
		return fmt.Sprintf("Placement is ready for routing with %d placed components.", quality.Metrics.PlacedCount)
	case FeedbackStatusBlocked:
		if len(quality.UnplacedRefs) > 0 {
			return "Placement is blocked by unplaced components: " + summarizedRefs(quality.UnplacedRefs, 5) + "."
		}
		if message := firstBlockingDiagnosticMessage(diagnostics); message != "" {
			return "Placement is blocked: " + message
		}
		return "Placement is blocked by validation issues."
	case FeedbackStatusNeedsRevision:
		if len(quality.EstimatedBoundsRefs) > 0 {
			return "Placement needs revision because some components use estimated or pad-derived bounds."
		}
		return "Placement needs revision before routing."
	default:
		return "Placement feedback status is unknown."
	}
}

func summarizedRefs(refs []string, limit int) string {
	if len(refs) == 0 {
		return ""
	}
	if limit <= 0 {
		return fmt.Sprintf("%d components", len(refs))
	}
	if len(refs) <= limit {
		return strings.Join(refs, ", ")
	}
	return strings.Join(refs[:limit], ", ") + fmt.Sprintf(", and %d more", len(refs)-limit)
}

func firstBlockingDiagnosticMessage(diagnostics []placement.PlacementDiagnostic) string {
	for _, diagnostic := range diagnostics {
		if diagnostic.Severity == reports.SeverityBlocked || diagnostic.Severity == reports.SeverityError {
			return strings.TrimSpace(diagnostic.Message)
		}
	}
	return ""
}

func placementRecommendations(diagnostics []placement.PlacementDiagnostic) []string {
	seen := map[string]struct{}{}
	recommendations := make([]string, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		suggestion := strings.TrimSpace(diagnostic.Suggestion)
		if suggestion == "" {
			continue
		}
		if _, ok := seen[suggestion]; ok {
			continue
		}
		seen[suggestion] = struct{}{}
		recommendations = append(recommendations, suggestion)
	}
	return recommendations
}
