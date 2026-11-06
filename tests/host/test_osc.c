#include <math.h>
#include "test_util.h"
#include "osc.h"

/* golden tables generated independently with Python (no value sits on an integer boundary) */
static const int A_T[30] = { 2, 9, 14, 19, 24, 27, 29, 29, 29, 27, 24, 20, 15, 9, 3, -2, -9, -14, -19, -24,
                             -27, -29, -29, -29, -27, -24, -20, -15, -9, -3 };
static const int B_T[20] = { 15, 21, 26, 28, 29, 29, 26, 21, 16, 10, 4, -1, -6, -8, -9, -9, -6, -1, 3, 9 };
static const int C_T[8]  = { 28, 31, 34, 36, 38, 39, 39, 39 };

typedef struct {
    int      n;
    osc_frame_t f[400];
    int      paces, cycles, hold_after, fail_at;
} rec_t;

static int emit(void *c, const osc_frame_t *f)
{
    rec_t *r = (rec_t *)c;
    if (r->fail_at && r->n + 1 == r->fail_at) return 1;
    if (r->n < 400) r->f[r->n] = *f;
    r->n++;
    return 0;
}
static void pace(void *c) { ((rec_t *)c)->paces++; }
static int hold(void *c) { rec_t *r = (rec_t *)c; return r->hold_after && r->n >= r->hold_after; }
static void cyc(void *c) { ((rec_t *)c)->cycles++; }

static osc_params_t params(int amp, int centre, double ph0, int inv, int trim)
{
    osc_params_t p;
    memset(&p, 0, sizeof p);
    for (int j = 0; j < OSC_SERVOS; j++) {
        p.amp[j] = amp; p.centre[j] = centre; p.phase0[j] = ph0; p.invert[j] = (uint8_t)inv; p.trim[j] = trim;
    }
    return p;
}

int main(void)
{
    /* ---- pulse width map ---- */
    CHECK_EQ(osc_angle_to_us(0), 500);
    CHECK_EQ(osc_angle_to_us(90), 1500);
    CHECK_EQ(osc_angle_to_us(180), 2500);
    CHECK_EQ(osc_angle_to_us(1), 511);        /* 11.11 + 500 + 0.5 -> 511 */
    CHECK_EQ(osc_angle_to_us(-30), 500);      /* DV-010 clamp */
    CHECK_EQ(osc_angle_to_us(500), 2500);
    for (int a = 0; a <= 180; a++) {          /* matches the float definition exactly */
        CHECK_EQ(osc_angle_to_us(a), (int)((double)a * 2000.0 / 180.0 + 500.0 + 0.5));
    }

    /* ---- one full cycle against the independent table ---- */
    static rec_t r;
    memset(&r, 0, sizeof r);
    osc_t o;
    osc_params_t pa = params(30, 0, 0.1, 0, 0);
    CHECK_EQ(osc_begin(&o, &pa, 600, 0.0), 0);
    CHECK(fabs(o.frames_per_cycle - 30.0) < 1e-12);
    osc_sink_t sink = { emit, pace, hold, cyc, &r };
    uint32_t n = 0;
    CHECK_EQ(osc_run(&o, 1.0, &sink, &n), OSC_DONE);
    CHECK_EQ(n, 30);
    CHECK_EQ(r.paces, 30);
    CHECK_EQ(r.cycles, 1);
    for (int i = 0; i < 30; i++) {
        CHECK_EQ(r.f[i].rel[0], A_T[i]);
        CHECK_EQ(r.f[i].abs[0], A_T[i] + 90);
        CHECK_EQ(r.f[i].us[0], osc_angle_to_us(A_T[i] + 90));
        CHECK_EQ(r.f[i].rel[3], A_T[i]);       /* all servos identical here */
    }

    /* ---- centre offset ---- */
    memset(&r, 0, sizeof r);
    osc_params_t pb = params(20, 10, 0.3, 0, 0);
    osc_begin(&o, &pb, 400, 0.0);
    osc_run(&o, 1.0, &sink, &n);
    CHECK_EQ(n, 20);
    for (int i = 0; i < 20; i++) CHECK_EQ(r.f[i].rel[1], B_T[i]);

    /* ---- invert negates the truncated value ---- */
    memset(&r, 0, sizeof r);
    pb.invert[2] = 1;
    osc_begin(&o, &pb, 400, 0.0);
    osc_run(&o, 1.0, &sink, &n);
    for (int i = 0; i < 20; i++) {
        CHECK_EQ(r.f[i].rel[2], -B_T[i]);
        CHECK_EQ(r.f[i].rel[1], B_T[i]);
    }

    /* ---- start phase in degrees ---- */
    memset(&r, 0, sizeof r);
    osc_params_t pc = params(40, 0, 0.0, 0, 0);
    osc_begin(&o, &pc, 1000, 45.0);
    osc_run(&o, 8.0 / 50.0, &sink, &n);        /* 8 frames */
    CHECK_EQ(n, 8);
    for (int i = 0; i < 8; i++) CHECK_EQ(r.f[i].rel[0], C_T[i]);

    /* ---- phase offset of exactly pi/2 starts at the peak ---- */
    osc_params_t pd = params(30, 0, 1.57079632679489661923, 0, 0);
    osc_begin(&o, &pd, 600, 0.0);
    osc_frame_t fr;
    osc_step(&o, &fr);
    CHECK_EQ(fr.rel[0], 30);

    /* ---- different phase per servo (a walk): servo 1 lags by a quarter cycle ---- */
    osc_params_t pw = params(30, 0, 0.0, 0, 0);
    pw.phase0[1] = 1.57079632679489661923;
    memset(&r, 0, sizeof r);
    osc_begin(&o, &pw, 600, 0.0);
    osc_run(&o, 1.0, &sink, &n);
    CHECK(r.f[0].rel[1] >= 29);                /* sin(pi/2) * 30 */
    CHECK(abs(r.f[0].rel[0]) <= 1);

    /* ---- trim and clamping ---- */
    osc_params_t pt = params(0, 0, 0.0, 0, 7);
    osc_begin(&o, &pt, 600, 0.0);
    osc_step(&o, &fr);
    CHECK_EQ(fr.abs[0], 97);
    pt = params(0, 120, 0.0, 0, 0);           /* centre would drive past 180 */
    osc_begin(&o, &pt, 600, 0.0);
    osc_step(&o, &fr);
    CHECK_EQ(fr.rel[0], 120);
    CHECK_EQ(fr.abs[0], 180);
    CHECK_EQ(fr.us[0], 2500);

    /* ---- fractional cycles and phase continuity across runs ---- */
    memset(&r, 0, sizeof r);
    osc_begin(&o, &pa, 600, 0.0);
    CHECK_EQ(osc_run(&o, 2.5, &sink, &n), OSC_DONE);
    CHECK_EQ(n, 75);
    CHECK_EQ(r.cycles, 2);                     /* only whole cycles are counted */
    CHECK_EQ(o.cycles_done, 2);
    for (int i = 0; i < 45; i++) CHECK_EQ(r.f[i].rel[0], r.f[i + 30].rel[0]);   /* periodic */

    memset(&r, 0, sizeof r);
    osc_begin(&o, &pa, 600, 0.0);
    osc_run(&o, 0.5, &sink, &n);               /* 15 frames */
    osc_run(&o, 0.5, &sink, &n);               /* the next 15 continue the same wave */
    CHECK_EQ(r.n, 30);
    for (int i = 0; i < 30; i++) CHECK_EQ(r.f[i].rel[0], A_T[i]);

    /* ---- period clamp (DV-009): 5 ms behaves like 20 ms ---- */
    osc_begin(&o, &pa, 5, 0.0);
    CHECK(fabs(o.frames_per_cycle - 1.0) < 1e-12);

    /* ---- hold: at start, and mid-run ---- */
    memset(&r, 0, sizeof r);
    r.hold_after = 1;
    r.n = 0;
    osc_begin(&o, &pa, 600, 0.0);
    r.n = 5;                                   /* hold() is already true */
    CHECK_EQ(osc_run(&o, 1.0, &sink, &n), OSC_HELD);
    CHECK_EQ(n, 0);
    memset(&r, 0, sizeof r);
    r.hold_after = 10;
    osc_begin(&o, &pa, 600, 0.0);
    CHECK_EQ(osc_run(&o, 5.0, &sink, &n), OSC_HELD);
    CHECK_EQ(n, 10);

    /* ---- emit failure aborts ---- */
    memset(&r, 0, sizeof r);
    r.fail_at = 4;
    osc_begin(&o, &pa, 600, 0.0);
    CHECK_EQ(osc_run(&o, 1.0, &sink, &n), OSC_ERR);
    CHECK_EQ(n, 3);

    /* ---- argument checks ---- */
    memset(&r, 0, sizeof r);
    osc_begin(&o, &pa, 600, 0.0);
    CHECK_EQ(osc_run(&o, -1.0, &sink, &n), OSC_ERR);
    CHECK_EQ(osc_run(&o, NAN, &sink, &n), OSC_ERR);
    CHECK_EQ(osc_begin(&o, NULL, 600, 0.0), -1);
    osc_params_t bad = pa;
    bad.phase0[2] = NAN;
    CHECK_EQ(osc_begin(&o, &bad, 600, 0.0), -1);
    CHECK_EQ(osc_begin(&o, &pa, 600, INFINITY), -1);
    CHECK_EQ(r.n, 0);

    /* ---- zero amplitude holds the centre ---- */
    memset(&r, 0, sizeof r);
    osc_params_t pz = params(0, 5, 0.7, 0, 0);
    osc_begin(&o, &pz, 400, 0.0);
    osc_run(&o, 1.0, &sink, &n);
    for (int i = 0; i < 20; i++) CHECK_EQ(r.f[i].rel[0], 5);

    TEST_DONE();
}
