package repair

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

const BundleSchemaV1 = "kicadai.repair.bundle.v1"

type Bundle struct {
	Schema        string                    `json:"schema"`
	ProjectRoot   string                    `json:"project_root,omitempty"`
	ProjectName   string                    `json:"project_name,omitempty"`
	Generated     bool                      `json:"generated,omitempty"`
	Request       json.RawMessage           `json:"request,omitempty"`
	Transaction   *transactions.Transaction `json:"transaction,omitempty"`
	StageIssues   []StageIssues             `json:"stage_issues,omitempty"`
	RepairOptions Options                   `json:"repair_options,omitempty"`
}

func LoadBundle(path string) (Bundle, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Bundle{}, err
	}
	return ParseBundle(data)
}

func ParseBundle(data []byte) (Bundle, error) {
	var bundle Bundle
	if err := json.Unmarshal(data, &bundle); err != nil {
		return Bundle{}, err
	}
	if err := ValidateBundle(bundle); err != nil {
		return Bundle{}, err
	}
	normalizeBundle(&bundle)
	return bundle, nil
}

func SaveBundle(path string, bundle Bundle) error {
	if bundle.Schema == "" {
		bundle.Schema = BundleSchemaV1
	}
	normalizeBundle(&bundle)
	if err := ValidateBundle(bundle); err != nil {
		return err
	}
	data, err := json.MarshalIndent(bundle, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return atomicWriteFile(path, data, 0o644)
}

func ValidateBundle(bundle Bundle) error {
	if strings.TrimSpace(bundle.Schema) == "" {
		return fmt.Errorf("repair bundle schema is required")
	}
	if bundle.Schema != BundleSchemaV1 {
		return fmt.Errorf("unsupported repair bundle schema %q", bundle.Schema)
	}
	if bundle.Transaction != nil {
		if result := transactions.Validate(*bundle.Transaction); len(result.Issues) > 0 {
			return fmt.Errorf("repair bundle transaction is invalid: %s", result.Issues[0].Message)
		}
	}
	if err := validateBundleProjectRoot(bundle.ProjectRoot); err != nil {
		return err
	}
	return nil
}

func BundleIssues(bundle Bundle) []reports.Issue {
	var issues []reports.Issue
	for _, group := range bundle.StageIssues {
		issues = append(issues, group.Issues...)
	}
	return issues
}

func normalizeBundle(bundle *Bundle) {
	bundle.ProjectRoot = filepath.ToSlash(strings.TrimSpace(bundle.ProjectRoot))
	bundle.ProjectName = strings.TrimSpace(bundle.ProjectName)
	for index := range bundle.StageIssues {
		bundle.StageIssues[index].Stage = strings.TrimSpace(bundle.StageIssues[index].Stage)
	}
}

func validateBundleProjectRoot(root string) error {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil
	}
	clean := filepath.Clean(root)
	if clean == "." {
		return nil
	}
	for _, part := range strings.Split(filepath.ToSlash(clean), "/") {
		if part == ".." {
			return fmt.Errorf("repair bundle project_root must not contain parent traversal")
		}
	}
	return nil
}

func atomicWriteFile(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	file, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
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
