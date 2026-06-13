package blocks

import (
	"encoding/json"
	"slices"
	"strconv"

	"kicadai/internal/libraryresolver"
	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

func ValidateOutputLibraries(output BlockOutput, index *libraryresolver.LibraryIndex) []reports.Issue {
	if index == nil {
		return []reports.Issue{{
			Code:       reports.CodeUnknownSymbolLibrary,
			Severity:   reports.SeverityWarning,
			Path:       "block.library",
			Message:    "library resolver roots are not configured; block library validation was skipped",
			Suggestion: "configure KiCad symbol and footprint roots before fabrication readiness checks",
		}}
	}
	symbolsByRef := map[string]string{}
	footprintsByRef := map[string]string{}
	var issues []reports.Issue
	connectWarningRefs := map[string]struct{}{}
	for i, operation := range output.Operations {
		switch operation.Op {
		case transactions.OpAddSymbol:
			var payload transactions.AddSymbolOperation
			if err := decodeBlockOperation(operation, &payload); err != nil {
				issues = append(issues, blockLibraryIssue(reports.CodeInvalidArgument, i, "raw", "failed to decode add_symbol operation: "+err.Error()))
				continue
			}
			symbolsByRef[payload.Ref] = payload.LibraryID
			if _, ok := libraryresolver.ResolveSymbol(*index, payload.LibraryID); !ok {
				issues = append(issues, blockLibraryIssue(reports.CodeMissingFile, i, "library_id", "symbol library record not found: "+payload.LibraryID))
			}
		case transactions.OpAssignFootprint:
			var payload transactions.AssignFootprintOperation
			if err := decodeBlockOperation(operation, &payload); err != nil {
				issues = append(issues, blockLibraryIssue(reports.CodeInvalidArgument, i, "raw", "failed to decode assign_footprint operation: "+err.Error()))
				continue
			}
			footprintsByRef[payload.Ref] = payload.FootprintID
			if _, ok := libraryresolver.ResolveFootprint(*index, payload.FootprintID); !ok {
				issues = append(issues, blockLibraryIssue(reports.CodeMissingFile, i, "footprint_id", "footprint library record not found: "+payload.FootprintID))
			}
		case transactions.OpPlaceFootprint:
			var payload transactions.PlaceFootprintOperation
			if err := decodeBlockOperation(operation, &payload); err != nil {
				issues = append(issues, blockLibraryIssue(reports.CodeInvalidArgument, i, "raw", "failed to decode place_footprint operation: "+err.Error()))
				continue
			}
			if payload.FootprintID == "" {
				issues = append(issues, blockLibraryIssue(reports.CodeMissingFile, i, "footprint_id", "place_footprint operation has no footprint_id"))
				continue
			}
			footprintsByRef[payload.Ref] = payload.FootprintID
			if _, ok := libraryresolver.ResolveFootprint(*index, payload.FootprintID); !ok {
				issues = append(issues, blockLibraryIssue(reports.CodeMissingFile, i, "footprint_id", "footprint library record not found: "+payload.FootprintID))
			}
		case transactions.OpConnect:
			var payload transactions.ConnectOperation
			if err := decodeBlockOperation(operation, &payload); err != nil {
				issues = append(issues, blockLibraryIssue(reports.CodeInvalidArgument, i, "raw", "failed to decode connect operation: "+err.Error()))
				continue
			}
			connectWarningRefs[payload.From.Ref] = struct{}{}
			connectWarningRefs[payload.To.Ref] = struct{}{}
		}
	}
	if len(connectWarningRefs) != 0 {
		issues = append(issues, reports.Issue{
			Code:       reports.CodePinmapUnverified,
			Severity:   reports.SeverityWarning,
			Path:       "block.connections",
			Message:    "connect operations require verified pinmaps before fabrication export",
			Refs:       sortedStringSet(connectWarningRefs),
			Suggestion: "validate symbol-footprint pinmaps before treating block output as fabrication-ready",
		})
	}
	refs := make([]string, 0, len(symbolsByRef))
	for ref := range symbolsByRef {
		refs = append(refs, ref)
	}
	slices.Sort(refs)
	for _, ref := range refs {
		symbolID := symbolsByRef[ref]
		footprintID := footprintsByRef[ref]
		if footprintID == "" {
			continue
		}
		result := libraryresolver.ValidateAssignment(*index, symbolID, footprintID)
		for _, issue := range result.Issues {
			issue.Path = "block.refs." + ref
			issue.Refs = append(slices.Clone(issue.Refs), ref)
			issues = append(issues, issue)
		}
	}
	return issues
}

func sortedStringSet(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	slices.Sort(result)
	return result
}

func decodeBlockOperation(operation transactions.Operation, target any) error {
	return json.Unmarshal(operation.Raw, target)
}

func blockLibraryIssue(code reports.Code, index int, field string, message string) reports.Issue {
	suggestion := "run library search commands or configure KiCad library roots"
	if code == reports.CodeInvalidArgument {
		suggestion = "inspect the generated block operation payload"
	}
	return reports.Issue{
		Code:       code,
		Severity:   reports.SeverityError,
		Path:       "operations[" + strconv.Itoa(index) + "]." + field,
		Message:    message,
		Suggestion: suggestion,
	}
}
