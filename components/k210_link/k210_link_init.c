#include <string.h>
#include "k210_internal.h"

static void rx_event(void *user, const k210_event_t *ev);

k210_err_t k210_link_init(k210_link_t *l, const k210_hal_t *hal, const k210_link_cfg_t *cfg,
                          uint8_t *rxbuf, uint16_t rxcap)
{
    if (!l || !hal || !hal->write || !hal->sleep_ms || !hal->ack_signal || !hal->ack_wait || !rxbuf || !rxcap) {
        return K210_ERR_ARG;
    }
    k210_link_cfg_t def = K210_LINK_CFG_DEFAULT;
    memset(l, 0, sizeof *l);
    l->hal = *hal;
    l->cfg = cfg ? *cfg : def;
    k210_rx_init(&l->rx, rxbuf, rxcap, rx_event, l);
    return K210_OK;
}

void k210_link_set_event_cb(k210_link_t *l, k210_event_cb cb, void *user)
{
    l->ev_cb = cb;
    l->ev_user = user;
}

/* Internal handling of the replies Phase 1 needs; every event is then forwarded to the user. */
static void rx_event(void *user, const k210_event_t *ev)
{
    k210_link_t *l = (k210_link_t *)user;

    if (ev->kind == K210_EV_SHORT) {
        if (ev->cmd == K210_CMD_ACK) {
            l->hal.ack_signal(l->hal.ctx, ev->b1);
        } else if (ev->cmd == K210_CMD_FW_NUMBER) {
            l->ver_word = (uint32_t)ev->b1 | ((uint32_t)ev->b2 << 8) | ((uint32_t)ev->b3 << 16);
            l->ver_valid = 1;
        }
    } else if (ev->kind == K210_EV_PAYLOAD && ev->crc_ok && ev->sub == K210_SUB_VERSION_TEXT) {
        size_t n = ev->len < sizeof l->ver_text - 1u ? ev->len : sizeof l->ver_text - 1u;
        memcpy(l->ver_text, ev->data, n);
        l->ver_text[n] = '\0';
        l->ver_text[sizeof l->ver_text - 1u] = '\0';
        l->vtxt_valid = 1;
    }

    if (l->ev_cb) {
        l->ev_cb(l->ev_user, ev);
    }
}
