// Package schematicassistant provides an offline, confirmed-specification workflow.
// It is a bounded passive-circuit lane, not behavioral synthesis or fabrication approval.
package schematicassistant

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"regexp"
	"strconv"
	"strings"
)

const Schema = "kicadai.schematic-assistant.v1"
const MaxInputBytes = 64 << 10
const PowerBudget = 0.125
const InputMin = 0.1
const InputMax = 24.0
const ResistanceMin = 1000.0
const ResistanceMax = 1e6
const LoadMax = 1e9
const CapacitanceMin = 1e-12
const CapacitanceMax = 1e-3

func Families() []string { return []string{"divider", "rc-filter"} }
func SupportedLimits(family string) []string {
	limits := []string{fmt.Sprintf("Input: %g–%g V; R1/R2: %g–%g ohms", InputMin, InputMax, ResistanceMin, ResistanceMax), fmt.Sprintf("Load: %g–%g ohms, or 0 for explicitly unloaded (not shorted)", ResistanceMin, LoadMax), fmt.Sprintf("Predicted resistor dissipation <= %g W (workflow budget, not a qualified part rating)", PowerBudget)}
	if family == "rc-filter" {
		limits = append(limits, fmt.Sprintf("Capacitance: %g–%g F", CapacitanceMin, CapacitanceMax))
	}
	return limits
}

var namePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,47}$`)

type Spec struct {
	Name   string   `json:"name"`
	Family string   `json:"family"`
	InputV *float64 `json:"input_v,omitempty"`
	R1     *float64 `json:"r1_ohms,omitempty"`
	R2     *float64 `json:"r2_ohms,omitempty"`
	C      *float64 `json:"capacitance_f,omitempty"`
	Load   *float64 `json:"load_ohms,omitempty"`
}
type Session struct {
	Schema             string `json:"schema"`
	Spec               Spec   `json:"spec"`
	ConfirmationSHA256 string `json:"confirmation_sha256,omitempty"`
}
type Question struct {
	Field  string `json:"field"`
	Prompt string `json:"prompt"`
}
type Measurement struct {
	Name        string  `json:"name"`
	Value       float64 `json:"value"`
	Unit        string  `json:"unit"`
	Explanation string  `json:"explanation"`
}

func Number(value float64) *float64 { return &value }
func Example(family string) (Session, error) {
	s := Spec{Name: strings.ReplaceAll(family, "-", "_"), Family: family, InputV: Number(5), R1: Number(10000), Load: Number(0)}
	switch family {
	case "divider":
		s.R2 = Number(10000)
	case "rc-filter":
		s.C = Number(100e-9)
	default:
		return Session{}, errors.New("supported families: divider, rc-filter")
	}
	return Confirm(s)
}
func Questions(s Spec) []Question {
	var q []Question
	if !namePattern.MatchString(s.Name) {
		q = append(q, Question{"name", "Project name (lowercase letters, digits, underscore; start with a letter):"})
	}
	if s.Family != "divider" && s.Family != "rc-filter" {
		return append(q, Question{"family", "Circuit family: divider or rc-filter?"})
	}
	if (s.Family == "divider" && s.C != nil) || (s.Family == "rc-filter" && s.R2 != nil) {
		return append(q, Question{"family", "Incompatible family-specific fields. Re-select divider or rc-filter to clear them:"})
	}
	numeric := []struct {
		field, prompt string
		p             *float64
		min, max      float64
	}{
		{"input_v", "Maximum positive input voltage in V (0.1–24):", s.InputV, InputMin, InputMax},
		{"r1_ohms", "R1 resistance in ohms (1k–1M; e.g. 10k):", s.R1, ResistanceMin, ResistanceMax},
		{"load_ohms", "Output load in ohms (1k–1G), or 0 for explicitly unloaded/high impedance:", s.Load, 0, LoadMax},
	}
	if s.Family == "divider" {
		numeric = append(numeric, struct {
			field, prompt string
			p             *float64
			min, max      float64
		}{"r2_ohms", "R2 resistance in ohms (1k–1M):", s.R2, ResistanceMin, ResistanceMax})
	}
	if s.Family == "rc-filter" {
		numeric = append(numeric, struct {
			field, prompt string
			p             *float64
			min, max      float64
		}{"capacitance_f", "C1 capacitance in farads (1p–1m; e.g. 100n):", s.C, CapacitanceMin, CapacitanceMax})
	}
	for _, n := range numeric {
		if n.p == nil || !finite(*n.p) || *n.p < n.min || *n.p > n.max || (n.field == "load_ohms" && *n.p > 0 && *n.p < ResistanceMin) {
			q = append(q, Question{n.field, n.prompt})
		}
	}
	return q
}
func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
func ParseNumber(text string) (float64, error) {
	text = strings.TrimSpace(text)
	if text == "" || len(text) > 64 {
		return 0, errors.New("enter one finite SI value, e.g. 10k or 100n; no unit suffix")
	}
	exponent := 0
	switch text[len(text)-1] {
	case 'G':
		exponent = 9
	case 'M':
		exponent = 6
	case 'k':
		exponent = 3
	case 'm':
		exponent = -3
	case 'u':
		exponent = -6
	case 'n':
		exponent = -9
	case 'p':
		exponent = -12
	}
	if exponent != 0 {
		text = text[:len(text)-1]
	}
	mantissa, exp, hasExp := strings.Cut(strings.ToLower(text), "e")
	if hasExp {
		value, err := strconv.Atoi(exp)
		if err != nil || value < -1000 || value > 1000 {
			return 0, errors.New("invalid exponent")
		}
		exponent += value
	}
	value, err := strconv.ParseFloat(mantissa+"e"+strconv.Itoa(exponent), 64)
	if err != nil || !finite(value) {
		return 0, errors.New("enter a finite number; M means mega, m means milli")
	}
	return value, nil
}
func Set(s *Spec, field, text string) error {
	if field == "name" {
		s.Name = strings.TrimSpace(text)
		return nil
	}
	if field == "family" {
		s.Family = strings.TrimSpace(text)
		s.R2 = nil
		s.C = nil
		return nil
	}
	value, err := ParseNumber(text)
	if err != nil {
		return err
	}
	switch field {
	case "input_v":
		s.InputV = Number(value)
	case "r1_ohms":
		s.R1 = Number(value)
	case "r2_ohms":
		s.R2 = Number(value)
	case "capacitance_f":
		s.C = Number(value)
	case "load_ohms":
		s.Load = Number(value)
	default:
		return errors.New("unknown specification field")
	}
	return nil
}
func Analyze(s Spec) ([]Measurement, error) {
	if q := Questions(s); len(q) != 0 {
		return nil, fmt.Errorf("specification needs clarification: %s", q[0].Field)
	}
	if (s.Family == "divider" && s.C != nil) || (s.Family == "rc-filter" && s.R2 != nil) {
		return nil, errors.New("fields from another family are not accepted")
	}
	vin, r, load := *s.InputV, *s.R1, *s.Load
	parallel := func(a, b float64) float64 {
		if b == 0 {
			return a
		}
		return a * b / (a + b)
	}
	var values []Measurement
	var powers []float64
	if s.Family == "divider" {
		bottom := parallel(*s.R2, load)
		out := vin * bottom / (r + bottom)
		p1 := (vin - out) * (vin - out) / r
		p2 := out * out / (*s.R2)
		values = []Measurement{{"Output voltage", out, "V", "VIN × (R2 || load) / (R1 + (R2 || load)); 0 load means no load resistor."}, {"Input current", (vin - out) / r, "A", "Current drawn from the ideal input source."}, {"Output resistance", parallel(r, *s.R2), "ohm", "Thevenin resistance before adding the specified load."}, {"R1 dissipation", p1, "W", "Ideal DC power; compare with a real part's derated rating."}, {"R2 dissipation", p2, "W", "Ideal DC power; not a manufacturer thermal model."}}
		powers = []float64{p1, p2}
		if load > 0 {
			p := out * out / load
			values = append(values, Measurement{"Load dissipation", p, "W", "Power in the explicitly included load resistor."})
			powers = append(powers, p)
		}
	} else {
		equivalent := parallel(r, load)
		gain := 1.0
		if load > 0 {
			gain = load / (r + load)
		}
		tau := equivalent * (*s.C)
		p := vin * vin / r
		values = []Measurement{{"DC gain", gain, "V/V", "Ideal zero-impedance source; finite load reduces DC gain."}, {"Cutoff frequency", 1 / (2 * math.Pi * tau), "Hz", "1 / (2π × (R1 || load) × C1)."}, {"Time constant", tau, "s", "Ideal first-order step response."}, {"95% settling time", -math.Log(0.05) * tau, "s", "Time to reach 95% of the final value, initially uncharged capacitor."}, {"R1 worst step dissipation", p, "W", "VIN²/R1 at the start of a 0→VIN step."}}
		powers = []float64{p}
		if load > 0 {
			out := vin * gain
			lp := out * out / load
			values = append(values, Measurement{"Load DC dissipation", lp, "W", "DC power in the explicitly included load resistor."})
			powers = append(powers, lp)
		}
	}
	for _, p := range powers {
		if p > PowerBudget {
			return nil, fmt.Errorf("predicted resistor dissipation %.6g W exceeds this lane's %.3g W budget; reduce input voltage or increase resistance (no component rating is inferred)", p, PowerBudget)
		}
	}
	return values, nil
}
func SpecHash(s Spec) (string, error) {
	raw, err := json.Marshal(s)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}
func Confirm(s Spec) (Session, error) {
	if _, err := Analyze(s); err != nil {
		return Session{}, err
	}
	hash, err := SpecHash(s)
	return Session{Schema: Schema, Spec: s, ConfirmationSHA256: hash}, err
}
func (s Session) ValidateConfirmation() error {
	if s.Schema != Schema {
		return errors.New("unsupported assistant session schema")
	}
	confirmed, err := Confirm(s.Spec)
	if err != nil {
		return err
	}
	if s.ConfirmationSHA256 == "" || s.ConfirmationSHA256 != confirmed.ConfirmationSHA256 {
		return errors.New("specification is unconfirmed or changed; run schematic resume to confirm it")
	}
	return nil
}
func Decode(reader io.Reader) (Session, error) {
	raw, err := io.ReadAll(io.LimitReader(reader, MaxInputBytes+1))
	if err != nil {
		return Session{}, err
	}
	if len(raw) > MaxInputBytes {
		return Session{}, errors.New("session exceeds 64 KiB")
	}
	var session Session
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&session); err != nil {
		return Session{}, err
	}
	var trailing any
	if decoder.Decode(&trailing) != io.EOF {
		return Session{}, errors.New("session must contain exactly one JSON object")
	}
	if session.Schema != Schema {
		return Session{}, errors.New("unsupported assistant session schema")
	}
	return session, nil
}
