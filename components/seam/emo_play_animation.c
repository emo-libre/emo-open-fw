#include <string.h>
#include "emo_seam.h"

emo_err_t emo_play_animation(const emo_asset_provider_t *prov, const emo_renderer_t *rend,
                             const char *name, int loop, emo_anim_t *resolved)
{
    if (!prov || !prov->find || !rend || !rend->play_media || !name || !*name || !resolved) {
        return EMO_ERR_ARG;
    }
    memset(resolved, 0, sizeof *resolved);
    int r = prov->find(prov->ctx, name, resolved);
    if (r != EMO_OK) {
        memset(resolved, 0, sizeof *resolved);
        return EMO_ERR_NOT_FOUND;
    }
    if (resolved->nclips == 0) {
        return EMO_OK;                              /* body-motion only */
    }
    int pr = rend->play_media(rend->ctx, resolved, loop);
    return pr == EMO_OK ? EMO_OK : (emo_err_t)pr;
}
