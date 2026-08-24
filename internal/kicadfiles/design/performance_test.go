package design

import (
	"path/filepath"
	"strconv"
	"testing"

	"kicadai/internal/kicadfiles"
)

func BenchmarkWriteProjectDirectory(b *testing.B) {
	design, err := LEDIndicatorDesign(LEDIndicatorInput{
		Name: "led_indicator", DesignID: kicadfiles.UUID("12345678-1234-5678-9234-123456789abc"),
		Seed: "performance", IncludePCB: true,
	})
	if err != nil {
		b.Fatal(err)
	}
	root := b.TempDir()
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		target := filepath.Join(root, design.Name+"-"+strconv.Itoa(iteration))
		if _, err := WriteProjectDirectory(target, design, WriteOptions{}); err != nil {
			b.Fatal(err)
		}
	}
}
