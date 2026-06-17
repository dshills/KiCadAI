package verification

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"path/filepath"
	"slices"
	"strings"

	"kicadai/internal/blocks"
	"kicadai/internal/reports"
	"kicadai/internal/transactions"
	"kicadai/internal/writercorrectness"
)

type Status string

const (
	StatusPass    Status = "pass"
	StatusWarning Status = "warning"
	StatusBlocked Status = "blocked"
	StatusSkipped Status = "skipped"
)

const (
	defaultPlacementToleranceMM  = 0.001
	defaultPlacementToleranceDeg = 0.1
)

type RunOptions struct {
	Registry      blocks.Registry
	Strict        bool
	OutputDir     string
	Overwrite     bool
	KeepArtifacts bool
	WriterOptions writercorrectness.Options
}

type RunResult struct {
	CaseID        string              `json:"case_id"`
	BlockID       string              `json:"block_id"`
	EvidenceLevel EvidenceLevel       `json:"evidence_level"`
	Status        Status              `json:"status"`
	Stages        []StageResult       `json:"stages"`
	Output        *blocks.BlockOutput `json:"output,omitempty"`
	Issues        []reports.Issue     `json:"issues,omitempty"`
	Artifacts     []reports.Artifact  `json:"artifacts,omitempty"`
}

type StageResult struct {
	Name    string          `json:"name"`
	Status  Status          `json:"status"`
	Issues  []reports.Issue `json:"issues,omitempty"`
	Summary string          `json:"summary,omitempty"`
}

type semanticSummary struct {
	Components map[string]actualComponent
	Nets       map[string][]actualPin
	Ports      map[string]blocks.BlockPort
	PCB        actualPCB
}

type actualComponent struct {
	Role        string
	Ref         string
	SymbolID    string
	FootprintID string
	Value       string
}

type actualPin struct {
	Ref string
	Pin string
}

type actualPCB struct {
	Placements map[string]actualPlacement
	PadNets    map[padKey]string
	Routes     map[string]struct{}
	ZoneNames  map[string]struct{}
	ZoneNets   map[string]struct{}
}

type actualPlacement struct {
	Ref         string
	Role        string
	FootprintID string
	XMM         float64
	YMM         float64
	RotationDeg float64
}

type padKey struct {
	Ref string
	Pad string
}

type connectEdge struct {
	NetName string
	From    actualPin
	To      actualPin
}

func RunCase(ctx context.Context, manifest Manifest, opts RunOptions) RunResult {
	registry := opts.Registry
	if registry == nil {
		registry = blocks.NewBuiltinRegistry()
	}
	result := RunResult{
		CaseID:        manifest.ID,
		BlockID:       manifest.BlockID,
		EvidenceLevel: manifest.Expected.EvidenceLevel,
		Status:        StatusPass,
	}
	manifestIssues := ValidateManifest(manifest, registry)
	result.addStage(StageResult{Name: "manifest", Issues: manifestIssues, Summary: "validated manifest"})
	if reports.HasBlockingIssue(manifestIssues) {
		result.finish()
		return result
	}
	request := blocks.BlockRequest{BlockID: manifest.BlockID, InstanceID: requestInstanceID(manifest), Params: manifest.Request.Params}
	output, instantiateIssues := registry.Instantiate(ctx, request)
	result.Output = &output
	instantiateIssues = append(instantiateIssues, output.Issues...)
	result.addStage(StageResult{Name: "instantiate", Issues: instantiateIssues, Summary: fmt.Sprintf("generated %d operation(s)", len(output.Operations))})
	if reports.HasBlockingIssue(instantiateIssues) {
		result.finish()
		return result
	}
	summary, summaryIssues := summarizeOutput(output)
	semanticIssues := append(summaryIssues, assertSemantics(manifest, summary, opts)...)
	result.addStage(StageResult{Name: "semantic_assertions", Issues: semanticIssues, Summary: "checked expected components, ports, nets, and pins"})
	if reports.HasBlockingIssue(semanticIssues) {
		result.finish()
		return result
	}
	if pcbAssertionsRequested(manifest.Expected) {
		pcbIssues := assertPCB(manifest, summary)
		result.addStage(StageResult{Name: "pcb_assertions", Issues: pcbIssues, Summary: "checked expected placements, pad nets, routes, and zones"})
		if reports.HasBlockingIssue(pcbIssues) {
			result.finish()
			return result
		}
	}
	if writerRequested(manifest.Expected.Writer) {
		stage, artifacts := runWriterStage(ctx, manifest, &output, opts)
		result.Artifacts = append(result.Artifacts, artifacts...)
		result.addStage(stage)
	}
	result.finish()
	return result
}

func (result *RunResult) addStage(stage StageResult) {
	if stage.Status == "" {
		stage.Status = statusForIssues(stage.Issues)
	}
	result.Stages = append(result.Stages, stage)
	result.Issues = append(result.Issues, stage.Issues...)
}

func (result *RunResult) finish() {
	SortIssues(result.Issues)
	result.Status = StatusPass
	for _, stage := range result.Stages {
		switch stage.Status {
		case StatusBlocked:
			result.Status = StatusBlocked
		case StatusWarning:
			if result.Status == StatusPass {
				result.Status = StatusWarning
			}
		}
	}
}

func writerRequested(writer ExpectedWriter) bool {
	return writer.Required || writer.OK || writer.AllowUnrouted || writer.RequireRoundTrip
}

func runWriterStage(ctx context.Context, manifest Manifest, output *blocks.BlockOutput, opts RunOptions) (StageResult, []reports.Artifact) {
	if strings.TrimSpace(opts.OutputDir) == "" {
		if manifest.Expected.Writer.Required {
			return StageResult{
				Name:   "writer_correctness",
				Issues: []reports.Issue{writerRunIssue(manifest, "output_dir", "writer verification requires an output directory")},
			}, nil
		}
		return StageResult{Name: "writer_correctness", Status: StatusSkipped, Summary: "writer verification skipped because no output directory was provided"}, nil
	}
	projectName := pathID(manifest.ID)
	projectDir := caseOutputDir(opts.OutputDir, manifest.ID)
	tx, err := blocks.ProjectTransactionForBlockOutputPtr(projectName, output, opts.Overwrite)
	if err != nil {
		return StageResult{
			Name:    "writer_correctness",
			Issues:  []reports.Issue{writerRunIssue(manifest, "transaction", err.Error())},
			Summary: "failed to build project transaction",
		}, nil
	}
	apply := transactions.Apply(tx, transactions.ApplyOptions{OutputDir: projectDir, Overwrite: opts.Overwrite})
	issues := contextualizeIssues(manifest, "apply", apply.Issues)
	artifacts := apply.Artifacts
	if reports.HasBlockingIssue(issues) {
		return StageResult{Name: "writer_correctness", Issues: issues, Summary: "failed to write generated project"}, artifacts
	}

	writerOptions := opts.WriterOptions
	writerOptions.AllowUnrouted = writerOptions.AllowUnrouted || manifest.Expected.Writer.AllowUnrouted
	writerOptions.RequireKiCadRoundTrip = writerOptions.RequireKiCadRoundTrip || manifest.Expected.Writer.RequireRoundTrip
	writerOptions.KeepArtifacts = writerOptions.KeepArtifacts || opts.KeepArtifacts
	writerResult := writercorrectness.Validate(ctx, projectDir, writerOptions)
	artifacts = append(artifacts, writerResult.Artifacts...)
	writerIssues := contextualizeIssues(manifest, "writer", writerResult.Issues)
	issues = append(issues, writerIssues...)
	if manifest.Expected.Writer.OK && !writerResult.OK && len(writerIssues) == 0 {
		issues = append(issues, writerRunIssue(manifest, "ok", fmt.Sprintf("writer correctness did not report OK; checks=%d failures=%d warnings=%d skipped=%d", len(writerResult.Checks), writerResult.OverallSummary.FailCount, writerResult.OverallSummary.WarningCount, writerResult.OverallSummary.SkippedCount)))
	}
	summary := fmt.Sprintf("wrote %s and ran %d writer correctness check(s)", projectDir, len(writerResult.Checks))
	return StageResult{Name: "writer_correctness", Issues: issues, Summary: summary}, artifacts
}

func caseOutputDir(root string, caseID string) string {
	return filepath.Join(root, pathID(caseID))
}

func writerRunIssue(manifest Manifest, path string, message string) reports.Issue {
	issue := runIssue("verification."+pathID(manifest.ID)+".writer."+pathSegment(path), message)
	issue.Suggestion = "case " + manifest.ID + " block " + manifest.BlockID
	return issue
}

func contextualizeIssues(manifest Manifest, stage string, issues []reports.Issue) []reports.Issue {
	if len(issues) == 0 {
		return nil
	}
	contextualized := make([]reports.Issue, 0, len(issues))
	prefix := "verification." + pathID(manifest.ID) + "." + pathSegment(stage)
	for _, issue := range issues {
		if strings.TrimSpace(issue.Path) == "" {
			issue.Path = prefix
		} else {
			issue.Path = prefix + "." + strings.TrimPrefix(issue.Path, ".")
		}
		if issue.Suggestion == "" {
			issue.Suggestion = "case " + manifest.ID + " block " + manifest.BlockID
		}
		contextualized = append(contextualized, issue)
	}
	return contextualized
}

func statusForIssues(issues []reports.Issue) Status {
	status := StatusPass
	for _, issue := range issues {
		if issue.Blocking() {
			return StatusBlocked
		}
		if issue.Severity == reports.SeverityError {
			return StatusBlocked
		}
		if issue.Severity == reports.SeverityWarning {
			status = StatusWarning
		}
	}
	return status
}

func summarizeOutput(output blocks.BlockOutput) (semanticSummary, []reports.Issue) {
	summary := semanticSummary{
		Components: map[string]actualComponent{},
		Nets:       map[string][]actualPin{},
		Ports:      map[string]blocks.BlockPort{},
		PCB: actualPCB{
			Placements: map[string]actualPlacement{},
			PadNets:    map[padKey]string{},
			Routes:     map[string]struct{}{},
			ZoneNames:  map[string]struct{}{},
			ZoneNets:   map[string]struct{}{},
		},
	}
	for _, port := range output.Instance.Ports {
		summary.Ports[port.Name] = port
	}
	var issues []reports.Issue
	var connects []connectEdge
	for index, operation := range output.Operations {
		switch operation.Op {
		case transactions.OpAddSymbol:
			var payload transactions.AddSymbolOperation
			if err := decodeOperation(operation, &payload); err != nil {
				issues = append(issues, runIssue("verification.operations", fmt.Sprintf("decode add_symbol operation %d: %v", index, err)))
				continue
			}
			component := summary.Components[payload.Ref]
			component.Ref = payload.Ref
			component.Role = payload.Role
			component.SymbolID = payload.LibraryID
			component.Value = payload.Value
			summary.Components[payload.Ref] = component
		case transactions.OpAssignFootprint:
			var payload transactions.AssignFootprintOperation
			if err := decodeOperation(operation, &payload); err != nil {
				issues = append(issues, runIssue("verification.operations", fmt.Sprintf("decode assign_footprint operation %d: %v", index, err)))
				continue
			}
			component := summary.Components[payload.Ref]
			component.Ref = payload.Ref
			if component.Role == "" {
				component.Role = payload.Role
			}
			component.FootprintID = payload.FootprintID
			summary.Components[payload.Ref] = component
		case transactions.OpConnect:
			var payload transactions.ConnectOperation
			if err := decodeOperation(operation, &payload); err != nil {
				issues = append(issues, runIssue("verification.operations", fmt.Sprintf("decode connect operation %d: %v", index, err)))
				continue
			}
			connects = append(connects, connectEdge{
				NetName: strings.TrimSpace(payload.NetName),
				From:    actualPin{Ref: payload.From.Ref, Pin: payload.From.Pin},
				To:      actualPin{Ref: payload.To.Ref, Pin: payload.To.Pin},
			})
		case transactions.OpPlaceFootprint:
			var payload transactions.PlaceFootprintOperation
			if err := decodeOperation(operation, &payload); err != nil {
				issues = append(issues, runIssue("verification.operations", fmt.Sprintf("decode place_footprint operation %d: %v", index, err)))
				continue
			}
			ref := strings.TrimSpace(payload.Ref)
			if ref == "" {
				issues = append(issues, runIssue("verification.operations", fmt.Sprintf("place_footprint operation %d requires ref", index)))
				continue
			}
			placement := actualPlacement{
				Ref:         ref,
				Role:        payload.Role,
				FootprintID: payload.FootprintID,
				XMM:         payload.At.XMM,
				YMM:         payload.At.YMM,
				RotationDeg: payload.Rotation,
			}
			summary.PCB.Placements[ref] = placement
			for _, pad := range payload.Pads {
				if pad.Net == nil {
					continue
				}
				summary.PCB.PadNets[padKey{Ref: ref, Pad: pad.Name}] = *pad.Net
			}
		case transactions.OpRoute:
			var payload transactions.RouteOperation
			if err := decodeOperation(operation, &payload); err != nil {
				issues = append(issues, runIssue("verification.operations", fmt.Sprintf("decode route operation %d: %v", index, err)))
				continue
			}
			netName := strings.TrimSpace(payload.NetName)
			if netName != "" {
				summary.PCB.Routes[netName] = struct{}{}
			}
		case transactions.OpAddZone:
			var payload transactions.AddZoneOperation
			if err := decodeOperation(operation, &payload); err != nil {
				issues = append(issues, runIssue("verification.operations", fmt.Sprintf("decode add_zone operation %d: %v", index, err)))
				continue
			}
			zoneName := strings.TrimSpace(payload.Name)
			if zoneName != "" {
				summary.PCB.ZoneNames[zoneName] = struct{}{}
			}
			if payload.NetName != nil {
				netName := strings.TrimSpace(*payload.NetName)
				if netName != "" {
					summary.PCB.ZoneNets[netName] = struct{}{}
				}
			}
		}
	}
	summary.Nets = summarizeConnects(connects)
	for netName, pins := range summary.Nets {
		summary.Nets[netName] = uniquePins(pins)
	}
	return summary, issues
}

func summarizeConnects(connects []connectEdge) map[string][]actualPin {
	nets := map[string][]actualPin{}
	if len(connects) == 0 {
		return nets
	}
	sets := newPinDisjointSet()
	for _, connect := range connects {
		sets.union(connect.From, connect.To)
	}
	pinsByRoot := map[actualPin][]actualPin{}
	namesByRoot := map[actualPin]map[string]struct{}{}
	for _, connect := range connects {
		root := sets.find(connect.From)
		pinsByRoot[root] = append(pinsByRoot[root], connect.From, connect.To)
		if connect.NetName != "" {
			if namesByRoot[root] == nil {
				namesByRoot[root] = map[string]struct{}{}
			}
			namesByRoot[root][connect.NetName] = struct{}{}
		}
	}
	anonymousRoots := make([]actualPin, 0, len(pinsByRoot))
	for root, pins := range pinsByRoot {
		pins = uniquePins(pins)
		names := sortedSetValues(namesByRoot[root])
		if len(names) == 0 {
			anonymousRoots = append(anonymousRoots, root)
			pinsByRoot[root] = pins
			continue
		}
		for _, name := range names {
			nets[name] = append(nets[name], pins...)
		}
	}
	slices.SortFunc(anonymousRoots, func(a, b actualPin) int {
		return comparePins(a, b)
	})
	for index, root := range anonymousRoots {
		nets[fmt.Sprintf("__anonymous_net_%d", index)] = pinsByRoot[root]
	}
	return nets
}

func assertSemantics(manifest Manifest, summary semanticSummary, opts RunOptions) []reports.Issue {
	var issues []reports.Issue
	roleToComponents := componentsByRole(summary.Components)
	componentIssues, matchedRefs := assertExpectedComponents(manifest, summary, roleToComponents)
	issues = append(issues, componentIssues...)
	for _, expected := range manifest.Expected.Ports {
		path := "verification." + pathID(manifest.ID) + ".ports." + pathSegment(expected.Name)
		port, ok := summary.Ports[expected.Name]
		if !ok {
			issues = append(issues, runIssue(path, "missing expected port "+expected.Name))
			continue
		}
		if expected.Direction != "" && string(port.Direction) != expected.Direction {
			issues = append(issues, runIssue(path+".direction", fmt.Sprintf("expected direction %s, got %s", expected.Direction, port.Direction)))
		}
	}
	for _, expected := range manifest.Expected.Nets {
		path := "verification." + pathID(manifest.ID) + ".nets." + pathSegment(expected.Name)
		actualPins, ok := summary.Nets[expected.Name]
		if !ok {
			issues = append(issues, runIssue(path, "missing expected net "+expected.Name))
			continue
		}
		actualPinSet := pinSet(actualPins)
		for _, expectedPin := range expected.Pins {
			ref := expectedPin.Ref
			if ref == "" && expectedPin.Role != "" {
				if !rolePinInNet(actualPinSet, roleToComponents[expectedPin.Role], expectedPin.Pin) {
					issues = append(issues, runIssue(path+".pins."+pathSegment(expectedPin.Role), fmt.Sprintf("expected role %s pin %s on net %s", expectedPin.Role, expectedPin.Pin, expected.Name)))
				}
				continue
			}
			if _, ok := actualPinSet[actualPin{Ref: ref, Pin: expectedPin.Pin}]; !ok {
				issues = append(issues, runIssue(path+".pins."+pathSegment(ref)+"."+pathSegment(expectedPin.Pin), fmt.Sprintf("expected pin %s:%s on net %s", ref, expectedPin.Pin, expected.Name)))
			}
		}
	}
	if opts.Strict {
		issues = append(issues, assertStrictSemantics(manifest, summary, matchedRefs)...)
	}
	return issues
}

func pcbAssertionsRequested(expected Expected) bool {
	if expected.EvidenceLevel == EvidencePCBVerified || expected.EvidenceLevel == EvidenceERCDRCVerified || expected.EvidenceLevel == EvidenceReferenceVerified {
		return true
	}
	return len(expected.PCB.Placements) > 0 ||
		len(expected.PCB.PadNets) > 0 ||
		len(expected.PCB.RequiredRoutes) > 0 ||
		len(expected.PCB.RequiredZones) > 0 ||
		expected.PCB.RequireRoutes ||
		expected.PCB.RequireZones
}

func assertPCB(manifest Manifest, summary semanticSummary) []reports.Issue {
	var issues []reports.Issue
	basePath := "verification." + pathID(manifest.ID) + ".pcb"
	roleToPlacements := placementsByRole(summary.PCB.Placements)
	matchedPlacements := map[string]struct{}{}
	for _, item := range orderedExpectedPlacements(manifest.Expected.PCB.Placements) {
		index := item.Index
		expected := item.Placement
		path := fmt.Sprintf("%s.placements.%d", basePath, index)
		placement, ok := matchPlacement(expected, summary.PCB.Placements, roleToPlacements, matchedPlacements)
		if !ok {
			if candidate, candidateOK := placementCandidate(expected, summary.PCB.Placements, roleToPlacements, matchedPlacements); candidateOK {
				issues = append(issues, runIssue(path, placementMismatchMessage(expected, candidate)))
				continue
			}
			issues = append(issues, runIssue(path, "missing expected PCB placement"))
			continue
		}
		matchedPlacements[placement.Ref] = struct{}{}
	}
	for _, expected := range manifest.Expected.PCB.PadNets {
		path := basePath + ".pad_nets." + pathSegment(expected.Ref) + "." + pathSegment(expected.Pad)
		net, ok := summary.PCB.PadNets[padKey{Ref: expected.Ref, Pad: expected.Pad}]
		if !ok {
			issues = append(issues, runIssue(path, fmt.Sprintf("missing expected pad net %s:%s=%s", expected.Ref, expected.Pad, expected.Net)))
			continue
		}
		if net != expected.Net {
			issues = append(issues, runIssue(path, fmt.Sprintf("expected pad net %s, got %s", expected.Net, net)))
		}
	}
	if manifest.Expected.PCB.RequireRoutes && len(summary.PCB.Routes) == 0 {
		issues = append(issues, runIssue(basePath+".routes", "expected at least one route"))
	}
	for _, netName := range manifest.Expected.PCB.RequiredRoutes {
		netName = strings.TrimSpace(netName)
		if _, ok := summary.PCB.Routes[netName]; !ok {
			issues = append(issues, runIssue(basePath+".routes."+pathSegment(netName), "missing required route "+netName))
		}
	}
	if manifest.Expected.PCB.RequireZones && len(summary.PCB.ZoneNames) == 0 && len(summary.PCB.ZoneNets) == 0 {
		issues = append(issues, runIssue(basePath+".zones", "expected at least one zone"))
	}
	for _, zone := range manifest.Expected.PCB.RequiredZones {
		zone = strings.TrimSpace(zone)
		_, nameOK := summary.PCB.ZoneNames[zone]
		_, netOK := summary.PCB.ZoneNets[zone]
		if !nameOK && !netOK {
			issues = append(issues, runIssue(basePath+".zones."+pathSegment(zone), "missing required zone "+zone))
		}
	}
	return issues
}

type expectedPlacementItem struct {
	Index     int
	Placement ExpectedPlacement
	Score     int
}

func orderedExpectedPlacements(placements []ExpectedPlacement) []expectedPlacementItem {
	items := make([]expectedPlacementItem, 0, len(placements))
	for index, placement := range placements {
		items = append(items, expectedPlacementItem{
			Index:     index,
			Placement: placement,
			Score:     placementSpecificity(placement),
		})
	}
	slices.SortStableFunc(items, func(a, b expectedPlacementItem) int {
		return b.Score - a.Score
	})
	return items
}

func placementSpecificity(placement ExpectedPlacement) int {
	score := 0
	if strings.TrimSpace(placement.Ref) != "" {
		score += 100
	}
	if strings.TrimSpace(placement.FootprintID) != "" {
		score += 20
	}
	if placement.XMM != nil {
		score += 10
	}
	if placement.YMM != nil {
		score += 10
	}
	if placement.RotationDeg != nil {
		score += 10
	}
	return score
}

func placementsByRole(placements map[string]actualPlacement) map[string][]actualPlacement {
	byRole := map[string][]actualPlacement{}
	for _, placement := range placements {
		byRole[placement.Role] = append(byRole[placement.Role], placement)
	}
	for role := range byRole {
		slices.SortFunc(byRole[role], func(a, b actualPlacement) int {
			return strings.Compare(a.Ref, b.Ref)
		})
	}
	return byRole
}

func matchPlacement(expected ExpectedPlacement, placements map[string]actualPlacement, roleToPlacements map[string][]actualPlacement, used map[string]struct{}) (actualPlacement, bool) {
	expectedRef := strings.TrimSpace(expected.Ref)
	if expectedRef != "" {
		placement, ok := placements[expectedRef]
		if !ok {
			return actualPlacement{}, false
		}
		if expected.Role != "" && placement.Role != expected.Role {
			return actualPlacement{}, false
		}
		if _, alreadyUsed := used[placement.Ref]; alreadyUsed {
			return actualPlacement{}, false
		}
		if !placementMatchesExpected(placement, expected) {
			return actualPlacement{}, false
		}
		return placement, ok
	}
	matches := roleToPlacements[expected.Role]
	for _, placement := range matches {
		if _, alreadyUsed := used[placement.Ref]; alreadyUsed {
			continue
		}
		if !expectedPlacementHasConstraints(expected) || placementMatchesExpected(placement, expected) {
			return placement, true
		}
	}
	return actualPlacement{}, false
}

func placementCandidate(expected ExpectedPlacement, placements map[string]actualPlacement, roleToPlacements map[string][]actualPlacement, used map[string]struct{}) (actualPlacement, bool) {
	expectedRef := strings.TrimSpace(expected.Ref)
	if expectedRef != "" {
		placement, ok := placements[expectedRef]
		if !ok {
			return actualPlacement{}, false
		}
		if _, alreadyUsed := used[placement.Ref]; alreadyUsed {
			return actualPlacement{}, false
		}
		return placement, true
	}
	for _, placement := range roleToPlacements[expected.Role] {
		if _, alreadyUsed := used[placement.Ref]; alreadyUsed {
			continue
		}
		return placement, true
	}
	return actualPlacement{}, false
}

func placementMismatchMessage(expected ExpectedPlacement, placement actualPlacement) string {
	role := expected.Role
	if role == "" {
		role = "any"
	}
	footprint := expected.FootprintID
	if footprint == "" {
		footprint = "any"
	}
	actualFootprint := placement.FootprintID
	if actualFootprint == "" {
		actualFootprint = "none"
	}
	return fmt.Sprintf(
		"expected placement role %s footprint %s at %s,%s rot %s got ref %s role %s footprint %s at %.6f,%.6f rot %.6f",
		role,
		footprint,
		expectedFloatString(expected.XMM),
		expectedFloatString(expected.YMM),
		expectedFloatString(expected.RotationDeg),
		placement.Ref,
		placement.Role,
		actualFootprint,
		placement.XMM,
		placement.YMM,
		placement.RotationDeg,
	)
}

func placementMatchesExpected(placement actualPlacement, expected ExpectedPlacement) bool {
	if expected.FootprintID != "" && placement.FootprintID != expected.FootprintID {
		return false
	}
	toleranceMM, toleranceDeg := placementTolerances(expected)
	if expected.XMM != nil && math.Abs(placement.XMM-*expected.XMM) > toleranceMM {
		return false
	}
	if expected.YMM != nil && math.Abs(placement.YMM-*expected.YMM) > toleranceMM {
		return false
	}
	if expected.RotationDeg != nil && angleDeltaDeg(placement.RotationDeg, *expected.RotationDeg) > toleranceDeg {
		return false
	}
	return true
}

func placementTolerances(expected ExpectedPlacement) (float64, float64) {
	tolerance := defaultPlacementToleranceMM
	if expected.ToleranceMM != nil {
		tolerance = *expected.ToleranceMM
	}
	rotationTolerance := defaultPlacementToleranceDeg
	if expected.ToleranceDeg != nil {
		rotationTolerance = *expected.ToleranceDeg
	}
	return tolerance, rotationTolerance
}

func expectedPlacementHasConstraints(expected ExpectedPlacement) bool {
	return expected.FootprintID != "" || expected.XMM != nil || expected.YMM != nil || expected.RotationDeg != nil
}

func expectedFloatString(value *float64) string {
	if value == nil {
		return "any"
	}
	return fmt.Sprintf("%.6f", *value)
}

func angleDeltaDeg(a float64, b float64) float64 {
	diff := math.Mod(math.Abs(a-b), 360)
	if diff > 180 {
		return 360 - diff
	}
	return diff
}

func assertExpectedComponents(manifest Manifest, summary semanticSummary, roleToComponents map[string][]actualComponent) ([]reports.Issue, map[string]struct{}) {
	var issues []reports.Issue
	matchedRefs := map[string]struct{}{}
	for index, expected := range manifest.Expected.Components {
		if expected.Ref == "" {
			continue
		}
		issues = append(issues, assertExpectedComponent(manifest, index, expected, summary.Components, roleToComponents, matchedRefs)...)
	}
	for index, expected := range manifest.Expected.Components {
		if expected.Ref != "" {
			continue
		}
		issues = append(issues, assertExpectedComponent(manifest, index, expected, summary.Components, roleToComponents, matchedRefs)...)
	}
	return issues, matchedRefs
}

func assertExpectedComponent(manifest Manifest, index int, expected ExpectedComponent, components map[string]actualComponent, roleToComponents map[string][]actualComponent, matchedRefs map[string]struct{}) []reports.Issue {
	var issues []reports.Issue
	component, ok := matchComponent(expected, components, roleToComponents, matchedRefs)
	path := expectedComponentIssuePath(manifest, index, expected)
	if !ok {
		issues = append(issues, runIssue(path, "missing expected component "+expectedComponentPathID(expected)))
		return issues
	}
	if expected.Role != "" && component.Role != expected.Role {
		issues = append(issues, runIssue(path+".role", fmt.Sprintf("expected role %s, got %s", expected.Role, component.Role)))
	}
	if expected.SymbolID != "" && component.SymbolID != expected.SymbolID {
		issues = append(issues, runIssue(path+".symbol_id", fmt.Sprintf("expected symbol %s, got %s", expected.SymbolID, component.SymbolID)))
	}
	if expected.FootprintID != "" && component.FootprintID != expected.FootprintID {
		issues = append(issues, runIssue(path+".footprint_id", fmt.Sprintf("expected footprint %s, got %s", expected.FootprintID, component.FootprintID)))
	}
	if expected.RefPrefix != "" && !strings.HasPrefix(component.Ref, expected.RefPrefix) {
		issues = append(issues, runIssue(path+".ref_prefix", fmt.Sprintf("expected ref prefix %s, got %s", expected.RefPrefix, component.Ref)))
	}
	if expected.Value != "" && component.Value != expected.Value {
		issues = append(issues, runIssue(path+".value", fmt.Sprintf("expected value %s, got %s", expected.Value, component.Value)))
	}
	return issues
}

func expectedComponentPathID(expected ExpectedComponent) string {
	if strings.TrimSpace(expected.Role) != "" && strings.TrimSpace(expected.Ref) != "" {
		return expected.Role + "." + expected.Ref
	}
	if strings.TrimSpace(expected.Role) != "" {
		return expected.Role
	}
	return expected.Ref
}

func expectedComponentIssuePath(manifest Manifest, index int, expected ExpectedComponent) string {
	path := "verification." + pathID(manifest.ID) + ".components." + pathSegment(expectedComponentPathID(expected))
	if expected.Ref == "" {
		path += fmt.Sprintf(".%d", index)
	}
	return path
}

func assertStrictSemantics(manifest Manifest, summary semanticSummary, matchedRefs map[string]struct{}) []reports.Issue {
	var issues []reports.Issue
	for _, component := range summary.Components {
		if _, ok := matchedRefs[component.Ref]; ok {
			continue
		}
		issues = append(issues, warningIssue("verification."+pathID(manifest.ID)+".components."+pathSegment(component.Ref), "unexpected generated component "+component.Ref))
	}
	expectedPorts := map[string]struct{}{}
	for _, port := range manifest.Expected.Ports {
		expectedPorts[port.Name] = struct{}{}
	}
	for portName := range summary.Ports {
		if _, ok := expectedPorts[portName]; !ok {
			issues = append(issues, warningIssue("verification."+pathID(manifest.ID)+".ports."+pathSegment(portName), "unexpected generated port "+portName))
		}
	}
	expectedNetNames := expectedNetNameSet(manifest.Expected.Nets)
	for netName := range summary.Nets {
		if _, ok := expectedNetNames[netName]; !ok {
			issues = append(issues, warningIssue("verification."+pathID(manifest.ID)+".nets."+pathSegment(netName), "unexpected generated net "+netName))
		}
	}
	return issues
}

func matchComponent(expected ExpectedComponent, components map[string]actualComponent, roleToComponents map[string][]actualComponent, matchedRefs map[string]struct{}) (actualComponent, bool) {
	if expected.Ref != "" {
		component, ok := components[expected.Ref]
		if ok {
			if _, used := matchedRefs[component.Ref]; used {
				return actualComponent{}, false
			}
			matchedRefs[component.Ref] = struct{}{}
		}
		return component, ok
	}
	matches := roleToComponents[expected.Role]
	for _, component := range matches {
		if !componentMatchesExpected(component, expected) {
			continue
		}
		if _, used := matchedRefs[component.Ref]; used {
			continue
		}
		matchedRefs[component.Ref] = struct{}{}
		return component, true
	}
	return actualComponent{}, false
}

func componentMatchesExpected(component actualComponent, expected ExpectedComponent) bool {
	return expected.RefPrefix == "" || strings.HasPrefix(component.Ref, expected.RefPrefix)
}

func rolePinInNet(actualPinSet map[actualPin]struct{}, components []actualComponent, pin string) bool {
	for _, component := range components {
		if _, ok := actualPinSet[actualPin{Ref: component.Ref, Pin: pin}]; ok {
			return true
		}
	}
	return false
}

func componentsByRole(components map[string]actualComponent) map[string][]actualComponent {
	byRole := map[string][]actualComponent{}
	for _, component := range components {
		byRole[component.Role] = append(byRole[component.Role], component)
	}
	for role := range byRole {
		slices.SortFunc(byRole[role], func(a, b actualComponent) int {
			if a.Ref < b.Ref {
				return -1
			}
			if a.Ref > b.Ref {
				return 1
			}
			return 0
		})
	}
	return byRole
}

func uniquePins(pins []actualPin) []actualPin {
	seen := map[actualPin]struct{}{}
	unique := make([]actualPin, 0, len(pins))
	for _, pin := range pins {
		if _, ok := seen[pin]; ok {
			continue
		}
		seen[pin] = struct{}{}
		unique = append(unique, pin)
	}
	slices.SortFunc(unique, func(a, b actualPin) int {
		return comparePins(a, b)
	})
	return unique
}

func pinSet(pins []actualPin) map[actualPin]struct{} {
	set := make(map[actualPin]struct{}, len(pins))
	for _, actual := range pins {
		set[actual] = struct{}{}
	}
	return set
}

type pinDisjointSet struct {
	parent map[actualPin]actualPin
}

func newPinDisjointSet() *pinDisjointSet {
	return &pinDisjointSet{parent: map[actualPin]actualPin{}}
}

func (sets *pinDisjointSet) find(pin actualPin) actualPin {
	parent, ok := sets.parent[pin]
	if !ok {
		sets.parent[pin] = pin
		return pin
	}
	root := pin
	for parent != root {
		root = parent
		parent = sets.parent[root]
	}
	for current := pin; sets.parent[current] != root; {
		next := sets.parent[current]
		sets.parent[current] = root
		current = next
	}
	return root
}

func (sets *pinDisjointSet) union(a actualPin, b actualPin) {
	rootA := sets.find(a)
	rootB := sets.find(b)
	if rootA == rootB {
		return
	}
	if comparePins(rootB, rootA) < 0 {
		rootA, rootB = rootB, rootA
	}
	sets.parent[rootB] = rootA
}

func sortedSetValues(values map[string]struct{}) []string {
	if len(values) == 0 {
		return nil
	}
	sorted := make([]string, 0, len(values))
	for value := range values {
		sorted = append(sorted, value)
	}
	slices.Sort(sorted)
	return sorted
}

func comparePins(a actualPin, b actualPin) int {
	if a.Ref < b.Ref {
		return -1
	}
	if a.Ref > b.Ref {
		return 1
	}
	return comparePinNames(a.Pin, b.Pin)
}

func comparePinNames(a string, b string) int {
	aIndex, bIndex := 0, 0
	for aIndex < len(a) && bIndex < len(b) {
		aDigit := isASCIIDigit(a[aIndex])
		bDigit := isASCIIDigit(b[bIndex])
		if aDigit && bDigit {
			aStart, bStart := aIndex, bIndex
			for aIndex < len(a) && isASCIIDigit(a[aIndex]) {
				aIndex++
			}
			for bIndex < len(b) && isASCIIDigit(b[bIndex]) {
				bIndex++
			}
			if compare := compareDigitRuns(a[aStart:aIndex], b[bStart:bIndex]); compare != 0 {
				return compare
			}
			continue
		}
		if a[aIndex] < b[bIndex] {
			return -1
		}
		if a[aIndex] > b[bIndex] {
			return 1
		}
		aIndex++
		bIndex++
	}
	if aIndex < len(a) {
		return 1
	}
	if bIndex < len(b) {
		return -1
	}
	return 0
}

func compareDigitRuns(a string, b string) int {
	aTrimmed := trimLeadingZeros(a)
	bTrimmed := trimLeadingZeros(b)
	if len(aTrimmed) < len(bTrimmed) {
		return -1
	}
	if len(aTrimmed) > len(bTrimmed) {
		return 1
	}
	if aTrimmed < bTrimmed {
		return -1
	}
	if aTrimmed > bTrimmed {
		return 1
	}
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

func trimLeadingZeros(value string) string {
	trimmed := strings.TrimLeft(value, "0")
	if trimmed == "" {
		return "0"
	}
	return trimmed
}

func isASCIIDigit(value byte) bool {
	return value >= '0' && value <= '9'
}

func decodeOperation(operation transactions.Operation, payload any) error {
	if len(operation.Raw) == 0 {
		return fmt.Errorf("operation %s has no raw payload", operation.Op)
	}
	return json.Unmarshal(operation.Raw, payload)
}

func runIssue(path string, message string) reports.Issue {
	return reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Path: path, Message: message}
}

func warningIssue(path string, message string) reports.Issue {
	return reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityWarning, Path: path, Message: message}
}
