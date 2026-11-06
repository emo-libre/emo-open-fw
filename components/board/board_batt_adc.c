/* Battery sense on ADC1 channel 3 (GPIO39), 12 bit, ~11 dB range class (ADC_ATTEN_DB_12 in IDF >= 5.2). */
#include "esp_adc/adc_cali.h"
#include "esp_adc/adc_cali_scheme.h"
#include "esp_adc/adc_oneshot.h"
#include "esp_log.h"
#include "board.h"

static const char *TAG = "board_batt";
static adc_oneshot_unit_handle_t s_adc;
static adc_cali_handle_t s_cali;
static bool s_cali_ok;

esp_err_t board_batt_init(void)
{
    if (s_adc) {
        return ESP_OK;
    }
    adc_oneshot_unit_init_cfg_t unit = { .unit_id = ADC_UNIT_1 };
    esp_err_t e = adc_oneshot_new_unit(&unit, &s_adc);
    if (e != ESP_OK) {
        s_adc = NULL;
        return e;
    }
    adc_oneshot_chan_cfg_t ch = { .atten = ADC_ATTEN_DB_12, .bitwidth = ADC_BITWIDTH_12 };
    e = adc_oneshot_config_channel(s_adc, ADC_CHANNEL_3, &ch);
    if (e != ESP_OK) {
        return e;
    }
    adc_cali_line_fitting_config_t cc = { .unit_id = ADC_UNIT_1, .atten = ADC_ATTEN_DB_12,
                                          .bitwidth = ADC_BITWIDTH_12 };
    s_cali_ok = (adc_cali_create_scheme_line_fitting(&cc, &s_cali) == ESP_OK);
    ESP_LOGI(TAG, "ADC ready, calibration %s", s_cali_ok ? "from eFuse" : "unavailable (raw scaling)");
    return ESP_OK;
}

int board_batt_read_pin_mv(void)
{
    if (!s_adc) {
        return -1;
    }
    int sum = 0, ok = 0;
    for (int i = 0; i < 16; i++) {
        int raw;
        if (adc_oneshot_read(s_adc, ADC_CHANNEL_3, &raw) == ESP_OK) {
            sum += raw;
            ok++;
        }
    }
    if (!ok) {
        return -1;
    }
    int raw = sum / ok;
    if (s_cali_ok) {
        int mv;
        if (adc_cali_raw_to_voltage(s_cali, raw, &mv) == ESP_OK) {
            return mv;
        }
    }
    return raw * 3100 / 4095;       /* rough full-scale guess when uncalibrated */
}
