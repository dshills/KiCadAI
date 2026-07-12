package aiprovider

import (
	"errors"
	"strings"
	"unicode"

	"kicadai/internal/circuitgraph"
)

const (
	ReferenceProfileBMP280          = "usb_c_bmp280"
	ReferenceProfileProtectedLED    = "usb_c_led_protected"
	BMP280ReferenceSchemaName       = "kicadai_bmp280_intent_v1"
	ProtectedLEDReferenceSchemaName = "kicadai_usb_c_led_intent_v1"
)

var (
	ErrUnsupportedReferencePrompt      = errors.New("prompt does not match a supported AI reference profile")
	ErrUnsupportedReferenceComposition = errors.New("prompt combines multiple AI reference profiles")
)

// ReferenceProfile is a bounded provider contract selected before model execution.
type ReferenceProfile struct {
	ID                string
	SchemaName        string
	CapabilityContext string
	schema            func() map[string]any
}

func (profile ReferenceProfile) IntentEnvelopeSchema() map[string]any {
	return profile.schema()
}

func BMP280Profile() ReferenceProfile {
	return ReferenceProfile{
		ID:                ReferenceProfileBMP280,
		SchemaName:        BMP280ReferenceSchemaName,
		CapabilityContext: BMP280ReferenceCapabilityContext,
		schema:            BMP280ReferenceIntentEnvelopeSchema,
	}
}

func ProtectedLEDProfile() ReferenceProfile {
	return ReferenceProfile{
		ID:                ReferenceProfileProtectedLED,
		SchemaName:        ProtectedLEDReferenceSchemaName,
		CapabilityContext: ProtectedLEDReferenceCapabilityContext,
		schema:            ProtectedLEDReferenceIntentEnvelopeSchema,
	}
}

func GenericCircuitProfile(capabilityContext string) ReferenceProfile {
	return ReferenceProfile{
		ID: circuitgraph.ProviderProfileID, SchemaName: circuitgraph.ProviderSchemaName,
		CapabilityContext: capabilityContext, schema: genericCircuitEnvelopeSchema,
	}
}

func genericCircuitEnvelopeSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"schema": map[string]any{"type": "string", "const": EnvelopeSchemaV1},
			"intent": circuitgraph.ProviderGraphSchema(),
		},
		"required": []string{"intent", "schema"}, "additionalProperties": false,
	}
}

func SelectReferenceProfile(prompt string) (ReferenceProfile, error) {
	tokens := referencePromptTokens(prompt)
	usbC := hasAdjacentTokens(tokens, "usb", "c") || hasToken(tokens, "usbc")
	bmp280 := hasToken(tokens, "bmp280")
	led := hasToken(tokens, "led") || hasToken(tokens, "leds")
	if bmp280 && led {
		return ReferenceProfile{}, ErrUnsupportedReferenceComposition
	}
	if !usbC {
		return ReferenceProfile{}, ErrUnsupportedReferencePrompt
	}
	switch {
	case bmp280:
		return BMP280Profile(), nil
	case led:
		return ProtectedLEDProfile(), nil
	default:
		return ReferenceProfile{}, ErrUnsupportedReferencePrompt
	}
}

func referencePromptTokens(prompt string) []string {
	return strings.FieldsFunc(strings.ToLower(prompt), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})
}

func hasToken(tokens []string, target string) bool {
	for _, token := range tokens {
		if token == target {
			return true
		}
	}
	return false
}

func hasAdjacentTokens(tokens []string, first, second string) bool {
	for index := 0; index+1 < len(tokens); index++ {
		if tokens[index] == first && tokens[index+1] == second {
			return true
		}
	}
	return false
}
