package boardfamily

// FamilySpec describes available reference designs, not an open-ended circuit
// synthesis capability or a statement that manufactured hardware was tested.
type FamilySpec struct {
	ID                   string    `json:"id"`
	SensorMPN            string    `json:"sensor_mpn"`
	SensorAddress        string    `json:"sensor_i2c_address"`
	Capabilities         []string  `json:"capabilities"`
	Unsupported          []string  `json:"unsupported"`
	Profiles             []Profile `json:"profiles"`
	DefaultConfiguration Config    `json:"default_configuration"`
	Conditions           []string  `json:"conditions"`
}

// Catalog returns fresh data so callers cannot modify a later caller's contract.
func Catalog() []FamilySpec {
	var result []FamilySpec
	for _, id := range []string{Family, FamilySHT31} {
		profiles := ProfilesFor(id)
		c := Config{"1", id, "standard", 3.2, 3.4, 1000, 10, 35, profiles[0].MaxBusPF}
		f := FamilySpec{
			ID: id, Profiles: profiles, DefaultConfiguration: c,
			Unsupported: []string{"wireless/radio operation", "USB or RS-232", "battery management or whole-board low-power guarantees", "regulator or protection circuitry", "external GPIO loads or added peripherals", "custom geometry or routing", "delivered firmware", "measured or certified hardware performance"},
			Conditions:  []string{"Fixed 120x80 mm, two copper layers, 1.6 mm FR4; ESP32-WROOM-32E-N4, external regulated 3.3 V, reset/boot and UART programming.", "Operating limits are declared bounds, not measurements: 3.2–3.4 V including ripple, source capability 1000–10000 mA, 10–35 C and 35–60% RH noncondensing indoor air; 50 mVpp maximum sensor-supply ripple.", "No hot plug or added pull-ups. Keep radios off; use 3.3 V UART, common ground and GPIO21 SDA / GPIO22 SCL. Hold controller RESET until power is stable.", "Total I2C capacitance includes board, pads, pins and connections. Assembly, firmware and bench characterization remain the user's responsibility."},
		}
		if id == Family {
			f.SensorMPN, f.SensorAddress = "BMP280", "0x76"
			f.Capabilities = []string{"air pressure measurement, 300–1100 hPa", "wired ESP32 sensor/controller with UART programming", "standard, fast or explicitly pull-up-only low_current I2C profile"}
			f.Unsupported = append(f.Unsupported, "humidity or accurate ambient-temperature measurement")
			f.Conditions = append(f.Conditions, "Firmware must apply BMP280 calibration coefficients; sensor die temperature is for compensation, not a qualified ambient thermometer.")
		} else {
			f.SensorMPN, f.SensorAddress = "SHT31-DIS-B2.5kS", "0x44"
			f.Capabilities = []string{"temperature and relative-humidity sensing within the declared indoor envelope", "wired ESP32 sensor/controller with UART programming", "standard or fast I2C profile"}
			f.Unsupported = append(f.Unsupported, "pressure measurement", "low_current or 10k pull-up profile", "guaranteed assembled-board ambient measurement accuracy")
			f.Conditions = append(f.Conditions, "Keep the sensor heater off, validate CRC and convert readings in firmware. Controller heat and sensor exposure affect ambient measurements.", "Supply slew within the sensor operating range must stay below 20 V/ms. Wait at least 1 ms for power-up and 1.5 ms after sensor soft reset. Controller RESET does not reset the sensor.", "Follow Sensirion handling/assembly instructions: do not wash or coat the sensor opening; prevent contamination.")
		}
		result = append(result, f)
	}
	return result
}
