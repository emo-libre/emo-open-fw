/* Builder for the sub-2 JSON media playlist:  [loop,[[avi,[[t,mp3],...]],...]]
 * Paths are supplied by the caller (this module knows no asset names). */
#ifndef K210_MEDIA_H
#define K210_MEDIA_H

#include <stddef.h>
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

/* The K210 has a ~1 KiB unchecked buffer for this body; stay under it (DV-002/DV-006). */
#define K210_PLAYLIST_SAFE_MAX 1000u

typedef struct { uint32_t t; const char *mp3; } k210_sound_t;   /* t: offset of the sound [I units] */

typedef struct {
    char    *buf;
    size_t   cap;
    size_t   len;
    uint16_t clips;
    uint8_t  err;
    uint8_t  closed;
} k210_playlist_t;

/* Start a playlist. loop: 0 = once, 1 = loop. buf must hold at least 16 bytes. Returns 0 or -1. */
int k210_playlist_init(k210_playlist_t *pl, char *buf, size_t cap, int loop);

/* Append one clip: an eye video path plus zero or more timed sounds. Returns 0, or -1 if the
 * arguments are bad or the playlist would exceed the buffer / K210_PLAYLIST_SAFE_MAX
 * (the playlist is then poisoned: finish returns NULL). */
int k210_playlist_add_clip(k210_playlist_t *pl, const char *avi, const k210_sound_t *sounds, size_t nsounds);

/* Close the playlist. Returns the NUL-terminated string (== buf) and its length, or NULL on error. */
const char *k210_playlist_finish(k210_playlist_t *pl, size_t *len_out);

/* Is this a path string we are willing to send? (printable ASCII, absolute, no "..", no quotes/backslash) */
int k210_media_path_ok(const char *path);
/* Is this a logical name we are willing to splice into a stock-form path? */
int k210_media_name_ok(const char *name);

#ifdef __cplusplus
}
#endif
#endif
