package schematiclayout

import "testing"

func TestClassifyOrdersSignalFlowStages(t *testing.T) {
	request := Classify(Request{Components: []Component{
		{Ref: "J2", Value: "HEADPHONE_OUT", LibraryID: "Connector:Conn_01x03_Pin"},
		{Ref: "U1", Value: "OPAMP", LibraryID: "Amplifier_Operational:LMV321"},
		{Ref: "J1", Value: "AUDIO_IN", LibraryID: "Connector:Conn_01x03_Pin"},
	}})
	if request.Components[0].Ref != "J1" || request.Components[1].Ref != "U1" || request.Components[2].Ref != "J2" {
		t.Fatalf("component order = %#v, want input, processing, output", []string{request.Components[0].Ref, request.Components[1].Ref, request.Components[2].Ref})
	}
}

func TestClassifyAssignsPowerAndGroundLanes(t *testing.T) {
	request := Classify(Request{Components: []Component{
		{Ref: "#PWR01", Value: "VCC", LibraryID: "power:VCC"},
		{Ref: "#PWR02", Value: "GND", LibraryID: "power:GND"},
		{Ref: "#PWR03", Value: "VEE", LibraryID: "power:VEE"},
	}})
	lanes := map[string]Lane{}
	for _, component := range request.Components {
		lanes[component.Ref] = component.Lane
	}
	if lanes["#PWR01"] != LanePositiveRail || lanes["#PWR02"] != LaneGround || lanes["#PWR03"] != LaneNegativeRail {
		t.Fatalf("lanes = %#v", lanes)
	}
}

func TestClassifyInfersFeedbackAndBias(t *testing.T) {
	request := Classify(Request{Components: []Component{
		{Ref: "R1", Value: "FEEDBACK"},
		{Ref: "R2", Value: "BIAS_REF"},
	}})
	roles := map[string]string{}
	for _, component := range request.Components {
		roles[component.Ref] = component.Role
	}
	if roles["R1"] != "feedback" || roles["R2"] != "bias" {
		t.Fatalf("roles = %#v", roles)
	}
}

func TestClassifyInfersNetRoles(t *testing.T) {
	request := Classify(Request{Nets: []Net{
		{Name: "GND"},
		{Name: "VCC"},
		{Name: "AUDIO_IN"},
		{Name: "HP_OUT"},
		{Name: "SDA"},
	}})
	roles := map[string]string{}
	for _, net := range request.Nets {
		roles[net.Name] = net.Role
	}
	if roles["GND"] != "ground" || roles["VCC"] != "power" || roles["AUDIO_IN"] != "input_signal" || roles["HP_OUT"] != "output_signal" || roles["SDA"] != "bus" {
		t.Fatalf("roles = %#v", roles)
	}
}
