package aiprovider

import (
	"errors"
	"testing"
)

func TestSelectReferenceProfile(t *testing.T) {
	tests := []struct {
		name    string
		prompt  string
		wantID  string
		wantErr error
	}{
		{name: "bmp280", prompt: "Create a protected USB-C BMP280 breakout", wantID: ReferenceProfileBMP280},
		{name: "bmp280_unicode_separator", prompt: "Create a USB‑C BMP280 breakout", wantID: ReferenceProfileBMP280},
		{name: "led", prompt: "Create a protected USB-C LED indicator", wantID: ReferenceProfileProtectedLED},
		{name: "plural_leds", prompt: "Create a USBC board with LEDs", wantID: ReferenceProfileProtectedLED},
		{name: "unsupported", prompt: "Create a USB-C motor controller", wantErr: ErrUnsupportedReferencePrompt},
		{name: "missing_usb_c", prompt: "Create an LED indicator", wantErr: ErrUnsupportedReferencePrompt},
		{name: "composition", prompt: "Create a USB-C BMP280 breakout with an LED", wantErr: ErrUnsupportedReferenceComposition},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			profile, err := SelectReferenceProfile(test.prompt)
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("error = %v, want %v", err, test.wantErr)
			}
			if profile.ID != test.wantID {
				t.Fatalf("profile = %q, want %q", profile.ID, test.wantID)
			}
		})
	}
}

func TestReferenceProfilesReturnFreshStrictSchemas(t *testing.T) {
	for _, profile := range []ReferenceProfile{BMP280Profile(), ProtectedLEDProfile()} {
		first := profile.IntentEnvelopeSchema()
		second := profile.IntentEnvelopeSchema()
		first["mutated"] = true
		if _, exists := second["mutated"]; exists {
			t.Fatalf("profile %q reused mutable schema", profile.ID)
		}
		if profile.SchemaName == "" || profile.CapabilityContext == "" {
			t.Fatalf("incomplete profile: %#v", profile)
		}
	}
}
