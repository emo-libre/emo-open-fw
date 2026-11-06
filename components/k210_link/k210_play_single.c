/* Single-file playback: payload subs 24 (avi once) / 26 (avi loop) / 25 (mp3 once) / 27 (mp3 loop).
 * The body is the path string including its NUL [I]. */
#include <stdio.h>
#include <string.h>
#include "k210_internal.h"
#include "k210_media.h"

k210_err_t k210_play_path(k210_link_t *l, k210_media_kind_t kind, bool loop, const char *path)
{
    uint8_t sub;
    if (kind == K210_MEDIA_AVI) {
        sub = loop ? K210_SUB_AVI_LOOP : K210_SUB_AVI_ONCE;
    } else if (kind == K210_MEDIA_MP3) {
        sub = loop ? K210_SUB_MP3_LOOP : K210_SUB_MP3_ONCE;
    } else {
        return K210_ERR_ARG;
    }
    if (!k210_media_path_ok(path)) {
        return K210_ERR_ARG;
    }
    return k210_send_payload(l, sub, path, strlen(path) + 1u, true);
}

static k210_err_t play_named(k210_link_t *l, k210_media_kind_t kind, const char *dir, const char *ext,
                             const char *name, bool loop)
{
    char path[128];
    if (!k210_media_name_ok(name)) {
        return K210_ERR_ARG;
    }
    int n = snprintf(path, sizeof path, "/spiffs/%s/%s.%s", dir, name, ext);
    if (n < 0 || (size_t)n >= sizeof path) {
        return K210_ERR_ARG;
    }
    return k210_play_path(l, kind, loop, path);
}

k210_err_t k210_play_avi(k210_link_t *l, const char *name, bool loop)
{
    return play_named(l, K210_MEDIA_AVI, "avi", "avi", name, loop);
}

k210_err_t k210_play_mp3(k210_link_t *l, const char *name, bool loop)
{
    return play_named(l, K210_MEDIA_MP3, "mp3", "mp3", name, loop);
}
