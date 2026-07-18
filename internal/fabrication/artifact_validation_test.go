package fabrication

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestValidateFabricationArtifactsPassesCompleteOutputs(t *testing.T) {
	root := testFabricationProject(t)
	writeTestPCB(t, root, "demo.kicad_pcb")
	packageDir := filepath.Join(root, "fabrication")
	writeFabricationFile(t, packageDir, "gerbers/demo-F_Cu.gbr")
	writeFabricationFile(t, packageDir, "gerbers/demo-B_Cu.gbr")
	writeFabricationFile(t, packageDir, "gerbers/demo-F_Mask.gbr")
	writeFabricationFile(t, packageDir, "gerbers/demo-B_Mask.gbr")
	writeFabricationFile(t, packageDir, "gerbers/demo-F_SilkS.gbr")
	writeFabricationFile(t, packageDir, "gerbers/demo-B_SilkS.gbr")
	writeFabricationFile(t, packageDir, "gerbers/demo-Edge_Cuts.gbr")
	writeFabricationFile(t, packageDir, "drill/demo.drl")

	result := ValidateFabricationArtifacts(context.Background(), PlotRequest{
		PCBPath:   filepath.Join(root, "demo.kicad_pcb"),
		GerberDir: filepath.Join(packageDir, "gerbers"),
		DrillDir:  filepath.Join(packageDir, "drill"),
	})
	if result.Gerber != EvidencePass || result.Drill != EvidencePass {
		t.Fatalf("validation = %#v, want pass/pass", result)
	}
}

func TestValidateFabricationArtifactsAcceptsKiCad10SilkscreenNames(t *testing.T) {
	root := testFabricationProject(t)
	writeTestPCB(t, root, "demo.kicad_pcb")
	packageDir := filepath.Join(root, "fabrication")
	for _, rel := range []string{
		"gerbers/demo-F_Cu.gbr", "gerbers/demo-B_Cu.gbr",
		"gerbers/demo-F_Mask.gbr", "gerbers/demo-B_Mask.gbr",
		"gerbers/demo-F_Silkscreen.gto", "gerbers/demo-B_Silkscreen.gbo",
		"gerbers/demo-Edge_Cuts.gm1", "drill/demo.drl",
	} {
		writeFabricationFile(t, packageDir, rel)
	}
	result := ValidateFabricationArtifacts(context.Background(), PlotRequest{
		PCBPath: filepath.Join(root, "demo.kicad_pcb"), GerberDir: filepath.Join(packageDir, "gerbers"), DrillDir: filepath.Join(packageDir, "drill"),
	})
	if result.Gerber != EvidencePass || result.Drill != EvidencePass {
		t.Fatalf("validation = %#v, want pass/pass", result)
	}
}

func TestValidateFabricationArtifactsFailsMissingCopperLayer(t *testing.T) {
	root := testFabricationProject(t)
	writeTestPCB(t, root, "demo.kicad_pcb")
	packageDir := filepath.Join(root, "fabrication")
	writeFabricationFile(t, packageDir, "gerbers/demo-F_Cu.gbr")
	writeFabricationFile(t, packageDir, "gerbers/demo-F_Mask.gbr")
	writeFabricationFile(t, packageDir, "gerbers/demo-B_Mask.gbr")
	writeFabricationFile(t, packageDir, "gerbers/demo-F_SilkS.gbr")
	writeFabricationFile(t, packageDir, "gerbers/demo-B_SilkS.gbr")
	writeFabricationFile(t, packageDir, "gerbers/demo-Edge_Cuts.gbr")
	writeFabricationFile(t, packageDir, "drill/demo.drl")

	result := ValidateFabricationArtifacts(context.Background(), PlotRequest{
		PCBPath:   filepath.Join(root, "demo.kicad_pcb"),
		GerberDir: filepath.Join(packageDir, "gerbers"),
		DrillDir:  filepath.Join(packageDir, "drill"),
	})
	if result.Gerber != EvidenceFail {
		t.Fatalf("gerber status = %s, want fail", result.Gerber)
	}
	if !hasIssuePath(result.Issues, "fabrication/gerbers/B_Cu") {
		t.Fatalf("issues = %#v, want B_Cu issue", result.Issues)
	}
}

func TestValidateFabricationArtifactsFailsMissingDrill(t *testing.T) {
	root := testFabricationProject(t)
	writeTestPCB(t, root, "demo.kicad_pcb")
	packageDir := filepath.Join(root, "fabrication")
	writeFabricationFile(t, packageDir, "gerbers/demo-F_Cu.gbr")
	writeFabricationFile(t, packageDir, "gerbers/demo-B_Cu.gbr")
	writeFabricationFile(t, packageDir, "gerbers/demo-F_Mask.gbr")
	writeFabricationFile(t, packageDir, "gerbers/demo-B_Mask.gbr")
	writeFabricationFile(t, packageDir, "gerbers/demo-F_SilkS.gbr")
	writeFabricationFile(t, packageDir, "gerbers/demo-B_SilkS.gbr")
	writeFabricationFile(t, packageDir, "gerbers/demo-Edge_Cuts.gbr")
	if err := os.MkdirAll(filepath.Join(packageDir, "drill"), 0o755); err != nil {
		t.Fatal(err)
	}

	result := ValidateFabricationArtifacts(context.Background(), PlotRequest{
		PCBPath:   filepath.Join(root, "demo.kicad_pcb"),
		GerberDir: filepath.Join(packageDir, "gerbers"),
		DrillDir:  filepath.Join(packageDir, "drill"),
	})
	if result.Drill != EvidenceFail {
		t.Fatalf("drill status = %s, want fail", result.Drill)
	}
	if !hasIssuePath(result.Issues, "fabrication/drill") {
		t.Fatalf("issues = %#v, want drill issue", result.Issues)
	}
}

func TestValidateFabricationArtifactsReportsEmptyDrillOnce(t *testing.T) {
	root := testFabricationProject(t)
	writeTestPCB(t, root, "demo.kicad_pcb")
	packageDir := filepath.Join(root, "fabrication")
	writeFabricationFile(t, packageDir, "gerbers/demo-F_Cu.gbr")
	writeFabricationFile(t, packageDir, "gerbers/demo-B_Cu.gbr")
	writeFabricationFile(t, packageDir, "gerbers/demo-F_Mask.gbr")
	writeFabricationFile(t, packageDir, "gerbers/demo-B_Mask.gbr")
	writeFabricationFile(t, packageDir, "gerbers/demo-F_SilkS.gbr")
	writeFabricationFile(t, packageDir, "gerbers/demo-B_SilkS.gbr")
	writeFabricationFile(t, packageDir, "gerbers/demo-Edge_Cuts.gbr")
	path := filepath.Join(packageDir, "drill", "demo.drl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	result := ValidateFabricationArtifacts(context.Background(), PlotRequest{
		PCBPath:   filepath.Join(root, "demo.kicad_pcb"),
		GerberDir: filepath.Join(packageDir, "gerbers"),
		DrillDir:  filepath.Join(packageDir, "drill"),
	})
	if result.Drill != EvidenceFail {
		t.Fatalf("drill status = %s, want fail", result.Drill)
	}
	if hasIssuePath(result.Issues, "fabrication/drill") {
		t.Fatalf("issues = %#v, did not want duplicate missing drill issue", result.Issues)
	}
}

func writeFabricationFile(t *testing.T, packageDir string, rel string) {
	t.Helper()
	path := filepath.Join(packageDir, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("artifact"), 0o644); err != nil {
		t.Fatal(err)
	}
}
