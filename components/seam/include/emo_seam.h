/* The seam between the brain and the three modes: an AssetProvider says what a logical animation name
 * resolves to; a Renderer plays the media. The brain only ever says "play(name)".
 * Mode 1 renderer = stock K210 plays by path/playlist. Portable: no ESP-IDF. */
#ifndef EMO_SEAM_H
#define EMO_SEAM_H

#include <stddef.h>
#include <stdint.h>
#include "k210_link.h"
#include "k210_media.h"

#ifdef __cplusplus
extern "C" {
#endif

/* One eye video with zero or more timed sounds (paths are supplied by the provider; this code knows no names). */
typedef struct {
    const char         *avi;
    const k210_sound_t *sounds;
    size_t              nsounds;
} emo_clip_t;

typedef struct {
    const char              *name;     /* logical animation name */
    const emo_clip_t        *clips;    /* eye video + sound, played by the renderer */
    size_t                   nclips;
    const char *const       *mot;      /* body-motion clip names, played by the ESP32 motion player */
    size_t                   nmot;
} emo_anim_t;

typedef enum {
    EMO_OK = 0,
    EMO_ERR_NOT_FOUND = -1,
    EMO_ERR_ARG = -2,
    EMO_ERR_RENDER = -3,
    EMO_ERR_MEDIA = -4      /* media cannot be sent: unsafe path or over the K210 safe size */
} emo_err_t;

/* ---- AssetProvider ---- */
typedef struct {
    /* Fill *out for `name`. Returned pointers must stay valid until the next find() call. */
    int  (*find)(void *ctx, const char *name, emo_anim_t *out);   /* EMO_OK or EMO_ERR_NOT_FOUND */
    void *ctx;
} emo_asset_provider_t;

/* ---- Renderer ---- */
typedef struct {
    int  (*play_media)(void *ctx, const emo_anim_t *anim, int loop);   /* EMO_OK or an error */
    void *ctx;
} emo_renderer_t;

/* Resolve `name`, hand the media to the renderer, and return the resolved animation (so the caller can start
 * its body motion). EMO_ERR_NOT_FOUND if the provider does not know it. A pure-motion animation (no clips)
 * is allowed: the renderer is not called. */
emo_err_t emo_play_animation(const emo_asset_provider_t *prov, const emo_renderer_t *rend,
                             const char *name, int loop, emo_anim_t *resolved);

/* ---- in-memory provider (tests, and the Mode 2 starter pack) ---- */
typedef struct {
    const emo_anim_t *table;
    size_t            count;
} emo_table_provider_t;

void emo_table_provider_init(emo_asset_provider_t *p, emo_table_provider_t *state,
                             const emo_anim_t *table, size_t count);

/* ---- Mode 1 renderer: stock K210 plays the media through the sub-2 JSON playlist ---- */
typedef struct {
    k210_link_t *link;
    char         buf[K210_PLAYLIST_SAFE_MAX + 1];   /* playlist scratch; the renderer is not re-entrant */
} emo_k210_renderer_t;

void emo_k210_renderer_init(emo_renderer_t *r, emo_k210_renderer_t *state, k210_link_t *link);

#ifdef __cplusplus
}
#endif
#endif
