package main

import (
	"context"
	"io"
	"os"
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
	ReadyForWrite     bool                                 `json:"ready_for_write"`
}

type circuitPreflightEvaluation struct {
	Data         circuitPreflightData
	Issues       []reports.Issue
	LibraryIndex *libraryresolver.LibraryIndex
}

func runCircuitPreflight(ctx context.Context, opts cliOptions, stdout io.Writer) error {
	if issue := parseCircuitPreflightArgs(&opts); issue != nil {
		return writeReportFailure(stdout, "circuit.preflight", *issue)
	}
	evaluation := evaluateCircuitPreflight(ctx, opts)
	return writeCircuitPreflightResult(stdout, evaluation.Data, evaluation.Issues)
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
	data := circuitPreflightData{InputPath: opts.requestPath, SchematicIssues: []reports.Issue{}, Gates: []circuitPreflightGate{}}
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
	if reports.HasBlockingIssue(issues) {
		result.Issues = preflightIssues(designworkflow.StageParseRequest, issues)
		return result
	}
	result.Data.Graph = &graph

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
		return result
	}
	result.Data.Resolution = &resolved

	request, lowerIssues := circuitgraph.ToDesignRequest(resolved)
	lowerIssues = preflightIssues(designworkflow.StageSchematic, lowerIssues)
	issues = append(issues, lowerIssues...)
	if reports.HasBlockingIssue(lowerIssues) {
		result.Data.Gates = append(result.Data.Gates, preflightGate("schematic_lowering", lowerIssues, designworkflow.StageSchematic))
		result.Issues = issues
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
		return result
	}

	result.Data.Gates = append(result.Data.Gates, circuitPreflightGate{Name: "kicad_erc", Status: externalEvidenceStatus(request.Validation.RequireERC), External: request.Validation.RequireERC})
	result.Data.Gates = append(result.Data.Gates, circuitPreflightGate{Name: "kicad_drc", Status: externalEvidenceStatus(request.Validation.RequireDRC), External: request.Validation.RequireDRC})
	result.Data.ReadyForWrite = !reports.HasBlockingIssue(issues)
	result.Issues = issues
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
