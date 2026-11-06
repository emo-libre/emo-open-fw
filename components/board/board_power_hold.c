#include "driver/gpio.h"
#include "board.h"

void board_power_hold(void)
{
    gpio_reset_pin(PIN_POWER_HOLD);
    gpio_set_level(PIN_POWER_HOLD, 1);          /* set the level first so the pin never glitches low */
    gpio_set_direction(PIN_POWER_HOLD, GPIO_MODE_OUTPUT);
}
