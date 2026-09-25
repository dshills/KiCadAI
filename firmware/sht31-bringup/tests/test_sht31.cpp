#include "sht31.hpp"
#include <cassert>
#include <cmath>
#include <iostream>
#include <vector>
struct Fake : sht31::Bus {
    std::vector<unsigned> commands, waits;
    std::vector<std::vector<uint8_t>> reads;
    int calls = 0, fail_at = -1;
    bool write(const uint8_t* p, size_t n) override {
        assert(n == 2); commands.push_back(sht31::word(p));
        return calls++ != fail_at;
    }
    bool read(uint8_t* p, size_t n) override {
        if (calls++ == fail_at) return false;
        assert(!reads.empty() && reads.front().size() == n);
        for (size_t i = 0; i < n; ++i) p[i] = reads.front()[i];
        reads.erase(reads.begin()); return true;
    }
    void wait_ms(unsigned ms) override { waits.push_back(ms); }
};
std::vector<uint8_t> status(uint16_t v) {
    std::vector<uint8_t> b{uint8_t(v >> 8), uint8_t(v), 0};
    b[2] = sht31::crc(b.data(), 2); return b;
}
int main() {
    const uint8_t example[] = {0xbe, 0xef};
    assert(sht31::crc(example, 2) == 0x92); // Independent manufacturer vector.
    const std::vector<uint8_t> data{0xbe, 0xef, 0x92, 0xbe, 0xef, 0x92};
    sht31::Sample s{};
    assert(sht31::decode(data.data(), 6, s) == sht31::Error::ok);
    assert(std::abs(s.celsius - (-45 + 175.0 * 48879 / 65535)) < 1e-10);
    assert(std::abs(s.humidity - (100.0 * 48879 / 65535)) < 1e-10);
    for (unsigned i = 0; i < 48; ++i) {
        auto damaged = data; damaged[i / 8] ^= uint8_t(1 << (i % 8));
        assert(sht31::decode(damaged.data(), 6, s) == sht31::Error::crc);
    }
    assert(sht31::decode(data.data(), 5, s) == sht31::Error::io);
    for (uint16_t raw : {uint16_t(0), uint16_t(65535)}) {
        auto b = status(raw); const auto second = b;
        b.insert(b.end(), second.begin(), second.end());
        assert(sht31::decode(b.data(), b.size(), s) == sht31::Error::ok);
        assert(s.celsius == (raw ? 130 : -45)); assert(s.humidity == (raw ? 100 : 0));
    }
    Fake f; f.reads = {status(0), data, status(0)};
    sht31::Sensor sensor(f);
    assert(sensor.sample(s) == sht31::Error::not_ready);
    assert(sensor.begin() == sht31::Error::ok);
    assert(sensor.sample(s) == sht31::Error::ok);
    assert((f.commands == std::vector<unsigned>{0x3093,0x30a2,0x3066,0x3041,0xf32d,0x2400,0xf32d}));
    assert((f.waits == std::vector<unsigned>{100,2,2,1,1,20}));
    // Every initialization/measurement I/O point is bounded and fails closed.
    for (int i = 0; i < 10; ++i) {
        Fake b; b.fail_at = i; b.reads = {status(0), data, status(0)};
        sht31::Sensor d(b); auto r = d.begin();
        if (r == sht31::Error::ok) r = d.sample(s);
        assert(r == sht31::Error::io);
        int calls = b.calls;
        assert(d.sample(s) == sht31::Error::not_ready); assert(b.calls == calls);
    }
    for (uint16_t flag : {uint16_t(0x2000),uint16_t(0x10),uint16_t(2),uint16_t(1)}) {
        Fake b; b.reads = {status(flag)}; sht31::Sensor d(b);
        assert(d.begin() == sht31::Error::status);
        Fake c; c.reads = {status(0), data, status(flag)}; sht31::Sensor e(c);
        assert(e.begin() == sht31::Error::ok);
        s.raw_t = 42; assert(e.sample(s) == sht31::Error::status); assert(s.raw_t == 42);
        assert(e.sample(s) == sht31::Error::not_ready);
    }
    for (int where = 0; where < 3; ++where) {
        Fake b; b.reads = {status(0), data, status(0)}; b.reads[where][2] ^= 1;
        sht31::Sensor d(b); auto r = d.begin();
        if (r == sht31::Error::ok) r = d.sample(s);
        assert(r == sht31::Error::crc);
        assert(d.sample(s) == sht31::Error::not_ready);
    }
    std::cout << "PASS: CRC vector, 48 bit faults, conversions, command order, waits, 10 I/O failures, status and CRC latching\n";
}
