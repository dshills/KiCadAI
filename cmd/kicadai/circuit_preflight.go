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

func runCircuitPreflight(ctx context.Context, opts cliOptions, stdout io.Writer) error {
	if len(opts.commandArgs) == 0 || strings.TrimSpace(opts.commandArgs[0]) != "preflight" {
		return writeReportFailure(stdout, "circuit", reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "circuit", Message: "circuit requires subcommand: preflight", Suggestion: "run kicadai circuit preflight --request graph.json"})
	}
	for index := 1; index < len(opts.commandArgs); index++ {
		switch strings.TrimSpace(opts.commandArgs[index]) {
		case "--json":
		case "--request":
			if index+1 >= len(opts.commandArgs) || strings.TrimSpace(opts.requestPath) != "" {
				return writeReportFailure(stdout, "circuit.preflight", reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "request", Message: "circuit preflight requires exactly one --request graph.json"})
			}
			index++
			opts.requestPath = opts.commandArgs[index]
		default:
			return writeReportFailure(stdout, "circuit.preflight", reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "circuit.preflight", Message: "unsupported circuit preflight argument " + opts.commandArgs[index]})
		}
	}
	if strings.TrimSpace(opts.requestPath) == "" {
		return writeReportFailure(stdout, "circuit.preflight", reports.Issue{Code: reports.CodeInvalidArgument, Severity: reports.SeverityError, Path: "request", Message: "circuit preflight requires --request graph.json"})
	}

	data := circuitPreflightData{InputPath: opts.requestPath, SchematicIssues: []reports.Issue{}, Gates: []circuitPreflightGate{}}
	catalogDir := opts.catalogDir
	if strings.TrimSpace(catalogDir) == components.DefaultCatalogDir {
		catalogDir = ""
	}
	catalog, err := components.LoadCatalog(ctx, components.LoadOptions{CatalogDir: catalogDir})
	if err != nil {
		return writeReportFailure(stdout, "circuit.preflight", reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Path: "catalog", Message: err.Error()})
	}
	_, err = generationcapability.BuildDocument(catalog)
	if err != nil {
		return writeReportFailure(stdout, "circuit.preflight", reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Path: "capability", Message: err.Error()})
	}
	data.CapabilityProfile = generationcapability.ProfileGenericCircuit
	if generic, ok := generationcapability.Lookup(generationcapability.ProfileGenericCircuit); ok {
		data.InputContract = generic.InputContract
		data.RequiredEvidence = generic.RequiredEvidence
	}

	contents, err := os.Open(opts.requestPath)
	if err != nil {
		return writeCircuitPreflightResult(stdout, data, []reports.Issue{{Code: reports.CodeMissingFile, Severity: reports.SeverityError, Path: opts.requestPath, Message: err.Error()}})
	}
	defer contents.Close()
	graph, issues := circuitgraph.DecodeStrict(contents)
	data.Gates = append(data.Gates, preflightGate("strict_decode", issues, designworkflow.StageParseRequest))
	if reports.HasBlockingIssue(issues) {
		return writeCircuitPreflightResult(stdout, data, preflightIssues(designworkflow.StageParseRequest, issues))
	}
	data.Graph = &graph

	libraryIndex, libraryIssues := circuitPreflightLibraryIndex(ctx, opts)
	issues = append(issues, preflightIssues(designworkflow.StageLibraryContext, libraryIssues)...)
	if reports.HasBlockingIssue(libraryIssues) {
		data.Gates = append(data.Gates, preflightGate("library_context", libraryIssues, designworkflow.StageLibraryContext))
		return writeCircuitPreflightResult(stdout, data, issues)
	}
	var symbols map[string]circuitgraph.LibrarySymbolEvidence
	var footprints map[string]circuitgraph.LibraryFootprintEvidence
	if libraryIndex != nil {
		symbols, footprints = circuitgraph.LibraryEvidenceFromIndex(*libraryIndex)
	}
	resolver := circuitgraph.NewResolver(circuitgraph.ResolveOptions{Catalog: catalog, CatalogID: "preflight-catalog", LibrarySymbols: symbols, LibraryFootprints: footprints, RequireLibraryEvidence: libraryIndex != nil})
	resolved, resolveIssues := resolver.Resolve(ctx, graph)
	resolveIssues = preflightIssues(designworkflow.StageComponentSelection, resolveIssues)
	issues = append(issues, resolveIssues...)
	data.Gates = append(data.Gates, preflightGate("catalog_resolution", resolveIssues, designworkflow.StageComponentSelection))
	if reports.HasBlockingIssue(resolveIssues) {
		return writeCircuitPreflightResult(stdout, data, issues)
	}
	data.Resolution = &resolved

	request, lowerIssues := circuitgraph.ToDesignRequest(resolved)
	lowerIssues = preflightIssues(designworkflow.StageSchematic, lowerIssues)
	issues = append(issues, lowerIssues...)
	if reports.HasBlockingIssue(lowerIssues) {
		data.Gates = append(data.Gates, preflightGate("schematic_lowering", lowerIssues, designworkflow.StageSchematic))
		return writeCircuitPreflightResult(stdout, data, issues)
	}
	data.Request = &request
	data.Gates = append(data.Gates, preflightGate("schematic_lowering", lowerIssues, designworkflow.StageSchematic))

	schematicIssues := schematicir.Validate(request.ExplicitCircuit.Schematic)
	schematicIssues = preflightIssues(designworkflow.StageSchematicElectrical, schematicIssues)
	data.SchematicIssues = schematicIssues
	issues = append(issues, schematicIssues...)
	data.Gates = append(data.Gates, preflightGate("schematic_electrical_and_readability", schematicIssues, designworkflow.StageSchematicElectrical))
	if reports.HasBlockingIssue(schematicIssues) {
		return writeCircuitPreflightResult(stdout, data, issues)
	}

	placed := designworkflow.PlaceExplicitCircuit(ctx, request, designworkflow.PlacementOptions{LibraryIndex: libraryIndex})
	placementIssues := preflightIssues(designworkflow.StagePlacement, placed.Stage.Issues)
	placed.Stage.Issues = placementIssues
	data.Placement = &placed
	issues = append(issues, placementIssues...)
	data.Gates = append(data.Gates, preflightGate("placement", placementIssues, designworkflow.StagePlacement))
	if reports.HasBlockingIssue(placementIssues) || placed.Stage.Status == designworkflow.StageStatusSkipped {
		return writeCircuitPreflightResult(stdout, data, issues)
	}

	routed := designworkflow.RouteExplicitCircuit(ctx, request, placed, designworkflow.RoutingOptions{})
	routingIssues := preflightIssues(designworkflow.StageRouting, routed.Stage.Issues)
	routed.Stage.Issues = routingIssues
	data.Routing = &routed
	issues = append(issues, routingIssues...)
	data.Gates = append(data.Gates, preflightGate("required_net_routing", routingIssues, designworkflow.StageRouting))
	if reports.HasBlockingIssue(routingIssues) || routed.Stage.Status == designworkflow.StageStatusSkipped {
		return writeCircuitPreflightResult(stdout, data, issues)
	}

	data.Gates = append(data.Gates, circuitPreflightGate{Name: "kicad_erc", Status: externalEvidenceStatus(request.Validation.RequireERC), External: request.Validation.RequireERC})
	data.Gates = append(data.Gates, circuitPreflightGate{Name: "kicad_drc", Status: externalEvidenceStatus(request.Validation.RequireDRC), External: request.Validation.RequireDRC})
	data.ReadyForWrite = !reports.HasBlockingIssue(issues)
	return writeCircuitPreflightResult(stdout, data, issues)
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
