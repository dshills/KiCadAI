package circuitgraph

import (
	"encoding/json"
)

const SynthesisPolicyVersion = "function-synthesis-policy-v1"

// FunctionIntent is the topology-level provider contract. It deliberately has
// no KiCad references, symbols, footprints, pins, pads, coordinates, layers,
// routes, support-component instances, or block identities.
type FunctionIntent struct {
	Functions    []FunctionRequirement  `json:"functions"`
	Interfaces   []InterfaceRequirement `json:"interfaces"`
	PowerDomains []PowerDomainIntent    `json:"power_domains"`
	Connections  []FunctionConnection   `json:"connections"`
	Constraints  SynthesisConstraints   `json:"constraints"`
}

type FunctionRequirement struct {
	ID                string                     `json:"id"`
	Role              ComponentRole              `json:"role"`
	ComponentID       string                     `json:"component_id,omitempty"`
	Query             *ComponentQuery            `json:"query,omitempty"`
	Value             string                     `json:"value,omitempty"`
	Parameters        []Parameter                `json:"parameters,omitempty"`
	RequiredRatings   []RequiredRating           `json:"required_ratings,omitempty"`
	RequiredFunctions []string                   `json:"required_functions,omitempty"`
	Usage             string                     `json:"usage"`
	Extensions        map[string]json.RawMessage `json:"extensions,omitempty"`
}

type InterfaceRequirement struct {
	ID      string            `json:"id"`
	Role    InterfaceRole     `json:"role"`
	Signals []InterfaceSignal `json:"signals"`
}

type InterfaceRole string

const (
	InterfacePowerInput  InterfaceRole = "power_input"
	InterfacePowerOutput InterfaceRole = "power_output"
	InterfaceAnalogInput InterfaceRole = "analog_input"
	InterfaceAnalogOut   InterfaceRole = "analog_output"
	InterfaceDigitalIn   InterfaceRole = "digital_input"
	InterfaceDigitalOut  InterfaceRole = "digital_output"
	InterfaceI2C         InterfaceRole = "i2c"
	InterfaceSPI         InterfaceRole = "spi"
	InterfaceUART        InterfaceRole = "uart"
	InterfaceGPIO        InterfaceRole = "gpio"
	InterfaceProgramming InterfaceRole = "programming"
)

type InterfaceSignal struct {
	Name string  `json:"name"`
	Role NetRole `json:"role"`
}

type PowerDomainIntent struct {
	Name         string            `json:"name"`
	Role         NetRole           `json:"role"`
	VoltageV     float64           `json:"voltage_v"`
	MaxCurrentMA float64           `json:"max_current_ma,omitempty"`
	Source       PowerDomainSource `json:"source"`
}

type PowerDomainSource string

const (
	PowerDomainExternal  PowerDomainSource = "external"
	PowerDomainGenerated PowerDomainSource = "generated"
)

type FunctionConnection struct {
	Name          string               `json:"name"`
	Role          NetRole              `json:"role"`
	VoltageDomain string               `json:"voltage_domain,omitempty"`
	CurrentMA     float64              `json:"current_ma,omitempty"`
	Endpoints     []FunctionalEndpoint `json:"endpoints"`
}

// FunctionalEndpoint is a strict tagged union. Exactly one of Function or
// Interface must be set. Port and Signal are semantic names, never KiCad pin or
// pad identifiers.
type FunctionalEndpoint struct {
	Function  string `json:"function,omitempty"`
	Port      string `json:"port,omitempty"`
	Interface string `json:"interface,omitempty"`
	Signal    string `json:"signal,omitempty"`
}

type SynthesisConstraints struct {
	MaxWidthMM                  float64 `json:"max_width_mm"`
	MaxHeightMM                 float64 `json:"max_height_mm"`
	PreferredComponentSpacingMM float64 `json:"preferred_component_spacing_mm"`
	Protection                  string  `json:"protection,omitempty"`
}
