package architecturesearch

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

type controlBehaviorManifest struct {
	Schema               string                       `json:"schema"`
	Version              int                          `json:"version"`
	FrozenAt             string                       `json:"frozen_at"`
	Fixtures             []controlBehaviorFixture     `json:"fixtures"`
	IdentityNeutralCases []identityNeutralControlCase `json:"identity_neutral_cases"`
}

type controlBehaviorFixture struct {
	ID               string `json:"id"`
	File             string `json:"file"`
	SHA256           string `json:"sha256"`
	ExpectedCode     string `json:"expected_code"`
	ExpectedPath     string `json:"expected_path"`
	ExpectedMessage  string `json:"expected_message"`
	PromotionOverlay bool   `json:"promotion_overlay,omitempty"`
}

type identityNeutralControlCase struct {
	ID              string  `json:"id"`
	Function        string  `json:"function"`
	Polarity        string  `json:"polarity"`
	StartupState    string  `json:"startup_state"`
	SafeState       string  `json:"safe_state"`
	From            string  `json:"from"`
	To              string  `json:"to"`
	Direction       string  `json:"direction"`
	ConsumerAction  string  `json:"consumer_action"`
	DependencyState string  `json:"dependency_state,omitempty"`
	StableForS      float64 `json:"stable_for_s,omitempty"`
}

func TestFrozenControlBehaviorCorpus(t *testing.T) {
	root := filepath.Join("testdata", "control_behavior_corpus")
	data, err := os.ReadFile(filepath.Join(root, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest controlBehaviorManifest
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Schema != "kicadai.control-behavior-corpus.v1" || manifest.Version != 1 || len(manifest.Fixtures) != 2 || len(manifest.IdentityNeutralCases) != 6 {
		t.Fatalf("manifest identity or coverage = %#v", manifest)
	}
	for _, fixture := range manifest.Fixtures {
		fixtureData, err := os.ReadFile(filepath.Join(root, fixture.File))
		if err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(fixtureData)
		if got := hex.EncodeToString(digest[:]); got != fixture.SHA256 {
			t.Fatalf("%s bytes hash = %s, want %s", fixture.ID, got, fixture.SHA256)
		}
		requirement, issues := DecodeStrict(bytes.NewReader(fixtureData))
		if requirement.Schema != SchemaIDV6 || requirement.Version != VersionV6 || requirement.Project.Name != fixture.ID {
			t.Fatalf("%s requirement = %#v issues=%#v", fixture.ID, requirement, issues)
		}
		matched := false
		for _, issue := range issues {
			if string(issue.Code) == fixture.ExpectedCode && issue.Path == fixture.ExpectedPath && issue.Message == fixture.ExpectedMessage {
				matched = true
			}
		}
		if !matched || len(issues) != 1 {
			t.Fatalf("%s precise rejection issues = %#v", fixture.ID, issues)
		}
		first := Normalize(requirement)
		second := Normalize(first)
		if !reflect.DeepEqual(first, second) {
			t.Fatalf("%s normalization replay differs", fixture.ID)
		}
	}
	for _, test := range manifest.IdentityNeutralCases {
		control := ControlSemantics{Function: test.Function, Polarity: test.Polarity, StartupState: test.StartupState, SafeState: test.SafeState}
		if got := controlTransitionDirection(control, test.From, test.To); got != test.Direction {
			t.Errorf("%s direction = %s, want %s", test.ID, got, test.Direction)
		}
		if got := physicalControlAction(control); got != test.ConsumerAction {
			t.Errorf("%s action = %s, want %s", test.ID, got, test.ConsumerAction)
		}
		if test.DependencyState != "" && (test.DependencyState != "valid" || test.StableForS <= 0) {
			t.Errorf("%s sequencing dependency is not bounded", test.ID)
		}
	}
}
