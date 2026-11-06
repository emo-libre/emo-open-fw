#include "mot.h"

mot_err_t mot_validate(const uint8_t *buf, size_t len, size_t *nframes, size_t *bad_index)
{
    if (nframes) {
        *nframes = 0;
    }
    if (bad_index) {
        *bad_index = 0;
    }
    if (!buf) {
        return MOT_ERR_ARG;
    }
    if (len == 0 || len % MOT_FRAME_LEN != 0) {
        return MOT_ERR_SIZE;
    }
    size_t n = len / MOT_FRAME_LEN;
    for (size_t i = 0; i < n; i++) {
        mot_frame_t f;
        mot_err_t e = mot_frame_parse(buf + i * MOT_FRAME_LEN, &f);
        if (e != MOT_OK) {
            if (bad_index) {
                *bad_index = i;
            }
            return e;
        }
    }
    if (nframes) {
        *nframes = n;
    }
    return MOT_OK;
}
