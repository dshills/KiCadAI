package circuitgraph

import (
	"bytes"
	"encoding/json"
	"io"
	"slices"
	"strings"

	"kicadai/internal/reports"
)

func DecodeStrict(reader io.Reader) (Document, []reports.Issue) {
	var buffer bytes.Buffer
	limited := io.LimitReader(reader, MaxDocumentBytes+1)
	if _, err := io.Copy(&buffer, limited); err != nil {
		return Document{}, []reports.Issue{graphIssue(CodeSchemaInvalid, "document", "read circuit graph: "+err.Error())}
	}
	if buffer.Len() > MaxDocumentBytes {
		return Document{}, []reports.Issue{graphIssue(CodeLimitExceeded, "document", "circuit graph exceeds maximum encoded size")}
	}
	decoder := json.NewDecoder(bytes.NewReader(buffer.Bytes()))
	decoder.DisallowUnknownFields()
	var document Document
	if err := decoder.Decode(&document); err != nil {
		return Document{}, []reports.Issue{graphIssue(CodeSchemaInvalid, "document", "decode circuit graph: "+err.Error())}
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return document, []reports.Issue{graphIssue(CodeSchemaInvalid, "document", "circuit graph contains trailing JSON value")}
		}
		return document, []reports.Issue{graphIssue(CodeSchemaInvalid, "document", "decode trailing circuit graph data: "+err.Error())}
	}
	if issues := Validate(document); len(issues) != 0 {
		return document, issues
	}
	return Normalize(document), nil
}

func Normalize(document Document) Document {
	normalized := cloneDocument(document)
	slices.SortStableFunc(normalized.Components, func(left, right Component) int {
		return strings.Compare(left.ID, right.ID)
	})
	for index := range normalized.Components {
		slices.SortStableFunc(normalized.Components[index].Parameters, func(left, right Parameter) int {
			return strings.Compare(left.Name, right.Name)
		})
		slices.Sort(normalized.Components[index].RequiredFunctions)
		slices.SortStableFunc(normalized.Components[index].RequiredRatings, func(left, right RequiredRating) int {
			if left.Kind != right.Kind {
				return strings.Compare(left.Kind, right.Kind)
			}
			if left.Unit != right.Unit {
				return strings.Compare(left.Unit, right.Unit)
			}
			return strings.Compare(left.Value, right.Value)
		})
		slices.SortStableFunc(normalized.Components[index].Properties, func(left, right Property) int {
			return strings.Compare(left.Name, right.Name)
		})
	}
	slices.SortStableFunc(normalized.Nets, func(left, right Net) int {
		return strings.Compare(left.Name, right.Name)
	})
	for index := range normalized.Nets {
		slices.SortStableFunc(normalized.Nets[index].Endpoints, compareEndpoints)
	}
	slices.SortStableFunc(normalized.NoConnects, compareEndpoints)
	slices.SortStableFunc(normalized.Buses, func(left, right Bus) int {
		return strings.Compare(left.ID, right.ID)
	})
	for index := range normalized.Buses {
		slices.SortStableFunc(normalized.Buses[index].Members, func(left, right BusMember) int {
			if left.Net != right.Net {
				return strings.Compare(left.Net, right.Net)
			}
			return strings.Compare(left.Label, right.Label)
		})
	}
	slices.SortStableFunc(normalized.Schematic.Groups, func(left, right SchematicGroup) int {
		if left.Rank != right.Rank {
			return left.Rank - right.Rank
		}
		return strings.Compare(left.ID, right.ID)
	})
	for index := range normalized.Schematic.Groups {
		slices.Sort(normalized.Schematic.Groups[index].Members)
	}
	slices.SortStableFunc(normalized.Schematic.Placements, func(left, right SchematicPlacement) int {
		return strings.Compare(left.Component, right.Component)
	})
	slices.SortStableFunc(normalized.PCB.Regions, func(left, right PCBRegion) int {
		return strings.Compare(left.ID, right.ID)
	})
	slices.SortStableFunc(normalized.PCB.Placements, func(left, right PCBPlacement) int {
		return strings.Compare(left.Component, right.Component)
	})
	slices.SortStableFunc(normalized.PCB.Keepouts, func(left, right PCBKeepout) int {
		return strings.Compare(left.ID, right.ID)
	})
	for index := range normalized.PCB.Keepouts {
		slices.Sort(normalized.PCB.Keepouts[index].Layers)
	}
	slices.SortStableFunc(normalized.PCB.Zones, func(left, right PCBZone) int {
		return strings.Compare(left.Net, right.Net)
	})
	for index := range normalized.PCB.Zones {
		slices.Sort(normalized.PCB.Zones[index].Layers)
	}
	return normalized
}

func compareEndpoints(left, right Endpoint) int {
	if left.Component != right.Component {
		return strings.Compare(left.Component, right.Component)
	}
	if left.Unit != right.Unit {
		return strings.Compare(left.Unit, right.Unit)
	}
	if left.SelectorKind != right.SelectorKind {
		return strings.Compare(string(left.SelectorKind), string(right.SelectorKind))
	}
	return strings.Compare(left.Selector, right.Selector)
}

func cloneDocument(document Document) Document {
	cloned := document
	cloned.Extensions = cloneRawMessages(document.Extensions)
	cloned.Components = append([]Component(nil), document.Components...)
	for index := range cloned.Components {
		component := &cloned.Components[index]
		if document.Components[index].Query != nil {
			query := *document.Components[index].Query
			component.Query = &query
		}
		if document.Components[index].Symbol != nil {
			constraint := *document.Components[index].Symbol
			component.Symbol = &constraint
		}
		if document.Components[index].Footprint != nil {
			constraint := *document.Components[index].Footprint
			component.Footprint = &constraint
		}
		component.Parameters = append([]Parameter(nil), document.Components[index].Parameters...)
		for parameterIndex := range component.Parameters {
			component.Parameters[parameterIndex].Value = cloneParameterValue(document.Components[index].Parameters[parameterIndex].Value)
		}
		component.RequiredRatings = append([]RequiredRating(nil), document.Components[index].RequiredRatings...)
		component.RequiredFunctions = append([]string(nil), document.Components[index].RequiredFunctions...)
		component.Properties = append([]Property(nil), document.Components[index].Properties...)
		component.Extensions = cloneRawMessages(document.Components[index].Extensions)
	}
	cloned.Nets = append([]Net(nil), document.Nets...)
	for index := range cloned.Nets {
		cloned.Nets[index].Required = cloneBool(document.Nets[index].Required)
		cloned.Nets[index].Endpoints = append([]Endpoint(nil), document.Nets[index].Endpoints...)
	}
	cloned.NoConnects = append([]Endpoint(nil), document.NoConnects...)
	cloned.Buses = append([]Bus(nil), document.Buses...)
	for index := range cloned.Buses {
		cloned.Buses[index].Members = append([]BusMember(nil), document.Buses[index].Members...)
	}
	cloned.Schematic.Groups = append([]SchematicGroup(nil), document.Schematic.Groups...)
	for index := range cloned.Schematic.Groups {
		cloned.Schematic.Groups[index].Members = append([]string(nil), document.Schematic.Groups[index].Members...)
	}
	cloned.Schematic.Placements = append([]SchematicPlacement(nil), document.Schematic.Placements...)
	cloned.Schematic.Rules.PositivePowerTop = cloneBool(document.Schematic.Rules.PositivePowerTop)
	cloned.Schematic.Rules.GroundBottom = cloneBool(document.Schematic.Rules.GroundBottom)
	cloned.Schematic.Rules.CenterOnPage = cloneBool(document.Schematic.Rules.CenterOnPage)
	cloned.Schematic.Rules.PreferLabelsForLongNets = cloneBool(document.Schematic.Rules.PreferLabelsForLongNets)
	cloned.Schematic.Rules.AvoidWireCrossings = cloneBool(document.Schematic.Rules.AvoidWireCrossings)
	cloned.PCB.Regions = append([]PCBRegion(nil), document.PCB.Regions...)
	cloned.PCB.Placements = append([]PCBPlacement(nil), document.PCB.Placements...)
	cloned.PCB.Keepouts = append([]PCBKeepout(nil), document.PCB.Keepouts...)
	for index := range cloned.PCB.Keepouts {
		cloned.PCB.Keepouts[index].Layers = append([]string(nil), document.PCB.Keepouts[index].Layers...)
	}
	cloned.PCB.Zones = append([]PCBZone(nil), document.PCB.Zones...)
	for index := range cloned.PCB.Zones {
		cloned.PCB.Zones[index].Layers = append([]string(nil), document.PCB.Zones[index].Layers...)
	}
	cloned.Policy.AllowReferenceAssignment = cloneBool(document.Policy.AllowReferenceAssignment)
	cloned.Policy.AllowValueNormalization = cloneBool(document.Policy.AllowValueNormalization)
	cloned.Policy.AllowLayoutInference = cloneBool(document.Policy.AllowLayoutInference)
	cloned.Policy.AllowSpacingAdjustment = cloneBool(document.Policy.AllowSpacingAdjustment)
	cloned.Policy.AllowLabelInsertion = cloneBool(document.Policy.AllowLabelInsertion)
	cloned.Policy.AllowPlacementAdjustment = cloneBool(document.Policy.AllowPlacementAdjustment)
	cloned.Policy.AllowRouteRetry = cloneBool(document.Policy.AllowRouteRetry)
	return cloned
}

func cloneParameterValue(value ParameterValue) ParameterValue {
	cloned := value
	if value.String != nil {
		stringValue := *value.String
		cloned.String = &stringValue
	}
	if value.Number != nil {
		numberValue := *value.Number
		cloned.Number = &numberValue
	}
	if value.Bool != nil {
		boolValue := *value.Bool
		cloned.Bool = &boolValue
	}
	cloned.List = append([]string(nil), value.List...)
	return cloned
}

func cloneBool(value *bool) *bool {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneRawMessages(values map[string]json.RawMessage) map[string]json.RawMessage {
	if values == nil {
		return nil
	}
	cloned := make(map[string]json.RawMessage, len(values))
	for key, value := range values {
		cloned[key] = append(json.RawMessage(nil), value...)
	}
	return cloned
}
