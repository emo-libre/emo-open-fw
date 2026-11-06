#include "driver/gpio.h"
#include "esp_rom_sys.h"
#include "board.h"

void board_k210_hard_reset(void)
{
    gpio_reset_pin(PIN_K210_RESET);
    gpio_set_level(PIN_K210_RESET, 1);
    gpio_set_direction(PIN_K210_RESET, GPIO_MODE_OUTPUT);
    gpio_set_level(PIN_K210_RESET, 0);
    esp_rom_delay_us(1000);
    gpio_set_level(PIN_K210_RESET, 1);
}
