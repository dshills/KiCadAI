package simmodel

const (
	RegistryVersion = "kicadai.trusted-simulation-registry.v1"
	ReportSchema    = "kicadai.trusted-simulation-report.v1"

	ModelLinearRegulatorIdealV1 = "linear_regulator_ideal_v1"
	ModelResistorDividerDCV1    = "resistor_divider_dc_v1"
	ModelRCLowpassACV1          = "rc_lowpass_ac_v1"
)

type NamedValue struct {
	Name  string  `json:"name"`
	Value float64 `json:"value"`
}

type CatalogEvidence struct {
	ModelID    string       `json:"model_id"`
	Parameters []NamedValue `json:"parameters,omitempty"`
}

type Binding struct {
	Role      string `json:"role"`
	Component string `json:"component"`
}

type Assertion struct {
	Metric string  `json:"metric"`
	Min    float64 `json:"min"`
	Max    float64 `json:"max"`
}

// Intent contains only trusted model selection, component bindings, bounded
// scalar operating conditions, and assertions. It deliberately has no model
// text, include path, command, expression, or executable field.
type Intent struct {
	ModelID    string       `json:"model_id"`
	Bindings   []Binding    `json:"bindings"`
	Inputs     []NamedValue `json:"inputs"`
	Assertions []Assertion  `json:"assertions"`
}

type ComponentEvidence struct {
	InstanceID  string
	CatalogID   string
	Family      string
	ValueSI     float64
	HasValueSI  bool
	ModelClaims []CatalogEvidence
}

type ResolvedBinding struct {
	Role            string       `json:"role"`
	Component       string       `json:"component"`
	CatalogID       string       `json:"catalog_id"`
	Family          string       `json:"family"`
	ValueSI         *float64     `json:"value_si,omitempty"`
	ModelParameters []NamedValue `json:"model_parameters,omitempty"`
}

type Plan struct {
	RegistryVersion string            `json:"registry_version"`
	RegistryHash    string            `json:"registry_hash"`
	CatalogID       string            `json:"catalog_id"`
	CatalogHash     string            `json:"catalog_hash"`
	ModelID         string            `json:"model_id"`
	Bindings        []ResolvedBinding `json:"bindings"`
	Inputs          []NamedValue      `json:"inputs"`
	Assertions      []Assertion       `json:"assertions"`
}

func ClonePlan(source Plan) Plan {
	clone := source
	clone.Bindings = append([]ResolvedBinding(nil), source.Bindings...)
	for index := range clone.Bindings {
		clone.Bindings[index].ModelParameters = append([]NamedValue(nil), source.Bindings[index].ModelParameters...)
		if source.Bindings[index].ValueSI != nil {
			value := *source.Bindings[index].ValueSI
			clone.Bindings[index].ValueSI = &value
		}
	}
	clone.Inputs = append([]NamedValue(nil), source.Inputs...)
	clone.Assertions = append([]Assertion(nil), source.Assertions...)
	return clone
}

type Diagnostic struct {
	Path       string `json:"path"`
	Message    string `json:"message"`
	Suggestion string `json:"suggestion,omitempty"`
}

type Measurement struct {
	Metric string  `json:"metric"`
	Value  float64 `json:"value"`
}

type AssertionResult struct {
	Metric string  `json:"metric"`
	Min    float64 `json:"min"`
	Max    float64 `json:"max"`
	Actual float64 `json:"actual"`
	Pass   bool    `json:"pass"`
}

type Report struct {
	Schema          string            `json:"schema"`
	RegistryVersion string            `json:"registry_version"`
	RegistryHash    string            `json:"registry_hash"`
	CatalogID       string            `json:"catalog_id"`
	CatalogHash     string            `json:"catalog_hash"`
	ModelID         string            `json:"model_id"`
	Bindings        []ResolvedBinding `json:"bindings"`
	Inputs          []NamedValue      `json:"inputs"`
	Measurements    []Measurement     `json:"measurements"`
	Assertions      []AssertionResult `json:"assertions"`
	Status          string            `json:"status"`
}
