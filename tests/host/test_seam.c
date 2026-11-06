#include "test_util.h"
#include "sim_k210.h"
#include "emo_seam.h"

static uint8_t g_rx[256];

/* synthetic content only */
static const k210_sound_t S_A[] = { { 0, "/test/demo_a.mp3" } };
static const k210_sound_t S_B[] = { { 0, "/test/demo_b1.mp3" }, { 700, "/test/demo_b2.mp3" } };
static const emo_clip_t C_A[] = { { "/test/demo_a.avi", S_A, 1 } };
static const emo_clip_t C_B[] = { { "/test/demo_b1.avi", S_B, 2 }, { "/test/demo_b2.avi", NULL, 0 } };
static const char *const M_A[] = { "demo_move_1", "demo_move_2" };
static const emo_clip_t C_BAD[] = { { "relative.avi", NULL, 0 } };

static const emo_anim_t TABLE[] = {
    { "demo_a", C_A, 1, M_A, 2 },
    { "demo_b", C_B, 2, NULL, 0 },
    { "demo_motion_only", NULL, 0, M_A, 1 },
    { "demo_bad", C_BAD, 1, NULL, 0 },
};

int main(void)
{
    static sim_t s;
    static k210_link_t l;
    sim_init(&s, &l, g_rx, sizeof g_rx);

    emo_asset_provider_t prov;
    emo_table_provider_t pstate;
    emo_table_provider_init(&prov, &pstate, TABLE, sizeof TABLE / sizeof TABLE[0]);
    emo_renderer_t rend;
    static emo_k210_renderer_t rstate;
    emo_k210_renderer_init(&rend, &rstate, &l);

    emo_anim_t a;

    /* ---- single clip: full animation goes out as a sub-2 JSON playlist ---- */
    CHECK_EQ(emo_play_animation(&prov, &rend, "demo_a", 0, &a), EMO_OK);
    CHECK_EQ(s.npays, 1);
    CHECK_EQ(s.pays[0].sub, 2);
    CHECK(strcmp((const char *)s.pays[0].data,
                 "[0,[[\"/test/demo_a.avi\",[[0,\"/test/demo_a.mp3\"]]]]]") == 0);
    CHECK_EQ(a.nmot, 2);                                   /* body motion handed back to the caller */
    CHECK(strcmp(a.mot[1], "demo_move_2") == 0);

    /* ---- loop flag, two clips, timed sounds ---- */
    CHECK_EQ(emo_play_animation(&prov, &rend, "demo_b", 1, &a), EMO_OK);
    CHECK_EQ(s.npays, 2);
    CHECK(strcmp((const char *)s.pays[1].data,
                 "[1,[[\"/test/demo_b1.avi\",[[0,\"/test/demo_b1.mp3\"],[700,\"/test/demo_b2.mp3\"]]],"
                 "[\"/test/demo_b2.avi\",[]]]]") == 0);

    /* ---- motion-only animation: renderer not called ---- */
    CHECK_EQ(emo_play_animation(&prov, &rend, "demo_motion_only", 0, &a), EMO_OK);
    CHECK_EQ(s.npays, 2);
    CHECK_EQ(a.nmot, 1);

    /* ---- unknown name ---- */
    CHECK_EQ(emo_play_animation(&prov, &rend, "nope", 0, &a), EMO_ERR_NOT_FOUND);
    CHECK(a.name == NULL);
    CHECK_EQ(s.npays, 2);

    /* ---- unsafe path in the provider's data: refused, nothing sent ---- */
    CHECK_EQ(emo_play_animation(&prov, &rend, "demo_bad", 0, &a), EMO_ERR_MEDIA);
    CHECK_EQ(s.npays, 2);

    /* ---- K210 unreachable: render error is reported ---- */
    s.fail_write = 1;
    CHECK_EQ(emo_play_animation(&prov, &rend, "demo_a", 0, &a), EMO_ERR_RENDER);
    s.fail_write = 0;

    /* ---- argument checks ---- */
    CHECK_EQ(emo_play_animation(NULL, &rend, "demo_a", 0, &a), EMO_ERR_ARG);
    CHECK_EQ(emo_play_animation(&prov, NULL, "demo_a", 0, &a), EMO_ERR_ARG);
    CHECK_EQ(emo_play_animation(&prov, &rend, "", 0, &a), EMO_ERR_ARG);
    CHECK_EQ(emo_play_animation(&prov, &rend, "demo_a", 0, NULL), EMO_ERR_ARG);

    /* ---- an animation too large for the K210's safe playlist size is refused, not truncated ---- */
    static emo_clip_t big[40];
    for (int i = 0; i < 40; i++) {
        big[i].avi = "/test/a_deliberately_long_path_for_the_size_limit_test_0123456789.avi";
    }
    static const emo_anim_t HUGE_TABLE[] = { { "huge", big, 40, NULL, 0 } };
    emo_asset_provider_t p2;
    emo_table_provider_t st2;
    emo_table_provider_init(&p2, &st2, HUGE_TABLE, 1);
    int before = s.npays;
    CHECK_EQ(emo_play_animation(&p2, &rend, "huge", 0, &a), EMO_ERR_MEDIA);
    CHECK_EQ(s.npays, before);

    TEST_DONE();
}
