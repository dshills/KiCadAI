// Package generationcapability describes the currently supported AI-to-KiCad
// generation paths. It is a declaration of proven contracts, not a claim that
// every circuit matching a schema can be fabricated without review.
package generationcapability

import (
	"slices"

	"kicadai/internal/circuitgraph"
)

type Kind string

const (
	KindGeneric Kind = "generic"
	KindBounded Kind = "bounded"
)

const (
	ProfileGenericCircuit   = circuitgraph.ProviderProfileID
	ProfileUSBCBMP280       = "usb_c_bmp280"
	ProfileUSBCProtectedLED = "usb_c_led_protected"
)

// Capability defines a provider/planner input contract and the evidence needed
// before its output may be promoted.
type Capability struct {
	ProfileID        string   `json:"profile_id"`
	Kind             Kind     `json:"kind"`
	InputContract    string   `json:"input_contract"`
	Supports         []string `json:"supports"`
	Limitations      []string `json:"limitations"`
	RequiredEvidence []string `json:"required_evidence"`
}

var commonRequiredEvidence = []string{
	"strict graph or request validation and trusted catalog resolution",
	"schematic electrical and readability checks",
	"required-net connectivity and route completion",
	"writer correctness and normalized round-trip checks",
	"KiCad ERC and DRC when the requested acceptance requires them",
}

var capabilities = []Capability{
	{
		ProfileID:     ProfileGenericCircuit,
		Kind:          KindGeneric,
		InputContract: circuitgraph.SchemaID,
		Supports: []string{
			"catalog-resolved components, packages, symbols, and pin functions",
			"up to 512 components, 1024 nets, and 512 endpoints per net",
			"explicit multi-unit component identities and required power units",
			"relative schematic layout intent and bounded PCB placement regions",
			"deterministic placement and ordered route-tree execution for proven graph shapes",
		},
		Limitations: []string{
			"ambiguous catalog resolution, unsupported pin functions, and incomplete graph intent fail closed",
			"not a free-form electronics language or a guarantee for arbitrary dense, high-speed, RF, or analog circuits",
			"routing is bounded and does not implement general rip-up-and-reroute",
			"analog performance, thermal behavior, and fabrication suitability require explicit evidence or review",
		},
		RequiredEvidence: commonRequiredEvidence,
	},
	{
		ProfileID:     ProfileUSBCBMP280,
		Kind:          KindBounded,
		InputContract: "kicadai_bmp280_intent_v1",
		Supports: []string{
			"USB-C powered BMP280 breakout with the proven protection, regulation, pull-up, decoupling, and connector composition",
		},
		Limitations: []string{
			"only the bounded reference composition is selected from natural-language prompt semantics",
		},
		RequiredEvidence: commonRequiredEvidence,
	},
	{
		ProfileID:     ProfileUSBCProtectedLED,
		Kind:          KindBounded,
		InputContract: "kicadai_usb_c_led_intent_v1",
		Supports: []string{
			"protected USB-C LED indicator with the proven fuse, TVS, bulk-capacitance, LED, and resistor composition",
		},
		Limitations: []string{
			"only the bounded reference composition is selected from natural-language prompt semantics",
		},
		RequiredEvidence: commonRequiredEvidence,
	},
}

// All returns a deterministic copy so callers cannot mutate the declared
// capability matrix.
func All() []Capability {
	result := make([]Capability, 0, len(capabilities))
	for _, capability := range capabilities {
		result = append(result, clone(capability))
	}
	return result
}

func Lookup(profileID string) (Capability, bool) {
	for _, capability := range capabilities {
		if capability.ProfileID == profileID {
			return clone(capability), true
		}
	}
	return Capability{}, false
}

func clone(capability Capability) Capability {
	capability.Supports = slices.Clone(capability.Supports)
	capability.Limitations = slices.Clone(capability.Limitations)
	capability.RequiredEvidence = slices.Clone(capability.RequiredEvidence)
	return capability
}
