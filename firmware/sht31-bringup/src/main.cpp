#include <Arduino.h>
#include <driver/i2c.h>
#include <esp_bt.h>
#include <esp_system.h>
#include <esp_wifi.h>
#include "sht31.hpp"

namespace {
constexpr i2c_port_t port = I2C_NUM_0;
bool fault = false;
uint32_t samples = 0;
class NativeBus : public sht31::Bus {
public:
    esp_err_t last = ESP_OK;
    bool write(const uint8_t* p, size_t n) override {
        last = i2c_master_write_to_device(port, sht31::address, p, n, pdMS_TO_TICKS(100));
        return last == ESP_OK;
    }
    bool read(uint8_t* p, size_t n) override {
        last = i2c_master_read_from_device(port, sht31::address, p, n, pdMS_TO_TICKS(100));
        return last == ESP_OK;
    }
    void wait_ms(unsigned ms) override {
        // One extra RTOS tick prevents tick-phase rounding from shortening a wait.
        delay(ms + portTICK_PERIOD_MS);
    }
} bus;
sht31::Sensor sensor(bus);
void stop(const char* stage, const char* reason) {
    fault = true;
    Serial.printf("{\"type\":\"fault\",\"stage\":\"%s\",\"reason\":\"%s\",\"i2c_error\":%d,\"ms\":%lu}\n",
                  stage, reason, int(bus.last), (unsigned long)millis());
    // No restart/retry: retain the first fault for bench diagnosis.
}
}
void setup() {
    Serial.begin(115200, SERIAL_8N1, 3, 1); // J2.3 RX, J2.4 TX; no USB required.
    delay(100);
    Serial.printf("{\"type\":\"boot\",\"firmware\":\"sht31-prototype-1\",\"family\":\"esp32_sht31_v1\",\"profile\":\"standard\",\"sda\":21,\"scl\":22,\"hz\":100000,\"address\":68,\"reset_reason\":%d}\n", int(esp_reset_reason()));
    // This application never initializes either radio. Fail if that changes.
    wifi_mode_t mode;
    if (esp_wifi_get_mode(&mode) != ESP_ERR_WIFI_NOT_INIT ||
        esp_bt_controller_get_status() != ESP_BT_CONTROLLER_STATUS_IDLE) {
        stop("startup", "radio_initialized");
        return;
    }
    i2c_config_t cfg{};
    cfg.mode = I2C_MODE_MASTER;
    cfg.sda_io_num = GPIO_NUM_21;
    cfg.scl_io_num = GPIO_NUM_22;
    cfg.sda_pullup_en = GPIO_PULLUP_DISABLE;
    cfg.scl_pullup_en = GPIO_PULLUP_DISABLE;
    cfg.master.clk_speed = sht31::bus_hz;
    bus.last = i2c_param_config(port, &cfg);
    if (bus.last == ESP_OK) bus.last = i2c_driver_install(port, cfg.mode, 0, 0, 0);
    if (bus.last != ESP_OK) { stop("startup", "i2c_setup"); return; }
    // Defense against framework defaults: use external R3/R4 only.
    bus.last = gpio_set_pull_mode(GPIO_NUM_21, GPIO_FLOATING);
    if (bus.last == ESP_OK) bus.last = gpio_set_pull_mode(GPIO_NUM_22, GPIO_FLOATING);
    if (bus.last != ESP_OK) { stop("startup", "gpio_pull_setup"); return; }
    const auto result = sensor.begin();
    if (result != sht31::Error::ok) { stop("startup", sht31::name(result)); return; }
    Serial.println("{\"type\":\"ready\",\"heater\":false,\"radio_initialized\":false}");
}
void loop() {
    if (fault) { delay(1000); return; }
    sht31::Sample value{};
    const auto result = sensor.sample(value);
    if (result != sht31::Error::ok) { stop("measurement", sht31::name(result)); return; }
    Serial.printf("{\"type\":\"sample\",\"seq\":%lu,\"ms\":%lu,\"raw_t\":%u,\"raw_rh\":%u,\"celsius\":%.4f,\"rh_percent\":%.4f,\"crc_ok\":true,\"status_ok\":true}\n",
                  (unsigned long)++samples, (unsigned long)millis(), unsigned(value.raw_t), unsigned(value.raw_rh), value.celsius, value.humidity);
    delay(1000); // Approx. 1 Hz; no RF, alerts, OTA, bus scanner or extra GPIO loads.
}
