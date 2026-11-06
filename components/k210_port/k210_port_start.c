/* UART transport. The protocol logic lives in k210_link (host-tested); this file only supplies
 * the HAL (UART write, FreeRTOS lock/ack queue/sleep) and the RX task. NOT yet compiled or run
 * on hardware — see RESUME.md. */
#include "driver/uart.h"
#include "esp_log.h"
#include "esp_timer.h"
#include "freertos/FreeRTOS.h"
#include "freertos/queue.h"
#include "freertos/semphr.h"
#include "freertos/task.h"
#include "k210_port.h"
#include "pins.h"

#define K210_UART_NUM    UART_NUM_1
#define RX_DRIVER_BYTES  2048
#define RX_PAYLOAD_MAX   1100          /* largest K210->ESP payload we keep; bigger ones are skipped (DV-001) */
#define RX_IDLE_RESET_US 100000        /* DV-007: abandon a half-received frame after 100 ms of silence */

static const char *TAG = "k210_port";

static k210_link_t s_link;
static uint8_t s_rxbuf[RX_PAYLOAD_MAX];
static SemaphoreHandle_t s_lock;
static QueueHandle_t s_ackq;

static int hal_write(void *ctx, const uint8_t *buf, size_t n)
{
    (void)ctx;
    return uart_write_bytes(K210_UART_NUM, (const char *)buf, n);
}

static void hal_sleep_ms(void *ctx, uint32_t ms)
{
    (void)ctx;
    TickType_t t = pdMS_TO_TICKS(ms);
    vTaskDelay(t ? t : 1);
}

static void hal_lock(void *ctx)   { (void)ctx; xSemaphoreTake(s_lock, portMAX_DELAY); }
static void hal_unlock(void *ctx) { (void)ctx; xSemaphoreGive(s_lock); }
static void hal_ack_flush(void *ctx) { (void)ctx; xQueueReset(s_ackq); }

static void hal_ack_signal(void *ctx, uint8_t ack)
{
    (void)ctx;
    xQueueOverwrite(s_ackq, &ack);
}

static int hal_ack_wait(void *ctx, uint32_t timeout_ms)
{
    (void)ctx;
    uint8_t a;
    return xQueueReceive(s_ackq, &a, pdMS_TO_TICKS(timeout_ms)) == pdTRUE ? (int)a : -1;
}

static void rx_task(void *arg)
{
    (void)arg;
    uint8_t buf[128];
    int64_t last_byte_us = esp_timer_get_time();

    for (;;) {
        int n = uart_read_bytes(K210_UART_NUM, buf, sizeof buf, pdMS_TO_TICKS(20));
        int64_t now = esp_timer_get_time();
        if (n > 0) {
            k210_link_feed(&s_link, buf, (size_t)n);
            last_byte_us = now;
        } else if (k210_rx_busy(&s_link.rx) && now - last_byte_us > RX_IDLE_RESET_US) {
            ESP_LOGW(TAG, "RX idle inside a frame: resetting parser");
            k210_rx_reset(&s_link.rx);
        }
    }
}

k210_link_t *k210_port_start(void)
{
    if (s_lock) {
        return &s_link;                         /* already started */
    }

    const uart_config_t cfg = {
        .baud_rate = K210_UART_BAUD,
        .data_bits = UART_DATA_8_BITS,
        .parity = UART_PARITY_DISABLE,
        .stop_bits = UART_STOP_BITS_1,
        .flow_ctrl = UART_HW_FLOWCTRL_DISABLE,
        .source_clk = UART_SCLK_DEFAULT,
    };
    if (uart_driver_install(K210_UART_NUM, RX_DRIVER_BYTES, 0, 0, NULL, 0) != ESP_OK ||
        uart_param_config(K210_UART_NUM, &cfg) != ESP_OK ||
        uart_set_pin(K210_UART_NUM, PIN_K210_UART_TX, PIN_K210_UART_RX, UART_PIN_NO_CHANGE, UART_PIN_NO_CHANGE) != ESP_OK) {
        ESP_LOGE(TAG, "UART setup failed");
        return NULL;
    }

    s_lock = xSemaphoreCreateMutex();
    s_ackq = xQueueCreate(1, sizeof(uint8_t));
    if (!s_lock || !s_ackq) {
        return NULL;
    }

    const k210_hal_t hal = {
        .write = hal_write, .sleep_ms = hal_sleep_ms, .lock = hal_lock, .unlock = hal_unlock,
        .ack_flush = hal_ack_flush, .ack_signal = hal_ack_signal, .ack_wait = hal_ack_wait, .ctx = NULL,
    };
    if (k210_link_init(&s_link, &hal, NULL, s_rxbuf, sizeof s_rxbuf) != K210_OK) {
        return NULL;
    }
    if (xTaskCreate(rx_task, "k210_rx", 3072, NULL, 8, NULL) != pdPASS) {
        return NULL;
    }
    ESP_LOGI(TAG, "K210 link up: UART%d %d baud TX=%d RX=%d", (int)K210_UART_NUM, K210_UART_BAUD,
             PIN_K210_UART_TX, PIN_K210_UART_RX);
    return &s_link;
}
