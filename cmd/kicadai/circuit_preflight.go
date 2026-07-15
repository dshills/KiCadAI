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
	"strings"

	"kicadai/internal/circuitgraph"
	"kicadai/internal/components"
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

func runCircuit(ctx context.Context, opts cliOptions, stdout io.Writer) error {
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
	default:
		return writeReportFailure(stdout, "circuit", reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "circuit", Message: "unsupported circuit subcommand " + opts.commandArgs[0], Suggestion: "run kicadai circuit preflight or circuit create"})
	}
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
		return writeCircuitCreateResult(stdout, data, evaluation.Issues)
	}
	if evaluation.LibraryIndex == nil {
		issue := reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Path: "library_index", Stage: string(designworkflow.StageLibraryContext), RetryScope: string(designworkflow.RetryScopeForStage(designworkflow.StageLibraryContext, reports.Issue{})), Message: "circuit create requires resolved symbol and footprint library evidence", Suggestion: "provide --symbols-root and --footprints-root, or a populated --library-cache"}
		return writeCircuitCreateResult(stdout, data, append(evaluation.Issues, issue))
	}
	checkOpts, err := checkOptions(opts)
	if err != nil {
		issue := reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "check_options", Message: err.Error()}
		return writeCircuitCreateResult(stdout, data, append(evaluation.Issues, issue))
	}
	createOpts, err := designCreateOptions(ctx, opts, checkOpts)
	if err != nil {
		issue := reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "create_options", Message: err.Error()}
		return writeCircuitCreateResult(stdout, data, append(evaluation.Issues, issue))
	}
	// Reuse the exact library evidence that participated in preflight resolution.
	createOpts.LibraryIndex = evaluation.LibraryIndex
	createOpts.Writer.LibraryIndex = *evaluation.LibraryIndex
	createOpts.Writer.HasLibraryIndex = true
	workflow := designworkflow.Create(ctx, *evaluation.Data.Request, createOpts)
	data.Workflow = &workflow
	if designworkflow.AcceptanceSatisfied(workflow.Acceptance.Requested, workflow.Acceptance.Achieved) {
		data.ProjectPaths = circuitProjectPaths(opts.output, workflow)
	}
	data.OutstandingEvidence = circuitOutstandingEvidence(evaluation.Data)
	issues := append([]reports.Issue(nil), evaluation.Issues...)
	issues = append(issues, designworkflow.WorkflowIssues(workflow)...)
	return writeCircuitCreateResult(stdout, data, issues)
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

func writeCircuitCreateResult(stdout io.Writer, data circuitCreateData, issues []reports.Issue) error {
	result := reports.ResultWithIssues("circuit.create", data, issues, nil)
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
	issues = append(issues, preflightIssues(designworkflow.StageLibraryContext, libraryIssues)...)
	if reports.HasBlockingIssue(libraryIssues) {
		result.Data.Gates = append(result.Data.Gates, preflightGate("library_context", libraryIssues, designworkflow.StageLibraryContext))
		result.Issues = issues
		return result
	}
	result.LibraryIndex = libraryIndex
	var symbols map[string]circuitgraph.LibrarySymbolEvidence
	var footprints map[string]circuitgraph.LibraryFootprintEvidence
	if libraryIndex != nil {
		symbols, footprints = circuitgraph.LibraryEvidenceFromIndex(*libraryIndex)
	}
	resolver := circuitgraph.NewResolver(circuitgraph.ResolveOptions{Catalog: catalog, CatalogID: "preflight-catalog", LibrarySymbols: symbols, LibraryFootprints: footprints, RequireLibraryEvidence: libraryIndex != nil})
	resolved, resolveIssues := resolver.Resolve(ctx, graph)
	resolveIssues = preflightIssues(designworkflow.StageComponentSelection, resolveIssues)
	issues = append(issues, resolveIssues...)
	result.Data.Gates = append(result.Data.Gates, preflightGate("catalog_resolution", resolveIssues, designworkflow.StageComponentSelection))
	if reports.HasBlockingIssue(resolveIssues) {
		result.Issues = issues
		result.Data.RepairOptions = circuitgraph.RepairOptions(graph, catalog, result.Issues)
		return result
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
	index, issues := libraryresolver.Load(ctx, libraryRootsFromOptions(opts), libraryresolver.LoadOptions{CachePath: opts.libraryCache, Refresh: opts.refreshLibraryCache})
	return &index, issues
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
