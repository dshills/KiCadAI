package fabrication

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"kicadai/internal/components"
	"kicadai/internal/reports"
)

func ApplyProcurementSnapshots(ctx context.Context, data ReportData, opts Options) ReportData {
	sources := opts.Sources
	if sources == nil && strings.TrimSpace(opts.SourceDir) != "" {
		loaded, err := components.LoadSources(ctx, components.SourceLoadOptions{SourceDir: opts.SourceDir})
		if err != nil {
			data.Issues = append(data.Issues, reports.Issue{Code: components.CodeSourceReadFailed, Severity: reports.SeverityError, Path: "fabrication.source_dir", Message: err.Error()})
			return data
		}
		sources = loaded
	}
	if sources == nil {
		return data
	}
	data.Issues = append(data.Issues, sources.Diagnostics...)
	rows, issues := EnrichBOMRowsWithProcurement(data.BOM, sources, components.ProcurementPolicy{})
	data.BOM = rows
	data.Issues = append(data.Issues, issues...)
	return data
}

func EnrichBOMRowsWithProcurement(rows []BOMRow, sources *components.SourceCollection, policy components.ProcurementPolicy) ([]BOMRow, []reports.Issue) {
	if sources == nil {
		return rows, nil
	}
	now := time.Now().UTC()
	if policy.Now != nil {
		now = policy.Now.UTC()
	}
	lifecycleMaxAge := policy.MaxLifecycleAgeDays
	if lifecycleMaxAge <= 0 {
		lifecycleMaxAge = 730
	}
	availabilityMaxAge := policy.MaxAvailabilityAgeDays
	if availabilityMaxAge <= 0 {
		availabilityMaxAge = 30
	}
	out := slices.Clone(rows)
	var issues []reports.Issue
	for index := range out {
		row := &out[index]
		ref := procurementReference(row.References)
		manufacturer := strings.TrimSpace(row.Manufacturer)
		mpn := strings.TrimSpace(row.MPN)
		if manufacturer == "" || mpn == "" {
			if policy.RequireLifecycle || policy.RequireAvailability {
				row.ProcurementOutcome = "blocked"
				issues = append(issues, procurementIssue(reports.SeverityError, "bom."+ref+".procurement", ref, "BOM row is missing manufacturer or MPN for procurement source lookup"))
			}
			continue
		}
		source, ok := sources.Find(manufacturer, mpn)
		if !ok {
			if policy.RequireLifecycle || policy.RequireAvailability {
				row.ProcurementOutcome = "blocked"
				issues = append(issues, procurementIssue(reports.SeverityError, "bom."+ref+".procurement", ref, "BOM row has no matching local procurement snapshot"))
			} else {
				issues = append(issues, procurementIssue(reports.SeverityWarning, "bom."+ref+".procurement", ref, "BOM row has no matching local procurement snapshot"))
			}
			continue
		}
		row.ProcurementSourceID = source.SourceID
		lifecycle := source.Lifecycle
		if lifecycle != nil {
			row.Lifecycle = firstNonEmpty(string(lifecycle.Status), row.Lifecycle)
			row.LifecycleSourceDate = lifecycle.SourceDate
			fresh := sourceEvidenceFresh(lifecycle.SourceDate, now, lifecycleMaxAge)
			row.LifecycleFresh = boolPtr(fresh)
			if !fresh {
				issues = append(issues, procurementIssue(lifecycleSeverity(policy), "bom."+ref+".lifecycle", ref, "BOM row lifecycle source snapshot is stale"))
				row.ReadinessNote = appendProcurementReadinessNote(row.ReadinessNote, "stale lifecycle source snapshot")
			}
			if lifecycleBlocksFabrication(lifecycle.Status, policy) {
				issues = append(issues, procurementIssue(reports.SeverityError, "bom."+ref+".lifecycle", ref, fmt.Sprintf("BOM row lifecycle status is %s", lifecycle.Status)))
				row.ReadinessNote = appendProcurementReadinessNote(row.ReadinessNote, "blocked lifecycle status "+string(lifecycle.Status))
			}
		} else if policy.RequireLifecycle {
			row.ProcurementOutcome = "blocked"
			issues = append(issues, procurementIssue(reports.SeverityError, "bom."+ref+".lifecycle", ref, "BOM row has no lifecycle evidence in local procurement snapshot"))
		}
		availability := source.Availability
		if availability != nil {
			row.AvailabilityStatus = string(availability.Status)
			row.AvailabilitySourceDate = availability.SourceDate
			fresh := sourceEvidenceFresh(availability.SourceDate, now, availabilityMaxAge)
			row.AvailabilityFresh = boolPtr(fresh)
			if !fresh {
				issues = append(issues, procurementIssue(availabilitySeverity(policy), "bom."+ref+".availability", ref, "BOM row availability source snapshot is stale"))
				row.ReadinessNote = appendProcurementReadinessNote(row.ReadinessNote, "stale availability source snapshot")
			}
			if availabilityBlocksFabrication(availability.Status, policy) {
				issues = append(issues, procurementIssue(reports.SeverityError, "bom."+ref+".availability", ref, fmt.Sprintf("BOM row availability status is %s", availability.Status)))
				row.ReadinessNote = appendProcurementReadinessNote(row.ReadinessNote, "blocked availability status "+string(availability.Status))
			} else if availabilityWarnsFabrication(availability.Status) {
				issues = append(issues, procurementIssue(reports.SeverityWarning, "bom."+ref+".availability", ref, fmt.Sprintf("BOM row availability status is %s", availability.Status)))
				row.ReadinessNote = appendProcurementReadinessNote(row.ReadinessNote, "review availability status "+string(availability.Status))
			}
		} else if policy.RequireAvailability {
			row.ProcurementOutcome = "blocked"
			issues = append(issues, procurementIssue(reports.SeverityError, "bom."+ref+".availability", ref, "BOM row has no availability evidence in local procurement snapshot"))
		}
		row.ProcurementOutcome = procurementOutcome(row, policy)
	}
	return out, issues
}

func sourceEvidenceFresh(sourceDate string, now time.Time, maxAgeDays int) bool {
	parsed, err := time.Parse("2006-01-02", strings.TrimSpace(sourceDate))
	if err != nil {
		return false
	}
	if parsed.After(now) {
		return true
	}
	return !parsed.AddDate(0, 0, maxAgeDays).Before(now)
}

func boolPtr(value bool) *bool {
	return &value
}

func lifecycleBlocksFabrication(status components.LifecycleStatus, policy components.ProcurementPolicy) bool {
	switch status {
	case components.LifecycleEOL, components.LifecycleObsolete:
		return true
	case components.LifecycleUnknown, components.LifecycleNRND:
		return policy.RequireLifecycle && !policy.AllowUnknownLifecycle
	default:
		return false
	}
}

func availabilityBlocksFabrication(status components.AvailabilityStatus, policy components.ProcurementPolicy) bool {
	if !policy.RequireAvailability {
		return false
	}
	switch status {
	case components.AvailabilityInStock, components.AvailabilityLimited:
		return false
	default:
		return true
	}
}

func availabilityWarnsFabrication(status components.AvailabilityStatus) bool {
	switch status {
	case components.AvailabilityBackorder, components.AvailabilityUnavailable, components.AvailabilityUnknown, components.AvailabilityNotChecked:
		return true
	default:
		return false
	}
}

func lifecycleSeverity(policy components.ProcurementPolicy) reports.Severity {
	if policy.RequireLifecycle {
		return reports.SeverityError
	}
	return reports.SeverityWarning
}

func availabilitySeverity(policy components.ProcurementPolicy) reports.Severity {
	if policy.RequireAvailability {
		return reports.SeverityError
	}
	return reports.SeverityWarning
}

func procurementOutcome(row *BOMRow, policy components.ProcurementPolicy) string {
	if row == nil {
		return ""
	}
	if row.ProcurementSourceID == "" {
		return strings.TrimSpace(row.ProcurementOutcome)
	}
	if row.ProcurementOutcome == "blocked" {
		return "blocked"
	}
	if row.LifecycleFresh != nil && !*row.LifecycleFresh && policy.RequireLifecycle {
		return "blocked"
	}
	if row.AvailabilityFresh != nil && !*row.AvailabilityFresh && policy.RequireAvailability {
		return "blocked"
	}
	if row.AvailabilityStatus != "" && availabilityBlocksFabrication(components.AvailabilityStatus(row.AvailabilityStatus), policy) {
		return "blocked"
	}
	if row.Lifecycle != "" && lifecycleBlocksFabrication(components.LifecycleStatus(row.Lifecycle), policy) {
		return "blocked"
	}
	return "snapshot"
}

func procurementReference(references []string) string {
	for _, ref := range references {
		if strings.TrimSpace(ref) != "" {
			return strings.TrimSpace(ref)
		}
	}
	return "component"
}

func appendProcurementReadinessNote(existing string, note string) string {
	existing = strings.TrimSpace(existing)
	note = strings.TrimSpace(note)
	if note == "" {
		return existing
	}
	if existing == "" {
		return note
	}
	for _, part := range strings.Split(existing, ";") {
		if strings.TrimSpace(part) == note {
			return existing
		}
	}
	return existing + "; " + note
}

func procurementIssue(severity reports.Severity, path string, ref string, message string) reports.Issue {
	return reports.Issue{
		Code:       reports.CodeValidationFailed,
		Severity:   severity,
		Path:       path,
		Message:    message,
		Refs:       compactIssueRefs(ref),
		Suggestion: "review local lifecycle and availability source snapshots before fabrication release",
	}
}

func compactIssueRefs(ref string) []string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil
	}
	return []string{ref}
}
