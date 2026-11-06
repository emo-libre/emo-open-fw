/* Bulk SPI transport. Per spec-clean/k210_link.md §4: 8-byte header, >=500 us, then the body padded
 * to a multiple of 64. CS is asserted per transaction (the K210 latches on it). NOT yet compiled or
 * run on hardware. */
#include <string.h>
#include "driver/gpio.h"
#include "driver/spi_master.h"
#include "esp_heap_caps.h"
#include "esp_log.h"
#include "esp_rom_sys.h"
#include "freertos/FreeRTOS.h"
#include "freertos/semphr.h"
#include "k210_port.h"
#include "k210_spi.h"
#include "pins.h"

static const char *TAG = "k210_spi";
static spi_device_handle_t s_dev;
static SemaphoreHandle_t s_mutex;
static uint8_t *s_hdr;      /* DMA-capable, 8 bytes */
static uint8_t *s_buf;      /* DMA-capable, one block */

static esp_err_t xfer(const uint8_t *tx, uint8_t *rx, size_t bytes)
{
    spi_transaction_t t = { .length = bytes * 8u, .tx_buffer = tx, .rx_buffer = rx };
    gpio_set_level(PIN_K210_SPI_CS, 0);
    esp_err_t e = spi_device_transmit(s_dev, &t);
    gpio_set_level(PIN_K210_SPI_CS, 1);
    return e;
}

esp_err_t k210_port_spi_init(void)
{
    if (s_dev) {
        return ESP_OK;
    }
    gpio_reset_pin(PIN_K210_SPI_CS);
    gpio_set_level(PIN_K210_SPI_CS, 1);
    gpio_set_direction(PIN_K210_SPI_CS, GPIO_MODE_OUTPUT);

    const spi_bus_config_t bus = {
        .mosi_io_num = PIN_K210_SPI_MOSI, .miso_io_num = PIN_K210_SPI_MISO, .sclk_io_num = PIN_K210_SPI_SCLK,
        .quadwp_io_num = -1, .quadhd_io_num = -1, .max_transfer_sz = K210_SPI_BLOCK + K210_SPI_PAD,
    };
    esp_err_t e = spi_bus_initialize(SPI2_HOST, &bus, SPI_DMA_CH_AUTO);
    if (e != ESP_OK) {
        return e;
    }
    const spi_device_interface_config_t dev = {
        .mode = 1, .clock_speed_hz = K210_SPI_HZ, .spics_io_num = -1, .queue_size = 7,
    };
    e = spi_bus_add_device(SPI2_HOST, &dev, &s_dev);
    if (e != ESP_OK) {
        return e;
    }
    s_hdr = heap_caps_malloc(K210_SPI_HEADER_LEN, MALLOC_CAP_DMA);
    s_buf = heap_caps_malloc(K210_SPI_BLOCK + K210_SPI_PAD, MALLOC_CAP_DMA);
    s_mutex = xSemaphoreCreateMutex();
    if (!s_hdr || !s_buf || !s_mutex) {
        ESP_LOGE(TAG, "out of DMA memory");
        return ESP_ERR_NO_MEM;
    }
    return ESP_OK;
}

static esp_err_t transfer(uint8_t *data, size_t len, uint8_t mode, bool is_send)
{
    if (!s_dev || !data || len == 0 || len > K210_SPI_BLOCK) {
        return ESP_ERR_INVALID_ARG;
    }
    size_t padded = k210_spi_padded_len(len);
    xSemaphoreTake(s_mutex, portMAX_DELAY);

    k210_spi_build_header(s_hdr, 0, 0, len, mode);
    esp_err_t e = xfer(s_hdr, NULL, K210_SPI_HEADER_LEN);
    if (e == ESP_OK) {
        esp_rom_delay_us(500);
        if (is_send) {
            memcpy(s_buf, data, len);
            memset(s_buf + len, 0, padded - len);
            e = xfer(s_buf, NULL, padded);
        } else {
            memset(s_buf, 0, padded);
            e = xfer(s_buf, s_buf, padded);
            if (e == ESP_OK) {
                memcpy(data, s_buf, len);
            }
        }
    }
    xSemaphoreGive(s_mutex);
    return e;
}

esp_err_t k210_port_spi_send(const uint8_t *data, size_t len, uint8_t mode)
{
    return transfer((uint8_t *)data, len, mode, true);
}

esp_err_t k210_port_spi_recv(uint8_t *data, size_t len, uint8_t mode)
{
    return transfer(data, len, mode, false);
}
