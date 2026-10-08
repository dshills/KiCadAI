# SHT31 controlled-prototype firmware

Only for `esp32_sht31_v1 / standard`: ESP32-WROOM-32E-N4, 4 MB flash,
GPIO21 SDA, GPIO22 SCL, 100 kHz, 0x44, external 4.7 kohm pull-ups.
This is a bench diagnostic, not production firmware or proof of working hardware.

## Build, without connecting hardware

Install PlatformIO Core 6.1.19 and the exact packages in `platformio.ini`.
The initial dependency installation may require network access. No provider key
or paid API is used. From the repository root:

```sh
pio run -d firmware/sht31-bringup -e sht31-standard
mkdir -p .cache/sht31-tests
c++ -std=c++11 -Wall -Wextra -Werror -fsanitize=address,undefined \
  -I firmware/sht31-bringup/include firmware/sht31-bringup/tests/test_sht31.cpp \
  -o .cache/sht31-tests/test-sht31
.cache/sht31-tests/test-sht31
node --test scripts/sht31-prototype/verify-run.test.mjs
```

The `esp32dev` build definition selects the ESP32/4 MB flash target only; it
does not imply this custom board contains a USB bridge, regulator, LED or
automatic reset circuit. Flash is DIO/40 MHz, CPU 160 MHz, factory partition
only, no OTA. Framework is the pinned Arduino-ESP32 2.0.17 package with its
ESP-IDF 4.4 driver. This cached, pinned toolchain is for a non-networked prototype;
it is not a claim of current SDK security maintenance or production qualification.

## Behavior

UART0 is 115200 baud, 8N1. Boot text may precede newline-delimited JSON.
The boot record identifies the fixed configuration and reset reason; `ready`
means only sensor initialization succeeded. Approximate 1 Hz `sample` records
contain sequence, uptime, raw words, converted Celsius/%RH and CRC/status flags.

No Wi-Fi or Bluetooth initialization occurs; startup checks both are uninitialized.
The IDF I2C driver is used directly to keep internal pull-ups disabled. The
framework's brownout detector remains enabled; it is not a 3.2 V rail-limit monitor.
There is no runtime supply, current, capacitance or ambient-environment measurement.

Sensor startup: 100 ms delay, break periodic mode, wait 2 ms, soft reset, wait
2 ms, heater disable, wait 1 ms, clear startup status, wait 1 ms, CRC-check status.
Measurements use 0x2400, a 20 ms wait, six-byte read with both CRCs, then CRC-checked
status. Heater/reset/command/checksum status faults prevent publication of a reading.
Every I2C operation has a 100 ms timeout. Any fault latches; no samples, automatic
retries, watchdog resets for recovery, calibration tricks or fabricated values follow.
Remove power and diagnose before restarting. Firmware does not switch off external
power, and a heater-status failure must be followed by operator power removal.

CRC and conversion follow [Sensirion SHT3x-DIS v7, sections 4.3 and 4.8-4.13](https://sensirion.com/media/documents/213E6A3B/63A5A569/Datasheet_SHT3x_DIS.pdf).
Driver configuration follows [Espressif's IDF 4.4 I2C reference](https://docs.espressif.com/projects/esp-idf/en/v4.4.7/esp32/api-reference/peripherals/i2c.html).
Host tests cover command ordering, waits, both word CRCs, bit corruption,
conversion endpoints, each I/O failure point, status errors and fault latching.
They do not simulate ESP32 electrical timing or prove on-target execution.

## Flashing: only after the bench procedure's authorization gates

Use a 3.3 V logic USB-UART adapter with TX -> J2.3, RX -> J2.4, GND -> J2.2.
Leave J2.1 and all DTR/RTS/power outputs disconnected. No RS-232 or 5 V signals.
Connect with power off. The adapter must not inject voltage into an unpowered board;
use a verified tri-state/disconnected TX until the board is powered. The board gets
regulated power from J1 only. Follow [BENCH.md](../../docs/prototypes/sht31/BENCH.md).

Hold BOOT, hold RESET, apply/stabilize power, release RESET, then release BOOT.
From the delivered package's `firmware` directory, with the intended port explicitly
substituted and the pinned esptool 4.11.0 available:

```sh
python -m esptool --chip esp32 --port YOUR_PORT --baud 115200 \
  --before no_reset --after no_reset write_flash \
  --flash_mode dio --flash_freq 40m --flash_size 4MB \
  0x1000 bootloader.bin 0x8000 partitions.bin 0x10000 firmware.bin
```

This factory partition table does not have OTA data and needs no `boot_app0.bin`.
Verify flash with the same file/offset pairs using `verify_flash` before releasing
RESET without BOOT to run. Never execute upload/erase commands during software-only
verification. Retain the full programming log and package hashes.

Use a serial logger that does not assert DTR/RTS; save the entire raw session from
before reset, not only selected samples. Run:

```sh
node scripts/sht31-prototype/verify-run.mjs /path/to/uart-log.txt
```

This checks log consistency, duration, sample sequence and plausibility, not physical
authenticity, sensor accuracy or all bench gates. Keep raw data and trace captures.
Timestamp wrap (about 49 days), resets and missing samples are rejected; this tool
is intended for a short supervised bring-up session.
