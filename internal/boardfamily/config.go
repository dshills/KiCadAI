// Package boardfamily configures bounded, engineered native board references.
// It does not search architectures, substitute arbitrary parts, place or autoroute.
package boardfamily

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
)

const Family = "esp32_bmp280_v1"
const FamilySHT31 = "esp32_sht31_v1"

type Config struct {
	Version               string  `json:"version"`
	Family                string  `json:"family"`
	Profile               string  `json:"profile"`
	SupplyMinV            float64 `json:"supply_min_v"`
	SupplyMaxV            float64 `json:"supply_max_v"`
	SupplyCapacityMA      float64 `json:"supply_capacity_ma"`
	AmbientMinC           float64 `json:"ambient_min_c"`
	AmbientMaxC           float64 `json:"ambient_max_c"`
	TotalBusCapacitancePF float64 `json:"total_bus_capacitance_pf"`
}

type Profile struct {
	ID             string  `json:"id"`
	ClockHz        int     `json:"clock_hz"`
	ResistanceOhms float64 `json:"resistance_ohms"`
	Value          string  `json:"value"`
	MPN            string  `json:"mpn"`
	MaxBusPF       float64 `json:"max_bus_pf"`
	RiseLimitNS    float64 `json:"rise_limit_ns"`
}

func Profiles() []Profile {
	return []Profile{
		{"standard", 100000, 4700, "4.7k", "RC0805FR-074K7L", 200, 1000},
		{"fast", 400000, 2200, "2.2k", "RC0805FR-072K2L", 100, 300},
		{"low_current", 100000, 10000, "10k", "RC0805FR-0710KL", 100, 1000},
	}
}

// ProfilesFor returns a fresh profile list. Profiles retains its original BMP280
// meaning for callers replaying the first-family contract.
func ProfilesFor(family string) []Profile {
	switch family {
	case Family:
		return Profiles()
	case FamilySHT31:
		return []Profile{
			{"standard", 100000, 4700, "4.7k", "RC0805FR-074K7L", 70, 300},
			{"fast", 400000, 2200, "2.2k", "RC0805FR-072K2L", 100, 300},
		}
	default:
		return nil
	}
}

func Decode(r io.Reader) (Config, error) {
	var c Config
	b, err := io.ReadAll(io.LimitReader(r, 65537))
	if err != nil {
		return c, err
	}
	if len(b) > 65536 {
		return c, errors.New("configuration exceeds 65536-byte limit")
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if e := d.Decode(&c); e != nil {
		return c, e
	}
	var extra any
	if e := d.Decode(&extra); e != io.EOF {
		return c, errors.New("configuration must contain exactly one JSON object")
	}
	return c, nil
}

type Electrical struct {
	Profile            Profile  `json:"profile"`
	RiseTimeNS         float64  `json:"worst_case_30_70_rise_ns"`
	TimeTo80PercentNS  float64  `json:"time_to_80_percent_ns"`
	PullupSinkMA       float64  `json:"worst_case_sink_ma_per_line"`
	PullupPowerMW      float64  `json:"worst_case_resistor_power_mw"`
	AllocatedCurrentMA float64  `json:"allocated_current_ma"`
	SourceMarginMA     float64  `json:"source_margin_ma"`
	Notes              []string `json:"notes"`
}

func Check(c Config) (Electrical, error) {
	var e Electrical
	for _, x := range []float64{c.SupplyMinV, c.SupplyMaxV, c.SupplyCapacityMA, c.AmbientMinC, c.AmbientMaxC, c.TotalBusCapacitancePF} {
		if math.IsNaN(x) || math.IsInf(x, 0) {
			return e, errors.New("non-finite operating limit")
		}
	}
	profiles := ProfilesFor(c.Family)
	if c.Version != "1" || len(profiles) == 0 {
		return e, errors.New("unsupported family or configuration version")
	}
	for _, p := range profiles {
		if p.ID == c.Profile {
			e.Profile = p
		}
	}
	if e.Profile.ID == "" {
		return e, errors.New("unsupported I2C profile")
	}
	if c.SupplyMinV < 3.2 || c.SupplyMaxV > 3.4 || c.SupplyMinV > c.SupplyMaxV {
		return e, errors.New("external regulated input must remain between 3.2 and 3.4 V, including ripple")
	}
	if c.SupplyCapacityMA < 1000 || c.SupplyCapacityMA > 10000 {
		return e, errors.New("source capability must be 1000–10000 mA; this is capability, not allowed load current")
	}
	if c.AmbientMinC < 10 || c.AmbientMaxC > 35 || c.AmbientMinC > c.AmbientMaxC {
		return e, errors.New("qualified ambient range is 10–35 C, 35–60% RH, noncondensing indoor air")
	}
	if c.TotalBusCapacitancePF < 50 || c.TotalBusCapacitancePF > e.Profile.MaxBusPF {
		return e, fmt.Errorf("total bus capacitance must be 50–%.0f pF for %s, including board, pins and connections", e.Profile.MaxBusPF, c.Profile)
	}
	// 1% resistance tolerance plus 0.15% temperature drift over 10–35 C.
	rmax := e.Profile.ResistanceOhms * 1.0115
	rmin := e.Profile.ResistanceOhms * 0.9885
	e.RiseTimeNS = 0.8473 * rmax * c.TotalBusCapacitancePF / 1000
	e.TimeTo80PercentNS = math.Log(5) * rmax * c.TotalBusCapacitancePF / 1000
	// Include Bosch's minimum 70k internal pull-up; ESP32 internal pull-ups must be disabled.
	e.PullupSinkMA = c.SupplyMaxV * (1/rmin + 1.0/70000.0) * 1000
	sensorPeakMA := 1.12
	if c.Family == FamilySHT31 {
		// SHT3x-DIS datasheet v7 Tables 3 and 7: external bus pulls only,
		// 1.5 mA maximum measuring current, with the heater disabled.
		e.PullupSinkMA = c.SupplyMaxV / rmin * 1000
		sensorPeakMA = 1.5
	}
	e.PullupPowerMW = c.SupplyMaxV * c.SupplyMaxV / rmin * 1000
	// 500 mA controller allocation is based on Espressif's supply recommendation,
	// not a claimed measured maximum. Add the sensor peak and two bus sinks,
	// 2 mA control-network allocation and 90 mA engineering reserve.
	e.AllocatedCurrentMA = 500 + sensorPeakMA + 2*e.PullupSinkMA + 2 + 90
	e.SourceMarginMA = c.SupplyCapacityMA - e.AllocatedCurrentMA
	if e.RiseTimeNS > e.Profile.RiseLimitNS || e.PullupSinkMA > 3 || e.PullupPowerMW > 62.5 || e.SourceMarginMA < 0 {
		return e, errors.New("electrical margin check failed")
	}
	e.Notes = []string{"External regulated 3.3 V only; no regulator, reverse-polarity, surge, ESD, or hot-plug protection.", "Radio disabled; external GPIO loads and extra pull-ups unsupported. UART is 3.3 V logic, not USB or RS-232.", "I2C capacitance is a declared total bound, not a measurement; validate the assembled bus before use.", "Firmware must set the selected I2C clock, disable internal pulls, use SDA GPIO21 / SCL GPIO22, and apply BMP280 calibration coefficients.", "Supply ripple at the sensor must not exceed 50 mV peak-to-peak. Keep RESET held until power is stable; no automatic-reset guarantee.", "Software-qualified reference, not manufactured-hardware performance or EMC certification."}
	if c.Family == FamilySHT31 {
		e.Notes[3] = "Firmware must set the selected I2C clock, disable controller internal pulls, use SDA GPIO21 / SCL GPIO22 at address 0x44, keep the SHT31 heater off, validate CRC and convert digital temperature/humidity readings. No firmware is delivered."
		e.Notes[4] = "Supply ripple at the sensor must not exceed 50 mV peak-to-peak; supply slew within the sensor operating range must stay below 20 V/ms. Hold controller RESET until power is stable, wait at least 1 ms for sensor power-up and 1.5 ms after sensor soft reset; controller RESET does not reset the sensor."
		e.Notes = append(e.Notes, "SHT31 temperature/humidity capability excludes pressure measurement and guaranteed assembled-board ambient accuracy; controller heat, contamination, enclosure and airflow require bench characterization. No coating or wash over the sensor opening; follow Sensirion handling and assembly instructions.")
	}
	return e, nil
}
