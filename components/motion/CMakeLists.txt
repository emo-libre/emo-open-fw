# Portable motion core: no ESP-IDF headers allowed here (host tests compile it with gcc).
idf_component_register(
    SRCS
        osc_angle_to_us.c
        osc_begin.c
        osc_step.c
        osc_run.c
    INCLUDE_DIRS include
)
