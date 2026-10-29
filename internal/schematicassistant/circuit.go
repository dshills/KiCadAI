package schematicassistant

import (
	"fmt"
	"kicadai/internal/schematicir"
	"strconv"
)

func Circuit(s Spec) (schematicir.Document, error) {
	if _, err := Analyze(s); err != nil {
		return schematicir.Document{}, err
	}
	component := func(id, ref string, role schematicir.ComponentRole, symbol, value string, pins int) schematicir.Component {
		c := schematicir.Component{ID: id, Ref: ref, Role: role, Symbol: symbol, Value: value}
		for i := 1; i <= pins; i++ {
			c.Pins = append(c.Pins, schematicir.Pin{Number: strconv.Itoa(i)})
		}
		if pins == 1 {
			zero := 0.0
			c.Pins[0].OffsetXMM = &zero
			c.Pins[0].OffsetYMM = &zero
		}
		return c
	}
	net := func(name string, role schematicir.NetRole, ends ...schematicir.EndpointRef) schematicir.Net {
		return schematicir.Net{Name: name, Role: role, Connect: ends, Label: name}
	}
	doc := *schematicir.NewDocument()
	doc.Metadata.Name = s.Name
	doc.Metadata.Title = fmt.Sprintf("%s — confirmed passive circuit", s.Family)
	doc.Metadata.Seed = "schematic-assistant-v1"
	doc.Circuit.Components = []schematicir.Component{
		component("input", "TP1", schematicir.ComponentRoleTestpoint, "Connector:TestPoint", fmt.Sprintf("VIN %.9g V", *s.InputV), 1),
		component("r1", "R1", schematicir.ComponentRoleResistor, "Device:R", strconv.FormatFloat(*s.R1, 'g', -1, 64)+"ohm", 2),
		component("output", "TP2", schematicir.ComponentRoleTestpoint, "Connector:TestPoint", "VOUT", 1),
		component("ground", "#PWR01", schematicir.ComponentRoleGroundSymbol, "power:GND", "GND", 1),
		component("ground_flag", "#FLG01", schematicir.ComponentRoleGroundSymbol, "power:PWR_FLAG", "PWR_FLAG", 1),
	}
	processing := []string{"r1", "ground", "ground_flag"}
	doc.Circuit.Nets = []schematicir.Net{net("VIN", schematicir.NetRoleSignal, "input.1", "r1.1")}
	out := net("VOUT", schematicir.NetRoleSignal, "r1.2", "output.1")
	ground := net("GND", schematicir.NetRoleGround, "ground.1", "ground_flag.1")
	noLabel := false
	ground.UseLabel = &noLabel
	if s.Family == "divider" {
		doc.Circuit.Components = append(doc.Circuit.Components, component("r2", "R2", schematicir.ComponentRoleResistor, "Device:R", strconv.FormatFloat(*s.R2, 'g', -1, 64)+"ohm", 2))
		processing = append(processing, "r2")
		out.Connect = append(out.Connect, "r2.1")
		ground.Connect = append(ground.Connect, "r2.2")
	} else {
		doc.Circuit.Components = append(doc.Circuit.Components, component("c1", "C1", schematicir.ComponentRoleCapacitor, "Device:C", strconv.FormatFloat(*s.C, 'g', -1, 64)+"F", 2))
		processing = append(processing, "c1")
		out.Connect = append(out.Connect, "c1.1")
		ground.Connect = append(ground.Connect, "c1.2")
	}
	if *s.Load > 0 {
		doc.Circuit.Components = append(doc.Circuit.Components, component("load", "R3", schematicir.ComponentRoleResistor, "Device:R", strconv.FormatFloat(*s.Load, 'g', -1, 64)+"ohm", 2))
		processing = append(processing, "load")
		out.Connect = append(out.Connect, "load.1")
		ground.Connect = append(ground.Connect, "load.2")
	}
	doc.Circuit.Nets = append(doc.Circuit.Nets, out, ground)
	doc.Layout.Groups = []schematicir.Group{{ID: "input", Label: "Input", Role: schematicir.GroupRoleInputStage, Members: []string{"input"}, Rank: 0, Side: schematicir.SideLeft}, {ID: "passive", Label: "Passive network", Role: schematicir.GroupRoleProcessingStage, Members: processing, Rank: 1}, {ID: "output", Label: "Output", Role: schematicir.GroupRoleOutputStage, Members: []string{"output"}, Rank: 2, Side: schematicir.SideRight}}
	doc.Layout.Rules.PositivePowerTop = &noLabel
	doc.Layout.Rules.GroundBottom = &noLabel
	doc.Layout.Rules.PreferLabelsForLongNets = &noLabel
	doc.Policy.Acceptance = schematicir.AcceptanceReadable
	return doc, nil
}
