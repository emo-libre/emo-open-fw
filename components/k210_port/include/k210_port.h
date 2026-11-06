/* ESP-IDF transport for the K210 link: UART1 + RX task (framing/ack from k210_link), and the bulk SPI. */
#ifndef K210_PORT_H
#define K210_PORT_H

#include <stddef.h>
#include <stdint.h>
#include "esp_err.h"
#include "k210_link.h"

#ifdef __cplusplus
extern "C" {
#endif

/* Install UART1 (1 Mbaud, GPIO12/4), build the HAL, start the RX task. Returns the link, or NULL. */
k210_link_t *k210_port_start(void);

/* Bulk SPI (HSPI, GPIO15/2/14, software CS GPIO13, 8 MHz, mode 1). One call moves up to
 * K210_SPI_BLOCK bytes; split larger transfers into blocks. */
esp_err_t k210_port_spi_init(void);
esp_err_t k210_port_spi_send(const uint8_t *data, size_t len, uint8_t mode);
esp_err_t k210_port_spi_recv(uint8_t *data, size_t len, uint8_t mode);

#ifdef __cplusplus
}
#endif
#endif
