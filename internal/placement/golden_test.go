package placement

import "testing"

func TestGoldenConnectorEdgePlacement(t *testing.T) {
	req := Request{
		Board: BoardPlacementArea{WidthMM: 60, HeightMM: 30, MarginMM: 1},
		Components: []Component{
			{
				Ref:         "J1",
				FootprintID: "Connector_Generic:Conn_01x04",
				Bounds:      Bounds{WidthMM: 1, HeightMM: 1, AnchorOffset: Point{XMM: 0.5, YMM: 0.5}, Source: BoundsExplicit},
				Edge:        EdgeLeft,
				Pads:        []PadSummary{{Name: "1"}, {Name: "2"}, {Name: "3"}, {Name: "4"}},
			},
		},
		Rules: Rules{MaxCandidatesPerPart: 100000},
	}

	result := Place(req)
	if result.Status != StatusPlaced {
		t.Fatalf("status = %s, want placed; issues=%#v", result.Status, result.Issues)
	}
	quality := BuildQualityReport(req, result)
	if quality.EdgeConstraintSatisfied != 1 {
		t.Fatalf("edge constraints = %d/%d, want 1/1; placement=%#v", quality.EdgeConstraintSatisfied, quality.EdgeConstraintCount, result.Placements)
	}
}

func TestGoldenDecouplingPlacementNearIC(t *testing.T) {
	req := Request{
		Board: BoardPlacementArea{WidthMM: 50, HeightMM: 25, MarginMM: 1},
		Components: []Component{
			{
				Ref:         "U1",
				FootprintID: "Package_SO:SOIC-8",
				Bounds:      Bounds{WidthMM: 5, HeightMM: 4, AnchorOffset: Point{XMM: 2.5, YMM: 2}, Source: BoundsExplicit},
				Fixed:       true,
				Position:    &Placement{XMM: 35, YMM: 12, Layer: "F.Cu"},
				Pads:        []PadSummary{{Name: "8"}},
			},
			{
				Ref:         "C1",
				FootprintID: "Capacitor_SMD:C_0805_2012Metric",
				Bounds:      Bounds{WidthMM: 2, HeightMM: 1.25, AnchorOffset: Point{XMM: 1, YMM: 0.625}, Source: BoundsExplicit},
				Pads:        []PadSummary{{Name: "1"}},
			},
		},
		Nets: []Net{{Name: "VCC", Weight: 5, Endpoints: []Endpoint{{Ref: "U1", Pin: "8"}, {Ref: "C1", Pin: "1"}}}},
	}

	result := Place(req)
	if result.Status != StatusPlaced {
		t.Fatalf("status = %s, want placed; issues=%#v", result.Status, result.Issues)
	}
	placements := placementResultsByRef(result.Placements)
	if boardDistance(placements["U1"].Bounds.Center().XMM-placements["C1"].Bounds.Center().XMM, placements["U1"].Bounds.Center().YMM-placements["C1"].Bounds.Center().YMM) > 15 {
		t.Fatalf("C1 placed too far from U1: U1=%#v C1=%#v", placements["U1"].Position, placements["C1"].Position)
	}
}

func TestGoldenKeepoutAvoidancePlacement(t *testing.T) {
	req := minimalRequest()
	req.Keepouts = []Keepout{{
		ID:     "mounting",
		Bounds: Rect{Min: Point{XMM: 0, YMM: 0}, Max: Point{XMM: 8, YMM: 8}},
		Layers: []string{"F.Cu"},
	}}

	result := Place(req)
	if result.Status != StatusPlaced {
		t.Fatalf("status = %s, want placed; issues=%#v", result.Status, result.Issues)
	}
	if result.Placements[0].Bounds.Intersects(req.Keepouts[0].Bounds) {
		t.Fatalf("placement intersects keepout: placement=%#v keepout=%#v", result.Placements[0].Bounds, req.Keepouts[0].Bounds)
	}
	quality := BuildQualityReport(req, result)
	if quality.KeepoutCount != 1 || quality.GeometryIssueCount != 0 {
		t.Fatalf("quality keepout summary = %#v", quality)
	}
}

func TestGoldenBottomSidePlacement(t *testing.T) {
	req := minimalRequest()
	req.Rules.AllowBackLayer = true
	req.Components[0].Side = SideBottom

	result := Place(req)
	if result.Status != StatusPlaced {
		t.Fatalf("status = %s, want placed; issues=%#v", result.Status, result.Issues)
	}
	if result.Placements[0].Position.Layer != "B.Cu" {
		t.Fatalf("layer = %s, want B.Cu", result.Placements[0].Position.Layer)
	}
	quality := BuildQualityReport(req, result)
	if quality.SideConstraintSatisfied != 1 {
		t.Fatalf("side constraints = %d/%d, want 1/1", quality.SideConstraintSatisfied, quality.SideConstraintCount)
	}
}

func TestGoldenQualityReportsEstimatedBounds(t *testing.T) {
	req := minimalRequest()
	req.Components[0].Bounds.Source = BoundsEstimated

	result := Place(req)
	if result.Status != StatusPlaced {
		t.Fatalf("status = %s, want placed; issues=%#v", result.Status, result.Issues)
	}
	quality := BuildQualityReport(req, result)
	if quality.Metrics.EstimatedBoundsCount != 1 || len(quality.EstimatedBoundsRefs) != 1 || quality.EstimatedBoundsRefs[0] != "R1" {
		t.Fatalf("estimated bounds quality = %#v", quality)
	}
	if len(quality.PlacementQualityWarnings) == 0 {
		t.Fatalf("expected estimated bounds warning: %#v", quality)
	}
}

func TestGoldenPlacementHardeningCorpus(t *testing.T) {
	cases := []struct {
		name           string
		request        Request
		wantNetReports int
	}{
		{name: "led_indicator", request: goldenLEDIndicatorRequest(), wantNetReports: 1},
		{name: "voltage_regulator", request: goldenRegulatorRequest(), wantNetReports: 2},
		{name: "mcu_minimal", request: goldenMCURequest(), wantNetReports: 3},
		{name: "usb_c_power", request: goldenUSBCPowerRequest(), wantNetReports: 3},
		{name: "i2c_sensor", request: goldenI2CSensorRequest(), wantNetReports: 3},
		{name: "opamp_gain_stage", request: goldenOpAmpRequest(), wantNetReports: 2},
		{name: "connector_breakout", request: goldenConnectorBreakoutRequest(), wantNetReports: 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := Place(tc.request)
			if result.Status != StatusPlaced {
				t.Fatalf("status = %s, want placed; issues=%#v", result.Status, result.Issues)
			}
			quality := BuildQualityReport(tc.request, result)
			if quality.Metrics.PlacedCount != len(tc.request.Components) || len(quality.UnplacedRefs) != 0 {
				t.Errorf("placement summary = placed %d unplaced_count %d, want %d/0", quality.Metrics.PlacedCount, len(quality.UnplacedRefs), len(tc.request.Components))
			}
			if len(quality.ProximityReports) != len(tc.request.ProximityRules) {
				t.Errorf("proximity reports = %d, want %d", len(quality.ProximityReports), len(tc.request.ProximityRules))
			}
			if len(quality.RegionReports) != len(tc.request.RegionRules) {
				t.Errorf("region reports = %d, want %d", len(quality.RegionReports), len(tc.request.RegionRules))
			}
			if len(quality.NetReports) != tc.wantNetReports {
				t.Errorf("net reports = %d, want %d", len(quality.NetReports), tc.wantNetReports)
			}
			for _, dimension := range quality.Score.Dimensions {
				if dimension.Status == scoreStatusFail {
					t.Errorf("score dimension failed: %#v", dimension)
				}
			}
		})
	}
}

func goldenLEDIndicatorRequest() Request {
	req := goldenBaseRequest(40, 20)
	req.Components = []Component{
		goldenComponent("R1", "Resistor_SMD:R_0805_2012Metric", 2, 1.25),
		goldenComponent("D1", "LED_SMD:LED_0805_2012Metric", 2, 1.25),
	}
	req.Nets = []Net{{Name: "LED", Role: NetSignal, Weight: 2, Endpoints: []Endpoint{{Ref: "R1", Pin: "1"}, {Ref: "D1", Pin: "1"}}}}
	req.ProximityRules = []ProximityRule{{ID: "led-series", Role: IntentPowerPath, AnchorRef: "R1", TargetRefs: []string{"D1"}, MaxDistanceMM: 15, Required: true}}
	return req
}

func goldenRegulatorRequest() Request {
	req := goldenBaseRequest(50, 25)
	req.Components = []Component{
		goldenComponent("U1", "Package_TO_SOT_SMD:SOT-223-3_TabPin2", 6.5, 7, "1", "2", "3"),
		goldenComponent("CIN", "Capacitor_SMD:C_0805_2012Metric", 2, 1.25),
		goldenComponent("COUT", "Capacitor_SMD:C_0805_2012Metric", 2, 1.25),
	}
	req.Nets = []Net{
		{Name: "VIN", Role: NetPower, Weight: 3, Endpoints: []Endpoint{{Ref: "U1", Pin: "1"}, {Ref: "CIN", Pin: "1"}}},
		{Name: "VOUT", Role: NetPower, Weight: 3, Endpoints: []Endpoint{{Ref: "U1", Pin: "3"}, {Ref: "COUT", Pin: "1"}}},
	}
	req.RegionRules = []RegionRule{{ID: "power", Region: "power", Refs: []string{"U1", "CIN", "COUT"}, Preferred: Rect{Min: Point{XMM: 1, YMM: 1}, Max: Point{XMM: 30, YMM: 20}}}}
	req.ProximityRules = []ProximityRule{
		{ID: "input-cap", Role: IntentDecoupling, AnchorRef: "U1", TargetRefs: []string{"CIN"}, MaxDistanceMM: 15, Required: true},
		{ID: "output-cap", Role: IntentDecoupling, AnchorRef: "U1", TargetRefs: []string{"COUT"}, MaxDistanceMM: 15, Required: true},
	}
	return req
}

func goldenMCURequest() Request {
	req := goldenBaseRequest(60, 35)
	req.Components = []Component{
		goldenComponent("U1", "Package_QFP:TQFP-32_7x7mm_P0.8mm", 7, 7, "VCC", "RESET", "MOSI"),
		goldenComponent("C1", "Capacitor_SMD:C_0805_2012Metric", 2, 1.25),
		goldenComponent("RRESET", "Resistor_SMD:R_0805_2012Metric", 2, 1.25),
		goldenComponent("JPROG", "Connector_Generic:Conn_01x03", 4, 6, "1", "2", "3"),
	}
	req.Nets = []Net{
		{Name: "VCC", Role: NetPower, Weight: 5, Endpoints: []Endpoint{{Ref: "U1", Pin: "VCC"}, {Ref: "C1", Pin: "1"}}},
		{Name: "RESET", Role: NetSignal, Weight: 3, Endpoints: []Endpoint{{Ref: "U1", Pin: "RESET"}, {Ref: "RRESET", Pin: "1"}}},
		{Name: "PROG", Role: NetSignal, Weight: 2, Endpoints: []Endpoint{{Ref: "U1", Pin: "MOSI"}, {Ref: "JPROG", Pin: "1"}}},
	}
	req.RegionRules = []RegionRule{{ID: "digital", Region: "digital", NetRoles: []NetRole{NetSignal}, Preferred: Rect{Min: Point{XMM: 1, YMM: 1}, Max: Point{XMM: 50, YMM: 30}}}}
	req.ProximityRules = []ProximityRule{
		{ID: "mcu-decoupling", Role: IntentDecoupling, AnchorRef: "U1", TargetRefs: []string{"C1"}, MaxDistanceMM: 15, Required: true},
		{ID: "reset-pull", Role: IntentReset, AnchorRef: "U1", TargetRefs: []string{"RRESET"}, MaxDistanceMM: 20},
	}
	return req
}

func goldenUSBCPowerRequest() Request {
	req := goldenBaseRequest(55, 25)
	j := goldenComponent("J1", "Connector_USB:USB_C_Receptacle", 8, 6, "CC1", "CC2", "VBUS")
	j.Edge = EdgeLeft
	req.Components = []Component{
		j,
		goldenComponent("RCC1", "Resistor_SMD:R_0805_2012Metric", 2, 1.25),
		goldenComponent("RCC2", "Resistor_SMD:R_0805_2012Metric", 2, 1.25),
		goldenComponent("F1", "Fuse:Fuse_1206", 3.2, 1.6),
	}
	req.Nets = []Net{
		{Name: "CC1", Role: NetSignal, Weight: 3, Endpoints: []Endpoint{{Ref: "J1", Pin: "CC1"}, {Ref: "RCC1", Pin: "1"}}},
		{Name: "CC2", Role: NetSignal, Weight: 3, Endpoints: []Endpoint{{Ref: "J1", Pin: "CC2"}, {Ref: "RCC2", Pin: "1"}}},
		{Name: "VBUS", Role: NetPower, Weight: 5, Endpoints: []Endpoint{{Ref: "J1", Pin: "VBUS"}, {Ref: "F1", Pin: "1"}}},
	}
	req.ProximityRules = []ProximityRule{
		{ID: "cc1-pulldown", Role: IntentConnector, AnchorRef: "J1", TargetRefs: []string{"RCC1"}, MaxDistanceMM: 18, Required: true},
		{ID: "cc2-pulldown", Role: IntentConnector, AnchorRef: "J1", TargetRefs: []string{"RCC2"}, MaxDistanceMM: 18, Required: true},
	}
	return req
}

func goldenI2CSensorRequest() Request {
	req := goldenBaseRequest(45, 25)
	req.Components = []Component{
		goldenComponent("U1", "Sensor:Generic_I2C", 3, 3, "SDA", "SCL", "VCC"),
		goldenComponent("RSDA", "Resistor_SMD:R_0805_2012Metric", 2, 1.25),
		goldenComponent("RSCL", "Resistor_SMD:R_0805_2012Metric", 2, 1.25),
		goldenComponent("C1", "Capacitor_SMD:C_0805_2012Metric", 2, 1.25),
	}
	req.Nets = []Net{
		{Name: "SDA", Role: NetSignal, Weight: 3, Endpoints: []Endpoint{{Ref: "U1", Pin: "SDA"}, {Ref: "RSDA", Pin: "1"}}},
		{Name: "SCL", Role: NetClock, Weight: 3, Endpoints: []Endpoint{{Ref: "U1", Pin: "SCL"}, {Ref: "RSCL", Pin: "1"}}},
		{Name: "VCC", Role: NetPower, Weight: 5, Endpoints: []Endpoint{{Ref: "U1", Pin: "VCC"}, {Ref: "C1", Pin: "1"}}},
	}
	req.RegionRules = []RegionRule{{ID: "sensor", Region: "sensor_region", Refs: []string{"U1", "RSDA", "RSCL", "C1"}, Preferred: Rect{Min: Point{XMM: 1, YMM: 1}, Max: Point{XMM: 35, YMM: 20}}}}
	req.ProximityRules = []ProximityRule{{ID: "sensor-decoupling", Role: IntentDecoupling, AnchorRef: "U1", TargetRefs: []string{"C1"}, MaxDistanceMM: 15, Required: true}}
	return req
}

func goldenOpAmpRequest() Request {
	req := goldenBaseRequest(45, 25)
	req.Components = []Component{
		goldenComponent("U1", "Package_TO_SOT_SMD:SOT-23-5", 3, 3, "OUT", "IN"),
		goldenComponent("RF", "Resistor_SMD:R_0805_2012Metric", 2, 1.25),
		goldenComponent("RG", "Resistor_SMD:R_0805_2012Metric", 2, 1.25),
	}
	req.Nets = []Net{
		{Name: "FB", Role: NetAnalog, Weight: 5, Endpoints: []Endpoint{{Ref: "U1", Pin: "OUT"}, {Ref: "RF", Pin: "1"}}},
		{Name: "GAIN", Role: NetAnalog, Weight: 4, Endpoints: []Endpoint{{Ref: "U1", Pin: "IN"}, {Ref: "RG", Pin: "1"}}},
	}
	req.ProximityRules = []ProximityRule{
		{ID: "feedback", Role: IntentFeedback, AnchorRef: "U1", TargetRefs: []string{"RF"}, MaxDistanceMM: 15, Required: true},
		{ID: "gain", Role: IntentFeedback, AnchorRef: "U1", TargetRefs: []string{"RG"}, MaxDistanceMM: 15, Required: true},
	}
	return req
}

func goldenConnectorBreakoutRequest() Request {
	req := goldenBaseRequest(30, 20)
	j := goldenComponent("J1", "Connector_Generic:Conn_01x04", 4, 8, "1", "2", "3", "4")
	j.Edge = EdgeRight
	req.Components = []Component{j}
	return req
}

func goldenBaseRequest(width float64, height float64) Request {
	return Request{
		Board: BoardPlacementArea{WidthMM: width, HeightMM: height, MarginMM: 1},
		Rules: Rules{MaxCandidatesPerPart: 1000},
	}
}

func goldenComponent(ref string, footprint string, width float64, height float64, pads ...string) Component {
	if len(pads) == 0 {
		pads = []string{"1", "2"}
	}
	padSummaries := make([]PadSummary, 0, len(pads))
	for index, pad := range pads {
		x := (float64(index) + 0.5) * (width / float64(len(pads)))
		padSummaries = append(padSummaries, PadSummary{Name: pad, XMM: x - width/2, YMM: 0})
	}
	return Component{
		Ref:         ref,
		FootprintID: footprint,
		Bounds:      Bounds{WidthMM: width, HeightMM: height, AnchorOffset: Point{XMM: width / 2, YMM: height / 2}, Source: BoundsExplicit},
		Pads:        padSummaries,
	}
}
