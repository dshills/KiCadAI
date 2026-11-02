package boardfamily

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestReferencedRequestSensorChoices(t *testing.T) {
	for _, tc := range []struct {
		prompt string
		want   []string
	}{
		{"Please make a pressure monitor.", nil},
		{"I'd like temperature and humidity sensing.", nil},
		{"Can you build a sensor controller?", nil},
		{"Use bmp280, please.", []string{"BMP280"}},
		{"Use (SHT31).", []string{"SHT31"}},
		{"Either BMP280 or SHT31; I have not decided.", []string{"BMP280", "SHT31"}},
		{"Do not use BMP280; I need SHT31.", []string{"BMP280", "SHT31"}},
		{"BMP280 is background only; I have not selected a sensor.", []string{"BMP280"}},
		{"Use XBMP280Y or SHT31D.", nil},
		{"Use BMP-280.", nil}, // Same literal identity rule as the existing decoder.
	} {
		t.Run(tc.prompt, func(t *testing.T) {
			request, _, err := prepareReferencedGenerateRequest(tc.prompt)
			if err != nil {
				t.Fatal(err)
			}
			facts := request.OutputSchema["properties"].(map[string]any)["facts"].(map[string]any)
			var got []string
			for _, variant := range facts["items"].(map[string]any)["anyOf"].([]any) {
				p := variant.(map[string]any)["properties"].(map[string]any)
				if p["kind"].(map[string]any)["enum"].([]string)[0] == "sensor" {
					got = p["value"].(map[string]any)["enum"].([]string)
				}
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("sensor choices: got %v, want %v", got, tc.want)
			}
			// Availability is not a positive requirement. Named prohibitions,
			// unresolved choices and background mentions must not disappear.
			for _, sensor := range []string{"BMP280", "SHT31"} {
				for _, state := range []string{"required", "not_required", "forbidden", "uncertain"} {
					f := RequirementFact{Kind: "sensor", Value: sensor, State: state, Quote: tc.prompt}
					encoded, err := json.Marshal(f)
					if err != nil {
						t.Fatal(err)
					}
					allowed := validateFact(f, encoded, tc.prompt) == nil
					raw := syntheticReferencedIntent(t, referencedChoice("sensor", sensor, state, 0))
					if (checkReferencedSchemaJSON(request.OutputSchema, raw) == nil) != allowed {
						t.Fatal("request schema disagrees with the existing identity validator")
					}
				}
			}
		})
	}
}

func TestReferencedRequestWithoutSensorNameStillSelectsBothFamilies(t *testing.T) {
	for _, tc := range []struct{ prompt, measurement, family string }{
		{"I need barometric measurements.", "pressure", "esp32_bmp280_v1"},
		{"Please measure humidity.", "humidity", "esp32_sht31_v1"},
	} {
		t.Run(tc.family, func(t *testing.T) {
			request, _, err := prepareReferencedGenerateRequest(tc.prompt)
			if err != nil {
				t.Fatal(err)
			}
			raw := syntheticReferencedIntent(t, referencedChoice("measurement", tc.measurement, "required", 0))
			if err := checkReferencedSchemaJSON(request.OutputSchema, raw); err != nil {
				t.Fatal(err)
			}
			d, err := DecodeReferencedIntent(tc.prompt, raw)
			if err != nil || d.Configuration == nil || d.Configuration.Family != tc.family {
				t.Fatalf("measurement request lost: %+v, %v", d, err)
			}
		})
	}
}

func TestReferencedRequestRejectsCapturedInventedSensorAtSchemaBoundary(t *testing.T) {
	file := filepath.Join("..", "..", "specs", "board-family-v2", "indexed-evaluation-03", "batch", "useful-01", "journal", "selection", "selection.json")
	b, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	var saved ReferencedSelection
	if err := json.Unmarshal(b, &saved); err != nil {
		t.Fatal(err)
	}
	if len(saved.RawIntent) == 0 {
		t.Fatal("captured raw extraction missing")
	}
	request, _, err := prepareReferencedGenerateRequest(saved.OriginalRequest)
	if err != nil {
		t.Fatal(err)
	}
	if err := checkReferencedSchemaJSON(request.OutputSchema, saved.RawIntent); err == nil {
		t.Fatal("request-specific schema still permits the captured invented sensor")
	}
}
