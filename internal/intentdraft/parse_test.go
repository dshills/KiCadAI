package intentdraft

import "testing"

func TestFindVoltages(t *testing.T) {
	values := findVoltages("make 5V input and 3V3 rail on VBUS")
	if len(values) != 3 {
		t.Fatalf("values = %#v", values)
	}
	if values[1].TextValue != "3.3V" {
		t.Fatalf("normalized voltage = %#v", values[1])
	}
	if values[0].Field.StartByte != 5 || values[0].Field.EndByte != 7 {
		t.Fatalf("offsets = %#v", values[0].Field)
	}
}

func TestFindCurrents(t *testing.T) {
	values := findCurrents("needs 1A input and 100mA rail")
	if len(values) != 2 || values[0].FloatValue != 1000 || values[1].FloatValue != 100 {
		t.Fatalf("values = %#v", values)
	}
}

func TestFindDimensions(t *testing.T) {
	values := findDimensions("make it 2 inch x 1 inch")
	if len(values) != 2 {
		t.Fatalf("values = %#v", values)
	}
	if values[0].FloatValue != 50.8 || values[1].FloatValue != 25.4 {
		t.Fatalf("dimensions = %#v", values)
	}
}

func TestFindLayers(t *testing.T) {
	values := findLayers("two-layer board and 4 layer option")
	if len(values) != 2 || values[0].IntValue != 2 || values[1].IntValue != 4 {
		t.Fatalf("values = %#v", values)
	}
}

func TestFindFrequencyGainAndValues(t *testing.T) {
	if values := findFrequencies("use a 16 MHz crystal"); len(values) != 1 || values[0].Unit != "Hz" || values[0].FloatValue != 16000000 {
		t.Fatalf("frequency = %#v", values)
	}
	if values := findGains("gain of 10"); len(values) != 1 || values[0].FloatValue != 10 {
		t.Fatalf("gain = %#v", values)
	}
	if values := findRCValues("add 10k and 100nF"); len(values) != 2 {
		t.Fatalf("rc values = %#v", values)
	}
}
