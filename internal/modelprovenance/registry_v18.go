package modelprovenance

import (
	"bytes"

	assets "kicadai"
)

// V18ExtensionPath is deliberately outside DefaultPath. LoadDefault remains
// the byte-for-byte historical registry constructor.
const V18ExtensionPath = "data/model-provenance/v18-extension.json"

// LoadV18 returns the historical registry plus the reviewed V18-only records.
func LoadV18() (Registry, []Diagnostic) {
	base, baseDiagnostics := LoadDefault()
	body, err := assets.DefaultModelProvenance.ReadFile(V18ExtensionPath)
	if err != nil {
		return Registry{}, append(baseDiagnostics, Diagnostic{Path: "document", Message: "read embedded V18 model provenance extension: " + err.Error()})
	}
	extension, extensionDiagnostics := DecodeStrict(bytes.NewReader(body))
	if len(baseDiagnostics) != 0 || len(extensionDiagnostics) != 0 {
		return Registry{}, append(baseDiagnostics, extensionDiagnostics...)
	}
	merged := Normalize(Registry{
		Schema:  Schema,
		Version: Version,
		Records: append(append([]Record(nil), base.Records...), extension.Records...),
	})
	return merged, Validate(merged)
}
