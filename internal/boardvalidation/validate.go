package boardvalidation

import (
	"context"
	"fmt"
	"math"
	"path/filepath"
	"slices"
	"strings"

	"kicadai/internal/kicadfiles"
	"kicadai/internal/kicadfiles/checks"
	pcbfiles "kicadai/internal/kicadfiles/pcb"
	"kicadai/internal/reports"
)

const routePointTolerance = kicadfiles.IU(100)

func Validate(ctx context.Context, targetPath string, opts Options) Result {
	result := NewResult(targetPath)
	target, err := ResolveTarget(targetPath)
	if err != nil {
		result.AddCheck(Check{
			Name:     CheckPCBStructuralValidation,
			Required: true,
			Status:   StatusError,
			Issues: []reports.Issue{{
				Code:     reports.CodeInvalidArgument,
				Severity: reports.SeverityError,
				Path:     filepath.ToSlash(targetPath),
				Message:  err.Error(),
			}},
		})
		result.Finish()
		return result
	}
	result.Target = target.InputPath
	result.BoardPath = target.BoardPath
	result.ProjectPath = target.ProjectPath

	board, err := pcbfiles.ReadFile(target.BoardPath)
	if err != nil {
		result.AddCheck(Check{
			Name:     CheckPCBStructuralValidation,
			Required: true,
			Status:   StatusError,
			Issues:   IssuesFromError(err, target.BoardPath),
		})
		result.Finish()
		return result
	}
	normalizeParsedBoard(&board)
	validateBoard(ctx, &result, target, &board, opts)
	result.Finish()
	return result
}

func ValidateBoard(ctx context.Context, board *pcbfiles.PCBFile, target Target, opts Options) Result {
	result := NewResult(target.InputPath)
	result.BoardPath = target.BoardPath
	result.ProjectPath = target.ProjectPath
	if board == nil {
		result.AddCheck(Check{
			Name:     CheckPCBStructuralValidation,
			Required: true,
			Status:   StatusError,
			Issues: []reports.Issue{{
				Code:     reports.CodeInvalidArgument,
				Severity: reports.SeverityError,
				Path:     target.BoardPath,
				Message:  "board is nil",
			}},
		})
		result.Finish()
		return result
	}
	normalized := cloneBoardForValidation(board)
	normalizeParsedBoard(&normalized)
	validateBoard(ctx, &result, target, &normalized, opts)
	result.Finish()
	return result
}

func cloneBoardForValidation(board *pcbfiles.PCBFile) pcbfiles.PCBFile {
	clone := *board
	clone.Footprints = slices.Clone(board.Footprints)
	for footprintIndex := range clone.Footprints {
		clone.Footprints[footprintIndex].Properties = slices.Clone(clone.Footprints[footprintIndex].Properties)
		clone.Footprints[footprintIndex].MetadataProperties = slices.Clone(clone.Footprints[footprintIndex].MetadataProperties)
		clone.Footprints[footprintIndex].Texts = slices.Clone(clone.Footprints[footprintIndex].Texts)
		clone.Footprints[footprintIndex].Pads = slices.Clone(clone.Footprints[footprintIndex].Pads)
		clone.Footprints[footprintIndex].Graphics = slices.Clone(clone.Footprints[footprintIndex].Graphics)
		clone.Footprints[footprintIndex].Models = slices.Clone(clone.Footprints[footprintIndex].Models)
	}
	return clone
}

func validateBoard(ctx context.Context, result *Result, target Target, board *pcbfiles.PCBFile, opts Options) {
	structuralIssues := IssuesFromError(pcbfiles.Validate(*board), target.BoardPath)
	result.AddCheck(Check{Name: CheckPCBStructuralValidation, Required: true, Issues: structuralIssues})

	result.AddCheck(Check{Name: CheckNetToPadValidation, Required: true, Issues: validateNetToPad(board)})

	connectivityIssues := IssuesFromError(pcbfiles.ValidateGeneratedConnectivity(*board), target.BoardPath)
	for index := range connectivityIssues {
		if connectivityIssues[index].Code == reports.CodeValidationFailed {
			connectivityIssues[index].Code = reports.CodeDisconnectedPad
		}
		connectivityIssues[index].Suggestion = appendRepairCategory(connectivityIssues[index].Suggestion, "connectivity")
	}
	result.AddCheck(Check{Name: CheckGeneratedConnectivity, Required: true, Issues: connectivityIssues})

	graph := buildBoardConnectivity(board)
	netStatuses, unroutedIssues := graph.netStatuses(opts)
	result.Nets = netStatuses
	result.AddCheck(Check{Name: CheckUnroutedNetValidation, Required: true, Issues: unroutedIssues})
	result.AddCheck(Check{Name: CheckRouteCompletion, Required: true, Issues: validateRouteCompletion(board, graph)})

	zoneStatuses, zoneIssues := validateZones(board, opts)
	result.Zones = zoneStatuses
	result.AddCheck(Check{Name: CheckZoneValidation, Required: true, Issues: zoneIssues})
	result.AddCheck(runDRCCheck(ctx, target, opts))
}

func normalizeParsedBoard(board *pcbfiles.PCBFile) {
	for footprintIndex := range board.Footprints {
		footprint := &board.Footprints[footprintIndex]
		if strings.TrimSpace(footprint.Path) == "" {
			uuidText := ""
			if footprint.UUID.Valid() {
				uuidText = string(footprint.UUID)
			}
			footprint.Path = "/" + firstNonEmpty(uuidText, footprint.Reference, fmt.Sprintf("footprint-%d", footprintIndex))
		}
		for propertyIndex := range footprint.Properties {
			property := &footprint.Properties[propertyIndex]
			if property.Layer == "" {
				backSide := footprint.Layer == kicadfiles.LayerBCu
				switch property.Name {
				case "Reference":
					if backSide {
						property.Layer = kicadfiles.LayerBSilkS
					} else {
						property.Layer = kicadfiles.LayerFSilkS
					}
				default:
					if backSide {
						property.Layer = kicadfiles.LayerBFab
					} else {
						property.Layer = kicadfiles.LayerFFab
					}
				}
			}
		}
	}
}

func validateNetToPad(board *pcbfiles.PCBFile) []reports.Issue {
	netNames := map[int]string{}
	for _, net := range board.Nets {
		netNames[net.Code] = net.Name
	}
	var issues []reports.Issue
	for footprintIndex := range board.Footprints {
		footprint := &board.Footprints[footprintIndex]
		seenPads := map[string]int{}
		for padIndex, pad := range footprint.Pads {
			path := fmt.Sprintf("footprints.%d.pads.%d", footprintIndex, padIndex)
			ref := firstNonEmpty(footprint.Reference, footprint.LibraryID)
			if _, ok := netNames[pad.NetCode]; !ok {
				issues = append(issues, reports.Issue{
					Code:       reports.CodeInvalidNetAssignment,
					Severity:   reports.SeverityError,
					Path:       path + ".net_code",
					Message:    fmt.Sprintf("pad %s/%s uses unknown net code %d", ref, pad.Name, pad.NetCode),
					Refs:       []string{ref},
					Suggestion: "repair category: net_assignment",
				})
			}
			if strings.TrimSpace(pad.NetName) != "" && pad.NetName != netNames[pad.NetCode] {
				issues = append(issues, reports.Issue{
					Code:       reports.CodeInvalidNetAssignment,
					Severity:   reports.SeverityError,
					Path:       path + ".net_name",
					Message:    fmt.Sprintf("pad %s/%s net name %q does not match net code %d", ref, pad.Name, pad.NetName, pad.NetCode),
					Refs:       []string{ref},
					Nets:       []string{pad.NetName, netNames[pad.NetCode]},
					Suggestion: "repair category: net_assignment",
				})
			}
			if firstIndex, ok := seenPads[pad.Name]; ok && !footprintAllowsDuplicatePads(footprint) {
				issues = append(issues, reports.Issue{
					Code:       reports.CodeValidationFailed,
					Severity:   reports.SeverityError,
					Path:       path + ".name",
					Message:    fmt.Sprintf("duplicate pad name %q also appears at pads.%d", pad.Name, firstIndex),
					Refs:       []string{ref},
					Suggestion: "repair category: footprint",
				})
			}
			seenPads[pad.Name] = padIndex
		}
	}
	return issues
}

func footprintAllowsDuplicatePads(footprint *pcbfiles.Footprint) bool {
	if footprint.DuplicatePadNumbersAreJumpers != nil && *footprint.DuplicatePadNumbersAreJumpers {
		return true
	}
	return len(footprint.NetTiePadGroups) > 0
}

type boardConnectivity struct {
	netNames       map[int]string
	netPads        map[int][]connectivityPad
	netCopperCount map[int]int
	netZoneCount   map[int]int
	routeAnchors   map[int]map[pointKey][]string
}

type connectivityPad struct {
	ref   string
	name  string
	point kicadfiles.Point
}

type pointKey struct {
	x int64
	y int64
}

func buildBoardConnectivity(board *pcbfiles.PCBFile) boardConnectivity {
	graph := boardConnectivity{
		netNames:       map[int]string{},
		netPads:        map[int][]connectivityPad{},
		netCopperCount: map[int]int{},
		netZoneCount:   map[int]int{},
		routeAnchors:   map[int]map[pointKey][]string{},
	}
	for _, net := range board.Nets {
		graph.netNames[net.Code] = net.Name
	}
	for footprintIndex := range board.Footprints {
		footprint := &board.Footprints[footprintIndex]
		ref := firstNonEmpty(footprint.Reference, footprint.LibraryID)
		for _, pad := range footprint.Pads {
			if pad.NetCode == 0 || pad.Type == "np_thru_hole" {
				continue
			}
			padPoint := absolutePadPosition(footprint, pad)
			graph.netPads[pad.NetCode] = append(graph.netPads[pad.NetCode], connectivityPad{
				ref:   ref,
				name:  pad.Name,
				point: padPoint,
			})
			graph.addRouteAnchor(pad.NetCode, padPoint, "pad")
		}
	}
	for _, track := range board.Tracks {
		if track.NetCode == 0 {
			continue
		}
		graph.netCopperCount[track.NetCode]++
		graph.addRouteAnchor(track.NetCode, track.Start, "track")
		graph.addRouteAnchor(track.NetCode, track.End, "track")
	}
	for _, arc := range board.TrackArcs {
		if arc.NetCode == 0 {
			continue
		}
		graph.netCopperCount[arc.NetCode]++
		graph.addRouteAnchor(arc.NetCode, arc.Start, "track_arc")
		graph.addRouteAnchor(arc.NetCode, arc.End, "track_arc")
	}
	for _, via := range board.Vias {
		if via.NetCode == 0 {
			continue
		}
		graph.netCopperCount[via.NetCode]++
		graph.addRouteAnchor(via.NetCode, via.Position, "via")
	}
	for _, zone := range board.Zones {
		if zone.NetCode > 0 {
			graph.netZoneCount[zone.NetCode]++
		}
	}
	return graph
}

func (graph boardConnectivity) addRouteAnchor(netCode int, point kicadfiles.Point, kind string) {
	if graph.routeAnchors[netCode] == nil {
		graph.routeAnchors[netCode] = map[pointKey][]string{}
	}
	graph.routeAnchors[netCode][newPointKey(point)] = append(graph.routeAnchors[netCode][newPointKey(point)], kind)
}

func (graph boardConnectivity) netStatuses(opts Options) ([]NetStatus, []reports.Issue) {
	netCodes := make([]int, 0, len(graph.netNames))
	for code := range graph.netNames {
		if code > 0 {
			netCodes = append(netCodes, code)
		}
	}
	slices.Sort(netCodes)
	var statuses []NetStatus
	var issues []reports.Issue
	for _, code := range netCodes {
		pads := graph.netPads[code]
		copperCount := graph.netCopperCount[code]
		zoneCount := graph.netZoneCount[code]
		status := NetStatusIgnored
		switch {
		case len(pads) == 0:
			status = NetStatusIgnored
		case len(pads) == 1:
			status = NetStatusSingleEndpoint
		case graph.allPadsHaveRouteAnchor(code, pads):
			status = NetStatusFullyRouted
		case copperCount == 0 && zoneCount > 0:
			status = NetStatusZoneDependent
		case copperCount == 0:
			status = NetStatusUnconnected
		default:
			status = NetStatusPartiallyRouted
		}
		row := NetStatus{
			Code:        code,
			Name:        graph.netNames[code],
			Status:      status,
			PadCount:    len(pads),
			CopperCount: copperCount,
			Refs:        refsForPads(pads),
		}
		if status == NetStatusUnconnected || status == NetStatusPartiallyRouted {
			issueCode := reports.CodeDisconnectedPad
			row.IssueCodes = []string{string(issueCode)}
			issues = append(issues, reports.Issue{
				Code:       issueCode,
				Severity:   reports.SeverityError,
				Path:       "nets." + graph.netNames[code],
				Message:    fmt.Sprintf("net %q is %s", graph.netNames[code], status),
				Refs:       row.Refs,
				Nets:       []string{graph.netNames[code]},
				Suggestion: "repair category: routing",
			})
		}
		statuses = append(statuses, row)
	}
	return statuses, issues
}

func (graph boardConnectivity) allPadsHaveRouteAnchor(netCode int, pads []connectivityPad) bool {
	if len(pads) == 0 {
		return false
	}
	anchors := graph.routeAnchors[netCode]
	if len(anchors) == 0 {
		return false
	}
	for _, pad := range pads {
		if !hasNearbyNonPadAnchor(anchors, pad.point) {
			return false
		}
	}
	return true
}

func refsForPads(pads []connectivityPad) []string {
	seen := map[string]struct{}{}
	var refs []string
	for _, pad := range pads {
		if pad.ref == "" {
			continue
		}
		if _, ok := seen[pad.ref]; ok {
			continue
		}
		seen[pad.ref] = struct{}{}
		refs = append(refs, pad.ref)
	}
	slices.Sort(refs)
	return refs
}

func validateRouteCompletion(board *pcbfiles.PCBFile, graph boardConnectivity) []reports.Issue {
	var issues []reports.Issue
	netNames := graph.netNames
	for index, track := range board.Tracks {
		path := fmt.Sprintf("tracks.%d", index)
		issues = append(issues, validateRouteNetAndLayer(path, track.NetCode, track.NetName, string(track.Layer), netNames)...)
		if track.NetCode > 0 {
			issues = append(issues, validateRouteEndpoint(path+".start", track.NetCode, track.Start, graph)...)
			issues = append(issues, validateRouteEndpoint(path+".end", track.NetCode, track.End, graph)...)
		}
	}
	for index, arc := range board.TrackArcs {
		path := fmt.Sprintf("track_arcs.%d", index)
		issues = append(issues, validateRouteNetAndLayer(path, arc.NetCode, arc.NetName, string(arc.Layer), netNames)...)
		if arc.NetCode > 0 {
			issues = append(issues, validateRouteEndpoint(path+".start", arc.NetCode, arc.Start, graph)...)
			issues = append(issues, validateRouteEndpoint(path+".end", arc.NetCode, arc.End, graph)...)
		}
	}
	for index, via := range board.Vias {
		path := fmt.Sprintf("vias.%d", index)
		if _, ok := netNames[via.NetCode]; via.NetCode > 0 && !ok {
			issues = append(issues, invalidRouteIssue(path+".net_code", fmt.Sprintf("via uses unknown net code %d", via.NetCode), nil))
		}
		if len(via.Layers) < 2 {
			issues = append(issues, invalidRouteIssue(path+".layers", "via must span at least two layers", netIssueNames(via.NetCode, netNames)))
		}
		for layerIndex, layer := range via.Layers {
			if !isCopperLayer(layer) {
				issues = append(issues, invalidRouteIssue(fmt.Sprintf("%s.layers.%d", path, layerIndex), "via layer must be copper", netIssueNames(via.NetCode, netNames)))
			}
		}
	}
	return issues
}

func validateRouteNetAndLayer(path string, netCode int, netName string, layer string, netNames map[int]string) []reports.Issue {
	var issues []reports.Issue
	if _, ok := netNames[netCode]; netCode > 0 && !ok {
		issues = append(issues, invalidRouteIssue(path+".net_code", fmt.Sprintf("route uses unknown net code %d", netCode), nil))
	}
	if strings.TrimSpace(netName) != "" && netName != netNames[netCode] {
		issues = append(issues, invalidRouteIssue(path+".net_name", fmt.Sprintf("route net name %q does not match net code %d", netName, netCode), []string{netName, netNames[netCode]}))
	}
	if !isCopperLayer(kicadfiles.BoardLayer(layer)) {
		issues = append(issues, invalidRouteIssue(path+".layer", "route layer must be copper", netIssueNames(netCode, netNames)))
	}
	return issues
}

func validateRouteEndpoint(path string, netCode int, point kicadfiles.Point, graph boardConnectivity) []reports.Issue {
	anchors := graph.routeAnchors[netCode]
	if len(anchors) == 0 || len(anchors[newPointKey(point)]) > 1 || hasNearbyAnchorExcludingSelf(anchors, point) {
		return nil
	}
	return []reports.Issue{invalidRouteIssue(path, "route endpoint is not connected to a same-net pad, via, or route endpoint", netIssueNames(netCode, graph.netNames))}
}

func validateZones(board *pcbfiles.PCBFile, opts Options) ([]ZoneStatus, []reports.Issue) {
	netNames := map[int]string{}
	for _, net := range board.Nets {
		netNames[net.Code] = net.Name
	}
	statuses := make([]ZoneStatus, 0, len(board.Zones))
	var issues []reports.Issue
	for index, zone := range board.Zones {
		path := fmt.Sprintf("zones.%d", index)
		status := StatusPass
		if len(zone.Polygons) == 0 {
			status = StatusFail
			issues = append(issues, reports.Issue{
				Code:       reports.CodeValidationFailed,
				Severity:   reports.SeverityError,
				Path:       path + ".polygons",
				Message:    "zone has no polygon outline",
				Nets:       netIssueNames(zone.NetCode, netNames),
				Suggestion: "repair category: zone",
			})
		}
		if len(zone.Layers) == 0 {
			status = StatusFail
			issues = append(issues, reports.Issue{
				Code:       reports.CodeValidationFailed,
				Severity:   reports.SeverityError,
				Path:       path + ".layers",
				Message:    "zone has no layers",
				Nets:       netIssueNames(zone.NetCode, netNames),
				Suggestion: "repair category: zone",
			})
		}
		if zone.Keepout != nil && zone.NetCode > 0 {
			status = StatusFail
			issues = append(issues, reports.Issue{
				Code:       reports.CodeInvalidNetAssignment,
				Severity:   reports.SeverityError,
				Path:       path + ".net",
				Message:    "keepout zone must not be assigned to a copper net",
				Nets:       netIssueNames(zone.NetCode, netNames),
				Suggestion: "repair category: zone",
			})
		}
		for filledIndex, filled := range zone.FilledPolygons {
			if !zoneHasLayer(zone, filled.Layer) {
				status = StatusFail
				issues = append(issues, reports.Issue{
					Code:       reports.CodeValidationFailed,
					Severity:   reports.SeverityError,
					Path:       fmt.Sprintf("%s.filled_polygons.%d.layer", path, filledIndex),
					Message:    "zone filled polygon is on an undeclared layer",
					Nets:       netIssueNames(zone.NetCode, netNames),
					Suggestion: "repair category: zone",
				})
			}
		}
		if len(zone.FilledPolygons) == 0 && zone.Keepout == nil {
			severity := reports.SeverityWarning
			if opts.StrictZones {
				severity = reports.SeverityError
				status = StatusFail
			}
			issues = append(issues, reports.Issue{
				Code:       reports.CodeValidationFailed,
				Severity:   severity,
				Path:       path + ".filled_polygons",
				Message:    "zone has no fill evidence; run KiCad refill/DRC for authoritative zone connectivity",
				Nets:       netIssueNames(zone.NetCode, netNames),
				Suggestion: "repair category: zone",
			})
		}
		statuses = append(statuses, ZoneStatus{
			Name:               zone.Name,
			NetCode:            zone.NetCode,
			NetName:            zone.NetName,
			Layers:             boardLayersToStrings(zone.Layers),
			PolygonCount:       len(zone.Polygons),
			FilledPolygonCount: len(zone.FilledPolygons),
			Status:             status,
			Evidence:           zoneEvidence(zone),
		})
	}
	return statuses, issues
}

func runDRCCheck(ctx context.Context, target Target, opts Options) Check {
	if strings.TrimSpace(opts.KiCadCLI) == "" {
		severity := reports.SeverityInfo
		status := StatusSkipped
		if opts.RequireDRC && !opts.AllowMissingDRC {
			severity = reports.SeverityError
			status = StatusFail
		}
		return Check{
			Name:     CheckKiCadDRC,
			Required: opts.RequireDRC,
			Status:   status,
			Issues: []reports.Issue{{
				Code:       reports.CodeSkippedExternalTool,
				Severity:   severity,
				Path:       "kicad_drc",
				Message:    "KiCad DRC was not run because no KiCad CLI path was configured",
				Suggestion: "set --kicad-cli or use --allow-missing-drc when DRC evidence is optional",
			}},
		}
	}
	cli := checks.KiCadCLI{Path: opts.KiCadCLI}
	checkOpts := checks.DefaultOptions()
	checkOpts.KiCadCLI = opts.KiCadCLI
	checkOpts.KeepArtifacts = opts.KeepArtifacts
	checkOpts.ArtifactDir = opts.ArtifactDir
	checkOpts.Allowlist = opts.Allowlist
	targetPath := target.BoardPath
	if target.ProjectDir != "" && target.ProjectPath != "" {
		targetPath = target.ProjectDir
	}
	drc, err := checks.RunDRC(ctx, cli, targetPath, checkOpts)
	issues := drcIssues(drc, err)
	artifacts := drcArtifacts(drc)
	status := StatusPass
	if err != nil {
		status = StatusError
	} else if len(issues) > 0 {
		status = StatusFail
	}
	return Check{Name: CheckKiCadDRC, Required: opts.RequireDRC, Status: status, Issues: issues, Artifacts: artifacts, Evidence: drc.ReportPath}
}

func drcIssues(result checks.CheckResult, err error) []reports.Issue {
	var issues []reports.Issue
	for _, finding := range result.Findings {
		issues = append(issues, reports.Issue{
			Code:       codeForDRCFinding(finding),
			Severity:   severityForDRCFinding(finding.Severity),
			Path:       filepath.ToSlash(finding.File),
			Message:    finding.Message,
			Refs:       finding.References,
			Nets:       checkFindingNets(finding),
			Suggestion: "repair category: " + string(finding.RepairCategory),
		})
	}
	for _, parserIssue := range result.ParserIssues {
		issues = append(issues, reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Path: result.ReportPath, Message: parserIssue.Message})
	}
	if err != nil {
		issues = append(issues, reports.Issue{Code: reports.CodeKiCadCLIFailed, Severity: reports.SeverityError, Path: result.TargetPath, Message: err.Error()})
	}
	return issues
}

func drcArtifacts(result checks.CheckResult) []reports.Artifact {
	if strings.TrimSpace(result.ReportPath) == "" {
		return nil
	}
	return []reports.Artifact{{Kind: reports.ArtifactDRCReport, Path: filepath.ToSlash(result.ReportPath), Description: "drc JSON report"}}
}

func codeForDRCFinding(finding checks.CheckFinding) reports.Code {
	switch finding.RepairCategory {
	case checks.RepairConnectivity:
		return reports.CodeDisconnectedPad
	case checks.RepairNetAssignment:
		return reports.CodeInvalidNetAssignment
	case checks.RepairOutline:
		return reports.CodeMissingBoardOutline
	default:
		return reports.CodeValidationFailed
	}
}

func severityForDRCFinding(severity string) reports.Severity {
	switch strings.ToLower(strings.TrimSpace(severity)) {
	case "warning", "warn", "exclusion", "excluded":
		return reports.SeverityWarning
	case "info", "notice":
		return reports.SeverityInfo
	default:
		return reports.SeverityError
	}
}

func checkFindingNets(finding checks.CheckFinding) []string {
	seen := map[string]struct{}{}
	var nets []string
	add := func(net string) {
		net = strings.TrimSpace(net)
		if net == "" {
			return
		}
		if _, ok := seen[net]; ok {
			return
		}
		seen[net] = struct{}{}
		nets = append(nets, net)
	}
	add(finding.Net)
	for _, net := range finding.Nets {
		add(net)
	}
	return nets
}

func invalidRouteIssue(path string, message string, nets []string) reports.Issue {
	return reports.Issue{
		Code:       reports.CodeValidationFailed,
		Severity:   reports.SeverityError,
		Path:       path,
		Message:    message,
		Nets:       nets,
		Suggestion: "repair category: routing",
	}
}

func netIssueNames(netCode int, netNames map[int]string) []string {
	if strings.TrimSpace(netNames[netCode]) == "" {
		return nil
	}
	return []string{netNames[netCode]}
}

func isCopperLayer(layer kicadfiles.BoardLayer) bool {
	name := string(layer)
	return name == string(kicadfiles.LayerAllCu) || strings.HasSuffix(name, ".Cu")
}

func zoneHasLayer(zone pcbfiles.Zone, layer kicadfiles.BoardLayer) bool {
	for _, candidate := range zone.Layers {
		if candidate == layer {
			return true
		}
	}
	return false
}

func zoneEvidence(zone pcbfiles.Zone) string {
	if len(zone.FilledPolygons) > 0 {
		return "filled_polygons"
	}
	if zone.Keepout != nil {
		return "keepout"
	}
	return "outline_only"
}

func boardLayersToStrings(layers []kicadfiles.BoardLayer) []string {
	out := make([]string, len(layers))
	for index, layer := range layers {
		out[index] = string(layer)
	}
	return out
}

func absolutePadPosition(footprint *pcbfiles.Footprint, pad pcbfiles.Pad) kicadfiles.Point {
	localX := pad.Position.X
	localY := pad.Position.Y
	if footprint.Layer == kicadfiles.LayerBCu {
		localX = -localX
	}
	theta := float64(footprint.Rotation) * math.Pi / 180
	cosTheta := math.Cos(theta)
	sinTheta := math.Sin(theta)
	x := float64(localX)*cosTheta - float64(localY)*sinTheta
	y := float64(localX)*sinTheta + float64(localY)*cosTheta
	return kicadfiles.Point{
		X: footprint.Position.X + kicadfiles.IU(math.Round(x)),
		Y: footprint.Position.Y + kicadfiles.IU(math.Round(y)),
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func appendRepairCategory(suggestion string, category string) string {
	if strings.TrimSpace(suggestion) == "" {
		return "repair category: " + category
	}
	if strings.Contains(suggestion, "repair category:") {
		return suggestion
	}
	return suggestion + "; repair category: " + category
}

func newPointKey(point kicadfiles.Point) pointKey {
	return pointKey{x: floorBucket(point.X), y: floorBucket(point.Y)}
}

func floorBucket(value kicadfiles.IU) int64 {
	return int64(math.Floor(float64(value) / float64(routePointTolerance)))
}

func hasNearbyAnchor(anchors map[pointKey][]string, point kicadfiles.Point) bool {
	key := newPointKey(point)
	for dx := int64(-1); dx <= 1; dx++ {
		for dy := int64(-1); dy <= 1; dy++ {
			if len(anchors[pointKey{x: key.x + dx, y: key.y + dy}]) > 0 {
				return true
			}
		}
	}
	return false
}

func hasNearbyNonPadAnchor(anchors map[pointKey][]string, point kicadfiles.Point) bool {
	key := newPointKey(point)
	for dx := int64(-1); dx <= 1; dx++ {
		for dy := int64(-1); dy <= 1; dy++ {
			for _, kind := range anchors[pointKey{x: key.x + dx, y: key.y + dy}] {
				if kind != "pad" {
					return true
				}
			}
		}
	}
	return false
}

func hasNearbyAnchorExcludingSelf(anchors map[pointKey][]string, point kicadfiles.Point) bool {
	key := newPointKey(point)
	for dx := int64(-1); dx <= 1; dx++ {
		for dy := int64(-1); dy <= 1; dy++ {
			candidates := anchors[pointKey{x: key.x + dx, y: key.y + dy}]
			if len(candidates) == 0 {
				continue
			}
			if dx != 0 || dy != 0 || len(candidates) > 1 {
				return true
			}
		}
	}
	return false
}
