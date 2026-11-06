#include "k210_internal.h"

k210_err_t k210_set_eye_mode(k210_link_t *l, uint16_t mode)
{
    if (mode < 1u || mode > 6u) {          /* 3 and 6 exist but their meaning is unverified */
        return K210_ERR_ARG;
    }
    return k210_send_short(l, K210_CMD_EYE_MODE, (uint8_t)(mode & 0xFFu), (uint8_t)(mode >> 8), 0);
}
