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
	Op          string        `json:"op"`
	Component   string        `json:"component,omitempty"`
	Net         string        `json:"net,omitempty"`
	Endpoint    *Endpoint     `json:"endpoint,omitempty"`
	Replacement *Endpoint     `json:"replacement,omitempty"`
	Region      string        `json:"region,omitempty"`
	Placement   *PCBPlacement `json:"placement,omitempty"`
	Bounds      *Bounds       `json:"bounds,omitempty"`
	Policy      string        `json:"policy,omitempty"`
	Enabled     *bool         `json:"enabled,omitempty"`
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
