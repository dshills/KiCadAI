package generate

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestGenerateBreakoutWritesProject(t *testing.T) {
	output := filepath.Join(t.TempDir(), "breakout")
	result := GenerateBreakout(validBreakoutRequest(), BreakoutOptions{OutputDir: output})
	if len(result.Issues) != 0 {
		t.Fatalf("unexpected issues: %#v", result.Issues)
	}
	if result.TransactionOperations == 0 {
		t.Fatal("expected transaction operations")
	}
	for _, name := range []string{"sensor_breakout.kicad_pro", "sensor_breakout.kicad_sch", "sensor_breakout.kicad_pcb", ".kicadai/manifest.json"} {
		if _, err := os.Stat(filepath.Join(output, name)); err != nil {
			t.Fatalf("expected %s: %v", name, err)
		}
	}
	board, err := os.ReadFile(filepath.Join(output, "sensor_breakout.kicad_pcb"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(board)
	for _, want := range []string{
		`"Edge.Cuts"`,
		`PinHeader_1x04_P2.54mm_Vertical`,
		`(net "VCC")`,
		`(net "GND")`,
		`(net "SCL")`,
		`(net "SDA")`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("PCB missing %q\n%s", want, text)
		}
	}
}

func TestValidateBreakoutRejectsInvalidBoard(t *testing.T) {
	req := validBreakoutRequest()
	req.Board.WidthMM = 0
	issues := ValidateBreakoutRequest(req)
	if len(issues) == 0 || issues[0].Path != "board.width_mm" {
		t.Fatalf("expected board width issue: %#v", issues)
	}
}

func TestValidateBreakoutRejectsPathName(t *testing.T) {
	req := validBreakoutRequest()
	req.Name = "../outside"
	issues := ValidateBreakoutRequest(req)
	if len(issues) == 0 {
		t.Fatal("expected name issue")
	}
	found := false
	for _, issue := range issues {
		if issue.Path == "name" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected name issue: %#v", issues)
	}
}

func TestValidateBreakoutRejectsTinyGroundZoneBoard(t *testing.T) {
	req := validBreakoutRequest()
	req.Board.WidthMM = 1.5
	issues := ValidateBreakoutRequest(req)
	if len(issues) == 0 {
		t.Fatal("expected board issue")
	}
	found := false
	for _, issue := range issues {
		if issue.Path == "board" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected board issue: %#v", issues)
	}
}

func TestValidateBreakoutRejectsMismatchedPins(t *testing.T) {
	req := validBreakoutRequest()
	req.Connectors[1].Pins = []string{"VCC", "GND"}
	issues := ValidateBreakoutRequest(req)
	if len(issues) == 0 {
		t.Fatal("expected issue")
	}
	found := false
	for _, issue := range issues {
		if issue.Path == "connectors[1].pins" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected connectors[1].pins issue: %#v", issues)
	}
}

func TestValidateBreakoutRejectsLargeConnector(t *testing.T) {
	req := validBreakoutRequest()
	req.Connectors[0].Pins = make([]string, 41)
	req.Connectors[1].Pins = make([]string, 41)
	for i := 0; i < 41; i++ {
		req.Connectors[0].Pins[i] = "P" + strconv.Itoa(i+1)
		req.Connectors[1].Pins[i] = "P" + strconv.Itoa(i+1)
	}
	issues := ValidateBreakoutRequest(req)
	if len(issues) == 0 {
		t.Fatal("expected issue")
	}
	found := false
	for _, issue := range issues {
		if strings.Contains(issue.Message, "40 or fewer") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected pin count issue: %#v", issues)
	}
}

func TestBreakoutTransactionGroundZoneWithNoConnectorsDoesNotPanic(t *testing.T) {
	req := BreakoutRequest{
		Name:       "empty_breakout",
		Board:      BoardRequest{WidthMM: 50, HeightMM: 30},
		GroundZone: true,
	}
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("BreakoutTransaction panicked: %v", r)
		}
	}()
	tx, err := BreakoutTransaction(req)
	if err != nil {
		t.Fatal(err)
	}
	if len(tx.Operations) == 0 {
		t.Fatal("expected base transaction operations")
	}
}

func validBreakoutRequest() BreakoutRequest {
	return BreakoutRequest{
		Kind:  "breakout_board",
		Name:  "sensor_breakout",
		Board: BoardRequest{WidthMM: 50, HeightMM: 30},
		Connectors: []ConnectorRequest{
			{Ref: "J1", Pins: []string{"VCC", "GND", "SCL", "SDA"}},
			{Ref: "J2", Pins: []string{"VCC", "GND", "SCL", "SDA"}},
		},
		GroundZone: true,
	}
}
