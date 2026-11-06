#ifndef EMO_BOARD_H
#define EMO_BOARD_H

#include <stdbool.h>
#include <stdint.h>
#include "driver/i2c_master.h"
#include "esp_err.h"
#include "pins.h"

#ifdef __cplusplus
extern "C" {
#endif

/* Latch the power-hold output high. Call first thing in app_main. */
void board_power_hold(void);

/* One ~1 ms low pulse on the K210 reset line, then idle high. */
void board_k210_hard_reset(void);

esp_err_t board_i2c_init(void);
i2c_master_bus_handle_t board_i2c_bus(void);
bool board_i2c_probe(uint8_t addr);
/* Probe the known device addresses and log what answers. Returns how many answered. */
int board_i2c_scan_known(void);

esp_err_t board_batt_init(void);
/* Averaged voltage at the ADC pin in mV (calibrated if eFuse data allows). Negative on error.
 * NOT yet scaled to battery volts: the divider ratio is unverified. */
int board_batt_read_pin_mv(void);

void board_charger_init(void);
bool board_charger_present(void);

#ifdef __cplusplus
}
#endif
#endif
