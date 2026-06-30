package profiles

import (
	"slices"
	"testing"
)

func TestBuiltinsValidateAndAreUnique(t *testing.T) {
	seen := map[string]struct{}{}
	for _, profile := range builtinProfiles {
		if profile.ID == "" {
			t.Fatalf("empty builtin profile id in %#v", profile)
		}
		if _, exists := seen[profile.ID]; exists {
			t.Fatalf("duplicate builtin profile id %q", profile.ID)
		}
		seen[profile.ID] = struct{}{}
		if issues := Validate(profile); len(issues) != 0 {
			t.Fatalf("builtin profile %s has validation issues: %#v", profile.ID, issues)
		}
	}
	if _, exists := seen[DefaultProfileID]; !exists {
		t.Fatalf("default profile %q is not registered", DefaultProfileID)
	}
}

func TestBuiltinsListOrderingIsDeterministic(t *testing.T) {
	summaries := List()
	if len(summaries) == 0 {
		t.Fatal("expected built-in profile summaries")
	}
	var ids []string
	for _, summary := range summaries {
		ids = append(ids, summary.ID)
	}
	if !slices.IsSorted(ids) {
		t.Fatalf("profile IDs are not sorted: %#v", ids)
	}
	second := List()
	for index, summary := range second {
		if ids[index] != summary.ID {
			t.Fatalf("profile list order changed: %#v != %#v", ids, second)
		}
	}
}

func TestResolveDefaultAndUnknownProfile(t *testing.T) {
	profile, issues := Resolve("")
	if len(issues) != 0 {
		t.Fatalf("default resolve issues = %#v", issues)
	}
	if profile.ID != DefaultProfileID {
		t.Fatalf("default profile ID = %q, want %q", profile.ID, DefaultProfileID)
	}

	_, issues = Resolve("missing_profile")
	if !hasIssuePath(issues, "fabrication_profile.id") {
		t.Fatalf("missing profile did not produce id issue: %#v", issues)
	}
}

func TestResolveReturnsClone(t *testing.T) {
	profile, issues := Resolve(DefaultProfileID)
	if len(issues) != 0 {
		t.Fatalf("resolve issues = %#v", issues)
	}
	profile.Stackup.AllowedLayerCounts[0] = 99
	again, issues := Resolve(DefaultProfileID)
	if len(issues) != 0 {
		t.Fatalf("resolve issues = %#v", issues)
	}
	if again.Stackup.AllowedLayerCounts[0] == 99 {
		t.Fatalf("Resolve returned mutable builtin storage")
	}
}

func TestGenericAssemblyThresholdsMatchExistingDefaults(t *testing.T) {
	profile, issues := Resolve(DefaultProfileID)
	if len(issues) != 0 {
		t.Fatalf("resolve issues = %#v", issues)
	}
	if profile.Drill.MinPadAnnularRingMM != 0.15 {
		t.Fatalf("pad annular ring = %v", profile.Drill.MinPadAnnularRingMM)
	}
	if profile.Drill.MinViaAnnularRingMM != 0.10 {
		t.Fatalf("via annular ring = %v", profile.Drill.MinViaAnnularRingMM)
	}
	if profile.Copper.MinCopperSliverMM != 0.127 {
		t.Fatalf("copper sliver = %v", profile.Copper.MinCopperSliverMM)
	}
	if profile.SolderMask.MinSolderMaskWebMM != 0.10 {
		t.Fatalf("solder mask web = %v", profile.SolderMask.MinSolderMaskWebMM)
	}
}
