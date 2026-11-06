#include <math.h>
#include "osc.h"

int osc_run(osc_t *o, double cycles, const osc_sink_t *sink, uint32_t *frames_out)
{
    if (frames_out) {
        *frames_out = 0;
    }
    if (!o || !sink || !sink->emit || !sink->pace || !isfinite(cycles) || cycles < 0.0) {
        return OSC_ERR;
    }
    if (sink->hold && sink->hold(sink->ctx)) {
        return OSC_HELD;                            /* held at the start: output nothing */
    }

    double total_d = floor(cycles * o->frames_per_cycle + 0.5);
    if (total_d > 4294967295.0) {
        return OSC_ERR;
    }
    const uint32_t total = (uint32_t)total_d;
    const uint32_t whole = (uint32_t)floor(cycles + 1e-9);   /* whole cycles in this run */

    uint32_t emitted = 0, whole_done = 0;
    int result = OSC_DONE;

    while (emitted < total) {
        osc_frame_t f;
        osc_step(o, &f);
        if (sink->emit(sink->ctx, &f) != 0) {
            result = OSC_ERR;
            break;
        }
        emitted++;

        /* whole cycle k is complete once k * frames_per_cycle frames have been emitted */
        while (whole_done < whole &&
               (double)emitted + 1e-9 >= (double)(whole_done + 1u) * o->frames_per_cycle) {
            whole_done++;
            o->cycles_done++;
            if (sink->cycle_done) {
                sink->cycle_done(sink->ctx);
            }
        }

        sink->pace(sink->ctx);
        if (sink->hold && sink->hold(sink->ctx)) {
            result = OSC_HELD;
            break;
        }
    }
    if (frames_out) {
        *frames_out = emitted;
    }
    return result;
}
