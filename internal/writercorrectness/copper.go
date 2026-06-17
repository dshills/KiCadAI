package writercorrectness

import (
	"fmt"
	"path/filepath"
	"strings"

	"kicadai/internal/kicadfiles"
	kpcb "kicadai/internal/kicadfiles/pcb"
	"kicadai/internal/reports"
)

const defaultMinAnnularRing = kicadfiles.IU(100000) // 0.1 mm

type CopperSnapshot struct {
	TrackCount int            `json:"track_count"`
	ViaCount   int            `json:"via_count"`
	ZoneCount  int            `json:"zone_count"`
	Tracks     []CopperObject `json:"tracks,omitempty"`
	Vias       []CopperObject `json:"vias,omitempty"`
	Zones      []ZoneObject   `json:"zones,omitempty"`
}

type CopperObject struct {
	Kind    string `json:"kind"`
	NetCode int    `json:"net_code"`
	NetName string `json:"net_name,omitempty"`
	Layer   string `json:"layer,omitempty"`
	WidthIU int64  `json:"width_iu,omitempty"`
}

type ZoneObject struct {
	NetCode      int      `json:"net_code"`
	NetName      string   `json:"net_name,omitempty"`
	Layers       []string `json:"layers,omitempty"`
	PolygonCount int      `json:"polygon_count"`
}

func CheckPCBCopperZones(target Target) (CopperSnapshot, []CheckResult) {
	if target.PCBPath == "" {
		return CopperSnapshot{}, []CheckResult{{
			Name:     CheckCopperNetReferences,
			Status:   CheckSkipped,
			Required: false,
			Summary:  "no PCB resolved",
		}, {
			Name:     CheckZoneNetReferences,
			Status:   CheckSkipped,
			Required: false,
			Summary:  "no PCB resolved",
		}}
	}
	board, err := kpcb.ReadFile(filepath.FromSlash(target.PCBPath))
	if err != nil {
		issue := BlockingIssue(reports.CodeValidationFailed, target.PCBPath, err.Error())
		return CopperSnapshot{}, []CheckResult{{
			Name:     CheckCopperNetReferences,
			Status:   CheckFail,
			Required: true,
			Issues:   []reports.Issue{issue},
		}, {
			Name:     CheckZoneNetReferences,
			Status:   CheckFail,
			Required: true,
			Issues:   []reports.Issue{issue},
		}}
	}
	snapshot := snapshotCopper(board)
	copperIssues, zoneIssues := validateCopperZones(board, target.PCBPath)
	return snapshot, []CheckResult{{
		Name:     CheckCopperNetReferences,
		Status:   StatusForIssues(copperIssues),
		Required: true,
		Issues:   copperIssues,
		Summary:  fmt.Sprintf("%d track(s), %d via(s)", len(board.Tracks), len(board.Vias)),
	}, {
		Name:     CheckZoneNetReferences,
		Status:   StatusForIssues(zoneIssues),
		Required: true,
		Issues:   zoneIssues,
		Summary:  fmt.Sprintf("%d zone(s)", len(board.Zones)),
	}}
}

func snapshotCopper(board kpcb.PCBFile) CopperSnapshot {
	snapshot := CopperSnapshot{TrackCount: len(board.Tracks), ViaCount: len(board.Vias), ZoneCount: len(board.Zones)}
	snapshot.Tracks = make([]CopperObject, 0, len(board.Tracks))
	snapshot.Vias = make([]CopperObject, 0, len(board.Vias))
	snapshot.Zones = make([]ZoneObject, 0, len(board.Zones))
	for _, track := range board.Tracks {
		snapshot.Tracks = append(snapshot.Tracks, CopperObject{Kind: "track", NetCode: track.NetCode, NetName: track.NetName, Layer: string(track.Layer), WidthIU: int64(track.Width)})
	}
	for _, via := range board.Vias {
		snapshot.Vias = append(snapshot.Vias, CopperObject{Kind: "via", NetCode: via.NetCode, NetName: via.NetName, WidthIU: int64(via.Size)})
	}
	for _, zone := range board.Zones {
		object := ZoneObject{NetCode: zone.NetCode, NetName: zone.NetName, PolygonCount: len(zone.Polygons)}
		for _, layer := range zone.Layers {
			object.Layers = append(object.Layers, string(layer))
		}
		snapshot.Zones = append(snapshot.Zones, object)
	}
	return snapshot
}

func validateCopperZones(board kpcb.PCBFile, path string) ([]reports.Issue, []reports.Issue) {
	validNets := map[int]string{}
	for _, net := range board.Nets {
		validNets[net.Code] = net.Name
	}
	validLayers := map[kicadfiles.BoardLayer]struct{}{}
	for _, layer := range board.Layers {
		validLayers[layer.Name] = struct{}{}
	}
	var copperIssues []reports.Issue
	for i, track := range board.Tracks {
		objectPath := fmt.Sprintf("%s.tracks.%d", slashPath(path), i)
		copperIssues = append(copperIssues, validateCopperNet(objectPath, track.NetCode, track.NetName, validNets)...)
		if track.Width <= 0 {
			copperIssues = append(copperIssues, reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Path: objectPath, Message: "track width must be positive", Nets: []string{track.NetName}})
		}
		if _, ok := validLayers[track.Layer]; !ok {
			copperIssues = append(copperIssues, reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Path: objectPath, Message: "track layer is not defined in board layer table", Nets: []string{track.NetName}})
		}
	}
	for i, via := range board.Vias {
		objectPath := fmt.Sprintf("%s.vias.%d", slashPath(path), i)
		copperIssues = append(copperIssues, validateCopperNet(objectPath, via.NetCode, via.NetName, validNets)...)
		if via.Size <= 0 || via.Drill <= 0 {
			copperIssues = append(copperIssues, reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Path: objectPath, Message: "via size and drill must be positive", Nets: []string{via.NetName}})
		}
		if via.Size < via.Drill+2*defaultMinAnnularRing {
			copperIssues = append(copperIssues, reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Path: objectPath, Message: "via annular ring is below minimum", Nets: []string{via.NetName}})
		}
		for _, layer := range via.Layers {
			if _, ok := validLayers[layer]; !ok {
				copperIssues = append(copperIssues, reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Path: objectPath, Message: "via layer is not defined in board layer table", Nets: []string{via.NetName}})
			}
		}
	}
	var zoneIssues []reports.Issue
	for i, zone := range board.Zones {
		objectPath := fmt.Sprintf("%s.zones.%d", slashPath(path), i)
		zoneIssues = append(zoneIssues, validateCopperNet(objectPath, zone.NetCode, zone.NetName, validNets)...)
		for _, layer := range zone.Layers {
			if _, ok := validLayers[layer]; !ok {
				zoneIssues = append(zoneIssues, reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Path: objectPath, Message: "zone layer is not defined in board layer table", Nets: []string{zone.NetName}})
			}
		}
		for polygonIndex, polygon := range zone.Polygons {
			if len(polygon) < 3 {
				zoneIssues = append(zoneIssues, reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Path: fmt.Sprintf("%s.polygons.%d", objectPath, polygonIndex), Message: "zone polygon must contain at least three points", Nets: []string{zone.NetName}})
				continue
			}
			if polygon[0] != polygon[len(polygon)-1] {
				zoneIssues = append(zoneIssues, reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Path: fmt.Sprintf("%s.polygons.%d", objectPath, polygonIndex), Message: "zone polygon must be closed", Nets: []string{zone.NetName}})
			}
		}
	}
	return copperIssues, zoneIssues
}

func validateCopperNet(path string, code int, name string, validNets map[int]string) []reports.Issue {
	want, ok := validNets[code]
	if !ok {
		return []reports.Issue{{Code: reports.CodeInvalidNetAssignment, Severity: reports.SeverityError, Path: path, Message: "copper object references missing net code", Nets: []string{name}}}
	}
	if strings.TrimSpace(name) != strings.TrimSpace(want) {
		return []reports.Issue{{Code: reports.CodeInvalidNetAssignment, Severity: reports.SeverityError, Path: path, Message: "copper object net name does not match net table", Nets: []string{name, want}}}
	}
	return nil
}
