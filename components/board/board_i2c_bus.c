/* I2C master bus (new IDF >= 5.2 driver). Pull-up arrangement on the board is unverified [V]:
 * internal pull-ups are enabled; they are harmless if external ones exist. */
#include "esp_log.h"
#include "board.h"

static const char *TAG = "board_i2c";
static i2c_master_bus_handle_t s_bus;

esp_err_t board_i2c_init(void)
{
    if (s_bus) {
        return ESP_OK;
    }
    i2c_master_bus_config_t cfg = {
        .i2c_port = I2C_NUM_0,
        .sda_io_num = PIN_I2C_SDA,
        .scl_io_num = PIN_I2C_SCL,
        .clk_source = I2C_CLK_SRC_DEFAULT,
        .glitch_ignore_cnt = 7,
        .flags.enable_internal_pullup = true,
    };
    esp_err_t e = i2c_new_master_bus(&cfg, &s_bus);
    if (e != ESP_OK) {
        ESP_LOGE(TAG, "i2c_new_master_bus: %s", esp_err_to_name(e));
        s_bus = NULL;
    }
    return e;
}

i2c_master_bus_handle_t board_i2c_bus(void)
{
    return s_bus;
}

bool board_i2c_probe(uint8_t addr)
{
    return s_bus && i2c_master_probe(s_bus, addr, 20) == ESP_OK;
}

int board_i2c_scan_known(void)
{
    static const struct { uint8_t addr; const char *what; } k[] = {
        { I2C_ADDR_PRESENCE, "presence chip" }, { I2C_ADDR_ALS_PS, "light/proximity" },
        { I2C_ADDR_TOF, "time-of-flight" },     { I2C_ADDR_MAG, "magnetometer" },
        { I2C_ADDR_IMU_A, "IMU (0x68)" },       { I2C_ADDR_IMU_B, "IMU (0x6A)" },
        { I2C_ADDR_IMU_C, "IMU (0x6B)" },
    };
    int found = 0;
    for (unsigned i = 0; i < sizeof k / sizeof k[0]; i++) {
        if (board_i2c_probe(k[i].addr)) {
            ESP_LOGI(TAG, "0x%02x answers: %s", k[i].addr, k[i].what);
            found++;
        }
    }
    return found;
}
