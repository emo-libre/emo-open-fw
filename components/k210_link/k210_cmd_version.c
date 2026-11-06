/* Version handshake: cmd 60 (firmware number) and cmd 105 (version text). */
#include <string.h>
#include "k210_internal.h"

#define POLL_MS 10u

k210_err_t k210_query_version(k210_link_t *l, uint32_t *word, unsigned tries, uint32_t wait_ms)
{
    if (!l || !word || !tries) {
        return K210_ERR_ARG;
    }
    l->ver_valid = 0;
    for (unsigned t = 0; t < tries; t++) {
        k210_err_t e = k210_send_short(l, K210_CMD_FW_NUMBER, 0, 0, 0);
        if (e != K210_OK) {
            return e;
        }
        for (uint32_t waited = 0;; waited += POLL_MS) {
            if (l->ver_valid) {
                *word = l->ver_word;
                return K210_OK;
            }
            if (waited >= wait_ms) {
                break;
            }
            l->hal.sleep_ms(l->hal.ctx, POLL_MS);
        }
    }
    return K210_ERR_TIMEOUT;
}

/* Poll until the K210 answers or timeout_ms elapses (it needs a few seconds after a reset). */
k210_err_t k210_wait_ready(k210_link_t *l, uint32_t timeout_ms, uint32_t *word)
{
    const uint32_t per_try = 200u;      /* stock polls at 200 ms */
    uint32_t spent = 0;
    k210_err_t e = K210_ERR_TIMEOUT;

    while (spent < timeout_ms) {
        e = k210_query_version(l, word, 1, per_try);
        if (e == K210_OK) {
            return K210_OK;
        }
        if (e != K210_ERR_TIMEOUT) {
            return e;
        }
        spent += per_try + l->cfg.gap_ms;
    }
    return K210_ERR_NOT_READY;
}

k210_err_t k210_query_version_text(k210_link_t *l, char *out, size_t cap, uint32_t wait_ms)
{
    if (!l || !out || cap < 2) {
        return K210_ERR_ARG;
    }
    l->vtxt_valid = 0;
    k210_err_t e = k210_send_short(l, K210_CMD_VERSION_TEXT, 0, 0, 0);
    if (e != K210_OK) {
        return e;
    }
    for (uint32_t waited = 0;; waited += POLL_MS) {
        if (l->vtxt_valid) {
            strncpy(out, l->ver_text, cap - 1);
            out[cap - 1] = '\0';
            return K210_OK;
        }
        if (waited >= wait_ms) {
            return K210_ERR_TIMEOUT;
        }
        l->hal.sleep_ms(l->hal.ctx, POLL_MS);
    }
}
