/* Link layer (framing + ack/retry) and the tier 0-1 command set for the K210. */
#ifndef K210_LINK_H
#define K210_LINK_H

#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>
#include "k210_proto.h"
#include "k210_rx.h"

#ifdef __cplusplus
extern "C" {
#endif

/* Platform hooks. write/sleep_ms/ack_signal/ack_wait are mandatory; the rest may be NULL. */
typedef struct {
    int  (*write)(void *ctx, const uint8_t *buf, size_t n);      /* bytes written, or <= 0 on error */
    void (*sleep_ms)(void *ctx, uint32_t ms);
    void (*lock)(void *ctx);                                     /* serialises senders */
    void (*unlock)(void *ctx);
    void (*ack_flush)(void *ctx);                                /* drop any stale ack */
    void (*ack_signal)(void *ctx, uint8_t ack);                  /* called from the RX path */
    int  (*ack_wait)(void *ctx, uint32_t timeout_ms);            /* ack byte 0..255, or -1 on timeout */
    void *ctx;
} k210_hal_t;

typedef struct {
    uint32_t ack_timeout_ms;   /* stock: 500 */
    uint8_t  max_retries;      /* resends after NACK/timeout (DV-003) */
    uint16_t gap_ms;           /* pause after each frame; stock: 10 */
    uint16_t max_payload;      /* DV-002 */
} k210_link_cfg_t;

#define K210_LINK_CFG_DEFAULT { 500u, 2u, 10u, K210_DEFAULT_MAX_PAYLOAD }

typedef struct k210_link {
    k210_hal_t      hal;
    k210_link_cfg_t cfg;
    k210_rx_t       rx;
    k210_event_cb   ev_cb;
    void           *ev_user;

    volatile uint8_t  ver_valid;        /* a cmd-60 reply arrived */
    volatile uint32_t ver_word;         /* b1 | b2<<8 | b3<<16 of that reply */
    volatile uint8_t  vtxt_valid;       /* a sub-105 reply arrived */
    char              ver_text[16];

    uint32_t stat_tx_short, stat_tx_payload, stat_tx_retries, stat_tx_nack, stat_tx_timeout;
} k210_link_t;

/* rxbuf/rxcap size the largest K210->ESP payload we accept (larger ones are skipped, DV-001). */
k210_err_t k210_link_init(k210_link_t *l, const k210_hal_t *hal, const k210_link_cfg_t *cfg,
                          uint8_t *rxbuf, uint16_t rxcap);
void k210_link_set_event_cb(k210_link_t *l, k210_event_cb cb, void *user);
void k210_link_feed(k210_link_t *l, const uint8_t *data, size_t n);   /* RX path entry */

/* ---- senders ---- */
k210_err_t k210_send_short(k210_link_t *l, uint8_t cmd, uint8_t b1, uint8_t b2, uint8_t b3);
/* wait_ack: wait for the K210's cmd-254 reply and retry per cfg. */
k210_err_t k210_send_payload(k210_link_t *l, uint8_t sub, const void *body, size_t len, bool wait_ack);

/* ---- tier 0 ---- */
k210_err_t k210_cmd_reset(k210_link_t *l);                                     /* cmd 52 */
k210_err_t k210_query_version(k210_link_t *l, uint32_t *word, unsigned tries, uint32_t wait_ms); /* cmd 60 */
k210_err_t k210_wait_ready(k210_link_t *l, uint32_t timeout_ms, uint32_t *word);
k210_err_t k210_query_version_text(k210_link_t *l, char *out, size_t cap, uint32_t wait_ms);     /* cmd 105 */

/* ---- tier 1 ---- */
k210_err_t k210_set_volume(k210_link_t *l, uint16_t level);                    /* cmd 23, 0..60 */

typedef enum {
    K210_EYE_DEFAULT  = 1,
    K210_EYE_VIGILANT = 2,
    K210_EYE_IDLE     = 4,
    K210_EYE_GAME     = 5
} k210_eye_mode_t;
k210_err_t k210_set_eye_mode(k210_link_t *l, uint16_t mode);                   /* cmd 56, 1..6 */

typedef enum { K210_MEDIA_AVI = 1, K210_MEDIA_MP3 = 2 } k210_media_kind_t;
/* Play a single file given its full path string (stock form or the /test plain path). */
k210_err_t k210_play_path(k210_link_t *l, k210_media_kind_t kind, bool loop, const char *path);
/* Play by logical name using the stock path form. */
k210_err_t k210_play_avi(k210_link_t *l, const char *name, bool loop);
k210_err_t k210_play_mp3(k210_link_t *l, const char *name, bool loop);
/* Play a full animation from a sub-2 JSON playlist (see k210_media.h). len excludes the NUL. */
k210_err_t k210_play_playlist(k210_link_t *l, const char *json, size_t len);

#ifdef __cplusplus
}
#endif
#endif
