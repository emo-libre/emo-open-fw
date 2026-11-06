#include "k210_internal.h"

#define K210_VOLUME_MAX 60u   /* values above are step/preset/lock codes, not levels */

k210_err_t k210_set_volume(k210_link_t *l, uint16_t level)
{
    if (level > K210_VOLUME_MAX) {
        return K210_ERR_ARG;
    }
    return k210_send_short(l, K210_CMD_VOLUME, (uint8_t)(level & 0xFFu), (uint8_t)(level >> 8), 0);
}
