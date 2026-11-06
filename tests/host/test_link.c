/* Link layer + tier 0-1 commands against the simulated K210. */
#include "test_util.h"
#include "sim_k210.h"
#include "k210_media.h"

static uint8_t g_rxbuf[512];

static void setup(sim_t *s, k210_link_t *l)
{
    sim_init(s, l, g_rxbuf, sizeof g_rxbuf);
}

int main(void)
{
    static sim_t s;
    static k210_link_t l;

    /* ---- short command on the wire ---- */
    setup(&s, &l);
    CHECK_EQ(k210_cmd_reset(&l), K210_OK);
    const uint8_t want_reset[] = { 0x55, 0xAA, 0x34, 0, 0, 0, 0x33 };
    CHECK_EQ(s.wire_len, 7);
    CHECK_MEM(s.wire, want_reset, 7);
    CHECK_EQ(s.now_ms, 10);                    /* inter-frame gap honoured */
    CHECK_EQ(s.nshorts, 1);
    CHECK_EQ(s.shorts[0].cmd, 52);

    /* ---- volume / eye mode ---- */
    setup(&s, &l);
    CHECK_EQ(k210_set_volume(&l, 30), K210_OK);
    const uint8_t want_vol[] = { 0x55, 0xAA, 0x17, 30, 0, 0, 0x34 };
    CHECK_MEM(s.wire, want_vol, 7);
    CHECK_EQ(k210_set_volume(&l, 61), K210_ERR_ARG);
    CHECK_EQ(s.writes, 1);                     /* rejected before touching the wire */
    CHECK_EQ(k210_set_eye_mode(&l, K210_EYE_IDLE), K210_OK);
    CHECK_EQ(s.shorts[1].cmd, 56);
    CHECK_EQ(s.shorts[1].b1, 4);
    CHECK_EQ(k210_set_eye_mode(&l, 0), K210_ERR_ARG);
    CHECK_EQ(k210_set_eye_mode(&l, 7), K210_ERR_ARG);

    /* ---- acked payload ---- */
    setup(&s, &l);
    const uint8_t body[] = "hello";
    CHECK_EQ(k210_send_payload(&l, 40, body, sizeof body, true), K210_OK);
    CHECK_EQ(s.npays, 1);
    CHECK_EQ(s.pays[0].sub, 40);
    CHECK_EQ(s.pays[0].len, sizeof body);
    CHECK(s.pays[0].crc_ok);
    CHECK_EQ(l.stat_tx_retries, 0);
    uint8_t exp_frame[64];
    size_t en = k210_build_payload(exp_frame, sizeof exp_frame, 40, body, sizeof body);
    CHECK_EQ(s.wire_len, en);
    CHECK_MEM(s.wire, exp_frame, en);

    /* ---- NACK twice, then success (DV-003) ---- */
    setup(&s, &l);
    s.nack_first = 2;
    CHECK_EQ(k210_send_payload(&l, 40, body, sizeof body, true), K210_OK);
    CHECK_EQ(s.npays, 3);
    CHECK_EQ(l.stat_tx_retries, 2);
    CHECK_EQ(l.stat_tx_nack, 2);

    /* ---- permanent NACK: gives up after 1 + max_retries attempts ---- */
    setup(&s, &l);
    s.nack_first = 100;
    CHECK_EQ(k210_send_payload(&l, 40, body, sizeof body, true), K210_ERR_NACK);
    CHECK_EQ(s.npays, 3);

    /* ---- no reply: timeout, with virtual time showing 3 x 500 ms of waiting ---- */
    setup(&s, &l);
    s.silent = 1;
    CHECK_EQ(k210_send_payload(&l, 40, body, sizeof body, true), K210_ERR_TIMEOUT);
    CHECK_EQ(s.npays, 3);
    CHECK(s.now_ms >= 1500);
    CHECK_EQ(l.stat_tx_timeout, 3);

    /* ---- fire and forget: no wait, no retry ---- */
    setup(&s, &l);
    s.silent = 1;
    CHECK_EQ(k210_send_payload(&l, 40, body, sizeof body, false), K210_OK);
    CHECK_EQ(s.npays, 1);
    CHECK(s.now_ms < 500);

    /* ---- limits and argument checks ---- */
    setup(&s, &l);
    static uint8_t big[2000];
    CHECK_EQ(k210_send_payload(&l, 2, big, 1001, true), K210_ERR_TOO_LONG);
    CHECK_EQ(k210_send_payload(&l, 2, NULL, 5, true), K210_ERR_ARG);
    CHECK_EQ(s.writes, 0);
    CHECK_EQ(k210_send_payload(&l, 2, big, 1000, true), K210_OK);

    /* ---- I/O failure is reported, not retried ---- */
    setup(&s, &l);
    s.fail_write = 1;
    CHECK_EQ(k210_send_payload(&l, 40, body, sizeof body, true), K210_ERR_IO);
    CHECK_EQ(k210_send_short(&l, 52, 0, 0, 0), K210_ERR_IO);

    /* ---- stale ack must not satisfy the next send ---- */
    setup(&s, &l);
    s.ack_pending = K210_ACK_GOOD;             /* leftover from an earlier frame */
    s.silent = 1;
    CHECK_EQ(k210_send_payload(&l, 40, body, sizeof body, true), K210_ERR_TIMEOUT);

    /* ---- version handshake ---- */
    setup(&s, &l);
    s.fw_word = 0x000102;
    uint32_t w = 0;
    CHECK_EQ(k210_query_version(&l, &w, 3, 200), K210_OK);
    CHECK_EQ(w, 0x000102);
    CHECK_EQ(s.nshorts, 1);                    /* answered on the first try */

    setup(&s, &l);                             /* K210 not up: 3 tries, then timeout */
    CHECK_EQ(k210_query_version(&l, &w, 3, 200), K210_ERR_TIMEOUT);
    CHECK_EQ(s.nshorts, 3);
    CHECK(s.now_ms >= 600);

    setup(&s, &l);                             /* wait_ready times out cleanly */
    CHECK_EQ(k210_wait_ready(&l, 1000, &w), K210_ERR_NOT_READY);
    CHECK(s.nshorts >= 4);

    char txt[16];
    setup(&s, &l);
    s.ver_text = "9.9.9";
    CHECK_EQ(k210_query_version_text(&l, txt, sizeof txt, 200), K210_OK);
    CHECK(strcmp(txt, "9.9.9") == 0);
    setup(&s, &l);
    CHECK_EQ(k210_query_version_text(&l, txt, sizeof txt, 100), K210_ERR_TIMEOUT);

    /* ---- play by name: subs 24/26 (avi) and 25/27 (mp3), body = stock-form path + NUL ---- */
    setup(&s, &l);
    CHECK_EQ(k210_play_avi(&l, "demo_a", false), K210_OK);
    CHECK_EQ(s.pays[0].sub, 24);
    CHECK(strcmp((const char *)s.pays[0].data, "/spiffs/avi/demo_a.avi") == 0);
    CHECK_EQ(s.pays[0].len, strlen("/spiffs/avi/demo_a.avi") + 1);
    CHECK_EQ(k210_play_avi(&l, "demo_a", true), K210_OK);
    CHECK_EQ(s.pays[1].sub, 26);
    CHECK_EQ(k210_play_mp3(&l, "demo_a", false), K210_OK);
    CHECK_EQ(s.pays[2].sub, 25);
    CHECK(strcmp((const char *)s.pays[2].data, "/spiffs/mp3/demo_a.mp3") == 0);
    CHECK_EQ(k210_play_mp3(&l, "demo_a", true), K210_OK);
    CHECK_EQ(s.pays[3].sub, 27);

    /* the /test plain path goes through unchanged */
    CHECK_EQ(k210_play_path(&l, K210_MEDIA_AVI, false, "/test/open_demo.avi"), K210_OK);
    CHECK(strcmp((const char *)s.pays[4].data, "/test/open_demo.avi") == 0);

    /* unsafe names are refused before anything is written */
    int before = s.writes;
    CHECK_EQ(k210_play_avi(&l, "../x", false), K210_ERR_ARG);
    CHECK_EQ(k210_play_avi(&l, "a/b", false), K210_ERR_ARG);
    CHECK_EQ(k210_play_avi(&l, "", false), K210_ERR_ARG);
    CHECK_EQ(k210_play_path(&l, K210_MEDIA_AVI, false, "relative"), K210_ERR_ARG);
    CHECK_EQ(s.writes, before);

    /* ---- full animation through sub 2 ---- */
    setup(&s, &l);
    char pbuf[256];
    k210_playlist_t pl;
    k210_sound_t snd[] = { { 0, "/test/demo_a.mp3" } };
    CHECK_EQ(k210_playlist_init(&pl, pbuf, sizeof pbuf, 0), 0);
    CHECK_EQ(k210_playlist_add_clip(&pl, "/test/demo_a.avi", snd, 1), 0);
    size_t pn = 0;
    const char *json = k210_playlist_finish(&pl, &pn);
    CHECK(json != NULL);
    CHECK_EQ(k210_play_playlist(&l, json, pn), K210_OK);
    CHECK_EQ(s.pays[0].sub, 2);
    CHECK_EQ(s.pays[0].len, pn + 1);
    CHECK(strcmp((const char *)s.pays[0].data, json) == 0);
    CHECK_EQ(k210_play_playlist(&l, json, pn - 1), K210_ERR_ARG);   /* length must match the string */
    CHECK_EQ(k210_play_playlist(&l, "", 0), K210_ERR_ARG);

    TEST_DONE();
}
