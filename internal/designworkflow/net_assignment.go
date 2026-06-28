package designworkflow

import (
	"sort"
	"strconv"
	"strings"

	"kicadai/internal/placement"
	"kicadai/internal/reports"
	"kicadai/internal/routing"
	"kicadai/internal/transactions"
)

type GeneratedNetAssignmentSource string

const (
	GeneratedNetSourcePlacement    GeneratedNetAssignmentSource = "placement"
	GeneratedNetSourceRouting      GeneratedNetAssignmentSource = "routing"
	GeneratedNetSourceRouteOp      GeneratedNetAssignmentSource = "route_operation"
	GeneratedNetSourcePad          GeneratedNetAssignmentSource = "pad"
	GeneratedNetSourceCopper       GeneratedNetAssignmentSource = "copper"
	GeneratedNetSourceBoard        GeneratedNetAssignmentSource = "board"
	GeneratedNetSourceGeneratedPCB GeneratedNetAssignmentSource = "generated_pcb"
)

type GeneratedNetTableEntry struct {
	Name    string                         `json:"name"`
	Code    int                            `json:"code"`
	Sources []GeneratedNetAssignmentSource `json:"sources,omitempty"`
}

type GeneratedNetTable struct {
	Nets   []GeneratedNetTableEntry `json:"nets"`
	ByName map[string]int           `json:"-"`
}

type generatedNetCollector struct {
	sources map[string]map[GeneratedNetAssignmentSource]struct{}
	issues  []reports.Issue
}

func BuildGeneratedNetTable(placed *PlacementStageResult, routed *RoutingStageResult) (GeneratedNetTable, []reports.Issue) {
	collector := generatedNetCollector{sources: map[string]map[GeneratedNetAssignmentSource]struct{}{}}
	if placed != nil {
		for index, net := range placed.Request.Nets {
			path := "placement.nets[" + strconv.Itoa(index) + "]"
			collector.add(path, net.Name, GeneratedNetSourcePlacement)
			for _, endpoint := range net.Endpoints {
				if strings.TrimSpace(endpoint.Ref) == "" || strings.TrimSpace(endpoint.Pin) == "" {
					collector.issues = append(collector.issues, generatedNetAssignmentIssue(path, "net endpoint requires ref and pin"))
				}
			}
		}
		for _, component := range placed.Request.Components {
			for _, pad := range component.Pads {
				collector.add("placement.components."+component.Ref+".pads."+pad.Name, pad.Net, GeneratedNetSourcePad)
			}
		}
	}
	if routed != nil {
		for index, net := range routed.Request.Nets {
			collector.add("routing.nets["+strconv.Itoa(index)+"]", net.Name, GeneratedNetSourceRouting)
		}
		for index, operation := range routed.Operations {
			if operation.Op != transactions.OpRoute {
				continue
			}
			// transactions.Operation caches the JSON net_name field in Net
			// when operations are created or unmarshaled.
			if strings.TrimSpace(operation.Net) != "" {
				collector.add("routing.operations."+strconv.Itoa(index)+".net_name", operation.Net, GeneratedNetSourceRouteOp)
				continue
			}
			collector.issues = append(collector.issues, generatedNetAssignmentIssue("routing.operations."+strconv.Itoa(index)+".net_name", "route operation missing net metadata"))
		}
	}
	return collector.table(), collector.issues
}

func (collector *generatedNetCollector) add(path string, name string, source GeneratedNetAssignmentSource) {
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	if name == "0" {
		collector.issues = append(collector.issues, generatedNetAssignmentIssue(path, "net code 0 is reserved for no-net and cannot be used as a generated net name"))
		return
	}
	if collector.sources[name] == nil {
		collector.sources[name] = map[GeneratedNetAssignmentSource]struct{}{}
	}
	collector.sources[name][source] = struct{}{}
}

func (collector generatedNetCollector) table() GeneratedNetTable {
	names := make([]string, 0, len(collector.sources))
	for name := range collector.sources {
		names = append(names, name)
	}
	sort.Strings(names)
	table := GeneratedNetTable{ByName: map[string]int{}}
	for index, name := range names {
		code := index + 1
		table.ByName[name] = code
		entry := GeneratedNetTableEntry{Name: name, Code: code}
		for source := range collector.sources[name] {
			entry.Sources = append(entry.Sources, source)
		}
		sort.Slice(entry.Sources, func(i, j int) bool { return entry.Sources[i] < entry.Sources[j] })
		table.Nets = append(table.Nets, entry)
	}
	return table
}

func generatedNetAssignmentIssue(path string, message string) reports.Issue {
	return reports.Issue{
		Code:     reports.CodeValidationFailed,
		Severity: reports.SeverityBlocked,
		Path:     "generated_net_assignment." + strings.Trim(path, "."),
		Message:  message,
	}
}

func generatedNetTableFromNames(names ...string) GeneratedNetTable {
	collector := generatedNetCollector{sources: map[string]map[GeneratedNetAssignmentSource]struct{}{}}
	for _, name := range names {
		collector.add("test", name, GeneratedNetSourceGeneratedPCB)
	}
	return collector.table()
}

func generatedNetCode(table GeneratedNetTable, name string) (int, bool) {
	code, ok := table.ByName[strings.TrimSpace(name)]
	return code, ok
}

func placementNetNames(nets []placement.Net) []string {
	return collectGeneratedNetNames(nets, func(net placement.Net) string { return net.Name })
}

func routingNetNames(nets []routing.Net) []string {
	return collectGeneratedNetNames(nets, func(net routing.Net) string { return net.Name })
}

func collectGeneratedNetNames[T any](items []T, name func(T) string) []string {
	set := map[string]struct{}{}
	for _, item := range items {
		if value := strings.TrimSpace(name(item)); value != "" {
			set[value] = struct{}{}
		}
	}
	names := make([]string, 0, len(set))
	for name := range set {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
