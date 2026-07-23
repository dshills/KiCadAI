// Package creationevidence writes the lane-neutral evidence contract shared by
// provider, intent, and circuit creation commands.
package creationevidence

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"kicadai/internal/atomicfile"
	"kicadai/internal/designworkflow"
	"kicadai/internal/manifest"
	"kicadai/internal/provenance"
	"kicadai/internal/reports"
)

const (
	DesignRequestPath       = ".kicadai/design-request.json"
	WorkflowResultPath      = ".kicadai/workflow-result.json"
	ValidationSummaryPath   = ".kicadai/validation-summary.json"
	DesignPromotionPath     = ".kicadai/design-promotion.json"
	DesignRequestSchema     = "kicadai.design-request-artifact.v1"
	WorkflowResultSchema    = "kicadai.workflow-result.v1"
	ValidationSummarySchema = "kicadai.validation-summary.v1"
	DesignPromotionSchema   = "kicadai.design-promotion.v1"
	maxEvidenceBackupBytes  = 16 << 20
)

type Applicability struct {
	Status    string `json:"status"`
	Rationale string `json:"rationale,omitempty"`
}

type Gate struct {
	Name      string `json:"name"`
	Status    string `json:"status"`
	Rationale string `json:"rationale,omitempty"`
}

// ValidationSummary is the shared automation view. Lane-specific status and
// repair fields remain available without changing the core paths.
type ValidationSummary struct {
	SchemaVersion             string       `json:"schema_version"`
	Status                    string       `json:"status"`
	Stage                     string       `json:"stage"`
	IssueCode                 reports.Code `json:"issue_code,omitempty"`
	Message                   string       `json:"message"`
	Detail                    string       `json:"detail,omitempty"`
	ArtifactPaths             []string     `json:"artifact_paths,omitempty"`
	RepairCategory            string       `json:"repair_category,omitempty"`
	RepairBundlePath          string       `json:"repair_bundle_path,omitempty"`
	SuggestedNextAction       string       `json:"suggested_next_action,omitempty"`
	RetryAllowed              bool         `json:"retry_allowed"`
	RetryKey                  string       `json:"retry_key,omitempty"`
	MaxAutomaticRetryAttempts int          `json:"max_automatic_retry_attempts"`
	UserClarificationRequired bool         `json:"user_clarification_required"`
	Gates                     []Gate       `json:"gates"`
}

type DesignRequestDocument struct {
	SchemaVersion string `json:"schema_version"`
	designworkflow.Request
}

type WorkflowResultDocument struct {
	SchemaVersion string `json:"schema_version"`
	designworkflow.WorkflowResult
}

type DesignPromotionDocument struct {
	SchemaVersion string        `json:"schema_version"`
	Applicability Applicability `json:"applicability"`
	designworkflow.PromotionReport
}

type Bundle struct {
	Lane                   string
	Request                designworkflow.Request
	Workflow               designworkflow.WorkflowResult
	Validation             ValidationSummary
	Promotion              *designworkflow.PromotionReport
	PromotionApplicability *Applicability
	Artifacts              []reports.Artifact
	AILane                 *manifest.AILaneSummary
}

// GatesFromWorkflow returns the stable lane-neutral gate projection for a
// workflow, including promotion when it has been evaluated.
func GatesFromWorkflow(workflow designworkflow.WorkflowResult) []Gate {
	gates := make([]Gate, 0, len(workflow.Stages)+1)
	for _, stage := range workflow.Stages {
		rationale := "workflow stage completed with status " + string(stage.Status)
		if stage.Status == designworkflow.StageStatusSkipped {
			rationale = "stage was explicitly not applicable to this workflow"
		}
		gates = append(gates, Gate{Name: string(stage.Name), Status: string(stage.Status), Rationale: rationale})
	}
	if workflow.Promotion != nil {
		gates = append(gates, Gate{Name: "promotion", Status: string(workflow.Promotion.Status), Rationale: "promotion gates were evaluated from this workflow result"})
	}
	return normalizeGates(gates)
}

func Write(root string, bundle Bundle) ([]reports.Artifact, []reports.Issue) {
	if strings.TrimSpace(root) == "" {
		return nil, []reports.Issue{artifactIssue(manifest.RelativePath, "creation evidence root is required")}
	}
	rootInfo, err := os.Stat(root)
	if err != nil {
		return nil, []reports.Issue{artifactIssue(manifest.RelativePath, "inspect creation evidence root: "+err.Error())}
	}
	if !rootInfo.IsDir() {
		return nil, []reports.Issue{artifactIssue(manifest.RelativePath, "creation evidence root is not a directory")}
	}
	// Creation lanes call Write only after the synchronous design workflow has
	// completed its project-write stage. Decode and hash the persisted bytes in
	// one streaming pass before changing any evidence file.
	transactionHash, err := validatedProvenanceHash(filepath.Join(root, filepath.FromSlash(provenance.RelativePath)))
	if err != nil {
		return nil, []reports.Issue{artifactIssue(provenance.RelativePath, "read transaction provenance: "+err.Error())}
	}

	bundle.Validation.SchemaVersion = ValidationSummarySchema
	bundle.Validation.Gates = normalizeGates(bundle.Validation.Gates)
	request := DesignRequestDocument{SchemaVersion: DesignRequestSchema, Request: designworkflow.NormalizeRequest(bundle.Request)}
	workflow := WorkflowResultDocument{SchemaVersion: WorkflowResultSchema, WorkflowResult: normalizeWorkflow(root, bundle.Workflow)}
	promotion := DesignPromotionDocument{
		SchemaVersion: DesignPromotionSchema,
		Applicability: Applicability{Status: "inapplicable", Rationale: "no promotion evaluation was requested for this creation result"},
	}
	if bundle.PromotionApplicability != nil {
		promotion.Applicability = *bundle.PromotionApplicability
	}
	if bundle.Promotion != nil {
		promotion.Applicability = Applicability{Status: "applicable", Rationale: "promotion gates were evaluated from this workflow result"}
		promotion.PromotionReport = normalizePromotion(root, *bundle.Promotion)
	}

	documents := []document{
		{path: DesignRequestPath, kind: reports.ArtifactPreview, schema: DesignRequestSchema, stage: string(designworkflow.StageParseRequest), description: "normalized creation design request", value: request},
		{path: WorkflowResultPath, kind: reports.ArtifactValidationReport, schema: WorkflowResultSchema, stage: string(designworkflow.StageFeedback), description: "creation workflow result", value: workflow},
		{path: ValidationSummaryPath, kind: reports.ArtifactValidationReport, schema: ValidationSummarySchema, stage: string(designworkflow.StageValidation), description: "creation validation summary", value: bundle.Validation},
		{path: DesignPromotionPath, kind: reports.ArtifactPromotionReport, schema: DesignPromotionSchema, stage: string(designworkflow.StageFeedback), description: "creation promotion evidence", value: promotion},
	}
	for index := range documents {
		data, err := json.MarshalIndent(documents[index].value, "", "  ")
		if err != nil {
			return nil, []reports.Issue{artifactIssue(documents[index].path, err.Error())}
		}
		documents[index].data = append(data, '\n')
	}
	backups, backupPath, err := captureBackups(root, documents)
	if err != nil {
		if strings.TrimSpace(backupPath) == "" {
			backupPath = ".kicadai"
		}
		return nil, []reports.Issue{artifactIssue(filepath.ToSlash(backupPath), err.Error())}
	}
	for _, document := range documents {
		if err := atomicWrite(filepath.Join(root, filepath.FromSlash(document.path)), document.data); err != nil {
			return nil, []reports.Issue{rollbackIssue(document.path, err, backups)}
		}
	}

	artifacts := append([]reports.Artifact(nil), bundle.Artifacts...)
	artifacts = append(artifacts, reports.Artifact{Kind: reports.ArtifactValidationReport, Path: provenance.RelativePath, Description: "KiCadAI generated transaction provenance"})
	evidence := make([]manifest.EvidenceArtifact, 0, len(documents)+1)
	for _, document := range documents {
		artifacts = append(artifacts, reports.Artifact{Kind: document.kind, Path: document.path, Description: document.description})
		evidence = append(evidence, manifest.EvidenceArtifact{Kind: document.kind, Path: document.path, SHA256: hashBytes(document.data), SchemaVersion: document.schema, GenerationStage: document.stage})
	}
	evidence = append(evidence, manifest.EvidenceArtifact{Kind: reports.ArtifactValidationReport, Path: provenance.RelativePath, SHA256: transactionHash, SchemaVersion: provenance.Schema, GenerationStage: string(designworkflow.StageProjectWrite)})
	sort.Slice(evidence, func(i, j int) bool { return evidence[i].Path < evidence[j].Path })

	current, manifestStatus, err := manifest.Read(root)
	if err != nil {
		return nil, []reports.Issue{rollbackIssue(manifest.RelativePath, fmt.Errorf("read existing manifest: %w", err), backups)}
	}
	if !manifestStatus.Present {
		current = manifest.Manifest{}
	}
	current.SchemaVersion = manifest.SchemaVersion
	current.CreationLane = strings.TrimSpace(bundle.Lane)
	current.ProjectName = designworkflow.NormalizeProjectName(bundle.Request.Name)
	current.GeneratorVersion = reports.Version
	projectArtifacts, externalArtifacts, err := mergeArtifacts(root, current.Artifacts, artifacts)
	if err != nil {
		return nil, []reports.Issue{rollbackIssue(manifest.RelativePath, err, backups)}
	}
	current.Artifacts = projectArtifacts
	current.ExternalEvidence = externalArtifacts
	current.Evidence = evidence
	current.AILane = bundle.AILane
	manifestArtifact, err := manifest.Write(root, current)
	if err != nil {
		return nil, []reports.Issue{rollbackIssue(manifest.RelativePath, err, backups)}
	}
	manifestArtifact.Path = manifest.RelativePath
	return append(artifacts, manifestArtifact), nil
}

func normalizeWorkflow(root string, workflow designworkflow.WorkflowResult) designworkflow.WorkflowResult {
	normalized := workflow
	normalized.Project.OutputDir = "."
	normalized.Stages = append([]designworkflow.StageResult(nil), workflow.Stages...)
	for index := range normalized.Stages {
		normalized.Stages[index].Artifacts = normalizeArtifactPaths(root, string(normalized.Stages[index].Name), workflow.Stages[index].Artifacts)
	}
	return normalized
}

func normalizePromotion(root string, report designworkflow.PromotionReport) designworkflow.PromotionReport {
	normalized := designworkflow.NormalizePromotionReport(report)
	normalized.Request = normalizeOptionalPath(root, normalized.Request, "promotion.request")
	normalized.ExternalEvidence = normalizeOptionalPath(root, normalized.ExternalEvidence, "promotion.external_evidence")
	for index := range normalized.Gates {
		for artifactIndex := range normalized.Gates[index].Artifacts {
			identity := fmt.Sprintf("promotion.gates.%s.artifacts.%d", normalized.Gates[index].ID, artifactIndex)
			normalized.Gates[index].Artifacts[artifactIndex] = normalizeOptionalPath(root, normalized.Gates[index].Artifacts[artifactIndex], identity)
		}
	}
	for index := range normalized.Issues {
		normalized.Issues[index].Artifact = normalizeOptionalPath(root, normalized.Issues[index].Artifact, fmt.Sprintf("promotion.issues.%d.artifact", index))
	}
	for index := range normalized.NextActions {
		for artifactIndex := range normalized.NextActions[index].Artifacts {
			identity := fmt.Sprintf("promotion.next_actions.%d.artifacts.%d", index, artifactIndex)
			normalized.NextActions[index].Artifacts[artifactIndex] = normalizeOptionalPath(root, normalized.NextActions[index].Artifacts[artifactIndex], identity)
		}
	}
	for index := range normalized.Artifacts {
		normalized.Artifacts[index].Path = normalizeOptionalPath(root, normalized.Artifacts[index].Path, fmt.Sprintf("promotion.artifacts.%d", index))
	}
	return normalized
}

func normalizeArtifactPaths(root string, stage string, artifacts []reports.Artifact) []reports.Artifact {
	result := append([]reports.Artifact(nil), artifacts...)
	for index := range result {
		result[index].Path = normalizeOptionalPath(root, result[index].Path, fmt.Sprintf("workflow.%s.artifacts.%d", stage, index))
	}
	return result
}

func normalizeOptionalPath(root string, value string, unavailableIdentity string) string {
	value = strings.TrimSpace(value)
	if value == "" || !filepath.IsAbs(value) {
		return filepath.ToSlash(value)
	}
	relative, err := filepath.Rel(root, value)
	if err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return filepath.ToSlash(relative)
	}
	identity := "unavailable"
	if hash, err := hashFile(value); err == nil {
		identity = hash[:12]
	} else {
		identity += "-" + hashBytes([]byte(unavailableIdentity))[:12]
	}
	return "external-evidence://" + identity + "/" + url.PathEscape(filepath.Base(value))
}

type document struct {
	path        string
	kind        reports.ArtifactKind
	schema      string
	stage       string
	description string
	value       any
	data        []byte
}

type fileBackup struct {
	path    string
	data    []byte
	mode    os.FileMode
	present bool
}

func captureBackups(root string, documents []document) ([]fileBackup, string, error) {
	backups := make([]fileBackup, 0, len(documents))
	for _, document := range documents {
		path := filepath.Join(root, filepath.FromSlash(document.path))
		info, err := os.Stat(path)
		if os.IsNotExist(err) {
			backups = append(backups, fileBackup{path: path})
			continue
		}
		if err != nil {
			return nil, document.path, err
		}
		if info.Size() > maxEvidenceBackupBytes {
			return nil, document.path, fmt.Errorf("evidence backup exceeds %d bytes", maxEvidenceBackupBytes)
		}
		data, err := readBoundedBackup(path)
		if err != nil {
			return nil, document.path, err
		}
		if len(data) > maxEvidenceBackupBytes {
			return nil, document.path, fmt.Errorf("evidence backup exceeds %d bytes", maxEvidenceBackupBytes)
		}
		backups = append(backups, fileBackup{path: path, data: data, mode: info.Mode().Perm(), present: true})
	}
	return backups, "", nil
}

func readBoundedBackup(path string) (data []byte, err error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := file.Close(); err == nil && closeErr != nil {
			err = closeErr
		}
	}()
	return io.ReadAll(io.LimitReader(file, maxEvidenceBackupBytes+1))
}

func restoreBackups(backups []fileBackup) error {
	var restoreErrors []error
	for _, backup := range backups {
		if !backup.present {
			if err := os.Remove(backup.path); err != nil && !os.IsNotExist(err) {
				restoreErrors = append(restoreErrors, fmt.Errorf("remove partial %s: %w", backup.path, err))
			}
			continue
		}
		if err := atomicfile.Write(backup.path, backup.data, backup.mode); err != nil {
			restoreErrors = append(restoreErrors, fmt.Errorf("restore %s: %w", backup.path, err))
		}
	}
	return errors.Join(restoreErrors...)
}

func normalizeGates(gates []Gate) []Gate {
	if len(gates) == 0 {
		return []Gate{}
	}
	result := make([]Gate, len(gates))
	copy(result, gates)
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result
}

func mergeArtifacts(root string, groups ...[]reports.Artifact) ([]reports.Artifact, []manifest.ExternalArtifact, error) {
	byPath := map[string]reports.Artifact{}
	externalByPath := map[string]reports.Artifact{}
	externalByURI := map[string]manifest.ExternalArtifact{}
	for _, group := range groups {
		for _, artifact := range group {
			path := strings.TrimSpace(artifact.Path)
			if path == "" {
				continue
			}
			if filepath.IsAbs(path) {
				if relative, err := filepath.Rel(root, path); err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
					path = relative
				} else {
					externalByPath[path] = artifact
					continue
				}
			}
			artifact.Path = filepath.ToSlash(filepath.Clean(path))
			byPath[artifact.Path] = artifact
		}
	}
	for path, artifact := range externalByPath {
		hash, err := hashFile(path)
		if err != nil {
			return nil, nil, fmt.Errorf("external artifact %s is unavailable: %w", path, err)
		}
		if len(hash) < 12 {
			return nil, nil, fmt.Errorf("external artifact %s produced an invalid SHA-256", path)
		}
		uri := "external-evidence://" + hash[:12] + "/" + url.PathEscape(filepath.Base(path))
		externalByURI[uri] = manifest.ExternalArtifact{Kind: artifact.Kind, URI: uri, SHA256: hash, GenerationStage: "external_evidence"}
	}
	paths := make([]string, 0, len(byPath))
	for path := range byPath {
		if path != manifest.RelativePath {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	result := make([]reports.Artifact, 0, len(paths))
	for _, path := range paths {
		result = append(result, byPath[path])
	}
	externalURIs := make([]string, 0, len(externalByURI))
	for uri := range externalByURI {
		externalURIs = append(externalURIs, uri)
	}
	sort.Strings(externalURIs)
	external := make([]manifest.ExternalArtifact, 0, len(externalURIs))
	for _, uri := range externalURIs {
		external = append(external, externalByURI[uri])
	}
	return result, external, nil
}

func atomicWrite(path string, data []byte) error {
	return atomicfile.Write(path, data, 0o644)
}

func hashFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func validatedProvenanceHash(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	decoder := json.NewDecoder(io.TeeReader(file, hash))
	var transaction provenance.TransactionProvenance
	if err := decoder.Decode(&transaction); err != nil {
		return "", err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return "", fmt.Errorf("transaction provenance contains multiple JSON values")
		}
		return "", err
	}
	if issues := provenance.Validate(transaction); len(issues) > 0 {
		return "", fmt.Errorf("%s", issues[0].Message)
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func hashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func artifactIssue(path string, message string) reports.Issue {
	return reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Path: path, Stage: "reporting", Message: fmt.Sprintf("write shared creation evidence: %s", message)}
}

func rollbackIssue(path string, cause error, backups []fileBackup) reports.Issue {
	if rollbackErr := restoreBackups(backups); rollbackErr != nil {
		cause = fmt.Errorf("%w; evidence rollback also failed: %v", cause, rollbackErr)
	}
	return artifactIssue(path, cause.Error())
}
