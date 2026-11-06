/* Full animation: payload sub 2 carries the JSON media playlist (NUL included). */
#include <string.h>
#include "k210_internal.h"
#include "k210_media.h"

k210_err_t k210_play_playlist(k210_link_t *l, const char *json, size_t len)
{
    if (!json || len == 0 || len >= K210_PLAYLIST_SAFE_MAX || json[len] != '\0' || strlen(json) != len) {
        return K210_ERR_ARG;
    }
    return k210_send_payload(l, K210_SUB_PLAYLIST, json, len + 1u, true);
}
