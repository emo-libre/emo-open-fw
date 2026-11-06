#include <string.h>
#include "k210_proto.h"

size_t k210_build_payload(uint8_t *out, size_t cap, uint8_t sub, const void *body, size_t len)
{
    if (!out || len > 0xFFFFu || (len && !body)) {
        return 0;
    }
    size_t total = K210_SHORT_LEN + len + 1u;
    if (cap < total) {
        return 0;
    }
    k210_build_short(out, K210_CMD_PAYLOAD, (uint8_t)(len & 0xFFu), (uint8_t)(len >> 8), sub);
    if (len) {
        memcpy(out + K210_SHORT_LEN, body, len);
    }
    out[K210_SHORT_LEN + len] = k210_crc8_maxim((const uint8_t *)body, len);
    return total;
}
