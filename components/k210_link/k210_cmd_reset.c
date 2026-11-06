#include "k210_internal.h"

k210_err_t k210_cmd_reset(k210_link_t *l)
{
    return k210_send_short(l, K210_CMD_RESET, 0, 0, 0);
}
