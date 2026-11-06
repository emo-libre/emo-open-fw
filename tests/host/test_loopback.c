/* Frame-parser loopback: encode random frames, interleave noise, feed in random chunk sizes,
 * and require every frame to come out intact and in order. */
#include "test_util.h"
#include "k210_rx.h"

#define MAXEV 4096

typedef struct {
    int n;
    struct { int kind; uint8_t cmd, b1, b2, b3, sub; uint16_t len; uint8_t crc_ok; uint8_t first, last; } ev[MAXEV];
} log_t;

static void on_ev(void *u, const k210_event_t *e)
{
    log_t *l = (log_t *)u;
    if (l->n >= MAXEV) return;
    l->ev[l->n].kind = e->kind;
    l->ev[l->n].cmd = e->cmd; l->ev[l->n].b1 = e->b1; l->ev[l->n].b2 = e->b2; l->ev[l->n].b3 = e->b3;
    l->ev[l->n].sub = e->sub; l->ev[l->n].len = e->len; l->ev[l->n].crc_ok = e->crc_ok;
    l->ev[l->n].first = e->len ? e->data[0] : 0;
    l->ev[l->n].last = e->len ? e->data[e->len - 1] : 0;
    l->n++;
}

int main(void)
{
    static uint8_t rxbuf[1024];
    static log_t log;
    static uint8_t stream[600000];
    static uint8_t expect_kind[MAXEV];
    static struct { uint8_t cmd, b1, b2, b3, sub; uint16_t len; uint8_t first, last; } exp[MAXEV];
    size_t sl = 0;
    int nexp = 0;

    for (int i = 0; i < 1500; i++) {
        if (rnd() % 3 == 0) {                         /* a little noise that cannot contain 0x55 */
            int nz = (int)(rnd() % 4);
            for (int k = 0; k < nz; k++) stream[sl++] = (uint8_t)(rnd() % 0x50);
        }
        if (rnd() % 2) {
            uint8_t cmd = (uint8_t)rnd();
            if (cmd == 0xFF) cmd = 0xFE;
            k210_build_short(stream + sl, cmd, (uint8_t)rnd(), (uint8_t)rnd(), (uint8_t)rnd());
            expect_kind[nexp] = K210_EV_SHORT;
            exp[nexp].cmd = cmd; exp[nexp].b1 = stream[sl + 3]; exp[nexp].b2 = stream[sl + 4]; exp[nexp].b3 = stream[sl + 5];
            nexp++;
            sl += K210_SHORT_LEN;
        } else {
            uint8_t body[700];
            size_t n = rnd() % 700;
            for (size_t k = 0; k < n; k++) body[k] = (uint8_t)rnd();
            uint8_t sub = (uint8_t)rnd();
            size_t w = k210_build_payload(stream + sl, sizeof stream - sl, sub, body, n);
            CHECK(w == n + 8);
            expect_kind[nexp] = K210_EV_PAYLOAD;
            exp[nexp].sub = sub; exp[nexp].len = (uint16_t)n;
            exp[nexp].first = n ? body[0] : 0; exp[nexp].last = n ? body[n - 1] : 0;
            nexp++;
            sl += w;
        }
    }

    k210_rx_t rx;
    k210_rx_init(&rx, rxbuf, sizeof rxbuf, on_ev, &log);
    size_t pos = 0;
    while (pos < sl) {
        size_t chunk = 1 + rnd() % 97;
        if (chunk > sl - pos) chunk = sl - pos;
        k210_rx_feed(&rx, stream + pos, chunk);
        pos += chunk;
    }

    CHECK_EQ(log.n, nexp);
    int mismatches = 0;
    for (int i = 0; i < nexp && i < log.n; i++) {
        int ok = (log.ev[i].kind == expect_kind[i]);
        if (ok && expect_kind[i] == K210_EV_SHORT) {
            ok = log.ev[i].cmd == exp[i].cmd && log.ev[i].b1 == exp[i].b1 &&
                 log.ev[i].b2 == exp[i].b2 && log.ev[i].b3 == exp[i].b3;
        } else if (ok) {
            ok = log.ev[i].sub == exp[i].sub && log.ev[i].len == exp[i].len && log.ev[i].crc_ok &&
                 log.ev[i].first == exp[i].first && log.ev[i].last == exp[i].last;
        }
        if (!ok) mismatches++;
    }
    CHECK_EQ(mismatches, 0);
    CHECK_EQ(rx.stat_sum_err, 0);
    CHECK_EQ(rx.stat_crc_err, 0);
    CHECK_EQ(rx.stat_overflow, 0);

    TEST_DONE();
}
