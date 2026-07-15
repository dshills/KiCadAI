package circuitgraph

import (
	"bytes"
	"encoding/json"
	"io"
	"strconv"
	"strings"

	"kicadai/internal/reports"
)

const (
	PatchSchemaID                 = "kicadai.circuit-patch.v1"
	PatchVersion                  = 1
	MaxPatchBytes                 = 1 << 20
	MaxPatchOps                   = 256
	CodePatchInvalid reports.Code = "GRAPH_PATCH_INVALID"
)

type PatchDocument struct {
	Schema     string           `json:"schema"`
	Version    int              `json:"version"`
	Operations []PatchOperation `json:"operations"`
}

type PatchOperation struct {
	Op             string          `json:"op"`
	Component      string          `json:"component,omitempty"`
	Net            string          `json:"net,omitempty"`
	Endpoint       *Endpoint       `json:"endpoint,omitempty"`
	Replacement    *Endpoint       `json:"replacement,omitempty"`
	Region         string          `json:"region,omitempty"`
	Placement      *PCBPlacement   `json:"placement,omitempty"`
	Bounds         *Bounds         `json:"bounds,omitempty"`
	Policy         string          `json:"policy,omitempty"`
	Enabled        *bool           `json:"enabled,omitempty"`
	ComponentPatch *ComponentPatch `json:"component_patch,omitempty"`
}

type ComponentPatch struct {
	ComponentID *string            `json:"component_id,omitempty"`
	VariantID   *string            `json:"variant_id,omitempty"`
	Query       *ComponentQuery    `json:"query,omitempty"`
	Value       *string            `json:"value,omitempty"`
	Symbol      *LibraryConstraint `json:"symbol,omitempty"`
	Footprint   *LibraryConstraint `json:"footprint,omitempty"`
	Units       *[]ComponentUnit   `json:"units,omitempty"`
}

// ApplyPatch mutates only an internal clone. The corrected graph is validated
// through DecodeStrict so normal graph invariants remain the final authority.
func ApplyPatch(document Document, patch PatchDocument) (Document, []reports.Issue) {
	if issues := ValidatePatch(patch); reports.HasBlockingIssue(issues) {
		return Document{}, issues
	}
	working := cloneDocument(document)
	for index, operation := range patch.Operations {
		if issue := applyPatchOperation(&working, operation); issue != nil {
			issue.Path = "patch.operations[" + strconv.Itoa(index) + "]"
			return Document{}, []reports.Issue{*issue}
		}
	}
	encoded, err := json.Marshal(working)
	if err != nil {
		return Document{}, []reports.Issue{graphIssue(CodePatchInvalid, "patch", "encode corrected graph: "+err.Error())}
	}
	corrected, issues := DecodeStrict(bytes.NewReader(encoded))
	return corrected, issues
}

func applyPatchOperation(document *Document, operation PatchOperation) *reports.Issue {
	findNet := func(name string) *Net {
		for i := range document.Nets {
			if document.Nets[i].Name == name {
				return &document.Nets[i]
			}
		}
		return nil
	}
	contains := func(entries []Endpoint, endpoint Endpoint) int {
		for i := range entries {
			if compareEndpoints(entries[i], endpoint) == 0 {
				return i
			}
		}
		return -1
	}
	switch operation.Op {
	case "replace_component":
		for i := range document.Components {
			if document.Components[i].ID != operation.Component {
				continue
			}
			patch := operation.ComponentPatch
			if patch.ComponentID != nil {
				document.Components[i].ComponentID = *patch.ComponentID
				document.Components[i].Query = nil
			}
			if patch.Query != nil {
				document.Components[i].Query = patch.Query
				document.Components[i].ComponentID = ""
				document.Components[i].VariantID = ""
			}
			if patch.VariantID != nil {
				document.Components[i].VariantID = *patch.VariantID
			}
			if patch.Value != nil {
				document.Components[i].Value = *patch.Value
			}
			if patch.Symbol != nil {
				document.Components[i].Symbol = patch.Symbol
			}
			if patch.Footprint != nil {
				document.Components[i].Footprint = patch.Footprint
			}
			if patch.Units != nil {
				document.Components[i].Units = append([]ComponentUnit(nil), (*patch.Units)...)
			}
			return nil
		}
		return issuePatch("component", "component does not exist")
	case "replace_endpoint":
		net := findNet(operation.Net)
		if net == nil {
			return issuePatch("net", "target net does not exist")
		}
		index := contains(net.Endpoints, *operation.Endpoint)
		if index < 0 {
			return issuePatch("endpoint", "target endpoint does not exist")
		}
		net.Endpoints[index] = *operation.Replacement
	case "add_net_endpoint":
		net := findNet(operation.Net)
		if net == nil {
			return issuePatch("net", "target net does not exist")
		}
		if contains(net.Endpoints, *operation.Endpoint) >= 0 {
			return issuePatch("endpoint", "endpoint already exists")
		}
		net.Endpoints = append(net.Endpoints, *operation.Endpoint)
	case "remove_net_endpoint":
		net := findNet(operation.Net)
		if net == nil {
			return issuePatch("net", "target net does not exist")
		}
		index := contains(net.Endpoints, *operation.Endpoint)
		if index < 0 {
			return issuePatch("endpoint", "target endpoint does not exist")
		}
		net.Endpoints = append(net.Endpoints[:index], net.Endpoints[index+1:]...)
	case "add_no_connect":
		if contains(document.NoConnects, *operation.Endpoint) >= 0 {
			return issuePatch("endpoint", "no-connect already exists")
		}
		document.NoConnects = append(document.NoConnects, *operation.Endpoint)
	case "remove_no_connect":
		index := contains(document.NoConnects, *operation.Endpoint)
		if index < 0 {
			return issuePatch("endpoint", "no-connect does not exist")
		}
		document.NoConnects = append(document.NoConnects[:index], document.NoConnects[index+1:]...)
	case "replace_pcb_placement":
		for i := range document.PCB.Placements {
			if document.PCB.Placements[i].Component == operation.Component {
				replacement := *operation.Placement
				replacement.Component = operation.Component
				document.PCB.Placements[i] = replacement
				return nil
			}
		}
		return issuePatch("component", "PCB placement does not exist")
	case "replace_pcb_region":
		for i := range document.PCB.Regions {
			if document.PCB.Regions[i].ID == operation.Region {
				document.PCB.Regions[i].Bounds = *operation.Bounds
				return nil
			}
		}
		return issuePatch("region", "PCB region does not exist")
	case "replace_policy":
		switch operation.Policy {
		case "allow_reference_assignment":
			document.Policy.AllowReferenceAssignment = operation.Enabled
		case "allow_value_normalization":
			document.Policy.AllowValueNormalization = operation.Enabled
		case "allow_layout_inference":
			document.Policy.AllowLayoutInference = operation.Enabled
		case "allow_spacing_adjustment":
			document.Policy.AllowSpacingAdjustment = operation.Enabled
		case "allow_label_insertion":
			document.Policy.AllowLabelInsertion = operation.Enabled
		case "allow_placement_adjustment":
			document.Policy.AllowPlacementAdjustment = operation.Enabled
		case "allow_route_retry":
			document.Policy.AllowRouteRetry = operation.Enabled
		}
	default:
		return issuePatch("op", "unsupported patch operation")
	}
	return nil
}

func issuePatch(path, message string) *reports.Issue {
	issue := graphIssue(CodePatchInvalid, path, message)
	return &issue
}

func DecodePatchStrict(reader io.Reader) (PatchDocument, []reports.Issue) {
	var buffer bytes.Buffer
	if _, err := io.Copy(&buffer, io.LimitReader(reader, MaxPatchBytes+1)); err != nil {
		return PatchDocument{}, []reports.Issue{graphIssue(CodePatchInvalid, "patch", "read circuit patch: "+err.Error())}
	}
	if buffer.Len() > MaxPatchBytes {
		return PatchDocument{}, []reports.Issue{graphIssue(CodeLimitExceeded, "patch", "circuit patch exceeds maximum encoded size")}
	}
	decoder := json.NewDecoder(bytes.NewReader(buffer.Bytes()))
	decoder.DisallowUnknownFields()
	var patch PatchDocument
	if err := decoder.Decode(&patch); err != nil {
		return PatchDocument{}, []reports.Issue{graphIssue(CodePatchInvalid, "patch", "decode circuit patch: "+err.Error())}
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return PatchDocument{}, []reports.Issue{graphIssue(CodePatchInvalid, "patch", "circuit patch contains trailing JSON value")}
	}
	return patch, ValidatePatch(patch)
}

func ValidatePatch(patch PatchDocument) []reports.Issue {
	var issues []reports.Issue
	if patch.Schema != PatchSchemaID {
		issues = append(issues, graphIssue(CodePatchInvalid, "patch.schema", "schema must be "+PatchSchemaID))
	}
	if patch.Version != PatchVersion {
		issues = append(issues, graphIssue(CodePatchInvalid, "patch.version", "version must be 1"))
	}
	if len(patch.Operations) == 0 || len(patch.Operations) > MaxPatchOps {
		issues = append(issues, graphIssue(CodePatchInvalid, "patch.operations", "operation count must be between 1 and 256"))
	}
	for index, operation := range patch.Operations {
		path := "patch.operations[" + strconvItoa(index) + "]"
		switch operation.Op {
		case "replace_component":
			if operation.Component == "" || operation.ComponentPatch == nil {
				issues = append(issues, graphIssue(CodePatchInvalid, path, "replace_component requires component and component_patch"))
				continue
			}
			selectionCount := 0
			if operation.ComponentPatch.ComponentID != nil {
				selectionCount++
			}
			if operation.ComponentPatch.Query != nil {
				selectionCount++
			}
			if selectionCount != 1 {
				issues = append(issues, graphIssue(CodePatchInvalid, path+".component_patch", "replace_component requires exactly one of component_id or query"))
			}
		case "replace_endpoint":
			if operation.Net == "" || operation.Endpoint == nil || operation.Replacement == nil || operation.Component != "" || operation.Region != "" {
				issues = append(issues, graphIssue(CodePatchInvalid, path, "replace_endpoint requires net, endpoint, and replacement only"))
				continue
			}
			if operation.Endpoint.Component != operation.Replacement.Component {
				issues = append(issues, graphIssue(CodePatchInvalid, path+".replacement.component", "replacement endpoint must remain on the same component"))
			}
		case "add_net_endpoint", "remove_net_endpoint", "add_no_connect", "remove_no_connect":
			if operation.Endpoint == nil || (strings.HasSuffix(operation.Op, "net_endpoint") && operation.Net == "") {
				issues = append(issues, graphIssue(CodePatchInvalid, path, operation.Op+" requires its typed endpoint and net when applicable"))
			}
		case "replace_pcb_placement":
			if operation.Component == "" || operation.Placement == nil {
				issues = append(issues, graphIssue(CodePatchInvalid, path, "replace_pcb_placement requires component and placement"))
			}
		case "replace_pcb_region":
			if operation.Region == "" || operation.Bounds == nil {
				issues = append(issues, graphIssue(CodePatchInvalid, path, "replace_pcb_region requires region and bounds"))
			}
		case "replace_policy":
			if !patchPolicyName(operation.Policy) || operation.Enabled == nil {
				issues = append(issues, graphIssue(CodePatchInvalid, path, "replace_policy requires a supported policy name and enabled bool"))
			}
		default:
			issues = append(issues, graphIssue(CodePatchInvalid, path+".op", "unsupported patch operation "+operation.Op))
		}
	}
	return finalizeGraphIssues(issues)
}

func patchPolicyName(name string) bool {
	switch name {
	case "allow_reference_assignment", "allow_value_normalization", "allow_layout_inference", "allow_spacing_adjustment", "allow_label_insertion", "allow_placement_adjustment", "allow_route_retry":
		return true
	default:
		return false
	}
}

func strconvItoa(value int) string {
	return strconv.Itoa(value)
}
