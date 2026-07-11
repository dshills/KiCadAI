package designworkflow

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"kicadai/internal/blocks"
	"kicadai/internal/kicadfiles"
	"kicadai/internal/reports"
	"kicadai/internal/schematicir"
	"kicadai/internal/schematiclayout"
	"kicadai/internal/transactions"
)

const schematicLayoutTargetDelimiter = "__"

func layoutSchematicOperations(output blocks.CompositionOutput, layout schematicir.Layout) ([]transactions.Operation, string, error) {
	document, targetByRef, err := schematicLayoutDocument(output, layout)
	if err != nil {
		return nil, "", err
	}
	if issues := schematicir.Validate(document); reports.HasBlockingIssue(issues) {
		return nil, "", fmt.Errorf("validate schematic layout intent: %s", formatSchematicLayoutIssues(issues))
	}
	result := schematicir.LayoutDocument(document)
	if !result.Report.Passed {
		return nil, "", fmt.Errorf("schematic layout failed with %d errors: %s", result.Report.ErrorCount, formatSchematicLayoutDiagnostics(result.Diagnostics))
	}
	placed := make(map[string]schematiclayout.PlacedComponent, len(result.Components))
	for _, component := range result.Components {
		placed[component.Ref] = component
	}
	normalized := append([]transactions.Operation(nil), output.Operations...)
	for index, operation := range normalized {
		if operation.Op != transactions.OpAddSymbol {
			continue
		}
		var payload transactions.AddSymbolOperation
		if err := json.Unmarshal(operation.Raw, &payload); err != nil {
			return nil, "", fmt.Errorf("decode add_symbol operation %d: %w", index, err)
		}
		target := targetByRef[payload.Ref]
		component, ok := placed[target]
		if !ok {
			return nil, "", fmt.Errorf("schematic layout did not place target %s for %s", target, payload.Ref)
		}
		at := schematicLayoutTransactionPoint(component.PlacedAt)
		deltaX := at.XMM - payload.At.XMM
		deltaY := at.YMM - payload.At.YMM
		payload.At = at
		for propertyIndex := range payload.Properties {
			property := &payload.Properties[propertyIndex]
			if property.At == nil {
				continue
			}
			moved := transactions.Point{XMM: property.At.XMM + deltaX, YMM: property.At.YMM + deltaY}
			property.At = &moved
		}
		payload.Properties = schematicLayoutTextProperties(payload, component)
		updated, err := workflowOperation(transactions.OpAddSymbol, payload)
		if err != nil {
			return nil, "", fmt.Errorf("encode laid-out add_symbol operation %d: %w", index, err)
		}
		normalized[index] = updated
	}
	return normalized, result.Report.SelectedPaper, nil
}

func schematicLayoutDocument(output blocks.CompositionOutput, layout schematicir.Layout) (schematicir.Document, map[string]string, error) {
	document := *schematicir.NewDocument()
	document.Metadata.Name = "generated_schematic_layout"
	document.Layout = schematicir.CloneLayout(layout)
	document.Policy.Acceptance = schematicir.AcceptanceReadable
	document.Policy.Repair.AllowLabelInsertion = true
	document.Policy.Repair.AllowGroupSpacingAdjustment = true

	instanceByRef := map[string]string{}
	for _, instance := range output.Instances {
		for _, ref := range instance.Refs {
			if owner, exists := instanceByRef[ref]; exists {
				return document, nil, fmt.Errorf("schematic reference %s belongs to both %s and %s", ref, owner, instance.InstanceID)
			}
			instanceByRef[ref] = instance.InstanceID
		}
	}
	targetByRef := map[string]string{}
	refByTarget := map[string]string{}
	for index, operation := range output.Operations {
		if operation.Op != transactions.OpAddSymbol {
			continue
		}
		var payload transactions.AddSymbolOperation
		if err := json.Unmarshal(operation.Raw, &payload); err != nil {
			return document, nil, fmt.Errorf("decode add_symbol operation %d: %w", index, err)
		}
		instanceID := strings.TrimSpace(instanceByRef[payload.Ref])
		role := strings.TrimSpace(payload.Role)
		if instanceID == "" || role == "" {
			return document, nil, fmt.Errorf("symbol %s requires an owning instance and role for layout", payload.Ref)
		}
		target, err := schematicLayoutTarget(instanceID, role)
		if err != nil {
			return document, nil, err
		}
		if existingRef := refByTarget[target]; existingRef != "" {
			return document, nil, fmt.Errorf("schematic layout target %s maps to both %s and %s", target, existingRef, payload.Ref)
		}
		targetByRef[payload.Ref] = target
		refByTarget[target] = payload.Ref
		unit := ""
		if payload.Unit > 0 {
			unit = strconv.Itoa(payload.Unit)
		}
		component := schematicir.Component{
			ID:     target,
			Ref:    payload.Ref,
			Unit:   unit,
			Role:   schematicLayoutComponentRole(role),
			Symbol: payload.LibraryID,
			Value:  payload.Value,
		}
		for _, pin := range payload.Pins {
			x, y := pin.XMM, pin.YMM
			component.Pins = append(component.Pins, schematicir.Pin{Number: pin.Number, OffsetXMM: &x, OffsetYMM: &y})
		}
		if len(payload.Properties) != 0 {
			component.Properties = map[string]string{}
			for _, property := range payload.Properties {
				if name := strings.TrimSpace(property.Name); name != "" {
					component.Properties[name] = property.Value
				}
			}
		}
		document.Circuit.Components = append(document.Circuit.Components, component)
	}
	nets, err := schematicLayoutNets(output.Operations, targetByRef)
	if err != nil {
		return document, nil, err
	}
	document.Circuit.Nets = nets
	return document, targetByRef, nil
}

func schematicLayoutTarget(instanceID, role string) (string, error) {
	instanceID = strings.TrimSpace(instanceID)
	role = strings.TrimSpace(role)
	if strings.Contains(instanceID, schematicLayoutTargetDelimiter) || strings.Contains(role, schematicLayoutTargetDelimiter) {
		return "", fmt.Errorf("schematic layout instance and role IDs cannot contain reserved delimiter __: %s, %s", instanceID, role)
	}
	return instanceID + schematicLayoutTargetDelimiter + role, nil
}

func schematicLayoutComponentRole(role string) schematicir.ComponentRole {
	normalized := strings.ToLower(strings.TrimSpace(role))
	switch {
	case strings.Contains(normalized, "usb_c_receptacle"):
		return schematicir.ComponentRoleInputConnector
	case normalized == "connector" || strings.Contains(normalized, "output_connector"):
		return schematicir.ComponentRoleOutputConnector
	case strings.Contains(normalized, "regulator"):
		return schematicir.ComponentRoleRegulator
	case normalized == "sensor":
		return schematicir.ComponentRoleSensor
	case strings.Contains(normalized, "pullup"):
		return schematicir.ComponentRolePullup
	case strings.Contains(normalized, "decoupling") || strings.Contains(normalized, "input_capacitor") || strings.Contains(normalized, "output_capacitor"):
		return schematicir.ComponentRoleDecouplingCapacitor
	case strings.Contains(normalized, "capacitor"):
		return schematicir.ComponentRoleCapacitor
	case strings.Contains(normalized, "resistor") || strings.HasSuffix(normalized, "_rd"):
		return schematicir.ComponentRoleResistor
	default:
		return schematicir.ComponentRoleGeneric
	}
}

func schematicLayoutNets(operations []transactions.Operation, targetByRef map[string]string) ([]schematicir.Net, error) {
	byName := map[string]*schematicir.Net{}
	endpointSeen := map[string]map[schematicir.EndpointRef]struct{}{}
	for index, operation := range operations {
		if operation.Op != transactions.OpConnect {
			continue
		}
		var payload transactions.ConnectOperation
		if err := json.Unmarshal(operation.Raw, &payload); err != nil {
			return nil, fmt.Errorf("decode connect operation %d: %w", index, err)
		}
		name := strings.TrimSpace(payload.NetName)
		if name == "" {
			continue
		}
		net := byName[name]
		if net == nil {
			net = &schematicir.Net{Name: name, Role: schematicLayoutNetRole(name)}
			byName[name] = net
			endpointSeen[name] = map[schematicir.EndpointRef]struct{}{}
		}
		if payload.UseLabels != nil {
			value := *payload.UseLabels
			net.UseLabel = &value
		}
		for _, endpoint := range []transactions.Endpoint{payload.From, payload.To} {
			target := targetByRef[endpoint.Ref]
			if target == "" || strings.TrimSpace(endpoint.Pin) == "" {
				continue
			}
			ref := schematicir.EndpointRef(target + "." + strings.TrimSpace(endpoint.Pin))
			if _, exists := endpointSeen[name][ref]; exists {
				continue
			}
			endpointSeen[name][ref] = struct{}{}
			net.Connect = append(net.Connect, ref)
		}
	}
	names := make([]string, 0, len(byName))
	for name := range byName {
		names = append(names, name)
	}
	sort.Strings(names)
	nets := make([]schematicir.Net, 0, len(names))
	for _, name := range names {
		if len(byName[name].Connect) >= 2 {
			nets = append(nets, *byName[name])
		}
	}
	return nets, nil
}

func schematicLayoutNetRole(name string) schematicir.NetRole {
	normalized := strings.ToLower(strings.TrimSpace(name))
	switch {
	case normalized == "gnd" || strings.Contains(normalized, "ground"):
		return schematicir.NetRoleGround
	case strings.Contains(normalized, "vcc") || strings.Contains(normalized, "vdd") || strings.Contains(normalized, "vin"):
		return schematicir.NetRolePowerPos
	case strings.Contains(normalized, "sda") || strings.Contains(normalized, "scl"):
		return schematicir.NetRoleBus
	default:
		return schematicir.NetRoleSignal
	}
}

func schematicLayoutTextProperties(payload transactions.AddSymbolOperation, component schematiclayout.PlacedComponent) []transactions.SymbolProperty {
	properties := append([]transactions.SymbolProperty(nil), payload.Properties...)
	rotation := 0.0
	doNotAutoplace := true
	showName := false
	desired := []transactions.SymbolProperty{
		{Name: "Reference", Value: payload.Ref, ShowName: &showName, DoNotAutoplace: &doNotAutoplace, At: schematicLayoutTextPoint(component, component.ReferenceText), Rotation: &rotation},
		{Name: "Value", Value: payload.Value, ShowName: &showName, DoNotAutoplace: &doNotAutoplace, At: schematicLayoutTextPoint(component, component.ValueText), Rotation: &rotation},
	}
	for _, property := range desired {
		if property.At == nil {
			continue
		}
		replaced := false
		for index := range properties {
			if strings.EqualFold(strings.TrimSpace(properties[index].Name), property.Name) {
				properties[index] = property
				replaced = true
				break
			}
		}
		if !replaced {
			properties = append(properties, property)
		}
	}
	return properties
}

func schematicLayoutTextPoint(component schematiclayout.PlacedComponent, text schematiclayout.TextBox) *transactions.Point {
	if text.Box.Empty() {
		return nil
	}
	point := schematiclayoutPointToTransaction(component.PlacedAt.X+text.At.X, component.PlacedAt.Y+text.At.Y)
	return &point
}

func schematicLayoutTransactionPoint(point kicadfiles.Point) transactions.Point {
	return schematiclayoutPointToTransaction(point.X, point.Y)
}

func schematiclayoutPointToTransaction(x, y kicadfiles.IU) transactions.Point {
	return transactions.Point{XMM: float64(x) / 1_000_000, YMM: float64(y) / 1_000_000}
}

func formatSchematicLayoutIssues(issues []reports.Issue) string {
	parts := make([]string, 0, len(issues))
	for _, issue := range issues {
		if issue.Severity == reports.SeverityInfo {
			continue
		}
		parts = append(parts, issue.Path+": "+issue.Message)
	}
	return strings.Join(parts, "; ")
}

func formatSchematicLayoutDiagnostics(diagnostics []schematiclayout.Diagnostic) string {
	parts := make([]string, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		if diagnostic.Severity != schematiclayout.SeverityError {
			continue
		}
		parts = append(parts, diagnostic.Code+": "+diagnostic.Message)
	}
	return strings.Join(parts, "; ")
}

func applySchematicPaper(tx transactions.Transaction, paper string) (transactions.Transaction, error) {
	if paper == "" {
		return tx, nil
	}
	for index, operation := range tx.Operations {
		if operation.Op != transactions.OpCreateProject {
			continue
		}
		var payload transactions.CreateProjectOperation
		if err := json.Unmarshal(operation.Raw, &payload); err != nil {
			return tx, fmt.Errorf("decode create_project operation: %w", err)
		}
		payload.Paper = paper
		updated, err := workflowOperation(transactions.OpCreateProject, payload)
		if err != nil {
			return tx, fmt.Errorf("encode create_project operation: %w", err)
		}
		tx.Operations[index] = updated
		return tx, nil
	}
	return tx, fmt.Errorf("schematic transaction is missing create_project operation")
}
