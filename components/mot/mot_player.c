#include <math.h>
#include <string.h>
#include "mot.h"

void mot_player_init(mot_player_t *p, const int trim[MOT_SERVOS])
{
    for (int j = 0; j < MOT_SERVOS; j++) {
        p->last[j] = 90.0;
        p->trim[j] = trim ? trim[j] : 0;
    }
}

static double clamp_deg(double a)
{
    return a < 0.0 ? 0.0 : (a > 180.0 ? 180.0 : a);
}

/* trunc(angle * 2000 / 180 + 500 + 0.5) */
static uint16_t us_from_deg(double deg)
{
    return (uint16_t)(clamp_deg(deg) * 2000.0 / 180.0 + 500.0 + 0.5);
}

static int play_move(mot_player_t *p, const mot_frame_t *f, const mot_sink_t *sink, uint32_t *emitted)
{
    double target[MOT_SERVOS];
    for (int j = 0; j < MOT_SERVOS; j++) {
        target[j] = clamp_deg((double)f->angle[j]);
    }
    unsigned steps = f->dur_ms <= MOT_STEP_MS ? 1u : (unsigned)f->dur_ms / MOT_STEP_MS;

    for (unsigned i = 1; i <= steps; i++) {
        mot_out_t o;
        for (int j = 0; j < MOT_SERVOS; j++) {
            double a = p->last[j] + (target[j] - p->last[j]) * (double)i / (double)steps;
            double with_trim = clamp_deg(a + (double)p->trim[j]);
            o.abs[j] = (int)lround(with_trim);
            o.us[j] = us_from_deg(with_trim);
        }
        if (sink->emit(sink->ctx, &o) != 0) {
            return MOT_PLAY_ERR;
        }
        (*emitted)++;
        sink->pace(sink->ctx);
        if (sink->hold && sink->hold(sink->ctx)) {
            /* stop where we are: remember the position actually reached */
            for (int j = 0; j < MOT_SERVOS; j++) {
                p->last[j] = p->last[j] + (target[j] - p->last[j]) * (double)i / (double)steps;
            }
            return MOT_PLAY_HELD;
        }
    }
    memcpy(p->last, target, sizeof target);
    return MOT_PLAY_DONE;
}

int mot_play(mot_player_t *p, const uint8_t *buf, size_t len, const mot_sink_t *sink, uint32_t *frames_out)
{
    uint32_t emitted = 0;
    if (frames_out) {
        *frames_out = 0;
    }
    if (!p || !sink || !sink->emit || !sink->pace) {
        return MOT_PLAY_ERR;
    }
    size_t n;
    if (mot_validate(buf, len, &n, NULL) != MOT_OK) {
        return MOT_PLAY_BAD_FILE;
    }
    if (sink->hold && sink->hold(sink->ctx)) {
        return MOT_PLAY_HELD;
    }

    int result = MOT_PLAY_DONE;
    for (size_t i = 0; i < n && result == MOT_PLAY_DONE; i++) {
        mot_frame_t f;
        mot_frame_parse(buf + i * MOT_FRAME_LEN, &f);        /* already validated */
        if (f.sub == MOT_SUB_MOVE) {
            result = play_move(p, &f, sink, &emitted);
        } else if (sink->procedural) {
            if (sink->procedural(sink->ctx, &f) != 0) {
                result = MOT_PLAY_ERR;
            }
        }                                                    /* else: skipped */
    }
    if (frames_out) {
        *frames_out = emitted;
    }
    return result;
}
