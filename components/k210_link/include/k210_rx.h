/* Byte-at-a-time frame parser for the K210 UART link (both directions use the same framing). */
#ifndef K210_RX_H
#define K210_RX_H

#include <stddef.h>
#include <stdint.h>
#include "k210_proto.h"

#ifdef __cplusplus
extern "C" {
#endif

typedef enum { K210_EV_SHORT = 1, K210_EV_PAYLOAD = 2 } k210_ev_kind_t;

typedef struct {
    k210_ev_kind_t kind;
    uint8_t cmd, b1, b2, b3;     /* K210_EV_SHORT */
    uint8_t sub;                 /* K210_EV_PAYLOAD */
    const uint8_t *data;         /* K210_EV_PAYLOAD: valid only during the callback */
    uint16_t len;
    uint8_t crc_ok;              /* K210_EV_PAYLOAD: body CRC matched */
} k210_event_t;

typedef void (*k210_event_cb)(void *user, const k210_event_t *ev);

typedef struct {
    uint8_t state;
    uint8_t idx;
    uint8_t hdr[5];
    uint8_t sub;
    uint16_t want, have;
    uint32_t skip;
    uint8_t *buf;
    uint16_t cap;
    k210_event_cb cb;
    void *user;
    uint32_t stat_sum_err, stat_crc_err, stat_overflow, stat_frames;
} k210_rx_t;

void k210_rx_init(k210_rx_t *rx, uint8_t *buf, uint16_t cap, k210_event_cb cb, void *user);
void k210_rx_reset(k210_rx_t *rx);
int  k210_rx_busy(const k210_rx_t *rx);   /* non-zero while inside a frame */
void k210_rx_feed(k210_rx_t *rx, const uint8_t *data, size_t n);

#ifdef __cplusplus
}
#endif
#endif
