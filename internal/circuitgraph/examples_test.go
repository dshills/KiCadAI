package circuitgraph

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestCheckedInCircuitGraphExamplesStrictDecode(t *testing.T) {
	for _, name := range []string{
		"rc_filter.json",
		"transistor_switch.json",
		"usb_c_led_indicator_protected.json",
		"usb_c_bmp280_breakout.json",
	} {
		t.Run(name, func(t *testing.T) {
			file, err := os.Open(filepath.Join(circuitGraphExamplesRoot(t), name))
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()
			document, issues := DecodeStrict(file)
			if len(issues) != 0 {
				t.Fatalf("decode issues = %#v", issues)
			}
			if document.Project.Name == "" || len(document.Components) < 2 || len(document.Nets) == 0 {
				t.Fatalf("incomplete example = %#v", document)
			}
		})
	}
}

func circuitGraphExamplesRoot(t *testing.T) string {
	t.Helper()
	_, sourcePath, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate circuit graph test source")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(sourcePath), "..", "..", "examples", "circuit-graph"))
}
