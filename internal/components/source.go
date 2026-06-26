package components

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"kicadai/internal/reports"
)

const SourceSchema = "kicadai.component.source.v1"

const (
	CodeSourceReadFailed      reports.Code = "COMPONENT_SOURCE_READ_FAILED"
	CodeSourceParseFailed     reports.Code = "COMPONENT_SOURCE_PARSE_FAILED"
	CodeSourceInvalidSchema   reports.Code = "COMPONENT_SOURCE_INVALID_SCHEMA"
	CodeSourceInvalidMetadata reports.Code = "COMPONENT_SOURCE_INVALID_METADATA"
	CodeSourceInvalidStatus   reports.Code = "COMPONENT_SOURCE_INVALID_STATUS"
	CodeSourceDuplicateRecord reports.Code = "COMPONENT_SOURCE_DUPLICATE_RECORD"
	CodeSourceInvalidDate     reports.Code = "COMPONENT_SOURCE_INVALID_DATE"
)

type LifecycleStatus string

const (
	LifecycleActive   LifecycleStatus = "active"
	LifecycleMature   LifecycleStatus = "mature"
	LifecycleNRND     LifecycleStatus = "nrnd"
	LifecycleEOL      LifecycleStatus = "eol"
	LifecycleObsolete LifecycleStatus = "obsolete"
	LifecycleUnknown  LifecycleStatus = "unknown"
)

type AvailabilityStatus string

const (
	AvailabilityInStock     AvailabilityStatus = "in_stock"
	AvailabilityLimited     AvailabilityStatus = "limited"
	AvailabilityBackorder   AvailabilityStatus = "backorder"
	AvailabilityUnavailable AvailabilityStatus = "unavailable"
	AvailabilityUnknown     AvailabilityStatus = "unknown"
	AvailabilityNotChecked  AvailabilityStatus = "not_checked"
)

type SourceConfidence string

const (
	SourceConfidenceCurated          SourceConfidence = "curated"
	SourceConfidenceProviderSnapshot SourceConfidence = "provider_snapshot"
	SourceConfidenceManualReview     SourceConfidence = "manual_review"
	SourceConfidenceNotChecked       SourceConfidence = "not_checked"
	SourceConfidenceUnknown          SourceConfidence = "unknown"
)

type SourceCollection struct {
	GeneratedAt *time.Time        `json:"generated_at,omitempty"`
	Records     []SourceRecord    `json:"records"`
	Diagnostics []reports.Issue   `json:"diagnostics,omitempty"`
	index       map[sourceKey]int `json:"-"`
}

type SourceRecord struct {
	Manufacturer string                `json:"manufacturer"`
	MPN          string                `json:"mpn"`
	Lifecycle    *LifecycleEvidence    `json:"lifecycle,omitempty"`
	Availability *AvailabilityEvidence `json:"availability,omitempty"`
	Notes        []string              `json:"notes,omitempty"`
	SourceID     string                `json:"source_id,omitempty"`
	SourceFile   string                `json:"source_file,omitempty"`
}

type LifecycleEvidence struct {
	Status     LifecycleStatus  `json:"status"`
	Source     string           `json:"source"`
	SourceDate string           `json:"source_date"`
	Confidence SourceConfidence `json:"confidence"`
}

type AvailabilityEvidence struct {
	Status     AvailabilityStatus `json:"status"`
	Source     string             `json:"source"`
	SourceDate string             `json:"source_date"`
	Confidence SourceConfidence   `json:"confidence"`
	Quantity   int                `json:"quantity,omitempty"`
	LeadTime   string             `json:"lead_time,omitempty"`
}

type SourceLoadOptions struct {
	SourceDir string `json:"source_dir,omitempty"`
}

type sourceFile struct {
	Schema      string         `json:"schema"`
	SourceID    string         `json:"source_id"`
	GeneratedAt string         `json:"generated_at,omitempty"`
	Records     []SourceRecord `json:"records"`
}

type sourceKey struct {
	Manufacturer string
	MPN          string
}

func LoadSources(ctx context.Context, opts SourceLoadOptions) (*SourceCollection, error) {
	dir := strings.TrimSpace(opts.SourceDir)
	collection := &SourceCollection{Records: []SourceRecord{}, Diagnostics: []reports.Issue{}}
	if dir == "" {
		return collection, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		files = append(files, filepath.Join(dir, entry.Name()))
	}
	sort.Strings(files)
	for _, file := range files {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		partial, issues := readSourceFile(file)
		collection.Diagnostics = append(collection.Diagnostics, issues...)
		if hasFatalSourceFileIssue(issues) {
			continue
		}
		if partial.GeneratedAt != "" && collection.GeneratedAt == nil {
			if parsed, err := parseSourceDate(partial.GeneratedAt); err == nil {
				collection.GeneratedAt = &parsed
			}
		}
		for _, record := range partial.Records {
			if len(validateSourceRecord(file, record)) != 0 {
				continue
			}
			record.SourceID = partial.SourceID
			record.SourceFile = filepath.ToSlash(file)
			collection.Records = append(collection.Records, record)
		}
	}
	collection.Diagnostics = append(collection.Diagnostics, ValidateSources(collection).Issues...)
	sortIssues(collection.Diagnostics)
	collection.rebuildIndex()
	return collection, nil
}

func hasFatalSourceFileIssue(issues []reports.Issue) bool {
	for _, issue := range issues {
		switch issue.Code {
		case CodeSourceReadFailed, CodeSourceParseFailed, CodeSourceInvalidSchema:
			return true
		}
		if strings.HasSuffix(issue.Path, ".source_id") || strings.HasSuffix(issue.Path, ".generated_at") {
			return true
		}
	}
	return false
}

func ValidateSources(collection *SourceCollection) reports.Result {
	if collection == nil {
		return reports.ErrorResult("component source validate", NewIssue(reports.CodeInvalidArgument, reports.SeverityBlocked, "sources", "component source collection is nil"))
	}
	var issues []reports.Issue
	seen := map[sourceKey]int{}
	for i, record := range collection.Records {
		path := fmt.Sprintf("records[%d]", i)
		issues = append(issues, validateSourceRecord(path, record)...)
		key := normalizeSourceKey(record.Manufacturer, record.MPN)
		if key.Manufacturer == "" || key.MPN == "" {
			continue
		}
		if first, ok := seen[key]; ok {
			issues = append(issues, NewIssue(CodeSourceDuplicateRecord, reports.SeverityBlocked, path, fmt.Sprintf("source record duplicates records[%d]", first)))
		} else {
			seen[key] = i
		}
	}
	sortIssues(issues)
	return reports.ResultWithIssues("component source validate", map[string]any{"record_count": len(collection.Records)}, issues, nil)
}

func (collection *SourceCollection) Find(manufacturer string, mpn string) (SourceRecord, bool) {
	if collection == nil {
		return SourceRecord{}, false
	}
	key := normalizeSourceKey(manufacturer, mpn)
	if collection.index != nil {
		index, ok := collection.index[key]
		if !ok || index < 0 || index >= len(collection.Records) {
			return SourceRecord{}, false
		}
		return collection.Records[index], true
	}
	for _, record := range collection.Records {
		if normalizeSourceKey(record.Manufacturer, record.MPN) == key {
			return record, true
		}
	}
	return SourceRecord{}, false
}

func (collection *SourceCollection) rebuildIndex() {
	if collection == nil {
		return
	}
	collection.index = map[sourceKey]int{}
	for i, record := range collection.Records {
		key := normalizeSourceKey(record.Manufacturer, record.MPN)
		if key.Manufacturer == "" || key.MPN == "" {
			continue
		}
		if _, exists := collection.index[key]; !exists {
			collection.index[key] = i
		}
	}
}

func readSourceFile(path string) (sourceFile, []reports.Issue) {
	body, err := os.ReadFile(path)
	if err != nil {
		return sourceFile{}, []reports.Issue{NewIssue(CodeSourceReadFailed, reports.SeverityBlocked, path, err.Error())}
	}
	var file sourceFile
	if err := json.Unmarshal(body, &file); err != nil {
		return sourceFile{}, []reports.Issue{NewIssue(CodeSourceParseFailed, reports.SeverityBlocked, path, err.Error())}
	}
	var issues []reports.Issue
	if file.Schema != SourceSchema {
		issues = append(issues, NewIssue(CodeSourceInvalidSchema, reports.SeverityBlocked, path+".schema", "component source schema must be "+SourceSchema))
	}
	if issue, ok := validateSourceTrimmed(path+".source_id", file.SourceID, "source id"); ok {
		issues = append(issues, issue)
	} else if file.SourceID == "" {
		issues = append(issues, NewIssue(CodeSourceInvalidMetadata, reports.SeverityBlocked, path+".source_id", "source id is required"))
	}
	if file.GeneratedAt != "" {
		if _, err := parseSourceDate(file.GeneratedAt); err != nil {
			issues = append(issues, NewIssue(CodeSourceInvalidDate, reports.SeverityBlocked, path+".generated_at", "generated_at must be YYYY-MM-DD"))
		}
	}
	for i, record := range file.Records {
		issues = append(issues, validateSourceRecord(fmt.Sprintf("%s.records[%d]", path, i), record)...)
	}
	sortIssues(issues)
	return file, issues
}

func validateSourceRecord(path string, record SourceRecord) []reports.Issue {
	var issues []reports.Issue
	if issue, ok := validateSourceTrimmed(path+".manufacturer", record.Manufacturer, "manufacturer"); ok {
		issues = append(issues, issue)
	} else if record.Manufacturer == "" {
		issues = append(issues, NewIssue(CodeSourceInvalidMetadata, reports.SeverityBlocked, path+".manufacturer", "manufacturer is required"))
	}
	if issue, ok := validateSourceTrimmed(path+".mpn", record.MPN, "mpn"); ok {
		issues = append(issues, issue)
	} else if record.MPN == "" {
		issues = append(issues, NewIssue(CodeSourceInvalidMetadata, reports.SeverityBlocked, path+".mpn", "mpn is required"))
	}
	if record.Lifecycle != nil {
		issues = append(issues, validateLifecycleEvidence(path+".lifecycle", *record.Lifecycle)...)
	}
	if record.Availability != nil {
		issues = append(issues, validateAvailabilityEvidence(path+".availability", *record.Availability)...)
	}
	return issues
}

func validateLifecycleEvidence(path string, evidence LifecycleEvidence) []reports.Issue {
	var issues []reports.Issue
	if !validLifecycleStatus(evidence.Status) {
		issues = append(issues, NewIssue(CodeSourceInvalidStatus, reports.SeverityBlocked, path+".status", "invalid lifecycle status: "+string(evidence.Status)))
	}
	issues = append(issues, validateSourceCommon(path, evidence.Source, evidence.SourceDate, evidence.Confidence)...)
	return issues
}

func validateAvailabilityEvidence(path string, evidence AvailabilityEvidence) []reports.Issue {
	var issues []reports.Issue
	if !validAvailabilityStatus(evidence.Status) {
		issues = append(issues, NewIssue(CodeSourceInvalidStatus, reports.SeverityBlocked, path+".status", "invalid availability status: "+string(evidence.Status)))
	}
	if evidence.Quantity < 0 {
		issues = append(issues, NewIssue(CodeSourceInvalidMetadata, reports.SeverityBlocked, path+".quantity", "availability quantity must not be negative"))
	}
	issues = append(issues, validateSourceCommon(path, evidence.Source, evidence.SourceDate, evidence.Confidence)...)
	return issues
}

func validateSourceCommon(path string, source string, sourceDate string, confidence SourceConfidence) []reports.Issue {
	var issues []reports.Issue
	if issue, ok := validateSourceTrimmed(path+".source", source, "source"); ok {
		issues = append(issues, issue)
	} else if source == "" {
		issues = append(issues, NewIssue(CodeSourceInvalidMetadata, reports.SeverityBlocked, path+".source", "source is required"))
	}
	if _, err := parseSourceDate(sourceDate); err != nil {
		issues = append(issues, NewIssue(CodeSourceInvalidDate, reports.SeverityBlocked, path+".source_date", "source_date must be YYYY-MM-DD"))
	}
	if !validSourceConfidence(confidence) {
		issues = append(issues, NewIssue(CodeSourceInvalidStatus, reports.SeverityBlocked, path+".confidence", "invalid source confidence: "+string(confidence)))
	}
	return issues
}

func validateSourceTrimmed(path string, value string, label string) (reports.Issue, bool) {
	if strings.TrimSpace(value) != value {
		return NewIssue(CodeSourceInvalidMetadata, reports.SeverityBlocked, path, label+" must not have leading or trailing whitespace"), true
	}
	return reports.Issue{}, false
}

func validLifecycleStatus(status LifecycleStatus) bool {
	switch status {
	case LifecycleActive, LifecycleMature, LifecycleNRND, LifecycleEOL, LifecycleObsolete, LifecycleUnknown:
		return true
	default:
		return false
	}
}

func validAvailabilityStatus(status AvailabilityStatus) bool {
	switch status {
	case AvailabilityInStock, AvailabilityLimited, AvailabilityBackorder, AvailabilityUnavailable, AvailabilityUnknown, AvailabilityNotChecked:
		return true
	default:
		return false
	}
}

func validSourceConfidence(confidence SourceConfidence) bool {
	switch confidence {
	case SourceConfidenceCurated, SourceConfidenceProviderSnapshot, SourceConfidenceManualReview, SourceConfidenceNotChecked, SourceConfidenceUnknown:
		return true
	default:
		return false
	}
}

func parseSourceDate(value string) (time.Time, error) {
	if strings.TrimSpace(value) != value || value == "" {
		return time.Time{}, fmt.Errorf("invalid source date")
	}
	return time.Parse("2006-01-02", value)
}

func normalizeSourceKey(manufacturer string, mpn string) sourceKey {
	return sourceKey{Manufacturer: normalizeSourcePart(manufacturer), MPN: normalizeSourcePart(mpn)}
}

func normalizeSourcePart(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(value)), " "))
}
