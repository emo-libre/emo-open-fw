/* Frame parser. Deviations from stock: DV-001 (bounded buffer), DV-004 (resync after a bad
 * header), DV-005 (repeated 0x55 keeps sync armed). */
#include <string.h>
#include "k210_rx.h"

enum { S_SYNC0 = 0, S_SYNC1, S_HDR, S_BODY, S_CRC, S_SKIP };

static void step(k210_rx_t *rx, uint8_t c, int allow_rescan);

static void emit(k210_rx_t *rx, const k210_event_t *ev)
{
    rx->stat_frames++;
    if (rx->cb) {
        rx->cb(rx->user, ev);
    }
}

static void header_done(k210_rx_t *rx, int allow_rescan)
{
    const uint8_t *h = rx->hdr;
    uint8_t sum = k210_short_sum(h[0], h[1], h[2], h[3]);

    if (sum != h[4]) {
        rx->stat_sum_err++;
        rx->state = S_SYNC0;
        if (allow_rescan) {
            /* The five bytes may contain the start of a genuine frame: replay them. */
            uint8_t tmp[5];
            memcpy(tmp, h, sizeof tmp);
            for (unsigned i = 0; i < sizeof tmp; i++) {
                step(rx, tmp[i], 0);
            }
        }
        return;
    }

    if (h[0] == K210_CMD_PAYLOAD) {
        rx->want = (uint16_t)(h[1] | (h[2] << 8));
        rx->sub  = h[3];
        rx->have = 0;
        if (rx->want > rx->cap) {
            rx->stat_overflow++;
            rx->skip = (uint32_t)rx->want + 1u;   /* body + CRC byte */
            rx->state = S_SKIP;
        } else {
            rx->state = rx->want ? S_BODY : S_CRC;
        }
        return;
    }

    k210_event_t ev = { .kind = K210_EV_SHORT, .cmd = h[0], .b1 = h[1], .b2 = h[2], .b3 = h[3] };
    rx->state = S_SYNC0;
    emit(rx, &ev);
}

static void step(k210_rx_t *rx, uint8_t c, int allow_rescan)
{
    switch (rx->state) {
    case S_SYNC0:
        if (c == K210_SYNC0) {
            rx->state = S_SYNC1;
        }
        break;
    case S_SYNC1:
        if (c == K210_SYNC1) {
            rx->state = S_HDR;
            rx->idx = 0;
        } else if (c != K210_SYNC0) {      /* 55 55 AA: stay armed (DV-005) */
            rx->state = S_SYNC0;
        }
        break;
    case S_HDR:
        rx->hdr[rx->idx++] = c;
        if (rx->idx == sizeof rx->hdr) {
            header_done(rx, allow_rescan);
        }
        break;
    case S_BODY:
        rx->buf[rx->have++] = c;
        if (rx->have == rx->want) {
            rx->state = S_CRC;
        }
        break;
    case S_CRC: {
        uint8_t ok = (k210_crc8_maxim(rx->buf, rx->want) == c);
        if (!ok) {
            rx->stat_crc_err++;
        }
        k210_event_t ev = { .kind = K210_EV_PAYLOAD, .sub = rx->sub, .data = rx->buf,
                            .len = rx->want, .crc_ok = ok };
        rx->state = S_SYNC0;
        emit(rx, &ev);
        break;
    }
    case S_SKIP:
        if (--rx->skip == 0) {
            rx->state = S_SYNC0;
        }
        break;
    default:
        rx->state = S_SYNC0;
        break;
    }
}

void k210_rx_init(k210_rx_t *rx, uint8_t *buf, uint16_t cap, k210_event_cb cb, void *user)
{
    memset(rx, 0, sizeof *rx);
    rx->buf = buf;
    rx->cap = cap;
    rx->cb = cb;
    rx->user = user;
}

void k210_rx_reset(k210_rx_t *rx)
{
    rx->state = S_SYNC0;
    rx->idx = 0;
    rx->have = 0;
    rx->want = 0;
    rx->skip = 0;
}

int k210_rx_busy(const k210_rx_t *rx)
{
    return rx->state != S_SYNC0;
}

void k210_rx_feed(k210_rx_t *rx, const uint8_t *data, size_t n)
{
    while (n--) {
        step(rx, *data++, 1);
    }
}
