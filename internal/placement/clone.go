package placement

import "slices"

func CloneRequest(request Request) Request {
	out := request
	out.Components = slices.Clone(request.Components)
	for index := range out.Components {
		out.Components[index].Pads = slices.Clone(out.Components[index].Pads)
		out.Components[index].Hints = slices.Clone(out.Components[index].Hints)
		out.Components[index].Mobility.Transforms = slices.Clone(out.Components[index].Mobility.Transforms)
		out.Components[index].Mobility.Constraints = slices.Clone(out.Components[index].Mobility.Constraints)
		if out.Components[index].Position != nil {
			position := *out.Components[index].Position
			out.Components[index].Position = &position
		}
		out.Components[index].Rotation.AllowedDeg = slices.Clone(out.Components[index].Rotation.AllowedDeg)
		if out.Components[index].Rotation.FixedDeg != nil {
			fixed := *out.Components[index].Rotation.FixedDeg
			out.Components[index].Rotation.FixedDeg = &fixed
		}
	}
	out.Nets = slices.Clone(request.Nets)
	for index := range out.Nets {
		out.Nets[index].Endpoints = slices.Clone(out.Nets[index].Endpoints)
	}
	out.Groups = slices.Clone(request.Groups)
	for index := range out.Groups {
		out.Groups[index].Components = slices.Clone(out.Groups[index].Components)
		if out.Groups[index].Anchor.At != nil {
			at := *out.Groups[index].Anchor.At
			out.Groups[index].Anchor.At = &at
		}
	}
	out.Keepouts = slices.Clone(request.Keepouts)
	for index := range out.Keepouts {
		out.Keepouts[index].Layers = slices.Clone(out.Keepouts[index].Layers)
		out.Keepouts[index].ExemptRefs = slices.Clone(out.Keepouts[index].ExemptRefs)
		if out.Keepouts[index].BlocksRoute != nil {
			value := *out.Keepouts[index].BlocksRoute
			out.Keepouts[index].BlocksRoute = &value
		}
	}
	out.Mechanical = slices.Clone(request.Mechanical)
	for index := range out.Mechanical {
		out.Mechanical[index].Layers = slices.Clone(out.Mechanical[index].Layers)
	}
	out.ProximityRules = slices.Clone(request.ProximityRules)
	for index := range out.ProximityRules {
		out.ProximityRules[index].TargetRefs = slices.Clone(out.ProximityRules[index].TargetRefs)
		out.ProximityRules[index].AnchorPins = slices.Clone(out.ProximityRules[index].AnchorPins)
		out.ProximityRules[index].TargetPins = slices.Clone(out.ProximityRules[index].TargetPins)
	}
	out.RegionRules = slices.Clone(request.RegionRules)
	for index := range out.RegionRules {
		out.RegionRules[index].Refs = slices.Clone(out.RegionRules[index].Refs)
		out.RegionRules[index].NetRoles = slices.Clone(out.RegionRules[index].NetRoles)
	}
	return out
}
