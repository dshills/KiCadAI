package schematiclayout

import (
	"fmt"
	"sort"

	"kicadai/internal/kicadfiles"
)

const defaultPartitionSheetMarginMM = 10.16

// Partition separates an oversized component graph into deterministic regions
// sized for the largest standard paper. Group membership is kept intact when
// possible; nets that cross regions are returned for hierarchical/global label
// emission by the writer phase.
func Partition(request Request) PartitionResult {
	return partition(request, nil)
}

// PartitionPlaced is the allocation-free form used after Layout has already
// computed the largest-sheet placement.
func PartitionPlaced(request Request, placed []PlacedComponent) PartitionResult {
	return partition(request, placed)
}

func partition(request Request, placedComponents []PlacedComponent) PartitionResult {
	request = NormalizeRequest(request)
	if len(request.Components) == 0 {
		return PartitionResult{Sheets: []PartitionSheet{{ID: "0-0", Name: "Sheet 0-0"}}, Complete: true}
	}
	portrait := request.Sheet.Width > 0 && request.Sheet.Height > request.Sheet.Width
	// Child sheets need enough room for their local wires and labels after the
	// hierarchy writer recenters them. A4 keeps partitioned pages readable and
	// lets the child layout solve smaller local graphs deterministically.
	sheet := SheetForPaperOrientation("A4", portrait)
	if request.Sheet.Width > sheet.Width || request.Sheet.Height > sheet.Height {
		sheet = request.Sheet
		if sheet.Margin <= 0 {
			sheet.Margin = kicadfiles.MM(defaultPartitionSheetMarginMM)
		}
	}
	placed := Result{Components: placedComponents}
	if len(placedComponents) == 0 {
		placed = Place(Request{
			Sheet:      sheet,
			Components: request.Components,
			Nets:       request.Nets,
			Groups:     request.Groups,
			Rules:      request.Rules,
		})
	}
	usable := UsableSheet(sheet)
	if usable.Width() <= 0 || usable.Height() <= 0 {
		sheet = SheetForPaperOrientation("A0", portrait)
		usable = UsableSheet(sheet)
	}

	groupByRef := make(map[string]string, len(request.Components))
	explicitGroups := make(map[string]struct{}, len(request.Groups))
	for _, group := range request.Groups {
		if !group.Inferred {
			explicitGroups[group.ID] = struct{}{}
		}
	}
	for _, component := range request.Components {
		if _, ok := explicitGroups[component.GroupID]; ok {
			groupByRef[component.Ref] = component.GroupID
		}
	}
	bodyByRef := make(map[string]Rect, len(placed.Components))
	for _, component := range placed.Components {
		bodyByRef[component.Ref] = componentBody(component)
	}
	oversizedGroups := oversizedPartitionGroups(groupByRef, usable, bodyByRef)
	assignments := partitionAssignments(placed.Components, groupByRef, oversizedGroups, usable, bodyByRef)
	assignments = limitPartitionAssignments(placed.Components, assignments, request.MaxComponentsPerSheet)
	partitions := map[string]*PartitionSheet{}
	for _, component := range placed.Components {
		id := assignments[component.Ref]
		partition := partitions[id]
		if partition == nil {
			partition = &PartitionSheet{ID: id, Name: partitionName(id)}
			partitions[id] = partition
		}
		partition.Components = append(partition.Components, component.Ref)
		body := bodyByRef[component.Ref]
		if len(partition.Components) == 1 {
			partition.Bounds = body
		} else {
			partition.Bounds = unionRect(partition.Bounds, body)
		}
	}

	var crossSheetNets []CrossSheetNet
	seenSheets := map[string]struct{}{}
	for _, net := range request.Nets {
		clear(seenSheets)
		for _, endpoint := range net.Endpoints {
			if sheetID := assignments[endpoint.Ref]; sheetID != "" {
				seenSheets[sheetID] = struct{}{}
			}
		}
		ids := sortedKeys(seenSheets)
		if len(ids) == 0 {
			continue
		}
		if len(ids) > 1 {
			crossSheetNets = append(crossSheetNets, CrossSheetNet{Name: net.Name, Sheets: ids})
		}
		for _, id := range ids {
			partitions[id].Nets = append(partitions[id].Nets, net.Name)
		}
	}

	ids := make([]string, 0, len(partitions))
	for id := range partitions {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	result := PartitionResult{Complete: len(ids) > 0, CrossSheetNets: crossSheetNets, SplitGroups: sortedKeys(oversizedGroups)}
	for _, id := range ids {
		sheet := *partitions[id]
		sort.Strings(sheet.Components)
		sort.Strings(sheet.Nets)
		result.Sheets = append(result.Sheets, sheet)
		if sheet.Bounds.Width() > usable.Width() || sheet.Bounds.Height() > usable.Height() {
			result.Complete = false
		}
	}
	return result
}

// limitPartitionAssignments applies an explicit sheet-capacity contract after
// geometry-based partitioning. It is deterministic and leaves the default
// zero-value behavior unchanged.
func limitPartitionAssignments(components []PlacedComponent, assignments map[string]string, maximum int) map[string]string {
	if maximum <= 0 || len(components) <= maximum {
		return assignments
	}
	byPartition := map[string][]PlacedComponent{}
	for _, component := range components {
		byPartition[assignments[component.Ref]] = append(byPartition[assignments[component.Ref]], component)
	}
	limited := make(map[string]string, len(assignments))
	partitionIDs := make([]string, 0, len(byPartition))
	for partitionID := range byPartition {
		partitionIDs = append(partitionIDs, partitionID)
	}
	sort.Strings(partitionIDs)
	for _, partitionID := range partitionIDs {
		members := byPartition[partitionID]
		sort.SliceStable(members, func(i, j int) bool { return members[i].Ref < members[j].Ref })
		if len(members) <= maximum {
			for _, member := range members {
				limited[member.Ref] = partitionID
			}
			continue
		}
		for index, member := range members {
			limited[member.Ref] = fmt.Sprintf("%s-%d", partitionID, index/maximum)
		}
	}
	return limited
}

func oversizedPartitionGroups(groupByRef map[string]string, usable Rect, bodyByRef map[string]Rect) map[string]struct{} {
	boundsByGroup := map[string]Rect{}
	for ref, group := range groupByRef {
		body, ok := bodyByRef[ref]
		if group == "" || !ok {
			continue
		}
		boundsByGroup[group] = unionRect(boundsByGroup[group], body)
	}
	oversized := map[string]struct{}{}
	for group, bounds := range boundsByGroup {
		if bounds.Width() > usable.Width() || bounds.Height() > usable.Height() {
			oversized[group] = struct{}{}
		}
	}
	return oversized
}

func partitionAssignments(components []PlacedComponent, groupByRef map[string]string, oversizedGroups map[string]struct{}, usable Rect, bodyByRef map[string]Rect) map[string]string {
	assignments := map[string]string{}
	groupCells := map[string]string{}
	originX, originY := kicadfiles.IU(0), kicadfiles.IU(0)
	maxWidth, maxHeight := kicadfiles.IU(0), kicadfiles.IU(0)
	for index, component := range components {
		body := bodyByRef[component.Ref]
		if index == 0 || body.MinX < originX {
			originX = body.MinX
		}
		if index == 0 || body.MinY < originY {
			originY = body.MinY
		}
		if body.Width() > maxWidth {
			maxWidth = body.Width()
		}
		if body.Height() > maxHeight {
			maxHeight = body.Height()
		}
	}
	cellWidth := usable.Width() - maxWidth
	cellHeight := usable.Height() - maxHeight
	if cellWidth <= 0 {
		cellWidth = usable.Width()
	}
	if cellWidth < usable.Width()/2 {
		cellWidth = usable.Width()
	}
	if cellHeight <= 0 {
		cellHeight = usable.Height()
	}
	if cellHeight < usable.Height()/2 {
		cellHeight = usable.Height()
	}
	for _, component := range components {
		body := bodyByRef[component.Ref]
		center := kicadfiles.Point{X: body.MinX + (body.MaxX-body.MinX)/2, Y: body.MinY + (body.MaxY-body.MinY)/2}
		cell := fmt.Sprintf("%d-%d", cellIndex(center.X, originX, cellWidth), cellIndex(center.Y, originY, cellHeight))
		if group := groupByRef[component.Ref]; group != "" {
			if _, oversized := oversizedGroups[group]; oversized {
				assignments[component.Ref] = cell
				continue
			}
			if prior := groupCells[group]; prior != "" {
				cell = prior
			} else {
				groupCells[group] = cell
			}
		}
		assignments[component.Ref] = cell
	}
	return assignments
}

func cellIndex(value, origin, size kicadfiles.IU) int {
	if size <= 0 || value <= origin {
		return 0
	}
	return int((value - origin) / size)
}

func partitionName(id string) string {
	return "Sheet " + id
}

func sortedKeys(values map[string]struct{}) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
