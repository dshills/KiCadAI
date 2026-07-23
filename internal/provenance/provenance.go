package provenance

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"kicadai/internal/manifest"
	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

const (
	Schema        = "kicadai.transaction.provenance.v1"
	SchemaVersion = "1"
	RelativePath  = ".kicadai/transaction.json"
)

type TransactionProvenance struct {
	Schema             string                      `json:"schema"`
	SchemaVersion      string                      `json:"schema_version"`
	ProjectName        string                      `json:"project_name"`
	GeneratorVersion   string                      `json:"generator_version,omitempty"`
	CreatedBy          string                      `json:"created_by,omitempty"`
	Transaction        transactions.Transaction    `json:"transaction"`
	OperationCount     int                         `json:"operation_count"`
	OperationSummaries []manifest.OperationSummary `json:"operation_summaries,omitempty"`
	Source             Source                      `json:"source,omitempty"`
}

type Source struct {
	Kind string `json:"kind,omitempty"`
	Seed string `json:"seed,omitempty"`
}

func New(projectName string, tx transactions.Transaction, generatorVersion string) TransactionProvenance {
	return TransactionProvenance{
		Schema:             Schema,
		SchemaVersion:      SchemaVersion,
		ProjectName:        strings.TrimSpace(projectName),
		GeneratorVersion:   strings.TrimSpace(generatorVersion),
		CreatedBy:          "kicadai",
		Transaction:        tx.Clone(),
		OperationCount:     len(tx.Operations),
		OperationSummaries: OperationSummaries(tx),
		Source:             Source{Kind: "transaction_apply"},
	}
}

func OperationSummaries(tx transactions.Transaction) []manifest.OperationSummary {
	summaries := make([]manifest.OperationSummary, 0, len(tx.Operations))
	for index, operation := range tx.Operations {
		summaries = append(summaries, manifest.OperationSummary{Index: index, Op: string(operation.Op)})
	}
	return summaries
}

func Validate(provenance TransactionProvenance) []reports.Issue {
	var issues []reports.Issue
	schema := strings.TrimSpace(provenance.Schema)
	projectName := strings.TrimSpace(provenance.ProjectName)
	generatorVersion := strings.TrimSpace(provenance.GeneratorVersion)
	if schema == "" {
		issues = append(issues, issue("provenance.schema", "transaction provenance schema is required"))
	} else if schema != Schema {
		issues = append(issues, issue("provenance.schema", "unsupported transaction provenance schema "+schema))
	}
	// Missing schema_version remains readable for v1 projects written before
	// shared creation evidence. All newly written provenance includes it.
	if strings.TrimSpace(provenance.SchemaVersion) != "" && provenance.SchemaVersion != SchemaVersion {
		issues = append(issues, issue("provenance.schema_version", "unsupported transaction provenance schema_version "+provenance.SchemaVersion))
	}
	if projectName == "" {
		issues = append(issues, issue("provenance.project_name", "transaction provenance project_name is required"))
	}
	if generatorVersion == "" {
		issues = append(issues, issue("provenance.generator_version", "transaction provenance generator_version is required"))
	}
	if txProject := strings.TrimSpace(provenance.Transaction.Project); txProject != "" && projectName != txProject {
		issues = append(issues, issue("provenance.project_name", "project_name does not match transaction project"))
	}
	if txName := strings.TrimSpace(provenance.Transaction.Name); txName != "" && projectName != txName {
		issues = append(issues, issue("provenance.project_name", "project_name does not match transaction name"))
	}
	if provenance.OperationCount != len(provenance.Transaction.Operations) {
		issues = append(issues, issue("provenance.operation_count", fmt.Sprintf("operation_count %d does not match transaction operation count %d", provenance.OperationCount, len(provenance.Transaction.Operations))))
	}
	issues = append(issues, validateOperationSummaries(provenance)...)
	validation := transactions.Validate(provenance.Transaction)
	for _, txIssue := range validation.Issues {
		txIssue.Path = "provenance.transaction." + txIssue.Path
		issues = append(issues, txIssue)
	}
	return issues
}

func validateOperationSummaries(provenance TransactionProvenance) []reports.Issue {
	if len(provenance.OperationSummaries) != len(provenance.Transaction.Operations) {
		return []reports.Issue{issue("provenance.operation_summaries", fmt.Sprintf("operation_summaries count %d does not match transaction operation count %d", len(provenance.OperationSummaries), len(provenance.Transaction.Operations)))}
	}
	if len(provenance.OperationSummaries) == 0 {
		return nil
	}
	var issues []reports.Issue
	for index, summary := range provenance.OperationSummaries {
		if summary.Index != index {
			issues = append(issues, issue(fmt.Sprintf("provenance.operation_summaries[%d].index", index), fmt.Sprintf("operation summary index %d does not match position %d", summary.Index, index)))
		}
		wantOp := string(provenance.Transaction.Operations[index].Op)
		if summary.Op != wantOp {
			issues = append(issues, issue(fmt.Sprintf("provenance.operation_summaries[%d].op", index), fmt.Sprintf("operation summary op %q does not match transaction op %q", summary.Op, wantOp)))
		}
	}
	return issues
}

func Write(root string, provenance TransactionProvenance) (reports.Artifact, error) {
	path, err := AbsPath(root, RelativePath)
	if err != nil {
		return reports.Artifact{}, err
	}
	if issues := Validate(provenance); len(issues) > 0 {
		return reports.Artifact{}, fmt.Errorf("transaction provenance is invalid: %s", issues[0].Message)
	}
	data, err := Marshal(provenance)
	if err != nil {
		return reports.Artifact{}, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return reports.Artifact{}, err
	}
	if err := atomicWriteFile(path, data, 0o644); err != nil {
		return reports.Artifact{}, err
	}
	return reports.Artifact{Kind: reports.ArtifactValidationReport, Path: RelativePath, Description: "KiCadAI generated transaction provenance"}, nil
}

func Read(root string) (TransactionProvenance, []reports.Issue, error) {
	path, err := AbsPath(root, RelativePath)
	if err != nil {
		return TransactionProvenance{}, []reports.Issue{issue("provenance.transaction", err.Error())}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		readIssue := issue("provenance.transaction", err.Error())
		if os.IsNotExist(err) {
			readIssue.Code = reports.CodeMissingFile
		}
		return TransactionProvenance{}, []reports.Issue{readIssue}, err
	}
	var provenance TransactionProvenance
	if err := json.Unmarshal(data, &provenance); err != nil {
		return TransactionProvenance{}, []reports.Issue{issue("provenance.transaction", err.Error())}, err
	}
	issues := Validate(provenance)
	return provenance, issues, nil
}

func Marshal(provenance TransactionProvenance) ([]byte, error) {
	data, err := json.MarshalIndent(provenance, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func AbsPath(root string, rel string) (string, error) {
	if strings.TrimSpace(root) == "" {
		return "", fmt.Errorf("provenance root required")
	}
	if strings.TrimSpace(rel) == "" {
		rel = RelativePath
	}
	if filepath.IsAbs(rel) {
		return "", fmt.Errorf("transaction provenance path must be relative")
	}
	cleanRel := filepath.Clean(filepath.FromSlash(rel))
	if cleanRel == "." {
		return "", fmt.Errorf("transaction provenance path is required")
	}
	for _, part := range strings.Split(filepath.ToSlash(cleanRel), "/") {
		if part == ".." {
			return "", fmt.Errorf("transaction provenance path must not contain parent traversal")
		}
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	path := filepath.Join(absRoot, cleanRel)
	relToRoot, err := filepath.Rel(absRoot, path)
	if err != nil || relToRoot == ".." || strings.HasPrefix(relToRoot, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("transaction provenance path must be inside project root")
	}
	return path, nil
}

func issue(path string, message string) reports.Issue {
	return reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityBlocked, Path: path, Message: message}
}

func atomicWriteFile(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	base := filepath.Base(path)
	if len(base) > 128 {
		base = base[:128]
	}
	file, err := os.CreateTemp(dir, "."+base+".tmp-*")
	if err != nil {
		return err
	}
	tempName := file.Name()
	closed := false
	defer func() {
		if !closed {
			_ = file.Close()
		}
		_ = os.Remove(tempName)
	}()
	if _, err := file.Write(data); err != nil {
		return err
	}
	if err := file.Chmod(perm); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	closed = true
	return os.Rename(tempName, path)
}
