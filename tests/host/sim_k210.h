/* A simulated K210 peer wired to a k210_link_t through the HAL, single-threaded and synchronous.
 * It parses what the host writes with the same frame parser, acknowledges payloads, and answers
 * cmd 60 / cmd 105. Virtual time advances only through sleep_ms / ack timeouts. */
#ifndef SIM_K210_H
#define SIM_K210_H

#include <string.h>
#include "k210_link.h"

typedef struct {
    k210_link_t *host;
    k210_rx_t    rx;
    uint8_t      rxbuf[2048];

    /* behaviour knobs */
    int nack_first;            /* answer the first N payloads with 0x55 */
    int silent;                /* never acknowledge */
    uint32_t fw_word;          /* reply to cmd 60 (0 = do not answer) */
    const char *ver_text;      /* reply to cmd 105 (NULL = do not answer) */

    /* capture */
    uint8_t  wire[8192];       /* every byte the host wrote */
    size_t   wire_len;
    int      writes;
    struct { uint8_t cmd, b1, b2, b3; } shorts[64];
    int      nshorts;
    struct { uint8_t sub; uint8_t data[1100]; uint16_t len; uint8_t crc_ok; } pays[32];
    int      npays;

    /* HAL state */
    int      ack_pending;      /* -1 none */
    uint32_t now_ms;
    int      fail_write;       /* make write() fail */
} sim_t;

static void sim_reply_short(sim_t *s, uint8_t cmd, uint8_t b1, uint8_t b2, uint8_t b3)
{
    uint8_t f[K210_SHORT_LEN];
    k210_build_short(f, cmd, b1, b2, b3);
    k210_link_feed(s->host, f, sizeof f);
}

static void sim_reply_payload(sim_t *s, uint8_t sub, const void *body, size_t len)
{
    uint8_t f[256];
    size_t n = k210_build_payload(f, sizeof f, sub, body, len);
    k210_link_feed(s->host, f, n);
}

static void sim_on_event(void *user, const k210_event_t *ev)
{
    sim_t *s = (sim_t *)user;
    if (ev->kind == K210_EV_SHORT) {
        if (s->nshorts < 64) {
            s->shorts[s->nshorts].cmd = ev->cmd;
            s->shorts[s->nshorts].b1 = ev->b1;
            s->shorts[s->nshorts].b2 = ev->b2;
            s->shorts[s->nshorts].b3 = ev->b3;
            s->nshorts++;
        }
        if (ev->cmd == K210_CMD_FW_NUMBER && s->fw_word) {
            sim_reply_short(s, K210_CMD_FW_NUMBER, (uint8_t)s->fw_word, (uint8_t)(s->fw_word >> 8),
                            (uint8_t)(s->fw_word >> 16));
        } else if (ev->cmd == K210_CMD_VERSION_TEXT && s->ver_text) {
            sim_reply_payload(s, K210_SUB_VERSION_TEXT, s->ver_text, strlen(s->ver_text) + 1u);
        }
    } else {
        if (s->npays < 32) {
            s->pays[s->npays].sub = ev->sub;
            s->pays[s->npays].len = ev->len;
            s->pays[s->npays].crc_ok = ev->crc_ok;
            memcpy(s->pays[s->npays].data, ev->data, ev->len < 1100 ? ev->len : 1100);
            s->npays++;
        }
        if (!s->silent) {
            uint8_t ack = K210_ACK_GOOD;
            if (!ev->crc_ok || s->nack_first > 0) {
                ack = K210_ACK_BAD;
                if (s->nack_first > 0) {
                    s->nack_first--;
                }
            }
            sim_reply_short(s, K210_CMD_ACK, ack, 0, 0);
        }
    }
}

/* ---- HAL ---- */
static int hal_write(void *ctx, const uint8_t *buf, size_t n)
{
    sim_t *s = (sim_t *)ctx;
    if (s->fail_write) {
        return -1;
    }
    if (s->wire_len + n <= sizeof s->wire) {
        memcpy(s->wire + s->wire_len, buf, n);
        s->wire_len += n;
    }
    s->writes++;
    k210_rx_feed(&s->rx, buf, n);
    return (int)n;
}
static void hal_sleep(void *ctx, uint32_t ms) { ((sim_t *)ctx)->now_ms += ms; }
static void hal_ack_flush(void *ctx) { ((sim_t *)ctx)->ack_pending = -1; }
static void hal_ack_signal(void *ctx, uint8_t a) { ((sim_t *)ctx)->ack_pending = a; }
static int hal_ack_wait(void *ctx, uint32_t timeout_ms)
{
    sim_t *s = (sim_t *)ctx;
    if (s->ack_pending >= 0) {
        int a = s->ack_pending;
        s->ack_pending = -1;
        return a;
    }
    s->now_ms += timeout_ms;
    return -1;
}

static void sim_init(sim_t *s, k210_link_t *host, uint8_t *host_rxbuf, uint16_t host_rxcap)
{
    memset(s, 0, sizeof *s);
    s->host = host;
    s->ack_pending = -1;
    k210_rx_init(&s->rx, s->rxbuf, sizeof s->rxbuf, sim_on_event, s);
    k210_hal_t hal = { .write = hal_write, .sleep_ms = hal_sleep, .lock = NULL, .unlock = NULL,
                       .ack_flush = hal_ack_flush, .ack_signal = hal_ack_signal,
                       .ack_wait = hal_ack_wait, .ctx = s };
    k210_link_cfg_t cfg = K210_LINK_CFG_DEFAULT;
    k210_link_init(host, &hal, &cfg, host_rxbuf, host_rxcap);
}

#endif
