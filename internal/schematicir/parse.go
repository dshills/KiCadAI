package schematicir

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"

	"kicadai/internal/reports"
)

const maxDocumentBytes = 1 << 20

func DecodeStrict(reader io.Reader) (Document, []reports.Issue) {
	var buffer bytes.Buffer
	limited := io.LimitReader(reader, maxDocumentBytes+1)
	if _, err := io.Copy(&buffer, limited); err != nil {
		return Document{}, []reports.Issue{issue("document", "read schematic IR: "+err.Error())}
	}
	if buffer.Len() > maxDocumentBytes {
		return Document{}, []reports.Issue{issue("document", "schematic IR exceeds maximum size")}
	}

	decoder := json.NewDecoder(bytes.NewReader(buffer.Bytes()))
	decoder.DisallowUnknownFields()
	var document Document
	if err := decoder.Decode(&document); err != nil {
		return Document{}, []reports.Issue{issue("document", "decode schematic IR: "+err.Error())}
	}
	document = applyDefaults(document, false)
	issues := validateDefaulted(document)
	if len(issues) != 0 {
		return document, issues
	}
	return normalizeBuses(normalizeNets(document)), nil
}

func Normalize(document Document) Document {
	return normalizeBuses(normalizeNets(applyDefaults(document, true)))
}

func applyDefaults(document Document, cloneNets bool) Document {
	if document.Schema == "" {
		document.Schema = SchemaID
	}
	if document.Version == 0 {
		document.Version = Version
	}
	if document.Metadata.Paper == "" {
		document.Metadata.Paper = DefaultPaper
	}
	if document.Layout.Flow == "" {
		document.Layout.Flow = FlowLeftToRight
	}
	if document.Layout.Origin == "" {
		document.Layout.Origin = OriginCentered
	}
	if document.Layout.Lanes.Power == "" {
		document.Layout.Lanes.Power = LanePositionTop
	}
	if document.Layout.Lanes.Ground == "" {
		document.Layout.Lanes.Ground = LanePositionBottom
	}
	if document.Layout.Lanes.Signals == "" {
		document.Layout.Lanes.Signals = LanePositionMiddle
	}
	if document.Layout.Rules.PositivePowerTop == nil {
		document.Layout.Rules.PositivePowerTop = boolPtr(true)
	}
	if document.Layout.Rules.GroundBottom == nil {
		document.Layout.Rules.GroundBottom = boolPtr(true)
	}
	if document.Layout.Rules.PreferLabelsForLongNets == nil {
		document.Layout.Rules.PreferLabelsForLongNets = boolPtr(true)
	}
	if document.Layout.Rules.AvoidWireCrossings == nil {
		document.Layout.Rules.AvoidWireCrossings = boolPtr(true)
	}
	if document.Layout.Rules.MinGroupSpacingMM == nil {
		document.Layout.Rules.MinGroupSpacingMM = floatPtr(DefaultMinGroupSpacingMM)
	}
	if document.Layout.Rules.MinComponentSpacingMM == nil {
		document.Layout.Rules.MinComponentSpacingMM = floatPtr(DefaultMinComponentSpacingMM)
	}
	if document.Policy.Acceptance == "" {
		document.Policy.Acceptance = AcceptanceStructural
	}
	if cloneNets && document.Circuit.Nets != nil {
		document.Circuit.Nets = append([]Net(nil), document.Circuit.Nets...)
		for index := range document.Circuit.Nets {
			document.Circuit.Nets[index].Connect = append([]EndpointRef(nil), document.Circuit.Nets[index].Connect...)
		}
	}
	if cloneNets {
		document.Circuit.Buses = cloneBuses(document.Circuit.Buses)
		document.Layout.Buses = cloneBusLayouts(document.Layout.Buses)
	}
	for index, net := range document.Circuit.Nets {
		if net.Role == NetRoleNoConnect && net.Name == "" {
			name := fmt.Sprintf("NC_invalid_%d", index)
			if len(net.Connect) == 1 {
				name = "NC_" + string(net.Connect[0])
			}
			document.Circuit.Nets[index].Name = name
		}
	}
	return document
}

func cloneBuses(buses []Bus) []Bus {
	if buses == nil {
		return nil
	}
	cloned := make([]Bus, len(buses))
	for index, bus := range buses {
		cloned[index] = bus
		cloned[index].Members = append([]BusMember(nil), bus.Members...)
	}
	return cloned
}

func cloneBusLayouts(layouts []BusLayout) []BusLayout {
	if layouts == nil {
		return nil
	}
	cloned := make([]BusLayout, len(layouts))
	for index, layout := range layouts {
		cloned[index] = layout
		cloned[index].Points = append([]LayoutPoint(nil), layout.Points...)
		cloned[index].Entries = append([]BusEntryLayout(nil), layout.Entries...)
	}
	return cloned
}

func boolPtr(value bool) *bool {
	return &value
}

func floatPtr(value float64) *float64 {
	return &value
}

func normalizeNets(document Document) Document {
	type mergeState struct {
		index int
		net   Net
	}
	seen := map[string]mergeState{}
	merged := make([]Net, 0, len(document.Circuit.Nets))
	for _, net := range document.Circuit.Nets {
		if net.Name == "" {
			merged = append(merged, net)
			continue
		}
		state, ok := seen[net.Name]
		if !ok {
			net.Connect = append([]EndpointRef(nil), net.Connect...)
			seen[net.Name] = mergeState{index: len(merged), net: net}
			merged = append(merged, net)
			continue
		}
		state.net.Connect = append(state.net.Connect, net.Connect...)
		merged[state.index] = state.net
		seen[net.Name] = state
	}
	document.Circuit.Nets = merged
	return document
}

func normalizeBuses(document Document) Document {
	if len(document.Circuit.Buses) != 0 {
		document.Circuit.Buses = cloneBuses(document.Circuit.Buses)
		slices.SortStableFunc(document.Circuit.Buses, func(left, right Bus) int {
			return strings.Compare(left.ID, right.ID)
		})
		for index := range document.Circuit.Buses {
			slices.SortStableFunc(document.Circuit.Buses[index].Members, func(left, right BusMember) int {
				if left.Net != right.Net {
					return strings.Compare(left.Net, right.Net)
				}
				return strings.Compare(left.Label, right.Label)
			})
		}
	}
	if len(document.Layout.Buses) != 0 {
		document.Layout.Buses = cloneBusLayouts(document.Layout.Buses)
		slices.SortStableFunc(document.Layout.Buses, func(left, right BusLayout) int {
			return strings.Compare(left.Bus, right.Bus)
		})
		for index := range document.Layout.Buses {
			slices.SortStableFunc(document.Layout.Buses[index].Entries, func(left, right BusEntryLayout) int {
				if left.Member != right.Member {
					return strings.Compare(left.Member, right.Member)
				}
				if left.Endpoint != right.Endpoint {
					return strings.Compare(string(left.Endpoint), string(right.Endpoint))
				}
				if left.At.XMM != right.At.XMM {
					if left.At.XMM < right.At.XMM {
						return -1
					}
					return 1
				}
				if left.At.YMM < right.At.YMM {
					return -1
				}
				if left.At.YMM > right.At.YMM {
					return 1
				}
				return 0
			})
		}
	}
	return document
}

func issue(path, message string) reports.Issue {
	return reports.Issue{
		Code:     reports.CodeValidationFailed,
		Severity: reports.SeverityError,
		Path:     path,
		Message:  message,
	}
}
