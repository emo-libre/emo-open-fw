/* Payload sender with ack-wait (stock: 500 ms) and bounded retry (DV-003).
 * Header, body and CRC are written separately under the link lock, so no big stack buffer. */
#include "k210_internal.h"

k210_err_t k210_send_payload(k210_link_t *l, uint8_t sub, const void *body, size_t len, bool wait_ack)
{
    if (!l || (len && !body)) {
        return K210_ERR_ARG;
    }
    if (len > l->cfg.max_payload || len > 0xFFFFu) {
        return K210_ERR_TOO_LONG;
    }

    uint8_t hdr[K210_SHORT_LEN];
    k210_build_short(hdr, K210_CMD_PAYLOAD, (uint8_t)(len & 0xFFu), (uint8_t)(len >> 8), sub);
    uint8_t crc = k210_crc8_maxim((const uint8_t *)body, len);

    unsigned attempts = 1u + (wait_ack ? l->cfg.max_retries : 0u);
    k210_err_t result = K210_ERR_IO;

    k210_lock(l);
    for (unsigned a = 0; a < attempts; a++) {
        if (wait_ack && l->hal.ack_flush) {
            l->hal.ack_flush(l->hal.ctx);
        }
        if (a) {
            l->stat_tx_retries++;
        }
        result = k210_write_all(l, hdr, sizeof hdr);
        if (result == K210_OK && len) {
            result = k210_write_all(l, (const uint8_t *)body, len);
        }
        if (result == K210_OK) {
            result = k210_write_all(l, &crc, 1);
        }
        if (result != K210_OK) {
            break;                                   /* I/O failure: do not retry */
        }
        l->stat_tx_payload++;
        if (!wait_ack) {
            break;
        }

        int ack = l->hal.ack_wait(l->hal.ctx, l->cfg.ack_timeout_ms);
        if (ack < 0) {
            l->stat_tx_timeout++;
            result = K210_ERR_TIMEOUT;
            continue;
        }
        if (ack == K210_ACK_BAD) {                   /* stock: only 0x55 is a rejection */
            l->stat_tx_nack++;
            result = K210_ERR_NACK;
            continue;
        }
        result = K210_OK;
        break;
    }
    k210_frame_gap(l);
    k210_unlock(l);
    return result;
}
