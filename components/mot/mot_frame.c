#include <string.h>
#include "mot.h"

static uint8_t body_sum(const uint8_t *raw)
{
    unsigned s = 0;
    for (unsigned i = 4; i <= 17; i++) {
        s += raw[i];
    }
    return (uint8_t)s;
}

mot_err_t mot_frame_parse(const uint8_t raw[MOT_FRAME_LEN], mot_frame_t *out)
{
    if (!raw || !out) {
        return MOT_ERR_ARG;
    }
    if (raw[0] != 0x55 || raw[1] != 0xAA || raw[2] != 0x55 || raw[3] != 0xAA) {
        return MOT_ERR_MAGIC;
    }
    if (raw[18] != body_sum(raw)) {
        return MOT_ERR_SUM;
    }
    if (raw[19] != 0xED) {
        return MOT_ERR_END;
    }
    out->type = raw[4];
    out->sub = raw[5];
    memcpy(out->params, raw + 6, 5);
    memcpy(out->angle, raw + 11, MOT_SERVOS);
    out->b15 = raw[15];
    out->dur_ms = (uint16_t)(raw[16] | (raw[17] << 8));
    return MOT_OK;
}

void mot_frame_build(uint8_t raw[MOT_FRAME_LEN], const mot_frame_t *f)
{
    raw[0] = 0x55; raw[1] = 0xAA; raw[2] = 0x55; raw[3] = 0xAA;
    raw[4] = f->type;
    raw[5] = f->sub;
    memcpy(raw + 6, f->params, 5);
    memcpy(raw + 11, f->angle, MOT_SERVOS);
    raw[15] = f->b15;
    raw[16] = (uint8_t)(f->dur_ms & 0xFFu);
    raw[17] = (uint8_t)(f->dur_ms >> 8);
    raw[18] = body_sum(raw);
    raw[19] = 0xED;
}
