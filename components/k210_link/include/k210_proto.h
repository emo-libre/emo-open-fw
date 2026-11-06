/* Wire constants and pure helpers for the ESP32 <-> K210 UART link.
 * Specification: spec-clean/k210_link.md.  No ESP-IDF dependencies. */
#ifndef K210_PROTO_H
#define K210_PROTO_H

#include <stddef.h>
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

#define K210_SYNC0            0x55u
#define K210_SYNC1            0xAAu
#define K210_CMD_PAYLOAD      0xFFu   /* short-frame cmd that marks a payload frame */
#define K210_CMD_ACK          0xFEu   /* K210 -> ESP acknowledgement (254)          */
#define K210_ACK_GOOD         0xAAu
#define K210_ACK_BAD          0x55u
#define K210_SHORT_LEN        7u      /* 55 AA cmd b1 b2 b3 sum                     */
#define K210_PAYLOAD_OVERHEAD 8u      /* 7-byte header + 1 CRC byte                 */

/* ESP -> K210 short commands (tiers 0-1) */
#define K210_CMD_VOLUME       23u
#define K210_CMD_EYE_MODE     56u
#define K210_CMD_RESET        52u
#define K210_CMD_FW_NUMBER    60u     /* request; K210 answers with a cmd-60 short frame */
#define K210_CMD_VERSION_TEXT 105u    /* request; K210 answers with payload sub 105      */

/* Payload sub-types */
#define K210_SUB_PLAYLIST     2u      /* JSON media playlist           */
#define K210_SUB_AVI_ONCE     24u
#define K210_SUB_MP3_ONCE     25u
#define K210_SUB_AVI_LOOP     26u
#define K210_SUB_MP3_LOOP     27u
#define K210_SUB_VERSION_TEXT 105u    /* K210 -> ESP */

/* Largest body we will send. The K210 copies sub-2 style bodies into ~1 KiB
 * buffers with no bounds check, so stay well below it (docs/deviations.md DV-002). */
#define K210_DEFAULT_MAX_PAYLOAD 1000u

typedef enum {
    K210_OK            = 0,
    K210_ERR_ARG       = -1,
    K210_ERR_IO        = -2,
    K210_ERR_NACK      = -3,
    K210_ERR_TIMEOUT   = -4,
    K210_ERR_TOO_LONG  = -5,
    K210_ERR_NOT_READY = -6
} k210_err_t;

/* CRC-8/MAXIM: reflected poly 0x8C, init 0, no xorout. check("123456789") = 0xA1 */
uint8_t k210_crc8_maxim(const uint8_t *data, size_t len);

/* 8-bit additive sum (bulk SPI per-block check) */
uint8_t k210_sum8(const uint8_t *data, size_t len);

/* Header checksum of a short or payload-header frame */
uint8_t k210_short_sum(uint8_t cmd, uint8_t b1, uint8_t b2, uint8_t b3);

/* Build a 7-byte short frame into out[7]. */
void k210_build_short(uint8_t out[K210_SHORT_LEN], uint8_t cmd, uint8_t b1, uint8_t b2, uint8_t b3);

/* Build a payload frame (header + body + CRC) into out. Returns total bytes,
 * or 0 on bad arguments / insufficient capacity / len > 0xFFFF. */
size_t k210_build_payload(uint8_t *out, size_t cap, uint8_t sub, const void *body, size_t len);

#ifdef __cplusplus
}
#endif
#endif /* K210_PROTO_H */
