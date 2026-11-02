package boardfamily

import (
	"encoding/json"
	"strings"
	"testing"
)

func sampleSpecification() Specification {
	c := Catalog()[0].DefaultConfiguration
	return Specification{Version: SpecificationVersion, Origin: "manual", Configuration: &c, Unresolved: []string{}, ReviewNotes: []string{}}
}

func TestSpecificationAllConfigurations(t *testing.T) {
	for _, family := range Catalog() {
		for _, profile := range family.Profiles {
			t.Run(family.ID+"/"+profile.ID, func(t *testing.T) {
				s := sampleSpecification()
				c := family.DefaultConfiguration
				c.Profile = profile.ID
				c.TotalBusCapacitancePF = profile.MaxBusPF
				s.Configuration = &c
				r, err := ReviewSpecification(s)
				if err != nil || !r.Ready || r.Electrical == nil {
					t.Fatalf("review: %+v %v", r, err)
				}
				confirmed, err := ConfirmSpecification(s, r.SHA256)
				if err != nil {
					t.Fatal(err)
				}
				b, _ := json.Marshal(confirmed)
				loaded, err := DecodeConfirmedSpecification(strings.NewReader(string(b)))
				if err != nil {
					t.Fatal(err)
				}
				got, err := VerifyConfirmedSpecification(loaded)
				if err != nil || got != c {
					t.Fatalf("configuration changed: %+v %v", got, err)
				}
			})
		}
	}
}

func TestSpecificationEditsRequireConfirmation(t *testing.T) {
	s := sampleSpecification()
	r, _ := ReviewSpecification(s)
	c, _ := ConfirmSpecification(s, r.SHA256)
	for _, edit := range []func(*ConfirmedSpecification){
		func(c *ConfirmedSpecification) {
			c.Specification.Configuration.Profile = "fast"
			c.Specification.Configuration.TotalBusCapacitancePF = 100
		},
		func(c *ConfirmedSpecification) { c.Specification.OriginalRequest = "changed" },
		func(c *ConfirmedSpecification) { c.Specification.ReviewNotes = []string{"changed"} },
		func(c *ConfirmedSpecification) { c.Specification.Unresolved = []string{"uncertain sensor"} },
		func(c *ConfirmedSpecification) { c.Acknowledgement = "" },
		func(c *ConfirmedSpecification) { c.ReviewSHA256 = "" },
	} {
		b, _ := json.Marshal(c)
		var changed ConfirmedSpecification
		_ = json.Unmarshal(b, &changed)
		edit(&changed)
		if _, err := VerifyConfirmedSpecification(changed); err == nil {
			t.Fatal("edited receipt was accepted")
		}
	}
}

func TestSpecificationRefusesIncompleteAndInvalidInputs(t *testing.T) {
	for _, edit := range []func(*Specification){
		func(s *Specification) { s.Configuration = nil },
		func(s *Specification) { s.Configuration.SupplyMaxV = 5 },
		func(s *Specification) { s.Configuration.Family = FamilySHT31; s.Configuration.Profile = "low_current" },
		func(s *Specification) { s.Unresolved = []string{"need wireless"} },
	} {
		s := sampleSpecification()
		edit(&s)
		r, err := ReviewSpecification(s)
		if err != nil || r.Ready {
			t.Fatalf("expected reviewable but blocked: %+v %v", r, err)
		}
		if _, err = ConfirmSpecification(s, r.SHA256); err == nil {
			t.Fatal("invalid proposal confirmed")
		}
	}
	if _, err := ConfirmSpecification(sampleSpecification(), "wrong"); err == nil {
		t.Fatal("wrong digest accepted")
	}
	for _, raw := range []string{`null`, `{}`, `{"version":"x","version":"y"}`, `{"unknown":1}`, `{} {}`, strings.Repeat(" ", 65537)} {
		if _, err := DecodeSpecification(strings.NewReader(raw)); err == nil {
			t.Fatalf("accepted malformed specification %.40s", raw)
		}
	}
	s := sampleSpecification()
	s.Origin = "ai"
	if _, err := ReviewSpecification(s); err == nil {
		t.Fatal("AI proposal missing original request")
	}
	s.OriginalRequest = "Build a pressure board"
	s.Unresolved = nil
	if _, err := ReviewSpecification(s); err == nil {
		t.Fatal("null unresolved array accepted")
	}
}
