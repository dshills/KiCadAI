# Controlled SHT31 prototype bring-up procedure

Status: procedure only. No board has been powered, programmed or measured for
this delivery. Execute only after explicit hardware/test authorization and the
[engineering release holds](REVIEW.md) are closed. Qualified operator required.

## Equipment and records

Use an ESD-controlled bench, insulated support (no mounting holes), current-limited
regulated supply, DMM, oscilloscope with low-capacitance probes/short ground springs,
3.3 V logic UART adapter and logic analyzer if available. All probe loading must
fit the declared 70 pF total bus budget. A calibrated independent T/RH reference
is required for comparison, but this procedure does not calibrate the board.
Record instrument models, serials, calibration dates and probe capacitances.

Create a new run folder per board/session. Copy `measurement-record.json` from
this directory, fill identification and approvals, and leave unperformed checks
as `not_run` (never default pass). Retain raw UART log, programmer log, rail/EN/bus
waveforms, assembly photographs and reference readings. Record firmware/package
SHA-256 and board serial. Failures are retained; a rerun gets a new folder.

## 1. Unpowered inspection: stop on any discrepancy

- Verify exact BOM/profile: 16 populated references, U2 SHT31-DIS-B2.5kS,
  U1 ESP32-WROOM-32E-N4, R3/R4 4.7 kohm. Inspect U2 pin 1 and cavity; check module
  orientation, exposed-pad process and J1/J2 pin numbering using native pad numbers.
- Inspect solder bridges, misplaced parts, damage and contamination. Check that
  J1.1 is VCC, J1.2 is ground, J2.2 ground, J2.3 GPIO3/RX, J2.4 GPIO1/TX.
- DMM-test intended ground continuity and power/ground isolation. Allow capacitors
  to charge in resistance mode; record the stabilized reading. There is no valid
  universal resistance threshold for this assembly; a persistent low resistance
  or disagreement with the schematic is a hold for engineering diagnosis.
- Check U2 pin/pad connectivity where accessible with suitable probes; do not scrape
  or press into the sensor opening. Use optical/X-ray inspection as appropriate for
  hidden pads. Never bridge fine-pitch pads with powered probes.

## 2. Power first, UART disconnected

1. With output OFF, set 3.30 V and an initial 0.10 A current limit. Connect J1 only,
   observing polarity. Hold RESET low. This initial current limit is a cautious
   short-screen, not the running-current requirement or an acceptance value.
2. Enable supply. Stop/remove power immediately for reversed polarity, smoke, odor,
   unexpected heating or sustained current limit. Verify VCC/GND at J1 and both
   ICs. Do not raise the current limit to overcome a held-reset fault.
3. Capture rail startup at U2 and EN. Require stable 3.2-3.4 V including transients,
   <=50 mVpp sensor-rail ripple and <20 V/ms slew within the sensor operating range.
   Record scope bandwidth, sample rate and acquisition settings; retain full-band
   transients as well as a declared 20 MHz-limited ripple trace.
4. After held-reset checks pass, output OFF; set running current limit to 1.0 A
   on a supply capable of at least 1 A. Reapply with RESET held, wait for a stable
   rail, then release. Capture rail and EN through startup. No current limiting,
   brownout/restart loop or rail-envelope violation is allowed. Record actual
   steady/peak current; the design's ~595 mA allocation is not a measured current
   target or maximum-current guarantee. Investigate unexplained heat/current.

## 3. Program and start

With output OFF, connect UART GND/TX/RX as in the firmware README, leaving J2.1
and DTR/RTS disconnected. Ensure adapter TX is tri-stated/disconnected whenever
the board is unpowered; connect/disconnect only with both sides safely inactive.
Do not back-power through I/O. Follow the manual BOOT/RESET sequence and flash/
verify commands in [firmware README](../../../firmware/sht31-bringup/README.md).
Do not erase unrelated hardware; explicitly select the intended adapter port.

Start capture before a manual RESET without BOOT. Require one `boot`, one `ready`
and then samples, no `fault`, brownout or extra boot. A missing SHT31/incorrect
address must fail initialization; do not scan for or silently select another sensor.
An ACK at 0x44 alone does not authenticate an SHT31 part: inspect its marking/BOM.

## 4. Bus and sensor checks

At U1 and U2 measure SDA/SCL with a verified low-loading probe. Confirm address
0x44, 100 kHz nominal clock (firmware target, measure actual), command 0x2400,
at least 20 ms command-to-read delay, six bytes and final NACK/STOP. Confirm
status command and heater-off state. Keep J3 otherwise unloaded.

For the selected standard-clock prototype, require SCL <=100 kHz, SCL low >=4.7 us,
high >=4.0 us, START hold >=4.0 us, repeated-START setup >=4.7 us, STOP setup >=4.0 us,
bus-free >=4.7 us and data setup >=250 ns. Check SDA hold against the vendor timing
table; do not infer it from nominal frequency. Use 30%-70% rise and 70%-30% fall:
both <=300 ns (SHT31's stricter rise constraint applies even at 100 kHz).
Check both receivers' high/low levels: high >=0.75 VCC, low <=0.25 VCC at U1;
high >=0.7 VCC, low <=0.3 VCC at U2; sensor-driven low <=0.4 V under this load.
Controller thresholds in its datasheet have nominal test-condition limits; these
are bench screening criteria, not a new all-temperature silicon guarantee.
Retain waveforms, not only logic-analyzer decoded text. Timing references:
[NXP UM10204](https://www.nxp.com/docs/en/user-guide/UM10204.pdf) and
[SHT3x-DIS Table 21](https://sensirion.com/media/documents/213E6A3B/63A5A569/Datasheet_SHT3x_DIS.pdf).

After thermal settling, capture at least ten minutes (>=600 s between first/last
sample, >=580 consecutive valid samples). Run the serial-log verifier. Record
reference T/RH, sensor T/RH and differences; note distance/airflow/enclosure and
reference uncertainty. Keep environment within 10-35 C and 35-60% RH, noncondensing.
Readings outside that envelope, abrupt discontinuities, unstable offsets or drift
need engineering investigation. Do not claim ambient accuracy merely because
readings look plausible or track a reference. Do not blow/breathe on the sensor,
apply liquids/solvents, enable its heater or force an out-of-envelope test.

## 5. Repeatability and negative checks

- Perform three controlled cold starts and three manual RESET starts, preserving
  logs. Before cold starts disable/disconnect UART injection and discharge rails;
  confirm no back-power. Every run must initialize and sample without faults.
- A missing/unpopulated-sensor test is optional only on a separately designated
  assembly prepared while unpowered; never desolder a working sensor or short a
  live bus for a test. Expected result: initialization fault, no valid samples.
  CRC corruption, heater/reset flags and I/O timeouts are already injected in
  host tests; physical fault injection needs a separate approved fixture.
- Controlled 3.2/3.4 V endpoint tests may follow nominal success within the existing
  envelope, with actual rail kept inside limits. Temperature/humidity chamber,
  RF, EMC, lifecycle and production-accuracy qualification are separate work.

## Exit decision

Mark the board `prototype_bringup_pass` only when all mandatory electrical,
programming, bus, ten-minute and restart checks pass with raw evidence and a named
reviewer. Unperformed optional checks remain `not_run`. Any fault gives
`hold_for_diagnosis`; turn off power, preserve evidence, identify root cause, fix
generator/firmware source as appropriate and regenerate a new package. Neither
status authorizes production, ordering more units or an accuracy claim.
