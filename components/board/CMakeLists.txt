idf_component_register(
    SRCS
        board_power_hold.c
        board_k210_reset.c
        board_i2c_bus.c
        board_batt_adc.c
        board_charger_in.c
    INCLUDE_DIRS include
    REQUIRES driver esp_adc esp_rom log
)
