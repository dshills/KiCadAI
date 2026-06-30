package profiles

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadDirectoryLoadsValidProfiles(t *testing.T) {
	dir := t.TempDir()
	writeProfileFixture(t, filepath.Join(dir, "local.json"), Profile{
		Schema:  SchemaV1,
		ID:      "local_profile",
		Name:    "Local Profile",
		Version: "2026-06",
		Units:   "mm",
		Stackup: Stackup{
			MinLayers:               2,
			MaxLayers:               2,
			AllowedLayerCounts:      []int{2},
			MinBoardThicknessMM:     1.0,
			MaxBoardThicknessMM:     1.6,
			DefaultBoardThicknessMM: 1.6,
		},
		Copper:     Copper{MinTraceWidthMM: 0.20, MinSpacingMM: 0.20},
		Drill:      Drill{MinDrillMM: 0.35},
		SolderMask: SolderMask{MinSolderMaskWebMM: 0.12},
	})
	profiles, issues := LoadDirectory(dir)
	if len(issues) != 0 {
		t.Fatalf("LoadDirectory issues = %#v", issues)
	}
	if len(profiles) != 1 || profiles[0].ID != "local_profile" || profiles[0].Source.Kind != SourceLocal {
		t.Fatalf("profiles = %#v", profiles)
	}
}

func TestLoadDirectoryRejectsMalformedJSON(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "bad.json"), []byte(`{"schema":`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, issues := LoadDirectory(dir)
	if len(issues) == 0 || !strings.Contains(issues[0].Message, "invalid fabrication profile JSON") {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestLoadDirectoryRejectsDuplicateLocalIDs(t *testing.T) {
	dir := t.TempDir()
	profile := validLocalProfile("duplicate_local")
	writeProfileFixture(t, filepath.Join(dir, "a.json"), profile)
	writeProfileFixture(t, filepath.Join(dir, "b.json"), profile)
	profiles, issues := LoadDirectory(dir)
	if len(profiles) != 1 {
		t.Fatalf("profiles = %#v", profiles)
	}
	if len(issues) == 0 || !strings.Contains(issues[0].Message, "duplicate local fabrication profile ID") {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestLoadRegistryRejectsBuiltinShadowing(t *testing.T) {
	dir := t.TempDir()
	profile := validLocalProfile(DefaultProfileID)
	writeProfileFixture(t, filepath.Join(dir, "shadow.json"), profile)
	registry, issues := LoadRegistry(LoadOptions{ProfileDir: dir})
	if len(issues) == 0 || !strings.Contains(issues[0].Message, "shadows an existing profile ID") {
		t.Fatalf("issues = %#v", issues)
	}
	resolved, resolveIssues := registry.Resolve(DefaultProfileID)
	if len(resolveIssues) != 0 {
		t.Fatalf("resolve issues = %#v", resolveIssues)
	}
	if resolved.Source.Kind != SourceBuiltin {
		t.Fatalf("built-in profile was shadowed: %#v", resolved.Source)
	}
}

func TestLoadRegistryHonorsEnvironmentDirectory(t *testing.T) {
	dir := t.TempDir()
	writeProfileFixture(t, filepath.Join(dir, "env.json"), validLocalProfile("env_profile"))
	t.Setenv(EnvProfileDir, dir)
	registry, issues := LoadRegistry(LoadOptions{})
	if len(issues) != 0 {
		t.Fatalf("LoadRegistry issues = %#v", issues)
	}
	profile, resolveIssues := registry.Resolve("env_profile")
	if len(resolveIssues) != 0 {
		t.Fatalf("resolve issues = %#v", resolveIssues)
	}
	if profile.ID != "env_profile" {
		t.Fatalf("profile = %#v", profile)
	}
}

func TestLoadRegistryFlagDirectoryOverridesEnvironment(t *testing.T) {
	envDir := t.TempDir()
	flagDir := t.TempDir()
	writeProfileFixture(t, filepath.Join(envDir, "env.json"), validLocalProfile("env_profile"))
	writeProfileFixture(t, filepath.Join(flagDir, "flag.json"), validLocalProfile("flag_profile"))
	t.Setenv(EnvProfileDir, envDir)
	registry, issues := LoadRegistry(LoadOptions{ProfileDir: flagDir})
	if len(issues) != 0 {
		t.Fatalf("LoadRegistry issues = %#v", issues)
	}
	if _, envIssues := registry.Resolve("env_profile"); len(envIssues) == 0 {
		t.Fatal("environment profile loaded despite explicit flag directory")
	}
	if _, flagIssues := registry.Resolve("flag_profile"); len(flagIssues) != 0 {
		t.Fatalf("flag profile missing: %#v", flagIssues)
	}
}

func TestLoadDirectoryRejectsSymlinkEscape(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.json")
	writeProfileFixture(t, outside, validLocalProfile("outside_profile"))
	if err := os.Symlink(outside, filepath.Join(dir, "escape.json")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	_, issues := LoadDirectory(dir)
	if len(issues) == 0 || !strings.Contains(issues[0].Message, "outside the profile directory") {
		t.Fatalf("issues = %#v", issues)
	}
}

func validLocalProfile(id string) Profile {
	return Profile{
		Schema:  SchemaV1,
		ID:      id,
		Name:    "Local " + id,
		Version: "2026-06",
		Units:   "mm",
		Stackup: Stackup{
			MinLayers:               2,
			MaxLayers:               2,
			AllowedLayerCounts:      []int{2},
			MinBoardThicknessMM:     1.0,
			MaxBoardThicknessMM:     1.6,
			DefaultBoardThicknessMM: 1.6,
		},
		Copper:     Copper{MinTraceWidthMM: 0.20, MinSpacingMM: 0.20},
		Drill:      Drill{MinDrillMM: 0.35},
		SolderMask: SolderMask{MinSolderMaskWebMM: 0.12},
	}
}

func writeProfileFixture(t *testing.T, path string, profile Profile) {
	t.Helper()
	data, err := json.MarshalIndent(profile, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}
