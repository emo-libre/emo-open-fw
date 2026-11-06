#include <math.h>
#include "test_util.h"
#include "mot.h"

typedef struct {
    int       n, paces, procs, hold_after, fail_at;
    mot_out_t o[200];
    uint8_t   proc_sub[8];
} rec_t;

static int emit(void *c, const mot_out_t *o)
{
    rec_t *r = (rec_t *)c;
    if (r->fail_at && r->n + 1 == r->fail_at) return 1;
    if (r->n < 200) r->o[r->n] = *o;
    r->n++;
    return 0;
}
static void pace(void *c) { ((rec_t *)c)->paces++; }
static int hold(void *c) { rec_t *r = (rec_t *)c; return r->hold_after && r->n >= r->hold_after; }
static int proc(void *c, const mot_frame_t *f)
{
    rec_t *r = (rec_t *)c;
    if (r->procs < 8) r->proc_sub[r->procs] = f->sub;
    r->procs++;
    return 0;
}

static void put(uint8_t *file, int idx, uint8_t sub, int a0, int a1, int a2, int a3, unsigned dur)
{
    mot_frame_t f = { .type = 2, .sub = sub, .params = { 0, 0, 0, 0, 0 },
                      .angle = { (uint8_t)a0, (uint8_t)a1, (uint8_t)a2, (uint8_t)a3 }, .b15 = 0,
                      .dur_ms = (uint16_t)dur };
    mot_frame_build(file + idx * MOT_FRAME_LEN, &f);
}

static uint16_t us_ref(double deg) { return (uint16_t)(deg * 2000.0 / 180.0 + 500.0 + 0.5); }

int main(void)
{
    uint8_t file[20 * 8];
    memset(file, 0, sizeof file);

    /* ---- build/parse round trip and layout ---- */
    mot_frame_t f = { .type = 2, .sub = 0xFE, .params = { 1, 2, 3, 4, 5 }, .angle = { 10, 20, 30, 40 },
                      .b15 = 0, .dur_ms = 0x0123 };
    uint8_t raw[20];
    mot_frame_build(raw, &f);
    CHECK_EQ(raw[0], 0x55); CHECK_EQ(raw[1], 0xAA); CHECK_EQ(raw[2], 0x55); CHECK_EQ(raw[3], 0xAA);
    CHECK_EQ(raw[4], 2); CHECK_EQ(raw[5], 0xFE);
    CHECK_EQ(raw[6], 1); CHECK_EQ(raw[10], 5);
    CHECK_EQ(raw[11], 10); CHECK_EQ(raw[14], 40);
    CHECK_EQ(raw[16], 0x23); CHECK_EQ(raw[17], 0x01);         /* little-endian duration */
    unsigned s = 0;
    for (int i = 4; i <= 17; i++) s += raw[i];
    CHECK_EQ(raw[18], s & 0xFF);
    CHECK_EQ(raw[19], 0xED);
    mot_frame_t g;
    CHECK_EQ(mot_frame_parse(raw, &g), MOT_OK);
    CHECK_EQ(g.dur_ms, 0x0123); CHECK_EQ(g.angle[2], 30); CHECK_EQ(g.sub, 0xFE); CHECK_EQ(g.params[3], 4);

    /* ---- corruptions ---- */
    uint8_t bad[20];
    memcpy(bad, raw, 20); bad[1] = 0; CHECK_EQ(mot_frame_parse(bad, &g), MOT_ERR_MAGIC);
    memcpy(bad, raw, 20); bad[12] ^= 1; CHECK_EQ(mot_frame_parse(bad, &g), MOT_ERR_SUM);
    memcpy(bad, raw, 20); bad[19] = 0; CHECK_EQ(mot_frame_parse(bad, &g), MOT_ERR_END);
    CHECK_EQ(mot_frame_parse(NULL, &g), MOT_ERR_ARG);

    /* ---- whole-file validation ---- */
    put(file, 0, 0xFE, 90, 90, 90, 90, 40);
    put(file, 1, 0xFE, 110, 70, 90, 100, 100);
    put(file, 2, 0x01, 0, 0, 0, 0, 0);
    size_t n = 0, bi = 99;
    CHECK_EQ(mot_validate(file, 60, &n, &bi), MOT_OK);
    CHECK_EQ(n, 3);
    CHECK_EQ(mot_validate(file, 59, &n, &bi), MOT_ERR_SIZE);
    CHECK_EQ(mot_validate(file, 0, &n, &bi), MOT_ERR_SIZE);
    CHECK_EQ(mot_validate(NULL, 20, &n, &bi), MOT_ERR_ARG);
    file[2 * 20 + 18] ^= 0xFF;
    CHECK_EQ(mot_validate(file, 60, &n, &bi), MOT_ERR_SUM);
    CHECK_EQ(bi, 2);
    CHECK_EQ(n, 0);
    file[2 * 20 + 18] ^= 0xFF;

    /* ---- playback: interpolation from the neutral pose ---- */
    static rec_t r;
    memset(&r, 0, sizeof r);
    mot_player_t p;
    mot_player_init(&p, NULL);
    mot_sink_t sink = { emit, pace, hold, proc, &r };
    uint32_t frames = 0;
    CHECK_EQ(mot_play(&p, file, 60, &sink, &frames), MOT_PLAY_DONE);
    /* frame 0: 40 ms -> 2 steps (neutral -> neutral); frame 1: 100 ms -> 5 steps; frame 2 is procedural */
    CHECK_EQ(frames, 7);
    CHECK_EQ(r.n, 7);
    CHECK_EQ(r.paces, 7);
    CHECK_EQ(r.procs, 1);
    CHECK_EQ(r.proc_sub[0], 0x01);
    CHECK_EQ(r.o[0].abs[0], 90);
    const int want0[5] = { 94, 98, 102, 106, 110 };           /* servo 0: 90 -> 110 */
    const int want1[5] = { 86, 82, 78, 74, 70 };              /* servo 1: 90 -> 70  */
    const int want3[5] = { 92, 94, 96, 98, 100 };             /* servo 3: 90 -> 100 */
    for (int i = 0; i < 5; i++) {
        CHECK_EQ(r.o[2 + i].abs[0], want0[i]);
        CHECK_EQ(r.o[2 + i].abs[1], want1[i]);
        CHECK_EQ(r.o[2 + i].abs[2], 90);
        CHECK_EQ(r.o[2 + i].abs[3], want3[i]);
        CHECK_EQ(r.o[2 + i].us[0], us_ref(want0[i]));
    }
    CHECK_EQ(r.o[6].us[0], us_ref(110));
    CHECK(fabs(p.last[0] - 110.0) < 1e-9);                    /* position carries to the next file */

    /* second file starts from where the first ended */
    uint8_t f2[20];
    put(f2, 0, 0xFE, 110, 70, 90, 100, 20);
    memset(&r, 0, sizeof r);
    CHECK_EQ(mot_play(&p, f2, 20, &sink, &frames), MOT_PLAY_DONE);
    CHECK_EQ(frames, 1);
    CHECK_EQ(r.o[0].abs[0], 110);

    /* ---- step counts: <=20 -> 1, 39 -> 1, 40 -> 2, 0 -> 1 ---- */
    const unsigned durs[] = { 0, 1, 20, 39, 40, 41, 100 };
    const unsigned want_steps[] = { 1, 1, 1, 1, 2, 2, 5 };
    for (unsigned i = 0; i < 7; i++) {
        uint8_t one[20];
        put(one, 0, 0xFE, 0, 0, 0, 0, durs[i]);
        mot_player_init(&p, NULL);
        memset(&r, 0, sizeof r);
        mot_play(&p, one, 20, &sink, &frames);
        CHECK_EQ(frames, want_steps[i]);
        CHECK_EQ(r.o[frames - 1].abs[0], 0);                  /* lands on the target */
    }

    /* ---- trim is added to every step; result clamped to 0..180 ---- */
    int trim[4] = { 5, -5, 0, 100 };
    mot_player_init(&p, trim);
    uint8_t one[20];
    put(one, 0, 0xFE, 90, 90, 90, 90, 20);
    memset(&r, 0, sizeof r);
    mot_play(&p, one, 20, &sink, &frames);
    CHECK_EQ(r.o[0].abs[0], 95);
    CHECK_EQ(r.o[0].abs[1], 85);
    CHECK_EQ(r.o[0].abs[3], 180);                             /* 190 clamped */
    CHECK_EQ(r.o[0].us[3], 2500);

    /* ---- angles above 180 are clamped ---- */
    mot_player_init(&p, NULL);
    put(one, 0, 0xFE, 250, 0, 0, 0, 20);
    memset(&r, 0, sizeof r);
    mot_play(&p, one, 20, &sink, &frames);
    CHECK_EQ(r.o[0].abs[0], 180);

    /* ---- procedural frames are skipped when there is no handler ---- */
    mot_sink_t noproc = { emit, pace, hold, NULL, &r };
    put(one, 0, 0x07, 1, 2, 3, 4, 100);
    memset(&r, 0, sizeof r);
    CHECK_EQ(mot_play(&p, one, 20, &noproc, &frames), MOT_PLAY_DONE);
    CHECK_EQ(frames, 0);

    /* ---- hold mid-move: stops, and remembers where it got to ---- */
    mot_player_init(&p, NULL);
    put(one, 0, 0xFE, 190, 90, 90, 90, 200);                  /* 10 steps, 90 -> 180 (clamped) */
    memset(&r, 0, sizeof r);
    r.hold_after = 3;
    CHECK_EQ(mot_play(&p, one, 20, &sink, &frames), MOT_PLAY_HELD);
    CHECK_EQ(frames, 3);
    CHECK(fabs(p.last[0] - 117.0) < 1e-9);                    /* 90 + 90*3/10 */

    /* ---- held before starting ---- */
    memset(&r, 0, sizeof r);
    r.hold_after = 1;
    r.n = 1;
    CHECK_EQ(mot_play(&p, one, 20, &sink, &frames), MOT_PLAY_HELD);
    CHECK_EQ(frames, 0);

    /* ---- invalid file plays nothing ---- */
    memset(&r, 0, sizeof r);
    uint8_t broken[40];
    put(broken, 0, 0xFE, 100, 100, 100, 100, 100);
    put(broken, 1, 0xFE, 120, 120, 120, 120, 100);
    broken[20 + 19] = 0;                                      /* second frame loses its terminator */
    mot_player_init(&p, NULL);
    CHECK_EQ(mot_play(&p, broken, 40, &sink, &frames), MOT_PLAY_BAD_FILE);
    CHECK_EQ(r.n, 0);                                         /* not even the good first frame */
    CHECK(fabs(p.last[0] - 90.0) < 1e-9);

    /* ---- emit failure ---- */
    memset(&r, 0, sizeof r);
    r.fail_at = 2;
    put(one, 0, 0xFE, 100, 100, 100, 100, 100);
    CHECK_EQ(mot_play(&p, one, 20, &sink, &frames), MOT_PLAY_ERR);
    CHECK_EQ(frames, 1);

    /* ---- argument checks ---- */
    CHECK_EQ(mot_play(NULL, one, 20, &sink, &frames), MOT_PLAY_ERR);
    CHECK_EQ(mot_play(&p, one, 20, NULL, &frames), MOT_PLAY_ERR);

    TEST_DONE();
}
