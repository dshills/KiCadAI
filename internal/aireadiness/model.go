package aireadiness

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type Category string

const (
	CategoryComponent     Category = "component"
	CategoryBlock         Category = "block"
	CategoryLayout        Category = "layout"
	CategoryValidation    Category = "validation"
	CategoryDocumentation Category = "documentation"
)

type Readiness string

const (
	ReadinessMissing      Readiness = "missing"
	ReadinessDraft        Readiness = "draft"
	ReadinessConnectivity Readiness = "connectivity"
	ReadinessCandidate    Readiness = "candidate"
	ReadinessVerified     Readiness = "verified"
)

type TaskType string

const (
	TaskAddComponent         TaskType = "add_component"
	TaskAddBlock             TaskType = "add_block"
	TaskVerifyPinmap         TaskType = "verify_pinmap"
	TaskVerifyLayout         TaskType = "verify_layout"
	TaskCaptureKiCadEvidence TaskType = "capture_kicad_evidence"
	TaskWriteDocs            TaskType = "write_docs"
)

type ParallelGroup string

const (
	ParallelGroupUnassigned            ParallelGroup = "unassigned"
	ParallelGroupFixturePromotion      ParallelGroup = "fixture_promotion"
	ParallelGroupCatalogBlockExpansion ParallelGroup = "catalog_block_expansion"
	ParallelGroupEngineHardening       ParallelGroup = "engine_hardening"
	ParallelGroupIntentAIUX            ParallelGroup = "intent_ai_ux"
	ParallelGroupDocumentation         ParallelGroup = "documentation"
)

type MatrixFile struct {
	Version string   `json:"version"`
	Records []Record `json:"records"`
}

type Record struct {
	ID             string        `json:"id"`
	Category       Category      `json:"category"`
	Domain         string        `json:"domain"`
	Title          string        `json:"title"`
	Readiness      Readiness     `json:"readiness"`
	Blocker        string        `json:"blocker"`
	EvidenceNeeded []string      `json:"evidence_needed"`
	NextTask       TaskType      `json:"next_task"`
	Evidence       []Evidence    `json:"evidence,omitempty"`
	ParallelGroup  ParallelGroup `json:"parallel_group,omitempty"`
	DependsOn      []string      `json:"depends_on,omitempty"`
}

type Evidence struct {
	Kind         string `json:"kind"`
	Path         string `json:"path,omitempty"`
	SemanticHash string `json:"semantic_hash,omitempty"`
	GitObjectID  string `json:"git_object_id,omitempty"`
	Description  string `json:"description,omitempty"`
}

type RequirementFile struct {
	Version            string   `json:"version"`
	Domain             string   `json:"domain"`
	RequiredCategories []string `json:"required_categories"`
	RequiredRecordIDs  []string `json:"required_record_ids,omitempty"`
}

type Matrix struct {
	Records []Record
}

var recordIDPattern = regexp.MustCompile(`^[a-z0-9]+\.[a-z0-9]+\.[a-z0-9_]+$`)

func LoadDir(root string) (Matrix, error) {
	var matrix Matrix
	matrixDir := filepath.Join(root, "matrix")
	err := filepath.WalkDir(matrixDir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || filepath.Ext(path) != ".json" {
			return nil
		}
		fileRecords, err := loadMatrixFile(path)
		if err != nil {
			return err
		}
		matrix.Records = append(matrix.Records, fileRecords...)
		return nil
	})
	if err != nil {
		return Matrix{}, err
	}
	sort.Slice(matrix.Records, func(i, j int) bool {
		return matrix.Records[i].ID < matrix.Records[j].ID
	})
	if err := Validate(matrix); err != nil {
		return Matrix{}, err
	}
	return matrix, nil
}

func LoadRequirement(path string) (RequirementFile, error) {
	var requirement RequirementFile
	data, err := os.ReadFile(path)
	if err != nil {
		return requirement, err
	}
	if err := json.Unmarshal(data, &requirement); err != nil {
		return RequirementFile{}, fmt.Errorf("%s: %w", path, err)
	}
	if strings.TrimSpace(requirement.Version) == "" {
		return RequirementFile{}, fmt.Errorf("%s: requirement version is required", path)
	}
	if strings.TrimSpace(requirement.Domain) == "" {
		return RequirementFile{}, fmt.Errorf("%s: requirement domain is required", path)
	}
	if len(requirement.RequiredCategories) == 0 && len(requirement.RequiredRecordIDs) == 0 {
		return RequirementFile{}, fmt.Errorf("%s: requirement must declare categories or record IDs", path)
	}
	return requirement, nil
}

func Validate(matrix Matrix) error {
	recordsByID := make(map[string]Record, len(matrix.Records))
	for _, record := range matrix.Records {
		if !recordIDPattern.MatchString(record.ID) {
			return fmt.Errorf("record %q must use <domain>.<category>.<slug> id format", record.ID)
		}
		idParts := strings.Split(record.ID, ".")
		if idParts[0] != record.Domain {
			return fmt.Errorf("record %s domain must match id domain", record.ID)
		}
		if idParts[1] != string(record.Category) {
			return fmt.Errorf("record %s category must match id category", record.ID)
		}
		if _, exists := recordsByID[record.ID]; exists {
			return fmt.Errorf("duplicate record id %q", record.ID)
		}
		recordsByID[record.ID] = record
		if !validCategory(record.Category) {
			return fmt.Errorf("record %s has unsupported category %q", record.ID, record.Category)
		}
		if strings.TrimSpace(record.Domain) == "" {
			return fmt.Errorf("record %s domain is required", record.ID)
		}
		if strings.TrimSpace(record.Title) == "" {
			return fmt.Errorf("record %s title is required", record.ID)
		}
		if !validReadiness(record.Readiness) {
			return fmt.Errorf("record %s has unsupported readiness %q", record.ID, record.Readiness)
		}
		if record.Readiness != ReadinessVerified && strings.TrimSpace(record.Blocker) == "" {
			return fmt.Errorf("record %s blocker is required until verified", record.ID)
		}
		if record.Readiness != ReadinessVerified && !hasNonEmptyString(record.EvidenceNeeded) {
			return fmt.Errorf("record %s evidence_needed is required until verified", record.ID)
		}
		if record.Readiness == ReadinessVerified && len(record.Evidence) == 0 {
			return fmt.Errorf("record %s evidence is required when verified", record.ID)
		}
		if !validTaskType(record.NextTask) {
			return fmt.Errorf("record %s has unsupported next_task %q", record.ID, record.NextTask)
		}
		if record.ParallelGroup != "" && !validParallelGroup(record.ParallelGroup) {
			return fmt.Errorf("record %s has unsupported parallel_group %q", record.ID, record.ParallelGroup)
		}
		if !sort.StringsAreSorted(record.DependsOn) {
			return fmt.Errorf("record %s depends_on must be sorted", record.ID)
		}
		for i, dependencyID := range record.DependsOn {
			if strings.TrimSpace(dependencyID) == "" {
				return fmt.Errorf("record %s depends_on contains an empty record id", record.ID)
			}
			if dependencyID == record.ID {
				return fmt.Errorf("record %s must not depend on itself", record.ID)
			}
			if i > 0 && dependencyID == record.DependsOn[i-1] {
				return fmt.Errorf("record %s depends_on contains duplicate record id %s", record.ID, dependencyID)
			}
		}
		for _, evidence := range record.Evidence {
			if strings.TrimSpace(evidence.Kind) == "" {
				return fmt.Errorf("record %s evidence kind is required", record.ID)
			}
			if evidence.Path != "" && evidence.SemanticHash == "" && evidence.GitObjectID == "" {
				return fmt.Errorf("record %s evidence %s must include semantic_hash or git_object_id", record.ID, evidence.Path)
			}
		}
	}
	for _, record := range matrix.Records {
		for _, dependencyID := range record.DependsOn {
			dependency, exists := recordsByID[dependencyID]
			if !exists {
				return fmt.Errorf("record %s depends on unknown record %s", record.ID, dependencyID)
			}
			if record.Readiness == ReadinessVerified && dependency.Readiness != ReadinessVerified {
				return fmt.Errorf("record %s is verified but dependency %s is %s", record.ID, dependencyID, dependency.Readiness)
			}
		}
	}
	if err := validateDependencyDAG(matrix, recordsByID); err != nil {
		return err
	}
	return nil
}

func ValidateRequirements(matrix Matrix, requirement RequirementFile) error {
	byCategory := map[string]int{}
	byID := map[string]struct{}{}
	for _, record := range matrix.Records {
		if record.Domain != requirement.Domain {
			continue
		}
		byCategory[string(record.Category)]++
		byID[record.ID] = struct{}{}
	}
	for _, category := range requirement.RequiredCategories {
		if byCategory[category] == 0 {
			return fmt.Errorf("domain %s missing required category %s", requirement.Domain, category)
		}
	}
	for _, id := range requirement.RequiredRecordIDs {
		if _, ok := byID[id]; !ok {
			return fmt.Errorf("domain %s missing required record %s", requirement.Domain, id)
		}
	}
	return nil
}

func loadMatrixFile(path string) ([]Record, error) {
	var file MatrixFile
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if strings.TrimSpace(file.Version) == "" {
		return nil, fmt.Errorf("%s: version is required", path)
	}
	return file.Records, nil
}

func validCategory(category Category) bool {
	switch category {
	case CategoryComponent, CategoryBlock, CategoryLayout, CategoryValidation, CategoryDocumentation:
		return true
	default:
		return false
	}
}

func validReadiness(readiness Readiness) bool {
	switch readiness {
	case ReadinessMissing, ReadinessDraft, ReadinessConnectivity, ReadinessCandidate, ReadinessVerified:
		return true
	default:
		return false
	}
}

func validTaskType(task TaskType) bool {
	switch task {
	case TaskAddComponent, TaskAddBlock, TaskVerifyPinmap, TaskVerifyLayout, TaskCaptureKiCadEvidence, TaskWriteDocs:
		return true
	default:
		return false
	}
}

func validParallelGroup(group ParallelGroup) bool {
	switch group {
	case ParallelGroupUnassigned, ParallelGroupFixturePromotion, ParallelGroupCatalogBlockExpansion, ParallelGroupEngineHardening, ParallelGroupIntentAIUX, ParallelGroupDocumentation:
		return true
	default:
		return false
	}
}

func validateDependencyDAG(matrix Matrix, recordsByID map[string]Record) error {
	const (
		unvisited = 0
		visiting  = 1
		visited   = 2
	)
	state := make(map[string]int, len(recordsByID))
	var visit func(string) error
	visit = func(recordID string) error {
		switch state[recordID] {
		case visited:
			return nil
		case visiting:
			return fmt.Errorf("dependency cycle includes record %s", recordID)
		}
		state[recordID] = visiting
		for _, dependencyID := range recordsByID[recordID].DependsOn {
			if err := visit(dependencyID); err != nil {
				return err
			}
		}
		state[recordID] = visited
		return nil
	}
	for _, record := range matrix.Records {
		if err := visit(record.ID); err != nil {
			return err
		}
	}
	return nil
}

func hasNonEmptyString(values []string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return true
		}
	}
	return false
}
