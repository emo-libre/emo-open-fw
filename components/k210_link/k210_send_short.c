#include "k210_internal.h"

k210_err_t k210_send_short(k210_link_t *l, uint8_t cmd, uint8_t b1, uint8_t b2, uint8_t b3)
{
    if (!l) {
        return K210_ERR_ARG;
    }
    uint8_t f[K210_SHORT_LEN];
    k210_build_short(f, cmd, b1, b2, b3);

    k210_lock(l);
    k210_err_t e = k210_write_all(l, f, sizeof f);
    if (e == K210_OK) {
        l->stat_tx_short++;
    }
    k210_frame_gap(l);
    k210_unlock(l);
    return e;
}
