package designworkflow

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"

	"kicadai/internal/blocks"
	"kicadai/internal/blocks/verification"
	"kicadai/internal/reports"
)

var blockEvidenceCache = struct {
	sync.Mutex
	byKey map[string]map[string]blockEvidence
}{byKey: map[string]map[string]blockEvidence{}}

type BlockEvidenceSummary struct {
	InstanceID        string                `json:"instance_id"`
	BlockID           string                `json:"block_id"`
	CaseID            string                `json:"case_id,omitempty"`
	EvidenceLevel     string                `json:"evidence_level,omitempty"`
	Status            string                `json:"status"`
	Readiness         blocks.BlockReadiness `json:"readiness"`
	VerificationLevel string                `json:"verification_level,omitempty"`
	ValidationRules   []string              `json:"validation_rules,omitempty"`
	RequiredRoutes    []string              `json:"required_routes,omitempty"`
	Gaps              []string              `json:"gaps,omitempty"`
}

func blockEvidenceForRequest(ctx context.Context, registry blocks.Registry, request Request) ([]BlockEvidenceSummary, []reports.Issue) {
	index, indexIssues := builtinBlockEvidenceIndex(ctx, registry)
	inventory := blockInventoryByID(registry)
	requiredRoutes := blockRequiredRoutesByID(registry)
	summaries := make([]BlockEvidenceSummary, 0, len(request.Blocks))
	issues := append([]reports.Issue(nil), indexIssues...)
	for blockIndex, instance := range request.Blocks {
		evidence, ok := index[instance.BlockID]
		summary := BlockEvidenceSummary{InstanceID: instance.ID, BlockID: instance.BlockID}
		if family, ok := inventory[instance.BlockID]; ok {
			summary.Readiness = family.Readiness
			summary.VerificationLevel = string(family.VerificationLevel)
			summary.ValidationRules = slices.Clone(family.ElectricalRules)
			summary.RequiredRoutes = slices.Clone(requiredRoutes[instance.BlockID])
			summary.Gaps = slices.Clone(family.Gaps)
		}
		if ok {
			summary.CaseID = evidence.CaseID
			summary.EvidenceLevel = string(evidence.EvidenceLevel)
			summary.Status = "verified"
		} else {
			summary.Status = "missing"
			issues = append(issues, reports.Issue{
				Code:       reports.CodeValidationFailed,
				Severity:   reports.SeverityWarning,
				Path:       "blocks[" + strconv.Itoa(blockIndex) + "].verification",
				Message:    "block has no built-in verification evidence: " + instance.BlockID,
				Suggestion: "add a block verification manifest before claiming stronger readiness",
			})
		}
		if request.Validation.Acceptance == AcceptanceFabricationCandidate && fabricationEvidenceBlocks(registry, instance.BlockID, evidence, ok) {
			issues = append(issues, reports.Issue{
				Code:       reports.CodeValidationFailed,
				Severity:   reports.SeverityError,
				Path:       "blocks[" + strconv.Itoa(blockIndex) + "].verification",
				Message:    "block fabrication readiness claim lacks ERC/DRC or reference verification evidence: " + instance.BlockID,
				Suggestion: "raise block verification evidence to erc_drc_verified or reference_verified",
			})
		}
		summaries = append(summaries, summary)
	}
	return summaries, issues
}

type blockInventoryProvider interface {
	Inventory() blocks.BlockLibraryInventory
}

func blockInventoryByID(registry blocks.Registry) map[string]blocks.BlockFamilyInventory {
	builtin, ok := registry.(blockInventoryProvider)
	if !ok {
		return nil
	}
	inventory := builtin.Inventory()
	byID := make(map[string]blocks.BlockFamilyInventory, len(inventory.Families))
	for _, family := range inventory.Families {
		byID[family.ID] = family
	}
	return byID
}

func blockRequiredRoutesByID(registry blocks.Registry) map[string][]string {
	byID := make(map[string][]string, len(registry.ListBlocks()))
	for _, summary := range registry.ListBlocks() {
		definition, ok := registry.GetBlock(summary.ID)
		if !ok || definition.PCBRealization == nil {
			continue
		}
		routes := slices.Clone(definition.PCBRealization.Validation.RequiredRoutes)
		slices.Sort(routes)
		byID[summary.ID] = routes
	}
	return byID
}

type blockEvidence struct {
	CaseID        string
	EvidenceLevel verification.EvidenceLevel
}

func builtinBlockEvidenceIndex(ctx context.Context, registry blocks.Registry) (map[string]blockEvidence, []reports.Issue) {
	key := blockEvidenceCacheKey(registry)
	blockEvidenceCache.Lock()
	if cached, ok := blockEvidenceCache.byKey[key]; ok {
		blockEvidenceCache.Unlock()
		return cloneBlockEvidenceIndex(cached), nil
	}
	blockEvidenceCache.Unlock()

	manifests, issues := verification.LoadSuite(builtinVerificationRoot())
	loadIssues := contextualizeEvidenceLoadIssues(issues)
	if reports.HasBlockingIssue(issues) {
		return map[string]blockEvidence{}, loadIssues
	}
	index := map[string]blockEvidence{}
	for _, manifest := range manifests {
		if ctx.Err() != nil {
			return index, append(loadIssues, reports.Issue{
				Code:     reports.CodeOperationCanceled,
				Severity: reports.SeverityWarning,
				Path:     "block_verification",
				Message:  "block verification evidence scan was interrupted: " + ctx.Err().Error(),
			})
		}
		result := verification.RunCase(ctx, manifest, verification.RunOptions{Registry: registry})
		if result.Status != verification.StatusPass && result.Status != verification.StatusWarning {
			continue
		}
		current, ok := index[manifest.BlockID]
		candidate := blockEvidence{CaseID: manifest.ID, EvidenceLevel: manifest.Expected.EvidenceLevel}
		if !ok || verificationEvidenceRank(candidate.EvidenceLevel) > verificationEvidenceRank(current.EvidenceLevel) {
			index[manifest.BlockID] = candidate
		}
	}
	if ctx.Err() != nil {
		return index, loadIssues
	}
	blockEvidenceCache.Lock()
	blockEvidenceCache.byKey[key] = cloneBlockEvidenceIndex(index)
	blockEvidenceCache.Unlock()
	return index, loadIssues
}

func blockEvidenceCacheKey(registry blocks.Registry) string {
	summaries := slices.Clone(registry.ListBlocks())
	slices.SortFunc(summaries, func(a, b blocks.BlockSummary) int {
		return strings.Compare(a.ID, b.ID)
	})
	parts := make([]string, 0, len(summaries))
	for _, summary := range summaries {
		parts = append(parts, summary.ID+":"+string(summary.VerificationLevel))
	}
	return strings.Join(parts, "|")
}

func cloneBlockEvidenceIndex(index map[string]blockEvidence) map[string]blockEvidence {
	clone := make(map[string]blockEvidence, len(index))
	for key, value := range index {
		clone[key] = value
	}
	return clone
}

func fabricationEvidenceBlocks(registry blocks.Registry, blockID string, evidence blockEvidence, hasEvidence bool) bool {
	definition, ok := registry.GetBlock(blockID)
	if !ok || !definition.Verification.Level.AllowsFabricationReadinessClaim() {
		return false
	}
	if !hasEvidence {
		return true
	}
	return verificationEvidenceRank(evidence.EvidenceLevel) < verificationEvidenceRank(verification.EvidenceERCDRCVerified)
}

func verificationEvidenceRank(level verification.EvidenceLevel) int {
	switch level {
	case verification.EvidenceReferenceVerified:
		return 6
	case verification.EvidenceERCDRCVerified:
		return 5
	case verification.EvidencePCBVerified:
		return 4
	case verification.EvidenceTransferVerified:
		return 3
	case verification.EvidenceSchematicVerified:
		return 2
	case verification.EvidenceDefinitionOnly:
		return 1
	default:
		return 0
	}
}

func builtinVerificationRoot() string {
	if root := strings.TrimSpace(os.Getenv("KICADAI_BLOCK_VERIFICATION_ROOT")); root != "" {
		return root
	}
	_, file, _, ok := runtime.Caller(0)
	if ok {
		return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "blocks", "testdata", "verification"))
	}
	return ""
}

func contextualizeEvidenceLoadIssues(issues []reports.Issue) []reports.Issue {
	contextualized := make([]reports.Issue, 0, len(issues))
	for _, issue := range issues {
		issue.Severity = reports.SeverityWarning
		if issue.Path == "" {
			issue.Path = "block_verification"
		}
		issue.Message = "load block verification evidence: " + issue.Message
		contextualized = append(contextualized, issue)
	}
	return contextualized
}
