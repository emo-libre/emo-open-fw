/* Sub-2 JSON playlist builder:  [loop,[[avi,[[t,mp3],...]],...]]  (compact, no spaces). */
#include <string.h>
#include "k210_media.h"

/* Reserve room for the closing "]]" and the NUL so finish can never overflow. */
#define RESERVE 3u

static size_t limit(const k210_playlist_t *pl)
{
    size_t lim = pl->cap;
    if (lim > K210_PLAYLIST_SAFE_MAX) {      /* body + NUL must stay <= SAFE_MAX */
        lim = K210_PLAYLIST_SAFE_MAX;
    }
    return lim;
}

static void put(k210_playlist_t *pl, const char *s, size_t n)
{
    if (pl->err) {
        return;
    }
    if (pl->len + n + RESERVE > limit(pl)) {
        pl->err = 1;
        return;
    }
    memcpy(pl->buf + pl->len, s, n);
    pl->len += n;
    pl->buf[pl->len] = '\0';
}

static void put_ch(k210_playlist_t *pl, char c)
{
    put(pl, &c, 1);
}

static void put_u32(k210_playlist_t *pl, uint32_t v)
{
    char tmp[11];
    size_t i = sizeof tmp;
    do {
        tmp[--i] = (char)('0' + v % 10u);
        v /= 10u;
    } while (v);
    put(pl, tmp + i, sizeof tmp - i);
}

/* Quote a path: reject anything the path validator would reject, then no escaping is needed. */
static void put_str(k210_playlist_t *pl, const char *s)
{
    if (!k210_media_path_ok(s)) {
        pl->err = 1;
        return;
    }
    put_ch(pl, '"');
    put(pl, s, strlen(s));
    put_ch(pl, '"');
}

int k210_playlist_init(k210_playlist_t *pl, char *buf, size_t cap, int loop)
{
    if (!pl || !buf || cap < 16u || loop < 0 || loop > 255) {
        return -1;
    }
    memset(pl, 0, sizeof *pl);
    pl->buf = buf;
    pl->cap = cap;
    buf[0] = '\0';
    put_ch(pl, '[');
    put_u32(pl, (uint32_t)loop);
    put(pl, ",[", 2);
    return pl->err ? -1 : 0;
}

int k210_playlist_add_clip(k210_playlist_t *pl, const char *avi, const k210_sound_t *sounds, size_t nsounds)
{
    if (!pl || pl->err || pl->closed || !avi || (nsounds && !sounds)) {
        if (pl) {
            pl->err = 1;
        }
        return -1;
    }
    if (pl->clips) {
        put_ch(pl, ',');
    }
    put_ch(pl, '[');
    put_str(pl, avi);
    put(pl, ",[", 2);
    for (size_t i = 0; i < nsounds; i++) {
        if (i) {
            put_ch(pl, ',');
        }
        put_ch(pl, '[');
        put_u32(pl, sounds[i].t);
        put_ch(pl, ',');
        put_str(pl, sounds[i].mp3);
        put_ch(pl, ']');
    }
    put(pl, "]]", 2);
    if (pl->err) {
        return -1;
    }
    pl->clips++;
    return 0;
}

const char *k210_playlist_finish(k210_playlist_t *pl, size_t *len_out)
{
    if (!pl || pl->err || pl->closed) {
        return NULL;
    }
    /* put() always left RESERVE bytes free */
    pl->buf[pl->len++] = ']';
    pl->buf[pl->len++] = ']';
    pl->buf[pl->len] = '\0';
    pl->closed = 1;
    if (len_out) {
        *len_out = pl->len;
    }
    return pl->buf;
}
