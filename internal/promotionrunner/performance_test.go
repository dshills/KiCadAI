package promotionrunner

import (
	"os"
	"path/filepath"
	"testing"
)

func BenchmarkCompareProjects(b *testing.B) {
	root := b.TempDir()
	first := filepath.Join(root, "run-1", "project")
	second := filepath.Join(root, "run-2", "project")
	for _, project := range []string{first, second} {
		if err := os.MkdirAll(project, 0o755); err != nil {
			b.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(project, "board.kicad_pcb"), []byte("(kicad_pcb (version 20240108))\n"), 0o600); err != nil {
			b.Fatal(err)
		}
	}
	toolchain := comparisonToolchain()
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		comparison, err := CompareProjects("case", first, second, root, toolchain)
		if err != nil || comparison.Status != "pass" {
			b.Fatalf("comparison status=%q err=%v", comparison.Status, err)
		}
	}
}
