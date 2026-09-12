package schematiclayout

import (
	"slices"
	"strings"

	"kicadai/internal/kicadfiles"
)

const FunctionalOwnershipV1 = "ownership-v1"

const FunctionalOwnershipV2 = "ownership-v2"

// functionalGroupPositions lays out each explicitly owned circuit block using
// the existing topology/role engine, then packs whole blocks into stable rows.
// Global power rails no longer create one remote capacitor bank. No electrical
// net, pin, symbol rotation, or physical-board coordinate is changed here.
func functionalGroupPositions(request Request, rules Rules) map[string]kicadfiles.Point {
	groups := append([]Group(nil), request.Groups...)
	slices.SortFunc(groups, func(a, b Group) int {
		if a.OriginalOrdinal != b.OriginalOrdinal {
			return a.OriginalOrdinal - b.OriginalOrdinal
		}
		return strings.Compare(a.ID, b.ID)
	})
	positions := map[string]kicadfiles.Point{}
	var x, y, rowHeight kicadfiles.IU
	width := kicadfiles.MM(420)
	gutter := max(rules.MinGroupGutter, kicadfiles.MM(25.4))
	for _, group := range groups {
		localRules := rules
		localRules.MinComponentSpacing = max(rules.MinComponentSpacing, kicadfiles.MM(15.24))
		localRules.MinStageSpacing = max(rules.MinStageSpacing, kicadfiles.MM(35.56))
		local := Request{Sheet: request.Sheet, Rules: localRules}
		ids := map[string]bool{}
		for _, c := range request.Components {
			if c.GroupID != group.ID {
				continue
			}
			ids[c.Ref] = true
			c.RankFixed = false
			c.GroupID = ""
			c.Stage = StageForRole(c.Role)
			local.Components = append(local.Components, c)
		}
		if len(local.Components) == 0 {
			continue
		}
		for _, n := range request.Nets {
			copy := n
			copy.Endpoints = nil
			for _, e := range n.Endpoints {
				if ids[e.Ref] {
					copy.Endpoints = append(copy.Endpoints, e)
				}
			}
			if len(copy.Endpoints) > 1 {
				local.Nets = append(local.Nets, copy)
			}
		}
		cells, _, _ := planPlacement(local)
		// A local block has only a handful of support parts. Spread those in a
		// row instead of reproducing the global three-high capacitor bank.
		auxiliaryLimit := 1
		active, pureSupport := pureDecouplingBlock(local.Components)
		if pureSupport {
			auxiliaryLimit = 2
		}
		cells, _ = spreadAuxiliaryLaneRanks(local.Components, cells, auxiliaryLimit)
		points := placementPositions(local.Components, cells, placementRankX(local.Components, cells, localRules), localRules)
		if pureSupport {
			var left, right kicadfiles.IU
			first := true
			for _, c := range local.Components {
				if c.Ref == active {
					continue
				}
				x := points[c.Ref].X
				if first {
					left, right, first = x, x, false
				} else {
					left = min(left, x)
					right = max(right, x)
				}
			}
			p := points[active]
			p.X = SnapPoint(kicadfiles.Point{X: (left + right) / 2}, rules.Grid).X
			points[active] = p
		}
		if request.FunctionalLocality {
			points = functionalSupportPositions(local.Components, points, localRules)
		}
		var bounds Rect
		for i, c := range local.Components {
			b := componentBoundsAt(c, points[c.Ref])
			if i == 0 {
				bounds = b
			} else {
				bounds = unionRect(bounds, b)
			}
		}
		bounds = bounds.Inflate(kicadfiles.MM(12.7))
		if x > 0 && x+bounds.Width() > width {
			x = 0
			y += rowHeight + gutter
			rowHeight = 0
		}
		for _, c := range local.Components {
			p := points[c.Ref]
			positions[c.Ref] = SnapPoint(kicadfiles.Point{X: p.X + x - bounds.MinX, Y: p.Y + y - bounds.MinY}, rules.Grid)
		}
		x += bounds.Width() + gutter
		rowHeight = max(rowHeight, bounds.Height())
	}
	return positions
}

func pureDecouplingBlock(components []Component) (string, bool) {
	active := ""
	for _, c := range components {
		if containsNormalizedRole(c.Role, "ic", "regulator", "sensor") {
			if active != "" {
				return "", false
			}
			active = c.Ref
		} else if !containsNormalizedRole(c.Role, "decoupling_capacitor", "bulk_capacitor", "decoupling") {
			return "", false
		}
	}
	return active, active != "" && len(components) > 1
}
