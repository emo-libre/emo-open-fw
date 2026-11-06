#ifndef K210_INTERNAL_H
#define K210_INTERNAL_H

#include "k210_link.h"

/* Write all n bytes through the HAL. K210_OK or K210_ERR_IO. */
k210_err_t k210_write_all(k210_link_t *l, const uint8_t *p, size_t n);
/* Honour cfg.gap_ms after a frame. */
void k210_frame_gap(k210_link_t *l);
void k210_lock(k210_link_t *l);
void k210_unlock(k210_link_t *l);

#endif
