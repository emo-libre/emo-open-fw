# Portable protocol core: no ESP-IDF headers allowed in this component (host tests compile it with gcc).
idf_component_register(
    SRCS
        k210_crc8_maxim.c
        k210_sum8.c
        k210_build_short.c
        k210_build_payload.c
        k210_rx_fsm.c
        k210_spi_hdr.c
        k210_media_path_ok.c
        k210_media_playlist.c
        k210_write_all.c
        k210_link_init.c
        k210_link_feed.c
        k210_send_short.c
        k210_send_payload.c
        k210_cmd_reset.c
        k210_cmd_version.c
        k210_cmd_volume.c
        k210_cmd_eye_mode.c
        k210_play_single.c
        k210_play_playlist.c
    INCLUDE_DIRS include
)
