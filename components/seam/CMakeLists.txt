idf_component_register(
    SRCS
        emo_play_animation.c
        emo_provider_table.c
        emo_renderer_k210.c
    INCLUDE_DIRS include
    REQUIRES k210_link
)
