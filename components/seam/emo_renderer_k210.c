/* Mode 1: the stock K210 does all decoding and display. We only send names (as paths) in a sub-2 playlist. */
#include "emo_seam.h"

static int k210_play_media(void *ctx, const emo_anim_t *anim, int loop)
{
    emo_k210_renderer_t *r = (emo_k210_renderer_t *)ctx;
    if (!anim || anim->nclips == 0) {
        return EMO_ERR_ARG;
    }

    k210_playlist_t pl;
    if (k210_playlist_init(&pl, r->buf, sizeof r->buf, loop ? 1 : 0) != 0) {
        return EMO_ERR_ARG;
    }
    for (size_t i = 0; i < anim->nclips; i++) {
        const emo_clip_t *c = &anim->clips[i];
        if (k210_playlist_add_clip(&pl, c->avi, c->sounds, c->nsounds) != 0) {
            /* poisoned: bad path or over the K210's safe size */
            return EMO_ERR_MEDIA;
        }
    }
    size_t len = 0;
    const char *json = k210_playlist_finish(&pl, &len);
    if (!json) {
        return EMO_ERR_MEDIA;
    }
    return k210_play_playlist(r->link, json, len) == K210_OK ? EMO_OK : EMO_ERR_RENDER;
}

void emo_k210_renderer_init(emo_renderer_t *r, emo_k210_renderer_t *state, k210_link_t *link)
{
    state->link = link;
    state->buf[0] = '\0';
    r->play_media = k210_play_media;
    r->ctx = state;
}
