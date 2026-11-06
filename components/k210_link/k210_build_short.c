#include "k210_proto.h"

uint8_t k210_short_sum(uint8_t cmd, uint8_t b1, uint8_t b2, uint8_t b3)
{
    return (uint8_t)(K210_SYNC0 + K210_SYNC1 + cmd + b1 + b2 + b3);
}

void k210_build_short(uint8_t out[K210_SHORT_LEN], uint8_t cmd, uint8_t b1, uint8_t b2, uint8_t b3)
{
    out[0] = K210_SYNC0;
    out[1] = K210_SYNC1;
    out[2] = cmd;
    out[3] = b1;
    out[4] = b2;
    out[5] = b3;
    out[6] = k210_short_sum(cmd, b1, b2, b3);
}
