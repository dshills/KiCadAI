package placement

import (
	"fmt"
	"math"
	"strings"

	"kicadai/internal/reports"
)

func ValidateGroups(request Request, placements []PlacementResult) []reports.Issue {
	request = NormalizeRequest(request)
	byRef := map[string]PlacementResult{}
	for _, placement := range placements {
		if placement.Reason == "" {
			byRef[strings.ToUpper(placement.Ref)] = placement
		}
	}
	var issues []reports.Issue
	spreadToleranceMM := request.Rules.GridMM
	if spreadToleranceMM <= 0 {
		spreadToleranceMM = DefaultRules().GridMM
	}
	for groupIndex, group := range request.Groups {
		if group.MaxSpreadMM <= 0 {
			continue
		}
		var centers []Point
		for _, ref := range group.Components {
			placement, ok := byRef[strings.ToUpper(ref)]
			if !ok {
				continue
			}
			centers = append(centers, placement.Bounds.Center())
		}
		reported := false
		for i := 0; i < len(centers) && !reported; i++ {
			for j := i + 1; j < len(centers); j++ {
				dx := centers[i].XMM - centers[j].XMM
				dy := centers[i].YMM - centers[j].YMM
				distanceSquared := dx*dx + dy*dy
				maxSpreadWithGridTolerance := group.MaxSpreadMM + spreadToleranceMM
				maxSpreadSquared := maxSpreadWithGridTolerance * maxSpreadWithGridTolerance
				if distanceSquared > maxSpreadSquared {
					issues = append(issues, reports.Issue{
						Code:     reports.CodeValidationFailed,
						Severity: reports.SeverityError,
						Path:     fmt.Sprintf("groups[%d].max_spread_mm", groupIndex),
						Message:  fmt.Sprintf("group %s spread %.2fmm exceeds %.2fmm", group.ID, math.Sqrt(distanceSquared), group.MaxSpreadMM),
					})
					reported = true
					break
				}
			}
		}
	}
	return issues
}
