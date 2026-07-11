package blocks

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"kicadai/internal/evaluate"
	"kicadai/internal/kicadfiles"
	"kicadai/internal/kicadfiles/schematic"
	"kicadai/internal/libraryresolver"
	"kicadai/internal/reports"
	"kicadai/internal/transactions"
)

func TestConcreteI2CSensorProfilesWriteElectricalSchematics(t *testing.T) {
	tests := []struct {
		name        string
		componentID string
		address     string
	}{
		{name: "bmp280", componentID: "sensor.bosch.bmp280.lga8", address: "0x76"},
		{name: "sht31", componentID: "sensor.sensirion.sht31_dis.dfn8", address: "0x44"},
	}
	index := concreteSensorResolverIndex()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, issues := NewBuiltinRegistry().Instantiate(context.Background(), BlockRequest{
				BlockID:    "i2c_sensor",
				InstanceID: "sensor",
				Params: map[string]any{
					"sensor_component_id": tt.componentID,
					"i2c_address":         tt.address,
				},
			})
			if reports.HasBlockingIssue(issues) {
				t.Fatalf("instantiate issues = %#v", issues)
			}
			tx, err := ProjectTransactionForBlockOutput(tt.name, output, false)
			if err != nil {
				t.Fatal(err)
			}
			outputDir := filepath.Join(t.TempDir(), tt.name)
			applied := transactions.Apply(tx, transactions.ApplyOptions{OutputDir: outputDir, LibraryIndex: &index})
			if reports.HasBlockingIssue(applied.Issues) {
				t.Fatalf("apply issues = %#v", applied.Issues)
			}
			path := filepath.Join(outputDir, tt.name+".kicad_sch")
			file, err := schematic.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := schematic.Validate(file); err != nil {
				t.Fatalf("schematic validation: %v", err)
			}
			report, err := evaluate.Schematic(path)
			if err != nil {
				t.Fatalf("schematic evaluation: %v", err)
			}
			for _, check := range report.Checks {
				if (check.Name == "schematic_validation" || check.Name == "schematic_electrical") && check.Status != evaluate.CheckPassed {
					t.Fatalf("check %s = %#v", check.Name, check)
				}
			}
		})
	}
}

func concreteSensorResolverIndex() libraryresolver.LibraryIndex {
	index := libraryresolver.LibraryIndex{
		Symbols: map[string]libraryresolver.SymbolRecord{
			"Device:C": passiveSensorTestSymbol("Device:C"),
			"Device:R": passiveSensorTestSymbol("Device:R"),
		},
		Footprints: map[string]libraryresolver.FootprintRecord{
			"Capacitor_SMD:C_0805_2012Metric": passiveSensorTestFootprint("Capacitor_SMD:C_0805_2012Metric"),
			"Resistor_SMD:R_0805_2012Metric":  passiveSensorTestFootprint("Resistor_SMD:R_0805_2012Metric"),
		},
	}
	for _, profile := range concreteI2CSensorProfiles {
		pins := make([]libraryresolver.SymbolPin, 0, len(profile.Pins))
		for _, pin := range profile.Pins {
			pins = append(pins, libraryresolver.SymbolPin{
				Number:     pin.Number,
				Electrical: "passive",
				Position: kicadfiles.Point{
					X: kicadfiles.MM(pin.XMM),
					Y: kicadfiles.MM(pin.YMM),
				},
			})
		}
		index.Symbols[profile.SymbolID] = libraryresolver.SymbolRecord{
			LibraryID:       profile.SymbolID,
			LibraryNickname: strings.SplitN(profile.SymbolID, ":", 2)[0],
			Name:            profile.Value,
			Units:           []libraryresolver.SymbolUnit{{Unit: 1}},
			Pins:            pins,
			Raw:             sensorTestRawSymbol(profile),
		}
		pads := make([]libraryresolver.FootprintPad, 0, len(profile.Pins))
		for _, pin := range profile.Pins {
			pads = append(pads, sensorTestPad(pin.Number))
		}
		index.Footprints[profile.FootprintID] = libraryresolver.FootprintRecord{
			FootprintID: profile.FootprintID,
			Name:        profile.Value,
			Attributes:  []string{"smd"},
			Pads:        pads,
		}
	}
	return index
}

func passiveSensorTestSymbol(id string) libraryresolver.SymbolRecord {
	name := strings.SplitN(id, ":", 2)[1]
	return libraryresolver.SymbolRecord{
		LibraryID:       id,
		LibraryNickname: strings.SplitN(id, ":", 2)[0],
		Name:            name,
		Units:           []libraryresolver.SymbolUnit{{Unit: 1}},
		Pins: []libraryresolver.SymbolPin{
			{Number: "1", Electrical: "passive", Position: kicadfiles.Point{Y: kicadfiles.MM(3.81)}},
			{Number: "2", Electrical: "passive", Position: kicadfiles.Point{Y: kicadfiles.MM(-3.81)}},
		},
		Raw: sensorTestRawSymbol(i2cSensorProfile{Value: name, Pins: []transactions.PinSpec{
			{Number: "1", YMM: 3.81},
			{Number: "2", YMM: -3.81},
		}}),
	}
}

func passiveSensorTestFootprint(id string) libraryresolver.FootprintRecord {
	return libraryresolver.FootprintRecord{
		FootprintID: id,
		Name:        id,
		Attributes:  []string{"smd"},
		Pads: []libraryresolver.FootprintPad{
			sensorTestPad("1"),
			sensorTestPad("2"),
		},
	}
}

func sensorTestPad(number string) libraryresolver.FootprintPad {
	return libraryresolver.FootprintPad{
		Name:   number,
		Type:   "smd",
		Shape:  "rect",
		Size:   kicadfiles.Point{X: kicadfiles.MM(0.8), Y: kicadfiles.MM(0.8)},
		Layers: []kicadfiles.BoardLayer{kicadfiles.LayerFCu, kicadfiles.LayerFPaste, kicadfiles.LayerFMask},
	}
}

func sensorTestRawSymbol(profile i2cSensorProfile) string {
	var pins strings.Builder
	for _, pin := range profile.Pins {
		fmt.Fprintf(&pins, `(pin passive line (at %g %g 0) (length 2.54) (name "P%s" (effects (font (size 1.27 1.27)))) (number "%s" (effects (font (size 1.27 1.27)))))`, pin.XMM, pin.YMM, pin.Number, pin.Number)
	}
	return fmt.Sprintf(`(symbol "%s"
  (exclude_from_sim no)
  (in_bom yes)
  (on_board yes)
  (property "Reference" "U" (at 0 8.89 0) (effects (font (size 1.27 1.27))))
  (property "Value" "%s" (at 0 6.35 0) (effects (font (size 1.27 1.27))))
  (property "Footprint" "" (at 0 0 0) (hide yes) (effects (font (size 1.27 1.27))))
  (property "Datasheet" "" (at 0 0 0) (hide yes) (effects (font (size 1.27 1.27))))
  (symbol "%s_0_1" (rectangle (start -5.08 5.08) (end 5.08 -5.08) (stroke (width 0.254) (type default)) (fill (type background))))
  (symbol "%s_1_1" %s))`, profile.Value, profile.Value, profile.Value, profile.Value, pins.String())
}
