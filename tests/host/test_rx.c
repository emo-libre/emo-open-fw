#include "test_util.h"
#include "k210_rx.h"

typedef struct {
    int shorts, pays, crc_bad;
    uint8_t last_cmd, last_b1, last_b2, last_b3, last_sub;
    uint8_t data[2048];
    uint16_t len;
} cap_t;

static void on_ev(void *u, const k210_event_t *ev)
{
    cap_t *c = (cap_t *)u;
    if (ev->kind == K210_EV_SHORT) {
        c->shorts++;
        c->last_cmd = ev->cmd; c->last_b1 = ev->b1; c->last_b2 = ev->b2; c->last_b3 = ev->b3;
    } else {
        c->pays++;
        c->last_sub = ev->sub;
        c->len = ev->len;
        memcpy(c->data, ev->data, ev->len);
        if (!ev->crc_ok) {
            c->crc_bad++;
        }
    }
}

static void feed_bytewise(k210_rx_t *rx, const uint8_t *d, size_t n)
{
    for (size_t i = 0; i < n; i++) {
        k210_rx_feed(rx, d + i, 1);
    }
}

int main(void)
{
    uint8_t buf[256];
    cap_t c;
    k210_rx_t rx;

    /* --- short frame, byte by byte --- */
    memset(&c, 0, sizeof c);
    k210_rx_init(&rx, buf, sizeof buf, on_ev, &c);
    uint8_t f[K210_SHORT_LEN];
    k210_build_short(f, 63, 1, 2, 3);
    feed_bytewise(&rx, f, sizeof f);
    CHECK_EQ(c.shorts, 1);
    CHECK_EQ(c.last_cmd, 63);
    CHECK_EQ(c.last_b3, 3);
    CHECK(!k210_rx_busy(&rx));

    /* --- noise before sync, and 55 55 AA --- */
    memset(&c, 0, sizeof c);
    const uint8_t noise[] = { 0x00, 0x13, 0xAA, 0x55, 0x55 };
    k210_rx_feed(&rx, noise, sizeof noise);
    k210_rx_feed(&rx, f + 1, sizeof f - 1);          /* AA cmd b1 b2 b3 sum  (the 55 came from noise) */
    CHECK_EQ(c.shorts, 1);                           /* DV-005 */

    /* --- bad header sum then a good frame: nothing emitted for the bad one --- */
    memset(&c, 0, sizeof c);
    uint8_t bad[K210_SHORT_LEN];
    k210_build_short(bad, 10, 0, 0, 0);
    bad[6] ^= 0xFF;
    k210_rx_feed(&rx, bad, sizeof bad);
    CHECK_EQ(c.shorts, 0);
    CHECK_EQ(rx.stat_sum_err, 1);
    k210_rx_feed(&rx, f, sizeof f);
    CHECK_EQ(c.shorts, 1);

    /* --- DV-004: a real frame hidden inside the five bytes of a failed header --- */
    memset(&c, 0, sizeof c);
    uint8_t good[K210_SHORT_LEN];
    k210_build_short(good, 20, 7, 8, 9);
    uint8_t stream[2 + 5 + K210_SHORT_LEN];
    stream[0] = 0x55; stream[1] = 0xAA;                       /* false start ... */
    stream[2] = 0x11; stream[3] = 0x22;                       /* ... garbage ... */
    memcpy(stream + 4, good, sizeof good);                    /* the real frame begins inside it */
    k210_rx_feed(&rx, stream, 4 + sizeof good);
    CHECK_EQ(c.shorts, 1);
    CHECK_EQ(c.last_cmd, 20);

    /* --- payload: good CRC --- */
    memset(&c, 0, sizeof c);
    uint8_t body[100];
    for (int i = 0; i < 100; i++) body[i] = (uint8_t)(i * 3);
    uint8_t pf[128];
    size_t pn = k210_build_payload(pf, sizeof pf, 68, body, sizeof body);
    feed_bytewise(&rx, pf, pn);
    CHECK_EQ(c.pays, 1);
    CHECK_EQ(c.crc_bad, 0);
    CHECK_EQ(c.last_sub, 68);
    CHECK_EQ(c.len, 100);
    CHECK_MEM(c.data, body, 100);

    /* --- payload: corrupted body byte -> event with crc_ok == 0, then recovery --- */
    memset(&c, 0, sizeof c);
    pf[7 + 10] ^= 0x01;
    k210_rx_feed(&rx, pf, pn);
    CHECK_EQ(c.pays, 1);
    CHECK_EQ(c.crc_bad, 1);
    CHECK_EQ(rx.stat_crc_err, 1);
    k210_rx_feed(&rx, f, sizeof f);
    CHECK_EQ(c.shorts, 1);

    /* --- zero-length payload --- */
    memset(&c, 0, sizeof c);
    pn = k210_build_payload(pf, sizeof pf, 5, NULL, 0);
    k210_rx_feed(&rx, pf, pn);
    CHECK_EQ(c.pays, 1);
    CHECK_EQ(c.len, 0);

    /* --- DV-001: payload larger than our buffer is skipped, parser resyncs --- */
    memset(&c, 0, sizeof c);
    static uint8_t huge[300], hf[320];
    memset(huge, 0xAA, sizeof huge);                           /* contains 0xAA but no 0x55 sync pairs */
    size_t hn = k210_build_payload(hf, sizeof hf, 3, huge, sizeof huge);   /* 300 > cap 256 */
    k210_rx_feed(&rx, hf, hn);
    CHECK_EQ(c.pays, 0);
    CHECK_EQ(rx.stat_overflow, 1);
    k210_rx_feed(&rx, f, sizeof f);
    CHECK_EQ(c.shorts, 1);

    /* --- reset drops a half frame --- */
    memset(&c, 0, sizeof c);
    pn = k210_build_payload(pf, sizeof pf, 68, body, sizeof body);   /* 108 bytes */
    k210_rx_feed(&rx, pf, 50);                                       /* half a frame */
    CHECK(k210_rx_busy(&rx));
    k210_rx_reset(&rx);
    CHECK(!k210_rx_busy(&rx));
    k210_rx_feed(&rx, f, sizeof f);
    CHECK_EQ(c.shorts, 1);

    TEST_DONE();
}
