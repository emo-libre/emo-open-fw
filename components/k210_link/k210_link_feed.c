#include "k210_internal.h"

void k210_link_feed(k210_link_t *l, const uint8_t *data, size_t n)
{
    k210_rx_feed(&l->rx, data, n);
}
