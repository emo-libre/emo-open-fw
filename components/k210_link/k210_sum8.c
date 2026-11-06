#include "k210_proto.h"

uint8_t k210_sum8(const uint8_t *data, size_t len)
{
    uint8_t s = 0;
    while (len--) {
        s = (uint8_t)(s + *data++);
    }
    return s;
}
