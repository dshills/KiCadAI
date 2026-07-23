package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"kicadai/internal/circuitgraph"
	"kicadai/internal/components"
	"kicadai/internal/creationevidence"
	"kicadai/internal/designworkflow"
	"kicadai/internal/generationcapability"
	"kicadai/internal/libraryresolver"
	"kicadai/internal/reports"
	"kicadai/internal/schematicir"
)

type circuitPreflightGate struct {
	Name     string          `json:"name"`
	Status   string          `json:"status"`
	External bool            `json:"external,omitempty"`
	Issues   []reports.Issue `json:"issues,omitempty"`
}

type circuitPreflightData struct {
	InputPath         string                               `json:"input_path"`
	CapabilityProfile string                               `json:"capability_profile"`
	InputContract     string                               `json:"input_contract"`
	Graph             *circuitgraph.Document               `json:"graph,omitempty"`
	Resolution        *circuitgraph.ResolvedDocument       `json:"resolution,omitempty"`
	Request           *designworkflow.Request              `json:"request,omitempty"`
	SchematicIssues   []reports.Issue                      `json:"schematic_issues"`
	Placement         *designworkflow.PlacementStageResult `json:"placement,omitempty"`
	Routing           *designworkflow.RoutingStageResult   `json:"routing,omitempty"`
	RequiredEvidence  []string                             `json:"required_evidence"`
	Gates             []circuitPreflightGate               `json:"gates"`
	RepairOptions     []circuitgraph.RepairOption          `json:"repair_options"`
	ReadyForWrite     bool                                 `json:"ready_for_write"`
}

type circuitPreflightEvaluation struct {
	Data         circuitPreflightData
	Issues       []reports.Issue
	LibraryIndex *libraryresolver.LibraryIndex
}

type circuitCreateData struct {
	Preflight           circuitPreflightData           `json:"preflight"`
	Workflow            *designworkflow.WorkflowResult `json:"workflow,omitempty"`
	ProjectPaths        []string                       `json:"project_paths,omitempty"`
	OutstandingEvidence []string                       `json:"outstanding_evidence"`
	EvidenceArtifacts   []reports.Artifact             `json:"evidence_artifacts,omitempty"`
}

type circuitPatchData struct {
	InputGraphHash            string                        `json:"input_graph_hash"`
	NormalizedPatchOperations []circuitgraph.PatchOperation `json:"normalized_patch_operations,omitempty"`
	Graph                     *circuitgraph.Document        `json:"graph,omitempty"`
	ChangedPaths              []string                      `json:"changed_paths"`
	CriticalDesignProjection  *circuitPatchProjection       `json:"critical_design_projection,omitempty"`
	Preflight                 circuitPreflightData          `json:"preflight"`
	ReadyForWrite             bool                          `json:"ready_for_write"`
}

// circuitPatchProjection makes the graph changes that can affect a generated
// design explicit without claiming that external KiCad evidence has run.
type circuitPatchProjection struct {
	Before circuitgraph.Document `json:"before"`
	After  circuitgraph.Document `json:"after"`
}

type circuitRepairPlanData struct {
	Preflight           circuitPreflightData    `json:"preflight"`
	Plan                circuitgraph.RepairPlan `json:"plan"`
	OutstandingEvidence []string                `json:"outstanding_evidence"`
}

func (data circuitPreflightData) BoundedDiagnostics(maxIssues int) any {
	data.SchematicIssues = reports.BoundedIssues(data.SchematicIssues, maxIssues)
	data.Gates = append([]circuitPreflightGate(nil), data.Gates...)
	for index := range data.Gates {
		data.Gates[index].Issues = reports.BoundedIssues(data.Gates[index].Issues, maxIssues)
	}
	if data.Placement != nil {
		placement := *data.Placement
		placement.Result.Issues = reports.BoundedIssues(placement.Result.Issues, maxIssues)
		placement.Stage.Issues = reports.BoundedIssues(placement.Stage.Issues, maxIssues)
		data.Placement = &placement
	}
	if data.Routing != nil {
		routing := *data.Routing
		routing.Result.Issues = reports.BoundedIssues(routing.Result.Issues, maxIssues)
		routing.Stage.Issues = reports.BoundedIssues(routing.Stage.Issues, maxIssues)
		data.Routing = &routing
	}
	return data
}

func (data circuitCreateData) BoundedDiagnostics(maxIssues int) any {
	data.Preflight = data.Preflight.BoundedDiagnostics(maxIssues).(circuitPreflightData)
	if data.Workflow != nil {
		workflow := *data.Workflow
		workflow.Stages = append([]designworkflow.StageResult(nil), workflow.Stages...)
		for index := range workflow.Stages {
			workflow.Stages[index].Issues = reports.BoundedIssues(workflow.Stages[index].Issues, maxIssues)
		}
		data.Workflow = &workflow
	}
	return data
}

func (data circuitPatchData) BoundedDiagnostics(maxIssues int) any {
	data.Preflight = data.Preflight.BoundedDiagnostics(maxIssues).(circuitPreflightData)
	return data
}

func (data circuitRepairPlanData) BoundedDiagnostics(maxIssues int) any {
	data.Preflight = data.Preflight.BoundedDiagnostics(maxIssues).(circuitPreflightData)
	data.Plan.Issues = reports.BoundedIssues(data.Plan.Issues, maxIssues)
	return data
}

func runCircuit(ctx context.Context, opts cliOptions, stdout io.Writer) error {
	if topic, ok := circuitHelpTopic(opts.commandArgs); ok {
		return writeCircuitHelp(stdout, opts, topic)
	}
	if len(opts.commandArgs) == 0 {
		return writeReportFailure(stdout, "circuit", reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "circuit", Message: "circuit requires subcommand: preflight or create", Suggestion: "run kicadai circuit preflight --request graph.json"})
	}
	switch strings.TrimSpace(opts.commandArgs[0]) {
	case "preflight":
		return runCircuitPreflight(ctx, opts, stdout)
	case "create":
		return runCircuitCreate(ctx, opts, stdout)
	case "patch":
		return runCircuitPatch(ctx, opts, stdout)
	case "repair-plan":
		return runCircuitRepairPlan(ctx, opts, stdout)
	default:
		return writeReportFailure(stdout, "circuit", reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "circuit", Message: "unsupported circuit subcommand " + opts.commandArgs[0], Suggestion: "run kicadai circuit preflight or circuit create"})
	}
}

func circuitHelpTopic(args []string) (string, bool) {
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h" || args[0] == "help") {
		return "circuit", true
	}
	if len(args) == 2 && (args[1] == "--help" || args[1] == "-h" || args[1] == "help") {
		switch args[0] {
		case "preflight", "create":
			return args[0], true
		}
	}
	return "", false
}

func writeCircuitHelp(output io.Writer, opts cliOptions, topic string) error {
	type helpSpec struct {
		usage       string
		description string
		next        string
		flags       []string
	}
	specs := map[string]helpSpec{
		"circuit": {
			usage:       "kicadai circuit <preflight|create|patch|repair-plan> [options]",
			description: "Validate, repair, or create a catalog-resolved circuit graph.",
			next:        "kicadai circuit preflight --request graph.json",
			flags:       []string{"request", "symbols-root", "footprints-root", "library-cache", "catalog-dir"},
		},
		"preflight": {
			usage:       "kicadai circuit preflight --request graph.json [options]",
			description: "Resolve and validate a circuit without writing a project.",
			next:        "kicadai circuit preflight --request graph.json",
			flags:       []string{"request", "symbols-root", "footprints-root", "library-cache", "refresh-library-cache", "catalog-dir", "format", "json"},
		},
		"create": {
			usage:       "kicadai circuit create --request graph.json --output ./out/project [options]",
			description: "Create and validate a KiCad project from a preflight-ready circuit graph.",
			next:        "kicadai circuit create --request graph.json --output ./out/project --overwrite",
			flags:       []string{"request", "output", "overwrite", "symbols-root", "footprints-root", "library-cache", "refresh-library-cache", "kicad-cli", "require-erc", "require-drc", "require-kicad-roundtrip", "strict-diffs", "format", "json"},
		},
	}
	spec, exists := specs[topic]
	if !exists {
		return errors.New("unsupported circuit help topic")
	}
	if _, err := io.WriteString(output, "Usage:\n  "+spec.usage+"\n\n"+spec.description+"\n\nOptions:\n"); err != nil {
		return err
	}
	for _, name := range spec.flags {
		definition, exists := opts.flagHelp[name]
		if !exists {
			continue
		}
		defaultText := ""
		if definition.Default != "" && definition.Default != "false" && definition.Default != "0" {
			defaultText = " (default " + definition.Default + ")"
		}
		if _, err := io.WriteString(output, "  --"+name+"\n      "+definition.Usage+defaultText+"\n"); err != nil {
			return err
		}
	}
	_, err := io.WriteString(output, "\nNext:\n  "+spec.next+"\n")
	return err
}

func runCircuitRepairPlan(ctx context.Context, opts cliOptions, stdout io.Writer) error {
	previous, issue := parseCircuitRepairPlanArgs(&opts)
	if issue != nil {
		return writeReportFailure(stdout, "circuit.repair-plan", *issue)
	}
	evaluation := evaluateCircuitPreflight(ctx, opts)
	data := circuitRepairPlanData{Preflight: evaluation.Data, OutstandingEvidence: circuitOutstandingEvidence(evaluation.Data)}
	if evaluation.Data.Graph == nil {
		data.Plan = circuitgraph.RepairPlan{HashVersion: circuitgraph.RepairPlanHashVersion, State: circuitgraph.RepairPlanBlocked, StopReason: "graph_unavailable", Issues: evaluation.Issues}
	} else {
		data.Plan = circuitgraph.PlanRepair(*evaluation.Data.Graph, evaluation.Data.ReadyForWrite, evaluation.Data.RepairOptions, evaluation.Issues, previous, 3)
	}
	return writeReportJSON(stdout, reports.ResultWithIssues("circuit.repair-plan", data, evaluation.Issues, nil))
}

func parseCircuitRepairPlanArgs(opts *cliOptions) ([]string, *reports.Issue) {
	if len(opts.commandArgs) == 0 || opts.commandArgs[0] != "repair-plan" {
		return nil, &reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "circuit", Message: "circuit requires subcommand: repair-plan"}
	}
	previous := []string{}
	for i := 1; i < len(opts.commandArgs); i++ {
		switch opts.commandArgs[i] {
		case "--json":
		case "--request":
			if i+1 >= len(opts.commandArgs) || opts.requestPath != "" {
				return nil, &reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "request", Message: "repair-plan requires exactly one --request"}
			}
			i++
			opts.requestPath = opts.commandArgs[i]
		case "--previous-hash":
			if i+1 >= len(opts.commandArgs) {
				return nil, &reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "previous_hash", Message: "repair-plan requires a hash value"}
			}
			i++
			previous = append(previous, opts.commandArgs[i])
		default:
			return nil, &reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "circuit.repair-plan", Message: "unsupported repair-plan argument " + opts.commandArgs[i]}
		}
	}
	if opts.requestPath == "" {
		return nil, &reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "request", Message: "repair-plan requires --request graph.json"}
	}
	return previous, nil
}

func runCircuitPatch(ctx context.Context, opts cliOptions, stdout io.Writer) error {
	patchPath, outputPath, issue := parseCircuitPatchArgs(&opts)
	if issue != nil {
		return writeReportFailure(stdout, "circuit.patch", *issue)
	}
	input, err := os.ReadFile(opts.requestPath)
	if err != nil {
		return writeReportFailure(stdout, "circuit.patch", reports.Issue{Code: reports.CodeMissingFile, Severity: reports.SeverityError, Path: opts.requestPath, Message: err.Error()})
	}
	hash := sha256.Sum256(input)
	data := circuitPatchData{InputGraphHash: hex.EncodeToString(hash[:]), ChangedPaths: []string{}, Preflight: circuitPreflightData{InputPath: opts.requestPath, SchematicIssues: []reports.Issue{}, Gates: []circuitPreflightGate{}}}
	graph, issues := circuitgraph.DecodePatchInputStrict(strings.NewReader(string(input)))
	if reports.HasBlockingIssue(issues) {
		return writeCircuitPatchResult(stdout, data, issues)
	}
	patchInput, err := os.Open(patchPath)
	if err != nil {
		return writeCircuitPatchResult(stdout, data, []reports.Issue{{Code: reports.CodeMissingFile, Severity: reports.SeverityError, Path: patchPath, Message: err.Error()}})
	}
	defer patchInput.Close()
	patch, issues := circuitgraph.DecodePatchStrict(patchInput)
	data.NormalizedPatchOperations = append([]circuitgraph.PatchOperation(nil), patch.Operations...)
	if reports.HasBlockingIssue(issues) {
		return writeCircuitPatchResult(stdout, data, issues)
	}
	corrected, issues := circuitgraph.ApplyPatch(graph, patch)
	if reports.HasBlockingIssue(issues) {
		return writeCircuitPatchResult(stdout, data, issues)
	}
	data.Graph = &corrected
	data.ChangedPaths = circuitPatchChangedPaths(patch)
	data.CriticalDesignProjection = &circuitPatchProjection{Before: graph, After: corrected}
	evaluation := evaluatePatchedCircuitPreflight(ctx, opts, corrected)
	data.Preflight = evaluation.Data
	data.ReadyForWrite = evaluation.Data.ReadyForWrite
	issues = append(issues, evaluation.Issues...)
	if !data.ReadyForWrite {
		return writeCircuitPatchResult(stdout, data, issues)
	}
	encoded, err := json.MarshalIndent(corrected, "", "  ")
	if err != nil {
		return writeCircuitPatchResult(stdout, data, append(issues, reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Path: "output", Message: err.Error()}))
	}
	if err := writeCircuitPatchGraph(outputPath, encoded); err != nil {
		return writeCircuitPatchResult(stdout, data, append(issues, reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Path: outputPath, Message: err.Error()}))
	}
	return writeCircuitPatchResult(stdout, data, issues)
}

func parseCircuitPatchArgs(opts *cliOptions) (string, string, *reports.Issue) {
	var patchPath string
	if len(opts.commandArgs) == 0 || opts.commandArgs[0] != "patch" {
		return "", "", &reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "circuit", Message: "circuit requires subcommand: patch"}
	}
	for index := 1; index < len(opts.commandArgs); index++ {
		arg := opts.commandArgs[index]
		switch arg {
		case "--json":
		case "--request", "--patch", "--output":
			if index+1 >= len(opts.commandArgs) {
				return "", "", &reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: strings.TrimPrefix(arg, "--"), Message: "circuit patch requires a value for " + arg}
			}
			index++
			value := opts.commandArgs[index]
			switch arg {
			case "--request":
				if opts.requestPath != "" {
					return "", "", &reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "request", Message: "circuit patch requires exactly one --request"}
				}
				opts.requestPath = value
			case "--patch":
				if patchPath != "" {
					return "", "", &reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "patch", Message: "circuit patch requires exactly one --patch"}
				}
				patchPath = value
			case "--output":
				if opts.output != "" {
					return "", "", &reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "output", Message: "circuit patch requires exactly one --output"}
				}
				opts.output = value
			}
		default:
			return "", "", &reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "circuit.patch", Message: "unsupported circuit patch argument " + arg}
		}
	}
	if opts.requestPath == "" || patchPath == "" || opts.output == "" {
		return "", "", &reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "circuit.patch", Message: "circuit patch requires --request, --patch, and --output"}
	}
	return patchPath, opts.output, nil
}

func evaluatePatchedCircuitPreflight(ctx context.Context, opts cliOptions, graph circuitgraph.Document) circuitPreflightEvaluation {
	inputPath := opts.requestPath
	encoded, err := json.Marshal(graph)
	if err != nil {
		return circuitPreflightEvaluation{Data: circuitPreflightData{InputPath: opts.requestPath}, Issues: []reports.Issue{{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Path: "graph", Message: err.Error()}}}
	}
	file, err := os.CreateTemp("", "kicadai-circuit-patch-*.json")
	if err != nil {
		return circuitPreflightEvaluation{Data: circuitPreflightData{InputPath: opts.requestPath}, Issues: []reports.Issue{{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Path: "graph", Message: err.Error()}}}
	}
	temporaryPath := file.Name()
	defer os.Remove(temporaryPath)
	if _, err := file.Write(encoded); err != nil {
		file.Close()
		return circuitPreflightEvaluation{Data: circuitPreflightData{InputPath: opts.requestPath}, Issues: []reports.Issue{{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Path: "graph", Message: err.Error()}}}
	}
	file.Close()
	opts.requestPath = temporaryPath
	evaluation := evaluateCircuitPreflight(ctx, opts)
	evaluation.Data.InputPath = inputPath
	return evaluation
}

func circuitPatchChangedPaths(patch circuitgraph.PatchDocument) []string {
	result := make([]string, 0, len(patch.Operations))
	for _, operation := range patch.Operations {
		if operation.Net != "" {
			result = append(result, "nets."+operation.Net)
		} else if operation.Component != "" {
			result = append(result, "components."+operation.Component)
		} else if operation.Region != "" {
			result = append(result, "pcb.regions."+operation.Region)
		} else {
			result = append(result, "policy."+operation.Policy)
		}
	}
	return result
}

func writeCircuitPatchGraph(path string, contents []byte) error {
	if _, err := os.Stat(path); err == nil {
		return errors.New("output already exists")
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".kicadai-patch-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err = temporary.Write(append(contents, '\n')); err != nil {
		temporary.Close()
		return err
	}
	if err = temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}

func writeCircuitPatchResult(stdout io.Writer, data circuitPatchData, issues []reports.Issue) error {
	result := reports.ResultWithIssues("circuit.patch", data, issues, nil)
	result.OK = data.ReadyForWrite && !reports.HasBlockingIssue(issues)
	if err := writeReportJSON(stdout, result); err != nil {
		return err
	}
	if !result.OK {
		return errors.New("circuit patch reported blocking issues")
	}
	return nil
}

func runCircuitPreflight(ctx context.Context, opts cliOptions, stdout io.Writer) error {
	if issue := parseCircuitPreflightArgs(&opts); issue != nil {
		return writeReportFailure(stdout, "circuit.preflight", *issue)
	}
	evaluation := evaluateCircuitPreflight(ctx, opts)
	return writeCircuitPreflightResult(stdout, evaluation.Data, evaluation.Issues)
}

func runCircuitCreate(ctx context.Context, opts cliOptions, stdout io.Writer) error {
	if issue := parseCircuitCreateArgs(&opts); issue != nil {
		return writeReportFailure(stdout, "circuit.create", *issue)
	}
	evaluation := evaluateCircuitPreflight(ctx, opts)
	data := circuitCreateData{Preflight: evaluation.Data, OutstandingEvidence: circuitOutstandingEvidence(evaluation.Data)}
	if !evaluation.Data.ReadyForWrite || evaluation.Data.Request == nil {
		return writeCircuitCreateResult(stdout, data, evaluation.Issues, opts.output)
	}
	if evaluation.LibraryIndex == nil {
		issue := reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Path: "library_index", Stage: string(designworkflow.StageLibraryContext), RetryScope: string(designworkflow.RetryScopeForStage(designworkflow.StageLibraryContext, reports.Issue{})), Message: "circuit create requires resolved symbol and footprint library evidence", Suggestion: "provide --symbols-root and --footprints-root, or a populated --library-cache"}
		return writeCircuitCreateResult(stdout, data, append(evaluation.Issues, issue), opts.output)
	}
	checkOpts, err := checkOptions(opts)
	if err != nil {
		issue := reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "check_options", Message: err.Error()}
		return writeCircuitCreateResult(stdout, data, append(evaluation.Issues, issue), opts.output)
	}
	createOpts, err := designCreateOptions(ctx, opts, checkOpts)
	if err != nil {
		issue := reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "create_options", Message: err.Error()}
		return writeCircuitCreateResult(stdout, data, append(evaluation.Issues, issue), opts.output)
	}
	// Reuse the exact library evidence that participated in preflight resolution.
	createOpts.LibraryIndex = evaluation.LibraryIndex
	createOpts.Writer.LibraryIndex = *evaluation.LibraryIndex
	createOpts.Writer.HasLibraryIndex = true
	// Create is synchronous and completes the transactional project-write stage,
	// including .kicadai/transaction.json, before core evidence is assembled.
	workflow := designworkflow.Create(ctx, *evaluation.Data.Request, createOpts)
	promotionFixture, promotionErr := designPromotionFixture(opts, *evaluation.Data.Request, workflow)
	var promotion *designworkflow.PromotionReport
	if promotionErr == nil {
		report := designworkflow.BuildInternalPromotionReport(promotionFixture, workflow)
		promotion = &report
		workflow.Promotion = promotionSummaryPointer(designworkflow.PromotionSummaryFromReport(report, designworkflow.PromotionReportArtifactPath))
	} else {
		evaluation.Issues = append(evaluation.Issues, reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Path: creationevidence.DesignPromotionPath, Stage: "reporting", Message: promotionErr.Error()})
	}
	validation := circuitCreationValidationSummary(workflow)
	var promotionApplicability *creationevidence.Applicability
	if promotionErr != nil {
		validation.Status = "blocked"
		validation.Message = "promotion evidence could not be evaluated: " + promotionErr.Error()
		validation.Gates = append(validation.Gates, creationevidence.Gate{Name: "promotion", Status: "error", Rationale: promotionErr.Error()})
		promotionApplicability = &creationevidence.Applicability{Status: "error", Rationale: "promotion evidence could not be evaluated: " + promotionErr.Error()}
	}
	coreArtifacts, dataIssues := creationevidence.Write(opts.output, creationevidence.Bundle{
		Lane:                   "circuit",
		Request:                *evaluation.Data.Request,
		Workflow:               workflow,
		Validation:             validation,
		Promotion:              promotion,
		PromotionApplicability: promotionApplicability,
		Artifacts:              designworkflow.WorkflowArtifacts(workflow),
	})
	data.EvidenceArtifacts = coreArtifacts
	evaluation.Issues = append(evaluation.Issues, dataIssues...)
	data.Workflow = &workflow
	if designworkflow.AcceptanceSatisfied(workflow.Acceptance.Requested, workflow.Acceptance.Achieved) {
		data.ProjectPaths = circuitProjectPaths(opts.output, workflow)
	}
	data.OutstandingEvidence = circuitOutstandingEvidence(evaluation.Data)
	issues := append([]reports.Issue(nil), evaluation.Issues...)
	issues = append(issues, designworkflow.WorkflowIssues(workflow)...)
	return writeCircuitCreateResult(stdout, data, issues, opts.output)
}

func parseCircuitCreateArgs(opts *cliOptions) *reports.Issue {
	if len(opts.commandArgs) == 0 || strings.TrimSpace(opts.commandArgs[0]) != "create" {
		return &reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "circuit", Message: "circuit requires subcommand: create", Suggestion: "run kicadai circuit create --request graph.json --output ./out/project"}
	}
	for index := 1; index < len(opts.commandArgs); index++ {
		switch strings.TrimSpace(opts.commandArgs[index]) {
		case "--json":
		case "--overwrite":
			opts.overwrite = true
		case "--require-erc":
			opts.requireERC = true
		case "--require-drc":
			opts.requireDRC = true
		case "--require-kicad-roundtrip":
			opts.requireKiCadRoundTrip = true
		case "--strict-diffs":
			opts.strictDiffs = true
		case "--request", "--output":
			if index+1 >= len(opts.commandArgs) {
				return &reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: strings.TrimPrefix(opts.commandArgs[index], "--"), Message: "circuit create requires a value for " + opts.commandArgs[index]}
			}
			index++
			value := opts.commandArgs[index]
			if strings.TrimSpace(value) == "" {
				return &reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: strings.TrimPrefix(opts.commandArgs[index-1], "--"), Message: "circuit create requires a non-empty value for " + opts.commandArgs[index-1]}
			}
			if opts.commandArgs[index-1] == "--request" {
				if strings.TrimSpace(opts.requestPath) != "" {
					return &reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "request", Message: "circuit create requires exactly one --request graph.json"}
				}
				opts.requestPath = value
			} else {
				if strings.TrimSpace(opts.output) != "" {
					return &reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "output", Message: "circuit create requires exactly one --output directory"}
				}
				opts.output = value
			}
		default:
			return &reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "circuit.create", Message: "unsupported circuit create argument " + opts.commandArgs[index]}
		}
	}
	if strings.TrimSpace(opts.requestPath) == "" {
		return &reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "request", Message: "circuit create requires --request graph.json"}
	}
	if strings.TrimSpace(opts.output) == "" {
		return &reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "output", Message: "circuit create requires --output directory"}
	}
	return nil
}

func circuitProjectPaths(output string, workflow designworkflow.WorkflowResult) []string {
	paths := []string{filepath.Clean(output)}
	for _, artifact := range designworkflow.WorkflowArtifacts(workflow) {
		if strings.TrimSpace(artifact.Path) != "" {
			paths = append(paths, artifact.Path)
		}
	}
	return paths
}

func circuitOutstandingEvidence(preflight circuitPreflightData) []string {
	result := append([]string(nil), preflight.RequiredEvidence...)
	if preflight.Request != nil && preflight.Request.Validation.RequireERC {
		result = append(result, "KiCad ERC must be run and pass")
	}
	if preflight.Request != nil && preflight.Request.Validation.RequireDRC {
		result = append(result, "KiCad DRC must be run and pass")
	}
	return result
}

type circuitDiagnosticsArtifact struct {
	Command    string          `json:"command"`
	Version    string          `json:"version"`
	TotalCount int             `json:"total_count"`
	Issues     []reports.Issue `json:"issues"`
}

func writeCircuitCreateResult(stdout io.Writer, data circuitCreateData, issues []reports.Issue, outputDir string) error {
	artifacts := append([]reports.Artifact(nil), data.EvidenceArtifacts...)
	if len(issues) > reports.DefaultMaxEmittedIssues {
		if artifact, issue := writeCompleteCircuitDiagnostics(outputDir, issues); issue != nil {
			issues = append(issues, *issue)
		} else if artifact.Path != "" {
			artifacts = append(artifacts, artifact)
		}
	}
	result := reports.ResultWithIssues("circuit.create", data, issues, artifacts)
	if data.Workflow != nil {
		result.OK = designworkflow.AcceptanceSatisfied(data.Workflow.Acceptance.Requested, data.Workflow.Acceptance.Achieved) && !reports.HasBlockingIssue(result.Issues)
	}
	if err := writeReportJSON(stdout, result); err != nil {
		return err
	}
	if !result.OK {
		return errors.New("circuit create reported blocking issues")
	}
	return nil
}

func circuitCreationValidationSummary(workflow designworkflow.WorkflowResult) creationevidence.ValidationSummary {
	status := "ready"
	message := "requested creation acceptance was achieved"
	if !designworkflow.AcceptanceSatisfied(workflow.Acceptance.Requested, workflow.Acceptance.Achieved) {
		status = "blocked"
		message = "requested creation acceptance was not achieved"
	}
	stage := ""
	if len(workflow.Stages) > 0 {
		stage = string(workflow.Stages[len(workflow.Stages)-1].Name)
	}
	return creationevidence.ValidationSummary{Status: status, Stage: stage, Message: message, Gates: creationevidence.GatesFromWorkflow(workflow)}
}

func writeCompleteCircuitDiagnostics(outputDir string, issues []reports.Issue) (reports.Artifact, *reports.Issue) {
	if strings.TrimSpace(outputDir) == "" {
		return reports.Artifact{}, nil
	}
	info, err := os.Stat(outputDir)
	if os.IsNotExist(err) {
		return reports.Artifact{}, nil
	}
	if err != nil {
		return reports.Artifact{}, &reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Path: ".kicadai/diagnostics.json", Stage: "reporting", Message: err.Error()}
	}
	if !info.IsDir() {
		return reports.Artifact{}, &reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: outputDir, Stage: "reporting", Message: "circuit output path is not a directory"}
	}
	artifactDir := filepath.Join(outputDir, ".kicadai")
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		return reports.Artifact{}, &reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Path: ".kicadai/diagnostics.json", Stage: "reporting", Message: err.Error()}
	}
	complete := circuitDiagnosticsArtifact{
		Command:    "circuit.create",
		Version:    reports.Version,
		TotalCount: len(issues),
		Issues:     reports.SortedIssues(issues),
	}
	return writeJSONArtifact(
		filepath.Join(artifactDir, "diagnostics.json"),
		complete,
		reports.ArtifactDiagnosticsReport,
		".kicadai/diagnostics.json",
		"complete circuit diagnostics omitted from bounded stdout",
	)
}

func parseCircuitPreflightArgs(opts *cliOptions) *reports.Issue {
	if len(opts.commandArgs) == 0 || strings.TrimSpace(opts.commandArgs[0]) != "preflight" {
		return &reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "circuit", Message: "circuit requires subcommand: preflight or create", Suggestion: "run kicadai circuit preflight --request graph.json"}
	}
	for index := 1; index < len(opts.commandArgs); index++ {
		switch strings.TrimSpace(opts.commandArgs[index]) {
		case "--json":
		case "--request":
			if index+1 >= len(opts.commandArgs) || strings.TrimSpace(opts.requestPath) != "" {
				return &reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "request", Message: "circuit preflight requires exactly one --request graph.json"}
			}
			index++
			opts.requestPath = opts.commandArgs[index]
		default:
			return &reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "circuit.preflight", Message: "unsupported circuit preflight argument " + opts.commandArgs[index]}
		}
	}
	if strings.TrimSpace(opts.requestPath) == "" {
		return &reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "request", Message: "circuit preflight requires --request graph.json"}
	}
	return nil
}

func evaluateCircuitPreflight(ctx context.Context, opts cliOptions) circuitPreflightEvaluation {
	data := circuitPreflightData{InputPath: opts.requestPath, SchematicIssues: []reports.Issue{}, Gates: []circuitPreflightGate{}, RepairOptions: []circuitgraph.RepairOption{}}
	result := circuitPreflightEvaluation{Data: data, Issues: []reports.Issue{}}
	catalogDir := opts.catalogDir
	if strings.TrimSpace(catalogDir) == components.DefaultCatalogDir {
		catalogDir = ""
	}
	catalog, err := components.LoadCatalog(ctx, components.LoadOptions{CatalogDir: catalogDir})
	if err != nil {
		result.Issues = []reports.Issue{{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Path: "catalog", Message: err.Error()}}
		return result
	}
	_, err = generationcapability.BuildDocument(catalog)
	if err != nil {
		result.Issues = []reports.Issue{{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Path: "capability", Message: err.Error()}}
		return result
	}
	result.Data.CapabilityProfile = generationcapability.ProfileGenericCircuit
	if generic, ok := generationcapability.Lookup(generationcapability.ProfileGenericCircuit); ok {
		result.Data.InputContract = generic.InputContract
		result.Data.RequiredEvidence = generic.RequiredEvidence
	}

	contents, err := os.Open(opts.requestPath)
	if err != nil {
		result.Issues = []reports.Issue{{Code: reports.CodeMissingFile, Severity: reports.SeverityError, Path: opts.requestPath, Message: err.Error()}}
		return result
	}
	defer contents.Close()
	graph, issues := circuitgraph.DecodeStrict(contents)
	result.Data.Gates = append(result.Data.Gates, preflightGate("strict_decode", issues, designworkflow.StageParseRequest))
	result.Data.Graph = &graph
	if reports.HasBlockingIssue(issues) {
		result.Issues = preflightIssues(designworkflow.StageParseRequest, issues)
		result.Data.RepairOptions = circuitgraph.RepairOptions(graph, catalog, result.Issues)
		return result
	}

	libraryIndex, libraryIssues := circuitPreflightLibraryIndex(ctx, opts)
	result.LibraryIndex = libraryIndex
	var symbols map[string]circuitgraph.LibrarySymbolEvidence
	var footprints map[string]circuitgraph.LibraryFootprintEvidence
	if libraryIndex != nil {
		symbols, footprints = circuitgraph.LibraryEvidenceFromIndex(*libraryIndex)
	}
	resolveOptions := circuitgraph.ResolveOptions{Catalog: catalog, CatalogID: "preflight-catalog", LibrarySymbols: symbols, LibraryFootprints: footprints, RequireLibraryEvidence: libraryIndex != nil}
	resolver := circuitgraph.NewResolver(resolveOptions)
	var resolved circuitgraph.ResolvedDocument
	var resolveIssues []reports.Issue
	if libraryIndex != nil {
		// Select the concrete catalog records and variants without allowing
		// unrelated inventory diagnostics to decide design validity. The strict
		// evidence-backed resolution below still verifies every selected pin and
		// pad after the corresponding library closure has passed.
		selectionResolver := circuitgraph.NewResolver(circuitgraph.ResolveOptions{Catalog: catalog, CatalogID: "preflight-catalog"})
		resolved, resolveIssues = selectionResolver.Resolve(ctx, graph)
	} else {
		resolved, resolveIssues = resolver.Resolve(ctx, graph)
	}
	resolveIssues = preflightIssues(designworkflow.StageComponentSelection, resolveIssues)
	issues = appendUniquePreflightIssues(issues, resolveIssues)
	result.Data.Gates = append(result.Data.Gates, preflightGate("catalog_resolution", resolveIssues, designworkflow.StageComponentSelection))
	if reports.HasBlockingIssue(resolveIssues) {
		result.Issues = issues
		result.Data.RepairOptions = circuitgraph.RepairOptions(graph, catalog, result.Issues)
		return result
	}
	if libraryIndex != nil {
		closure, closureIssues := libraryresolver.ResolveDesignClosure(*libraryIndex, circuitPreflightClosureRequest(resolved))
		closureIssues = append(closureIssues, libraryresolver.DesignClosureIssuesFrom(libraryIssues, closure)...)
		closureIssues = preflightIssues(designworkflow.StageLibraryContext, closureIssues)
		issues = appendUniquePreflightIssues(issues, closureIssues)
		result.Data.Gates = append(result.Data.Gates, preflightGate("library_context", closureIssues, designworkflow.StageLibraryContext))
		if reports.HasBlockingIssue(closureIssues) {
			result.Issues = issues
			result.Data.RepairOptions = circuitgraph.RepairOptions(graph, catalog, result.Issues)
			return result
		}
		resolved, resolveIssues = circuitgraph.BindLibraryEvidence(resolved, resolveOptions)
		resolveIssues = preflightIssues(designworkflow.StageComponentSelection, resolveIssues)
		issues = appendUniquePreflightIssues(issues, resolveIssues)
		if reports.HasBlockingIssue(resolveIssues) {
			result.Data.Gates = append(result.Data.Gates, preflightGate("library_evidence", resolveIssues, designworkflow.StageComponentSelection))
			result.Issues = issues
			result.Data.RepairOptions = circuitgraph.RepairOptions(graph, catalog, result.Issues)
			return result
		}
	}
	result.Data.Resolution = &resolved

	request, lowerIssues := circuitgraph.ToDesignRequest(resolved)
	lowerIssues = preflightIssues(designworkflow.StageSchematic, lowerIssues)
	issues = append(issues, lowerIssues...)
	if reports.HasBlockingIssue(lowerIssues) {
		result.Data.Gates = append(result.Data.Gates, preflightGate("schematic_lowering", lowerIssues, designworkflow.StageSchematic))
		result.Issues = issues
		result.Data.RepairOptions = circuitgraph.RepairOptions(graph, catalog, result.Issues)
		return result
	}
	result.Data.Request = &request
	result.Data.Gates = append(result.Data.Gates, preflightGate("schematic_lowering", lowerIssues, designworkflow.StageSchematic))

	schematicIssues := schematicir.Validate(request.ExplicitCircuit.Schematic)
	schematicIssues = preflightIssues(designworkflow.StageSchematicElectrical, schematicIssues)
	result.Data.SchematicIssues = schematicIssues
	issues = append(issues, schematicIssues...)
	result.Data.Gates = append(result.Data.Gates, preflightGate("schematic_electrical_and_readability", schematicIssues, designworkflow.StageSchematicElectrical))
	if reports.HasBlockingIssue(schematicIssues) {
		result.Issues = issues
		result.Data.RepairOptions = circuitgraph.RepairOptions(graph, catalog, result.Issues)
		return result
	}

	placed := designworkflow.PlaceExplicitCircuit(ctx, request, designworkflow.PlacementOptions{LibraryIndex: libraryIndex})
	placementIssues := preflightIssues(designworkflow.StagePlacement, placed.Stage.Issues)
	placed.Stage.Issues = placementIssues
	result.Data.Placement = &placed
	issues = append(issues, placementIssues...)
	result.Data.Gates = append(result.Data.Gates, preflightGate("placement", placementIssues, designworkflow.StagePlacement))
	if reports.HasBlockingIssue(placementIssues) || placed.Stage.Status == designworkflow.StageStatusSkipped {
		result.Issues = issues
		result.Data.RepairOptions = circuitgraph.RepairOptions(graph, catalog, result.Issues)
		return result
	}

	routed := designworkflow.RouteExplicitCircuit(ctx, request, placed, designworkflow.RoutingOptions{})
	routingIssues := preflightIssues(designworkflow.StageRouting, routed.Stage.Issues)
	routed.Stage.Issues = routingIssues
	result.Data.Routing = &routed
	issues = append(issues, routingIssues...)
	result.Data.Gates = append(result.Data.Gates, preflightGate("required_net_routing", routingIssues, designworkflow.StageRouting))
	if reports.HasBlockingIssue(routingIssues) || routed.Stage.Status == designworkflow.StageStatusSkipped {
		result.Issues = issues
		result.Data.RepairOptions = circuitgraph.RepairOptions(graph, catalog, result.Issues)
		return result
	}

	result.Data.Gates = append(result.Data.Gates, circuitPreflightGate{Name: "kicad_erc", Status: externalEvidenceStatus(request.Validation.RequireERC), External: request.Validation.RequireERC})
	result.Data.Gates = append(result.Data.Gates, circuitPreflightGate{Name: "kicad_drc", Status: externalEvidenceStatus(request.Validation.RequireDRC), External: request.Validation.RequireDRC})
	result.Data.ReadyForWrite = !reports.HasBlockingIssue(issues)
	result.Issues = issues
	result.Data.RepairOptions = circuitgraph.RepairOptions(graph, catalog, result.Issues)
	return result
}

func circuitPreflightLibraryIndex(ctx context.Context, opts cliOptions) (*libraryresolver.LibraryIndex, []reports.Issue) {
	if !pinmapShouldUseLibraryResolver(opts) {
		return nil, nil
	}
	roots := libraryRootsFromOptions(opts)
	if requestPath := strings.TrimSpace(opts.requestPath); requestPath != "" {
		roots.ProjectDir = filepath.Dir(requestPath)
	}
	index, issues := libraryresolver.Load(ctx, roots, libraryresolver.LoadOptions{CachePath: opts.libraryCache, Refresh: opts.refreshLibraryCache})
	return &index, issues
}

func circuitPreflightClosureRequest(resolved circuitgraph.ResolvedDocument) libraryresolver.ClosureRequest {
	request := libraryresolver.ClosureRequest{}
	for _, component := range resolved.Components {
		request.Variants = append(request.Variants, libraryresolver.VariantReference{
			ComponentID: component.ComponentID, VariantID: component.VariantID, FootprintID: component.FootprintID,
		})
		pinsBySymbol := map[string]map[string]struct{}{}
		unitsBySymbol := map[string]map[int]struct{}{}
		padSet := map[string]struct{}{}
		for _, function := range component.Functions {
			if pinsBySymbol[function.SymbolID] == nil {
				pinsBySymbol[function.SymbolID] = map[string]struct{}{}
			}
			pinsBySymbol[function.SymbolID][function.SymbolPin] = struct{}{}
			if unitsBySymbol[function.SymbolID] == nil {
				unitsBySymbol[function.SymbolID] = map[int]struct{}{}
			}
			unitsBySymbol[function.SymbolID][function.Unit] = struct{}{}
			padSet[function.Pad] = struct{}{}
		}
		for _, symbol := range component.Symbols {
			unitSet := unitsBySymbol[symbol.SymbolID]
			if unitSet == nil {
				unitSet = map[int]struct{}{}
			}
			unitSet[symbol.Unit] = struct{}{}
			request.Symbols = append(request.Symbols, libraryresolver.SymbolReference{
				LibraryID: symbol.SymbolID, Units: sortedIntSet(unitSet), Pins: sortedStringSet(pinsBySymbol[symbol.SymbolID]),
			})
		}
		request.Footprints = append(request.Footprints, libraryresolver.FootprintReference{LibraryID: component.FootprintID, Pads: sortedStringSet(padSet)})
	}
	return request
}

func sortedStringSet(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	slices.Sort(result)
	return result
}

func sortedIntSet(values map[int]struct{}) []int {
	result := make([]int, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	slices.Sort(result)
	return result
}

func appendUniquePreflightIssues(existing []reports.Issue, additional []reports.Issue) []reports.Issue {
	seen := make(map[string]struct{}, len(existing)+len(additional))
	for _, issue := range existing {
		seen[preflightIssueKey(issue)] = struct{}{}
	}
	for _, issue := range additional {
		key := preflightIssueKey(issue)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		existing = append(existing, issue)
	}
	return existing
}

func preflightIssueKey(issue reports.Issue) string {
	return strings.Join([]string{
		string(issue.Code), string(issue.Severity), issue.IssueID, issue.RootCauseID,
		issue.Stage, issue.RetryScope, issue.Path, issue.Message, issue.Suggestion,
		issue.OperationID, strings.Join(issue.UUIDs, "\x1f"), strings.Join(issue.Refs, "\x1f"), strings.Join(issue.Nets, "\x1f"),
	}, "\x00")
}

func preflightGate(name string, issues []reports.Issue, stage designworkflow.StageName) circuitPreflightGate {
	issues = preflightIssues(stage, issues)
	status := "pass"
	if reports.HasBlockingIssue(issues) {
		status = "failed"
	} else if len(issues) > 0 {
		status = "warning"
	}
	return circuitPreflightGate{Name: name, Status: status, Issues: issues}
}

func preflightIssues(stage designworkflow.StageName, issues []reports.Issue) []reports.Issue {
	result := append([]reports.Issue(nil), issues...)
	for index := range result {
		if result[index].Stage == "" {
			result[index].Stage = string(stage)
		}
		if result[index].RetryScope == "" {
			result[index].RetryScope = string(designworkflow.RetryScopeForStage(stage, result[index]))
		}
	}
	return result
}

func externalEvidenceStatus(required bool) string {
	if required {
		return "required_external"
	}
	return "not_required"
}

func writeCircuitPreflightResult(stdout io.Writer, data circuitPreflightData, issues []reports.Issue) error {
	return writeReportJSON(stdout, reports.ResultWithIssues("circuit.preflight", data, issues, nil))
}
