idf_component_register(
    SRCS
        k210_port_start.c
        k210_port_spi.c
    INCLUDE_DIRS include
    REQUIRES k210_link board driver esp_timer esp_rom log
)
