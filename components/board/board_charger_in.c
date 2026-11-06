#include "driver/gpio.h"
#include "board.h"

void board_charger_init(void)
{
    gpio_reset_pin(PIN_CHARGER_PRESENT);
    gpio_set_direction(PIN_CHARGER_PRESENT, GPIO_MODE_INPUT);   /* GPIO36 is input-only, no pulls */
}

bool board_charger_present(void)
{
    return gpio_get_level(PIN_CHARGER_PRESENT) != 0;
}
