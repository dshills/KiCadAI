package aiprovider

import "slices"

func BMP280ReferenceIntentEnvelopeSchema() map[string]any {
	strength := map[string]any{"type": "string", "enum": []string{"required", "preferred", "optional", "forbidden"}}
	powerInput := strictObject(map[string]any{
		"kind":     map[string]any{"type": "string", "enum": []string{"usb_c"}},
		"voltage":  map[string]any{"type": "string", "enum": []string{"5V"}},
		"strength": strength,
	})
	powerRail := strictObject(map[string]any{
		"name":     map[string]any{"type": "string", "enum": []string{"VCC"}},
		"voltage":  map[string]any{"type": "string", "enum": []string{"3.3V"}},
		"strength": strength,
		"alias":    map[string]any{"type": "string", "enum": []string{"3v3"}},
	})
	interfaceIntent := strictObject(map[string]any{
		"kind":      map[string]any{"type": "string", "enum": []string{"i2c"}},
		"voltage":   map[string]any{"type": "string", "enum": []string{"3.3V"}},
		"connector": map[string]any{"type": "string", "enum": []string{"external"}},
		"strength":  strength,
		"bus":       map[string]any{"type": "string"},
	})
	functionParams := strictObject(map[string]any{
		"sensor_component_id": map[string]any{"type": "string", "enum": []string{"sensor.bosch.bmp280.lga8"}},
		"i2c_address":         map[string]any{"type": "string", "enum": []string{"0x76", "0x77"}},
		"supply_voltage":      map[string]any{"type": "string", "enum": []string{"3.3V"}},
		"include_pullups":     map[string]any{"type": "boolean"},
		"include_decoupling":  map[string]any{"type": "boolean"},
	})
	functionIntent := strictObject(map[string]any{
		"kind":      map[string]any{"type": "string", "enum": []string{"sensor"}},
		"family":    map[string]any{"type": "string", "enum": []string{"i2c_sensor"}},
		"params":    functionParams,
		"strength":  strength,
		"interface": map[string]any{"type": "string", "enum": []string{"i2c"}},
		"bus":       map[string]any{"type": "string"},
		"supply":    map[string]any{"type": "string"},
	})
	intent := strictObject(map[string]any{
		"version":               map[string]any{"type": "string", "enum": []string{"0.1.0"}},
		"name":                  map[string]any{"type": "string"},
		"kind":                  map[string]any{"type": "string", "enum": []string{"breakout"}},
		"acceptance":            map[string]any{"type": "string", "enum": []string{"erc-drc"}},
		"auto_schematic_layout": map[string]any{"type": "boolean", "const": true},
		"power": strictObject(map[string]any{
			"inputs": map[string]any{"type": "array", "items": powerInput},
			"rails":  map[string]any{"type": "array", "items": powerRail},
		}),
		"interfaces": map[string]any{"type": "array", "items": interfaceIntent},
		"functions":  map[string]any{"type": "array", "items": functionIntent},
		"protection": strictObject(map[string]any{
			"esd":              strength,
			"reverse_polarity": strength,
			"overcurrent":      strength,
			"transient":        strength,
			"bulk_capacitance": strength,
		}),
	})
	return strictObject(map[string]any{
		"schema": map[string]any{"type": "string", "enum": []string{EnvelopeSchemaV1}},
		"intent": intent,
	})
}

func strictObject(properties map[string]any) map[string]any {
	required := make([]string, 0, len(properties))
	for name := range properties {
		required = append(required, name)
	}
	slices.Sort(required)
	return map[string]any{
		"type":                 "object",
		"properties":           properties,
		"required":             required,
		"additionalProperties": false,
	}
}
