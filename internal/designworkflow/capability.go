package designworkflow

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"kicadai/internal/blocks"
	"kicadai/internal/capabilitygate"
	"kicadai/internal/reports"
)

const (
	CodeCapabilityUnsupported       reports.Code = "CAPABILITY_UNSUPPORTED"
	CodeCapabilityExperimentalOptIn reports.Code = "CAPABILITY_EXPERIMENTAL_OPT_IN_REQUIRED"
	CodeCapabilityAssessmentInvalid reports.Code = "CAPABILITY_ASSESSMENT_INVALID"
	CodeBlockEvidenceLoadFailed     reports.Code = "BLOCK_EVIDENCE_LOAD_FAILED"
)

func assessCreateRequest(ctx context.Context, request Request, opts CreateOptions) (capabilitygate.Assessment, error) {
	if opts.InitialCapabilityAssessment != nil {
		assessment := *opts.InitialCapabilityAssessment
		if err := capabilitygate.Validate(assessment); err != nil {
			return capabilitygate.Assessment{}, err
		}
		if assessment.ExperimentalOptIn != opts.ExperimentalOptIn {
			return capabilitygate.Assessment{}, fmt.Errorf("initial capability assessment experimental opt-in does not match create options")
		}
		return assessment, nil
	}
	if request.ExplicitCircuit != nil {
		return assessExplicitCircuitCapability(request, opts.ExperimentalOptIn)
	}
	return assessBlockRequestCapability(ctx, opts.BlockRegistry, request, opts.ExperimentalOptIn)
}

func assessBlockRequestCapability(ctx context.Context, registry blocks.Registry, request Request, experimentalOptIn bool) (capabilitygate.Assessment, error) {
	input := capabilitygate.Input{Stage: string(StageBlockPlanning), ExperimentalOptIn: experimentalOptIn}
	if registry == nil {
		input.Requirements = append(input.Requirements, capabilitygate.Requirement{
			Kind: capabilitygate.RequirementArchitecture, ID: "block_registry", EvidenceIDs: []string{"block-registry"},
		})
		input.Evidence = append(input.Evidence, capabilitygate.Evidence{
			ID: "block-registry", Kind: "block_registry", Status: capabilitygate.EvidenceMissing,
		})
		input.Gaps = append(input.Gaps, capabilitygate.Gap{
			Code: string(CodeCapabilityUnsupported), Kind: capabilitygate.RequirementArchitecture,
			ID: "block_registry", Stage: string(StageBlockPlanning), Reason: "block registry is unavailable",
			Action: "provide a validated block registry",
		})
		return capabilitygate.Assess(input)
	}
	evidenceSummaries, evidenceIssues := blockEvidenceForRequest(ctx, registry, request)
	loadIssueIndex := 0
	for _, issue := range evidenceIssues {
		if issue.Code != CodeBlockEvidenceLoadFailed {
			input.Risks = append(input.Risks, capabilitygate.Risk{
				Code: string(issue.Code), Stage: string(StageBlockPlanning),
				Summary: issue.Message, Mitigation: firstNonEmpty(issue.Suggestion, "repair block verification evidence"),
			})
			continue
		}
		index := loadIssueIndex
		loadIssueIndex++
		evidenceID := fmt.Sprintf("block-evidence-load:%d", index)
		input.Evidence = append(input.Evidence, capabilitygate.Evidence{
			ID: evidenceID, Kind: "block_verification", Status: capabilitygate.EvidenceFailed,
			Source: "block-verification://load", Stage: string(StageBlockPlanning),
			Description: issue.Message,
		})
		input.Requirements = append(input.Requirements, capabilitygate.Requirement{
			Kind: capabilitygate.RequirementArchitecture, ID: evidenceID, EvidenceIDs: []string{evidenceID},
		})
		input.Gaps = append(input.Gaps, capabilitygate.Gap{
			Code: string(CodeCapabilityUnsupported), Kind: capabilitygate.RequirementArchitecture,
			ID: evidenceID, Stage: string(StageBlockPlanning), Reason: issue.Message,
			Action: firstNonEmpty(issue.Suggestion, "restore reproducible block verification evidence"),
		})
	}
	evidenceByInstance := make(map[string]BlockEvidenceSummary, len(evidenceSummaries))
	for _, summary := range evidenceSummaries {
		evidenceByInstance[summary.InstanceID] = summary
	}
	for _, instance := range request.Blocks {
		definition, ok := registry.GetBlock(instance.BlockID)
		evidenceID := "block:" + stableCapabilityIdentity(instance.BlockID, instance.ID)
		if !ok {
			input.Evidence = append(input.Evidence, capabilitygate.Evidence{
				ID: evidenceID, Kind: "block_verification", Status: capabilitygate.EvidenceFailed,
				Source: "block-registry://" + instance.BlockID, Stage: string(StageBlockPlanning),
			})
			input.Requirements = append(input.Requirements, capabilitygate.Requirement{
				Kind: capabilitygate.RequirementArchitecture, ID: instance.BlockID, EvidenceIDs: []string{evidenceID},
			})
			input.Gaps = append(input.Gaps, capabilitygate.Gap{
				Code: string(CodeCapabilityUnsupported), Kind: capabilitygate.RequirementArchitecture,
				ID: instance.BlockID, Stage: string(StageBlockPlanning),
				Reason: "requested block is not registered",
				Action: "add a generic registered block with verification evidence",
			})
			continue
		}
		summary := evidenceByInstance[instance.ID]
		evidenceDigest, err := capabilitygate.Digest(struct {
			Definition blocks.BlockDefinition
			Summary    BlockEvidenceSummary
		}{Definition: definition, Summary: summary})
		if err != nil {
			return capabilitygate.Assessment{}, fmt.Errorf("digest block evidence %q: %w", instance.BlockID, err)
		}
		status := capabilitygate.EvidenceVerified
		source := "block-verification://" + summary.CaseID
		if summary.Status != "verified" || strings.TrimSpace(summary.CaseID) == "" {
			status = capabilitygate.EvidenceInferred
			source = "block-registry://" + definition.ID + "/" + definition.Version
			input.Risks = append(input.Risks, capabilitygate.Risk{
				Code: "CAPABILITY_BLOCK_EVIDENCE_PROVISIONAL", Stage: string(StageBlockPlanning),
				Summary:    "block " + definition.ID + " has no reproducible built-in verification case",
				Mitigation: "add and promote a generic block verification manifest",
			})
		}
		input.Evidence = append(input.Evidence, capabilitygate.Evidence{
			ID: evidenceID, Kind: "block_verification", Status: status, Source: source,
			Digest: evidenceDigest, Stage: string(StageBlockPlanning),
			Description: "registered block definition and built-in verification evidence",
		})
		input.Requirements = append(input.Requirements,
			capabilitygate.Requirement{
				Kind: capabilitygate.RequirementDomain, ID: firstNonEmpty(strings.TrimSpace(definition.Category), "circuit"),
				Description: "block electrical domain", EvidenceIDs: []string{evidenceID},
			},
			capabilitygate.Requirement{
				Kind: capabilitygate.RequirementArchitecture, ID: definition.ID,
				Description: "registered circuit block", EvidenceIDs: []string{evidenceID},
			},
			capabilitygate.Requirement{
				Kind: capabilitygate.RequirementPhysical, ID: definition.ID + ":pcb_realization",
				Description: "block physical realization contract", EvidenceIDs: []string{evidenceID},
			},
		)
		for _, component := range definition.Components {
			componentID := firstNonEmpty(strings.TrimSpace(component.ComponentID), strings.TrimSpace(component.FootprintID), strings.TrimSpace(component.SymbolID), strings.TrimSpace(component.Role))
			if componentID == "" {
				continue
			}
			input.Requirements = append(input.Requirements, capabilitygate.Requirement{
				Kind: capabilitygate.RequirementComponent, ID: componentID,
				Description: "block component or package requirement", EvidenceIDs: []string{evidenceID},
			})
		}
	}
	if len(request.Blocks) == 0 {
		input.Requirements = append(input.Requirements, capabilitygate.Requirement{
			Kind: capabilitygate.RequirementArchitecture, ID: "circuit_architecture", EvidenceIDs: []string{"architecture-missing"},
		})
		input.Evidence = append(input.Evidence, capabilitygate.Evidence{
			ID: "architecture-missing", Kind: "block_registry", Status: capabilitygate.EvidenceMissing,
		})
		input.Gaps = append(input.Gaps, capabilitygate.Gap{
			Code: string(CodeCapabilityUnsupported), Kind: capabilitygate.RequirementArchitecture,
			ID: "circuit_architecture", Stage: string(StageBlockPlanning),
			Reason: "request contains no explicit circuit and no block instances",
			Action: "provide a normalized explicit circuit or registered block composition",
		})
	}
	appendWorkflowContractRequirements(&input, request)
	return capabilitygate.Assess(input)
}

func assessExplicitCircuitCapability(request Request, experimentalOptIn bool) (capabilitygate.Assessment, error) {
	circuit := request.ExplicitCircuit
	input := capabilitygate.Input{Stage: string(StageBlockPlanning), ExperimentalOptIn: experimentalOptIn}
	if circuit == nil {
		return capabilitygate.Assessment{}, fmt.Errorf("explicit circuit is required")
	}
	digest, err := capabilitygate.Digest(circuit)
	if err != nil {
		return capabilitygate.Assessment{}, fmt.Errorf("digest explicit circuit: %w", err)
	}
	status := capabilitygate.EvidenceVerified
	if !validSHA256(circuit.ResolutionHash) || !validSHA256(circuit.CatalogHash) {
		status = capabilitygate.EvidenceInferred
		input.Risks = append(input.Risks, capabilitygate.Risk{
			Code: "CAPABILITY_EXPLICIT_PROVENANCE_PROVISIONAL", Stage: string(StageBlockPlanning),
			Summary:    "explicit circuit lacks complete resolution or catalog provenance",
			Mitigation: "lower the circuit from a deterministic architecture search and catalog snapshot",
		})
	}
	provenanceEvidenceID := "explicit-circuit:" + digest[:16]
	input.Evidence = append(input.Evidence, capabilitygate.Evidence{
		ID: provenanceEvidenceID, Kind: "explicit_circuit_provenance", Status: status,
		Source: "explicit-circuit://" + firstNonEmpty(circuit.ResolutionHash, "unresolved"),
		Digest: digest, Stage: string(StageBlockPlanning),
		Description: "normalized explicit circuit, catalog, and lowering provenance",
	})
	input.Requirements = append(input.Requirements,
		capabilitygate.Requirement{
			Kind: capabilitygate.RequirementDomain, ID: firstNonEmpty(request.Intent.Category, "explicit_circuit"),
			Description: "explicit circuit electrical domain", EvidenceIDs: []string{provenanceEvidenceID},
		},
		capabilitygate.Requirement{
			Kind: capabilitygate.RequirementArchitecture, ID: "explicit_circuit",
			Description: "deterministically lowered explicit circuit", EvidenceIDs: []string{provenanceEvidenceID},
		},
		capabilitygate.Requirement{
			Kind: capabilitygate.RequirementPhysical, ID: "explicit_pcb_realization",
			Description: "explicit component, net, region, and routing contracts", EvidenceIDs: []string{provenanceEvidenceID},
		},
	)
	for _, component := range circuit.Components {
		input.Requirements = append(input.Requirements, capabilitygate.Requirement{
			Kind:        capabilitygate.RequirementComponent,
			ID:          firstNonEmpty(strings.TrimSpace(component.FootprintID), strings.TrimSpace(component.ID)),
			Description: "explicit component and package identity", EvidenceIDs: []string{provenanceEvidenceID},
		})
	}
	if circuit.Simulation != nil || circuit.ClosedLoop != nil {
		input.Requirements = append(input.Requirements, capabilitygate.Requirement{
			Kind: capabilitygate.RequirementModel, ID: "explicit_simulation_resolution",
			Description: "explicit simulation and closed-loop model resolution", EvidenceIDs: []string{provenanceEvidenceID},
		})
	}
	appendWorkflowContractRequirements(&input, request)
	return capabilitygate.Assess(input)
}

func appendWorkflowContractRequirements(input *capabilitygate.Input, request Request) {
	contract := struct {
		Acceptance     AcceptanceLevel
		RequireERC     bool
		RequireDRC     bool
		StrictUnrouted bool
		StrictZones    bool
	}{
		Acceptance: request.Validation.Acceptance, RequireERC: request.Validation.RequireERC,
		RequireDRC: request.Validation.RequireDRC, StrictUnrouted: request.Validation.StrictUnrouted,
		StrictZones: request.Validation.StrictZones,
	}
	digest, err := capabilitygate.Digest(contract)
	if err != nil {
		return
	}
	evidenceID := "workflow-contract:" + digest[:16]
	input.Evidence = append(input.Evidence, capabilitygate.Evidence{
		ID: evidenceID, Kind: "workflow_contract", Status: capabilitygate.EvidenceVerified,
		Source: "workflow://validation-contract", Digest: digest, Stage: string(StageBlockPlanning),
		Description: "registered deterministic workflow validation capabilities",
	})
	input.Requirements = append(input.Requirements, capabilitygate.Requirement{
		Kind: capabilitygate.RequirementVerification, ID: "writer_correctness",
		Description: "writer correctness and normalized round-trip verification", EvidenceIDs: []string{evidenceID},
	})
	if request.Validation.RequireERC {
		input.Requirements = append(input.Requirements, capabilitygate.Requirement{
			Kind: capabilitygate.RequirementVerification, ID: "erc",
			Description: "KiCad electrical rules verification", EvidenceIDs: []string{evidenceID},
		})
	}
	if request.Validation.RequireDRC {
		input.Requirements = append(input.Requirements, capabilitygate.Requirement{
			Kind: capabilitygate.RequirementVerification, ID: "strict_drc",
			Description: "strict KiCad design rules verification", EvidenceIDs: []string{evidenceID},
		})
	}
	if request.Validation.StrictUnrouted {
		input.Requirements = append(input.Requirements, capabilitygate.Requirement{
			Kind: capabilitygate.RequirementVerification, ID: "route_completion",
			Description: "strict required-net route completion", EvidenceIDs: []string{evidenceID},
		})
	}
}

func capabilityGateIssues(assessment capabilitygate.Assessment) []reports.Issue {
	switch assessment.Classification {
	case capabilitygate.ClassificationUnsupported:
		issues := make([]reports.Issue, 0, len(assessment.Gaps))
		for _, gap := range assessment.Gaps {
			issues = append(issues, reports.Issue{
				Code: CodeCapabilityUnsupported, Severity: reports.SeverityBlocked,
				Path:    "capability." + string(gap.Kind) + "." + gap.ID,
				Message: gap.Reason, Suggestion: gap.Action,
			})
		}
		if len(issues) == 0 {
			issues = append(issues, reports.Issue{
				Code: CodeCapabilityUnsupported, Severity: reports.SeverityBlocked,
				Path: "capability", Message: "request is outside the validated capability envelope",
			})
		}
		return issues
	case capabilitygate.ClassificationExperimental:
		if !assessment.ExperimentalOptIn {
			return []reports.Issue{{
				Code: CodeCapabilityExperimentalOptIn, Severity: reports.SeverityBlocked,
				Path:       "capability.experimental_opt_in",
				Message:    "request is experimental and requires explicit authorization before generation",
				Suggestion: "rerun with explicit experimental opt-in; the result will not be fabrication-ready",
			}}
		}
	}
	return nil
}

func applyWorkflowCapability(result WorkflowResult, initial capabilitygate.Assessment) WorkflowResult {
	assessment := initial
	assessmentInvalid := false
	projectRoot := stableCapabilityProjectRoot(result.Project.OutputDir)
	for index, stage := range result.Stages {
		digest, err := capabilitygate.Digest(capabilityStageDigest{
			Name: stage.Name, Status: stage.Status, Issues: capabilityIssueDigests(stage.Issues, projectRoot),
		})
		if err != nil {
			continue
		}
		evidenceID := fmt.Sprintf("workflow:%03d:%s", index, stage.Name)
		checkpoint := capabilitygate.CheckpointInput{Stage: string(stage.Name)}
		switch stage.Status {
		case StageStatusBlocked:
			checkpoint.Evidence = []capabilitygate.Evidence{{
				ID: evidenceID, Kind: "workflow_stage", Status: capabilitygate.EvidenceFailed,
				Source: "workflow://" + string(stage.Name), Digest: digest, Stage: string(stage.Name),
			}}
			checkpoint.Gaps = []capabilitygate.Gap{{
				Code: "CAPABILITY_STAGE_FAILED", Kind: capabilitygate.RequirementVerification,
				ID: string(stage.Name), Stage: string(stage.Name),
				Reason: "required workflow evidence failed at " + string(stage.Name),
				Action: "resolve the blocking stage diagnostics and reassess the request",
			}}
		case StageStatusSkipped:
			checkpoint.Risks = []capabilitygate.Risk{{
				Code: "CAPABILITY_STAGE_SKIPPED", Stage: string(stage.Name),
				Summary: "workflow stage was not applicable or was skipped",
			}}
		default:
			checkpoint.Evidence = []capabilitygate.Evidence{{
				ID: evidenceID, Kind: "workflow_stage", Status: capabilitygate.EvidenceVerified,
				Source: "workflow://" + string(stage.Name), Digest: digest, Stage: string(stage.Name),
			}}
		}
		next, err := capabilitygate.Reassess(assessment, checkpoint)
		if err != nil {
			result.Stages[index].Issues = append(result.Stages[index].Issues, reports.Issue{
				Code: CodeCapabilityAssessmentInvalid, Severity: reports.SeverityBlocked,
				Path:       "capability.checkpoints." + string(stage.Name),
				Message:    "record capability checkpoint: " + err.Error(),
				Suggestion: "repair the capability evidence before relying on this workflow",
			})
			result.Stages[index].Status = StageStatusBlocked
			assessmentInvalid = true
			failed, fallbackErr := capabilitygate.Reassess(assessment, capabilitygate.CheckpointInput{
				Stage: string(stage.Name),
				Gaps: []capabilitygate.Gap{{
					Code: string(CodeCapabilityAssessmentInvalid), Kind: capabilitygate.RequirementVerification,
					ID: "assessment_integrity", Stage: string(stage.Name),
					Reason: "capability checkpoint evidence could not be validated",
					Action: "repair the capability evidence and rerun the workflow",
				}},
			})
			if fallbackErr != nil {
				failed, _ = capabilitygate.Assess(capabilitygate.Input{
					Stage: string(stage.Name),
					Requirements: []capabilitygate.Requirement{{
						Kind: capabilitygate.RequirementVerification, ID: "assessment_integrity",
						EvidenceIDs: []string{"assessment-integrity"},
					}},
					Evidence: []capabilitygate.Evidence{{
						ID: "assessment-integrity", Kind: "assessment_integrity",
						Status: capabilitygate.EvidenceFailed, Stage: string(stage.Name),
					}},
					Gaps: []capabilitygate.Gap{{
						Code: string(CodeCapabilityAssessmentInvalid), Kind: capabilitygate.RequirementVerification,
						ID: "assessment_integrity", Stage: string(stage.Name),
						Reason: "capability checkpoint evidence could not be validated",
						Action: "repair the capability evidence and rerun the workflow",
					}},
				})
			}
			assessment = failed
			break
		}
		assessment = next
	}
	if assessmentInvalid {
		promotion := result.Promotion
		result = BuildWorkflowResult(result.Project, result.Acceptance.Requested, result.Stages)
		result.Promotion = promotion
	}
	result.Capability = &assessment
	if !assessment.AllowsFabricationReadyClaim() {
		result.Acceptance.FabricationReady = false
		if result.Acceptance.Achieved == AcceptanceFabricationCandidate {
			result.Acceptance.Achieved = AcceptanceERCDRC
		}
	}
	return result
}

type capabilityStageDigest struct {
	Name   StageName               `json:"name"`
	Status StageStatus             `json:"status"`
	Issues []capabilityIssueDigest `json:"issues,omitempty"`
}

type capabilityIssueDigest struct {
	Code     reports.Code     `json:"code"`
	Severity reports.Severity `json:"severity"`
	Path     string           `json:"path,omitempty"`
}

func stableCapabilityProjectRoot(outputDir string) string {
	if outputDir == "" {
		return ""
	}
	projectRoot := filepath.Clean(outputDir)
	if absolute, err := filepath.Abs(projectRoot); err == nil {
		projectRoot = absolute
	}
	return projectRoot
}

func capabilityIssueDigests(issues []reports.Issue, projectRoot string) []capabilityIssueDigest {
	result := make([]capabilityIssueDigest, 0, len(issues))
	for _, issue := range issues {
		result = append(result, capabilityIssueDigest{
			Code: issue.Code, Severity: issue.Severity, Path: stableCapabilityIssuePath(issue.Path, projectRoot),
		})
	}
	return result
}

func stableCapabilityIssuePath(issuePath, projectRoot string) string {
	if issuePath == "" {
		return ""
	}
	issuePath = filepath.Clean(issuePath)
	if projectRoot == "" || !filepath.IsAbs(issuePath) {
		return filepath.ToSlash(issuePath)
	}
	relative, err := filepath.Rel(projectRoot, issuePath)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return filepath.ToSlash(issuePath)
	}
	if relative == "." {
		return "."
	}
	return filepath.ToSlash(relative)
}

func stableCapabilityIdentity(parts ...string) string {
	var normalized []string
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			normalized = append(normalized, part)
		}
	}
	if len(normalized) == 0 {
		return "unnamed"
	}
	return strings.Join(normalized, ":")
}
