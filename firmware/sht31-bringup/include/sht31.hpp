#pragma once
#include <stddef.h>
#include <stdint.h>

// SHT3x-DIS v7 sections 4.3, 4.8-4.13. No retries or recovery that hides faults.
namespace sht31 {
constexpr uint8_t address = 0x44;
constexpr unsigned bus_hz = 100000;
enum class Error { ok, io, crc, status, not_ready };
struct Sample { uint16_t raw_t, raw_rh; double celsius, humidity; };
struct Bus {
    virtual ~Bus() = default;
    virtual bool write(const uint8_t* data, size_t size) = 0;
    virtual bool read(uint8_t* data, size_t size) = 0;
    virtual void wait_ms(unsigned ms) = 0;
};
inline uint8_t crc(const uint8_t* bytes, size_t n) {
    uint8_t result = 0xff;
    for (size_t i = 0; i < n; ++i) {
        result ^= bytes[i];
        for (unsigned bit = 0; bit < 8; ++bit)
            result = (result & 0x80) ? uint8_t((result << 1) ^ 0x31) : uint8_t(result << 1);
    }
    return result;
}
inline uint16_t word(const uint8_t* p) { return uint16_t((uint16_t(p[0]) << 8) | p[1]); }
inline Error decode(const uint8_t* p, size_t n, Sample& out) {
    if (n != 6) return Error::io;
    if (crc(p, 2) != p[2] || crc(p + 3, 2) != p[5]) return Error::crc;
    const auto t = word(p), rh = word(p + 3);
    out = {t, rh, -45.0 + 175.0 * t / 65535.0, 100.0 * rh / 65535.0};
    return Error::ok;
}
inline const char* name(Error e) {
    switch (e) {
    case Error::ok: return "ok";
    case Error::io: return "i2c_io";
    case Error::crc: return "crc";
    case Error::status: return "sensor_status";
    default: return "not_ready";
    }
}
class Sensor {
    Bus& bus;
    bool ready = false;
    bool command(uint16_t cmd) {
        const uint8_t bytes[] = {uint8_t(cmd >> 8), uint8_t(cmd)};
        return bus.write(bytes, sizeof bytes);
    }
    Error status() {
        uint8_t bytes[3] = {};
        if (!command(0xf32d) || !bus.read(bytes, sizeof bytes)) return Error::io;
        if (crc(bytes, 2) != bytes[2]) return Error::crc;
        // Heater, unexpected reset, command error, write checksum error.
        if (word(bytes) & 0x2013) return Error::status;
        return Error::ok;
    }
public:
    explicit Sensor(Bus& b) : bus(b) {}
    Error begin() {
        ready = false;
        bus.wait_ms(100); // Power-up margin; external rail stability still required.
        if (!command(0x3093)) return Error::io; // End a previous firmware's periodic mode.
        bus.wait_ms(2);
        if (!command(0x30a2)) return Error::io;
        bus.wait_ms(2); // Soft reset maximum is 1.5 ms in the selected voltage range.
        if (!command(0x3066)) return Error::io; // Explicitly disable heater.
        bus.wait_ms(1);
        if (!command(0x3041)) return Error::io; // Clear startup reset flag only here.
        bus.wait_ms(1);
        const Error result = status();
        ready = result == Error::ok;
        return result;
    }
    Error sample(Sample& out) {
        if (!ready) return Error::not_ready;
        ready = false; // Any failure latches until an explicit reinitialization.
        if (!command(0x2400)) return Error::io; // High repeatability, no clock stretch.
        bus.wait_ms(20); // Selected supply range: maximum conversion time 15 ms.
        uint8_t bytes[6] = {};
        if (!bus.read(bytes, sizeof bytes)) return Error::io;
        Sample candidate{};
        Error result = decode(bytes, sizeof bytes, candidate);
        if (result != Error::ok) return result;
        result = status();
        if (result != Error::ok) return result;
        out = candidate; // Never publish stale, CRC-invalid or heater-on values.
        ready = true;
        return Error::ok;
    }
};
} // namespace sht31
