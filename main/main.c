/* Phase 1 bring-up: power hold, board peripherals, K210 link, version handshake, tier 0-1 commands.
 * NOT yet compiled or run on hardware — see RESUME.md. */
#include <string.h>
#include "esp_log.h"
#include "freertos/FreeRTOS.h"
#include "freertos/task.h"
#include "board.h"
#include "k210_link.h"
#include "k210_port.h"
#include "sdkconfig.h"

static const char *TAG = "emo";

static void log_err(const char *what, k210_err_t e)
{
    if (e == K210_OK) {
        ESP_LOGI(TAG, "%s: ok", what);
    } else {
        ESP_LOGW(TAG, "%s: error %d", what, (int)e);
    }
}

void app_main(void)
{
    board_power_hold();                                   /* first: keep the board powered */
    ESP_LOGI(TAG, "emo-open-fw phase 1 bring-up");

    if (board_i2c_init() == ESP_OK) {
        ESP_LOGI(TAG, "I2C devices answering: %d", board_i2c_scan_known());
    }
    if (board_batt_init() == ESP_OK) {
        ESP_LOGI(TAG, "battery pin: %d mV (divider unverified)", board_batt_read_pin_mv());
    }
    board_charger_init();
    ESP_LOGI(TAG, "charger present: %s", board_charger_present() ? "yes" : "no");

    k210_link_t *k = k210_port_start();
    if (!k) {
        ESP_LOGE(TAG, "K210 link failed to start");
        return;
    }

    /* Stock order: soft reset, 500 ms, then the hard-reset pulse. */
    log_err("K210 soft reset (cmd 52)", k210_cmd_reset(k));
    vTaskDelay(pdMS_TO_TICKS(500));
#if CONFIG_EMO_K210_HARD_RESET
    board_k210_hard_reset();
    ESP_LOGI(TAG, "K210 reset line pulsed");
#endif

    uint32_t word = 0;
    k210_err_t e = k210_wait_ready(k, CONFIG_EMO_K210_READY_TIMEOUT_MS, &word);
    if (e != K210_OK) {
        ESP_LOGE(TAG, "K210 did not answer the version query (err %d). Check wiring/power; link stats tx=%u nack=%u timeout=%u",
                 (int)e, (unsigned)k->stat_tx_short, (unsigned)k->stat_tx_nack, (unsigned)k->stat_tx_timeout);
        return;
    }
    ESP_LOGI(TAG, "K210 TALKING: firmware number word=0x%06x", (unsigned)word);

    char text[16] = "";
    if (k210_query_version_text(k, text, sizeof text, 1000) == K210_OK) {
        ESP_LOGI(TAG, "K210 version text: \"%s\"", text);
    } else {
        ESP_LOGW(TAG, "K210 gave no version text");
    }

    log_err("volume", k210_set_volume(k, CONFIG_EMO_DEMO_VOLUME));
    log_err("eye mode default", k210_set_eye_mode(k, K210_EYE_DEFAULT));

    if (strlen(CONFIG_EMO_DEMO_AVI)) {
        log_err("play avi " CONFIG_EMO_DEMO_AVI, k210_play_avi(k, CONFIG_EMO_DEMO_AVI, false));
    }
    if (strlen(CONFIG_EMO_DEMO_MP3)) {
        log_err("play mp3 " CONFIG_EMO_DEMO_MP3, k210_play_mp3(k, CONFIG_EMO_DEMO_MP3, false));
    }
    ESP_LOGI(TAG, "link stats: short=%u payload=%u retries=%u nack=%u timeout=%u rx_frames=%u rx_sumerr=%u",
             (unsigned)k->stat_tx_short, (unsigned)k->stat_tx_payload, (unsigned)k->stat_tx_retries,
             (unsigned)k->stat_tx_nack, (unsigned)k->stat_tx_timeout, (unsigned)k->rx.stat_frames,
             (unsigned)k->rx.stat_sum_err);
}
