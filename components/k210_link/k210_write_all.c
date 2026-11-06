#include "k210_internal.h"

k210_err_t k210_write_all(k210_link_t *l, const uint8_t *p, size_t n)
{
    while (n) {
        int w = l->hal.write(l->hal.ctx, p, n);
        if (w <= 0) {
            return K210_ERR_IO;
        }
        p += (size_t)w;
        n -= (size_t)w;
    }
    return K210_OK;
}

void k210_frame_gap(k210_link_t *l)
{
    if (l->cfg.gap_ms) {
        l->hal.sleep_ms(l->hal.ctx, l->cfg.gap_ms);
    }
}

void k210_lock(k210_link_t *l)
{
    if (l->hal.lock) {
        l->hal.lock(l->hal.ctx);
    }
}

void k210_unlock(k210_link_t *l)
{
    if (l->hal.unlock) {
        l->hal.unlock(l->hal.ctx);
    }
}
